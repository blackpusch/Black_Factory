# GengiSWAP Engine Gengis CLI

## Overview
CLI executable entry point (`gengis.exe`) for the GengiSWAP deterministic task execution ring. Validates task definitions, runs chunk verification and delivery commands, interacts with Author approval gates, and regenerates task status artifacts.

## Interface
- `[main.go:main]` Program entry point executing `run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)`.
- Subcommands:
  - `plan <TAREA_ID>`: validates `index.md` and `chunks/`, prompts Author approval (`[s/N]`), and initializes state via `ring.PlanTask`.
  - `check <TAREA_ID>`: executes verification and delivery commands of active chunk via `ring.CheckChunk`.
  - `decide <TAREA_ID> [--qa-pass] [--qa-notes]`: prompts and records Author decision (`GO_NEXT`, `RETRY_CHUNK`, `RESPLIT_CHUNK`, `ABORT`) via `ring.DecideChunk`.
  - `status <TAREA_ID>`: displays task state and regenerates `status.md` via `ring.ShowStatus`.
- Flags:
  - `--root <DIR>`: explicit GengiSWAP root directory on all subcommands.
  - `--qa-pass`: boolean flag for Preparador QA evaluation on `decide`.
  - `--qa-notes`: string notes for Preparador QA on `decide`.

## Invariants
- `[main.go:run]` A valid subcommand (`plan`, `check`, `decide`, `status`) is mandatory; unknown subcommand returns an error.
- `[main.go:run]` Exactly one non-empty positional argument (`<TAREA_ID>`) is required after flag parsing.
- `[main.go:run]` Flags `--qa-pass` and `--qa-notes` are registered and permitted only under the `decide` subcommand.
- `[main.go:resolvePaths]` Root directory resolution order: 1) explicit `--root` flag, 2) `GENGIS_ROOT` environment variable, 3) auto-detection via `detectGengisRoot()`.
- `[main.go:isGengisRoot]` Identified as GengiSWAP root if containing `gengis_global_log.md` or a directory `gengis_task`.

## Side Effects
- Dispatches execution to `ring.PlanTask`, `ring.CheckChunk`, `ring.DecideChunk`, and `ring.ShowStatus`.
- Reads environment variable `GENGIS_ROOT`.
- Writes errors to `os.Stderr` and terminates process with `os.Exit(1)` on error.

## Verification
- Run: `go test ./...`
