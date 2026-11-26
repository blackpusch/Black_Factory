# 1_Intake Engine CLI (`intake.exe`)

## Overview
CLI executable entry point (`1_Intake/intake.exe`) for Team 1 (`1_Intake`) of Black Factory. Dispatches deterministic ingestion, quarantine resolution, listing, on-demand Markdown/JSON viewing, and Option i runtime verification with exit codes `0..5`.

## Interface
- `[main.go:ExitOK]` (`0`), `[main.go:ExitUsageOrError]` (`1`), `[main.go:ExitQuarantined]` (`2`), `[main.go:ExitRejectedG4]` (`3`), `[main.go:ExitBrainOrLockUnavailable]` (`4`), `[main.go:ExitDedupeConflict]` (`5`)
- `[main.go:run]` CLI dispatcher accepting injectable `io.Reader`, `io.Writer`, and `intake.BrainClient`
- `[main.go:discoverRepoRoot]` Automatic workspace root discovery from `--repo-root`, `BF_REPO_ROOT`, `os.Executable()`, or current working directory
- `[main.go:reorderFlags]` Positional-independent CLI flag reordering
- Subcommands:
  - `ingest`: flags `--input`, `--project`, `--title`, `--description`, `--note`, `--kind`, `--source`, `--source-ref`, `--path`, `--paths`, `--impact-flag`, `--max-ftd-files`, `--strict-g4`, `--update`, `--dry-run`, `--data-dir`, `--vault`, `--brain-bin`, `--repo-root`, `--json`
  - `resolve`: flags `--id`, `--project`, `--title`, `--description`, `--note`, `--kind`, `--source`, `--source-ref`, `--path`, `--paths`, `--impact-flag`, `--max-ftd-files`, `--strict-g4`, `--update`, `--dry-run`, `--data-dir`, `--vault`, `--brain-bin`, `--repo-root`, `--json`
  - `list`: flags `--status`, `--lane`, `--project`, `--risk`, `--data-dir`, `--repo-root`, `--json`
  - `show`: flags `--id`, `--format`, `--data-dir`, `--repo-root`, `--json`
  - `verify`: flags `--data-dir`, `--vault`, `--brain-bin`, `--repo-root`, `--json`

## Invariants
- `[main.go:cmdIngest]` & `[main.go:cmdResolve]` Executes the 5-stage pipeline in memory when `--dry-run` is active without acquiring `.intake.lock` or writing any files to `workitems/`, `quarantine/`, or `intake_events.jsonl`.
- `[main.go:cmdIngest]` Maps Tier 1 G4 violations to exit code `3` (`ExitRejectedG4`) with `0` files written to `workitems/` or `quarantine/` and a payload-free `REJECTED_G4` event in `intake_events.jsonl`.
- `[main.go:cmdIngest]` & `[main.go:cmdResolve]` Maps `ErrBrainUnavailable` and `ErrIntakeLocked` to exit code `4` (`ExitBrainOrLockUnavailable`), and duplicate ready `dedupe_key` with different payload SHA-256 (without `--update`) to exit code `5` (`ExitDedupeConflict`).

## Side Effects
- Reads JSON payloads from local files or `stdin` (`--input -`).
- Delegates atomic persistence and `.intake.lock` acquisition to `intake.Store`.
- Invokes `0_Brain/brain.exe` in read-only mode via `intake.SubprocessBrainClient`.
- Exits process via `os.Exit` with codes `0..5`.

## Verification
- Run: `go test -v -count=1 -cover ./...`
