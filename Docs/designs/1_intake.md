# Black Factory Team 1 (`1_Intake`): Deterministic Request Normalization, G4 Perimeter Gate, and Methodological Lane Triage

*   **Author:** blackpusch
*   **Status:** Approved
*   **Date:** 2026-11-22

--------------------------------------------------------------------------------

## Overview

`1_Intake` (`1_Intake/engine` -> `1_Intake/intake.exe`) is the deterministic perimeter gate and methodological triage engine for Black Factory. Compiled as a standalone pure-Go CLI executable (`CGO_ENABLED=0`, Windows primary target with POSIX fallback, sharing the SQLite-free core package `pkg/bfcore` and permitting only `golang.org/x/sys`), it ingests software engineering requests and defect reports from command-line flags or local structured JSON payloads (`--input <file.json>` or `--input -` from `stdin`, up to `MaxInputBytes = 1 MiB` after UTF-8 BOM stripping). It enforces a two-tier secret and personal-data sanitization boundary (`G4`), consults the persistent knowledge engine (`0_Brain/brain.exe`) under the mandatory Read-Before-Act protocol, and emits a versioned JSON `WorkItem` (`schema_version: "1.0"`).

Before describing the routing outcomes, the specification relies on the following repository baseline definitions (see [`README.md`](../../README.md) and [`Docs/BLACK_FACTORY_BRAIN.md`](../../Docs/BLACK_FACTORY_BRAIN.md)):
*   **Global Factory Invariants (`G1–G6`):** `G1` (Literal Evidence: verifiable commands, exit codes, and outputs), `G2` (Traceability: decision and provenance records using ASCII/Unicode Box-Drawing diagrams), `G3` (Data Integrity: tests run in isolated `t.TempDir()` directories without mutating production data), `G4` (Zero Leaks: no secrets, tokens, or personal host paths persisted), `G5` (Technical Rigor: reproducible verification commands), and `G6` (Stop Principle: fail-safe halt when scope or preconditions are uncertain).
*   **Architectural Conventions (`Option 3C`, `Option i`, `OKF`, `[AUTOR]`, `HITL`):** `Option 3C` combines Open Knowledge Format (`OKF`, Markdown documents with strict YAML frontmatter) with a compiled Go CLI engine and progressive-load agent skills; `Option i` keeps all operational runtime directories (`0_Brain/vault/`, `1_Intake/runtime/`) and compiled `.exe` binaries excluded from Git via `.gitignore`; `[AUTOR]` (`blackpusch`) is the human governance authority required for Human-in-the-Loop (`HITL`, `requires_autor_hitl: true`) approvals on critical risk zones.

Each ingested request is deterministically routed into one of three operational outcomes under gitignored local storage (`1_Intake/runtime/`, Option i privacy):
1. **`ASDD` (Agentic / Spec-Driven Development lane, `status: ready`, exit code `0`):** for new features (`feature`), operations changes (`ops`), multi-module or `> 5`-file changes, requests with explicit architectural impact flags (`contract`, `schema`, `db`, `auth`, `security`, `governance`), requests touching `CRITICO` risk zones (`RZ-001`), or requests affecting code paths without active `adr`/`wiki` coverage in `0_Brain`.
2. **`FTD` (Fast-Track Development lane, `status: ready`, exit code `0`):** exclusively for bounded, single-module `bug`, `audit_fix`, or `refactor` requests with explicit concrete file paths in `affected_paths` (`1..5` files, zero directories or globs), active `adr`/`wiki` documentation in `0_Brain` (`status: "active"`), zero `impact_flags`, and no `CRITICO` risk-zone hits (`NONE`, `BAJO`, `MEDIO`, or `ALTO`).
3. **`Quarantine` (`status: needs_clarification`, exit code `2`):** a fail-safe `G6` hold when required metadata fields are missing, the target `project` is not registered as `active` in `0_Brain`, a personal/out-of-repository path, directory, or glob is supplied inside `affected_paths`, or `affected_paths` are omitted.

## The Problem

Black Factory separates software delivery into seven specialized teams (`0_Brain` through `6_Filtro_de_Riesgo`) backed by the deterministic execution ring `GengiSWAP` (`gengis.exe`). While `0_Brain` (`0_Brain/brain.exe`) already provides persistent Open Knowledge Format (`OKF`) memory, SQLite FTS5 indexing, `G4` secret validation, and Ed25519 governance, `1_Intake` currently exists only as a 5-line skeleton (`1_Intake/README.md`). Without a deterministic `1_Intake` engine and a clean `0_Brain` contract, the factory faces five operational and integration gaps:

1. **Unvalidated Perimeter and Secret/PII Contamination (`G4` vs. `G1` Conflict):**
   Raw bug reports, crash dumps, and operator tickets routinely contain either live credentials (API keys, Bearer tokens, PEM private keys, database connection strings—including JSON-escaped tokens such as `\u0067hp_...`) or environmental personal data (Windows profile paths `C:\Users\<user>\...`, Unix home paths `/home/<user>/...`, author email addresses). Without a perimeter gate, raw secrets can leak into disk artifacts or downstream prompts; conversely, a naive single-tier blocker rejects every stack trace containing a local Windows username, preventing legitimate defect reports from entering the pipeline.
2. **Ad-Hoc and Non-Reproducible Lane Routing (`ASDD` vs. `FTD`):**
   Downstream teams (`2_Spec_Writer` and `3_Builder`) require a deterministic assignment between full specification workflows (`ASDD`) and bounded maintenance workflows (`FTD`), along with a suggested `GengiSWAP` task template (`modulo_nuevo`, `correccion_hallazgos`, `auditoria_e2e`, `mapeo_estatico`). Leaving this classification to ad-hoc agent judgment allows high-risk changes to bypass architectural specification or inflates review overhead on localized bug fixes.
3. **Silent Risk-Zone Bypass on Empty Frontmatter `covers_paths` (`0_Brain` Root-Cause Gap):**
   In the live `0_Brain` vault (`0_Brain/vault/wiki/risk_zones.md:11` and `0_Brain/schemas/risk_zones.template.md:11`), the YAML frontmatter declares `covers_paths: []` while the actual risk-zone catalog (`RZ-001` on `GengiSWAP/engine/` as `CRITICO`; `RZ-002` on `0_Brain/engine/` as `ALTO`) lives inside a 6-column Markdown table (`risk_zones.md:35-38`). Because `brain.exe context --json` (`0_Brain/engine/query.go:95-105`) matches files only against frontmatter `covers_paths` using case-sensitive comparisons (`0_Brain/engine/index.go:54-93`), relying on the unpatched `context` output produces `matched_paths: []` and misses cataloged risk zones—an issue that affects both `1_Intake` and downstream `6_Filtro_de_Riesgo`. Additionally, `0_Brain/vault/index.md:35` lists project `black-factory` before `0_Brain/vault/projects/black-factory.md` exists on disk.
4. **FTS5 Implicit-`AND` Mismatch and Non-Empty Slice Serialization in `0_Brain`:**
   `brain.exe query` (`0_Brain/engine/query.go:408-452`) joins search terms with an implicit FTS5 `AND` unless explicit boolean `OR` operators are supplied (`isFTSBooleanOp`, `query.go:428-431`) and serializes zero matches as JSON `null`. Passing raw multi-sentence titles directly to `brain.exe query` returns `null` whenever a single natural-language word is absent from vault documents.
5. **Lack of Idempotency, Traceability, and Option i Privacy for Operational Incidents:**
   Repeated submissions of the same external ticket (`project + source + source_ref`) or retries after a clarification hold (`G6`) lack a deterministic deduplication key, atomic quarantine-to-ready promotion lifecycle, machine-queryable telemetry (`trace_id`, per-phase timings, payload-free `G4` rejection events in `intake_events.jsonl`), and `.gitignore` isolation under Option i privacy.

**Proposed Solution:**
First, extract SQLite-free shared primitives (`g4Rules`/`ScanG4`, `NormalizeCoverPath`/`MatchCoverPath`/`MatchCoverPathFold`, OS kernel `lock_*` with bounded retry, and ancestor-chain `reparse_*` guards) into a pure-Go monorepo package (`pkg/bfcore`, `CGO_ENABLED=0`, permitting `golang.org/x/sys`), and resolve the root-cause gaps directly in `0_Brain` (exposing structured `risk_hits` from `wiki/risk_zones.md` inside `brain.exe context --json`, initializing `0_Brain/vault/projects/black-factory.md`, and returning non-null `[]` JSON slices). Second, build `1_Intake/engine` (`module blackfactory/intake`) and CLI `1_Intake/intake.exe` implementing dual-phase in-memory `G4` enforcement (raw bytes + decoded JSON strings + pre-write `ScanG4All` verification), read-only `0_Brain` preflight and subprocess consultation (`BrainClient`), 100% deterministic locatability and `ASDD`/`FTD` lane triage, and atomic JSON `WorkItem` persistence (`WI-<hash12>.json`) under `1_Intake/runtime/`.

## Goals

*   **IG1 — Zero-CGO Pure-Go Determinism & Shared Core Reuse:** Compile `1_Intake/intake.exe` from `1_Intake/engine` with `CGO_ENABLED=0` and zero SQLite dependencies by consuming the shared monorepo package `pkg/bfcore` (which permits only `golang.org/x/sys` for Windows kernel locking and reparse-point checks), passing `go vet ./...` and `go test -count=1 -cover ./...` on isolated `t.TempDir()` directories (`G3`).
*   **IG2 — Dual-Phase Tier 1 G4 Secret Blocking & Fixed-Point Tier 2 Redaction:** Detect all 8 Tier 1 hard-secret rules from `pkg/bfcore` (`0_Brain/engine/validator.go:49-105`) both on raw input bytes and on decoded JSON string values in memory before writing any `WorkItem` file (`0` files written to `workitems/` or `quarantine/`, exit code `3`, logging only a sanitized payload-free `REJECTED_G4` event to `intake_events.jsonl` under `.intake.lock`). Apply fixed-point delimiter-preserving subgroup redaction (`[REDACTED:G4:<rule>]`) for the 3 Tier 2 environmental rules on free-text fields (`title`, `description`, `note`) and enforce `ScanG4All` verification on the final serialized JSON buffer before any disk write.
*   **IG3 — Root-Cause Risk-Zone Detection (`RZ-001` / `RZ-002`) via `0_Brain` & Case-Insensitive Path Matching:** Surface all path matches against `wiki/risk_zones.md` through `brain.exe context --json` (`risk_hits` parsed natively by `0_Brain` using `pkg/bfcore.MatchCoverPathFold` for Windows case-insensitivity), routing `CRITICO` hits unconditionally to `ASDD` with `requires_autor_hitl: true` and failing safe to `ASDD` (`RISK_ZONES_UNPARSEABLE_FAILSAFE`) if risk-zone metadata is missing or malformed.
*   **IG4 — Deterministic Triage & Unified `WI-<hash12>` Idempotency:** Given identical input and `0_Brain` vault state, emit identical `lane` (`ASDD` | `FTD`), `risk_level` (`NONE` | `BAJO` | `MEDIO` | `ALTO` | `CRITICO`), `triage_reasons`, and deterministic `id` (`WI-<dedupe_key[:12]>` when identity fields are present, or `WI-<sanitized_payload_sha256[:12]>` with `dedupe_key: ""` when incomplete in quarantine), preventing duplicate records and supporting atomic promotion from `quarantine/` to `workitems/` via `intake.exe resolve` or re-ingestion.
*   **IG5 — Windows Latency Budget (`< 500 ms` p95):** Complete end-to-end local ingestion and triage (including read-only `0_Brain` preflight, at most 2 `brain.exe` subprocess calls, and atomic file commit under `.intake.lock`) in under `500 ms` p95 on Windows for vaults containing 50–100 indexed documents.
*   **IG6 — Option i Privacy Compliance:** Keep all operational runtime artifacts (`1_Intake/runtime/workitems/`, `quarantine/`, `intake_events.jsonl`, `.intake.lock`) and compiled binaries (`1_Intake/intake.exe`) excluded from Git via `.gitignore`, verified automatically by `intake.exe verify`.

## Non-Goals

*   **NG1 — External Webhooks, Network Integrations, or Custom YAML/Markdown Input Parsers in v1:** GitHub Issues API, Sentry webhooks, web forms, Slack, email ingestion, the `IngestSource` adapter interface, and custom Markdown+YAML frontmatter input parsing are excluded from v1; v1 accepts only CLI flags and local JSON files/`stdin` (`--input <file.json>` or `--input -`).
*   **NG2 — Any Mutation or Recovery of `0_Brain/vault/` at Runtime:** `1_Intake` is strictly a read-only consumer of `0_Brain`. It never creates proposals (`_proposals/`), modifies `log.md`, writes to `.index/brain.db`, or invokes `brain.exe verify` during `ingest`/`resolve` preflight (because `brain.exe verify` executes `RecoverJournal`, which mutates the vault).
*   **NG3 — Probabilistic, LLM-Driven, or Lexical-Heuristic Triage Inside `1_Intake`:** `1_Intake` never invokes an LLM, network service, or free-text lexical heuristic dictionary (`HEURISTIC_LEXICAL_ESCALATION`) to classify work items, nor does it infer missing `affected_paths` from FTS5 search results; all lane routing is governed 100% by structured deterministic rules.
*   **NG4 — Separate `SDD` or `TDD` Output Lanes:** Spec-Driven Development (`SDD`) is unified under the `ASDD` lane, and Test-Driven Development (`TDD`) is a mandatory execution discipline enforced downstream in `3_Builder` across both lanes.
*   **NG5 — Persisting Duplicate Markdown `WorkItem` Files or Dual Operational Logs:** `WI-<hash12>.json` is the sole persisted `WorkItem` source of truth in `workitems/` and `quarantine/`, and `intake_events.jsonl` is the sole append-only operational log in v1 (no `intake_log.md` and no `--emit-md` flag); human-readable Markdown views are rendered exclusively on demand via `intake.exe show --format md`.
*   **NG6 — Replacing Post-Build Diff Governance (`6_Filtro_de_Riesgo`):** `1_Intake` attaches pre-build risk metadata based on declared `affected_paths`, whereas `6_Filtro_de_Riesgo` evaluates actual Git diffs after `3_Builder` execution.

