# 0_Brain Engine

## Overview
Core deterministic engine for `0_Brain` (Go module `blackfactory/brain`). Implements Open Knowledge Format (OKF) frontmatter parsing, vault validation, G4 secret scanning, atomic SQLite FTS5 full-text indexing, Git drift detection, non-blocking OS vault locking, and cryptographic proposal governance with fail-closed journal recovery.

## Interface
- `[drift.go:DriftItem]`, `[drift.go:GitHead]`, `[drift.go:CheckDrift]`
- `[frontmatter.go:Frontmatter]`, `[frontmatter.go:Document]`, `[frontmatter.go:Link]`, `[frontmatter.go:ParseDocument]`, `[frontmatter.go:ParseFile]`, `[frontmatter.go:ExtractRelativeLinks]`, `[frontmatter.go:ListVaultMarkdownFiles]`, `[frontmatter.go:LoadVaultDocs]`
- `[index.go:IndexStats]`, `[index.go:DefaultIndexPath]`, `[index.go:NormalizeCoverPath]`, `[index.go:MatchCoverPath]`, `[index.go:BuildIndex]`, `[index.go:BuildIndexAt]`
- `[lock_windows.go:FileLock]`, `[lock_windows.go:AcquireVaultLock]`, `[lock_windows.go:FileLock.Unlock]`, `[lock_other.go:AcquireVaultLock]`, `[lock_windows.go:ErrVaultLocked]`
- `[query.go:QueryOptions]`, `[query.go:SearchResult]`, `[query.go:Search]`, `[query.go:ContextForFiles]`, `[query.go:IsTransversalType]`
- `[validator.go:IssueCode]`, `[validator.go:ValidationIssue]`, `[validator.go:ValidationReport]`, `[validator.go:ValidateDocumentMeta]`, `[validator.go:ScanG4]`, `[validator.go:ValidateVault]`
- `[vault.go:ProposeRequest]`, `[vault.go:ProposeResult]`, `[vault.go:ApplyOptions]`, `[vault.go:JournalRecord]`, `[vault.go:ApplyResult]`, `[vault.go:IsProtectedVaultTarget]`, `[vault.go:InitVault]`, `[vault.go:ProposeUpdate]`, `[vault.go:ApplyUpdate]`, `[vault.go:RecoverJournal]`, `[vault.go:SignProposal]`
- Subdirectories:
  - `cmd`: [./cmd/README.md](./cmd/README.md) - CLI command implementations

## Invariants
- `[validator.go:ValidateVault]` Vault root must contain core files `AGENTS.md`, `index.md`, `constitution.md`, and `log.md`. Document IDs must be globally unique (case-insensitive). Markdown relative links must resolve to existing files within vault root without escaping (`..`).
- `[validator.go:ScanG4]` Deterministic secret scanning against 11 regex patterns (PEM private keys, bearer/API tokens, credentials in URIs, personal paths, emails).
- `[lock_windows.go:AcquireVaultLock]` Vault lock uses `LockFileEx` (`LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY`) on `.brain.lock`; returns `ErrVaultLocked` immediately on contention. Unlock releases handle without unlinking file.
- `[query.go:openReadOnlyDB]` SQLite index connection is enforced as read-only via `PRAGMA query_only = ON`.
- `[vault.go:ApplyUpdate]` Target path must match canonical allowlist; proposals in `_proposals/` must pass G4 scanning and OKF validation. Protected documents (`constitution.md`, `wiki/risk_zones.md`) strictly require valid Ed25519 signature from `.autor/autor.pub` within 24-hour expiration window.
- `[vault.go:recoverJournal]` Journal recovery is fail-closed: forward recovery if content matches hash, rollback if base matches, otherwise yields `ErrCommitDegraded`.

## Side Effects
- Traverses and reads filesystem markdown documents via `os.ReadFile` and `filepath.WalkDir`.
- Compiles SQLite FTS5 database at `.index/brain.db` via temporary file promotion and atomic replacement.
- Acquires and releases exclusive OS file lock on `.brain.lock`.
- Executes external `git` commands (`rev-parse`, `diff`) for drift detection.
- Writes proposal files to `_proposals/`, modifies vault target documents atomically, appends entries to `log.md`, and manages atomic `.brain-journal.json`.

## Verification
- Run: `go test -count=1 ./...`
