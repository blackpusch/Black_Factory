# 2_Spec_Writer Engine CLI (`specwriter.exe`)

## Overview
CLI executable entry point (`2_Spec_Writer/specwriter.exe`) for Team 2 (`2_Spec_Writer`) of Black Factory. Exposes five subcommands (`prepare`, `validate`, `render`, `compile`, `verify`) returning structured JSON (`--json`) or concise human summaries with exit codes `0..6`.

## Interface
- `[main.go:run]` — CLI dispatcher accepting injectable `io.Writer` and `bfcore.BrainClient`
- `[main.go:reorderFlags]` — positional-independent CLI flag reordering
- Subcommands:
  - `prepare`: `--workitem` (required), `--repo-root` (`.`), `--factory-root` (`--repo-root`), `--vault`, `--data-dir`, `--brain-bin`, `--refresh-context`, `--reset-attempts`, `--reason`, `--json`
  - `validate`: `--spec` (required), `--stage <draft|final>` (required), `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json`
  - `render`: `--spec` (required), `--json`
  - `compile`: `--spec` (required), `--task-root` (required), `--task-id` (required), `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json`
  - `verify`: `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json`

## Invariants
- Returns deterministic exit codes `0..6` matching `specwriter.Exit*` and `specwriter.IsRetryableReason`.
- Never invokes LLM APIs and never executes `verify_steps` written inside `spec.json`.
- `render` and `verify` perform zero disk writes.

## Side Effects
- Delegates runtime persistence (`prepare`, `validate`, `compile`) and read-only checks (`render`, `verify`) to `blackfactory/specwriter`.
- Exits process via `os.Exit` with codes `0..6`.

## Verification
- Run: `go vet ./... && go test -v -count=1 -cover ./...`