## Open Questions

None. All v1 architectural decisions have been ratified by `[AUTOR]` (`blackpusch`) and incorporated into this specification.

## Critical User Journeys (CUJs)

### CUJ 1: Fast-Track (`FTD`) Ingestion of a Bounded Bug Report with Tier 2 Path Redaction

*   **Given**: `0_Brain/vault/` is initialized and indexed (`.index/brain.db` present, no `.brain-journal.json` pending), active project document `0_Brain/vault/projects/black-factory.md` (`status: active`) is indexed, and an active wiki document `0_Brain/vault/wiki/brain_engine_g4.md` (`status: active`, `covers_paths: ["0_Brain/engine/"]`) covers `0_Brain/engine/validator.go` (which also matches `RZ-002` at level `ALTO` in `wiki/risk_zones.md`).
*   **When**: An operator runs `intake.exe ingest --input bug_report.json --json`, where `bug_report.json` contains `{"project": "black-factory", "kind": "bug", "source": "cli", "source_ref": "BUG-104", "title": "Fix delimiter capture in validator G4 path scan", "description": "Stack trace in C:\\Users\\jdoe\\go\\src\\validator.go reported by jdoe@example.com", "affected_paths": ["0_Brain/engine/validator.go"], "impact_flags": []}`.
*   **Then**:
    1. `intake.exe` strips any UTF-8 BOM, verifies zero Tier 1 secrets in raw bytes, decodes the JSON payload with `DisallowUnknownFields()`, verifies zero Tier 1 secrets across all decoded string fields, and applies fixed-point Tier 2 redaction inside `description` to produce `[REDACTED:G4:win_user_path]\go\src\validator.go` and `[REDACTED:G4:email_address]`, recording `g4_redactions` counts and canonical `sanitized_payload_sha256`.
    2. `intake.exe` verifies read-only preflight via `os.Lstat` on `0_Brain/vault/.index/brain.db` and absence of `.brain-journal.json`, invokes `0_Brain/brain.exe context --vault 0_Brain/vault --project black-factory --files 0_Brain/engine/validator.go --json` (which returns both `wiki/brain_engine_g4.md` with `status: "active"` and structured `risk_hits` matching `RZ-002` at level `ALTO`), verifies single-module scope (`1 <= 5` concrete files) and active `0_Brain` coverage, and assigns `lane: "FTD"`, `risk_level: "ALTO"`, `suggested_template: "correccion_hallazgos"`, and `status: "ready"`.
    3. `intake.exe` verifies `ScanG4All` on the final serialized `WorkItem` JSON buffer in memory, acquires `1_Intake/runtime/.intake.lock` (with bounded retry), writes `1_Intake/runtime/workitems/WI-<hash12>.json` atomically, appends a `TRIAGED` event to `intake_events.jsonl`, prints the JSON result to `stdout`, and exits with code `0`.

### CUJ 2: Tier 1 G4 Hard-Secret Fail-Closed Rejection (Raw and JSON-Escaped)

*   **Given**: A developer or script invokes `intake.exe ingest --input ticket.json --json` where `ticket.json` contains a GitHub personal access token (either literal `ghp_...` or JSON Unicode-escaped `\u0067hp_...`), a Bearer token, or the phrase `PRIVATE KEY`.
*   **When**: `intake.exe` executes Stage 1 (raw byte scan) and Stage 2 (decoded string field scan) in memory before creating any `WorkItem` or calling `0_Brain/brain.exe`.
*   **Then**:
    1. `intake.exe` aborts ingestion immediately, creating `0` files in `workitems/` and `0` files in `quarantine/` (`0` payload bytes written to disk).
    2. `intake.exe` sanitizes `source` and `source_ref` against all 11 `G4` rules (replacing any match with `[REDACTED:G4:<rule>]`), acquires `1_Intake/runtime/.intake.lock`, appends a payload-free forensic record `{timestamp, trace_id, event: "REJECTED_G4", rule_name, field, line, raw_sha256, source, source_ref}` (verified by `ScanG4All` to contain zero secret or PII substrings) to `1_Intake/runtime/intake_events.jsonl`, outputs the structured error JSON to `stderr`, and exits with code `3`.

### CUJ 3: Quarantine (`G6`) on Missing Scope and Atomic Promotion via `intake.exe resolve`

*   **Given**: A defect report is ingested via `intake.exe ingest --input vague_bug.json --json` without `affected_paths` (or with a personal path `C:\Users\admin\app\main.go` inside `affected_paths`).
*   **When**: `intake.exe` evaluates the Locatability & Metadata Gate (Stage 3), and the operator subsequently supplies the missing repository-relative path via `intake.exe resolve --id WI-<hash12> --path "GengiSWAP/engine/ring.go" --note "Scope confirmed by AUTOR" --json`.
*   **Then**:
    1. On the initial `ingest`, `intake.exe` acquires `.intake.lock`, writes `1_Intake/runtime/quarantine/WI-<hash12>.json` with `status: "needs_clarification"`, `lane: ""`, `suggested_template: ""`, `quarantine_info.reason_codes` (`["UNCERTAIN_SCOPE"]` or `["INVALID_AFFECTED_PATH_G4"]`), and an actionable `resolve_hint` command, appends a `QUARANTINED` event to `intake_events.jsonl`, and exits with code `2`.
    2. On `intake.exe resolve`, `intake.exe` loads `quarantine/WI-<hash12>.json`, replaces `affected_paths` with `["GengiSWAP/engine/ring.go"]`, runs `0_Brain/brain.exe context --json` (matching `RZ-001` at level `CRITICO` via case-insensitive `MatchCoverPathFold`), assigns `lane: "ASDD"` with `requires_autor_hitl: true`, `suggested_template: "correccion_hallazgos"`, and `revision: 2`, verifies no conflicting `workitems/WI-<dedupe_key[:12]>.json` exists, atomically writes `1_Intake/runtime/workitems/WI-<dedupe_key[:12]>.json`, removes the file from `quarantine/`, appends `PROMOTED` to `intake_events.jsonl`, and exits with code `0`.

## Technical Plan (Architecture)

### 1. System Topology and Five-Stage Execution Pipeline

`1_Intake` is structured under `1_Intake/` following the same hybrid Option 3C + Option i architecture as `0_Brain`, backed by the shared SQLite-free package `pkg/bfcore`:

```text
Black Factory\
├── go.work                                (Go workspace linking pkg/bfcore, 0_Brain/engine, 1_Intake/engine)
├── pkg\
│   └── bfcore\                            (Module blackfactory/pkg/bfcore: CGO_ENABLED=0, no SQLite, only golang.org/x/sys)
│       ├── go.mod
│       ├── README.md                      (Shared package documentation & symbol index)
│       ├── g4.go                          (Canonical 11 G4 rules, ScanG4, Tier 1/Tier 2 classification, fixed-point redactor)
│       ├── paths.go                       (NormalizeCoverPath, MatchCoverPath, MatchCoverPathFold)
│       ├── lock_windows.go                (LockFileEx via golang.org/x/sys/windows with bounded retry backoff)
│       ├── lock_other.go                  (POSIX flock LOCK_EX|LOCK_NB with bounded retry backoff)
│       ├── reparse_windows.go             (FILE_ATTRIBUTE_REPARSE_POINT & CheckNoReparseChain via os.Lstat)
│       ├── reparse_other.go               (POSIX ModeSymlink|ModeIrregular & CheckNoReparseChain via os.Lstat)
│       └── bfcore_test.go                 (Unit tests for G4, fixed-point redaction, glob folding, locking, reparse chains)
└── 1_Intake\
    ├── README.md                          (Team 1 boundaries, invariants, CLI verification)
    ├── schemas\
    │   └── workitem.schema.json           (Canonical JSON Schema Draft 2020-12 v1.0 for WorkItem)
    ├── skills\
    │   ├── README.md                      (Skill catalog for Team 1)
    │   └── intake-triage\
    │       └── SKILL.md                   (Progressive-load operational guide for intake.exe)
    ├── engine\                            (Go module blackfactory/intake, CGO_ENABLED=0, imports blackfactory/pkg/bfcore)
    │   ├── go.mod
    │   ├── README.md                      (Package intake documentation & exported symbol index)
    │   ├── model.go                       (WorkItem v1.0 structs, enums, dedupe_key, sanitized_payload_sha256)
    │   ├── input.go                       (JSON file/stdin reader with 1 MiB quota & BOM strip, CLI flag merger)
    │   ├── paths.go                       (Repo-relative path guard before path.Clean, root module counter)
    │   ├── brain_client.go                (BrainClient interface, read-only preflight, disjunctive OR keyword query)
    │   ├── triage.go                      (100% deterministic Locatability Gate + ASDD/FTD lane triage engine)
    │   ├── store.go                       (Atomic storage under .intake.lock, quarantine promotion, intake_events.jsonl)
    │   ├── render.go                      (On-demand ASCII/Box-Drawing Markdown renderer for show --format md)
    │   ├── intake_test.go                 (Unit & table-driven triage, G4, risk-zone, and latency benchmark tests)
    │   └── cmd\
    │       └── intake\
    │           ├── README.md              (CLI subcommand, flag, and exit-code documentation)
    │           ├── main.go                (CLI dispatcher: ingest, resolve, list, show, verify; exit codes 0..5)
    │           └── main_test.go           (E2E CLI lifecycle tests covering all 6 exit codes on t.TempDir())
    ├── intake.exe                         (Compiled local binary; ignored by .gitignore)
    └── runtime\                           (Untracked Option i operational data; ignored by .gitignore)
        ├── .intake.lock                   (OS kernel lock file; never unlinked on release)
        ├── workitems\                     (Triaged ready WorkItems: WI-<hash12>.json)
        ├── quarantine\                    (G6 holds: WI-<hash12>.json)
        └── intake_events.jsonl            (Append-only structured telemetry & payload-free G4 rejections)
```

#### Five-Stage Ingestion and Triage Data Flow

```text
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 1: RAW INPUT BOUNDARY & PHASE-A TIER 1 G4 GATE (In-Memory)                  │
│  • Read up to MaxInputBytes (1 MiB) from --input <file.json> (verifying no        │
│    reparse points across ancestor chain) or --input - (stdin), or CLI flags       │
│  • Strip UTF-8 BOM (\xEF\xBB\xBF); scan raw lines/flag values against 8 Tier 1    │
│    G4 hard-secret rules (plus 3 Tier 2 rules if --strict-g4 is active),           │
│    suppressing literal_private_key on pem_private_key lines                       │
│  • On hit: write 0 files to workitems/ or quarantine/; acquire .intake.lock       │
│    (skip if --dry-run), sanitize source/source_ref, verify ScanG4All on event,    │
│    append REJECTED_G4 to intake_events.jsonl; print JSON error to stderr ──► EX 3 │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │ (0 Phase-A Tier 1 hits)
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 2: JSON DECODING, PHASE-B TIER 1 SCAN, FIXED-POINT TIER 2 & PATH HYGIENE    │
│  • Decode JSON with DisallowUnknownFields() and merge explicit CLI flag overrides │
│  • Phase-B Tier 1 scan: run 8 Tier 1 G4 rules (or all 11 if --strict-g4) across   │
│    ALL decoded string fields (catches JSON escapes like \u0067hp_...) ─────► EX 3 │
│  • Apply fixed-point delimiter-preserving Tier 2 redaction ([REDACTED:G4:<rule>]) │
│    to free-text fields: title, description, note; if Tier 2 matches in metadata   │
│    fields (project, source, source_ref), flag for quarantine                      │
│  • Validate affected_paths BEFORE path.Clean: reject .., :, \\, \\?\, leading /,  │
│    Windows reserved device names, trailing / (directories), globs (*?[), or Tier 2│
│    personal paths -> discard offending paths in memory, flag quarantine code      │
│    INVALID_AFFECTED_PATH_G4                                                       │
│  • Compute canonical sanitized_payload_sha256 and length-prefixed dedupe_key      │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 3: READ-ONLY 0_BRAIN PREFLIGHT & METADATA / LOCATABILITY GATE               │
│  • Read-only Preflight (NG2): checkancestor chain via CheckNoReparseChain; verify │
│    0_Brain/vault/.index/brain.db exists via os.Lstat and .brain-journal.json does │
│    NOT exist (never invokes mutating brain.exe verify during ingest/resolve)──►EX 4│
│  • Validate project & scope: if required fields (project, title, description,     │
│    kind, source, source_ref) are missing/invalid, Stage 2 flagged invalid paths   │
│    or metadata PII, or affected_paths is empty (UNCERTAIN_SCOPE):                 │
│    verify ScanG4All on JSON buffer, acquire .intake.lock, persist WI-<hash12>.json│
│    to runtime/quarantine/ with resolve_hint, append QUARANTINED event ─────► EX 2 │
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │ (Metadata complete & explicit paths valid)
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 4: 0_BRAIN CONSULTATION (<= 2 SUBPROCESSES) & RISK-ZONE EVALUATION          │
│  • Subprocess 1: run brain.exe context --vault <v> --project <p> --files <csv>    │
│    --json -> returns matched active docs (constitution, risk_zones, wiki, adr)    │
│     plus structured risk_hits evaluated by 0_Brain against wiki/risk_zones.md      │
│  • Subprocess 2: run brain.exe query --vault <v> --project <p> --q "<disjunctive  │
│    OR tokens or project check>" --json -> confirms project registration (status:  │
│    "active" in type: "project") and records supplemental search evidence          │
│  • If project is not registered as active -> quarantine (UNREGISTERED_PROJECT)EX 2│
└─────────────────────────────────────────┬─────────────────────────────────────────┘
                                          │
                                          ▼
┌───────────────────────────────────────────────────────────────────────────────────┐
│ STAGE 5: DETERMINISTIC TWO-LANE TRIAGE (ASDD vs. FTD), IDEMPOTENCY & COMMIT       │
│  • Evaluate 100% deterministic triggers (Rules 7..13):                            │
│    ├─ Any trigger fires ──► lane = "ASDD"                                         │
│    └─ Zero triggers fire ─► lane = "FTD"                                          │
│  • Verify ScanG4All on final serialized WorkItem JSON buffer in memory            │
│  • Acquire kernel lock runtime/.intake.lock (with bounded retry backoff):         │
│    ├─ Existing ready WI with same dedupe_key & same sha256 ──► idempotent   EX 0  │
│    ├─ Existing ready WI with different sha256 & no --update ─► conflict     EX 5  │
│    └─ Write temp JSON -> atomic rename to workitems/WI-<hash12>.json (deleting    │
│       prior quarantine file if promoted) -> append intake_events.jsonl ────► EX 0 │
└───────────────────────────────────────────────────────────────────────────────────┘
```

