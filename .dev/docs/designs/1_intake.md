# Black Factory Team 1 (`1_Intake`): Deterministic Request Normalization, G4 Perimeter Gate, and Methodological Lane Triage

*   **Author:** blackpusch
*   **Status:** Draft
*   **Date:** 2026-11-22

--------------------------------------------------------------------------------

## Overview

`1_Intake` (`1_Intake/engine` -> `1_Intake/intake.exe`) is the deterministic perimeter gate and methodological triage engine for Black Factory. Compiled as a standalone pure-Go CLI executable (`CGO_ENABLED=0`, Windows-first, zero third-party dependencies), it ingests software engineering requests and defect reports from command-line flags or local structured files (JSON or Markdown with YAML frontmatter), enforces a two-tier secret and personal-data sanitization boundary (`G4`), consults the persistent knowledge engine (`0_Brain/brain.exe`) under the mandatory Read-Before-Act protocol, and emits a versioned JSON `WorkItem` (`schema_version: "1.0"`).

Each ingested request is deterministically routed into one of three operational outcomes under untracked local storage (`1_Intake/runtime/`, Option i privacy):
1. **`ASDD` (Agentic / Spec-Driven Development lane, `status: ready`, exit code `0`):** for new features, operations changes, multi-module or >5-file changes, requests with explicit or heuristic architectural impact flags (`contract`, `schema`, `db`, `auth`, `security`, `governance`), requests touching `CRITICO` risk zones (`RZ-001`), or requests affecting code paths without existing `adr`/`wiki` coverage in `0_Brain`.
2. **`FTD` (Fast-Track Development lane, `status: ready`, exit code `0`):** exclusively for bounded, single-module `bug`, `audit_fix`, or `refactor` requests with explicit `affected_paths`, active `adr`/`wiki` documentation in `0_Brain`, zero `impact_flags`, zero lexical heuristic escalations, and no `CRITICO` risk-zone hits.
3. **`Quarantine` (`status: needs_clarification`, exit code `2`):** a fail-safe `G6` hold when required metadata fields are missing, the target `project` is not registered in `0_Brain`, a personal or out-of-repository path is supplied inside `affected_paths`, or `affected_paths` are omitted and `0_Brain` full-text search cannot locate any matching `covers_paths` scope.

## The Problem

Black Factory separates software delivery into seven specialized teams (`0_Brain` through `6_Filtro_de_Riesgo`) backed by the deterministic execution ring `GengiSWAP` (`gengis.exe`). While `0_Brain` (`0_Brain/brain.exe`) already provides persistent Open Knowledge Format (OKF) memory, SQLite FTS5 indexing, `G4` secret validation, and Ed25519 governance, `1_Intake` currently exists only as a 5-line skeleton (`1_Intake/README.md`). Without a deterministic `1_Intake` engine, the factory faces five operational failure modes:

1. **Unvalidated Perimeter and Secret/PII Contamination (`G4` vs. `G1` Conflict):**
   Raw bug reports, crash dumps, and operator tickets routinely contain either live credentials (API keys, Bearer tokens, PEM private keys, database connection strings) or environmental personal data (Windows profile paths `C:\Users\<user>\...`, Unix home paths `/home/<user>/...`, author email addresses). Without a perimeter gate, raw secrets can leak into disk artifacts or downstream prompts; conversely, a naive single-tier blocker rejects every stack trace containing a local Windows username, preventing legitimate defect reports from entering the pipeline.
2. **Ad-Hoc and Non-Reproducible Lane Routing (`ASDD` vs. `FTD`):**
   Downstream teams (`2_Spec_Writer` and `3_Builder`) require a deterministic assignment between full specification workflows (`ASDD`) and bounded maintenance workflows (`FTD`), along with a suggested `GengiSWAP` template (`modulo_nuevo`, `correccion_hallazgos`, `auditoria_e2e`, `mapeo_estatico`). Leaving this classification to ad-hoc agent judgment allows high-risk changes to bypass architectural specification or inflates review overhead on localized bug fixes.
3. **Silent Risk-Zone Bypass on Empty Frontmatter `covers_paths` (`0_Brain` Integration Gap):**
   In the live `0_Brain` vault (`0_Brain/vault/wiki/risk_zones.md:11` and `0_Brain/schemas/risk_zones.template.md:11`), the YAML frontmatter declares `covers_paths: []` while the actual risk-zone catalog (`RZ-001` on `GengiSWAP/engine/` as `CRITICO`; `RZ-002` on `0_Brain/engine/` as `ALTO`) lives inside a 6-column Markdown table (`risk_zones.md:35-38`). Because `brain.exe context --json` (`0_Brain/engine/query.go:95-105`) matches files exclusively against frontmatter `covers_paths`, relying solely on `brain.exe context` produces `matched_paths: []` and silently misses every cataloged risk zone.
4. **FTS5 Query Mismatch and False `NO_BRAIN_COVERAGE` Escalations:**
   `brain.exe query` (`0_Brain/engine/query.go:408-452`) joins search terms with an implicit FTS5 `AND`. Passing raw multi-sentence titles or descriptions directly to `brain.exe query` returns `null` whenever a single natural-language word is absent from vault documents, causing false `NO_BRAIN_COVERAGE` outcomes and misinterpreting uppercase prose words (`AND`, `OR`, `NOT`, `NEAR`) as FTS5 syntax operators.
5. **Lack of Idempotency, Traceability, and Option i Privacy for Operational Incidents:**
   Repeated submissions of the same external ticket (`project + source + source_ref`) or retries after a clarification hold (`G6`) lack a deterministic deduplication key, atomic quarantine-to-ready promotion lifecycle, machine-queryable telemetry (`trace_id`, per-phase timings, payload-free `G4` rejection logs), and `.gitignore` isolation under Option i privacy.

**Proposed Solution:** Build `1_Intake/engine` (`module blackfactory/intake`, `CGO_ENABLED=0`, stdlib-only) and CLI `1_Intake/intake.exe` implementing in-memory two-tier `G4` enforcement, lightweight `0_Brain` preflight and subprocess consultation (`BrainClient`), dual risk-zone matching (frontmatter `covers_paths` + deterministic parser for `## Catálogo de Zonas de Riesgo` in `wiki/risk_zones.md`), separated locatability and lane triage rules, and atomic JSON `WorkItem` persistence under untracked `1_Intake/runtime/`.

## Goals

*   **G1 — Zero-CGO Pure-Go Determinism:** Compile `1_Intake/intake.exe` from `1_Intake/engine` with `CGO_ENABLED=0` and zero third-party Go dependencies, passing `go vet ./...` and `go test -count=1 ./...` on isolated `t.TempDir()` directories (`G3`).
*   **G2 — Zero False Negatives on Tier 1 G4 Secrets & Full `0_Brain` Parity:** Detect 100% of the 8 Tier 1 hard-secret rules from `0_Brain/engine/validator.go:49-105` in memory before creating any `WorkItem` file (`0` files written to `workitems/` or `quarantine/`, exit code `3`, logging only a payload-free `REJECTED_G4` event to `intake_events.jsonl` and `stderr`), while applying delimiter-preserving subgroup redaction (`[REDACTED:G4:<rule>]`) for the 3 Tier 2 environmental rules on parsed free-text fields (`title`, `description`, `note`) and enforcing 100% regex/glob equivalence with `0_Brain` via an automated cross-module parity test (`parity_test.go`).
*   **G3 — Zero False Negatives on Risk-Zone Detection (`RZ-001` / `RZ-002`):** Catch 100% of path matches against `wiki/risk_zones.md` even when YAML frontmatter `covers_paths` is `[]`, by combining `brain.exe context --json` output with a deterministic parser for the `## Catálogo de Zonas de Riesgo` Markdown table (`ID`, `Proyecto`, `Rutas cubiertas`, `Nivel`, `Compuerta obligatoria`), routing `CRITICO` hits unconditionally to `ASDD` with `requires_autor_hitl: true`.
*   **G4 — 100% Reproducible Triage & Idempotency:** Given identical input and `0_Brain` vault state, emit identical `lane` (`ASDD` | `FTD`), `risk_level`, `triage_reasons`, and deterministic `id` (`WI-<dedupe_key[:12]>`), preventing duplicate records across re-ingestions and supporting atomic promotion from `quarantine/` to `workitems/` via `intake ingest` or `intake resolve`.
*   **G5 — Windows Latency Budget (`< 500 ms` p95):** Complete end-to-end local ingestion and triage (including `0_Brain` preflight, subprocess calls to `brain.exe`, risk-zone table evaluation, and atomic file commit under `.intake.lock`) in under `500 ms` p95 on Windows for vaults containing 50–100 indexed documents.
*   **G6 — Strict Option i Privacy Compliance:** Keep all operational runtime artifacts (`1_Intake/runtime/workitems/`, `quarantine/`, `intake_log.md`, `intake_events.jsonl`, `.intake.lock`) and compiled binaries (`1_Intake/intake.exe`) excluded from Git via `.gitignore`, verified automatically by `intake.exe verify`.

## Non-Goals

*   **NG1 — Direct External Webhook or Network Integrations in v1:** GitHub Issues API, Sentry webhooks, web forms, Slack, and email ingestion are excluded from v1; v1 implements only CLI flags and local file/`stdin` inputs behind the `IngestSource` Go interface.
*   **NG2 — Direct Mutation or Indexing of `0_Brain/vault/`:** `1_Intake` is strictly a read-only consumer of `0_Brain`. It does not create proposals (`_proposals/`), modify `log.md`, or write to `.index/brain.db`.
*   **NG3 — Probabilistic or LLM-Driven Triage Inside `1_Intake`:** `1_Intake` never invokes an LLM or network service to classify work items; all lane routing is governed by deterministic rules and a versioned local lexical dictionary.
*   **NG4 — Separate `SDD` or `TDD` Output Lanes:** `SDD` is unified under the `ASDD` lane, and `TDD` is a mandatory execution discipline enforced downstream in `3_Builder` across both lanes.
*   **NG5 — Persisting Duplicate Markdown `WorkItem` Files by Default:** `WI-<hash12>.json` is the sole persisted source of truth in `workitems/` and `quarantine/`; human-readable Markdown views are rendered on demand via `intake.exe show --format md` (or explicit `--emit-md`).
*   **NG6 — Replacing Post-Build Diff Governance (`6_Filtro_de_Riesgo`):** `1_Intake` attaches pre-build risk metadata based on declared or inferred paths, whereas `6_Filtro_de_Riesgo` evaluates actual Git diffs after `3_Builder` execution.

