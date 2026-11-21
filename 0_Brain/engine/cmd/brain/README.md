# 0_Brain Engine Brain CLI

## Overview
CLI executable entry point (`brain.exe`) for the `0_Brain` persistent memory engine of Black Factory. Dispatches CLI subcommands and orchestrates vault operations, SQLite FTS5 index management, schema validation, and proposal governance.

## Interface
- `[main.go:BrainService]` interface defining vault operations (`InitVault`, `BuildIndex`, `Search`, `ContextForFiles`, `ValidateVault`, `CheckDrift`, `ProposeUpdate`, `ApplyUpdate`, `SignProposal`)
- `[main.go:LocalService]` default local implementation of `BrainService`
- `[main.go:VerifyOutput]` verification result structure containing validation report and drift items
- Subcommands:
  - `init-vault`: flags `--vault`, `--schemas`, `--json`
  - `index`: flags `--vault`, `--db`, `--json`
  - `query`: flags `--vault`, `--db`, `--project`, `--type`, `--tag`, `--limit`, `--q`, `--json`
  - `context`: flags `--vault`, `--db`, `--project`, `--file`, `--files`, `--json`
  - `verify`: flags `--vault`, `--repo`, `--fail-on-drift`, `--json`
  - `propose`: flags `--vault`, `--target`, `--title`, `--project`, `--id`, `--type`, `--status`, `--operation`, `--author`, `--reason`, `--tags`, `--covers`, `--verified-head`, `--body`, `--body-file`, `--json`
  - `apply`: flags `--vault`, `--proposal`, `--autor-sig`, `--approve-autor`, `--json`
  - `sign`: flags `--vault`, `--proposal`, `--key`, `--json`
  - `mcp`: reserved for Phase 2

## Invariants
- `[main.go:run]` If `svc` is nil, execution defaults to `LocalService{}`; empty args prints usage and exits with code 1; unknown subcommand exits with code 1; subcommand `mcp` returns exit code 2.
- `[main.go:cmdInitVault]` `schemasDir` must resolve to an existing directory; otherwise exits with code 1.
- `[main.go:cmdVerify]` Must acquire OS vault lock via `brain.AcquireVaultLock` (exit code 4 on contention), released via `defer lock.Unlock()`. Journal recovery failure exits with code 3. Overall OK requires `report.OK == true` and (if `--fail-on-drift`) `len(drift) == 0`.
- `[main.go:cmdApply]` Error mapping: `brain.ErrAutorApprovalRequired` -> exit code 2, `brain.ErrJournalCorrupt` / `brain.ErrCommitDegraded` -> exit code 3, `brain.ErrVaultLocked` -> exit code 4, other errors -> exit code 1.
- `[main.go:resolveVaultPath]` Resolves in order: `--vault` flag -> `BRAIN_VAULT` env -> default `0_Brain/vault`.
- `[main.go:resolveDBPath]` Resolves in order: `--db` flag -> default `brain.DefaultIndexPath(vaultRoot)`.

## Side Effects
- Mutates filesystem when initializing vault (`svc.InitVault`), building SQLite index (`svc.BuildIndex`), writing proposals to `_proposals/` (`svc.ProposeUpdate`), or applying updates to target files (`svc.ApplyUpdate`).
- Acquires and releases kernel file lock on `.brain.lock` via `brain.AcquireVaultLock`.
- Reads files and Git history for validation and drift checks.
- Exits process via `os.Exit` with command-specific exit codes.

## Verification
- Run: `go test ./...`