### 2. Two-Tier G4 Perimeter Design and Shared `pkg/bfcore` Contract

Both `0_Brain/engine` and `1_Intake/engine` import `blackfactory/pkg/bfcore`, which houses the single authoritative registry of the 11 regular expressions originally defined in `0_Brain/engine/validator.go:49-105`, partitioned into two operational tiers:

| Rule Name (`pkg/bfcore` ID) | Tier | Default Action in `1_Intake` | Target Scope | Special Handling |
| :--- | :---: | :--- | :--- | :--- |
| `pem_private_key` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Marks line to suppress `literal_private_key` (`validator.go:159-167`) |
| `literal_private_key` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Skipped on lines where `pem_private_key` already matched |
| `bearer_token` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | 0 payload files written; payload-free `REJECTED_G4` event in JSONL |
| `known_token` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Matches `ghp_`, `github_pat_`, `sk-`, `AKIA`, `AIza`, `xox*` |
| `jwt_token` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Matches 3-part base64url `eyJ...` tokens |
| `conn_uri_credentials` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Matches `postgres://`, `mysql://`, `mongodb://`, `redis://`, etc. with user:pass |
| `conn_password_param` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Matches `password=`, `passwd=`, `pwd=` parameters |
| `secret_assignment` | **Tier 1** | Fail-Closed (`Exit 3`) | Raw bytes + all decoded JSON strings | Matches `api_key`, `access_token`, `client_secret`, `private_key` assignments |
| `win_user_path` | **Tier 2** | Fixed-Point Subgroup Redact (or `Exit 3` w/ `--strict-g4`) | Decoded `title`, `description`, `note` | Replaces `C:\Users\<user>` with `[REDACTED:G4:win_user_path]`; in `affected_paths` or metadata triggers quarantine (`Exit 2`) |
| `unix_user_path` | **Tier 2** | Fixed-Point Subgroup Redact (or `Exit 3` w/ `--strict-g4`) | Decoded `title`, `description`, `note` | Preserves leading `(?:^\|[\s"'(=:])` and trailing `(?:/\|$\|\s\|["'])` delimiters from `validator.go:67`, replacing only `(?:/home\|/Users)/[A-Za-z0-9._-]+` in a fixed-point loop so adjacent paths (`/home/a /home/b`) never slip through |
| `email_address` | **Tier 2** | Fixed-Point Subgroup Redact (or `Exit 3` w/ `--strict-g4`) | Decoded `title`, `description`, `note` | Replaces email token with `[REDACTED:G4:email_address]`; in metadata triggers quarantine (`Exit 2`) |

**Memory-to-Disk G4 Invariant:**
1. **Dual-Phase Tier 1 Scan:** Running Tier 1 on raw bytes (Stage 1) blocks malformed payloads containing raw secrets before allocation, while running Tier 1 again on every decoded JSON string value (Stage 2) closes the JSON escape vector (e.g., `"\u0067hp_..."` or `"postgres:\/\/user:pass@host"`).
2. **Pre-Write `ScanG4All` Barrier:** Neither `SaveTriaged`, `SaveQuarantined`, nor `AppendEvent` may write bytes to disk unless `bfcore.ScanG4All(serializedJSONBytes)` returns `0` violations across all 11 rules. In `REJECTED_G4` events, `source` and `source_ref` are sanitized through `bfcore.RedactAllG4` prior to serialization so a secret or email accidentally passed in `--source-ref` never leaks into `intake_events.jsonl`.

### 3. `0_Brain` Integration Contract (`BrainClient`) and Phase 0 Root-Cause Alignment

Rather than duplicating Markdown table parsers inside `1_Intake`, Phase 0 of the implementation plan resolves the two upstream gaps directly in `0_Brain` and `pkg/bfcore`:
*   **Structured Risk Hits in `0_Brain` (`brain.exe context --json`):** `0_Brain/engine/query.go` (`ContextForFiles`) uses `bfcore.MatchCoverPathFold` (case-insensitive on Windows so `gengiswap/engine/ring.go` matches `GengiSWAP/engine/`) and natively parses the 6-column `## Catálogo de Zonas de Riesgo` table in `0_Brain/vault/wiki/risk_zones.md` (`| ID | Proyecto | Rutas cubiertas (`covers_paths`) | Nivel | Compuerta obligatoria | Evidencia `G5` |`), stripping per-pattern backticks, splitting `,` and `;`, normalizing `CRÍTICO` -> `CRITICO`, and emitting matched `risk_hits` and `risk_zones_sha256` directly in the `context --json` response.
*   **Active Project Document (`projects/black-factory.md`) and Non-Null JSON Slices:** `0_Brain/vault/projects/black-factory.md` (`type: project`, `project: black-factory`, `status: active`) is created and indexed in `0_Brain/vault/`, and `0_Brain/engine/query.go` initializes result slices to `[]SearchResult{}` so JSON output is always `[]` instead of `null`.

`1_Intake/engine/brain_client.go` defines the `BrainClient` interface to decouple triage logic from the transport mechanism (CLI subprocess in v1; persistent `brain.exe mcp` JSON-RPC client in Phase 2):

```go
type BrainClient interface {
    Preflight(vaultRoot, dbPath string) (BrainPreflight, error)
    Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error)
    Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error)
}
```

1. **Strictly Read-Only Preflight (`Preflight` — enforcing `NG2`):**
   *   Verifies that `vaultRoot` and `dbPath` (`<vaultRoot>/.index/brain.db`) traverse zero symlinks or NTFS reparse points via `bfcore.CheckNoReparseChain`.
   *   Checks `os.Lstat` on `<vaultRoot>/.index/brain.db` (capturing `brain_db_mtime`). If missing, returns `ErrBrainIndexMissing` (wrapped as `ErrBrainUnavailable`) -> `intake.exe` exits with code `4`.
   *   Checks `os.Lstat` on `<vaultRoot>/.brain-journal.json`. If `.brain-journal.json` exists, `Preflight` **immediately returns `ErrBrainUnavailable` (exit code `4`)** without spawning `brain.exe verify` (which would call `RecoverJournal` and mutate the vault, violating `NG2`), instructing the operator in `stderr` to run `0_Brain/brain.exe verify` manually.
2. **File Context & Structured Risk Evaluation (`Context` — Subprocess 1):**
   *   Invokes `0_Brain/brain.exe context --vault <vaultRoot> --db <dbPath> --project <project> --files <csv> --json`.
   *   Collects matched active documents (`type` in `constitution`, `risk_zones`, `wiki`, `adr` with `status == "active"`) and structured `risk_hits` (`id`, `project`, `level`, `matched_paths`, `mandatory_gate`) plus `risk_zones_sha256`.
   *   **Fail-Safe Invariant:** If `risk_zones_parse_error` is reported by `0_Brain` or zero `type: "risk_zones"` active documents exist in the vault, `1_Intake` records reason code `RISK_ZONES_UNPARSEABLE_FAILSAFE`, sets `requires_autor_hitl: true`, and forces `lane: "ASDD"`.