## Open Questions

All architectural questions identified during ideation and the seven-lens Goldfish review have been resolved with `[AUTOR]` (`blackpusch`) and incorporated into this specification. No open questions remain.

## Critical User Journeys (CUJs)

### CUJ 1: Fast-Track (`FTD`) Ingestion of a Bounded Bug Report with Tier 2 Path Redaction

*   **Given**: `0_Brain/vault/` is initialized and indexed (`.index/brain.db` present, no `.brain-journal.json` pending), project `black-factory` is registered in `0_Brain/vault/index.md`, and an active wiki document covers `0_Brain/engine/validator.go` (which matches `RZ-002` at level `ALTO` in `wiki/risk_zones.md`).
*   **When**: An operator runs `intake.exe ingest --input bug_report.md --json`, where `bug_report.md` contains valid frontmatter (`project: black-factory`, `kind: bug`, `source: cli`, `source_ref: BUG-104`, `affected_paths: ["0_Brain/engine/validator.go"]`, no `impact_flags`) and a stack trace in the body containing `C:\Users\jdoe\go\src\validator.go` and `jdoe@example.com`.
*   **Then**:
    1. `intake.exe` verifies zero Tier 1 secrets in raw bytes, parses the document, and redacts the Windows user path and email inside `description` to `[REDACTED:G4:win_user_path]` and `[REDACTED:G4:email_address]`, recording `g4_redactions` counts and `sanitized_payload_sha256`.
    2. `intake.exe` queries `0_Brain/brain.exe context` and `query`, parses `## Catálogo de Zonas de Riesgo` in `wiki/risk_zones.md` (matching `RZ-002` with level `ALTO`), verifies single-module scope (`<= 5` files) and existing `0_Brain` coverage, and assigns `lane: "FTD"`, `risk_level: "ALTO"`, `suggested_template: "correccion_hallazgos"`, and `status: "ready"`.
    3. `intake.exe` acquires `1_Intake/runtime/.intake.lock`, writes `1_Intake/runtime/workitems/WI-<hash12>.json` atomically, appends entries to `intake_log.md` and `intake_events.jsonl`, prints the JSON result to `stdout`, and exits with code `0`.

### CUJ 2: Tier 1 G4 Hard-Secret Fail-Closed Rejection

*   **Given**: A developer or script invokes `intake.exe ingest --input ticket.json --json` where `ticket.json` contains a GitHub personal access token (`ghp_...`), a Bearer token, or the literal phrase `PRIVATE KEY`.
*   **When**: `intake.exe` executes Step 1 (Tier 1 G4 raw gate) in memory before creating any `WorkItem` or calling `0_Brain/brain.exe`.
*   **Then**:
    1. `intake.exe` aborts ingestion immediately, creating `0` files in `workitems/` and `0` files in `quarantine/` (`0` payload bytes written to disk).
    2. `intake.exe` appends a payload-free forensic record `{timestamp, trace_id, event: "REJECTED_G4", rule_name, field, line, raw_sha256, source, source_ref}` (containing zero secret substrings) to `1_Intake/runtime/intake_events.jsonl`, outputs the structured error JSON to `stderr`, and exits with code `3`.

### CUJ 3: Quarantine (`G6`) on Missing Scope and Atomic Promotion via `intake resolve`

*   **Given**: A non-technical defect report is ingested via `intake.exe ingest --input vague_bug.json --json` without `affected_paths` (or with a personal path `C:\Users\admin\app\main.go` inside `affected_paths`), and `brain.exe query` with extracted technical keywords matches `0` documents containing `covers_paths` (or the personal path in `affected_paths` is discarded by path hygiene).
*   **When**: `intake.exe` evaluates the Locatability & Metadata Gate (Step 3/4), and the operator subsequently supplies the missing repository-relative path via `intake.exe resolve --id WI-<hash12> --path "GengiSWAP/engine/ring.go" --note "Scope confirmed by AUTOR" --json`.
*   **Then**:
    1. On the initial `ingest`, `intake.exe` writes `1_Intake/runtime/quarantine/WI-<hash12>.json` with `status: "needs_clarification"`, `lane: ""`, `quarantine_info.reason_codes` (`UNCERTAIN_SCOPE` or `INVALID_AFFECTED_PATH_G4`), and an actionable `resolve_hint` command, exiting with code `2`.
    2. On `intake.exe resolve`, `intake.exe` patches `affected_paths` to `["GengiSWAP/engine/ring.go"]`, re-runs `0_Brain` context/risk-zone evaluation (matching `RZ-001` at level `CRITICO`), assigns `lane: "ASDD"` with `requires_autor_hitl: true` and `revision: 2`, atomically promotes the file to `1_Intake/runtime/workitems/WI-<hash12>.json`, removes the file from `quarantine/`, logs `PROMOTED` to `intake_log.md` and `intake_events.jsonl`, and exits with code `0`.

## Technical Plan (Architecture)

### 1. System Topology and Five-Stage Execution Pipeline

`1_Intake` is structured under `1_Intake/` following the same hybrid Option 3C + Option i architecture as `0_Brain`:

```text
1_Intake\
├── README.md                          (Team 1 boundaries, invariants, CLI verification)
├── schemas\
│   └── workitem.schema.json           (Canonical JSON Schema v1.0 for WorkItem)
├── skills\
│   ├── README.md                      (Skill catalog for Team 1)
│   └── intake-triage\
│       └── SKILL.md                   (Progressive-load operational guide for intake.exe)
├── engine\                            (Go module blackfactory/intake, CGO_ENABLED=0, stdlib-only)
│   ├── go.mod
│   ├── model.go                       (WorkItem v1.0 structs, enums, length-prefixed dedupe_key)
│   ├── input.go                       (IngestSource port: CLI flags, JSON, Markdown+YAML, stdin)
│   ├── g4.go                          (11 G4 rules, Tier 1 fail-closed, Tier 2 subgroup redactor)
│   ├── paths.go                       (POSIX normalization, repo-relative check, personal path guard)
│   ├── brain_client.go                (BrainClient interface, preflight, keyword extractor, null-safe JSON)
│   ├── risk_zones.go                  (MatchCoverPath & wiki/risk_zones.md 6-column table parser)
│   ├── triage.go                      (Two-stage Locatability + ASDD/FTD engine & bilingual lexicon)
│   ├── store.go                       (Atomic storage, quarantine promotion, intake_log.md, events.jsonl)
│   ├── render.go                      (On-demand ASCII/Box-Drawing Markdown renderer for show/export)
│   ├── lock_windows.go / lock_other.go(Kernel file lock on runtime/.intake.lock)
│   ├── reparse_windows.go / reparse_other.go (Symlink and NTFS reparse-point rejection)
│   ├── parity_test.go                 (Cross-module G4 regex & MatchCoverPath parity vs 0_Brain)
│   ├── intake_test.go                 (Unit & table-driven triage, G4, risk-zone, and keyword tests)
│   └── cmd\intake\
│       ├── main.go                    (CLI dispatcher: ingest, resolve, list, show, verify; exit 0..5)
│       └── main_test.go               (E2E CLI lifecycle tests covering all 6 exit codes on t.TempDir)
├── intake.exe                         (Compiled local binary; ignored by .gitignore)
└── runtime\                           (Untracked Option i operational data; ignored by .gitignore)
    ├── .intake.lock                   (OS kernel lock file; never unlinked on release)
    ├── workitems\                     (Triaged ready WorkItems: WI-<hash12>.json)
    ├── quarantine\                    (G6 holds: WI-<hash12>.json or WIQ-<hash12>.json)
    ├── intake_log.md                  (Append-only human-readable operational log)
    └── intake_events.jsonl            (Append-only structured telemetry & payload-free G4 rejections)
```

#### Five-Stage Ingestion and Triage Data Flow

```text
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 1: RAW INPUT ADAPTER & TIER 1 G4 RAW GATE (In-Memory Only)                  │
│  • Read raw bytes from CLI flags, --input <file.json|.md>, or --input - (stdin)   │
│  • Scan raw lines against 8 Tier 1 G4 hard-secret rules (plus 3 Tier 2 rules if   │
│    --strict-g4 is active), suppressing literal_private_key on pem_private_key line│
│  • On hit: write 0 files to workitems/ or quarantine/; append payload-free        │
│    REJECTED_G4 event to intake_events.jsonl; print JSON error to stderr ──► EX 3  │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │ (0 Tier 1 hits)
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 2: PARSING, TIER 2 SUBGROUP REDACTION & AFFECTED_PATHS HYGIENE              │
│  • Parse JSON or Markdown+YAML frontmatter into RawIntake struct                  │
│  • Apply delimiter-preserving Tier 2 redaction ([REDACTED:G4:<rule>]) ONLY to     │
│    parsed in-memory string values of free-text fields: title, description, note   │
│  • Validate affected_paths: normalize \ -> /, strip leading ./, check for .. or   │
│    personal/out-of-repo paths (win_user_path / unix_user_path / absolute drive).  │
│    If offending path found: discard raw path in memory, flag for quarantine       │
│    (invalid_path_reason: "G4_personal_path_in_affected_paths")                    │
│  • Compute canonical sanitized_payload_sha256 and length-prefixed dedupe_key      │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 3: 0_BRAIN PREFLIGHT, PROJECT VALIDATION & METADATA COMPLETENESS GATE       │
│  • Preflight: verify 0_Brain/vault/.index/brain.db exists and no pending          │
│    .brain-journal.json exists; if journal exists, run brain.exe verify --json     │
│    (if brain.db missing or verify exits 3/4 or fails) ────────────────────► EX 4  │
│  • Validate project: query brain.exe query --type project --project <p> --json,   │
│    falling back to parsing "## Proyectos Registrados" in 0_Brain/vault/index.md   │
│  • If required fields (project, title, description, kind, source, source_ref) are │
│    missing, project is unregistered, or Stage 2 flagged invalid affected_paths:   │
│    persist WI/WIQ JSON to runtime/quarantine/ with resolve_hint ──────────► EX 2  │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │ (Metadata complete & project valid)
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 4: 0_BRAIN CONSULTATION, LOCATABILITY GATE & DUAL RISK-ZONE MATCHING        │
│  • Extract top 3-5 technical keywords from title/description (strip ES/EN         │
│    stop-words & FTS5 reserved operators AND/OR/NOT/NEAR)                          │
│  • Case A (affected_paths omitted): run brain.exe query --project <p> --json      │
│    (null-safe decoder). Collect matched docs with non-empty covers_paths.         │
│    If 0 docs with covers_paths match -> persist to quarantine/ (UNCERTAIN_SCOPE)  │
│    ───────────────────────────────────────────────────────────────────────► EX 2  │
│  • Case B (explicit or inferred paths available): run brain.exe context --json    │
│    and brain.exe query --json; parse 6-column "## Catálogo de Zonas de Riesgo"    │
│    table in wiki/risk_zones.md (filtering by Proyecto column and MatchCoverPath)  │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 5: TWO-LANE TRIAGE (ASDD vs. FTD), IDEMPOTENCY & ATOMIC STORAGE             │
│  • Evaluate deterministic triggers + versioned bilingual heuristic lexicon        │
│    ├─ Any trigger fires ──► lane = "ASDD"                                         │
│    └─ Zero triggers fire ─► lane = "FTD"                                          │
│  • Acquire kernel lock runtime/.intake.lock:                                      │
│    ├─ Existing ready WI with same dedupe_key & same sha256 ──► idempotent   EX 0  │
│    ├─ Existing ready WI with different sha256 & no --update ─► conflict     EX 5  │
│    └─ Write temp JSON -> atomic rename to workitems/WI-<hash12>.json (deleting    │
│       prior quarantine file if promoted) -> append intake_log.md & events ──► EX 0│
└───────────────────────────────────────────────────────────────────────────────────┘
```

