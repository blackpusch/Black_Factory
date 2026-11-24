package builder

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

func validateTimeoutSec(timeoutSec int) (int, bool) {
	if timeoutSec == 0 {
		return MaxVerifyTimeoutSec, true
	}
	if timeoutSec < 0 || timeoutSec > MaxVerifyTimeoutSec {
		return 0, false
	}
	return timeoutSec, true
}

func mapSnapshotError(err error, traceID, taskID string) (CLIResult, int) {
	if errors.Is(err, bfcore.ErrReparsePoint) {
		return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, traceID, taskID, nil)
	}
	if errors.Is(err, ErrDiffTooLarge) || errors.Is(err, ErrObservedFileTooBig) {
		return failResult(ExitFixable, ReasonDiffTooLarge, traceID, taskID, []ValidationIssue{{
			Pointer: "/diff",
			Code:    ReasonDiffTooLarge,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}
	if errors.Is(err, ErrForbiddenGitEntry) || errors.Is(err, ErrRegularFileRequired) {
		return failResult(ExitInfraOrRefusal, ReasonGitError, traceID, taskID, []ValidationIssue{{
			Pointer: "/worktree",
			Code:    ReasonGitError,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}
	return failResult(ExitInfraOrRefusal, ReasonGitError, traceID, taskID, []ValidationIssue{{
		Pointer: "/worktree",
		Code:    ReasonGitError,
		Message: bfcore.RedactAllG4(err.Error()),
	}})
}

func checkWorktreeHeadMatchesBaseRef(ctx context.Context, gc GitClient, sctx SessionRuntimeContext) *struct {
	res  CLIResult
	code int
} {
	head, err := gc.RevParse(ctx, sctx.AbsWorktree, "HEAD^{commit}")
	if err != nil {
		r, c := failResult(ExitInfraOrRefusal, ReasonGitError, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/git/head",
			Code:    ReasonGitError,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
		return &struct {
			res  CLIResult
			code int
		}{r, c}
	}
	if head != sctx.Session.ActiveBaseRef {
		r, c := failResult(ExitExternalDecision, ReasonUnexpectedCommitInWorktree, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/git/head",
			Code:    ReasonUnexpectedCommitInWorktree,
			Message: "el HEAD del worktree avanzo fuera de ActiveBaseRef sin usar builder commit",
		}})
		return &struct {
			res  CLIResult
			code int
		}{r, c}
	}
	return nil
}

func changedFilePaths(entries []ChangedFileEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func snapshotAndCheckG4(ctx context.Context, gc GitClient, sctx SessionRuntimeContext) (WorktreeSnapshot, string, *CLIResult, int) {
	snap, err := ComputeWorktreeSnapshot(ctx, gc, sctx.AbsRepo, sctx.AbsWorktree, sctx.Session.ActiveBaseRef)
	if err != nil {
		r, c := mapSnapshotError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		return WorktreeSnapshot{}, "", &r, c
	}
	cleanDiff, finding, pointer, g4Err := ScanWorktreeChangesG4(sctx.AbsWorktree, snap)
	if finding != nil {
		r, c := failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: pointer,
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("violacion G4 regla %s (linea %d)", finding.Rule, finding.Line),
		}})
		return WorktreeSnapshot{}, "", &r, c
	}
	if g4Err != nil {
		r, c := mapSnapshotError(g4Err, sctx.Session.TraceID, sctx.Session.TaskID)
		return WorktreeSnapshot{}, "", &r, c
	}
	return snap, cleanDiff, nil, ExitOK
}

func checkScopeCompliance(sctx SessionRuntimeContext, activeChunk RingChunk, entries []ChangedFileEntry) (*CLIResult, int) {
	violations := ScopeViolations(sctx.Meta.Scope, activeChunk.Scope, changedFilePaths(entries))
	if len(violations) == 0 {
		return nil, ExitOK
	}
	issues := make([]ValidationIssue, 0, len(violations))
	for i, v := range violations {
		issues = append(issues, ValidationIssue{
			Pointer: fmt.Sprintf("/scope/%d", i),
			Code:    ReasonScopeViolation,
			Message: bfcore.RedactAllG4(v),
		})
	}
	r, c := failResult(ExitFixable, ReasonScopeViolation, sctx.Session.TraceID, sctx.Session.TaskID, issues)
	return &r, c
}

func persistSessionOnWriteError(err error, traceID, taskID string) (CLIResult, int) {
	if errors.Is(err, ErrG4Violation) {
		return failResult(ExitG4, ReasonG4Violation, traceID, taskID, nil)
	}
	if errors.Is(err, bfcore.ErrReparsePoint) {
		return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, traceID, taskID, nil)
	}
	return failResult(ExitInfraOrRefusal, ReasonIOError, traceID, taskID, nil)
}

func isTestFilePath(rel string) bool {
	base := strings.ToLower(filepath.Base(bfcore.NormalizeCoverPath(rel)))
	if strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	return false
}

func scopePatternAllowsTests(p string) bool {
	low := strings.ToLower(bfcore.NormalizeCoverPath(p))
	return strings.Contains(low, "_test.go") ||
		strings.Contains(low, "test_") ||
		strings.Contains(low, "_test.py") ||
		strings.Contains(low, ".test.") ||
		strings.Contains(low, ".spec.")
}

func effectiveAllowedTestPattern(taskScope, chunkScope Scope) (Scope, bool) {
	eff := chunkScope
	explicitTestPattern := false
	if len(eff.CanModify) == 0 {
		eff = Scope{CanModify: taskScope.CanModify, ReadOnly: taskScope.ReadOnly, Forbidden: taskScope.Forbidden}
	} else {
		eff = Scope{
			CanModify: chunkScope.CanModify,
			ReadOnly:  append(append([]string{}, taskScope.ReadOnly...), chunkScope.ReadOnly...),
			Forbidden: append(append([]string{}, taskScope.Forbidden...), chunkScope.Forbidden...),
		}
	}
	for _, pat := range eff.CanModify {
		if scopePatternAllowsTests(pat) {
			explicitTestPattern = true
			break
		}
	}
	return eff, explicitTestPattern
}

func shouldRequireTestFileChange(taskScope, chunkScope Scope, changed []ChangedFileEntry) bool {
	eff, explicit := effectiveAllowedTestPattern(taskScope, chunkScope)
	if explicit {
		return true
	}
	for _, ch := range changed {
		if ch.Status == "deleted" {
			continue
		}
		norm := bfcore.NormalizeCoverPath(ch.Path)
		dir := filepath.ToSlash(filepath.Dir(norm))
		if dir == "." {
			dir = ""
		} else {
			dir += "/"
		}
		base := filepath.Base(norm)
		ext := strings.ToLower(filepath.Ext(base))
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		var candidates []string
		switch ext {
		case ".go":
			candidates = []string{dir + stem + "_test.go"}
		case ".py":
			candidates = []string{dir + "test_" + base, dir + stem + "_test.py"}
		case ".js", ".ts", ".jsx", ".tsx":
			candidates = []string{dir + stem + ".test" + ext, dir + stem + ".spec" + ext}
		}
		for _, cand := range candidates {
			if eff.Allows(cand) && len(ScopeViolations(taskScope, chunkScope, []string{cand})) == 0 {
				return true
			}
		}
	}
	return false
}

func hasTestFileInChanged(changed []ChangedFileEntry) bool {
	for _, ch := range changed {
		if ch.Status != "deleted" && isTestFilePath(ch.Path) {
			return true
		}
	}
	return false
}

// TDDRed ejecuta builder tdd-red siguiendo la precedencia [2] -> [6a] -> [5] -> [4] -> [3] -> [1] -> [0].
func TDDRed(opts TDDRedOptions) (CLIResult, int) {
	start := time.Now()
	timeoutSec, ok := validateTimeoutSec(opts.TimeoutSec)
	if !ok || opts.CmdIndex < 0 {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
			Pointer: "/cli/options",
			Code:    ReasonInvalidUsage,
			Message: "parametros --timeout (1..300) o --cmd-index invalidos",
		}})
	}

	sctx, errRes, code := LoadAndVerifySessionAndWorktree(opts.TaskDir, opts.RepoRoot, opts.FactoryRoot, opts.DataDir, opts.WorktreeRoot, "", true)
	if errRes != nil {
		return *errRes, code
	}
	defer func() { _ = sctx.Lock.Unlock() }()

	if sctx.Session.BounceBrakeActive {
		return failResult(ExitAttemptsBlocked, ReasonMaxBounceCycles, sctx.Session.TraceID, sctx.Session.TaskID, nil)
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

	// [3] Barrera G4 antes de reglas de dominio [1] o ejecucion de comando
	snap, _, g4Res, g4Code := snapshotAndCheckG4(ctx, gc, sctx)
	if g4Res != nil {
		return *g4Res, g4Code
	}

	// [1] Reglas de dominio
	if opts.CmdIndex >= len(activeChunk.VerifyCommands) {
		return failResult(ExitUsage, ReasonCmdIndexOutOfRange, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/cli/cmd_index",
			Code:    ReasonCmdIndexOutOfRange,
			Message: fmt.Sprintf("--cmd-index=%d fuera de rango (%d VerifyCommands)", opts.CmdIndex, len(activeChunk.VerifyCommands)),
		}})
	}
	if scopeRes, scopeCode := checkScopeCompliance(sctx, activeChunk, snap.ChangedFiles); scopeRes != nil {
		return *scopeRes, scopeCode
	}
	if len(snap.ChangedFiles) == 0 {
		return failResult(ExitFixable, ReasonTDDRedNoChanges, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/worktree",
			Code:    ReasonTDDRedNoChanges,
			Message: "el worktree no tiene cambios respecto a ActiveBaseRef; escribe una prueba fallida antes de tdd-red",
		}})
	}

	hasTest := hasTestFileInChanged(snap.ChangedFiles)
	if !hasTest && shouldRequireTestFileChange(sctx.Meta.Scope, activeChunk.Scope, snap.ChangedFiles) {
		return failResult(ExitFixable, ReasonTDDRedMissingTestChange, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/worktree/changed_files",
			Code:    ReasonTDDRedMissingTestChange,
			Message: "tdd-red requiere al menos un archivo de prueba modificado dentro del alcance efectivo",
		}})
	}

	runner := opts.Runner
	if runner == nil {
		runner = ShellRunner{Timeout: time.Duration(timeoutSec) * time.Second}
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	rawCmd := activeChunk.VerifyCommands[opts.CmdIndex]
	ev, runErr := runner.Run(runCtx, sctx.AbsWorktree, rawCmd)

	sanCmd, excerpt, trunc, tier1, sanErr := SanitizeCommandOutputG4(rawCmd, ev.Output, MaxCommandOutputBytes)
	if tier1 != nil {
		return failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/command_output",
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("violacion G4 Tier 1 en salida de comando (%s)", tier1.Rule),
		}})
	}
	if sanErr != nil {
		return failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if runErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/command",
			Code:    ReasonIOError,
			Message: bfcore.RedactAllG4(runErr.Error()),
		}})
	}
	if ev.ExitCode == 0 {
		return failResult(ExitFixable, ReasonTDDRedPassedUnexpected, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/command/exit_code",
			Code:    ReasonTDDRedPassedUnexpected,
			Message: "el VerifyCommand retorno exit 0 en fase Red; debe fallar de forma determinista",
		}})
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sctx.Session.TDD.Red = &RedEvidence{
		ChunkID:               activeChunk.ID,
		GengisAttempt:         attemptNum,
		BounceCycle:           sctx.Session.BounceCycle,
		InheritedFromCycle:    sctx.Session.BounceCycle,
		CmdIndex:              opts.CmdIndex,
		Command:               sanCmd,
		ExitCode:              ev.ExitCode,
		DurationMS:            ev.DurationMs,
		OutputExcerpt:         excerpt,
		Truncated:             trunc,
		RedWorktreeTreeSHA256: snap.WorktreeTreeSHA256,
		HasTestFile:           hasTest,
		RecordedAt:            now,
	}
	sctx.Session.RequiresRedTest = false
	sctx.Session.UpdatedAt = now

	if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
		return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
	}

	_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
		Event:       "TDD_RED",
		TraceID:     sctx.Session.TraceID,
		TaskID:      sctx.Session.TaskID,
		ChunkID:     activeChunk.ID,
		Attempt:     attemptNum,
		BounceCycle: sctx.Session.BounceCycle,
		ExitCode:    ExitOK,
		Reason:      ReasonOK,
		DurationMS:  time.Since(start).Milliseconds(),
		Detail:      fmt.Sprintf("cmd_index=%d exit=%d", opts.CmdIndex, ev.ExitCode),
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
	}, ExitOK
}