3. **Active Project Verification & Disjunctive FTS5 Query (`Query` & `ExtractQueryTokens` — Subprocess 2):**
   *   `ExtractQueryTokens(title, description string, maxTokens int) []string` extracts up to `maxTokens` (default `5`) high-specificity technical tokens: file paths (`/` or `\`), filenames (`*.go`, `*.md`, `*.json`, `*.sql`), catalog codes (`RZ-\d+`, `ADR-\d+`, `G[1-6]`), `CamelCase`/`snake_case` identifiers, and alphanumeric words `>= 4` chars, filtering out Spanish/English stop-words and FTS5 reserved uppercase words (`AND`, `OR`, `NOT`, `NEAR`) and stripping punctuation outside `[A-Za-z0-9_./-]`.
   *   `BuildDisjunctiveFTSQuery(tokens []string) string` wraps each sanitized token in double quotes and joins them with explicit `OR` (`"\"validator.go\" OR \"scang4\" OR \"delimiter\""`), which `0_Brain/engine/query.go:buildFTS5Query` (`L428-431`) executes as a single BM25-ranked disjunctive FTS5 query without triggering the `quoteAll` fallback (`query.go:60`).
   *   `IsProjectRegistered` verifies that `project` has an active project record (`type: "project"`, `status: "active"`). To stay strictly within the `<= 2` subprocess budget, if `<vaultRoot>/projects/<project>.md` exists (verified via `CheckNoReparseChain` and frontmatter `status: active`), registration is confirmed in-process (< 1 ms) and Subprocess 2 runs `brain.exe query --vault <vaultRoot> --db <dbPath> --project <project> --q "<OR-query>" --json`; otherwise Subprocess 2 queries `--type project --project <project> --limit 1 --json` and filters for `status == "active"`.

### 4. Canonical `WorkItem` JSON Schema (`schema_version: "1.0"`)

Every persisted file in `1_Intake/runtime/workitems/WI-<hash12>.json` or `1_Intake/runtime/quarantine/WI-<hash12>.json` conforms to the following JSON structure:

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
  "description": "Stack trace in [REDACTED:G4:win_user_path]\\go\\src\\validator.go reported by [REDACTED:G4:email_address]",
  "note": "",
  "kind": "bug",
  "source": "cli",
  "source_ref": "BUG-104",
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
      "detail": "Single-module bug with explicit affected_paths (1<=5), active 0_Brain wiki/adr coverage, zero impact_flags, and ALTO (non-CRITICO) risk zone"
    }
  ],
  "sanitization": {
    "strict_g4": false,
    "sanitized_payload_sha256": "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
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
        "command": "0_Brain/brain.exe query --vault 0_Brain/vault --project black-factory --q \"\\\"validator.go\\\" OR \\\"scang4\\\" OR \\\"delimiter\\\"\" --json",
        "exit_code": 0,
        "duration_ms": 21,
        "result_count": 1
      }
    ],
    "matched_docs": [
      {
        "id": "brain-constitution",
        "rel_path": "constitution.md",
        "type": "constitution",
        "status": "active",
        "verified_head": "",
        "transversal": true
      },
      {
        "id": "brain-risk-zones",
        "rel_path": "wiki/risk_zones.md",
        "type": "risk_zones",
        "status": "active",
        "verified_head": "",
        "transversal": true
      },
      {
        "id": "brain-engine-g4",
        "rel_path": "wiki/brain_engine_g4.md",
        "type": "wiki",
        "status": "active",
        "verified_head": "",
        "transversal": false
      }
    ]
  },
  "quarantine_info": null
}
```

#### Canonical Hashing, Unified `WI-<hash12>` Identity, and Auxiliary Schemas

1. **Canonical `sanitized_payload_sha256` Formula:**
   Computed as the lowercase hex SHA-256 digest of the compact UTF-8 JSON serialization (`encoding/json.Marshal` on a fixed-order struct with sorted string slices) of the sanitized input fields:
   `CanonicalPayload{Project: lower(trim(project)), Title: trim(redactedTitle), Description: trim(redactedDescription), Note: trim(redactedNote), Kind: lower(trim(kind)), Source: lower(trim(source)), SourceRef: trim(sourceRef), AffectedPaths: cleanPaths, ImpactFlags: sortedFlags}`.
2. **Canonical `dedupe_key` and Unified `WI-<hash12>` ID Formula:**
   *   When `project`, `source`, and `source_ref` are all non-empty (and contain zero Tier 2 violations), `dedupe_key` is computed via length-prefixed SHA-256 over lowercased trimmed values (`p`, `s`, `r`) to prevent delimiter collisions if any field contains `|`:
       `dedupe_key = sha256_hex(len(p) + ":" + p + "|" + len(s) + ":" + s + "|" + len(r) + ":" + r)`
       and `id = "WI-" + dedupe_key[:12]`.
   *   When any of `(project, source, source_ref)` is missing or flagged during quarantine (`Exit 2`), `dedupe_key` is set to `""` and the item is assigned `id = "WI-" + sanitized_payload_sha256[:12]` in `1_Intake/runtime/quarantine/WI-<sanitized_payload_sha256[:12]>.json`. Using `sanitized_payload_sha256[:12]` instead of hashing empty identity strings prevents unrelated incomplete tickets from colliding in `quarantine/` while keeping a single `WI-` prefix across the entire system.
   *   **Collision & Promotion Contract:** Every `O(1)` lookup on `WI-<hash12>.json` verifies the full 64-hex `dedupe_key` (or `sanitized_payload_sha256` when `dedupe_key == ""`) inside the loaded file; if a 48-bit prefix collision ever occurs between two distinct keys, `Store` returns an explicit error (`Exit 1`). When `intake.exe resolve --id WI-<oldHash12>` completes missing identity fields so that the newly computed `WI-<dedupe_key[:12]>` differs from `WI-<oldHash12>`, `Store` checks `workitems/WI-<dedupe_key[:12]>.json` first: if a ready item already exists and `--update` is not set, `resolve` aborts with `Exit 5` (leaving `quarantine/WI-<oldHash12>.json` untouched); otherwise it writes `workitems/WI-<dedupe_key[:12]>.json` (`revision: prev + 1`) and unlinks `quarantine/WI-<oldHash12>.json`.
3. **`quarantine_info` Object Schema (non-null when `status == "needs_clarification"`):**
   ```json
   {
     "reason_codes": ["MISSING_REQUIRED_FIELDS", "INVALID_AFFECTED_PATH_G4", "UNCERTAIN_SCOPE"],
     "missing_fields": ["source_ref"],
     "invalid_path_reason": "G4_personal_path_in_affected_paths",
     "detail": "affected_paths contained a personal or non-repo path and was discarded in memory",
     "resolve_hint": "1_Intake/intake.exe resolve --id WI-6b86b273ff34 --source-ref <REF> --path <repo/relative/file.go> --json"
   }
   ```
4. **`IntakeEvent` Schema (`1_Intake/runtime/intake_events.jsonl`):**
   Every line in `intake_events.jsonl` is a compact JSON object passing `bfcore.ScanG4All` with `event` in `{"REJECTED_G4", "QUARANTINED", "TRIAGED", "IDEMPOTENT_HIT", "UPDATED", "PROMOTED", "CONFLICT_DEDUPE", "VERIFIED"}`:
   *   For `REJECTED_G4`: `{timestamp, trace_id, event: "REJECTED_G4", rule_name, field, line, raw_sha256, source, source_ref}` (zero secret/PII substrings).
   *   For lifecycle events (`QUARANTINED`, `TRIAGED`, `IDEMPOTENT_HIT`, `UPDATED`, `PROMOTED`, `CONFLICT_DEDUPE`): `{timestamp, trace_id, event, id, prev_id, revision, project, source, source_ref, status, lane, risk_level, reason_codes, duration_ms}`.
   *   For `VERIFIED`: `{timestamp, trace_id, event: "VERIFIED", ok, workitems_checked, quarantine_checked, events_checked}`.

### 5. Deterministic Triage Decision Matrix (`Quarantine` vs. `ASDD` vs. `FTD`)

| Priority | Rule Code | `rule_kind` | Trigger Condition | Outcome (`status` / `lane`) | Exit Code |
| :---: | :--- | :--- | :--- | :--- | :---: |
| **1** | `G4_TIER1_SECRET` | `deterministic` | Raw input bytes OR any decoded JSON string field matches any of the 8 Tier 1 G4 rules (or Tier 2 w/ `--strict-g4`) | Rejected (0 files in `workitems/` or `quarantine/`; sanitized `REJECTED_G4` in `intake_events.jsonl`) | `3` |
| **2** | `BRAIN_UNAVAILABLE` | `deterministic` | `.index/brain.db` missing, `.brain-journal.json` present, `brain.exe` missing/timeout/non-zero exit, or `.intake.lock` contention (`ErrIntakeLocked`) | Aborted (`G6` stop before triage) | `4` |
| **3** | `MISSING_REQUIRED_FIELDS` | `deterministic` | Any of `project`, `title`, `description`, `kind`, `source`, `source_ref` is empty after trim, `kind` outside `{bug, feature, refactor, audit_fix, ops}`, `impact_flags` outside `{contract, schema, db, auth, security, governance}`, or Tier 2 PII detected in metadata fields | `needs_clarification` in `quarantine/` (`lane: ""`, `suggested_template: ""`) | `2` |
| **4** | `INVALID_AFFECTED_PATH_G4` | `deterministic` | Any entry in `affected_paths` matches Tier 2 personal paths (`win_user_path`/`unix_user_path`), external drive/UNC/NT prefix, leading `/`, `..`, Windows reserved name, trailing `/` (directory), or glob (`*?[`) | Offending paths discarded in memory -> `needs_clarification` in `quarantine/` | `2` |
| **5** | `UNCERTAIN_SCOPE` | `deterministic` | `len(affected_paths) == 0` on input (or after Stage 2 path hygiene) | `needs_clarification` in `quarantine/` | `2` |
| **6** | `UNREGISTERED_PROJECT` | `deterministic` | `project` has no active document (`type: "project"`, `status: "active"`) in `0_Brain` | `needs_clarification` in `quarantine/` | `2` |
| **7** | `RISK_ZONE_CRITICO` | `deterministic` | Any `affected_path` matches a `CRITICO` row in `wiki/risk_zones.md` via `0_Brain` `context --json` (e.g. `RZ-001` `GengiSWAP/engine/`) | `ready` / `lane: "ASDD"`, `requires_autor_hitl: true` | `0` |
| **8** | `RISK_ZONES_UNPARSEABLE_FAILSAFE` | `deterministic` | `0_Brain` reports `wiki/risk_zones.md` missing, inactive, or `## Catálogo de Zonas de Riesgo` table unparseable | `ready` / `lane: "ASDD"`, `requires_autor_hitl: true` | `0` |
| **9** | `KIND_REQUIRES_ASDD` | `deterministic` | `kind == "feature"` or `kind == "ops"` | `ready` / `lane: "ASDD"` | `0` |
| **10** | `EXPLICIT_IMPACT_FLAGS` | `deterministic` | `len(impact_flags) > 0` (`contract`, `schema`, `db`, `auth`, `security`, `governance`) | `ready` / `lane: "ASDD"` | `0` |
| **11** | `MULTI_MODULE_OR_FILE_THRESHOLD` | `deterministic` | `affected_paths` spans `> 1` distinct root module (`CountRootModules`, where root-level files map to `"."`) or `> max_ftd_files` (default `5`) | `ready` / `lane: "ASDD"` | `0` |
| **12** | `NO_BRAIN_COVERAGE` | `deterministic` | Any explicit path in `affected_paths` lacks a matching active `type: "wiki"` or `type: "adr"` document (`status == "active"`, `len(matched_paths) > 0`) in `0_Brain` `context --json` | `ready` / `lane: "ASDD"` | `0` |
| **13** | `FTD_ELIGIBLE_BOUNDED_MAINTENANCE` | `deterministic` | Rules 1–12 did not fire (`kind` in `{bug, audit_fix, refactor}`, `1..5` concrete single-module files, active `wiki`/`adr` coverage for all paths, zero `impact_flags`, and `risk_level` in `{NONE, BAJO, MEDIO, ALTO}`) | `ready` / `lane: "FTD"` (carries `risk_level` and `risk_hits` if matched) | `0` |

**Risk-Level Aggregation & Downstream `suggested_template` Precedence:**
*   **`risk_level` Ordering:** Across all matched `risk_hits` for the work item's `project` (or `"*"`), `risk_level` is the maximum severity in the closed enumeration `NONE < BAJO < MEDIO < ALTO < CRITICO`.
*   **`suggested_template` Precedence:**
    1. If `status == "needs_clarification"` (quarantine) -> `""`
    2. Else if `lane == "ASDD"` and `kind == "feature"` -> `"modulo_nuevo"`
    3. Else if `lane == "ASDD"` and `kind == "ops"` -> `"auditoria_e2e"`
    4. Else if `lane == "ASDD"` and `kind` in `{bug, audit_fix, refactor}`: if `NO_BRAIN_COVERAGE` is the **sole** `ASDD` trigger -> `"mapeo_estatico"`; if any structural/risk trigger (`RISK_ZONE_CRITICO`, `RISK_ZONES_UNPARSEABLE_FAILSAFE`, `EXPLICIT_IMPACT_FLAGS`, `MULTI_MODULE_OR_FILE_THRESHOLD`) also fired -> `"correccion_hallazgos"`
    5. Else (`lane == "FTD"`) -> `"correccion_hallazgos"`

### 6. State Machine, Exit Codes (`0..5`), and CLI Subcommand Contract (`intake.exe`)

```text
                       ┌───────────────────────────┐
                       │     Raw Request Input     │
                       └─────────────┬─────────────┘
                                     │
         ┌───────────────────────────┼───────────────────────────┐
         │ (Tier 1 G4 Secret)        │ (Missing fields/scope)    │ (Complete & locatable)
         ▼                           ▼                           ▼
┌──────────────────┐      ┌───────────────────────┐      ┌───────────────────────┐
│   REJECTED_G4    │      │      QUARANTINED      │      │        TRIAGED        │
│  (Exit Code 3)   │      │ status:               │      │     status: ready     │
│ 0 WorkItem files │      │  needs_clarification  │      │    dir: workitems/    │
│ Event in JSONL   │      │   dir: quarantine/    │      │   lane: ASDD | FTD    │
└──────────────────┘      │     (Exit Code 2)     │      │     (Exit Code 0)     │
                          └───────────┬───────────┘      └───────────▲───────────┘
                                      │                              │
                                      │ intake.exe resolve --id WI-..│
                                      │ OR re-ingest same dedupe_key │
                                      └──────────────────────────────┘
                                       (Atomic promote: revision++,
                                        delete from quarantine/,
                                        log PROMOTED in JSONL)
```

#### Consolidated Exit Code Contract (`0..5`)

| Exit Code | Symbolic Name | Trigger Conditions | Disk Side Effects |
| :---: | :--- | :--- | :--- |
| **`0`** | `ExitOK` | `ingest`/`resolve` produced `status: "ready"` (`ASDD` or `FTD`), or matched an identical existing `ready` `WorkItem` (`idempotent_hit: true`), or `list`/`show`/`verify` succeeded (`ok: true`). | Atomic write to `workitems/WI-<hash12>.json` and event in `intake_events.jsonl` (zero writes if `--dry-run`, `list`, or `show`). |
| **`1`** | `ExitUsageOrError` | Invalid CLI flags, unknown subcommand, malformed JSON syntax, input exceeding `MaxInputBytes` (`1 MiB`), symlink/reparse-point violation, missing `--id` target in `resolve`/`show`, 48-bit ID prefix collision, or `intake.exe verify` failure (`ok: false`). | Zero `WorkItem` files written or modified. |
| **`2`** | `ExitQuarantined` | Request placed (or updated) in `1_Intake/runtime/quarantine/WI-<hash12>.json` with `status: "needs_clarification"` under Rules 3–6 (`G6` hold). | Atomic write to `quarantine/WI-<hash12>.json` and `QUARANTINED` event in `intake_events.jsonl` (zero writes if `--dry-run`). |
| **`3`** | `ExitRejectedG4` | Phase-A (raw bytes) or Phase-B (decoded JSON strings) matched a Tier 1 `G4` rule (or Tier 2 rule with `--strict-g4`). | `0` files created in `workitems/` or `quarantine/`; sanitized `REJECTED_G4` event appended to `intake_events.jsonl` (zero writes if `--dry-run`). |
| **`4`** | `ExitBrainOrLockUnavailable` | `0_Brain` preflight failed (`.index/brain.db` missing or `.brain-journal.json` present), `0_Brain/brain.exe` missing/timed out/failed, or `1_Intake/runtime/.intake.lock` remained busy after bounded retry (`ErrIntakeLocked`). | Zero `WorkItem` files written or modified. |
| **`5`** | `ExitDedupeConflict` | A `status: "ready"` file `workitems/WI-<dedupe_key[:12]>.json` already exists with a different `sanitized_payload_sha256` and `--update` was not passed (during `ingest` or `resolve` promotion). | Existing `workitems/` (and `quarantine/`) files left unmodified; `CONFLICT_DEDUPE` event appended to `intake_events.jsonl`. |

#### Subcommand Specifications

All 5 subcommands accept `--data-dir` (or `BF_INTAKE_DIR`, default `<repoRoot>/1_Intake/runtime`), `--vault` (or `BRAIN_VAULT`, default `<repoRoot>/0_Brain/vault`), `--brain-bin` (or `BRAIN_BIN`, default `<repoRoot>/0_Brain/brain.exe`), `--repo-root` (or `BF_REPO_ROOT`, discovered automatically from `os.Executable()` / current working directory upward to `.git` or `0_Brain` sibling, matching `0_Brain/engine/cmd/brain/main.go:539-578`), and `--json`, using flag reordering (`reorderFlags`) so flags may appear in any position:

1. **`intake.exe ingest`:**
   *   Flags: `--input <path.json|->`, `--project`, `--title`, `--description`, `--note`, `--kind`, `--source`, `--source-ref`, `--path` (repeatable), `--paths` (CSV), `--impact-flag` (repeatable), `--max-ftd-files` (default `5`), `--strict-g4`, `--update`, `--dry-run`, `--json`.
   *   Precedence & Lifecycle: If `--input` is provided, explicit CLI flags override corresponding fields decoded from the JSON input. Reading from `stdin` requires explicit `--input -`.
   *   **Re-Ingestion & `--update` Semantics:**
       *   If `workitems/WI-<hash12>.json` exists with the same `sanitized_payload_sha256`: returns `idempotent_hit: true` (`Exit 0`) without incrementing `revision`.
       *   If `workitems/WI-<hash12>.json` exists with a different `sanitized_payload_sha256`: without `--update`, exits `5` (`ErrDedupeConflict`); with `--update`, preserves `created_at`, sets `revision = prev.Revision + 1`, updates `updated_at`, atomically overwrites `workitems/WI-<hash12>.json` (or moves it to `quarantine/` if the updated payload fails a gate), and logs `UPDATED`.
       *   If `quarantine/WI-<hash12>.json` exists: re-ingesting an identical payload returns `Exit 2` without incrementing `revision`; re-ingesting a modified payload that still fails a gate overwrites `quarantine/WI-<hash12>.json` with `revision = prev.Revision + 1` (`Exit 2`); re-ingesting a completed payload promotes it atomically to `workitems/WI-<hash12>.json` with `revision = prev.Revision + 1`, deletes `quarantine/WI-<hash12>.json`, logs `PROMOTED`, and exits `0`.
   *   **`--dry-run` Invariant:** Executes Stages 1–5 in memory (including `0_Brain` read-only queries) and prints the resulting JSON/summary to `stdout`/`stderr` with the exact exit code (`0..5`), but **never acquires `.intake.lock` and writes `0` bytes** to `workitems/`, `quarantine/`, or `intake_events.jsonl`.
2. **`intake.exe resolve`:**
   *   Flags: `--id <WI-ID>` (required), `--project`, `--title`, `--description`, `--note`, `--kind`, `--source`, `--source-ref`, `--path` (repeatable, replaces `affected_paths`), `--paths` (CSV, replaces `affected_paths`), `--impact-flag` (repeatable, replaces `impact_flags`), `--max-ftd-files` (default `5`), `--strict-g4`, `--update`, `--dry-run`, `--json`.
   *   Behavior: Loads `runtime/quarantine/<WI-ID>.json` (returning `Exit 1` if not found), applies supplied field patches, re-runs Stages 1–5, and if all gates pass, checks for conflict at `runtime/workitems/WI-<dedupe_key[:12]>.json` (`Exit 5` if different hash exists without `--update`), writes `runtime/workitems/WI-<dedupe_key[:12]>.json` (`revision: prev + 1`), deletes `runtime/quarantine/<WI-ID>.json`, appends `PROMOTED` to `intake_events.jsonl`, and exits `0` (or updates `quarantine/` with `revision: prev + 1` and exits `2` if still incomplete).
3. **`intake.exe list`:**
   *   Flags: `--status <ready|needs_clarification>`, `--lane <ASDD|FTD>`, `--project <slug>`, `--risk <NONE|BAJO|MEDIO|ALTO|CRITICO>`, `--json`.
   *   Behavior: Scans `workitems/` and `quarantine/` sorted deterministically by `updated_at DESC, id ASC`, always serializing an empty result as `[]` (never `null`) in `--json` mode.
4. **`intake.exe show`:**
   *   Flags: `--id <WI-ID>` (or positional `<WI-ID>`), `--format <json|md>` (default `json` when `--json` is set, otherwise `md`), `--json`.
   *   Behavior: Renders either the authoritative JSON or an on-demand Markdown view with ASCII/Unicode Box-Drawing summary tables (never Mermaid). Exits `1` if `<WI-ID>` is not found.
5. **`intake.exe verify`:**
   *   Flags: `--repo-root <path>`, `--json`.
   *   Behavior: Ensures `<dataDir>` exists (`os.MkdirAll`), acquires `.intake.lock`, and verifies:
       (a) When inside a Git worktree (`.git` present at `repoRoot`), `1_Intake/runtime/` is ignored by Git (`git check-ignore`).
       (b) Zero `WorkItem` IDs or non-empty `dedupe_key` values exist simultaneously in `workitems/` and `quarantine/`.
       (c) Every persisted `.json` file conforms to `schema_version: "1.0"`, matches its filename `WI-<dedupe_key[:12]>` (or `WI-<sanitized_payload_sha256[:12]>` when `dedupe_key == ""` in `quarantine/`), and passes `bfcore.ScanG4All` across all 11 `G4` rules with zero violations.
       (d) Every line in `intake_events.jsonl` (if the file exists) is valid JSON and passes `bfcore.ScanG4All`.
       (e) Read-only `0_Brain` preflight succeeds and `0_Brain/brain.exe context --vault <vault> --files README.md --json` returns valid risk-zone metadata without parse errors.
       Appends a `VERIFIED` event to `intake_events.jsonl`, prints a structured `VerifyReport` JSON (`{ok, repo_root, data_dir, vault_path, git_ignored, workitems_checked, quarantine_checked, events_checked, brain_ok, errors}`), and exits `0` if `ok == true` or `1` if `ok == false`.

## Alternatives Considered

### Alternative A: Direct Go Module Import of `blackfactory/brain` (`0_Brain/engine`) vs. Duplicating Code with AST Scraping (`parity_test.go`)

*   **Pros**: Importing `blackfactory/brain` directly shares `ScanG4` and `MatchCoverPath` in-process; duplicating the code into `1_Intake` avoids importing `modernc.org/sqlite v1.62.0`.
*   **Cons**: Importing `blackfactory/brain` pulls `modernc.org/sqlite` (~12 MB) into `intake.exe` and couples Team 1 to Team 0's SQLite driver; conversely, copying 6 files into `1_Intake` and using `go/ast` scraping (`parity_test.go`) against `../../0_Brain/engine/validator.go` is brittle and breaks whenever `0_Brain` refactors internal symbols or when `MatchCoverPath` requires case-insensitive folding on Windows.
*   **Reason for Rejection**: Rejected in favor of extracting the SQLite-free shared primitives (`g4.go`, `paths.go`, `lock_*`, `reparse_*`) into `pkg/bfcore` (`module blackfactory/pkg/bfcore`, `CGO_ENABLED=0`, permitting only `golang.org/x/sys`), imported by both `0_Brain/engine` and `1_Intake/engine`.

### Alternative B: Emitting Three Separate Methodological Lanes (`ASDD`, `SDD`, and `FTD`)

*   **Pros**: Mirrors the historical terminology (`ASDD`/`SDD`, `FTD`, `TDD`) referenced in `README.md:7` and `0_Brain/schemas/constitution.template.md:25`.
*   **Cons**: At intake time—before `2_Spec_Writer` has produced a specification—there is no deterministic observable boundary separating `ASDD` from `SDD`, leaving no deterministic criterion to separate the two. Meanwhile, `TDD` is a build-time verification discipline in `3_Builder`, not an intake routing lane.
*   **Reason for Rejection**: Rejected by author: `1_Intake` emits two mutually exclusive lanes (`ASDD` unifying `ASDD`/`SDD`, and `FTD` for bounded single-module maintenance), while `TDD` remains mandatory across both lanes in `3_Builder`.

### Alternative C: Uniform Fail-Closed G4 Policy Across All 11 Rules (No Tier 2 Redaction)

*   **Pros**: Matches `0_Brain`'s vault validator (`ScanG4`), which treats all 11 `G4` rules identically as blocking violations; requires zero redaction logic.
*   **Cons**: Windows crash dumps, Go compiler diagnostics, and user bug reports frequently contain local profile paths (`C:\Users\<user>\...`, `/home/<user>/...`) or commit author emails. Blocking all 11 rules unconditionally at the perimeter prevents real stack traces from ever being ingested into Black Factory.
*   **Reason for Rejection**: Rejected by author: `1_Intake` enforces a two-tier `G4` perimeter (8 Tier 1 hard-secret rules fail-closed on both raw bytes and decoded JSON strings with exit code `3` and `0` payload files written; 3 Tier 2 environmental rules redacted via fixed-point delimiter-preserving replacement on `title`/`description`/`note`, or quarantined on `affected_paths` and metadata), with `--strict-g4` available to promote Tier 2 to fail-closed when required.

### Alternative D: Building a Shadow Markdown Table Parser for `wiki/risk_zones.md` Inside `1_Intake`

*   **Pros**: Avoids modifying `0_Brain/engine/query.go` during Team 1 implementation.
*   **Cons**: Both `0_Brain/vault/wiki/risk_zones.md:11` and `0_Brain/schemas/risk_zones.template.md:11` declare `covers_paths: []` in YAML frontmatter and store the actual path patterns (`GengiSWAP/engine/`, `0_Brain/engine/`) and severity levels (`CRITICO`, `ALTO`) in the `## Catálogo de Zonas de Riesgo` Markdown table (`risk_zones.md:35-38`). Parsing `0_Brain`'s internal Spanish Markdown tables directly from `1_Intake` violates `0_Brain` encapsulation, breaks Phase 2 MCP transport migration, and leaves the bug unfixed for `6_Filtro_de_Riesgo`.
*   **Reason for Rejection**: Rejected in favor of fixing the root cause in `0_Brain` (Phase 0): `0_Brain/engine` parses `## Catálogo de Zonas de Riesgo` using case-insensitive `bfcore.MatchCoverPathFold` and emits structured `risk_hits` and `risk_zones_sha256` directly in `brain.exe context --json`.

### Alternative E: Unconditional `ASDD` Routing for All Risk-Zone Levels (`CRITICO` and `ALTO` Alike)

*   **Pros**: Simplifies risk-zone triage to a binary check (any match in `wiki/risk_zones.md` forces `ASDD`).
*   **Cons**: In `0_Brain/vault/wiki/risk_zones.md:37-38`, `RZ-001` (`GengiSWAP/engine/`) has level `CRITICO` (requiring PR + `4_Revisores` + `[AUTOR]` approval), whereas `RZ-002` (`0_Brain/engine/`) has level `ALTO` (requiring `go vet`, `go test -count=1 ./...`, and `G4`/`G5` review). Routing every `ALTO` hit unconditionally to `ASDD` makes `FTD` (`correccion_hallazgos`) unusable for localized bug or audit fixes inside `0_Brain/engine/`.
*   **Reason for Rejection**: Rejected by author: `CRITICO` hits unconditionally force `ASDD` and `requires_autor_hitl: true`, whereas `ALTO` (as well as `MEDIO` and `BAJO`) hits force `ASDD` for `feature`/`ops` or when `impact_flags` are present, but allow `FTD` for bounded single-module `bug`/`audit_fix`/`refactor` requests while attaching `risk_level` and `risk_hits` for downstream enforcement.

### Alternative F: Persisting Derived Markdown Views (`WI-<hash12>.md`) or Dual Logs (`intake_log.md`) Alongside JSON

*   **Pros**: Allows operators browsing `1_Intake/runtime/` in a file manager to open `.md` files directly without running `intake.exe show`.
*   **Cons**: Persisting two files per `WorkItem` across `quarantine/` and `workitems/` plus two log files (`intake_log.md` and `intake_events.jsonl`) doubles file I/O during atomic promotion and creates split-brain risk if an operator manually edits `WI-<hash12>.md` expecting `intake.exe resolve` or `2_Spec_Writer` to read those edits.
*   **Reason for Rejection**: Rejected by author: `WI-<hash12>.json` and `intake_events.jsonl` are the sole persisted runtime artifacts on disk; human-readable Markdown views are rendered exclusively on demand via `intake.exe show --id <WI-ID> --format md`.

## Non-Functional Requirements

*   **Security & Privacy (`G3`, `G4`, Option i):**
    *   *Zero-Secret Persistence:* Tier 1 `G4` failures (checked on both raw input bytes and decoded JSON string values) never create files in `workitems/` or `quarantine/`. The forensic event written to `intake_events.jsonl` and `stderr` records only `{timestamp, trace_id, event: "REJECTED_G4", rule_name, field, line, raw_sha256, source, source_ref}` (with `source` and `source_ref` sanitized via `bfcore.RedactAllG4`) and never echoes the matched substring or raw line text.
    *   *Pre-Write `ScanG4All` Barrier & Subprocess Hygiene:* Every serialized JSON buffer (`WorkItem` or `IntakeEvent`) must pass `bfcore.ScanG4All` with zero findings in memory before being written to disk. `BrainClient` invokes `0_Brain/brain.exe` only after Stage 1 and Stage 2 succeed, ensuring raw secrets or personal paths never appear in Windows process creation command lines.
    *   *Ancestor-Chain Reparse-Point and Path-Traversal Protection:* All file reads (`--input`, `0_Brain/vault/`) and runtime directory operations validate the full ancestor directory chain via `os.Lstat` (`bfcore.CheckNoReparseChain`) to reject NTFS symlinks, junctions, and mount points, and reject any `affected_paths` entry containing `..`, `:`, `\\`, `\\?\`, leading `/`, Windows reserved device names, trailing `/`, or globs before calling `path.Clean`.
    *   *Option i Git Isolation:* `.gitignore` excludes `1_Intake/runtime/` and `1_Intake/intake.exe`. `intake.exe verify` executes `git check-ignore` against the active runtime path when inside a Git repository and exits with code `1` if `1_Intake/runtime/` is tracked or not ignored by Git.
*   **Performance & Latency (`< 500 ms` p95 on Windows):**
    *   *Subprocess Budget:* `intake.exe ingest` spawns at most 2 `0_Brain/brain.exe` subprocesses (`context --json` and `query --json`, ~20–45 ms each on a 60-document SQLite FTS5 index), avoiding any `brain.exe verify` spawn during `ingest`/`resolve` by checking `.index/brain.db` and `.brain-journal.json` directly via read-only `os.Lstat` in-process (< 1 ms).
    *   *`O(1)` Deduplication Lookup:* Because ready and quarantined items are named deterministically as `WI-<hash12>.json`, idempotency lookup checks `os.Lstat` on `workitems/WI-<hash12>.json` and `quarantine/WI-<hash12>.json` in `O(1)` time without scanning the directory, verifying the full 64-hex `dedupe_key` (or `sanitized_payload_sha256`) on hit.
*   **Concurrency, Rollout & Rollback:**
    *   *Kernel Locking & Atomic Promotion:* Every disk write to `1_Intake/runtime/` (across Stages 1 through 5, including `REJECTED_G4` events and `quarantine/` writes) acquires an exclusive OS kernel lock on `1_Intake/runtime/.intake.lock` via `pkg/bfcore` (`windows.LockFileEx` on Windows; `syscall.Flock` on POSIX) with a bounded retry loop (up to `250 ms` total backoff in `25 ms` intervals before returning `ErrIntakeLocked` -> exit code `4`), and commits JSON files via same-directory temporary file write + atomic rename retry loop (`replaceFileAtomic`).
    *   *Schema Versioning:* Every `WorkItem` carries `"schema_version": "1.0"` and `"triage_reasons[].ruleset_version": "triage-det-v1.0"`. Additive minor fields preserve backward compatibility; unrecognized major `schema_version` values abort `intake.exe verify` and `resolve` with exit code `1` without modifying existing files.

## Additional Resources

*   [`README.md`](../../README.md) — Black Factory 7-team blueprint and global invariants `G1–G6`.
*   [`1_Intake/README.md`](../../1_Intake/README.md) — Initial scope definition for Team 1.
*   [`0_Brain/README.md`](../../0_Brain/README.md), [`0_Brain/engine/README.md`](../../0_Brain/engine/README.md), and [`0_Brain/engine/cmd/brain/README.md`](../../0_Brain/engine/cmd/brain/README.md) — `0_Brain` engine architecture, `g4Rules`, `MatchCoverPath`, and `brain.exe` CLI contract.
*   [`Docs/BLACK_FACTORY_BRAIN.md`](../BLACK_FACTORY_BRAIN.md) — Option 3C hybrid architecture and Option i privacy model.
*   [`GengiSWAP/README.md`](../../GengiSWAP/README.md) and [`GengiSWAP/engine/README.md`](../../GengiSWAP/engine/README.md) — Downstream deterministic execution ring and canonical task templates.

## Future Improvements

*   **Phase 2 MCP Transport (`BrainClient` over `brain.exe mcp`):** Replace per-command `os/exec` subprocess spawns in `BrainClient` with a persistent stdio/JSON-RPC client once `0_Brain` implements the reserved `brain.exe mcp` subcommand (`0_Brain/engine/cmd/brain/main.go:114-116`).
*   **Phase 2 External Ingestion Adapters & Markdown+YAML Input:** Add structured adapters for GitHub Issues JSON payloads, Sentry event webhooks, and Markdown+YAML frontmatter files without altering Stages 2–5 of the triage engine.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

### Phase 0: Shared Core Extraction (`pkg/bfcore`) and `0_Brain` Root-Cause Alignment

#### 1. [`go.work`](../../go.work) and [`pkg/bfcore/go.mod`](../../pkg/bfcore/go.mod) (Create)

*   **Type / Scope**: Go Workspace & Shared SQLite-Free Core Module (`module blackfactory/pkg/bfcore`)
*   **Proposed Changes**:
    *   Create [`go.work`](../../go.work) at the repository root declaring `go 1.26.0` and linking `./pkg/bfcore`, `./0_Brain/engine`, and `./1_Intake/engine`.
    *   Create [`pkg/bfcore/go.mod`](../../pkg/bfcore/go.mod) declaring `module blackfactory/pkg/bfcore`, targeting `go 1.26.0`, and requiring only `golang.org/x/sys v0.49.0` (matching [`0_Brain/engine/go.mod`](../../0_Brain/engine/go.mod#L13)) with zero SQLite dependencies.
*   **Rationale**: Allows `0_Brain/engine` and `1_Intake/engine` to share compiled `G4`, path-glob, kernel-lock, and reparse-point primitives without pulling `modernc.org/sqlite` into `1_Intake/intake.exe` or maintaining fragile AST-scraping parity tests.

#### 2. [`pkg/bfcore/g4.go`](../../pkg/bfcore/g4.go), [`pkg/bfcore/paths.go`](../../pkg/bfcore/paths.go), [`pkg/bfcore/lock_windows.go`](../../pkg/bfcore/lock_windows.go), [`pkg/bfcore/lock_other.go`](../../pkg/bfcore/lock_other.go), [`pkg/bfcore/reparse_windows.go`](../../pkg/bfcore/reparse_windows.go), [`pkg/bfcore/reparse_other.go`](../../pkg/bfcore/reparse_other.go), [`pkg/bfcore/bfcore_test.go`](../../pkg/bfcore/bfcore_test.go), and [`pkg/bfcore/README.md`](../../pkg/bfcore/README.md) (Create)

*   **Type / Scope**: Shared Security, Glob, Locking, and Reparse-Point Primitives (`package bfcore`)
*   **Proposed Changes**:
    *   `pkg/bfcore/g4.go`: Move the canonical 11 `G4` rule definitions from [`0_Brain/engine/validator.go:49-105`](../../0_Brain/engine/validator.go#L49-L105), tagging each rule with `G4Tier` (`Tier1HardSecret` for the 8 credential rules; `Tier2Environmental` for `win_user_path`, `unix_user_path`, `email_address`). Export:
        *   `ScanG4All(text string) []G4Finding`: runs all 11 rules in fail-closed mode (suppressing `literal_private_key` on lines where `pem_private_key` matched, per [`0_Brain/engine/validator.go:159-167`](../../0_Brain/engine/validator.go#L159-L167)).
        *   `ScanTier1(field, text string, strictG4 bool) []G4Finding`: scans line-by-line for the 8 Tier 1 rules (plus the 3 Tier 2 rules when `strictG4 == true`) without storing the matched secret substring.
        *   `RedactTier2Text(field, text string) (string, []G4Redaction)`: runs in a fixed-point loop (up to 16 passes until 0 Tier 2 matches remain) on free-text strings, replacing `win_user_path` and `email_address` with `[REDACTED:G4:<rule>]` and replacing the `(?:/home|/Users)/[A-Za-z0-9._-]+` core of `unix_user_path` while preserving leading `(?:^|[\s"'(=:])` and trailing `(?:/|$|\s|["'])` delimiters so adjacent paths (`/home/alice /home/bob`) are both redacted.
        *   `RedactAllG4(text string) string`: redacts any Tier 1 or Tier 2 match to `[REDACTED:G4:<rule>]` for forensic metadata fields (`source`, `source_ref` in `REJECTED_G4`).
    *   `pkg/bfcore/paths.go`: Export `NormalizeCoverPath(p string) string`, `MatchCoverPath(pattern, filePath string) bool` (from [`0_Brain/engine/index.go:32-93`](../../0_Brain/engine/index.go#L32-L93)), and `MatchCoverPathFold(pattern, filePath string) bool` (applying `strings.ToLower` to both arguments before matching for Windows case-insensitivity).
    *   `pkg/bfcore/lock_windows.go` & `lock_other.go`: Export `AcquireFileLock(lockPath string, maxWait time.Duration) (*FileLock, error)` and `(*FileLock).Unlock() error` using `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)` + `windows.LockFileEx` (`windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY`) on Windows and `syscall.Flock(LOCK_EX | LOCK_NB)` on POSIX, retrying every `25 ms` up to `maxWait` (default `250 ms` in `1_Intake`) before returning `ErrLockBusy`, and never unlinking `lockPath` on unlock.
    *   `pkg/bfcore/reparse_windows.go` & `reparse_other.go`: Export `IsReparseOrSymlink(fi os.FileInfo) bool` (checking `os.ModeSymlink | os.ModeIrregular` and `windows.FILE_ATTRIBUTE_REPARSE_POINT = 0x00000400`) and `CheckNoReparseChain(rootPath, targetPath string) error`, which walks every ancestor component from `rootPath` down to `targetPath` using `os.Lstat` and rejects any symlink, NTFS junction, or mount point.
    *   `pkg/bfcore/bfcore_test.go` & `pkg/bfcore/README.md`: Unit tests for all 11 G4 rules, fixed-point adjacent `unix_user_path` redaction, `MatchCoverPathFold`, lock contention backoff, and ancestor reparse-point rejection, plus package README.
*   **Rationale**: Establishes a single tested source of truth for security and filesystem primitives across Team 0 and Team 1.

#### 3. [`0_Brain/engine/go.mod`](../../0_Brain/engine/go.mod), [`0_Brain/engine/validator.go`](../../0_Brain/engine/validator.go), [`0_Brain/engine/index.go`](../../0_Brain/engine/index.go), [`0_Brain/engine/query.go`](../../0_Brain/engine/query.go), and [`0_Brain/engine/brain_test.go`](../../0_Brain/engine/brain_test.go) (Modify)

*   **Type / Scope**: `0_Brain` Root-Cause Alignment for Structured Risk Zones & Non-Null JSON Slices (`package brain`)
*   **Proposed Changes**:
    *   Update `0_Brain/engine/go.mod` to reference `blackfactory/pkg/bfcore` via local `replace ../../pkg/bfcore`, and delegate `ScanG4`, `NormalizeCoverPath`, and `MatchCoverPath` in `validator.go` and `index.go` to `bfcore` (preserving existing exported signatures in `package brain`).
    *   In [`0_Brain/engine/query.go`](../../0_Brain/engine/query.go):
        *   Initialize result slices as `results := make([]SearchResult, 0)` so `brain.exe query --json` and `brain.exe context --json` always serialize empty results as `[]` rather than `null`.
        *   Use `bfcore.MatchCoverPathFold` when matching `covers_paths` in `ContextForFiles` (`query.go:95-105`) so Windows path casing variations never miss coverage.
        *   Add `ParseRiskZonesCatalog(markdown string) ([]RiskZoneEntry, error)` to parse the 6-column table under `## Catálogo de Zonas de Riesgo` in `wiki/risk_zones.md` (normalizing header backticks to match `| ID | Proyecto | Rutas cubiertas (`covers_paths`) | Nivel | Compuerta obligatoria | Evidencia `G5` |` from [`0_Brain/schemas/risk_zones.template.md:35`](../../0_Brain/schemas/risk_zones.template.md#L35), splitting column 3 by `,` and `;`, stripping per-pattern backticks, and normalizing `CRÍTICO` -> `CRITICO` across `CRITICO`, `ALTO`, `MEDIO`, `BAJO`).
        *   Populate `RiskHits []RiskZoneHit` and `RiskZonesSHA256 string` on `SearchResult` for `type == "risk_zones"` documents returned by `ContextForFiles` (matching requested `files` and `project` via `bfcore.MatchCoverPathFold`, and setting `RiskZonesParseError` if the table is missing or malformed).
    *   Ensure `0_Brain/vault/projects/black-factory.md` (`type: project`, `project: black-factory`, `status: active`) and `0_Brain/vault/wiki/brain_engine_g4.md` (`type: wiki`, `project: black-factory`, `status: active`, `covers_paths: ["0_Brain/engine/"]`) are present and indexed in the local vault when running E2E verification, and add unit tests in `0_Brain/engine/brain_test.go` covering `ParseRiskZonesCatalog` and `ContextForFiles` `RiskHits`.
*   **Rationale**: Fixes the `covers_paths: []` risk-zone gap and `null` slice serialization at their source in `0_Brain` so both `1_Intake` and `6_Filtro_de_Riesgo` consume structured risk-zone hits directly from `brain.exe context --json`.

--------------------------------------------------------------------------------

### Phase 1: `1_Intake` Configuration, Schemas, Skills, and Engine Implementation

#### 4. [`.gitignore`](../../.gitignore) (Modify)

*   **Type / Scope**: Git Version Control Configuration (Option i Privacy)
*   **Proposed Changes**: Append `1_Intake/runtime/` and `1_Intake/intake.exe` to the Black Factory private/local artifact exclusion block (adjacent to `0_Brain/vault/` and `0_Brain/brain.exe` at lines 22–27).
*   **Rationale**: Excludes operational `WorkItem` files, quarantine holds, `.intake.lock`, audit events (`intake_events.jsonl`), and the compiled `intake.exe` binary from default Git tracking.

#### 5. [`1_Intake/README.md`](../../1_Intake/README.md) (Modify)

*   **Type / Scope**: Team 1 Module Architecture & Operations Documentation
*   **Proposed Changes**: Replace the 5-line skeleton with the full module specification following [`0_Brain/README.md`](../../0_Brain/README.md): Overview, `Estructura del Módulo` (ASCII/Unicode Box-Drawing tree of `engine`, `schemas`, `skills`, untracked `runtime`), Invariants (`G1–G6`, two-tier `G4`, `.intake.lock`, Option i), Side Effects, `Verification` commands, and `Compilación, Pruebas y Uso CLI Verificado` PowerShell examples (`ingest`, `resolve`, `list`, `show`, `verify`).
*   **Rationale**: Documents the module entry point, architecture, and CLI contracts for Team 1.

#### 6. [`1_Intake/schemas/workitem.schema.json`](../../1_Intake/schemas/workitem.schema.json) (Create)

*   **Type / Scope**: JSON Schema Contract (`schema_version: "1.0"`)
*   **Proposed Changes**:
    *   Define the strict JSON Schema (Draft 2020-12) for `WorkItem` v1.0 with `additionalProperties: false` and required top-level properties.
    *   Define closed enumerations:
        *   `status`: `"ready"` | `"needs_clarification"`
        *   `kind`: `"bug"` | `"feature"` | `"refactor"` | `"audit_fix"` | `"ops"`
        *   `lane`: `"ASDD"` | `"FTD"` | `""`
        *   `paths_confidence`: `"explicit"` | `"none"`
        *   `impact_flags[]`: `"contract"` | `"schema"` | `"db"` | `"auth"` | `"security"` | `"governance"`
        *   `risk_level`: `"NONE"` | `"BAJO"` | `"MEDIO"` | `"ALTO"` | `"CRITICO"`
        *   `suggested_template`: `"modulo_nuevo"` | `"correccion_hallazgos"` | `"auditoria_e2e"` | `"mapeo_estatico"` | `""`
    *   Include an `if`/`then`/`else` constraint requiring `lane` in `{"ASDD", "FTD"}`, `suggested_template != ""`, `dedupe_key` matching `^[0-9a-f]{64}$`, and `quarantine_info == null` when `status == "ready"`, and requiring `lane == ""`, `suggested_template == ""`, and `quarantine_info != null` when `status == "needs_clarification"`.
    *   Define nested object schemas for `risk_hits`, `triage_reasons`, `sanitization`, `brain_evidence`, and `quarantine_info`.
*   **Rationale**: Provides a versioned, language-agnostic contract consumed by `1_Intake` validation and downstream teams (`2_Spec_Writer`, `3_Builder`, `4_Revisores`, and `6_Filtro_de_Riesgo`).

#### 7. [`1_Intake/skills/README.md`](../../1_Intake/skills/README.md) and [`1_Intake/skills/intake-triage/SKILL.md`](../../1_Intake/skills/intake-triage/SKILL.md) (Create)

*   **Type / Scope**: Progressive-Load Agent Skill Catalog & Operational Guide
*   **Proposed Changes**:
    *   Create `1_Intake/skills/README.md` documenting the progressive-load skill catalog for Team 1 (`intake-triage`), aligned with [`0_Brain/skills/README.md`](../../0_Brain/skills/README.md).
    *   Create `1_Intake/skills/intake-triage/SKILL.md` with YAML frontmatter (`name: intake-triage`, `description`, `version: "1.0"`, `owner: 1_Intake`) and step-by-step instructions for invoking `1_Intake/intake.exe` (`ingest`, `resolve`, `list`, `show`, `verify`), interpreting exit codes `0..5`, and complying with `G1–G6` and Option i privacy.
*   **Rationale**: Provides explicit CLI flag contracts and exit-code handling so agents run deterministic intake and quarantine resolution without bypassing `0_Brain`.

#### 8. [`1_Intake/engine/go.mod`](../../1_Intake/engine/go.mod) and [`1_Intake/engine/README.md`](../../1_Intake/engine/README.md) (Create)

*   **Type / Scope**: Go Module Definition & Package Technical Documentation
*   **Proposed Changes**:
    *   Declare `module blackfactory/intake` targeting `go 1.26.0` in `1_Intake/engine/go.mod`, requiring `blackfactory/pkg/bfcore v0.0.0` (with `replace blackfactory/pkg/bfcore => ../../pkg/bfcore`) and `golang.org/x/sys v0.49.0`, with zero SQLite dependencies.
    *   Create `1_Intake/engine/README.md` documenting package `intake` overview, exported `[file.go:Symbol]` interface index, link to [`./cmd/intake/README.md`](./cmd/intake/README.md), package invariants, filesystem/process side effects, and `go test -v -count=1 -cover ./...` verification command, matching [`0_Brain/engine/README.md`](../../0_Brain/engine/README.md).
*   **Rationale**: Enforces a pure-Go (`CGO_ENABLED=0`) SQLite-free build sharing `pkg/bfcore` while maintaining documentation symmetry across all Black Factory Go engines.

#### 9. [`1_Intake/engine/model.go`](../../1_Intake/engine/model.go) (Create)

*   **Type / Scope**: Go Domain Model, Canonical Hashing & Schema Validator (`package intake`)
*   **Proposed Changes**:
    *   Define constants: `SchemaVersion = "1.0"`, `TriageDetRulesetVersion = "triage-det-v1.0"`, `MaxInputBytes = 1 << 20` (1 MiB), `DefaultMaxFTDFiles = 5`.
    *   Define closed enum types and validation sets for `WorkItemStatus` (`ready`, `needs_clarification`), `WorkKind` (`bug`, `feature`, `refactor`, `audit_fix`, `ops`), `Lane` (`ASDD`, `FTD`, `""`), `RiskLevel` (`NONE`, `BAJO`, `MEDIO`, `ALTO`, `CRITICO` with integer severity rank `0..4`), `ImpactFlag` (`contract`, `schema`, `db`, `auth`, `security`, `governance`), and `SuggestedTemplate` (`modulo_nuevo`, `correccion_hallazgos`, `auditoria_e2e`, `mapeo_estatico`, `""`).
    *   Define structs with explicit JSON tags:
        *   `RawIntake`: `{Project, Title, Description, Note, Kind, Source, SourceRef string; AffectedPaths, ImpactFlags []string}`.
        *   `WorkItem`, `SanitizationReport`, `G4Redaction` (`bfcore.G4Redaction`), `BrainEvidence`, `BrainInvocation`, `BrainDocMatch` (`{ID, RelPath, Type, Status, Project, VerifiedHead string; Transversal bool; CoversPaths, MatchedPaths []string; RiskHits []RiskHit}`), `RiskHit` (`{ID, Project, Level string; MatchedPaths []string; MandatoryGate string}`), `TriageReason` (`{Code, RuleKind, RulesetVersion, Detail string}`), `QuarantineInfo` (`{ReasonCodes, MissingFields []string; InvalidPathReason, Detail, ResolveHint string}`), `IntakeEvent`, and `VerifyReport` (`{OK bool; RepoRoot, DataDir, VaultPath string; GitIgnored, BrainOK bool; WorkItemsChecked, QuarantineChecked, EventsChecked int; Errors []string}`).
    *   Implement `ComputeSanitizedSHA256(in RawIntake, cleanPaths, sortedFlags []string) string` using deterministic compact JSON serialization of `CanonicalPayload`.
    *   Implement `ComputeDedupeKey(project, source, sourceRef string) string` (returning `""` if any of the 3 trimmed inputs is empty; otherwise returning length-prefixed SHA-256 hex `fmt.Sprintf("%d:%s|%d:%s|%d:%s", len(p), p, len(s), s, len(r), r)` over lowercased trimmed inputs).
    *   Implement `FormatWorkItemID(dedupeKey, payloadSHA256 string) string`: returns `"WI-" + dedupeKey[:12]` when `dedupeKey != ""`, else `"WI-" + payloadSHA256[:12]`.
    *   Implement `ValidateWorkItem(wi *WorkItem) []string` checking all schema invariants and verifying `bfcore.ScanG4All` returns `0` findings on the marshaled JSON bytes.
*   **Rationale**: Centralizes canonical data types, deterministic hashing, and schema validation in a single file with zero I/O dependencies.

#### 10. [`1_Intake/engine/input.go`](../../1_Intake/engine/input.go) (Create)

*   **Type / Scope**: JSON Input Boundary, Quota/BOM Enforcement & CLI Flag Merger (`package intake`)
*   **Proposed Changes**:
    *   Implement `ReadRawInput(inputPath, repoRoot string, stdin io.Reader) ([]byte, error)`:
        *   If `inputPath == ""` (CLI flags only), returns `nil, nil`.
        *   If `inputPath == "-"`, reads from `stdin` via `io.LimitReader(stdin, MaxInputBytes+1)`, returning `ErrInputTooLarge` (`Exit 1`) if `> MaxInputBytes`.
        *   Otherwise verifies `bfcore.CheckNoReparseChain(repoRoot, inputPath)`, checks file size `<= MaxInputBytes`, reads the file bytes, and strips any leading UTF-8 BOM (`\xEF\xBB\xBF`).
    *   Implement `ScanRawAndFlagsTier1(raw []byte, cliOverrides RawIntake, strictG4 bool) []bfcore.G4Finding`: scans `raw` (with `field = "raw_input"`) and each non-empty string/slice field in `cliOverrides` (with `field` set to the exact CLI flag name, e.g. `"--title"`, `"--source-ref"`) using `bfcore.ScanTier1`.
    *   Implement `ParseJSONIntake(raw []byte) (RawIntake, error)`: strips UTF-8 BOM, decodes JSON using `json.NewDecoder` with `DisallowUnknownFields()`, and accepts either a JSON array `["a", "b"]` or a comma-separated string `"a,b"` for `affected_paths` and `impact_flags`.
    *   Implement `MergeCLIOverrides(base RawIntake, overrides RawIntake) RawIntake` and `ScanDecodedTier1(in RawIntake, strictG4 bool) []bfcore.G4Finding` (scanning every decoded field of the merged `RawIntake` with `bfcore.ScanTier1` to block JSON-escaped secrets such as `\u0067hp_...`).
*   **Rationale**: Enforces the `1 MiB` input quota, ancestor reparse-point check, UTF-8 BOM handling, and dual-phase Tier 1 G4 scanning on JSON and CLI inputs without speculative v2 parsers.

#### 11. [`1_Intake/engine/paths.go`](../../1_Intake/engine/paths.go) (Create)

*   **Type / Scope**: Pre-Clean Path Hygiene, G4 Path Guard & Root Module Counting (`package intake`)
*   **Proposed Changes**:
    *   Implement `SanitizeAffectedPaths(rawPaths []string, repoRoot string) (cleanPaths []string, invalidReason string)`:
        *   Before calling `path.Clean`, inspects each raw path:
            1. Checks against Tier 2 personal path regexes (`win_user_path`, `unix_user_path`). If matched, discards all raw paths in memory and returns `nil, "G4_personal_path_in_affected_paths"`.
            2. If `repoRoot != ""` and the path is an absolute path inside `repoRoot` (compared case-insensitively on Windows after normalizing `\` -> `/`), strips the `repoRoot` prefix.
            3. Rejects any path that is empty, starts with `/` or `\`, starts with `\\` (UNC) or `\\?\` (NT device prefix), contains `:` (drive letter or relative drive `C:file.go`), contains control/null bytes, contains `..` as a path segment, ends with `/` or `\` (directory path), contains glob metacharacters (`*`, `?`, `[`), or has any segment matching a Windows reserved device name (`CON`, `PRN`, `AUX`, `NUL`, `COM1..9`, `LPT1..9`, case-insensitively with or without extension), returning `nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"`.
        *   Normalizes valid relative file paths (`\` -> `/`, trims leading `./`, applies `path.Clean`, and deduplicates while preserving order). Both `"G4_personal_path_in_affected_paths"` and `"PATH_TRAVERSAL_OR_INVALID_SCOPE"` map to Priority Rule 4 (`INVALID_AFFECTED_PATH_G4`).
    *   Implement `CountRootModules(paths []string) []string`: extracts distinct top-level directory segments (e.g., `"0_Brain"`, `"GengiSWAP"`, `"1_Intake"`, mapping root-level files without a `/` such as `"README.md"` to module `"."`) for the multi-module threshold rule (`MULTI_MODULE_OR_FILE_THRESHOLD`).
*   **Rationale**: Prevents path traversal, directory/glob threshold evasion, and personal path leakage before `path.Clean` can mask offending segments.

#### 12. [`1_Intake/engine/brain_client.go`](../../1_Intake/engine/brain_client.go) (Create)

*   **Type / Scope**: Read-Only `0_Brain` Subprocess Client & Disjunctive FTS5 Query Builder (`package intake`)
*   **Proposed Changes**:
    *   Define `BrainClient` interface (`Preflight`, `Context`, `Query`) and `SubprocessBrainClient` (with configurable subprocess timeout, defaulting to `5s`, and binary path `brainBin`).
    *   Implement `Preflight(vaultRoot, dbPath string) (BrainPreflight, error)`:
        *   Calls `bfcore.CheckNoReparseChain` on `vaultRoot` and `dbPath`.
        *   Calls `os.Lstat` on `dbPath` (`.index/brain.db`). If missing, returns `ErrBrainIndexMissing` (wrapped in `ErrBrainUnavailable`, exit code `4`).
        *   Calls `os.Lstat` on `<vaultRoot>/.brain-journal.json`. If `.brain-journal.json` exists, returns `ErrBrainUnavailable` immediately (`exit code 4`) without invoking `brain.exe verify` (preserving `NG2`).
    *   Implement `ExtractQueryTokens(title, description string, maxTokens int) []string` (default `maxTokens = 5`, matching paths, `*.go`/`*.md`/`*.json`/`*.sql` filenames, `RZ-\d+`, `ADR-\d+`, `G[1-6]`, `CamelCase`/`snake_case` symbols, and words `>= 4` chars, stripping Spanish/English stop-words, FTS5 boolean operators `AND`/`OR`/`NOT`/`NEAR`, and non-token punctuation) and `BuildDisjunctiveFTSQuery(tokens []string) string` (quoting each token `"\"<t>\""` and joining with `" OR "`).
    *   Implement `Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error)` and `Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error)`:
        *   Invoke `brain.exe context --vault <vaultRoot> --db <dbPath> --project <project> --files <csv> --json` and `brain.exe query --vault <vaultRoot> --db <dbPath> --project <project> --q <orQuery> --json`.
        *   Capture literal command strings, exit codes, and `duration_ms` (`G1`), decode JSON handling both `null` and `[]` slices, and map missing binary, timeout, or non-zero exit code to `ErrBrainUnavailable` (`exit code 4`).
*   **Rationale**: Satisfies mandatory Read-Before-Act consultation with `0_Brain` within `<= 2` subprocesses and `< 500 ms` p95 while strictly respecting `NG2` read-only isolation.

#### 13. [`1_Intake/engine/triage.go`](../../1_Intake/engine/triage.go) (Create)

*   **Type / Scope**: 100% Deterministic Locatability & Lane Triage Engine (`package intake`)
*   **Proposed Changes**:
    *   Implement `EvaluateMetadataAndScope(raw RawIntake, cleanPaths []string, invalidPathReason string, metadataHasTier2 bool) *QuarantineInfo`: evaluates Priority Rules 3–5 (`MISSING_REQUIRED_FIELDS`, `INVALID_AFFECTED_PATH_G4`, `UNCERTAIN_SCOPE`) and constructs the `QuarantineInfo` struct with an actionable `resolve_hint` CLI command.
    *   Implement `EvaluateProjectRegistration(raw RawIntake, projectActive bool) *QuarantineInfo`: evaluates Priority Rule 6 (`UNREGISTERED_PROJECT`) when `0_Brain` has no `type: "project"` document for `raw.Project` with `status == "active"`.
    *   Implement `TriageWorkItem(wi *WorkItem, contextResp BrainContextResponse, maxFTDFiles int)`:
        *   Aggregates `risk_hits` from `contextResp` (filtering by `strings.EqualFold(hit.Project, wi.Project) || hit.Project == "*"`) to compute `wi.RiskLevel` (`NONE < BAJO < MEDIO < ALTO < CRITICO`).
        *   Evaluates Priority Rules 7–13 in deterministic order:
            *   Rule 7 (`RISK_ZONE_CRITICO`): any hit with `level == "CRITICO"` -> `lane = "ASDD"`, `requires_autor_hitl = true`.
            *   Rule 8 (`RISK_ZONES_UNPARSEABLE_FAILSAFE`): `contextResp.RiskZonesParseError != ""` or zero active `type: "risk_zones"` docs -> `lane = "ASDD"`, `requires_autor_hitl = true`.
            *   Rule 9 (`KIND_REQUIRES_ASDD`): `wi.Kind == "feature" || wi.Kind == "ops"` -> `lane = "ASDD"`.
            *   Rule 10 (`EXPLICIT_IMPACT_FLAGS`): `len(wi.ImpactFlags) > 0` -> `lane = "ASDD"`.
            *   Rule 11 (`MULTI_MODULE_OR_FILE_THRESHOLD`): `len(CountRootModules(wi.AffectedPaths)) > 1 || len(wi.AffectedPaths) > maxFTDFiles` -> `lane = "ASDD"`.
            *   Rule 12 (`NO_BRAIN_COVERAGE`): any path in `wi.AffectedPaths` is not matched (`bfcore.MatchCoverPathFold`) by at least one active document in `contextResp.MatchedDocs` with `type` in `{"wiki", "adr"}` and `status == "active"` -> `lane = "ASDD"`.
            *   Rule 13 (`FTD_ELIGIBLE_BOUNDED_MAINTENANCE`): when Rules 7–12 did not fire -> `lane = "FTD"`.
        *   Assigns `wi.SuggestedTemplate` according to the deterministic precedence rules in Section 5 and populates `wi.TriageReasons`.
*   **Rationale**: Enforces the two-stage Locatability -> Lane Triage separation with zero heuristic false positives and records structured explanations in `quarantine_info` or `triage_reasons[]`.

#### 14. [`1_Intake/engine/store.go`](../../1_Intake/engine/store.go) (Create)

*   **Type / Scope**: Atomic Option i Runtime Store & JSONL Telemetry Logger (`package intake`)
*   **Proposed Changes**:
    *   Implement `Store` managing `<dataDir>/workitems/`, `<dataDir>/quarantine/`, `<dataDir>/intake_events.jsonl`, and `<dataDir>/.intake.lock`:
        *   `withLock(fn func() error) error`: ensures `<dataDir>` exists (`os.MkdirAll`), acquires `<dataDir>/.intake.lock` via `bfcore.AcquireFileLock(lockPath, 250*time.Millisecond)` (mapping `bfcore.ErrLockBusy` to `ErrIntakeLocked`, exit code `4`), executes `fn`, and releases the lock.
        *   `replaceFileAtomic(tmpPath, dstPath string) error`: commits temporary files in the same directory to `dstPath` with bounded retry on Windows `os.ErrPermission` (matching [`0_Brain/engine/index.go:392-405`](../../0_Brain/engine/index.go#L392-L405)).
        *   `SaveTriaged(wi *WorkItem, prevQuarantineID string, update, dryRun bool) (SaveResult, error)`:
            *   Verifies `ValidateWorkItem(wi)` and `bfcore.ScanG4All(jsonBytes)` in memory before acquiring the lock.
            *   If `dryRun == true`, returns the simulated `SaveResult` with zero disk writes.
            *   Under `.intake.lock`, checks `workitems/<wi.ID>.json`: verifies full 64-hex `dedupe_key` (returning `ErrIDCollision` -> `Exit 1` on 48-bit prefix mismatch); if `sanitized_payload_sha256` matches, appends `IDEMPOTENT_HIT` to `intake_events.jsonl` and returns `IdempotentHit: true` (`Exit 0`); if `sanitized_payload_sha256` differs and `!update`, appends `CONFLICT_DEDUPE` and returns `ErrDedupeConflict` (`Exit 5`); if `update`, sets `wi.Revision = existing.Revision + 1` and `wi.CreatedAt = existing.CreatedAt`.
            *   If promoting from `quarantine/` (`prevQuarantineID != ""` or `quarantine/<wi.ID>.json` exists), sets `wi.Revision = prev.Revision + 1`, writes `workitems/<wi.ID>.json` via `replaceFileAtomic`, deletes `quarantine/<prevQuarantineID>.json` (and `quarantine/<wi.ID>.json` if different), and appends `PROMOTED` to `intake_events.jsonl`.
        *   `SaveQuarantined(wi *WorkItem, dryRun bool) (SaveResult, error)`: verifies `bfcore.ScanG4All(jsonBytes)`, checks existing `quarantine/<wi.ID>.json` under `.intake.lock` (preserving `revision` if `sanitized_payload_sha256` is unchanged, or incrementing `revision` if modified), writes `quarantine/<wi.ID>.json` via `replaceFileAtomic`, and appends `QUARANTINED` to `intake_events.jsonl`.
        *   `AppendRejectedG4Event(ev IntakeEvent, dryRun bool) error`: sanitizes `ev.Source` and `ev.SourceRef` with `bfcore.RedactAllG4`, verifies `bfcore.ScanG4All` on the serialized JSONL line, and appends it to `intake_events.jsonl` under `.intake.lock` (skipping disk write if `dryRun`).
        *   `List`, `LoadByID`, `LoadQuarantinedByID`, and `VerifyRuntime(repoRoot, vaultRoot, brainBin string) (VerifyReport, error)`: implements filtered listing (always returning non-nil `[]WorkItem`), ID lookup, and the 5-point `intake.exe verify` integrity check (skipping `git check-ignore` only when `<repoRoot>/.git` does not exist, such as inside isolated `t.TempDir()` unit tests).
*   **Rationale**: Provides atomic file replacement via temp-file write and rename on Windows, `O(1)` deduplication by filename with 64-hex collision verification, and a single machine-queryable JSONL audit log protected by `.intake.lock`.

#### 15. [`1_Intake/engine/render.go`](../../1_Intake/engine/render.go) (Create)

*   **Type / Scope**: On-Demand ASCII/Box-Drawing Markdown Renderer (`package intake`)
*   **Proposed Changes**: Implement `RenderWorkItemMarkdown(wi *WorkItem) string` producing a deterministic Markdown report for `intake.exe show --format md` beginning with `<!-- AUTO-GENERATED DERIVED VIEW: DO NOT EDIT (Source: <id>.json) -->`, an ASCII/Unicode Box-Drawing triage header box, structured tables for `triage_reasons`, `risk_hits`, `sanitization.g4_redactions`, and `brain_evidence.invocations` (`G1`), and actionable `quarantine_info` guidance when `status == "needs_clarification"`.
*   **Rationale**: Delivers human-readable views on demand via `intake.exe show --format md` without violating the no-Mermaid constraint (`G2`) or persisting duplicate `.md` files (`NG5`).

#### 16. [`1_Intake/engine/intake_test.go`](../../1_Intake/engine/intake_test.go) (Create)

*   **Type / Scope**: Unit, Table-Driven & Latency Benchmark Test Suite (`package intake`)
*   **Proposed Changes**: Implement table-driven unit tests on isolated `t.TempDir()` directories (`G3`) covering:
    *   All 8 Tier 1 fail-closed G4 rules on both raw bytes and decoded JSON-escaped strings (`\u0067hp_...`), verifying `pem_private_key` line suppression over `literal_private_key`, `0` files created in `workitems/` or `quarantine/`, and sanitized `REJECTED_G4` events in `intake_events.jsonl`.
    *   All 3 Tier 2 redaction rules on `title`/`description`/`note` (verifying fixed-point redaction on adjacent `unix_user_path` strings `"/home/alice /home/bob"`, quarantine routing when Tier 2 PII appears in metadata fields, and `--strict-g4` fail-closed promotion).
    *   Pre-clean path hygiene in `SanitizeAffectedPaths` (rejecting personal paths, `..`, UNC `\\server\share`, `C:rel.go`, `\\?\`, leading `/`, Windows reserved device names `CON`/`NUL`/`COM1`, trailing `/` directories, and globs `*?[`, routing to `quarantine/` under `INVALID_AFFECTED_PATH_G4` with zero raw path persistence).
    *   Disjunctive FTS5 technical keyword extraction (`ExtractQueryTokens` and `BuildDisjunctiveFTSQuery`) confirming `AND`/`OR`/`NOT`/`NEAR` stripping and `"\"t1\" OR \"t2\""` formatting.
    *   Deterministic triage across all 13 priority rules: `RZ-001` (`CRITICO` -> `ASDD` + `requires_autor_hitl: true`, including mixed-case Windows path `gengiswap/engine/ring.go`), `RZ-002` (`ALTO` -> `FTD` for bounded single-module `bug` with active `wiki`/`adr` coverage, `ASDD` for `feature` or `impact_flags`), `MEDIO`/`BAJO` risk levels, and `RISK_ZONES_UNPARSEABLE_FAILSAFE` fail-safe to `ASDD`.
    *   Unified `WI-<hash12>` idempotency (`dedupe_key` vs `sanitized_payload_sha256` in quarantine), conflict detection (`ErrDedupeConflict` exit `5`), `--update` revision increment, `--dry-run` zero-write invariant, and atomic promotion from `WI-<payloadSHA[:12]>` to `WI-<dedupeKey[:12]>` deleting the old quarantine file.
    *   Latency test/benchmark verifying `< 500 ms` p95 over a 60-document temporary `0_Brain` vault (using a mock `BrainClient` for unit test isolation and running the real subprocess benchmark when `0_Brain/brain.exe` is present or built in `t.TempDir()`).
*   **Rationale**: Verifies every functional and non-functional contract on isolated temporary directories (`G3`).

#### 17. [`1_Intake/engine/cmd/intake/README.md`](../../1_Intake/engine/cmd/intake/README.md), [`1_Intake/engine/cmd/intake/main.go`](../../1_Intake/engine/cmd/intake/main.go), and [`1_Intake/engine/cmd/intake/main_test.go`](../../1_Intake/engine/cmd/intake/main_test.go) (Create)

*   **Type / Scope**: CLI Executable Entry Point, Documentation & E2E Tests (`package main` -> `1_Intake/intake.exe`)
*   **Proposed Changes**:
    *   `1_Intake/engine/cmd/intake/README.md`: Document the `intake.exe` CLI entry point, `[main.go:Symbol]` interface index, subcommands (`ingest`, `resolve`, `list`, `show`, `verify`), flag contracts, exit code mapping (`0..5`), invariants, side effects, and verification command (`go test -v -count=1 -cover ./...`), matching [`0_Brain/engine/cmd/brain/README.md`](../../0_Brain/engine/cmd/brain/README.md).
    *   `1_Intake/engine/cmd/intake/main.go`:
        *   Implement `main()` and `run(args []string, in io.Reader, out, errOut io.Writer, client intake.BrainClient) int` dispatching `ingest`, `resolve`, `list`, `show`, and `verify`.
        *   Implement `reorderFlags` so boolean and value flags work in any position before or after positional arguments.
        *   Implement deterministic workspace root discovery (`discoverRepoRoot` inspecting `--repo-root`, `BF_REPO_ROOT`, `os.Executable()` directory, and current working directory upward to `.git` or `0_Brain` sibling) to resolve default paths for `--vault`, `--data-dir`, and `--brain-bin`.
        *   Enforce the consolidated exit code table (`0..5` defined in Section 6), mapping `ErrIntakeLocked` and `ErrBrainUnavailable` to exit code `4`.
    *   `1_Intake/engine/cmd/intake/main_test.go`: Implement E2E CLI tests (`TestCLI_E2E_FullLifecycle_AllExitCodes`, `TestCLI_VerifyDetectsG4AndQuarantinePromotion`) executing `run(...)` across all 5 subcommands and asserting all 6 exit codes (`0`, `1`, `2`, `3`, `4`, `5`), JSON stdout/stderr envelopes, `--dry-run`, `--update`, on-demand Markdown rendering (`show --format md`), and round-trip compatibility with `0_Brain` vault structures in `t.TempDir()`.
*   **Rationale**: Delivers and verifies the complete command-line interface consumed by operators, `2_Spec_Writer`, and automated scripts without mutating production directories (`G3`).

#### 18. [`README.md`](../../README.md) (Modify)

*   **Type / Scope**: Root Repository Blueprint Documentation
*   **Proposed Changes**:
    *   Update `System Verification` (lines 39–50) to add the `pkg/bfcore` and `1_Intake Engine` test commands (`Set-Location "C:\Black Factory\1_Intake\engine"; go test -count=1 ./...`).
    *   Update `Artefactos privados / locales (ignorados en .gitignore)` (lines 58–61) to include `1_Intake/runtime/` and `1_Intake/intake.exe`.
    *   Add a subsection `### Verificación y Compilación de 1_Intake (intake.exe)` after line 78 (parallel to `### Verificación y Compilación de GengiSWAP (gengis.exe)` at lines 63–78).
*   **Rationale**: Keeps the root repository blueprint synchronized with Team 1's executable and Option i runtime artifacts.

### Verification Plan

*   **Shared Core & `0_Brain` Root-Cause Tests**:
    ```powershell
    Set-Location "C:\Black Factory\pkg\bfcore"
    $env:CGO_ENABLED = "0"
    go vet ./...
    go test -v -count=1 -cover ./...

    Set-Location "C:\Black Factory\0_Brain\engine"
    $env:CGO_ENABLED = "0"
    go vet ./...
    go test -v -count=1 -cover ./...
    go build -o ..\brain.exe .\cmd\brain
    ```
*   **`1_Intake` Compilation/Build**:
    ```powershell
    Set-Location "C:\Black Factory\1_Intake\engine"
    $env:CGO_ENABLED = "0"
    go build -o ..\intake.exe .\cmd\intake
    ```
*   **`1_Intake` Static Analysis & Linting**:
    ```powershell
    Set-Location "C:\Black Factory\1_Intake\engine"
    $env:CGO_ENABLED = "0"
    go vet ./...
    ```
*   **`1_Intake` Unit, Integration & E2E Tests with Coverage**:
    ```powershell
    Set-Location "C:\Black Factory\1_Intake\engine"
    $env:CGO_ENABLED = "0"
    go test -v -count=1 -cover ./...
    ```
*   **Test Coverage & Quantitative Success Criteria**:
    *   `85%+` statement coverage in `blackfactory/pkg/bfcore`, `blackfactory/intake`, and `blackfactory/intake/cmd/intake` packages (verified via `go test -v -count=1 -cover ./...`).
    *   `0` missed detections across all Tier 1 G4 test vectors (both raw and JSON-escaped strings across all 8 rules: `exit 3`, `0` files in `workitems/` or `quarantine/`, payload-free `REJECTED_G4` entry in `intake_events.jsonl`) and fixed-point Tier 2 redaction vectors in `bfcore_test.go` and `intake_test.go`.
    *   `0` missed detections in the `RZ-001` (`CRITICO` -> `ASDD` + `requires_autor_hitl: true`, including case-insensitive Windows paths) and `RZ-002` (`ALTO` -> `FTD` for bounded single-module `bug` with active `wiki`/`adr` coverage, `ASDD` for `feature`/flags) test cases when `0_Brain/vault/wiki/risk_zones.md` declares `covers_paths: []` in frontmatter.
    *   `< 500 ms` p95 local ingestion and triage latency on Windows against a 60-document `0_Brain` vault (verified via the latency benchmark in `1_Intake/engine/intake_test.go`).
*   **E2E CLI & Option i Privacy Verification**:
    ```powershell
    Set-Location "C:\Black Factory"
    & "C:\Black Factory\1_Intake\intake.exe" verify --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\1_Intake\runtime" --repo-root "C:\Black Factory" --json
    git status --ignored -s 1_Intake/
    ```
    Must confirm `ok: true` (exit code `0`) and `!! 1_Intake/intake.exe` / `!! 1_Intake/runtime/` ignored in Git.

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
