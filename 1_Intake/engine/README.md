# 1_Intake Engine

## Overview
Core deterministic request normalization, two-tier G4 perimeter sanitization, read-only `0_Brain` consultation, and methodological lane triage engine for `1_Intake` (Go module `blackfactory/intake`, `CGO_ENABLED=0`, zero SQLite dependencies, delegating shared FTD Option `5B`, path hygiene, and read-only `0_Brain` primitives to `blackfactory/pkg/bfcore`).

## Interface
- `[model.go:SchemaVersion]`, `[model.go:TriageDetRulesetVersion]`, `[model.go:MaxInputBytes]`, `[model.go:DefaultMaxFTDFiles]` (delegated to `bfcore.DefaultMaxFTDFiles`)
- `[model.go:RawIntake]`, `[model.go:WorkItem]`, `[model.go:SanitizationReport]`, `[model.go:BrainEvidence]`, `[model.go:BrainInvocation]`, `[model.go:BrainDocMatch]`, `[model.go:RiskHit]`, `[model.go:TriageReason]`, `[model.go:QuarantineInfo]`, `[model.go:IntakeEvent]`, `[model.go:VerifyReport]`
- `[model.go:ComputeSanitizedSHA256]`, `[model.go:ComputeDedupeKey]`, `[model.go:FormatWorkItemID]`, `[model.go:RiskLevelRank]`, `[model.go:ValidateWorkItem]`
- `[input.go:ErrInputTooLarge]`, `[input.go:ReadRawInput]`, `[input.go:ScanRawAndFlagsTier1]`, `[input.go:ParseJSONIntake]`, `[input.go:MergeCLIOverrides]`, `[input.go:ScanDecodedTier1]`, `[input.go:SanitizeAndRedactTier2]`
- `[paths.go:SanitizeAffectedPaths]`, `[paths.go:CountRootModules]` (delegated to `bfcore.SanitizeRelativePaths` and `bfcore.CountRootModules`)
- `[brain_client.go:ErrBrainUnavailable]`, `[brain_client.go:ErrBrainIndexMissing]`, `[brain_client.go:BrainPreflight]`, `[brain_client.go:BrainContextResponse]`, `[brain_client.go:BrainQueryOpts]`, `[brain_client.go:BrainClient]`, `[brain_client.go:SubprocessBrainClient]`, `[brain_client.go:ExtractQueryTokens]`, `[brain_client.go:BuildDisjunctiveFTSQuery]`, `[brain_client.go:IsProjectActiveOnDisk]`
- `[triage.go:EvaluateMetadataAndScope]`, `[triage.go:EvaluateProjectRegistration]`, `[triage.go:TriageWorkItem]`
- `[store.go:ErrIntakeLocked]`, `[store.go:ErrDedupeConflict]`, `[store.go:ErrIDCollision]`, `[store.go:ErrWorkItemNotFound]`, `[store.go:Store]`, `[store.go:NewStore]`
- `[render.go:RenderWorkItemMarkdown]`
- Subdirectories:
  - `cmd/intake`: [./cmd/intake/README.md](./cmd/intake/README.md) - CLI executable entry point (`intake.exe`)

## Invariants
- `[input.go:ReadRawInput]` Rejects payloads exceeding `MaxInputBytes` (`1 MiB`) and verifies ancestor paths contain zero symlinks or NTFS reparse points via `bfcore.CheckNoReparseChain`.
- `[input.go:ScanRawAndFlagsTier1]` & `[input.go:ScanDecodedTier1]` Enforces dual-phase Tier 1 G4 scanning on both raw bytes and decoded JSON string values (catching JSON Unicode escapes such as `\u0067hp_...`) before creating any `WorkItem` or spawning `0_Brain/brain.exe`.
- `[paths.go:SanitizeAffectedPaths]` Delegates to `bfcore.SanitizeRelativePaths` with `{AllowDir: false, SplitCommas: true, FoldCaseDedupe: false}`, rejecting personal paths (`win_user_path`/`unix_user_path`), traversal (`..`), drive/UNC/NT prefixes, leading/trailing slashes, Windows reserved device names, and globs (`*?[`) while preserving `sanitized_payload_sha256` bit-for-bit.
- `[brain_client.go:SubprocessBrainClient]` Wraps `*bfcore.SubprocessBrainClient` (inheriting strictly read-only `NG2` `Preflight` and `Context` while implementing `Query` via `ExecBrain`) and re-exports `ErrBrainUnavailable` and `ErrBrainIndexMissing`.
- `[triage.go:TriageWorkItem]` Evaluates FTD bounds via `bfcore.EvaluateFTDBounds` (Option `5B`: `<= 5` effective files, up to `5` sibling `*_test.go` exempt, `<= 10` physical files, `<= 1` root module) and path coverage via `bfcore.FindUncoveredPaths`.
- `[store.go:Store]` Protects all runtime disk writes (`workitems/`, `quarantine/`, `intake_events.jsonl`) with exclusive OS kernel lock `.intake.lock` (`bfcore.AcquireFileLock` with 250 ms bounded retry) and verifies `bfcore.ScanG4All` returns `0` findings on every serialized JSON buffer prior to writing.

## Side Effects
- Acquires and releases exclusive OS kernel file lock on `<dataDir>/.intake.lock`.
- Atomically writes and promotes `WI-<hash12>.json` files across `<dataDir>/quarantine/` and `<dataDir>/workitems/`.
- Appends G4-verified JSONL telemetry events to `<dataDir>/intake_events.jsonl`.
- Spawns read-only `0_Brain/brain.exe` (`context --json`, `query --json`) subprocesses and `git check-ignore` during `VerifyRuntime`.

## Verification
- Run: `go vet ./... && go test -v -count=1 -cover ./...`
