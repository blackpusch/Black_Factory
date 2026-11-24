package builder

import (
	"context"
	"errors"
	"fmt"
	"time"

	"blackfactory/pkg/bfcore"
)

// Cleanup ejecuta builder cleanup: retira el worktree fisico tras cierre DONE/ABORTED (salvo --force)
// bajo .session.lock -> .repo-<sha16>.lock y preserva la rama build/<task_id> por defecto.
func Cleanup(opts CleanupOptions) (CLIResult, int) {
	start := time.Now()
	sctx, errRes, code := LoadAndVerifySessionAndWorktree(opts.TaskDir, opts.RepoRoot, opts.FactoryRoot, opts.DataDir, opts.WorktreeRoot, "", false)
	if errRes != nil {
		return *errRes, code
	}
	defer func() { _ = sctx.Lock.Unlock() }()

	ringSt, err := LoadRingState(sctx.AbsTaskDir)
	if err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}

	isTerminal := ringSt != nil && (ringSt.Status == "DONE" || ringSt.Status == "ABORTED")
	if !opts.Force && !isTerminal {
		return failResult(ExitExternalDecision, ReasonTaskTerminalState, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/ring/status",
			Code:    ReasonTaskTerminalState,
			Message: "la tarea aun no esta en estado DONE o ABORTED (use --force para forzar limpieza)",
		}})
	}
	if opts.DeleteBranch && !opts.Force && (ringSt == nil || ringSt.Status != "ABORTED") {
		return failResult(ExitExternalDecision, ReasonTaskTerminalState, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/cli/delete_branch",
			Code:    ReasonTaskTerminalState,
			Message: "--delete-branch en tareas no ABORTED requiere --force para evitar perder commits aprobados",
		}})
	}

	gc := opts.Git
	if gc == nil {
		gc = ExecGitClient{}
	}
	ctx := context.Background()

	if sctx.Session.WorktreeStatus == "active" || opts.DeleteBranch {
		if sctx.Session.WorktreeStatus == "active" && !opts.Force {
			dirty, dErr := gc.StatusPorcelainZ(ctx, sctx.AbsWorktree)
			if dErr == nil && len(dirty) > 0 {
				return failResult(ExitFixable, ReasonWorktreeDirty, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
					Pointer: "/worktree",
					Code:    ReasonWorktreeDirty,
					Message: "el worktree contiene cambios sin commitear (use --force para descartarlos)",
				}})
			}
		}

		rLock, rErr := AcquireRepoLock(sctx.AbsDataDir, sctx.Session.RepoRootSHA256)
		if rErr != nil {
			if errors.Is(rErr, ErrLockBusy) {
				return failResult(ExitInfraOrRefusal, ReasonLockBusy, sctx.Session.TraceID, sctx.Session.TaskID, nil)
			}
			return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		}
		defer func() { _ = rLock.Unlock() }()

		if sctx.Session.WorktreeStatus == "active" {
			if err := gc.WorktreeRemove(ctx, sctx.AbsRepo, sctx.AbsWorktree, opts.Force); err != nil {
				return failResult(ExitInfraOrRefusal, ReasonGitError, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
					Pointer: "/worktree",
					Code:    ReasonGitError,
					Message: bfcore.RedactAllG4(err.Error()),
				}})
			}
			_ = gc.WorktreePrune(ctx, sctx.AbsRepo)
		}

		if opts.DeleteBranch {
			if exists, _ := gc.BranchExists(ctx, sctx.AbsRepo, sctx.Session.BuildBranch); exists {
				if err := gc.DeleteBranch(ctx, sctx.AbsRepo, sctx.Session.BuildBranch); err != nil {
					return failResult(ExitInfraOrRefusal, ReasonGitError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
				}
			}
		}
	}

	sctx.Session.WorktreeStatus = "removed"
	sctx.Session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}

	_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
		Event:       "CLEANUP",
		TraceID:     sctx.Session.TraceID,
		TaskID:      sctx.Session.TaskID,
		ChunkID:     sctx.Session.ActiveChunkID,
		Attempt:     sctx.Session.ActiveAttempt,
		BounceCycle: sctx.Session.BounceCycle,
		ExitCode:    ExitOK,
		Reason:      ReasonOK,
		DurationMS:  time.Since(start).Milliseconds(),
		Detail:      fmt.Sprintf("delete_branch=%v", opts.DeleteBranch),
	})

	return CLIResult{
		OK:          true,
		ExitCode:    ExitOK,
		Reason:      ReasonOK,
		Retryable:   false,
		TraceID:     sctx.Session.TraceID,
		TaskID:      sctx.Session.TaskID,
		BuildBranch: sctx.Session.BuildBranch,
	}, ExitOK
}
