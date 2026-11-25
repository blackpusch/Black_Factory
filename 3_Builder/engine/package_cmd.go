package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

var uiExtensions = map[string]bool{
	".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true,
	".less": true, ".jsx": true, ".tsx": true, ".vue": true, ".svelte": true,
	".xaml": true, ".qml": true, ".ui": true,
}

var uiDirSegments = map[string]bool{
	"ui": true, "frontend": true, "web": true, "views": true,
	"components": true, "pages": true, "static": true,
}

var uxDirSegments = map[string]bool{
	"cmd": true, "cli": true, "skills": true,
}

// EvaluateReviewerApplicability calcula de forma determinista y sin override regresivo la
// aplicabilidad de los cuatro ejes de 4_Revisores segun Docs/designs/3_builder.md §4.2.
func EvaluateReviewerApplicability(changed []ChangedFileEntry, includeUI, includeUX bool) ReviewerApplicability {
	uiAuto := false
	uxSurfaceAuto := false

	for _, entry := range changed {
		norm := bfcore.NormalizeCoverPath(entry.Path)
		segs := strings.Split(norm, "/")
		base := ""
		if len(segs) > 0 {
			base = segs[len(segs)-1]
		}
		if entry.Status != "deleted" {
			ext := strings.ToLower(filepath.Ext(base))
			if uiExtensions[ext] {
				uiAuto = true
			}
			for _, seg := range segs[:max(0, len(segs)-1)] {
				if uiDirSegments[strings.ToLower(seg)] {
					uiAuto = true
					break
				}
			}
		}
		for _, seg := range segs {
			if uxDirSegments[strings.ToLower(seg)] {
				uxSurfaceAuto = true
				break
			}
		}
		lowBase := strings.ToLower(base)
		if lowBase == "readme.md" || strings.HasSuffix(lowBase, ".schema.json") {
			uxSurfaceAuto = true
		}
	}

	uiAxis := AxisApplicability{
		Applicable:  false,
		Mode:        "auto",
		Reason:      "no_gui_files_in_changed_files",
		ProfileRole: "reviewer_ui",
	}
	if uiAuto {
		uiAxis.Applicable = true
		uiAxis.Mode = "auto"
		uiAxis.Reason = "gui_surface_detected"
	} else if includeUI {
		uiAxis.Applicable = true
		uiAxis.Mode = "included"
		uiAxis.Reason = "cli_include_ui"
	}

	uxAxis := AxisApplicability{
		Applicable:  false,
		Mode:        "auto",
		Reason:      "internal_library_no_user_surface",
		ProfileRole: "reviewer_ux",
	}
	if uiAxis.Applicable || uxSurfaceAuto {
		uxAxis.Applicable = true
		uxAxis.Mode = "auto"
		uxAxis.Reason = "user_surface_detected"
	} else if includeUX {
		uxAxis.Applicable = true
		uxAxis.Mode = "included"
		uxAxis.Reason = "cli_include_ux"
	}

	return ReviewerApplicability{
		Security: AxisApplicability{
			Applicable: true, Mode: "always", Reason: "always_required", ProfileRole: "reviewer_security",
		},
		CodeArchitecture: AxisApplicability{
			Applicable: true, Mode: "always", Reason: "always_required", ProfileRole: "reviewer_code",
		},
		UX: uxAxis,
		UI: uiAxis,
	}
}