### 2. Two-Tier G4 Perimeter Design and Regex Parity Contract

`1_Intake/engine/g4.go` mirrors the exact 11 regular expressions defined in `0_Brain/engine/validator.go:49-105`, partitioned into two operational tiers:

| Rule Name (`0_Brain` ID) | Tier | Default Action | Target Scope | Special Handling |
| :--- | :---: | :--- | :--- | :--- |
| `pem_private_key` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Marks line to suppress `literal_private_key` (`validator.go:159-167`) |
| `literal_private_key` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Skipped on lines where `pem_private_key` already matched |
| `bearer_token` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | 0 payload files written; payload-free `REJECTED_G4` event in JSONL |
| `known_token` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Matches `ghp_`, `github_pat_`, `sk-`, `AKIA`, `AIza`, `xox*` |
| `jwt_token` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Matches 3-part base64url `eyJ...` tokens |
| `conn_uri_credentials` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Matches `postgres://`, `mysql://`, `mongodb://`, `redis://`, etc. with user:pass |
| `conn_password_param` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Matches `password=`, `passwd=`, `pwd=` parameters |
| `secret_assignment` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw input lines | Matches `api_key`, `access_token`, `client_secret`, `private_key` assignments |
| `win_user_path` | **Tier 2** | Subgroup Redact (or `Exit 3` w/ `--strict-g4`) | Parsed `title`, `description`, `note` | Replaces `C:\Users\<user>` with `[REDACTED:G4:win_user_path]`; in `affected_paths` triggers quarantine (`Exit 2`) |
| `unix_user_path` | **Tier 2** | Subgroup Redact (or `Exit 3` w/ `--strict-g4`) | Parsed `title`, `description`, `note` | Preserves leading `[ \t"'(=:]` and trailing `[/ \t"')]`, replacing only `(?:/home|/Users)/[A-Za-z0-9._-]+` |
| `email_address` | **Tier 2** | Subgroup Redact (or `Exit 3` w/ `--strict-g4`) | Parsed `title`, `description`, `note` | Replaces email token with `[REDACTED:G4:email_address]` |

**Cross-Module Parity Enforcement (`parity_test.go`):**
To guarantee that `1_Intake/engine/g4.go` and `1_Intake/engine/risk_zones.go` never drift from `0_Brain/engine/validator.go` and `0_Brain/engine/index.go` without importing `modernc.org/sqlite`, `1_Intake/engine/parity_test.go` reads `../../0_Brain/engine/validator.go` during `go test`, extracts the literal `g4Rules` slice entries (`name` and raw backtick regex strings), and asserts 1-to-1 equality against `1_Intake`'s compiled rule registry, plus asserts that every Tier 2 redacted string passes `0_Brain`'s `ScanG4` patterns with zero violations.

### 3. `0_Brain` Integration Contract (`BrainClient`) and Dual Risk-Zone Parser

`1_Intake/engine/brain_client.go` defines the `BrainClient` interface to decouple triage logic from the transport mechanism (CLI subprocess in v1; local MCP JSON-RPC client in Phase 2):

```go
type BrainClient interface {
    Preflight(vaultRoot, dbPath string) (BrainPreflight, error)
    IsProjectRegistered(vaultRoot, dbPath, project string) (bool, error)
    Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error)
    Context(vaultRoot, dbPath, project string, files []string) ([]BrainDocMatch, BrainInvocation, error)
    LoadRiskZones(vaultRoot string, contextDocs []BrainDocMatch) ([]RiskZoneRow, string, error)
}
```

1. **Lightweight Preflight (`Preflight`):**
   - Checks `os.Stat` on `<vaultRoot>/.index/brain.db` (capturing `brain_db_mtime`). If missing, returns `ErrBrainIndexMissing` -> `intake.exe` exits with code `4`.
   - Checks `os.Stat` on `<vaultRoot>/.brain-journal.json`. If the journal file exists (indicating an interrupted or active `0_Brain` transaction), invokes `brain.exe verify --vault <vaultRoot> --json`. If `brain.exe verify` returns exit code `3` (`ErrJournalCorrupt` / `ErrCommitDegraded`), exit code `4` (`ErrVaultLocked`), or `ok: false`, `intake.exe` aborts with exit code `4`.
2. **Project Registration Check (`IsProjectRegistered`):**
   - Queries `brain.exe query --vault <vaultRoot> --type project --project <project> --limit 1 --json`. If at least one active document matches, returns `true`.
   - Fallback (solving the current state where `0_Brain/vault/index.md:35` lists `black-factory` before `projects/black-factory.md` exists on disk): reads `<vaultRoot>/index.md` (with reparse-point rejection) and parses the Markdown table under `## Proyectos Registrados`, matching the `Proyecto` column case-insensitively after stripping backticks.
