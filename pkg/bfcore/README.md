# pkg/bfcore (Shared SQLite-Free Core Primitives)

## Overview
Shared pure-Go (`CGO_ENABLED=0`) security, path hygiene, normalized file hashing, FTD Option `5B` scope evaluation, read-only `0_Brain` subprocess transport, OS kernel file locking, and ancestor-chain reparse-point verification package (`blackfactory/pkg/bfcore`) consumed by `0_Brain/engine`, `1_Intake/engine`, and `2_Spec_Writer/engine`. Permits only `golang.org/x/sys` with zero SQLite dependencies.

## Interface
- `[g4.go:G4Tier]`, `[g4.go:Tier1HardSecret]`, `[g4.go:Tier2Environmental]`
- `[g4.go:G4Finding]`, `[g4.go:G4Redaction]`, `[g4.go:G4RulesCount]`
- `[g4.go:ScanG4All]`, `[g4.go:ScanTier1]`, `[g4.go:RedactTier2Text]`, `[g4.go:RedactAllG4]`, `[g4.go:MatchesPersonalPathG4]`
- `[paths.go:MaxObservedFileBytes]`, `[paths.go:PathSanitizeOpts]`
- `[paths.go:NormalizeCoverPath]`, `[paths.go:MatchCoverPath]`, `[paths.go:MatchCoverPathFold]`
- `[paths.go:SanitizeRelativePaths]`, `[paths.go:CheckNoReparseUpToNearestExisting]`, `[paths.go:ComputeNormalizedFileSHA256]`
- `[ftd.go:DefaultMaxFTDFiles]`, `[ftd.go:MaxFTDExemptGoTests]`, `[ftd.go:MaxFTDPhysicalFiles]`, `[ftd.go:MaxFTDRootModules]`
- `[ftd.go:FTDEvaluation]`, `[ftd.go:CountRootModules]`, `[ftd.go:EvaluateFTDBounds]`
- `[brain_client.go:ErrBrainUnavailable]`, `[brain_client.go:ErrBrainIndexMissing]`, `[brain_client.go:ErrBrainJournalActive]`
- `[brain_client.go:RiskHit]`, `[brain_client.go:BrainInvocation]`, `[brain_client.go:BrainDocMatch]`, `[brain_client.go:BrainPreflight]`, `[brain_client.go:BrainContextResponse]`
- `[brain_client.go:BrainClient]`, `[brain_client.go:SubprocessBrainClient]`, `[brain_client.go:NewSubprocessBrainClient]`, `[brain_client.go:SubprocessBrainClient.ExecBrain]`
- `[brain_client.go:FindUncoveredPaths]`, `[brain_client.go:RiskLevelRank]`
- `[lock_windows.go:FileLock]`, `[lock_windows.go:ErrLockBusy]`, `[lock_windows.go:AcquireFileLock]`, `[lock_windows.go:FileLock.Unlock]`, `[lock_other.go:AcquireFileLock]`
- `[reparse_windows.go:ErrReparsePoint]`, `[reparse_windows.go:IsReparseOrSymlink]`, `[reparse_windows.go:CheckNoReparseChain]`, `[reparse_other.go:IsReparseOrSymlink]`, `[reparse_other.go:CheckNoReparseChain]`

## Invariants
- `[g4.go:ScanG4All]` Evaluates all 11 canonical G4 rules line-by-line without transcribing matched secret substrings; suppresses `literal_private_key` on lines where `pem_private_key` matched.
- `[g4.go:RedactTier2Text]` Executes a fixed-point loop (up to 16 passes until 0 Tier 2 matches remain) preserving leading/trailing delimiters on `unix_user_path` so adjacent paths (`/home/alice /home/bob`) are both redacted.
- `[paths.go:SanitizeRelativePaths]` Validates raw paths before `path.Clean`, always returning a non-nil `[]string{}`, unconditionally rejecting globs (`*?[`), personal G4 paths (before `repoRoot` stripping), traversal (`..`), `"."`/`"./"`, control bytes, `:`, UNC/NT prefixes, Windows reserved device segments, and empty entries when `SplitCommas == false`.
- `[paths.go:ComputeNormalizedFileSHA256]` Enforces relative containment via `SanitizeRelativePaths` before `CheckNoReparseChain` and `os.Lstat`, returns `("absent", "", nil)` for missing files, rejects directories/reparse points and files exceeding `MaxObservedFileBytes` (`10 MiB`), and streams UTF-8 BOM stripping plus CRLF/CR-to-LF normalization.
- `[ftd.go:EvaluateFTDBounds]` Implements Option `5B`: exempts `*_test.go` files only when sharing a directory with at least one productive (non-`_test.go`) modified file up to `MaxFTDExemptGoTests` (`5`) and `MaxFTDPhysicalFiles` (`10`), enforces `<= 1` root module over all physical files without exceptions, counts all non-Go files `1:1`, and returns `WithinBounds: false` on empty input.
- `[brain_client.go:SubprocessBrainClient.Preflight]` Strictly read-only (`NG2`): checks `.index/brain.db` and absence of `.brain-journal.json` via `os.Lstat`, returning multi-wrapped errors so `errors.Is` matches both `ErrBrainUnavailable` and the specific sentinel (`ErrBrainIndexMissing` or `ErrBrainJournalActive`).
- `[lock_windows.go:AcquireFileLock]` Acquires an exclusive OS kernel lock (`LockFileEx` on Windows, `flock` on POSIX) with bounded 25 ms retry backoff up to `maxWait`, returning `ErrLockBusy` on timeout and never unlinking the lock file on release.
- `[reparse_windows.go:CheckNoReparseChain]` Walks every existing ancestor component from `rootPath` to `targetPath` via `os.Lstat`, rejecting symlinks, NTFS directory junctions (`os.ModeIrregular`), and Win32 reparse points (`0x00000400`).

## Side Effects
- Opens/creates lock files and acquires/releases OS kernel locks via `AcquireFileLock` / `Unlock`.
- Inspects filesystem metadata via `os.Lstat` (`CheckNoReparseChain`, `CheckNoReparseUpToNearestExisting`, `Preflight`) and streams regular files read-only (`ComputeNormalizedFileSHA256`).
- Spawns read-only `0_Brain/brain.exe` subprocesses (`ExecBrain`, `Context`).

## Verification
- Run: `go vet ./... && go test -v -count=1 -cover ./...`