func verifyBrainFreshness(client bfcore.BrainClient, sctx SessionRuntimeContext, activeChunk RingChunk) (string, *CLIResult, int) {
	saved := sctx.Session.BrainContext
	if saved.ChunkID != activeChunk.ID {
		r, c := failResult(ExitExternalDecision, ReasonStaleBrainContext, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/brain_context/chunk_id",
			Code:    ReasonStaleBrainContext,
			Message: "el contexto de 0_Brain corresponde a otro chunk; ejecuta builder context antes de package",
		}})
		return "", &r, c
	}

	qFiles := effectiveQueriedFiles(sctx.Meta.Scope, activeChunk)
	liveSnap, err := QueryBrainSnapshot(client, sctx.Roots.VaultRoot, "", sctx.Session.Project, activeChunk.ID, qFiles)
	if err != nil {
		reason := ReasonBrainUnavailable
		if errors.Is(err, bfcore.ErrBrainJournalActive) {
			reason = ReasonBrainJournalActive
		} else if errors.Is(err, bfcore.ErrReparsePoint) {
			reason = ReasonReparsePointDetected
		}
		r, c := failResult(ExitInfraOrRefusal, reason, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/brain", Code: reason, Message: bfcore.RedactAllG4(err.Error()),
		}})
		return "", &r, c
	}

	if liveSnap.RiskZonesSHA256 != saved.RiskZonesSHA256 || len(liveSnap.MatchedDocHashes) != len(saved.MatchedDocHashes) {
		r, c := failResult(ExitExternalDecision, ReasonStaleBrainContext, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/brain_context",
			Code:    ReasonStaleBrainContext,
			Message: "0_Brain cambio respecto al ultimo snapshot; ejecuta builder context --refresh-brain",
		}})
		return "", &r, c
	}

	savedMap := make(map[string]string, len(saved.MatchedDocHashes))
	for _, h := range saved.MatchedDocHashes {
		savedMap[h.RelPath] = h.SHA256
	}
	for _, lh := range liveSnap.MatchedDocHashes {
		if savedMap[lh.RelPath] != lh.SHA256 {
			r, c := failResult(ExitExternalDecision, ReasonStaleBrainContext, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: "/brain_context/matched_doc_hashes",
				Code:    ReasonStaleBrainContext,
				Message: "documento de 0_Brain cambio respecto al ultimo snapshot; ejecuta builder context --refresh-brain",
			}})
			return "", &r, c
		}
	}
	for _, sh := range saved.MatchedDocHashes {
		state, diskHash, hErr := bfcore.ComputeNormalizedFileSHA256(sctx.Roots.VaultRoot, sh.RelPath)
		if errors.Is(hErr, bfcore.ErrReparsePoint) {
			r, c := failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			return "", &r, c
		}
		if hErr != nil || state != "present" || diskHash != sh.SHA256 {
			r, c := failResult(ExitExternalDecision, ReasonStaleBrainContext, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: "/brain_context/matched_doc_hashes",
				Code:    ReasonStaleBrainContext,
				Message: "archivo en vault cambio o desaparecio desde builder context; ejecuta --refresh-brain",
			}})
			return "", &r, c
		}
	}
	return liveSnap.BrainDBMTime, nil, ExitOK
}