3. **Deterministic Technical Keyword Extractor for FTS5 (`ExtractQueryTokens`):**
   - Scans `title` and `description` for technical tokens: file paths (`/` or `\`), filenames with extensions (`*.go`, `*.md`, `*.json`, `*.sql`), `CamelCase` or `snake_case` identifiers, and catalog codes (`RZ-\d+`, `ADR-\d+`, `G[1-6]`), followed by remaining alphanumeric words `>= 4` chars.
   - Filters out a built-in bilingual (Spanish/English) stop-word table and **strips/escapes FTS5 reserved uppercase operators (`AND`, `OR`, `NOT`, `NEAR`)** and punctuation so `0_Brain/engine/query.go:buildFTS5Query` never receives malformed FTS5 expressions.
   - Selects up to 5 highest-specificity tokens and queries `brain.exe query` (deduplicating returned `BrainDocMatch` items by `id`).
4. **Dual Risk-Zone Evaluation (`LoadRiskZones`):**
   - Inspects `BrainDocMatch` entries of `type: "risk_zones"` returned by `Context` (or locates `<vaultRoot>/wiki/risk_zones.md`).
   - Computes `risk_zones_sha256` of the file bytes and parses the 6-column Markdown table under `## Catálogo de Zonas de Riesgo`:
     `| ID | Proyecto | Rutas cubiertas (`covers_paths`) | Nivel | Compuerta obligatoria | Evidencia `G5` |`
   - Strips surrounding backticks from each cell, splits comma/semicolon-separated patterns in column 3, filters rows where `strings.EqualFold(row.Project, workItem.Project)` or `row.Project == "*"`, and evaluates each normalized `affected_path` using `MatchCoverPath` (identical DP glob algorithm to `0_Brain/engine/index.go:54-93`).
   - Unions any table matches with any non-empty `matched_paths` returned directly by `brain.exe context --json`.
   - **Fail-Safe Invariant:** If `wiki/risk_zones.md` is missing, unreadable, contains zero valid rows under `## Catálogo de Zonas de Riesgo`, or contains an unrecognized `Nivel` value (outside `CRITICO`, `ALTO`, `MEDIO`, `BAJO`), `1_Intake` records reason code `RISK_ZONES_UNPARSEABLE_FAILSAFE`, sets `requires_autor_hitl: true`, and forces `lane: "ASDD"`.

### 4. Canonical `WorkItem` JSON Schema (`schema_version: "1.0"`)

Every persisted file in `1_Intake/runtime/workitems/WI-<hash12>.json` or `1_Intake/runtime/quarantine/WI[Q]-<hash12>.json` conforms to the following JSON structure:

```json
{
  "schema_version": "1.0",
  "id": "WI-9f86d081884c",
  "dedupe_key": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
  "trace_id": "trc-20261122T130000Z-9f86d081",
  "revision": 1,
  "status": "ready",
  "created_at": "2026-11-22T13:00:00Z",
  "updated_at": "2026-11-22T13:00:00Z",
  "project": "black-factory",
  "title": "Fix delimiter capture in validator G4 path scan",
  "description": "Stack trace in [REDACTED:G4:win_user_path]\\app.go reported by [REDACTED:G4:email_address]",
  "note": "",
  "kind": "bug",
  "source": "cli",
  "source_ref": "BUG-2026-042",
  "affected_paths": [
    "0_Brain/engine/validator.go"
  ],
  "paths_confidence": "explicit",
  "impact_flags": [],
  "lane": "FTD",
  "suggested_template": "correccion_hallazgos",
  "risk_level": "ALTO",
  "requires_autor_hitl": false,
  "risk_hits": [
    {
      "id": "RZ-002",
      "project": "black-factory",
      "level": "ALTO",
      "matched_paths": [
        "0_Brain/engine/validator.go"
      ],
      "mandatory_gate": "go vet + go test -count=1 ./... + revisión G4/G5"
    }
  ],
  "triage_reasons": [
    {
      "code": "FTD_ELIGIBLE_BOUNDED_MAINTENANCE",
      "rule_kind": "deterministic",
      "ruleset_version": "triage-det-v1.0",
      "detail": "Single-module bug with explicit affected_paths (1<=5), active 0_Brain coverage, zero impact_flags, and ALTO (non-CRITICO) risk zone",
      "matched_terms": []
    }
  ],
  "sanitization": {
    "strict_g4": false,
    "sanitized_payload_sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "g4_redactions": [
      {
        "rule": "win_user_path",
        "field": "description",
        "count": 1
      },
      {
        "rule": "email_address",
        "field": "description",
        "count": 1
      }
    ]
  },
  "brain_evidence": {
    "vault_path": "0_Brain/vault",
    "brain_db_mtime": "2026-11-22T12:00:00Z",
    "risk_zones_sha256": "4a8b9c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b",
    "extracted_query_tokens": [
      "validator.go",
      "scang4",
      "delimiter"
    ],
    "invocations": [
      {
        "command": "0_Brain/brain.exe context --vault 0_Brain/vault --project black-factory --files 0_Brain/engine/validator.go --json",
        "exit_code": 0,
        "duration_ms": 28,
        "result_count": 3
      },
      {
        "command": "0_Brain/brain.exe query --vault 0_Brain/vault --project black-factory --q validator.go --json",
        "exit_code": 0,
        "duration_ms": 21,
        "result_count": 2
      }
    ],
    "matched_docs": [
      {
        "id": "brain-constitution",
        "rel_path": "constitution.md",
        "type": "constitution",
        "verified_head": "",
        "transversal": true
      },
      {
        "id": "brain-risk-zones",
        "rel_path": "wiki/risk_zones.md",
        "type": "risk_zones",
        "verified_head": "",
        "transversal": true
      }
    ]
  },
  "quarantine_info": null
}
```

**Canonical `dedupe_key` Computation:**
When `project`, `source`, and `source_ref` are all non-empty after trimming whitespace, `dedupe_key` is computed via length-prefixed SHA256 to prevent delimiter collision if any field contains `|`:
`dedupe_key = sha256(fmt.Sprintf("%d:%s|%d:%s|%d:%s", len(p), p, len(s), s, len(r), r))` where `p = lower(trim(project))`, `s = lower(trim(source))`, `r = lower(trim(source_ref))`. The `WorkItem` ID is `WI-<dedupe_key[:12]>`.
If any of the three identity fields is missing during quarantine (`Exit 2`), a provisional ID `WIQ-<sanitized_payload_sha256[:12]>` is assigned in `quarantine/` and upgraded to `WI-<dedupe_key[:12]>` when `intake.exe resolve` supplies the missing identity fields.

### 5. Triage Decision Matrix (`Quarantine` vs. `ASDD` vs. `FTD`)

| Priority | Rule Code | `rule_kind` | Trigger Condition | Outcome (`status` / `lane`) | Exit Code |
| :---: | :--- | :--- | :--- | :--- | :---: |
| **1** | `G4_TIER1_SECRET` | `deterministic` | Raw input matches any of the 8 Tier 1 G4 rules (or Tier 2 w/ `--strict-g4`) | Rejected (0 files; payload-free `REJECTED_G4` in JSONL) | `3` |
| **2** | `BRAIN_UNAVAILABLE` | `deterministic` | `.index/brain.db` missing, `.brain-journal.json` corrupt (`exit 3`), or `.brain.lock` busy (`exit 4`) | Aborted (`G6` stop before triage) | `4` |
| **3** | `MISSING_REQUIRED_FIELDS` | `deterministic` | Any of `project`, `title`, `description`, `kind`, `source`, `source_ref` is empty, or `kind` / `impact_flags` invalid | `needs_clarification` in `quarantine/` | `2` |
| **4** | `UNREGISTERED_PROJECT` | `deterministic` | `project` not found in `type: project` docs or `## Proyectos Registrados` in `0_Brain/vault/index.md` | `needs_clarification` in `quarantine/` | `2` |
| **5** | `INVALID_AFFECTED_PATH_G4` | `deterministic` | `affected_paths` contains personal path (`win_user_path`/`unix_user_path`), drive letter outside repo, or `..` | Offending path discarded in memory -> `needs_clarification` in `quarantine/` | `2` |
| **6** | `UNCERTAIN_SCOPE` | `deterministic` | `affected_paths` omitted on input AND `brain.exe query` returns 0 documents with non-empty `covers_paths` | `needs_clarification` in `quarantine/` | `2` |
| **7** | `RISK_ZONE_CRITICO` | `deterministic` | Any path matches a `CRITICO` row in `wiki/risk_zones.md` (e.g. `RZ-001` `GengiSWAP/engine/`) | `ready` / `lane: "ASDD"`, `requires_autor_hitl: true` | `0` |
| **8** | `RISK_ZONES_UNPARSEABLE_FAILSAFE` | `deterministic` | `wiki/risk_zones.md` missing or `## Catálogo de Zonas de Riesgo` table malformed | `ready` / `lane: "ASDD"`, `requires_autor_hitl: true` | `0` |
| **9** | `KIND_REQUIRES_ASDD` | `deterministic` | `kind == "feature"` or `kind == "ops"` | `ready` / `lane: "ASDD"` | `0` |
| **10** | `EXPLICIT_IMPACT_FLAGS` | `deterministic` | `len(impact_flags) > 0` (`contract`, `schema`, `db`, `auth`, `security`, `governance`) | `ready` / `lane: "ASDD"` | `0` |
| **11** | `INFERRED_PATHS_FAILSAFE` | `deterministic` | `paths_confidence != "explicit"` (`inferred_from_query` matched scope in `0_Brain`) | `ready` / `lane: "ASDD"` | `0` |
| **12** | `MULTI_MODULE_OR_FILE_THRESHOLD` | `deterministic` | `affected_paths` spans `> 1` top-level directory/module or `> max_ftd_files` (default `5`) | `ready` / `lane: "ASDD"` | `0` |
| **13** | `NO_BRAIN_COVERAGE` | `deterministic` | Explicit `affected_paths` have `0` matching `type: "wiki"` or `type: "adr"` documents in `0_Brain` | `ready` / `lane: "ASDD"` | `0` |
| **14** | `HEURISTIC_LEXICAL_ESCALATION` | `heuristic` | Bilingual word-boundary regex catalog (`lexicon-v1.0`: e.g., `migration`, `migración`, `ddl`, `schema`, `esquema`, `jwt`, `ed25519`, `oauth`, `csrf`, `constitution`, `breaking change`) matches `title` or `description` | `ready` / `lane: "ASDD"` (records `matched_terms`) | `0` |
| **15** | `FTD_ELIGIBLE_BOUNDED_MAINTENANCE` | `deterministic` | Rules 1–14 did not fire (`kind` in `bug`/`audit_fix`/`refactor`, explicit single-module paths `<= 5`, active `wiki`/`adr` coverage, `risk_level` is `NONE` or `ALTO`) | `ready` / `lane: "FTD"` (carries `risk_level: "ALTO"` if `RZ-002` matched) | `0` |

**Downstream `suggested_template` Mapping for `GengiSWAP`:**
- `lane == "ASDD"` and `kind == "feature"` -> `"modulo_nuevo"`
- `lane == "ASDD"` and `kind == "ops"` -> `"auditoria_e2e"`
- `lane == "ASDD"` and `kind` in `bug`/`audit_fix`/`refactor` -> `"correccion_hallazgos"` (or `"mapeo_estatico"` when triggered by `NO_BRAIN_COVERAGE`)
- `lane == "FTD"` -> `"correccion_hallazgos"`

### 6. State Machine and CLI Subcommand Contract (`intake.exe`)

```text
                       ┌─────────────────────────┐
                       │   Raw Request Input     │
                       └────────────┬────────────┘
                                    │
         ┌──────────────────────────┼──────────────────────────┐
         │ (Tier 1 G4 Secret)       │ (Missing fields/scope)   │ (Complete & locatable)
         ▼                          ▼                          ▼
┌─────────────────┐      ┌─────────────────────┐      ┌─────────────────────┐
│   REJECTED_G4   │      │     QUARANTINED     │      │      TRIAGED        │
│   (Exit Code 3) │      │ status: needs_clarif│      │   status: ready     │
│ 0 WorkItem files│      │ dir: quarantine/    │      │   dir: workitems/   │
│ Event in JSONL  │      │   (Exit Code 2)     │      │ lane: ASDD | FTD    │
└─────────────────┘      └──────────┬──────────┘      │   (Exit Code 0)     │
                                    │                 └──────────▲──────────┘
                                    │ intake resolve --id WI-... │
                                    │ OR re-ingest same dedupe   │
                                    └────────────────────────────┘
                                     (Atomic promote: revision++,
                                      delete from quarantine/,
                                      log PROMOTED)
```

All 5 subcommands accept `--data-dir` (or `BF_INTAKE_DIR`, default `1_Intake/runtime`), `--vault` (or `BRAIN_VAULT`, default `0_Brain/vault`), `--brain-bin` (or `BRAIN_BIN`, default `0_Brain/brain.exe`), and `--json`, using flag reordering (`reorderFlags` pattern from `0_Brain/engine/cmd/brain/main.go:141`):

1. **`intake.exe ingest`:**
   - Flags: `--input <path|->`, `--project`, `--title`, `--description`, `--note`, `--kind`, `--source`, `--source-ref`, `--path` (repeatable), `--paths` (CSV), `--impact-flag` (repeatable), `--max-ftd-files` (default `5`), `--strict-g4`, `--update`, `--emit-md`, `--dry-run`, `--json`.
   - Precedence: If `--input` is provided, explicit CLI flags override corresponding fields in the input file. Reading from `stdin` requires explicit `--input -` (never blocks implicitly on open stdin handles in Windows).
2. **`intake.exe resolve`:**
   - Flags: `--id <WI-ID>` (required), `--project`, `--title`, `--description`, `--note`, `--kind`, `--source`, `--source-ref`, `--path` (repeatable), `--paths` (CSV), `--impact-flag` (repeatable), `--clear-heuristic-escalation` (requires `--note` documenting `[AUTOR]` justification), `--strict-g4`, `--emit-md`, `--json`.
   - Behavior: Loads the quarantined JSON from `runtime/quarantine/<WI-ID>.json`, applies supplied field patches, re-runs Stages 1–5, and if all gates pass, writes `runtime/workitems/WI-<hash12>.json` (`revision: prev + 1`), removes the quarantine file, appends `PROMOTED` to `intake_log.md` and `intake_events.jsonl`, and exits `0` (or exits `2` if still incomplete).
3. **`intake.exe list`:**
   - Flags: `--status <ready|needs_clarification>`, `--lane <ASDD|FTD>`, `--project <slug>`, `--risk <NONE|ALTO|CRITICO>`, `--json`.
   - Behavior: Scans `workitems/` and `quarantine/` sorted deterministically by `updated_at DESC, id ASC`.
4. **`intake.exe show`:**
   - Flags: `--id <WI-ID>` (or positional `<WI-ID>`), `--format <json|md>` (default `json` when `--json` is set, otherwise `md`), `--json`.
   - Behavior: Renders either the authoritative JSON or an on-demand Markdown view with ASCII/Unicode Box-Drawing summary tables (never Mermaid).
5. **`intake.exe verify`:**
   - Flags: `--repo-root <path>`, `--json`.
   - Behavior: Acquires `.intake.lock` and verifies: (a) `1_Intake/runtime/` is ignored by Git (`git check-ignore`), (b) zero `WorkItem` IDs or `dedupe_key`s exist simultaneously in `workitems/` and `quarantine/`, (c) every persisted `.json` file conforms to `schema_version: "1.0"`, matches its `id`/`dedupe_key`, and passes all 11 `G4` rules with zero violations, (d) `intake_log.md` and `intake_events.jsonl` pass `G4` scanning, and (e) `0_Brain/brain.exe verify --vault <vault> --json` succeeds and `wiki/risk_zones.md` table parses cleanly.

## Alternatives Considered

### Alternative A: Direct Go Module Import of `blackfactory/brain` (`0_Brain/engine`) into `1_Intake/engine`

*   **Pros**: Allows direct in-process calls to `brain.ScanG4`, `brain.MatchCoverPath`, `brain.Search`, and `brain.ContextForFiles` without spawning `0_Brain/brain.exe` or maintaining a cross-module parity test.
*   **Cons**: Pulls `modernc.org/sqlite v1.62.0` (`0_Brain/engine/go.mod:5`) directly into `1_Intake/intake.exe`, inflating binary size (~12 MB) and compilation times; tightly couples Team 1 and Team 0 compile-time release cycles; bypasses literal CLI command evidence capture (`G1`); and complicates the Phase 2 transition to a standalone `brain mcp` server (`Docs/BLACK_FACTORY_BRAIN.md:165-173`).
*   **Reason for Rejection**: Rejected by author: `1_Intake/engine` must remain a zero-dependency pure-Go stdlib module that interacts with `0_Brain` through the `BrainClient` subprocess boundary (`0_Brain/brain.exe --json`), while `parity_test.go` enforces 100% compile-time/test-time equivalence for the 11 `G4` regex rules and `MatchCoverPath`.

### Alternative B: Emitting Three Separate Methodological Lanes (`ASDD`, `SDD`, and `FTD`)

*   **Pros**: Mirrors the historical terminology (`ASDD`/`SDD`, `FTD`, `TDD`) referenced in `README.md:7` and `0_Brain/schemas/constitution.template.md:25`.
*   **Cons**: At intake time—before `2_Spec_Writer` has produced a specification—there is no deterministic observable boundary separating `ASDD` from `SDD`, which would force arbitrary classifications. Meanwhile, `TDD` is a build-time verification discipline in `3_Builder`, not an intake routing lane.
*   **Reason for Rejection**: Rejected by author: `1_Intake` emits two mutually exclusive lanes (`ASDD` unifying `ASDD`/`SDD`, and `FTD` for bounded single-module maintenance), while `TDD` remains mandatory across both lanes in `3_Builder`.

### Alternative C: Uniform Fail-Closed G4 Policy Across All 11 Rules (No Tier 2 Redaction)

*   **Pros**: Matches `0_Brain/engine/validator.go:152-178` (`ScanG4`), which treats all 11 `G4` rules identically as blocking violations; requires zero redaction logic.
*   **Cons**: Windows crash dumps, Go compiler diagnostics, and user bug reports frequently contain local profile paths (`C:\Users\<user>\...`, `/home/<user>/...`) or commit author emails. Blocking all 11 rules unconditionally at the perimeter prevents real stack traces from ever being ingested into Black Factory.
*   **Reason for Rejection**: Rejected by author: `1_Intake` enforces a two-tier `G4` perimeter (8 Tier 1 hard-secret rules fail-closed with exit code `3` and `0` payload files written; 3 Tier 2 environmental rules redacted via delimiter-preserving subgroup replacement on parsed `title`/`description`/`note` fields, or quarantined on `affected_paths`), with `--strict-g4` available to promote Tier 2 to fail-closed when required.

### Alternative D: Relying Solely on `brain.exe context --json` `matched_paths` for Risk-Zone Detection

*   **Pros**: Avoids reading and parsing the Markdown table inside `0_Brain/vault/wiki/risk_zones.md` from `1_Intake`.
*   **Cons**: Both `0_Brain/vault/wiki/risk_zones.md:11` and `0_Brain/schemas/risk_zones.template.md:11` declare `covers_paths: []` in YAML frontmatter and store the actual path patterns (`GengiSWAP/engine/`, `0_Brain/engine/`) and severity levels (`CRITICO`, `ALTO`) in the `## Catálogo de Zonas de Riesgo` Markdown table (`risk_zones.md:35-38`). Because `ContextForFiles` (`0_Brain/engine/query.go:95-105`) only evaluates frontmatter `covers_paths`, relying solely on `brain.exe context --json` misses 100% of cataloged risk zones.
*   **Reason for Rejection**: Rejected by author: `1_Intake` must apply a dual risk-zone strategy combining `brain.exe context --json` with a deterministic parser for `## Catálogo de Zonas de Riesgo` in `wiki/risk_zones.md` (failing safe to `ASDD` if the table is missing or unparseable).

### Alternative E: Unconditional `ASDD` Routing for All Risk-Zone Levels (`CRITICO` and `ALTO` Alike)

*   **Pros**: Simplifies risk-zone triage to a binary check (any match in `wiki/risk_zones.md` forces `ASDD`).
*   **Cons**: In `0_Brain/vault/wiki/risk_zones.md:37-38`, `RZ-001` (`GengiSWAP/engine/`) has level `CRITICO` (requiring PR + `4_Revisores` + `[AUTOR]` approval), whereas `RZ-002` (`0_Brain/engine/`) has level `ALTO` (requiring `go vet`, `go test -count=1 ./...`, and `G4`/`G5` review). Routing every `ALTO` hit unconditionally to `ASDD` makes `FTD` (`correccion_hallazgos`) unusable for localized bug or audit fixes inside `0_Brain/engine/`.
*   **Reason for Rejection**: Rejected by author: `CRITICO` hits unconditionally force `ASDD` and `requires_autor_hitl: true`, whereas `ALTO` hits force `ASDD` for `feature`/`ops` or when `impact_flags` are present, but allow `FTD` for bounded single-module `bug`/`audit_fix`/`refactor` requests while attaching `risk_level: "ALTO"` and `risk_hits` for downstream enforcement.

### Alternative F: Persisting Derived Markdown Views (`WI-xxx.md`) Alongside JSON by Default

*   **Pros**: Allows operators browsing `1_Intake/runtime/workitems/` or `quarantine/` in a file manager to open a `.md` file directly without running `intake.exe show`.
*   **Cons**: Persisting two files per `WorkItem` across `quarantine/` and `workitems/` doubles file I/O during atomic promotion and creates split-brain risk if an operator manually edits `WI-xxx.md` expecting `intake.exe resolve` or `2_Spec_Writer` to read those edits.
*   **Reason for Rejection**: Rejected by author: `WI-<hash12>.json` is the single persisted source of truth on disk; human-readable Markdown views are rendered on demand via `intake.exe show --id <WI-ID> --format md` (or exported explicitly when `--emit-md` is passed).

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Security & Privacy (`G3`, `G4`, Option i):**
    *   *Zero-Secret Persistence:* Tier 1 `G4` failures never create files in `workitems/` or `quarantine/`. The forensic event written to `intake_events.jsonl` and `stderr` records only `{timestamp, trace_id, event: "REJECTED_G4", rule_name, field, line, raw_sha256, source, source_ref}` and never echoes the matched substring or raw line text.
    *   *Subprocess Argument Hygiene:* `BrainClient` invokes `0_Brain/brain.exe` only after Stage 1 (Tier 1 raw scan) and Stage 2 (Tier 2 redaction and keyword extraction) succeed, ensuring raw secrets or personal paths never appear in Windows process creation command lines.
    *   *Reparse-Point and Path-Traversal Protection:* All file reads (`--input`, `0_Brain/vault/index.md`, `0_Brain/vault/wiki/risk_zones.md`) and runtime directory operations reject NTFS symlinks, junctions, and mount points (`FILE_ATTRIBUTE_REPARSE_POINT = 0x00000400`, matching `0_Brain/engine/reparse_windows.go`), and reject any `affected_paths` entry containing `..`.
    *   *Option i Git Isolation:* `.gitignore` excludes `1_Intake/runtime/` and `1_Intake/intake.exe`. `intake.exe verify` executes `git check-ignore` against the active runtime path when inside a Git repository and fails closed if untracked privacy is broken.
*   **Performance & Latency (`< 500 ms` p95 on Windows):**
    *   *Subprocess Budget:* `intake.exe ingest` spawns at most 2 `0_Brain/brain.exe` subprocesses (`context --json` and `query --json`, ~20–45 ms each on a 60-document SQLite FTS5 index), avoiding an unconditional 3rd `brain.exe verify` spawn by checking `.index/brain.db` and `.brain-journal.json` directly via `os.Stat` in-process (< 1 ms).
    *   *O(1) Deduplication Lookup:* Because ready and standard quarantined items are named deterministically as `WI-<dedupe_key[:12]>.json`, idempotency lookup checks `os.Stat` on `workitems/WI-<hash12>.json` and `quarantine/WI-<hash12>.json` in $O(1)$ time without scanning the directory.
*   **Concurrency, Rollout & Rollback:**
    *   *Kernel Locking & Atomic Promotion:* All writes to `1_Intake/runtime/` acquire an exclusive non-blocking OS lock on `1_Intake/runtime/.intake.lock` (`LockFileEx` with `LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY` on Windows; `flock(LOCK_EX|LOCK_NB)` on POSIX) and commit JSON files via same-directory temporary file write + `os.Rename` retry loop.
    *   *Schema Versioning:* Every `WorkItem` carries `"schema_version": "1.0"`, `"triage_reasons[].ruleset_version": "triage-det-v1.0"` / `"lexicon-v1.0"`. Additive minor fields preserve backward compatibility; unrecognized major `schema_version` values fail safely in `intake.exe verify` and `resolve` without corrupting existing files.

--------------------------------------------------------------------------------

### Additional Resources

*   [`README.md`](../../README.md) — Black Factory 7-team blueprint and global invariants `G1–G6`.
*   [`1_Intake/README.md`](../../1_Intake/README.md) — Initial scope definition for Team 1.
*   [`0_Brain/README.md`](../../0_Brain/README.md), [`0_Brain/engine/README.md`](../../0_Brain/engine/README.md), and [`0_Brain/engine/cmd/brain/README.md`](../../0_Brain/engine/cmd/brain/README.md) — `0_Brain` engine architecture, `g4Rules`, `MatchCoverPath`, and `brain.exe` CLI contract.
*   [`Docs/BLACK_FACTORY_BRAIN.md`](../../Docs/BLACK_FACTORY_BRAIN.md) — Option 3C hybrid architecture and Option i privacy model.
*   [`GengiSWAP/README.md`](../../GengiSWAP/README.md) and [`GengiSWAP/engine/README.md`](../../GengiSWAP/engine/README.md) — Downstream deterministic execution ring and canonical task templates.

### Future Improvements

*   **Phase 2 MCP Transport (`BrainClient` over `brain.exe mcp`):** Replace per-command `os/exec` subprocess spawns in `BrainClient` with a persistent stdio/JSON-RPC client once `0_Brain` implements the reserved `brain.exe mcp` subcommand (`0_Brain/engine/cmd/brain/main.go:114-116`).
*   **Phase 2 External Ingestion Adapters (`IngestSource` Implementations):** Add structured adapters for GitHub Issues JSON payloads, Sentry event webhooks, and support form exports implementing the `IngestSource` interface without altering Stages 2–5 of the triage engine.
*   **First-Class Structured Risk-Zone Emission in `0_Brain`:** Extend `0_Brain` so `brain.exe context --json` natively indexes and emits structured `RZ-*` rows from `wiki/risk_zones.md`, allowing `1_Intake` to retire its local Markdown table parser in a future minor release.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

### Target Files to Create/Modify

#### 1. `[.gitignore](../../.gitignore)` (Modify)

*   **Type / Scope**: Git Version Control Configuration (Option i Privacy)
*   **Proposed Changes**: Append `1_Intake/runtime/` and `1_Intake/intake.exe` to the Black Factory private/local artifact exclusion block (adjacent to `0_Brain/vault/` and `0_Brain/brain.exe` at lines 22–27).
*   **Rationale**: Guarantees that operational `WorkItem` files, quarantine holds, `.intake.lock`, audit logs (`intake_log.md`, `intake_events.jsonl`), and the compiled `intake.exe` binary are never tracked or committed to Git.

#### 2. `[1_Intake/README.md](../../1_Intake/README.md)` (Modify)

*   **Type / Scope**: Team 1 Module Architecture & Operations Documentation
*   **Proposed Changes**: Replace the 5-line skeleton with the full module specification following the repository standard in [`0_Brain/README.md`](../../0_Brain/README.md): Overview, Subdirectories & System Boundaries (`engine`, `schemas`, `skills`, untracked `runtime`), Invariants (`G1–G6`, two-tier `G4`, `.intake.lock`, Option i), Side Effects, ASCII/Unicode Box-Drawing architecture diagram, Verification commands, and verified PowerShell CLI usage examples (`ingest`, `resolve`, `list`, `show`, `verify`).
*   **Rationale**: Establishes the authoritative entry point for humans and agents interacting with Team 1.

#### 3. `[1_Intake/schemas/workitem.schema.json](../../1_Intake/schemas/workitem.schema.json)` (Create)

*   **Type / Scope**: JSON Schema Contract (`schema_version: "1.0"`)
*   **Proposed Changes**: Define the strict JSON Schema (Draft 2020-12) for `WorkItem` v1.0, specifying all required top-level properties, closed enumerations (`status`: `ready` | `needs_clarification`; `kind`: `bug` | `feature` | `refactor` | `audit_fix` | `ops`; `lane`: `ASDD` | `FTD` | `""`; `paths_confidence`: `explicit` | `inferred_from_query` | `none`; `impact_flags`: `contract` | `schema` | `db` | `auth` | `security` | `governance`; `risk_level`: `NONE` | `ALTO` | `CRITICO`; `suggested_template`: `modulo_nuevo` | `correccion_hallazgos` | `auditoria_e2e` | `mapeo_estatico`), and nested schemas for `risk_hits`, `triage_reasons`, `sanitization`, `brain_evidence`, and `quarantine_info`.
*   **Rationale**: Provides a versioned, language-agnostic contract consumed by `1_Intake` validation and downstream `2_Spec_Writer`, `4_Revisores`, and `6_Filtro_de_Riesgo`.

#### 4. `[1_Intake/skills/README.md](../../1_Intake/skills/README.md)` (Create)

*   **Type / Scope**: Agent Skill Catalog Documentation
*   **Proposed Changes**: Document the progressive-load skill catalog for Team 1 (`intake-triage`), mapping when `1_Intake` and `2_Spec_Writer` agents must load [`./intake-triage/SKILL.md`](./intake-triage/SKILL.md).
*   **Rationale**: Aligns Team 1 with the `rsc-harness` progressive-load skill architecture established in [`0_Brain/skills/README.md`](../../0_Brain/skills/README.md).

#### 5. `[1_Intake/skills/intake-triage/SKILL.md](../../1_Intake/skills/intake-triage/SKILL.md)` (Create)

*   **Type / Scope**: Modular Agent Skill Definition
*   **Proposed Changes**: Define YAML frontmatter (`name: intake-triage`, `description`, `version: "1.0"`, `owner: 1_Intake`) and step-by-step operational instructions for invoking `1_Intake/intake.exe` (`ingest`, `resolve`, `list`, `show`, `verify`), interpreting exit codes `0..5`, and complying with `G1–G6` and Option i privacy.
*   **Rationale**: Enables agents to execute deterministic intake and quarantine resolution without guessing flags or bypassing `0_Brain`.

#### 6. `[1_Intake/engine/go.mod](../../1_Intake/engine/go.mod)` (Create)

*   **Type / Scope**: Go Module Definition
*   **Proposed Changes**: Declare `module blackfactory/intake` targeting `go 1.26.0` (matching [`0_Brain/engine/go.mod`](../../0_Brain/engine/go.mod)) with zero external `require` directives.
*   **Rationale**: Enforces a pure-Go standard-library build (`CGO_ENABLED=0`) without pulling SQLite or third-party YAML parsers into `intake.exe`.

#### 7. `[1_Intake/engine/README.md](../../1_Intake/engine/README.md)` (Create)

*   **Type / Scope**: Go Package Technical Documentation
*   **Proposed Changes**: Document package `intake` overview, exported `[file.go:Symbol]` interface index, package invariants, filesystem/process side effects, and `go test -count=1 ./...` verification command, matching the layout of [`0_Brain/engine/README.md`](../../0_Brain/engine/README.md) and [`GengiSWAP/engine/README.md`](../../GengiSWAP/engine/README.md).
*   **Rationale**: Maintains structural documentation symmetry across all Black Factory Go engines.

#### 8. `[1_Intake/engine/model.go](../../1_Intake/engine/model.go)` (Create)

*   **Type / Scope**: Go Domain Model & Schema Validator (`package intake`)
*   **Proposed Changes**: Define constants and data structures: `SchemaVersion = "1.0"`, `TriageDetRulesetVersion = "triage-det-v1.0"`, `LexiconRulesetVersion = "lexicon-v1.0"`, `WorkItem`, `RawIntake`, `SanitizationReport`, `G4Redaction`, `BrainEvidence`, `BrainInvocation`, `BrainDocMatch`, `RiskHit`, `TriageReason`, `QuarantineInfo`, `IntakeEvent`, and `VerifyReport`. Implement `ComputeDedupeKey(project, source, sourceRef string) string` (length-prefixed SHA256 over lowercased trimmed values), `FormatWorkItemID(dedupeKey, payloadSHA256 string) string` (`WI-<dedupeKey[:12]>` or `WIQ-<payloadSHA256[:12]>`), `ComputeSanitizedSHA256(wi *WorkItem) string`, and `ValidateWorkItem(wi *WorkItem) []string`.
*   **Rationale**: Centralizes canonical data types, deterministic hashing, and schema validation in a single file with zero I/O dependencies.

#### 9. `[1_Intake/engine/input.go](../../1_Intake/engine/input.go)` (Create)

*   **Type / Scope**: Input Normalization & `IngestSource` Port (`package intake`)
*   **Proposed Changes**: Define the `IngestSource` interface (`ReadRaw() ([]byte, string, error)` and `Parse(raw []byte) (RawIntake, error)`) and implement v1 readers:
    *   JSON object parser (`ParseJSONIntake`) enforcing known fields and decoding string or array formats.
    *   Markdown + YAML frontmatter parser (`ParseMarkdownIntake`, reusing the flat YAML key/list parsing conventions of [`0_Brain/engine/frontmatter.go`](../../0_Brain/engine/frontmatter.go)) that populates `description` from the Markdown body when `description` is omitted in frontmatter.
    *   `MergeCLIOverrides(base RawIntake, overrides RawIntake) RawIntake` so explicit CLI flags override fields loaded from `--input <file>` or `--input -` (`stdin`), while enforcing a maximum raw input size quota (`MaxInputBytes = 1 << 20`, 1 MiB) and rejecting symlinks/reparse points on `--input` files.
*   **Rationale**: Isolates v1 CLI/file/stdin parsing behind a clean port ready for v2 adapters (GitHub Issues, Sentry) and prevents Windows `stdin` hangs by requiring explicit `--input -`.

#### 10. `[1_Intake/engine/g4.go](../../1_Intake/engine/g4.go)` (Create)

*   **Type / Scope**: Two-Tier G4 Perimeter Gate & Redactor (`package intake`)
*   **Proposed Changes**: Define `G4Tier` (`Tier1HardSecret`, `Tier2Environmental`), `G4Finding` (`Rule`, `Tier`, `Field`, `Line`), and the 11 compiled regex rules matching [`0_Brain/engine/validator.go:49-105`](../../0_Brain/engine/validator.go#L49-L105). Implement:
    *   `ScanTier1Raw(raw []byte, strictG4 bool) []G4Finding`: scans line-by-line for the 8 Tier 1 rules (plus the 3 Tier 2 rules if `strictG4 == true`), suppressing `literal_private_key` on any line where `pem_private_key` already matched (`validator.go:159-167`), and never storing the matched secret text.
    *   `RedactTier2Text(field, text string) (string, []G4Redaction)`: runs on parsed free-text fields (`title`, `description`, `note`), replacing `win_user_path` and `email_address` matches with `[REDACTED:G4:<rule>]` and replacing only the `(?:/home|/Users)/[A-Za-z0-9._-]+` path core of `unix_user_path` while preserving leading (`[\s"'(=:]`) and trailing (`(?:/|$|\s|["'])`) delimiters.
    *   `ScanG4All(text string) []G4Finding`: runs all 11 rules in fail-closed mode for `intake.exe verify` audits on persisted runtime files.
*   **Rationale**: Enforces the two-tier G4 contract without corrupting JSON/YAML syntax delimiters or persisting secrets to disk.

#### 11. `[1_Intake/engine/paths.go](../../1_Intake/engine/paths.go)` (Create)

*   **Type / Scope**: Path Normalization, G4 Path Guard & Module Counting (`package intake`)
*   **Proposed Changes**: Implement `SanitizeAffectedPaths(rawPaths []string, repoRoot string) (cleanPaths []string, invalidReason string)`:
    *   Checks each raw path against Tier 2 personal path regexes (`win_user_path`, `unix_user_path`), Windows drive-letter prefixes (`[A-Za-z]:`), UNC prefixes (`\\`), and `..` segments. If an absolute path starts with `repoRoot`, strips `repoRoot` only if the stripped prefix did not match `win_user_path`/`unix_user_path`; if any personal path, external absolute path, or `..` traversal is present, discards the raw path value in memory and returns `invalidReason = "G4_personal_path_in_affected_paths"` (or `"PATH_TRAVERSAL_REJECTED"`).
    *   Normalizes valid relative paths (`\` -> `/`, `path.Clean`, trims leading `./` and `/`, deduplicates while preserving order).
    *   Implement `CountRootModules(paths []string) []string` to extract distinct top-level directory segments (e.g., `0_Brain`, `GengiSWAP`, `1_Intake`) for the multi-module threshold rule.
*   **Rationale**: Prevents `[REDACTED:G4:...]` markers from polluting `affected_paths` and ensures personal paths in `affected_paths` trigger clean quarantine holds (`Exit 2`) with zero personal data written to disk.

#### 12. `[1_Intake/engine/brain_client.go](../../1_Intake/engine/brain_client.go)` (Create)

*   **Type / Scope**: `0_Brain` Subprocess Client & FTS5 Keyword Extractor (`package intake`)
*   **Proposed Changes**: Define `BrainClient` interface and `SubprocessBrainClient` (with configurable subprocess timeout, defaulting to `5s`):
    *   `Preflight(vaultRoot, dbPath string)`: checks `os.Stat` on `dbPath` (`.index/brain.db`) and `<vaultRoot>/.brain-journal.json`. If `.brain-journal.json` exists, runs `brain.exe verify --vault <vaultRoot> --json`. Maps missing `brain.db`, `verify` exit codes `3`/`4`, or `ok: false` to `ErrBrainUnavailable` (exit code `4`).
    *   `IsProjectRegistered(vaultRoot, dbPath, project string)`: runs `brain.exe query --vault <vaultRoot> --db <dbPath> --type project --project <project> --limit 1 --json`; if empty, reads `<vaultRoot>/index.md` (verifying no reparse point) and parses the `## Proyectos Registrados` Markdown table for a case-insensitive match on column 1 (`Proyecto`).
    *   `ExtractQueryTokens(title, description string, maxTokens int) []string`: extracts technical tokens (paths, `*.go`/`*.md` filenames, `RZ-\d+`, `ADR-\d+`, `CamelCase`/`snake_case` symbols, words `>= 4` chars), strips Spanish/English stop-words, and strips FTS5 reserved operators (`AND`, `OR`, `NOT`, `NEAR`) and punctuation.
    *   `Query` and `Context`: invoke `brain.exe query --json` and `brain.exe context --json`, capture literal command strings, exit codes, and `duration_ms` (`G1`), and unmarshal JSON output handling both `null` and `[]` slices cleanly.
*   **Rationale**: Satisfies mandatory Read-Before-Act consultation with `0_Brain` within `< 500 ms` p95 while adapting to `0_Brain`'s actual exit codes, `null` slice serialization, FTS5 `AND` semantics, and current `index.md` project table.

#### 13. `[1_Intake/engine/risk_zones.go](../../1_Intake/engine/risk_zones.go)` (Create)

*   **Type / Scope**: Glob Matcher & `wiki/risk_zones.md` Catalog Parser (`package intake`)
*   **Proposed Changes**: Implement:
    *   `NormalizeCoverPath(p string) string` and `MatchCoverPath(pattern, filePath string) bool` using the exact segment dynamic-programming glob algorithm from [`0_Brain/engine/index.go:32-93`](../../0_Brain/engine/index.go#L32-L93) with case-insensitive path matching for Windows compatibility.
    *   `ParseRiskZonesMarkdown(content string) ([]RiskZoneRow, error)`: locates heading `## Catálogo de Zonas de Riesgo`, validates the 6-column Markdown table header (`ID`, `Proyecto`, `Rutas cubiertas`, `Nivel`, `Compuerta obligatoria`, `Evidencia`), strips inline backticks from cells, splits comma-separated path patterns, and validates `Nivel` (`CRITICO`, `ALTO`, `MEDIO`, `BAJO`).
    *   `EvaluateRiskZones(project string, paths []string, contextDocs []BrainDocMatch, rows []RiskZoneRow, parseErr error) ([]RiskHit, RiskLevel, bool, *TriageReason)`: matches `paths` against both `contextDocs` (`type: "risk_zones"` `matched_paths`) and `rows` (where `strings.EqualFold(row.Project, project)` or `row.Project == "*"`), returning highest severity (`CRITICO` > `ALTO` > `NONE`), `requiresAutorHITL`, and fail-safe reason `RISK_ZONES_UNPARSEABLE_FAILSAFE` if `parseErr != nil` or `len(rows) == 0`.
*   **Rationale**: Eliminates the silent `matched_paths: []` false negative when `0_Brain/vault/wiki/risk_zones.md` has `covers_paths: []` in frontmatter.

#### 14. `[1_Intake/engine/triage.go](../../1_Intake/engine/triage.go)` (Create)

*   **Type / Scope**: Deterministic & Heuristic Lane Triage Engine (`package intake`)
*   **Proposed Changes**: Implement:
    *   `EvaluateLocatability(raw RawIntake, invalidPathReason string, projectRegistered bool, queryMatches []BrainDocMatch) *QuarantineInfo`: evaluates Priority Rules 3–6 (`MISSING_REQUIRED_FIELDS`, `UNREGISTERED_PROJECT`, `INVALID_AFFECTED_PATH_G4`, `UNCERTAIN_SCOPE`) and builds the actionable `resolve_hint` CLI command string.
    *   `ScanHeuristicLexicon(title, description string) []string`: evaluates word-boundary regular expressions from the versioned bilingual catalog (`lexicon-v1.0`: contract, schema/esquema, migration/migración, DDL, auth/autenticación, password/contraseña, token, JWT, Ed25519, CSRF, SQLite, lock/cerrojo, constitution/constitución, risk_zones).
    *   `TriageWorkItem(wi *WorkItem, brainDocs []BrainDocMatch, riskHits []RiskHit, riskLevel RiskLevel, riskFailSafe *TriageReason, maxFTDFiles int, clearHeuristics bool)`: evaluates Priority Rules 7–15 in deterministic order, sets `lane` (`ASDD` | `FTD`), `risk_level`, `requires_autor_hitl`, `suggested_template`, and appends all matching `TriageReason` items (`rule_kind: "deterministic"` or `"heuristic"`).
*   **Rationale**: Enforces the two-stage Locatability -> Lane Triage separation and guarantees that every `ASDD`, `FTD`, or `quarantine/` verdict is 100% explainable via `triage_reasons[]`.

#### 15. `[1_Intake/engine/store.go](../../1_Intake/engine/store.go)` (Create)

*   **Type / Scope**: Atomic Option i Runtime Store & Telemetry Logger (`package intake`)
*   **Proposed Changes**: Implement `Store` managing `<dataDir>/workitems/`, `<dataDir>/quarantine/`, `<dataDir>/intake_log.md`, `<dataDir>/intake_events.jsonl`, and `<dataDir>/.intake.lock`:
    *   `SaveTriaged` / `SaveQuarantined`: acquires `.intake.lock`, performs $O(1)$ lookup for existing `WI-<hash12>.json` in `workitems/` and `quarantine/`, enforces idempotency (`idempotent_hit: true` exit `0` when `sanitized_payload_sha256` matches; `ErrDedupeConflict` exit `5` when already `ready` in `workitems/` with different hash and `update == false`; atomic promotion from `quarantine/` to `workitems/` with `revision++` and deletion of the `quarantine/` file after `replaceFileAtomic` succeeds).
    *   `AppendRejectedG4Event`: appends a payload-free `REJECTED_G4` JSON line to `intake_events.jsonl` under `.intake.lock` without creating any file in `workitems/` or `quarantine/`.
    *   `List`, `LoadByID`, and `VerifyRuntime`: implements filtered listing, ID lookup across `workitems/` and `quarantine/`, and the 5-point `intake.exe verify` integrity check (including `git check-ignore` validation for Option i privacy).
*   **Rationale**: Guarantees crash-safe atomic transitions on Windows, $O(1)$ deduplication, and dual human/machine auditability (`intake_log.md` + `intake_events.jsonl`).

#### 16. `[1_Intake/engine/render.go](../../1_Intake/engine/render.go)` (Create)

*   **Type / Scope**: On-Demand ASCII/Box-Drawing Markdown Renderer (`package intake`)
*   **Proposed Changes**: Implement `RenderWorkItemMarkdown(wi *WorkItem) string` producing a deterministic Markdown report beginning with `<!-- AUTO-GENERATED DERIVED VIEW: DO NOT EDIT (Source: <id>.json) -->`, an ASCII/Unicode Box-Drawing triage header box, structured tables for `triage_reasons`, `risk_hits`, `sanitization.g4_redactions`, and `brain_evidence.invocations` (`G1`), and actionable `quarantine_info` guidance when `status == "needs_clarification"`.
*   **Rationale**: Delivers human-readable views for `intake.exe show --format md` and `--emit-md` without violating the no-Mermaid constraint or introducing dual-file source-of-truth drift.

#### 17. `[1_Intake/engine/lock_windows.go](../../1_Intake/engine/lock_windows.go)` and `[1_Intake/engine/lock_other.go](../../1_Intake/engine/lock_other.go)` (Create)

*   **Type / Scope**: OS Kernel File Locking (`package intake`)
*   **Proposed Changes**: Implement `AcquireIntakeLock(dataDir string) (*FileLock, error)` and `(*FileLock).Unlock() error` on `<dataDir>/.intake.lock` using Windows `syscall.CreateFile` + `LockFileEx` (`LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY`) in `lock_windows.go` and POSIX `syscall.Flock(LOCK_EX | LOCK_NB)` in `lock_other.go`, returning `ErrIntakeLocked` immediately on contention and releasing the handle without deleting `.intake.lock` (matching [`0_Brain/engine/lock_windows.go`](../../0_Brain/engine/lock_windows.go)).
*   **Rationale**: Prevents race conditions during concurrent `ingest`, `resolve`, and `verify` calls on Windows and Unix.

#### 18. `[1_Intake/engine/reparse_windows.go](../../1_Intake/engine/reparse_windows.go)` and `[1_Intake/engine/reparse_other.go](../../1_Intake/engine/reparse_other.go)` (Create)

*   **Type / Scope**: NTFS Reparse-Point and Symlink Guard (`package intake`)
*   **Proposed Changes**: Implement `IsReparsePointOrSymlink(path string, info os.FileInfo) (bool, error)` checking `os.ModeSymlink | os.ModeIrregular` and Windows `FILE_ATTRIBUTE_REPARSE_POINT` (`0x00000400`) via `syscall.Win32FileAttributeData` (matching [`0_Brain/engine/reparse_windows.go`](../../0_Brain/engine/reparse_windows.go)).
*   **Rationale**: Prevents symlink or NTFS junction traversal attacks when reading `--input` files or accessing `1_Intake/runtime/` and `0_Brain/vault/`.

#### 19. `[1_Intake/engine/parity_test.go](../../1_Intake/engine/parity_test.go)` (Create)

*   **Type / Scope**: Cross-Module Automated Parity Test (`package intake`)
*   **Proposed Changes**: Implement `TestG4RulesParityWith0Brain` and `TestMatchCoverPathParityWith0Brain`:
    *   Parses [`../../0_Brain/engine/validator.go`](../../0_Brain/engine/validator.go) using `go/parser` and `go/ast` to extract every element of `var g4Rules` (`name` and raw regex literal) and asserts exact 1-to-1 equality with `1_Intake`'s 11 G4 rules (8 Tier 1 + 3 Tier 2).
    *   Verifies that every Tier 2 redacted output string (`[REDACTED:G4:win_user_path]`, `[REDACTED:G4:unix_user_path]`, `[REDACTED:G4:email_address]`) produces zero matches against all 11 regexes from `0_Brain/engine/validator.go`.
    *   Executes shared glob test vectors (`*`, `**`, `?`, prefix directories, Windows backslashes) against `MatchCoverPath` and compares against [`../../0_Brain/engine/index.go`](../../0_Brain/engine/index.go).
*   **Rationale**: Mathematically eliminates silent regex or glob divergence between Team 0 (`0_Brain`) and Team 1 (`1_Intake`) without linking `modernc.org/sqlite` into `intake.exe`.

#### 20. `[1_Intake/engine/intake_test.go](../../1_Intake/engine/intake_test.go)` (Create)

*   **Type / Scope**: Unit, Table-Driven & Benchmark Test Suite (`package intake`)
*   **Proposed Changes**: Implement table-driven unit tests covering:
    *   All 8 Tier 1 fail-closed G4 rules (`pem_private_key` line suppression over `literal_private_key`, 0 files created in `workitems/` or `quarantine/`, payload-free `REJECTED_G4` event in `intake_events.jsonl`).
    *   All 3 Tier 2 redaction rules on parsed `title`/`description`/`note` (verifying delimiter preservation on `unix_user_path` inside JSON/YAML strings and `--strict-g4` fail-closed promotion).
    *   Personal path and `..` rejection in `affected_paths` (`G4_personal_path_in_affected_paths` routing to `quarantine/` with zero raw path persistence).
    *   FTS5 technical keyword extraction (`ExtractQueryTokens`) and stripping of `AND`, `OR`, `NOT`, `NEAR` operators and bilingual stop-words.
    *   `wiki/risk_zones.md` 6-column table parsing (`RZ-001` `CRITICO` -> `ASDD` + `requires_autor_hitl: true`; `RZ-002` `ALTO` -> `FTD` allowed for bounded single-module `bug` with `risk_level: "ALTO"`, `ASDD` forced for `feature` or `impact_flags`; malformed table fail-safe to `ASDD`).
    *   Two-stage Locatability (`UNCERTAIN_SCOPE` -> `quarantine/` exit `2`) vs. Lane Triage (`NO_BRAIN_COVERAGE` / `INFERRED_PATHS_FAILSAFE` -> `ASDD` exit `0`).
    *   Idempotency (`WI-<hash12>` vs `WIQ-<hash12>`), conflict detection (`ErrDedupeConflict`), atomic quarantine promotion (`revision: 2`), and Windows latency benchmark (`< 500 ms` p95 over 60 vault docs in `t.TempDir()`).
*   **Rationale**: Provides deterministic TDD verification of every functional and non-functional requirement on isolated temporary directories (`G3`).

#### 21. `[1_Intake/engine/cmd/intake/README.md](../../1_Intake/engine/cmd/intake/README.md)` (Create)

*   **Type / Scope**: CLI Package Documentation (`package main`)
*   **Proposed Changes**: Document the `intake.exe` CLI entry point, `[main.go:Symbol]` interface index, subcommands (`ingest`, `resolve`, `list`, `show`, `verify`), flag contracts, exit code mapping (`0..5`), invariants, side effects, and verification command (`go test ./...`), matching [`0_Brain/engine/cmd/brain/README.md`](../../0_Brain/engine/cmd/brain/README.md).
*   **Rationale**: Keeps CLI contract documentation adjacent to `cmd/intake/main.go`.

#### 22. `[1_Intake/engine/cmd/intake/main.go](../../1_Intake/engine/cmd/intake/main.go)` (Create)

*   **Type / Scope**: CLI Executable Entry Point (`package main` -> `1_Intake/intake.exe`)
*   **Proposed Changes**: Implement `main()` and `run(args []string, in io.Reader, out, errOut io.Writer, client intake.BrainClient) int` dispatching `ingest`, `resolve`, `list`, `show`, and `verify`:
    *   Implement `reorderFlags` so boolean and value flags work in any position before or after positional arguments.
    *   Implement deterministic workspace root discovery (`--vault`, `BRAIN_VAULT`, `--data-dir`, `BF_INTAKE_DIR`, `--brain-bin`, `BRAIN_BIN`, resolving defaults relative to the repository root or current working directory).
    *   Enforce the exact exit code table: `0` (ready / idempotent hit / command OK), `1` (CLI usage, format, or I/O error), `2` (quarantined in `quarantine/` under `G6`, still emitting the JSON envelope with `id`, `quarantine_path`, and `resolve_hint`), `3` (G4 fail-closed rejection, 0 payload files created), `4` (`0_Brain` unavailable, locked, or corrupt journal), and `5` (idempotency conflict without `--update`).
*   **Rationale**: Provides the deterministic command-line interface consumed by operators, `2_Spec_Writer`, and automated scripts.

#### 23. `[1_Intake/engine/cmd/intake/main_test.go](../../1_Intake/engine/cmd/intake/main_test.go)` (Create)

*   **Type / Scope**: End-to-End CLI Integration Tests (`package main`)
*   **Proposed Changes**: Implement E2E tests (`TestCLI_E2E_FullLifecycle_AllExitCodes`, `TestCLI_VerifyDetectsG4AndQuarantinePromotion`) executing `run(...)` across all 5 subcommands and asserting all 6 exit codes (`0`, `1`, `2`, `3`, `4`, `5`), JSON stdout/stderr envelopes, on-demand Markdown rendering (`show --format md`), and round-trip compatibility with `0_Brain` vault structures in `t.TempDir()`.
*   **Rationale**: Verifies the full CLI contract end-to-end without mutating production directories (`G3`).

#### 24. `[README.md](../../README.md)` (Modify)

*   **Type / Scope**: Root Repository Blueprint Documentation
*   **Proposed Changes**: Update `System Verification` (lines 39–51) to add the `1_Intake Engine` test command (`Set-Location "C:\Black Factory\1_Intake\engine"; go test -count=1 ./...`), update `Artefactos privados / locales (ignorados en .gitignore)` (lines 58–61) to include `1_Intake/runtime/` and `1_Intake/intake.exe`, and add a quick-start compilation and CLI verification subsection for `1_Intake` (`intake.exe`).
*   **Rationale**: Keeps the root blueprint synchronized with Team 1's executable and Option i runtime artifacts.

### Verification Plan

*   **Compilation/Build**:
    ```powershell
    Set-Location "C:\Black Factory\1_Intake\engine"
    $env:CGO_ENABLED = "0"
    go build -o ..\intake.exe .\cmd\intake
    ```
*   **Static Analysis & Linting**:
    ```powershell
    Set-Location "C:\Black Factory\1_Intake\engine"
    $env:CGO_ENABLED = "0"
    go vet ./...
    ```
*   **Unit, Cross-Module Parity & E2E Tests**:
    ```powershell
    Set-Location "C:\Black Factory\1_Intake\engine"
    $env:CGO_ENABLED = "0"
    go test -v -count=1 ./...
    ```
*   **Test Coverage & Quantitative Success Criteria**:
    *   `85%+` statement coverage across `1_Intake/engine/...` (verified via `go test -cover ./...`).
    *   `0%` false negatives across all 8 Tier 1 G4 rules (`exit 3`, `0` files in `workitems/` or `quarantine/`, payload-free `REJECTED_G4` entry in `intake_events.jsonl`) and `100%` AST regex parity with `0_Brain/engine/validator.go` (`TestG4RulesParityWith0Brain`).
    *   `0%` false negatives on `RZ-001` (`CRITICO` -> `ASDD` + `requires_autor_hitl: true`) and `RZ-002` (`ALTO` -> `FTD` for bounded single-module bug, `ASDD` for feature/flags) even when `0_Brain/vault/wiki/risk_zones.md` has `covers_paths: []` in frontmatter.
    *   `< 500 ms` p95 local ingestion and triage latency on Windows against a 60-document `0_Brain` vault.
*   **E2E CLI & Option i Privacy Verification**:
    ```powershell
    & "C:\Black Factory\1_Intake\intake.exe" verify --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\1_Intake\runtime" --json
    git status --ignored -s 1_Intake/
    ```
    Must confirm `ok: true` (exit code `0`) and `!! 1_Intake/intake.exe` / `!! 1_Intake/runtime/` ignored in Git.

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
