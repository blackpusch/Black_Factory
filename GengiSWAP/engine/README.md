# GengiSWAP Engine

## Overview
Core deterministic execution ring for GengiSWAP (Go module `gengiswap/ring`). Enforces non-negotiable software factory invariants: strict scope boundaries (`CanModify`, `ReadOnly`, `Forbidden`), isolated workspace execution, execution command evidence capture (exit codes, output truncation, elapsed duration), Preparador QA gates, and Author decision transitions (`GO_NEXT`, `RETRY_CHUNK`, `RESPLIT_CHUNK`, `ABORT`).

## Interface
- `[bridge.go:ParseTaskFromDir]`, `[bridge.go:ParseTargetRepoFromDir]`, `[bridge.go:SyncFiles]`, `[bridge.go:CLIConfig]`, `[bridge.go:DecideOptions]`, `[bridge.go:PlanTask]`, `[bridge.go:CheckChunk]`, `[bridge.go:DecideChunk]`, `[bridge.go:ShowStatus]`
- `[ring.go:Decision]`, `[ring.go:Verdict]`, `[ring.go:PlanVerdict]`, `[ring.go:Scope]`, `[ring.go:Status]`, `[ring.go:Task]`, `[ring.go:Attempt]`, `[ring.go:Chunk]`, `[ring.go:Event]`, `[ring.go:State]`, `[ring.go:Preparador]`, `[ring.go:Chef]`, `[ring.go:Auditor]`, `[ring.go:Store]`, `[ring.go:MemStore]`, `[ring.go:FileStore]`, `[ring.go:ResumePolicy]`, `[ring.go:Engine]`, `[ring.go:RenderQAReport]`, `[ring.go:RenderStatusMD]`
- `[runner.go:Evidence]`, `[runner.go:CommandRunner]`, `[runner.go:ShellRunner]`, `[runner.go:Workspace]`, `[runner.go:NopWorkspace]`, `[runner.go:GitWorkspace]`
- Subdirectories:
  - `cmd`: [./cmd/README.md](./cmd/README.md) - CLI command implementations

## Invariants
- `[ring.go:validateChunks]` Chunks slice must be non-empty; each Chunk must possess a non-empty ID, unique non-duplicated ID, non-empty Spec, at least one acceptance criterion, non-empty FailureProof, and at least one non-blank verify command; otherwise returns `ErrInvalidChunk`.
- `[ring.go:Scope.Allows]` File path is disallowed if normalizing to `..` or starting with `../`, or matching `Forbidden` or `ReadOnly`; path is allowed only if matching under `CanModify`.
- `[ring.go:applyQAResult]` `att.QAPassed` is true if and only if `len(att.ScopeViolations) == 0 && cmdsOK && att.PrepPassed && len(att.Evidence) > 0`.
- `[ring.go:allowedDecisions]` Decision `GoNext` permitted only when `att.QAPassed` is true; `RetryChunk` permitted only when `len(c.Attempts) < maxRetries`.
- `[ring.go:auditStep]` Out-of-scope modifications on rejected attempts are cleanly stashed/reverted via `WS.Revert`.
- `[runner.go:GitWorkspace.Commit]` Stages and commits only dirty working tree files explicitly matching the in-scope files list.
- `[runner.go:GitWorkspace.Revert]` Preserves git history: uncommitted changes are stashed via `git stash push --include-untracked` and commits are reverted via `git revert --no-edit base..HEAD`.

## Side Effects
- Spawns external processes via `cmd.exe /S /C` (Windows) or `sh -c` (Unix) with `PYTHONUTF8=1` and `PYTHONIOENCODING=utf-8`.
- Executes Git operations (`git add`, `git commit`, `git stash`, `git revert`).
- Writes QA reports to `workspace/qa/<chunk>.md`, atomic state files `.ring/state.json`, regenerates `status.md`, and appends events to `gengis_global_log.md`.

## Verification
- Run: `go test -count=1 ./...`