// Package ejecuta builder package: verifica frescura 0_Brain, TDD Red previo, evolucion real,
// ejecuta todos los VerifyCommands, re-ejecuta el snapshot post-comando y emite DeliveryBundle + diff.
func Package(opts PackageOptions) (CLIResult, int) {
	start := time.Now()
	timeoutSec, ok := validateTimeoutSec(opts.TimeoutSec)
	if !ok {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
			Pointer: "/cli/timeout",
			Code:    ReasonInvalidUsage,
			Message: "--timeout debe estar entre 1 y 300 segundos",
		}})
	}

	sctx, errRes, code := LoadAndVerifySessionAndWorktree(opts.TaskDir, opts.RepoRoot, opts.FactoryRoot, opts.DataDir, opts.WorktreeRoot, opts.VaultRoot, true)
	if errRes != nil {
		return *errRes, code
	}
	defer func() { _ = sctx.Lock.Unlock() }()

	if sctx.Session.BounceBrakeActive {
		return failResult(ExitAttemptsBlocked, ReasonMaxBounceCycles, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}

	client := opts.BrainClient
	if client == nil {
		bin := sctx.Roots.BrainBin
		if strings.TrimSpace(opts.BrainBin) != "" {
			bin = opts.BrainBin
		}
		client = bfcore.NewSubprocessBrainClient(bin, 0)
	}
	if _, pfReason, pfErr := CheckBrainPreflight(client, sctx.Roots.VaultRoot, ""); pfErr != nil {
		return failResult(ExitInfraOrRefusal, pfReason, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/brain", Code: pfReason, Message: bfcore.RedactAllG4(pfErr.Error()),
		}})
	}

	gc := opts.Git
	if gc == nil {
		gc = ExecGitClient{}
	}
	ctx := context.Background()

	_, activeChunk, attemptNum, syncChanged, syncErr, syncCode := SyncSessionWithRingState(ctx, gc, &sctx, nil, true)
	if syncErr != nil {
		return *syncErr, syncCode
	}
	if syncChanged {
		if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
	}

	if headErr := checkWorktreeHeadMatchesBaseRef(ctx, gc, sctx); headErr != nil {
		return headErr.res, headErr.code
	}

	if _, staleRes, staleCode := verifyBrainFreshness(client, sctx, activeChunk); staleRes != nil {
		return *staleRes, staleCode
	}

	if sctx.Session.TDD.Red == nil || sctx.Session.RequiresRedTest {
		return failResult(ExitExternalDecision, ReasonTDDRedRequired, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/tdd/red",
			Code:    ReasonTDDRedRequired,
			Message: "se requiere evidencia TDD Red valida antes de empaquetar",
		}})
	}

	// [3] G4 pre-comando
	snapPre, _, g4Pre, g4PreCode := snapshotAndCheckG4(ctx, gc, sctx)
	if g4Pre != nil {
		return *g4Pre, g4PreCode
	}

	// [1] Reglas pre-comando: Scope, changed >= 1, evolucion real respecto a Red y ultimo BOUNCE
	if scopeRes, scopeCode := checkScopeCompliance(sctx, activeChunk, snapPre.ChangedFiles); scopeRes != nil {
		return *scopeRes, scopeCode
	}
	if len(snapPre.ChangedFiles) == 0 {
		return failResult(ExitFixable, ReasonTDDRedNoChanges, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if snapPre.WorktreeTreeSHA256 == sctx.Session.TDD.Red.RedWorktreeTreeSHA256 {
		return failResult(ExitFixable, ReasonTDDGreenUnchanged, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/git/worktree_tree_sha256",
			Code:    ReasonTDDGreenUnchanged,
			Message: "el worktree es identico a la fase Red; implementa los cambios minimos en Green antes de empaquetar",
		}})
	}
	if sctx.Session.BounceCycle > 0 && sctx.Session.LastBouncedWorktreeTreeSHA256 != "" &&
		snapPre.WorktreeTreeSHA256 == sctx.Session.LastBouncedWorktreeTreeSHA256 {
		return failResult(ExitFixable, ReasonTDDGreenUnchanged, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/git/worktree_tree_sha256",
			Code:    ReasonTDDGreenUnchanged,
			Message: "el worktree es identico al ultimo BOUNCE; aplica remediacion real antes de re-empaquetar",
		}})
	}

	runner := opts.Runner
	if runner == nil {
		runner = ShellRunner{Timeout: time.Duration(timeoutSec) * time.Second}
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if len(activeChunk.VerifyCommands) == 0 {
		return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/chunks/verify_commands",
			Code:    ReasonArtifactStateCorrupt,
			Message: "el chunk activo no declara VerifyCommands",
		}})
	}

	greenEvidence := make([]CommandEvidence, 0, len(activeChunk.VerifyCommands))
	for i, rawCmd := range activeChunk.VerifyCommands {
		ev, runErr := runner.Run(runCtx, sctx.AbsWorktree, rawCmd)
		sanCmd, excerpt, trunc, tier1, sanErr := SanitizeCommandOutputG4(rawCmd, ev.Output, MaxCommandOutputBytes)
		if tier1 != nil {
			return failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: fmt.Sprintf("/command_output/%d", i),
				Code:    ReasonG4Violation,
				Message: fmt.Sprintf("violacion G4 Tier 1 en salida de comando %d (%s)", i, tier1.Rule),
			}})
		}
		if sanErr != nil {
			return failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		}
		if runErr != nil {
			return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: fmt.Sprintf("/verify_commands/%d", i),
				Code:    ReasonIOError,
				Message: bfcore.RedactAllG4(runErr.Error()),
			}})
		}
		if ev.ExitCode != 0 {
			return failResult(ExitFixable, ReasonTDDGreenFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: fmt.Sprintf("/verify_commands/%d", i),
				Code:    ReasonTDDGreenFailed,
				Message: fmt.Sprintf("VerifyCommand[%d] fallo con exit %d: %s", i, ev.ExitCode, strings.TrimSpace(excerpt)),
			}})
		}
		greenEvidence = append(greenEvidence, CommandEvidence{
			CmdIndex:      i,
			Command:       sanCmd,
			ExitCode:      ev.ExitCode,
			DurationMS:    ev.DurationMs,
			OutputExcerpt: excerpt,
			Truncated:     trunc,
		})
	}

	// Re-ejecutar snapshot + G4 + Scope DESPUES de correr VerifyCommands (evita bypass por tests que dejan archivos sucios)
	snapPost, cleanDiff, g4Post, g4PostCode := snapshotAndCheckG4(ctx, gc, sctx)
	if g4Post != nil {
		return *g4Post, g4PostCode
	}
	if scopeRes, scopeCode := checkScopeCompliance(sctx, activeChunk, snapPost.ChangedFiles); scopeRes != nil {
		return *scopeRes, scopeCode
	}
	if len(snapPost.ChangedFiles) == 0 {
		return failResult(ExitFixable, ReasonTDDRedNoChanges, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if snapPost.WorktreeTreeSHA256 == sctx.Session.TDD.Red.RedWorktreeTreeSHA256 ||
		(sctx.Session.BounceCycle > 0 && sctx.Session.LastBouncedWorktreeTreeSHA256 != "" &&
			snapPost.WorktreeTreeSHA256 == sctx.Session.LastBouncedWorktreeTreeSHA256) {
		return failResult(ExitFixable, ReasonTDDGreenUnchanged, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}

	diffBytes := []byte(cleanDiff)
	if len(diffBytes) > MaxDiffBytes {
		return failResult(ExitFixable, ReasonDiffTooLarge, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	diffSum := sha256.Sum256(diffBytes)
	diffSHA := hex.EncodeToString(diffSum[:])

	stem := fmt.Sprintf("%s_a%d_r%d_c%d", activeChunk.ID, attemptNum, sctx.Session.ResetEpoch, sctx.Session.BounceCycle)
	diffRelPath := filepath.ToSlash(filepath.Join(sctx.Session.TaskID, "bundles", stem+".diff"))
	bundleRelPath := filepath.ToSlash(filepath.Join(sctx.Session.TaskID, "bundles", stem+".bundle.json"))
	diffAbsPath := filepath.Join(sctx.TaskStateDir, "bundles", stem+".diff")
	bundleAbsPath := filepath.Join(sctx.TaskStateDir, "bundles", stem+".bundle.json")

	now := time.Now().UTC().Format(time.RFC3339)
	effCanModify := activeChunk.Scope.CanModify
	if len(effCanModify) == 0 {
		effCanModify = sctx.Meta.Scope.CanModify
	}
	effReadOnly := append(append([]string{}, sctx.Meta.Scope.ReadOnly...), activeChunk.Scope.ReadOnly...)
	effForbidden := append(append([]string{}, sctx.Meta.Scope.Forbidden...), activeChunk.Scope.Forbidden...)
	if effCanModify == nil {
		effCanModify = []string{}
	}
	if effReadOnly == nil {
		effReadOnly = []string{}
	}
	if effForbidden == nil {
		effForbidden = []string{}
	}

	matchedDocs := sctx.Session.BrainContext.MatchedDocs
	if matchedDocs == nil {
		matchedDocs = []BrainDocSummary{}
	}
	riskHits := sctx.Session.BrainContext.RiskHits
	if riskHits == nil {
		riskHits = []bfcore.RiskHit{}
	}
	prevBounces := sctx.Session.PreviousBounceIDs
	if prevBounces == nil {
		prevBounces = []string{}
	}

	bundle := DeliveryBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      fmt.Sprintf("BND-%s-%s-a%d-r%d-c%d", sctx.Session.TaskID, activeChunk.ID, attemptNum, sctx.Session.ResetEpoch, sctx.Session.BounceCycle),
		CreatedAt:     now,
		TraceID:       sctx.Session.TraceID,
		WorkItemID:    sctx.Session.WorkItemID,
		SpecSHA256:    sctx.Session.SpecSHA256,
		Lane:          sctx.Session.Lane,
		TaskID:        sctx.Session.TaskID,
		ChunkID:       activeChunk.ID,
		GengisAttempt: attemptNum,
		ResetEpoch:    sctx.Session.ResetEpoch,
		BounceCycle:   sctx.Session.BounceCycle,
		Git: GitBundleInfo{
			TargetBranch:       sctx.Session.TargetBranch,
			BuildBranch:        sctx.Session.BuildBranch,
			BaseRef:            sctx.Session.ActiveBaseRef,
			WorktreeTreeSHA256: snapPost.WorktreeTreeSHA256,
			ChangedFiles:       snapPost.ChangedFiles,
			DiffRelPath:        diffRelPath,
			DiffSHA256:         diffSHA,
			DiffBytes:          int64(len(diffBytes)),
		},
		Scope: ScopeInfo{
			CanModify: effCanModify,
			ReadOnly:  effReadOnly,
			Forbidden: effForbidden,
		},
		BrainContext: BundleBrainInfo{
			BrainDBMTime:     sctx.Session.BrainContext.BrainDBMTime,
			RiskZonesSHA256:  sctx.Session.BrainContext.RiskZonesSHA256,
			HighestRiskLevel: sctx.Session.BrainContext.HighestRiskLevel,
			MatchedDocs:      matchedDocs,
			RiskHits:         riskHits,
		},
		TDDEvidence: TDDEvidence{
			Red:   *sctx.Session.TDD.Red,
			Green: greenEvidence,
		},
		ReviewerApplicability: EvaluateReviewerApplicability(snapPost.ChangedFiles, opts.IncludeUI, opts.IncludeUX),
		PreviousBounces:       append([]string{}, prevBounces...),
	}
	bundle = NormalizeBundleForCanonicalCopy(bundle)

	bundleSHA, err := CanonicalBundleSHA256(bundle)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if err := WriteFileAtomicBytes(diffAbsPath, diffBytes); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if err := WriteJSONAtomicG4(bundleAbsPath, bundle); err != nil {
		return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
	}

	sctx.Session.LatestBundleID = bundle.BundleID
	sctx.Session.LatestBundleSHA256 = bundleSHA
	sctx.Session.LatestWorktreeTreeSHA256 = snapPost.WorktreeTreeSHA256
	sctx.Session.ActiveBounceOrderID = ""
	sctx.Session.ApprovedBundleID = ""
	sctx.Session.ApprovedBundleSHA256 = ""
	sctx.Session.ApprovedWorktreeTreeSHA256 = ""
	sctx.Session.UpdatedAt = now

	if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
		return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
	}

	_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
		Event:        "PACKAGE",
		TraceID:      sctx.Session.TraceID,
		TaskID:       sctx.Session.TaskID,
		ChunkID:      activeChunk.ID,
		Attempt:      attemptNum,
		BounceCycle:  sctx.Session.BounceCycle,
		ExitCode:     ExitOK,
		Reason:       ReasonOK,
		BundleSHA256: bundleSHA,
		DurationMS:   time.Since(start).Milliseconds(),
		Detail:       fmt.Sprintf("bundle_id=%s diff_bytes=%d", bundle.BundleID, len(diffBytes)),
	})

	return CLIResult{
		OK:           true,
		ExitCode:     ExitOK,
		Reason:       ReasonOK,
		Retryable:    false,
		TraceID:      sctx.Session.TraceID,
		TaskID:       sctx.Session.TaskID,
		ChunkID:      activeChunk.ID,
		Attempt:      attemptNum,
		BounceCycle:  sctx.Session.BounceCycle,
		BuildBranch:  sctx.Session.BuildBranch,
		WorktreePath: sctx.AbsWorktree,
		BundleID:     bundle.BundleID,
		BundleSHA256: bundleSHA,
		BundlePath:   bundleRelPath,
		DiffPath:     diffRelPath,
	}, ExitOK
}
