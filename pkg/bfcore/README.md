# pkg/bfcore (Shared SQLite-Free Core Primitives)

## Overview
Shared pure-Go (`CGO_ENABLED=0`) security, path-glob matching, OS kernel file locking, and ancestor-chain reparse-point verification package (`blackfactory/pkg/bfcore`) consumed by both `0_Brain/engine` and `1_Intake/engine`. Permits only `golang.org/x/sys` with zero SQLite dependencies.

## Interface
- `[g4.go:G4Tier]`, `[g4.go:Tier1HardSecret]`, `[g4.go:Tier2Environmental]`
- `[g4.go:G4Finding]`, `[g4.go:G4Redaction]`, `[g4.go:G4RulesCount]`
- `[g4.go:ScanG4All]`, `[g4.go:ScanTier1]`, `[g4.go:RedactTier2Text]`, `[g4.go:RedactAllG4]`, `[g4.go:MatchesPersonalPathG4]`
- `[paths.go:NormalizeCoverPath]`, `[paths.go:MatchCoverPath]`, `[paths.go:MatchCoverPathFold]`
- `[lock_windows.go:FileLock]`, `[lock_windows.go:ErrLockBusy]`, `[lock_windows.go:AcquireFileLock]`, `[lock_windows.go:FileLock.Unlock]`, `[lock_other.go:AcquireFileLock]`
- `[reparse_windows.go:IsReparseOrSymlink]`, `[reparse_windows.go:CheckNoReparseChain]`, `[reparse_other.go:IsReparseOrSymlink]`, `[reparse_other.go:CheckNoReparseChain]`

## Invariants
- `[g4.go:ScanG4All]` Evaluates all 11 canonical G4 rules line-by-line without transcribing matched secret substrings; suppresses `literal_private_key` on lines where `pem_private_key` matched.
- `[g4.go:RedactTier2Text]` Executes a fixed-point loop (up to 16 passes until 0 Tier 2 matches remain) preserving leading/trailing delimiters on `unix_user_path` so adjacent paths (`/home/alice /home/bob`) are both redacted.
- `[paths.go:MatchCoverPathFold]` Normalizes path separators to `/` and performs case-insensitive prefix and glob (`*`, `**`, `?`) matching for Windows compatibility.
- `[lock_windows.go:AcquireFileLock]` Acquires an exclusive OS kernel lock (`LockFileEx` on Windows, `flock` on POSIX) with bounded 25 ms retry backoff up to `maxWait`, returning `ErrLockBusy` on timeout and never unlinking the lock file on release.
- `[reparse_windows.go:CheckNoReparseChain]` Walks every existing ancestor component from `rootPath` to `targetPath` via `os.Lstat`, rejecting symlinks, NTFS directory junctions (`os.ModeIrregular`), and Win32 reparse points (`0x00000400`).

## Side Effects
- Opens/creates lock files and acquires/releases OS kernel locks via `AcquireFileLock` / `Unlock`.
- Inspects filesystem metadata via `os.Lstat` in `CheckNoReparseChain`.

## Verification
- Run: `go test -v -count=1 -cover ./...`
