# 2_Spec_Writer Engine

## Overview
Pure-Go (`CGO_ENABLED=0`, zero SQLite, zero LLM/HTTP) deterministic specification gate and `GengiSWAP` compiler engine (`module blackfactory/specwriter`), consuming `blackfactory/pkg/bfcore` and embedding `GengiSWAP` v1 Markdown templates via `go:embed`.

## Interface
- `[embed.go:TemplatesFS]` — embedded `templates/*.md.tmpl` (`index.md.tmpl`, `chunk.md.tmpl`, `status.md.tmpl`)
- `[model.go:ExitOK]`, `[model.go:ExitFixable]`, `[model.go:ExitUsage]`, `[model.go:ExitG4]`, `[model.go:ExitExternalDecision]`, `[model.go:ExitAttemptsBlocked]`, `[model.go:ExitInfraOrRefusal]`
- `[model.go:MaxValidateAttemptsPerStage]`, `[model.go:MaxExternalCycles]`, `[model.go:MaxVerifyTimeoutSec]`, `[model.go:MaxWorkUnitsASDD]`, `[model.go:MaxSpecBytes]`, `[model.go:MaxGoldfishBytes]`, `[model.go:MaxObservedFileBytes]`
- `[model.go:IsRetryableReason]`, `[model.go:CanonicalSpecSHA256]`, `[model.go:ComputeScopeSHA256]`
- `[model.go:Spec]`, `[model.go:SpecContext]`, `[model.go:AttemptsState]`, `[model.go:GoldfishReview]`, `[model.go:ValidationIssue]`, `[model.go:RenderedTaskFiles]`, `[model.go:CLIResult]`, `[model.go:PrepareResult]`, `[model.go:VerifyReport]`, `[model.go:SpecWriterEvent]`
- `[state.go:WriteJSONAtomicG4]`, `[state.go:LoadAttemptsStrict]`, `[state.go:ReadFileWithShortRetry]`, `[state.go:AppendEvent]`
- `[verify.go:PinnedBridgeGoSHA256]`, `[verify.go:PinnedRingGoSHA256]`, `[verify.go:CheckReparseSafety]`, `[verify.go:CheckBrainPreflight]`, `[verify.go:CheckGitIgnore]`, `[verify.go:CheckGengisEngineHashes]`, `[verify.go:VerifyRuntime]`
- `[prepare.go:Prepare]`
- `[validate.go:Validate]`, `[validate.go:ValidateSpecAndRenderCheck]`
- `[render.go:RenderSpecMarkdown]`
- `[compile.go:Compile]`, `[compile.go:SerializeVerifyStepCmd]`
- Subdirectories:
  - `cmd/specwriter`: [./cmd/specwriter/README.md](./cmd/specwriter/README.md) — CLI executable entry point (`specwriter.exe`)

## Invariants
- `[validate.go:Validate]` & `[compile.go:Compile]` Enforce fixed precedence `2 -> 5 -> 3 -> 6 -> 4 -> 1` under strict lock ordering `.wi.lock -> .compile.lock -> .events.lock` with shared `ValidateSpecAndRenderCheck`.
- `[validate.go:ValidateSpecAndRenderCheck]` Enforces closed command/flag/env allowlist, `G2` (zero Mermaid), `G5` (1-based CRLF/BOM + ±5 literal anchor), NTFS segment case parity, `GengiSWAP` `validateChunks` + `parseIndexFile`/`parseChunkFile` guards, and `goldfish_review.json` consistency in `ASDD --stage final`.
- `[compile.go:Compile]` Enforces anti-bypass matching `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)`, two-zone G4 boundary (`<TARGET_REPO>` substituted only after `ScanG4All == 0`), and atomic create-only NTFS directory promotion (`LIVE_TASK_STATE` / `TASK_DIR_EXISTS`).
- `[render.go:RenderSpecMarkdown]` Pure `stdout` renderer (`0` disk writes) verifying `MaxSpecBytes`, `G4` raw+decoded, `schema_version == "1.0"`, and `G4` on rendered output.
- `[verify.go:VerifyRuntime]` Strictly read-only (`0` locks, `0` created files) with deterministic exit-code priority, individual `git check-ignore -q` checks, and `RZ-001` pinned hash verification.

## Side Effects
- Acquires granular `.wi.lock`, static `<task-root>/.compile.lock`, and brief `.events.lock` OS kernel locks.
- Writes JSON artifacts under `2_Spec_Writer/runtime/<project>/<WI-id>/` and appends G4-verified JSONL lines to `2_Spec_Writer/runtime/specwriter_events.jsonl`.
- Creates `<task-root>/<task-id>` atomically via sibling staging directory and single `os.Rename`.
- Spawns read-only `0_Brain/brain.exe` (`context --json`) and `git` (`rev-parse`, `check-ignore`) subprocesses.

## Verification
- Run: `go vet ./... && go test -count=1 -cover ./...` (with `CGO_ENABLED=0`, `GENGIS_CONTRACT_STRICT=1`, `GENGIS_BIN=C:\Black Factory\GengiSWAP\gengis.exe`)
