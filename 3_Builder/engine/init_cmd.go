package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

// Init ejecuta builder init: valida en seco todas las precondiciones antes de mutar disco,
// crea la rama aislada y git worktree bajo .session.lock -> .repo-<sha16>.lock, vincula la
// unica linea Repositorio destino en <taskDir>/index.md y guarda session.json con rollback total.
func Init(opts InitOptions) (CLIResult, int) {
	start := time.Now()
	if strings.TrimSpace(opts.TaskDir) == "" {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
			Pointer: "/cli/task_dir",
			Code:    ReasonInvalidUsage,
			Message: "--task-dir es obligatorio",
		}})
	}

	roots := ResolveCommonRoots(opts.FactoryRoot, opts.RepoRoot, opts.VaultRoot, opts.DataDir, opts.BrainBin, opts.GengisRoot, opts.TaskDir, opts.WorktreeRoot)
	absTaskDir, err := filepath.Abs(opts.TaskDir)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", nil)
	}
	absTaskDir = filepath.Clean(absTaskDir)
	taskID := filepath.Base(absTaskDir)
	if !IsValidTaskID(taskID) {
		return failResult(ExitUsage, ReasonTaskDirInvalid, "", taskID, []ValidationIssue{{
			Pointer: "/task_dir",
			Code:    ReasonTaskDirInvalid,
			Message: fmt.Sprintf("task_id %q invalido (patron ^[a-z0-9][a-z0-9_-]{2,63}$, sin nombres reservados)", taskID),
		}})
	}

	if err := CheckReparseSafety(roots.FactoryRoot, roots.RepoRoot, roots.VaultRoot, roots.DataDir, absTaskDir, roots.WorktreeRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}

	repoBase, repoSHA, absRepoClean, err := ComputePathIdentity(roots.RepoRoot)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}
	_, factorySHA, absFactoryClean, err := ComputePathIdentity(roots.FactoryRoot)
	_ = factorySHA
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}
	_, _, absDataClean, err := ComputePathIdentity(roots.DataDir)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}
	_, _, absWorktreeRootClean, err := ComputePathIdentity(roots.WorktreeRoot)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}
	absWorktreePath := filepath.Join(absWorktreeRootClean, taskID)
	wtBase, wtSHA, absWorktreeClean, err := ComputePathIdentity(absWorktreePath)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}
	if err := CheckReparseSafety(absWorktreeClean); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}

	// Verificar no solapamiento entre --worktree-root / worktree y raices del repo/factory/runtime/task
	for _, rootToCheck := range []struct {
		label string
		path  string
	}{
		{"repo_root", absRepoClean},
		{"factory_root", absFactoryClean},
		{"data_dir", absDataClean},
		{"task_dir", absTaskDir},
	} {
		if pathsOverlap(absWorktreeRootClean, rootToCheck.path) || pathsOverlap(absWorktreeClean, rootToCheck.path) {
			return failResult(ExitUsage, ReasonInvalidUsage, "", taskID, []ValidationIssue{{
				Pointer: "/cli/worktree_root",
				Code:    ReasonInvalidUsage,
				Message: fmt.Sprintf("--worktree-root no puede solaparse con %s", rootToCheck.label),
			}})
		}
	}

	taskStateDir := filepath.Join(absDataClean, taskID)
	sessionPath := filepath.Join(taskStateDir, "session.json")
	if _, err := os.Lstat(sessionPath); err == nil {
		return failResult(ExitUsage, ReasonSessionAlreadyExists, "", taskID, []ValidationIssue{{
			Pointer: "/session",
			Code:    ReasonSessionAlreadyExists,
			Message: "ya existe session.json para esta tarea",
		}})
	} else if !errors.Is(err, os.ErrNotExist) {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", taskID, nil)
	}

	// Validaciones 100% read-only de index.md y estado del anillo antes de mutar disco
	indexPath := filepath.Join(absTaskDir, "index.md")
	meta, err := ParseIndexFile(absTaskDir)
	if err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", taskID, nil)
		}
		return failResult(ExitUsage, ReasonTaskDirInvalid, "", taskID, []ValidationIssue{{
			Pointer: "/index",
			Code:    ReasonTaskDirInvalid,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}
	if meta.RepoLineCount != 1 {
		return failResult(ExitInfraOrRefusal, ReasonIndexBindInvariantBreak, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/index/repositorio_destino",
			Code:    ReasonIndexBindInvariantBreak,
			Message: fmt.Sprintf("se esperaba exactamente 1 linea 'Repositorio destino', halladas %d", meta.RepoLineCount),
		}})
	}
	if meta.SpecSHA256 != "" && !reHex64.MatchString(meta.SpecSHA256) {
		return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/index/spec_sha256",
			Code:    ReasonTaskDirInvalid,
			Message: "Spec SHA-256 invalido en index.md",
		}})
	}
	if meta.WorkItemID != "" && !reWorkItemID.MatchString(meta.WorkItemID) {
		return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/index/work_item",
			Code:    ReasonTaskDirInvalid,
			Message: "WorkItem ID invalido en index.md",
		}})
	}
	if meta.Lane != "" && meta.Lane != "FTD" && meta.Lane != "ASDD" {
		return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/index/lane",
			Code:    ReasonTaskDirInvalid,
			Message: fmt.Sprintf("carril %q invalido en index.md (solo FTD o ASDD)", meta.Lane),
		}})
	}
	if meta.TargetRepo != "" && meta.TargetRepo != "<TARGET_REPO>" &&
		!strings.EqualFold(filepath.Clean(meta.TargetRepo), absRepoClean) &&
		!strings.EqualFold(filepath.Clean(meta.TargetRepo), absWorktreeClean) {
		return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/index/repositorio_destino",
			Code:    ReasonTaskDirInvalid,
			Message: "Repositorio destino existente en index.md no coincide con --repo-root",
		}})
	}

	gc := opts.Git
	if gc == nil {
		gc = ExecGitClient{}
	}
	ctx := context.Background()

	ringSt, err := LoadRingState(absTaskDir)
	if err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, meta.TraceID, taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, meta.TraceID, taskID, nil)
	}

	var (
		baseRef40 string
		chunks    []RingChunk
	)
	if ringSt != nil {
		// Secuencia B (post-plan): solo permitida cuando PlanApproved && RUNNING && Index==0 && 0 intentos
		if !ringSt.PlanApproved || ringSt.Status != "RUNNING" || ringSt.Index != 0 || len(ringSt.Chunks) == 0 || len(ringSt.Chunks[0].Attempts) != 0 {
			return failResult(ExitInfraOrRefusal, ReasonTaskAlreadyActive, meta.TraceID, taskID, []ValidationIssue{{
				Pointer: "/ring/state",
				Code:    ReasonTaskAlreadyActive,
				Message: "Secuencia B rechazada: la tarea ya inicio intentos, no esta aprobada o ya cerro",
			}})
		}
		rawBaseRef, bErr := LoadRingBaseRef(absTaskDir)
		if bErr != nil {
			if errors.Is(bErr, bfcore.ErrReparsePoint) {
				return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, meta.TraceID, taskID, nil)
			}
			return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, meta.TraceID, taskID, nil)
		}
		if rawBaseRef == "" {
			return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, meta.TraceID, taskID, nil)
		}
		resolved, rErr := gc.RevParse(ctx, absRepoClean, rawBaseRef+"^{commit}")
		if rErr != nil || !reHex40.MatchString(resolved) {
			return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, meta.TraceID, taskID, nil)
		}
		baseRef40 = resolved
		chunks = ringSt.Chunks
	} else {
		// Secuencia A (pre-plan)
		refCandidate := strings.TrimSpace(opts.BaseRef)
		explicitRef := refCandidate != ""
		if !explicitRef && strings.TrimSpace(meta.TargetBranch) != "" {
			refCandidate = strings.TrimSpace(meta.TargetBranch)
			explicitRef = true
		}
		if refCandidate == "" {
			refCandidate = "HEAD"
		}
		resolved, rErr := gc.RevParse(ctx, absRepoClean, refCandidate+"^{commit}")
		if rErr != nil || !reHex40.MatchString(resolved) {
			if explicitRef {
				return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
					Pointer: "/cli/base_ref",
					Code:    ReasonTaskDirInvalid,
					Message: fmt.Sprintf("referencia base %q no resuelve a un commit valido", refCandidate),
				}})
			}
			return failResult(ExitInfraOrRefusal, ReasonGitError, meta.TraceID, taskID, nil)
		}
		baseRef40 = resolved
		parsedChunks, cErr := ParseChunksDir(absTaskDir, absRepoClean)
		if cErr != nil {
			if errors.Is(cErr, bfcore.ErrReparsePoint) {
				return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, meta.TraceID, taskID, nil)
			}
			return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
				Pointer: "/chunks",
				Code:    ReasonTaskDirInvalid,
				Message: bfcore.RedactAllG4(cErr.Error()),
			}})
		}
		chunks = parsedChunks
	}

	if len(chunks) == 0 {
		return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, nil)
	}
	for _, ch := range chunks {
		if len(ch.VerifyCommands) == 0 {
			return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
				Pointer: "/chunks/" + ch.ID + "/verify_commands",
				Code:    ReasonTaskDirInvalid,
				Message: "chunk sin VerifyCommands",
			}})
		}
	}

	trackedPaths, err := gc.LsTreePaths(ctx, absRepoClean, baseRef40)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, meta.TraceID, taskID, nil)
	}
	if ptr, msg := ValidateVerifyCommandsWorktreeCompat(chunks, trackedPaths); ptr != "" {
		return failResult(ExitUsage, ReasonTaskDirInvalid, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: ptr,
			Code:    ReasonTaskDirInvalid,
			Message: msg,
		}})
	}

	buildBranch := strings.TrimSpace(opts.Branch)
	if buildBranch == "" {
		buildBranch = "build/" + taskID
	}
	if err := gc.CheckBranchFormat(ctx, absRepoClean, buildBranch); err != nil {
		return failResult(ExitUsage, ReasonInvalidUsage, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/cli/branch",
			Code:    ReasonInvalidUsage,
			Message: fmt.Sprintf("nombre de rama %q invalido", buildBranch),
		}})
	}
	if exists, err := gc.BranchExists(ctx, absRepoClean, buildBranch); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, meta.TraceID, taskID, nil)
	} else if exists {
		return failResult(ExitExternalDecision, ReasonBranchCollision, meta.TraceID, taskID, []ValidationIssue{{
			Pointer: "/cli/branch",
			Code:    ReasonBranchCollision,
			Message: fmt.Sprintf("la rama %q ya existe en el repositorio", buildBranch),
		}})
	}
	if wtBranches, err := gc.ListWorktreeBranches(ctx, absRepoClean); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, meta.TraceID, taskID, nil)
	} else {
		for _, b := range wtBranches {
			if b == buildBranch {
				return failResult(ExitExternalDecision, ReasonBranchCollision, meta.TraceID, taskID, nil)
			}
		}
	}

	traceID := meta.TraceID
	if traceID == "" {
		traceID = DeterministicFallbackTraceID(taskID, baseRef40)
	}

	// Preflight + Context read-only contra 0_Brain ANTES de mutar disco
	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(roots.BrainBin, 0)
	}
	pf, pfReason, pfErr := CheckBrainPreflight(client, roots.VaultRoot, "")
	if pfErr != nil {
		return failResult(ExitInfraOrRefusal, pfReason, traceID, taskID, []ValidationIssue{{
			Pointer: "/brain",
			Code:    pfReason,
			Message: bfcore.RedactAllG4(pfErr.Error()),
		}})
	}
	queriedFiles := collectUnionCanModify(meta.Scope, chunks)
	brainSnap, bErr := QueryBrainSnapshot(client, roots.VaultRoot, pf.DBPath, strings.TrimSpace(opts.Project), chunks[0].ID, queriedFiles)
	if bErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonBrainUnavailable, traceID, taskID, []ValidationIssue{{
			Pointer: "/brain",
			Code:    ReasonBrainUnavailable,
			Message: bfcore.RedactAllG4(bErr.Error()),
		}})
	}

	// Cerrojo granular .session.lock -> re-verificacion -> .repo-<sha16>.lock
	sLock, err := AcquireSessionLock(taskStateDir)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return failResult(ExitInfraOrRefusal, ReasonLockBusy, traceID, taskID, nil)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, traceID, taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
	}
	defer func() { _ = sLock.Unlock() }()

	if _, err := os.Lstat(sessionPath); err == nil {
		return failResult(ExitUsage, ReasonSessionAlreadyExists, traceID, taskID, nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
	}

	rLock, err := AcquireRepoLock(absDataClean, repoSHA)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return failResult(ExitInfraOrRefusal, ReasonLockBusy, traceID, taskID, nil)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, traceID, taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
	}
	defer func() { _ = rLock.Unlock() }()

	// Re-verificar colision de rama bajo .repo-<sha16>.lock (cierra ventana TOCTOU pre-lock)
	if exists, bErr := gc.BranchExists(ctx, absRepoClean, buildBranch); bErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, traceID, taskID, nil)
	} else if exists {
		return failResult(ExitExternalDecision, ReasonBranchCollision, traceID, taskID, []ValidationIssue{{
			Pointer: "/cli/branch",
			Code:    ReasonBranchCollision,
			Message: fmt.Sprintf("la rama %q ya existe en el repositorio", buildBranch),
		}})
	}
	if wtBranches, bErr := gc.ListWorktreeBranches(ctx, absRepoClean); bErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, traceID, taskID, nil)
	} else {
		for _, b := range wtBranches {
			if b == buildBranch {
				return failResult(ExitExternalDecision, ReasonBranchCollision, traceID, taskID, nil)
			}
		}
	}

	origIndexBytes, err := ReadFileWithShortRetry(indexPath)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
	}
	if err := os.MkdirAll(absWorktreeRootClean, 0o755); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
	}

	committed := false
	wtCreated := false
	indexBound := false
	defer func() {
		if committed {
			return
		}
		if indexBound {
			_ = WriteFileAtomicBytes(indexPath, origIndexBytes)
		}
		if wtCreated {
			_ = gc.WorktreeRemove(context.Background(), absRepoClean, absWorktreeClean, true)
			_ = gc.WorktreePrune(context.Background(), absRepoClean)
			_ = gc.DeleteBranch(context.Background(), absRepoClean, buildBranch)
		}
	}()

	if err := gc.WorktreeAdd(ctx, absRepoClean, absWorktreeClean, buildBranch, baseRef40); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, traceID, taskID, []ValidationIssue{{
			Pointer: "/worktree",
			Code:    ReasonGitError,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}
	wtCreated = true

	indexBound = true
	if err := BindIndexTargetRepo(indexPath, absWorktreeClean); err != nil {
		if errors.Is(err, ErrG4Violation) {
			return failResult(ExitG4, ReasonG4Violation, traceID, taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIndexBindInvariantBreak, traceID, taskID, []ValidationIssue{{
			Pointer: "/index/repositorio_destino",
			Code:    ReasonIndexBindInvariantBreak,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}

	now := time.Now().UTC().Format(time.RFC3339)
	st := SessionState{
		SchemaVersion:      SchemaVersion,
		TaskID:             taskID,
		Project:            strings.TrimSpace(opts.Project),
		WorkItemID:         meta.WorkItemID,
		TraceID:            traceID,
		SpecSHA256:         meta.SpecSHA256,
		Lane:               meta.Lane,
		RepoRootBasename:   repoBase,
		RepoRootSHA256:     repoSHA,
		WorktreeBasename:   wtBase,
		WorktreePathSHA256: wtSHA,
		TargetBranch:       meta.TargetBranch,
		BuildBranch:        buildBranch,
		InitialBaseRef:     baseRef40,
		ActiveChunkID:      chunks[0].ID,
		ActiveAttempt:      1,
		ActiveBaseRef:      baseRef40,
		ResetEpoch:         0,
		BounceCycle:        0,
		PreviousBounceIDs:  []string{},
		SeenOrderIDs:       []string{},
		WorktreeStatus:     "active",
		BrainContext:       brainSnap,
		TDD:                TDDState{Red: nil},
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := WriteJSONAtomicG4(sessionPath, st); err != nil {
		if errors.Is(err, ErrG4Violation) {
			return failResult(ExitG4, ReasonG4Violation, traceID, taskID, []ValidationIssue{{
				Pointer: "/session",
				Code:    ReasonG4Violation,
				Message: bfcore.RedactAllG4(err.Error()),
			}})
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, traceID, taskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
	}
	committed = true

	_ = AppendEvent(absDataClean, BuilderEvent{
		Event:      "INIT",
		TraceID:    traceID,
		TaskID:     taskID,
		ChunkID:    chunks[0].ID,
		Attempt:    1,
		ExitCode:   ExitOK,
		Reason:     ReasonOK,
		DurationMS: time.Since(start).Milliseconds(),
		Detail:     fmt.Sprintf("branch=%s seq_b=%v", buildBranch, ringSt != nil),
	})

	gengisBin := ResolveGengisBin(roots.GengisRoot, "")
	nextCmd := ""
	if ringSt == nil {
		nextCmd = fmt.Sprintf(`"%s" plan "%s" --root "%s"`, gengisBin, absTaskDir, filepath.Clean(roots.GengisRoot))
	}

	return CLIResult{
		OK:               true,
		ExitCode:         ExitOK,
		Reason:           ReasonOK,
		Retryable:        false,
		TraceID:          traceID,
		TaskID:           taskID,
		ChunkID:          chunks[0].ID,
		Attempt:          1,
		BuildBranch:      buildBranch,
		WorktreePath:     absWorktreeClean,
		NextAutorCommand: nextCmd,
	}, ExitOK
}

func pathsOverlap(a, b string) bool {
	ca := strings.ToLower(filepath.Clean(a))
	cb := strings.ToLower(filepath.Clean(b))
	if ca == cb {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(ca, strings.TrimSuffix(cb, sep)+sep) ||
		strings.HasPrefix(cb, strings.TrimSuffix(ca, sep)+sep)
}

func collectUnionCanModify(taskScope Scope, chunks []RingChunk) []string {
	seen := map[string]bool{}
	var out []string
	add := func(list []string) {
		for _, item := range list {
			n := strings.TrimSpace(item)
			if n != "" && n != "." && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	for _, ch := range chunks {
		add(ch.Scope.CanModify)
	}
	if len(out) == 0 {
		add(taskScope.CanModify)
	}
	sort.Strings(out)
	return out
}
