package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

// SessionRuntimeContext agrupa las raices resueltas, el cerrojo activo y SessionState cargado.
type SessionRuntimeContext struct {
	Roots        ResolvedRoots
	AbsTaskDir   string
	AbsRepo      string
	AbsDataDir   string
	AbsWorktree  string
	TaskStateDir string
	SessionPath  string
	Lock         *bfcore.FileLock
	Session      SessionState
	Meta         ParsedIndexMetadata
}

// QueryBrainSnapshot consulta 0_Brain en solo lectura y calcula hashes de documentos activos.
func QueryBrainSnapshot(client bfcore.BrainClient, vaultRoot, dbPath, project, chunkID string, files []string) (BrainContextSnapshot, error) {
	pf, _, err := CheckBrainPreflight(client, vaultRoot, dbPath)
	if err != nil {
		return BrainContextSnapshot{}, err
	}
	resp, inv, err := client.Context(vaultRoot, pf.DBPath, project, files)
	if err != nil {
		return BrainContextSnapshot{}, err
	}
	docs := make([]BrainDocSummary, 0, len(resp.MatchedDocs))
	hashes := make([]MatchedDocHash, 0, len(resp.MatchedDocs))
	seenRel := map[string]bool{}
	for _, d := range resp.MatchedDocs {
		docs = append(docs, BrainDocSummary{
			ID:           d.ID,
			RelPath:      d.RelPath,
			Type:         d.Type,
			Status:       d.Status,
			VerifiedHead: d.VerifiedHead,
		})
		if d.RelPath != "" && !seenRel[d.RelPath] {
			seenRel[d.RelPath] = true
			state, sumHex, hErr := bfcore.ComputeNormalizedFileSHA256(vaultRoot, d.RelPath)
			if hErr == nil && state == "present" {
				hashes = append(hashes, MatchedDocHash{RelPath: d.RelPath, SHA256: sumHex})
			}
		}
	}

	highest := "NONE"
	maxRank := 0
	for _, rh := range resp.RiskHits {
		if r := bfcore.RiskLevelRank(rh.Level); r > maxRank {
			maxRank = r
			highest = strings.ToUpper(strings.TrimSpace(rh.Level))
		}
	}
	riskHits := resp.RiskHits
	if riskHits == nil {
		riskHits = []bfcore.RiskHit{}
	}
	qFiles := files
	if qFiles == nil {
		qFiles = []string{}
	}

	return BrainContextSnapshot{
		ChunkID:          chunkID,
		QueriedFiles:     qFiles,
		BrainDBMTime:     pf.BrainDBMTime,
		RiskZonesSHA256:  resp.RiskZonesSHA256,
		HighestRiskLevel: highest,
		MatchedDocs:      docs,
		MatchedDocHashes: hashes,
		RiskHits:         riskHits,
		Invocations:      []bfcore.BrainInvocation{inv},
		RefreshedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// LoadAndVerifySessionAndWorktree implementa [2] y [6a] de la precedencia unificada:
// reparse safety, cerrojo .session.lock, carga estricta de session.json e integridad del worktree/index.md.
// En exito retorna Lock abierto que el llamador debe liberar con defer sctx.Lock.Unlock().
func LoadAndVerifySessionAndWorktree(taskDir, repoRoot, factoryRoot, dataDir, worktreeRoot, vaultRoot string, requireActiveWorktree bool) (SessionRuntimeContext, *CLIResult, int) {
	if strings.TrimSpace(taskDir) == "" {
		r, c := failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
			Pointer: "/cli/task_dir", Code: ReasonInvalidUsage, Message: "--task-dir es obligatorio",
		}})
		return SessionRuntimeContext{}, &r, c
	}
	roots := ResolveCommonRoots(factoryRoot, repoRoot, vaultRoot, dataDir, "", "", taskDir, worktreeRoot)
	absTaskDir, err := filepath.Abs(taskDir)
	if err != nil {
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", "", nil)
		return SessionRuntimeContext{}, &r, c
	}
	absTaskDir = filepath.Clean(absTaskDir)
	taskID := filepath.Base(absTaskDir)

	if err := CheckReparseSafety(roots.FactoryRoot, roots.RepoRoot, roots.DataDir, absTaskDir, roots.VaultRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}

	_, repoSHA, absRepoClean, err := ComputePathIdentity(roots.RepoRoot)
	if err != nil {
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}
	_, _, absDataClean, err := ComputePathIdentity(roots.DataDir)
	if err != nil {
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}
	_, wtSHA, absWorktreeClean, err := ComputePathIdentity(filepath.Join(roots.WorktreeRoot, taskID))
	if err != nil {
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}

	taskStateDir := filepath.Join(absDataClean, taskID)
	sLock, err := AcquireSessionLock(taskStateDir)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			r, c := failResult(ExitInfraOrRefusal, ReasonLockBusy, "", taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}

	unlockOnFail := true
	defer func() {
		if unlockOnFail && sLock != nil {
			_ = sLock.Unlock()
		}
	}()

	st, err := LoadSessionStrict(taskStateDir)
	if err != nil {
		if errors.Is(err, ErrSessionMissing) {
			r, c := failResult(ExitUsage, ReasonSessionNotFound, "", taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		if errors.Is(err, ErrSessionCorrupt) {
			r, c := failResult(ExitInfraOrRefusal, ReasonSessionStateCorrupt, "", taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		r, c := failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}

	meta, err := ParseIndexFile(absTaskDir)
	if err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		r, c := failResult(ExitUsage, ReasonTaskDirInvalid, st.TraceID, taskID, nil)
		return SessionRuntimeContext{}, &r, c
	}

	if requireActiveWorktree {
		if st.WorktreeStatus != "active" {
			r, c := failResult(ExitInfraOrRefusal, ReasonWorktreeMissing, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		if st.WorktreePathSHA256 != wtSHA || st.RepoRootSHA256 != repoSHA {
			r, c := failResult(ExitInfraOrRefusal, ReasonWorktreePathMismatch, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		if err := bfcore.CheckNoReparseChain("", absWorktreeClean); err != nil {
			if errors.Is(err, bfcore.ErrReparsePoint) {
				r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, st.TraceID, taskID, nil)
				return SessionRuntimeContext{}, &r, c
			}
			r, c := failResult(ExitInfraOrRefusal, ReasonIOError, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		fi, lErr := os.Lstat(absWorktreeClean)
		if lErr != nil || !fi.IsDir() {
			r, c := failResult(ExitInfraOrRefusal, ReasonWorktreeMissing, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		boundRepo, bErr := ExtractIndexTargetRepo(filepath.Join(absTaskDir, "index.md"))
		if bErr != nil {
			if errors.Is(bErr, bfcore.ErrReparsePoint) {
				r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, st.TraceID, taskID, nil)
				return SessionRuntimeContext{}, &r, c
			}
			r, c := failResult(ExitInfraOrRefusal, ReasonWorktreePathMismatch, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
		if !strings.EqualFold(filepath.Clean(boundRepo), absWorktreeClean) {
			r, c := failResult(ExitInfraOrRefusal, ReasonWorktreePathMismatch, st.TraceID, taskID, nil)
			return SessionRuntimeContext{}, &r, c
		}
	}

	unlockOnFail = false
	return SessionRuntimeContext{
		Roots:        roots,
		AbsTaskDir:   absTaskDir,
		AbsRepo:      absRepoClean,
		AbsDataDir:   absDataClean,
		AbsWorktree:  absWorktreeClean,
		TaskStateDir: taskStateDir,
		SessionPath:  filepath.Join(taskStateDir, "session.json"),
		Lock:         sLock,
		Session:      st,
		Meta:         meta,
	}, nil, ExitOK
}

// BuildGengisDecideCommand construye la linea exacta entrecomillada de gengis.exe decide para [AUTOR].
func BuildGengisDecideCommand(gengisBin, taskDir, gengisRoot string, qaPass bool, summary string) string {
	note := strings.TrimSpace(summary)
	if note == "" {
		if qaPass {
			note = "Modo QA PASS"
		} else {
			note = "Modo QA FAIL"
		}
	}
	note = strings.NewReplacer("\r", " ", "\n", " ", `"`, "'", "`", "'", "$", "", "%", "").Replace(note)
	return fmt.Sprintf(`"%s" decide "%s" --root "%s" --qa-pass=%t --qa-notes="%s"`,
		gengisBin, filepath.Clean(taskDir), filepath.Clean(gengisRoot), qaPass, note)
}

// SyncSessionWithRingState sincroniza session.json con <taskDir>/.ring/state.json y .ring/base_ref.
func SyncSessionWithRingState(ctx context.Context, gc GitClient, sctx *SessionRuntimeContext, brainClient bfcore.BrainClient, requirePlanApproved bool) (*RingState, RingChunk, int, bool, *CLIResult, int) {
	ringSt, err := LoadRingState(sctx.AbsTaskDir)
	if err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return nil, RingChunk{}, 0, false, &r, c
		}
		r, c := failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		return nil, RingChunk{}, 0, false, &r, c
	}

	if ringSt == nil {
		if requirePlanApproved {
			r, c := failResult(ExitExternalDecision, ReasonPlanNotApproved, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return nil, RingChunk{}, 0, false, &r, c
		}
		chunks, cErr := ParseChunksDir(sctx.AbsTaskDir, sctx.AbsWorktree)
		if cErr != nil || len(chunks) == 0 {
			r, c := failResult(ExitUsage, ReasonTaskDirInvalid, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return nil, RingChunk{}, 0, false, &r, c
		}
		return nil, chunks[0], 1, false, nil, ExitOK
	}

	if ringSt.Status == "DONE" || ringSt.Status == "ABORTED" {
		r, c := failResult(ExitExternalDecision, ReasonTaskTerminalState, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		return ringSt, RingChunk{}, 0, false, &r, c
	}
	if !ringSt.PlanApproved {
		if requirePlanApproved {
			r, c := failResult(ExitExternalDecision, ReasonPlanNotApproved, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return ringSt, RingChunk{}, 0, false, &r, c
		}
		if len(ringSt.Chunks) == 0 {
			r, c := failResult(ExitUsage, ReasonTaskDirInvalid, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return ringSt, RingChunk{}, 0, false, &r, c
		}
		return ringSt, ringSt.Chunks[0], 1, false, nil, ExitOK
	}

	if ringSt.Index < 0 || ringSt.Index >= len(ringSt.Chunks) {
		r, c := failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		return ringSt, RingChunk{}, 0, false, &r, c
	}
	activeChunk := ringSt.Chunks[ringSt.Index]

	if n := len(activeChunk.Attempts); n > 0 {
		last := activeChunk.Attempts[n-1]
		if last.Verdict == nil {
			if last.QADone {
				gBin := ResolveGengisBin(sctx.Roots.GengisRoot, "")
				r, c := failResult(ExitExternalDecision, ReasonGengisDecideRequired, sctx.Session.TraceID, sctx.Session.TaskID, nil)
				r.ChunkID = activeChunk.ID
				r.Attempt = last.N
				r.NextAutorCommand = BuildGengisDecideCommand(gBin, sctx.AbsTaskDir, sctx.Roots.GengisRoot, last.QAPassed, last.QANotes)
				return ringSt, activeChunk, last.N, false, &r, c
			}
			r, c := failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return ringSt, activeChunk, last.N, false, &r, c
		}
	}

	attemptNum := len(activeChunk.Attempts) + 1
	windowChanged := sctx.Session.ActiveChunkID != activeChunk.ID || sctx.Session.ActiveAttempt != attemptNum
	chunkChanged := sctx.Session.ActiveChunkID != activeChunk.ID

	if windowChanged {
		baseRef := sctx.Session.ActiveBaseRef
		if rawBase, bErr := LoadRingBaseRef(sctx.AbsTaskDir); bErr == nil && rawBase != "" {
			if resolved, rErr := gc.RevParse(ctx, sctx.AbsWorktree, rawBase+"^{commit}"); rErr == nil && reHex40.MatchString(resolved) {
				baseRef = resolved
			}
		}
		sctx.Session.ActiveChunkID = activeChunk.ID
		sctx.Session.ActiveAttempt = attemptNum
		sctx.Session.ActiveBaseRef = baseRef
		sctx.Session.ResetEpoch = 0
		sctx.Session.BounceCycle = 0
		sctx.Session.BounceBrakeActive = false
		sctx.Session.PreviousBounceIDs = []string{}
		sctx.Session.SeenOrderIDs = []string{}
		sctx.Session.ActiveBounceOrderID = ""
		sctx.Session.RequiresRedTest = false
		sctx.Session.LastBouncedWorktreeTreeSHA256 = ""
		sctx.Session.LatestBundleID = ""
		sctx.Session.LatestBundleSHA256 = ""
		sctx.Session.LatestWorktreeTreeSHA256 = ""
		sctx.Session.ApprovedBundleID = ""
		sctx.Session.ApprovedBundleSHA256 = ""
		sctx.Session.ApprovedWorktreeTreeSHA256 = ""
		sctx.Session.TDD.Red = nil
		sctx.Session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		if chunkChanged && brainClient != nil {
			qFiles := effectiveQueriedFiles(sctx.Meta.Scope, activeChunk)
			if snap, qErr := QueryBrainSnapshot(brainClient, sctx.Roots.VaultRoot, "", sctx.Session.Project, activeChunk.ID, qFiles); qErr == nil {
				sctx.Session.BrainContext = snap
			}
		}
	}
	return ringSt, activeChunk, attemptNum, windowChanged, nil, ExitOK
}

// Context ejecuta builder context: sincroniza con .ring/state.json, consulta 0_Brain si aplica y
// devuelve el payload autocontenido ContextResult para el agente constructor.
func Context(opts ContextOptions) (ContextResult, int) {
	start := time.Now()
	sctx, errRes, code := LoadAndVerifySessionAndWorktree(opts.TaskDir, opts.RepoRoot, opts.FactoryRoot, opts.DataDir, opts.WorktreeRoot, opts.VaultRoot, true)
	if errRes != nil {
		return contextErrorFromCLI(*errRes), code
	}
	defer func() { _ = sctx.Lock.Unlock() }()

	gc := opts.Git
	if gc == nil {
		gc = ExecGitClient{}
	}
	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(sctx.Roots.BrainBin, 0)
		if strings.TrimSpace(opts.BrainBin) != "" {
			client = bfcore.NewSubprocessBrainClient(opts.BrainBin, 0)
		}
	}

	if _, pfReason, pfErr := CheckBrainPreflight(client, sctx.Roots.VaultRoot, ""); pfErr != nil {
		return contextErrorFromCLI(CLIResult{
			ExitCode: ExitInfraOrRefusal, Reason: pfReason, Retryable: IsRetryableReason(ExitInfraOrRefusal, pfReason),
			TraceID: sctx.Session.TraceID, TaskID: sctx.Session.TaskID,
			Issues: []ValidationIssue{{Pointer: "/brain", Code: pfReason, Message: bfcore.RedactAllG4(pfErr.Error())}},
		}), ExitInfraOrRefusal
	}

	ringSt, activeChunk, attemptNum, changed, syncErr, syncCode := SyncSessionWithRingState(context.Background(), gc, &sctx, client, false)
	if syncErr != nil {
		return contextErrorFromCLI(*syncErr), syncCode
	}

	prevRiskLevel := sctx.Session.BrainContext.HighestRiskLevel
	if opts.RefreshBrain {
		qFiles := effectiveQueriedFiles(sctx.Meta.Scope, activeChunk)
		snap, qErr := QueryBrainSnapshot(client, sctx.Roots.VaultRoot, "", sctx.Session.Project, activeChunk.ID, qFiles)
		if qErr != nil {
			return contextErrorFromCLI(CLIResult{
				ExitCode: ExitInfraOrRefusal, Reason: ReasonBrainUnavailable, Retryable: true,
				TraceID: sctx.Session.TraceID, TaskID: sctx.Session.TaskID,
			}), ExitInfraOrRefusal
		}
		sctx.Session.BrainContext = snap
		sctx.Session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		changed = true

		riskNote := fmt.Sprintf("risk=%s", snap.HighestRiskLevel)
		if prevRiskLevel != "" && prevRiskLevel != snap.HighestRiskLevel {
			riskNote = fmt.Sprintf("risk_changed=%s->%s", prevRiskLevel, snap.HighestRiskLevel)
		}
		_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
			Event:       "BRAIN_CONTEXT_REFRESHED",
			TraceID:     sctx.Session.TraceID,
			TaskID:      sctx.Session.TaskID,
			ChunkID:     activeChunk.ID,
			Attempt:     attemptNum,
			BounceCycle: sctx.Session.BounceCycle,
			ExitCode:    ExitOK,
			Reason:      ReasonOK,
			DurationMS:  time.Since(start).Milliseconds(),
			Detail:      riskNote,
		})
	}

	if changed {
		if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
			return contextErrorFromCLI(CLIResult{
				ExitCode: ExitInfraOrRefusal, Reason: ReasonIOError, Retryable: true,
				TraceID: sctx.Session.TraceID, TaskID: sctx.Session.TaskID,
			}), ExitInfraOrRefusal
		}
	}

	var activeBounce *BounceOrder
	if sctx.Session.ActiveBounceOrderID != "" {
		activeBounce = loadBounceByOrderID(filepath.Join(sctx.TaskStateDir, "bounces"), sctx.Session.ActiveBounceOrderID)
	}

	effCanModify := activeChunk.Scope.CanModify
	if len(effCanModify) == 0 {
		effCanModify = sctx.Meta.Scope.CanModify
	}
	effReadOnly := append(append([]string{}, sctx.Meta.Scope.ReadOnly...), activeChunk.Scope.ReadOnly...)
	effForbidden := append(append([]string{}, sctx.Meta.Scope.Forbidden...), activeChunk.Scope.Forbidden...)
	planApproved := ringSt != nil && ringSt.PlanApproved

	return ContextResult{
		OK:                 true,
		ExitCode:           ExitOK,
		Reason:             ReasonOK,
		Retryable:          false,
		TraceID:            sctx.Session.TraceID,
		TaskID:             sctx.Session.TaskID,
		WorktreePath:       sctx.AbsWorktree,
		BuildBranch:        sctx.Session.BuildBranch,
		TargetBranch:       sctx.Session.TargetBranch,
		PlanApproved:       planApproved,
		ChunkID:            activeChunk.ID,
		Attempt:            attemptNum,
		BounceCycle:        sctx.Session.BounceCycle,
		BaseRef:            sctx.Session.ActiveBaseRef,
		Spec:               activeChunk.Spec,
		Acceptance:         nonNilStrings(activeChunk.Acceptance),
		VerifyCommands:     nonNilStrings(activeChunk.VerifyCommands),
		FailureProof:       activeChunk.FailureProof,
		EffectiveCanModify: nonNilStrings(effCanModify),
		EffectiveReadOnly:  effReadOnly,
		EffectiveForbidden: effForbidden,
		GengisFeedback:     nonNilStrings(activeChunk.Feedback),
		BrainContext:       sctx.Session.BrainContext,
		ActiveBounce:       activeBounce,
	}, ExitOK
}

func effectiveQueriedFiles(taskScope Scope, chunk RingChunk) []string {
	if len(chunk.Scope.CanModify) > 0 {
		return chunk.Scope.CanModify
	}
	return taskScope.CanModify
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func contextErrorFromCLI(r CLIResult) ContextResult {
	return ContextResult{
		OK:                 false,
		ExitCode:           r.ExitCode,
		Reason:             r.Reason,
		Retryable:          r.Retryable,
		TraceID:            r.TraceID,
		TaskID:             r.TaskID,
		ChunkID:            r.ChunkID,
		Attempt:            r.Attempt,
		Acceptance:         []string{},
		VerifyCommands:     []string{},
		EffectiveCanModify: []string{},
		EffectiveReadOnly:  []string{},
		EffectiveForbidden: []string{},
		GengisFeedback:     []string{},
	}
}

func loadBounceByOrderID(bouncesDir, orderID string) *BounceOrder {
	entries, err := os.ReadDir(bouncesDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".order.json") {
			continue
		}
		raw, rErr := ReadFileWithShortRetry(filepath.Join(bouncesDir, e.Name()))
		if rErr != nil {
			continue
		}
		var bo BounceOrder
		dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
		if dec.Decode(&bo) == nil && bo.OrderID == orderID {
			return &bo
		}
	}
	return nil
}
