package builder

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

// Commit ejecuta builder commit: verifica aprobacion intacta (bytes crudos + modo octal),
// invoca gengis.exe check <taskDir> --root <gengisRoot>, valida que main no mutara y reconcilia state.json.
func Commit(opts CommitOptions) (CLIResult, int) {
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

	if strings.TrimSpace(opts.GengisRoot) != "" {
		sctx.Roots.GengisRoot = filepath.Clean(opts.GengisRoot)
	}

	if sctx.Session.BounceBrakeActive {
		return failResult(ExitAttemptsBlocked, ReasonMaxBounceCycles, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}

	gengisBin := ResolveGengisBin(sctx.Roots.GengisRoot, opts.GengisBin)
	if err := bfcore.CheckNoReparseChain("", gengisBin); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return failResult(ExitInfraOrRefusal, ReasonGengisUnavailable, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		}
	}
	gfi, gStatErr := os.Lstat(gengisBin)
	if gStatErr != nil || bfcore.IsReparseOrSymlink(gfi) || gfi.IsDir() || !gfi.Mode().IsRegular() {
		if gStatErr == nil && bfcore.IsReparseOrSymlink(gfi) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, sctx.Session.TraceID, sctx.Session.TaskID, nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonGengisUnavailable, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/cli/gengis_bin",
			Code:    ReasonGengisUnavailable,
			Message: "binario gengis.exe no disponible o no es archivo regular",
		}})
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

	// [3] Barrera G4 fail-closed antes de gengis.exe check
	snap, _, g4Res, g4Code := snapshotAndCheckG4(ctx, gc, sctx)
	if g4Res != nil {
		return *g4Res, g4Code
	}

	// [1] Reglas anti-bypass
	if sctx.Session.ApprovedBundleSHA256 == "" {
		return failResult(ExitFixable, ReasonApprovalRequired, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/approved_bundle_sha256",
			Code:    ReasonApprovalRequired,
			Message: "se requiere aplicar una orden APPROVE valida antes de commit",
		}})
	}
	if snap.WorktreeTreeSHA256 != sctx.Session.ApprovedWorktreeTreeSHA256 {
		return failResult(ExitFixable, ReasonWorktreeDriftAfterAppr, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/git/worktree_tree_sha256",
			Code:    ReasonWorktreeDriftAfterAppr,
			Message: "el worktree sufrio cambios (o CRLF/LF/BOM/modo) tras la aprobacion; re-empaqueta y re-aprueba",
		}})
	}
	if scopeRes, scopeCode := checkScopeCompliance(sctx, activeChunk, snap.ChangedFiles); scopeRes != nil {
		return *scopeRes, scopeCode
	}

	mainHeadBefore, err := gc.RevParse(ctx, sctx.AbsRepo, "HEAD^{commit}")
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonGitError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	attemptsBefore := len(activeChunk.Attempts)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, gengisBin, "check", sctx.AbsTaskDir, "--root", sctx.Roots.GengisRoot)
	cmd.Dir = sctx.Roots.GengisRoot
	cmd.Env = withUTF8PythonEnv(os.Environ())
	cmd.WaitDelay = 3 * time.Second
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	runErr := cmd.Run()

	mainHeadAfter, afterErr := gc.RevParse(ctx, sctx.AbsRepo, "HEAD^{commit}")
	if afterErr != nil || mainHeadAfter != mainHeadBefore {
		return failResult(ExitInfraOrRefusal, ReasonGitError, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/repo/head",
			Code:    ReasonGitError,
			Message: "el HEAD del repositorio principal cambio o fallo tras gengis.exe check",
		}})
	}

	ringAfter, err := LoadRingState(sctx.AbsTaskDir)
	if err != nil || ringAfter == nil || ringAfter.Index < 0 || ringAfter.Index >= len(ringAfter.Chunks) {
		return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	chunkAfter := ringAfter.Chunks[ringAfter.Index]
	if chunkAfter.ID != activeChunk.ID || len(chunkAfter.Attempts) <= attemptsBefore {
		msg := "gengis.exe check fallo sin registrar intento en .ring/state.json"
		if runErr != nil {
			msg = fmt.Sprintf("%s: %s", msg, bfcore.RedactAllG4(runErr.Error()))
		}
		return failResult(ExitInfraOrRefusal, ReasonGengisUnavailable, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/gengis",
			Code:    ReasonGengisUnavailable,
			Message: msg,
		}})
	}

	lastAtt := chunkAfter.Attempts[len(chunkAfter.Attempts)-1]
	if lastAtt.QADone && !lastAtt.QAPassed {
		decideCmd := BuildGengisDecideCommand(gengisBin, sctx.AbsTaskDir, sctx.Roots.GengisRoot, false, lastAtt.QANotes)
		r, c := failResult(ExitExternalDecision, ReasonGengisDecideRequired, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/ring/qa_passed",
			Code:    ReasonGengisDecideRequired,
			Message: "gengis.exe check marco QA FAIL; [AUTOR] debe ejecutar gengis.exe decide",
		}})
		r.ChunkID = activeChunk.ID
		r.Attempt = lastAtt.N
		r.NextAutorCommand = decideCmd
		return r, c
	}

	if !lastAtt.QAPassed || !reHex40.MatchString(lastAtt.CommitRef) {
		return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	wtHead, err := gc.RevParse(ctx, sctx.AbsWorktree, "HEAD^{commit}")
	if err != nil || wtHead != lastAtt.CommitRef {
		return failResult(ExitInfraOrRefusal, ReasonGitError, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/worktree/head",
			Code:    ReasonGitError,
			Message: "HEAD del worktree tras gengis.exe check no coincide con CommitRef registrado",
		}})
	}

	nextCmd := BuildGengisDecideCommand(gengisBin, sctx.AbsTaskDir, sctx.Roots.GengisRoot, true, lastAtt.QANotes)
	_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
		Event:        "COMMIT_SEALED",
		TraceID:      sctx.Session.TraceID,
		TaskID:       sctx.Session.TaskID,
		ChunkID:      activeChunk.ID,
		Attempt:      attemptNum,
		BounceCycle:  sctx.Session.BounceCycle,
		ExitCode:     ExitOK,
		Reason:       ReasonOK,
		BundleSHA256: sctx.Session.ApprovedBundleSHA256,
		DurationMS:   time.Since(start).Milliseconds(),
		Detail:       "commit_ref=" + lastAtt.CommitRef,
	})

	return CLIResult{
		OK:               true,
		ExitCode:         ExitOK,
		Reason:           ReasonOK,
		Retryable:        false,
		TraceID:          sctx.Session.TraceID,
		TaskID:           sctx.Session.TaskID,
		ChunkID:          activeChunk.ID,
		Attempt:          attemptNum,
		BounceCycle:      sctx.Session.BounceCycle,
		BuildBranch:      sctx.Session.BuildBranch,
		WorktreePath:     sctx.AbsWorktree,
		BundleID:         sctx.Session.ApprovedBundleID,
		BundleSHA256:     sctx.Session.ApprovedBundleSHA256,
		CommitRef:        lastAtt.CommitRef,
		NextAutorCommand: nextCmd,
	}, ExitOK
}
