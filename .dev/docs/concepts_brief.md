# Black Factory Team 1 (`1_Intake`): Deterministic Request Normalization, G4 Perimeter Gate, and Methodological Lane Triage

* **Author:** blackpusch
* **Date:** 2026-11-22

## Overview

`1_Intake` (`1_Intake/engine` -> `1_Intake/intake.exe`) is the entry gate for Black Factory. It receives software engineering requests and defect reports from the command line or local structured files (JSON or Markdown with YAML frontmatter), enforces a two-tier secret and personal-data boundary (`G4`), queries the persistent memory engine (`0_Brain/brain.exe`) under the Read-Before-Act protocol, and outputs a versioned JSON `WorkItem` assigned either to the full specification lane (`ASDD`), the fast-track maintenance lane (`FTD`), or a clarification hold (`quarantine/`, `G6`).

The target consumers of `1_Intake` are:
1. **`2_Spec_Writer` (Team 2):** consumes triaged `WorkItem` JSON files from `1_Intake/runtime/workitems/`, including attached `0_Brain` context documents, risk-zone hits, and suggested `GengiSWAP` templates (`modulo_nuevo`, `correccion_hallazgos`).
2. **`4_Revisores` (Team 4) and `6_Filtro_de_Riesgo` (Team 6):** consume the `risk_level` (`NONE`, `ALTO`, `CRITICO`), `requires_autor_hitl`, and `risk_hits` recorded during intake to enforce human-in-the-loop (`[AUTOR]`) gates.
3. **Human Operators and Automated Callers (`[AUTOR]`):** submit incidents via `intake.exe ingest`, inspect holds and render on-demand Markdown views via `intake.exe list` / `show`, unblock incomplete requests via `intake.exe resolve`, and audit runtime integrity via `intake.exe verify`.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ 1_Intake CLI (1_Intake/intake.exe: ingest | resolve | list | show | verify)  │
├──────────────────────────────────────────────────────────────────────────────┤
│ [1] Input Adapter (CLI flags | --input <file.json|.md> | --input - [stdin])  │
│        │                                                                     │
│        ▼ (In-memory buffer — 0 payload bytes written prior to Tier 1 pass)   │
│ [2] Two-Tier G4 Perimeter (11 regex rules aligned w/ 0_Brain validator.go)   │
│        ├─ Tier 1 (8 hard-secret rules) ──────► FAIL-CLOSED (Exit 3, 0 files, │
│        │                                       payload-free event in JSONL)  │
│        ├─ Personal path in affected_paths ───► Discard path -> quarantine/   │
│        └─ Tier 2 (3 path/email rules on parsed title/description/note;       │
│                   --strict-g4 -> Exit 3) ────► Subgroup Redaction & Hash     │
│        │                                                                     │
│        ▼                                                                     │
│ [3] Metadata & Locatability Gate (G5 / G6)                                   │
│        ├─ Missing required fields / unknown project ──► quarantine/ (Exit 2) │
│        └─ Omitted paths + 0_Brain cannot locate scope ► quarantine/ (Exit 2) │
│        │                                                                     │
│        ▼                                                                     │
│ [4] 0_Brain Consultation (BrainClient -> 0_Brain/brain.exe)                  │
│        ├─ Preflight: missing brain.db or pending .brain-journal.json         │
│        │  triggers brain.exe verify --json (Exit 3/4 -> STOP Exit 4)         │
│        └─ Dual Risk-Zone Match + Deterministic Rules + Versioned Heuristics  │
│               ├─ ASDD (CRITICO, feature, impact_flags, NO_BRAIN_COVERAGE,    │
│               │        >1 root module, >5 files, inferred paths, heuristics) │
│               └─ FTD  (explicit single-module bug/audit_fix/refactor with    │
│                        active 0_Brain wiki/ADR coverage and no CRITICO hit)  │
│        │                                                                     │
│        ▼ (Kernel Lock .intake.lock + Atomic Temp-Write/Rename)               │
│ [5] Option i Untracked Runtime (1_Intake/runtime/)                           │
│        ├─ workitems/WI-<hash12>.json   (status: ready; sole source of truth) │
│        ├─ quarantine/WI-<hash12>.json  (status: needs_clarification)         │
│        ├─ intake_log.md                (human-readable append-only log)      │
│        └─ intake_events.jsonl          (machine-queryable audit stream)      │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Analysis

Parallel evaluation across the seven Goldfish lenses (`egm-design`, `egm-codebase-details`, `egm-zoom-out`, `egm-comprehension`, `egm-implementation-readiness`, `egm-structural`, and `egm-inventive-catalyst`) exposed concrete mismatches between the initial concept and the live `0_Brain` codebase (`0_Brain/engine/`), along with structural contradictions and operational risks. All findings have been resolved into deterministic engineering rules below.

### 1. Codebase Reality Audits (`G5` Verified Facts & Approved Resolutions)

* **Fact 1 — `brain.exe query` and `context` exit codes and `null` JSON slices (`0_Brain/engine/cmd/brain/main.go:199-288`, `0_Brain/engine/query.go:53,94`):**
  In the current `0_Brain` CLI, only `cmdVerify` (`main.go:313-323`) and `cmdApply` (`main.go:472-480`) call `AcquireVaultLock` (exit `4`) and `RecoverJournal` (exit `3`). Both `cmdQuery` and `cmdContext` open `.index/brain.db` directly via `openReadOnlyDB` (`0_Brain/engine/query.go:138-156`) with `PRAGMA query_only = ON`, returning exit `1` on missing index files or errors. In addition, empty search results serialize as JSON `null` (`var results []SearchResult` in `query.go:53,94`).
  * *Approved Resolution:* `BrainClient` runs a fast preflight check before `query`/`context`: it verifies that `.index/brain.db` exists and that no pending `.brain-journal.json` exists. If `.brain-journal.json` is present (or unconditionally during `intake verify`), `BrainClient` invokes `brain.exe verify --json`. If `verify` returns exit `3` (degraded journal) or exit `4` (`.brain.lock` busy), or if `.index/brain.db` is missing, `1_Intake` aborts with exit `4` (`G6` stop). Additionally, `BrainClient`'s JSON decoder accepts both `null` and `[]` when unmarshaling `[]SearchResult`.
* **Fact 2 — `0_Brain/vault/wiki/risk_zones.md` has empty frontmatter `covers_paths: []` (`risk_zones.md:11`) and backtick-wrapped table cells (`risk_zones.md:35-38`):**
  Because `ContextForFiles` (`0_Brain/engine/query.go:95-105`) computes `matched_paths` solely against frontmatter `covers_paths` indexed in SQLite (`0_Brain/engine/index.go:254-265`), `brain.exe context --json` currently returns `matched_paths: []` for `brain-risk-zones`. Meanwhile, the Markdown table in `0_Brain/vault/wiki/risk_zones.md:35-38` uses the 6-column header `| ID | Proyecto | Rutas cubiertas (`covers_paths`) | Nivel | Compuerta obligatoria | Evidencia `G5` |` with backtick-wrapped values (for example, `` `RZ-001` | `black-factory` | `GengiSWAP/engine/` | `CRITICO` ``).
  * *Approved Resolution:* `1_Intake` implements dual risk-zone evaluation: it inspects `matched_paths` from `brain.exe context --json` AND parses the `## Catálogo de Zonas de Riesgo` table (stripping inline backticks, splitting multi-path cells, and matching paths via `MatchCoverPath`). If no `type: risk_zones` document exists or the table cannot be parsed, `1_Intake` fails safe to `ASDD` (`requires_autor_hitl: true`).
* **Fact 3 — Registered Project Paradox (`0_Brain/vault/index.md:35` vs. `0_Brain/vault/projects/`):**
  `0_Brain/vault/index.md:35` lists `black-factory` in the `## Proyectos Registrados` table linking to `projects/black-factory.md`, but `0_Brain/vault/projects/black-factory.md` does not exist on disk yet, and `ContextForFiles` (`query.go:97-105,381-388`) returns `black-factory` transversal documents for any arbitrary `--project` string.
  * *Approved Resolution:* `1_Intake` validates `project` registration against `type: project` documents in `0_Brain` first, with a deterministic fallback to parsing the `## Proyectos Registrados` table in `0_Brain/vault/index.md`. Unknown projects route to `quarantine/` (`needs_clarification`, exit code `2`).
* **Fact 4 — Tier 2 G4 Regex Boundary Capture Trap (`0_Brain/engine/validator.go:60-69`) and Personal Paths in `affected_paths`:**
  In `validator.go:67`, the `unix_user_path` pattern `(?:^|[\s"'(=:])(?:/home|/Users)/[A-Za-z0-9._-]+(?:/|$|\s|["')])` captures leading delimiters (`=`, `:`, quotes, whitespace) and trailing slashes/quotes. Also, `validator.go:159-167` suppresses `literal_private_key` on a line when `pem_private_key` already matched that line.
  * *Approved Resolution:* Tier 2 redaction operates exclusively on the **already-parsed in-memory string values** of free-text fields (`title`, `description`, `note`)—never on raw unparsed JSON/YAML bytes—and preserves adjacent delimiters (`=`, `:`, quotes, whitespace) in `unix_user_path` via subgroup replacement. In `affected_paths`, `1_Intake` never inserts `[REDACTED:...]`: if a personal or out-of-repo absolute path is supplied in `affected_paths`, `1_Intake` discards the raw path in memory and routes the item to `quarantine/` (`needs_clarification`, exit `2`) with `invalid_path_reason: "G4_personal_path_in_affected_paths"` (persisting 0 characters of the personal path).
* **Fact 5 — `.gitignore` Option i Gap (`.gitignore:1-31`):**
  `.gitignore` currently excludes `0_Brain/vault/`, `0_Brain/brain.exe`, `*.db`, and `*.exe`, but does not exclude `1_Intake/runtime/`.
  * *Approved Resolution:* Add `1_Intake/runtime/` to `.gitignore` before `intake.exe` creates runtime directories, and enforce a `git check-ignore` check inside `intake verify`.
* **Fact 6 — FTS5 Implicit `AND` and Reserved Operators in `brain.exe query` (`0_Brain/engine/query.go:408-452`):**
  `buildFTS5Query` joins all query tokens with `AND`. Passing a full multi-sentence `title` or `description` to `brain.exe query` produces empty results (`null`) whenever a single word is absent from a vault document, and uppercase FTS5 keywords (`AND`, `OR`, `NOT`, `NEAR`) in user text can alter FTS5 query syntax.
  * *Approved Resolution:* `1_Intake` extracts technical identifiers from `title`/`description` (module names, partial paths, `*.go`/`*.md` filenames, `CamelCase`/`snake_case` symbols, `RZ-NNN`/`ADR-NNNN` codes), removes Spanish/English stop-words, strips/escapes FTS5 reserved operators (`AND`, `OR`, `NOT`, `NEAR`), and queries a bounded set of top 3–5 tokens (joined via explicit `OR` or deduplicated across per-token lookups by document `id`).

### 2. Core Contradictions & Inventive Resolutions (TRIZ / Catalyst Analysis)

| ID | Contradiction | Conflicting Forces | Approved Resolution Pattern |
| :--- | :--- | :--- | :--- |
| **C1** | **Zero-Disk Tier 1 G4 Secret Rejection vs. 3 AM Forensic Auditability** | Tier 1 must create 0 files in `workitems/` or `quarantine/` (`exit 3`, 0 payload bytes on disk), yet operators need an audit trail when automated callers fail on a G4 rule. | **Payload-Free Forensic Telemetry Event:** Create 0 files in `workitems/` or `quarantine/`. Append a payload-free event `{timestamp, event: "REJECTED_G4", rule_name, field, line, raw_sha256, source, source_ref}` (zero secret values or surrounding text) to `1_Intake/runtime/intake_events.jsonl` and emit the same JSON object to `stderr` (`--json`). |
| **C2** | **Quarantine (`Exit 2`) vs. `ASDD` (`Exit 0`) When `affected_paths` Are Omitted** | Rule 1 sends omitted paths with no `0_Brain` coverage to `quarantine/`, whereas Rule 2 routes `NO_BRAIN_COVERAGE` and `paths_confidence != explicit` to `ASDD`. | **Two-Stage Separation (Locatability Gate Before Lane Triage):** Stage A (Locatability): If `affected_paths` is omitted, query `0_Brain` with extracted technical keywords. If `0_Brain` returns 0 documents with `covers_paths`, the target scope cannot be located -> **Quarantine (`Exit 2`, `UNCERTAIN_SCOPE`)**. Stage B (Lane Triage): Once scope is located, explicit paths lacking `adr`/`wiki` coverage route to **`ASDD` (`Exit 0`, `NO_BRAIN_COVERAGE`)**; inferred paths with located module coverage route to **`ASDD` (`Exit 0`, `INFERRED_PATHS_FAILSAFE`)**. |
| **C3** | **Module Isolation (`intake.exe` vs. `brain.exe`) vs. Zero Regex/Glob Drift & `<500ms p95` Latency** | Spawning `brain.exe` preserves binary boundaries and avoids pulling `modernc.org/sqlite` into `1_Intake`, but duplicating 11 G4 regexes and `MatchCoverPath` risks drift, and extra subprocesses cost latency on Windows. | **Zero-Dependency Engine + Source-Level Parity Test + Lightweight Preflight:** Keep `1_Intake/engine` pure Go stdlib (`CGO_ENABLED=0`). Enforce 100% G4 regex and `MatchCoverPath` equivalence via `parity_test.go` parsing `0_Brain/engine/validator.go`. Check `.index/brain.db` and `.brain-journal.json` in-process before running `context` + `query`, calling `brain.exe verify` only on journal anomaly or `intake verify`. |
| **C4** | **Deterministic `dedupe_key` vs. Incomplete Quarantined Inputs** | `dedupe_key` requires `(project, source, source_ref)`, which may be missing when an item is quarantined (`Exit 2`), and returning non-zero `Exit 2` can hide the generated `<WI-ID>` from scripts. | **Two-Tier ID Keying + Structured Quarantine Output:** Use canonical length-prefixed hashing for `dedupe_key` when `(project, source, source_ref)` are present (`id: WI-<dedupe_key[:12]>`). If any identity field is missing, assign `WIQ-<payload_hash[:12]>` in `quarantine/` and upgrade it atomically to `WI-<dedupe_key[:12]>` upon `intake resolve`. Always emit the JSON result envelope (including `id`, `quarantine_path`, `missing_fields`, and `resolve_hint`) on `Exit 2`. |
| **C5** | **JSON Source of Truth vs. Human-Readable Markdown Views** | Persisting both `WI-xxx.json` and `WI-xxx.md` across `quarantine/` and `workitems/` risks manual edits to `.md` files diverging from JSON. | **Single JSON Source of Truth on Disk + On-Demand Markdown Rendering:** Persist exclusively `WI-<hash12>.json` in `workitems/` and `quarantine/`. Render the Markdown view on demand via `intake show --id <ID> --format md` (or optional `--emit-md` export flag). |

### 3. Secondary Trade-offs and Ideal Final Result (IFR)

* **Trade-offs Accepted:**
  1. Parsing `## Catálogo de Zonas de Riesgo` from `0_Brain/vault/wiki/risk_zones.md` couples `1_Intake` to the 6-column Markdown table structure until `0_Brain` populates frontmatter `covers_paths` or emits structured risk hits in `brain.exe context --json`. Making the parser fail-safe (escalating to `ASDD` + `requires_autor_hitl: true` on parse failure) prevents silent bypasses.
  2. Strict parity with `0_Brain`'s `literal_private_key` rule (`(?i)\bPRIVATE\s+KEY\b` in `validator.go:57`) means descriptions containing the literal English phrase `"private key"` trigger Tier 1 fail-closed (`Exit 3`), logged via a payload-free `REJECTED_G4` event in `intake_events.jsonl` so operators can immediately see which rule fired.
* **Ideal Final Result (IFR):**
  Every incoming request is scanned for Tier 1 secrets before touching disk, Tier 2 redactions cleanly sanitize parsed free-text fields without corrupting delimiters or paths, every risk-zone match (`RZ-001` `CRITICO`, `RZ-002` `ALTO`) is caught even when frontmatter `covers_paths` is empty, and every emitted `WorkItem` is stored as a single authoritative JSON file carrying a self-contained `triage_reasons` and `brain_evidence` trace within `<500ms p95` on Windows.

## Goals vs. Non-Goals

* **In-Scope Goals (v1 MVP):**
  * Pure-Go deterministic CLI engine (`1_Intake/engine` -> `1_Intake/intake.exe`, `CGO_ENABLED=0`, Windows-first, zero third-party dependencies).
  * Two v1 input channels: CLI flags (`intake ingest ...`) and local structured files (JSON or Markdown with YAML frontmatter via `--input <path>` or explicit `--input -` for `stdin`), backed by an extensible `IngestSource` Go interface.
  * Two-tier G4 perimeter enforcement matching all 11 rules in `0_Brain/engine/validator.go:49-105` (8 Tier 1 fail-closed rules returning `exit 3` with 0 payload files created and a payload-free event in `intake_events.jsonl`; 3 Tier 2 subgroup-redaction rules on parsed `title`/`description`/`note`, upgradable to fail-closed via `--strict-g4`; personal paths in `affected_paths` routed to `quarantine/` without persisting the raw path), verified by a cross-module parity test.
  * Read-Before-Act integration with `0_Brain/brain.exe` (`context --json`, `query --json` with deterministic technical keyword extraction, `null`-safe JSON decoding, and `.brain-journal.json`/`.index/brain.db` preflight) via `BrainClient`, combined with deterministic parsing of `## Catálogo de Zonas de Riesgo` in `wiki/risk_zones.md`.
  * Deterministic two-lane triage (`ASDD` vs. `FTD`) and `G6` quarantine routing (`needs_clarification`, `exit 2`) with explicit distinction between deterministic rules (`rule_kind: "deterministic"`) and versioned bilingual lexical heuristics (`rule_kind: "heuristic"`).
  * Option i untracked storage in `1_Intake/runtime/` (`workitems/`, `quarantine/`, `intake_log.md`, `intake_events.jsonl`, protected by `.intake.lock` and excluded in `.gitignore`), persisting only `WI-<hash12>.json` as source of truth and rendering Markdown views on demand via `intake show --format md` (or `--emit-md`), with exit codes `0..5`.
* **Out-of-Scope Non-Goals (Deferred to v2+):**
  * Direct network/webhook adapters for GitHub Issues, Sentry, web forms, Slack, or email (v1 defines only the internal Go port/interface).
  * Direct mutation of `0_Brain/vault/` or `.index/brain.db` by `1_Intake` (`1_Intake` is strictly read-only against `0_Brain`).
  * LLM-based or probabilistic lane classification inside `1_Intake`.
  * Emitting `SDD` or `TDD` as separate lanes (`SDD` is unified under `ASDD`; `TDD` is the mandatory build discipline in `3_Builder`).
  * Replacing `6_Filtro_de_Riesgo` (Team 6): `1_Intake` tags pre-build risk metadata (`risk_level`, `risk_hits`, `requires_autor_hitl`), while `6_Filtro_de_Riesgo` enforces the post-build diff gate.

## Alternatives Weighed

1. **Direct Go Module Import of `blackfactory/brain` (`0_Brain/engine`) into `1_Intake/engine`:**
   * *Rejected by author:* Importing `blackfactory/brain` couples binary releases across Team 0 and Team 1, pulls `modernc.org/sqlite v1.62.0` into `intake.exe` (inflating binary size and build time), and bypasses literal CLI command evidence (`G1`) and the Phase 2 `brain mcp` boundary. Instead, `1_Intake` invokes `0_Brain/brain.exe` via `BrainClient` and enforces regex/glob parity via a cross-module test.
2. **Three Separate Methodological Lanes (`ASDD`, `SDD`, `FTD`):**
   * *Rejected by author:* Maintaining three lanes creates ambiguous boundary criteria between `ASDD` and `SDD` at intake time before a specification exists. `1_Intake` emits two lanes (`ASDD` and `FTD`), treating `TDD` as a transversal builder invariant.
3. **Uniform Fail-Closed G4 Policy on All 11 Rules (No Tier 2 Redaction):**
   * *Rejected by author:* Real bug reports and Windows stack traces routinely contain local profile paths (`C:\Users\<user>\...`) or commit author emails. Rejecting all 11 rules unconditionally at the perimeter would block valid crash reports from entering the factory, whereas unrestricted persistence would violate `G4`. The two-tier policy (fail-closed on 8 hard-secret rules, subgroup redaction on parsed free-text fields for 3 environmental rules, quarantine on personal paths in `affected_paths`, plus `--strict-g4`) resolves both constraints.
4. **Relying Solely on `brain.exe context --json` `matched_paths` for Risk-Zone Detection:**
   * *Rejected by author:* `0_Brain/vault/wiki/risk_zones.md:11` and `0_Brain/schemas/risk_zones.template.md:11` currently ship with `covers_paths: []` in YAML frontmatter and store the actual paths (`GengiSWAP/engine/`, `0_Brain/engine/`) and severity levels (`CRITICO`, `ALTO`) in a Markdown table. Relying only on `matched_paths` produces a 100% false-negative rate on `RZ-001` and `RZ-002`.
5. **Unconditional `ASDD` Routing for All Risk-Zone Levels (`CRITICO` and `ALTO` Alike):**
   * *Rejected by author:* In `wiki/risk_zones.md:37-38`, `RZ-002` (`0_Brain/engine/`) is level `ALTO` (requiring `go vet`, `go test`, and `G4`/`G5` review), whereas `RZ-001` (`GengiSWAP/engine/`) is `CRITICO` (requiring PR + `4_Revisores` + `[AUTOR]` approval). Forcing `ASDD` on every `ALTO` hit would prevent fast-track (`FTD`) bug or audit fixes on `0_Brain/engine/`. Under the chosen policy, `CRITICO` always forces `ASDD` + `requires_autor_hitl: true`, while `ALTO` allows `FTD` only for bounded single-module `bug`/`audit_fix`/`refactor` items with no `impact_flags`, carrying `risk_level: "ALTO"` downstream.
6. **Persisting Derived Markdown Views (`WI-xxx.md`) Alongside JSON by Default:**
   * *Rejected by author:* Maintaining two files per `WorkItem` across `quarantine/` and `workitems/` creates dual-file synchronization hazards and risks manual edits to `.md` files diverging from the JSON source of truth. `WI-<hash12>.json` is the sole persisted artifact, while Markdown views are rendered on demand via `intake show --format md` (or `--emit-md`).

## Key Decisions & Staging for Next Steps

1. **Canonical `WorkItem` Schema (`schema_version: "1.0"`):**
   * **Identity & Provenance:** `schema_version`, `id` (`WI-<hash12>` or `WIQ-<hash12>`), `dedupe_key` (canonical length-prefixed SHA256 over `lower(trim(project))`, `lower(trim(source))`, `lower(trim(source_ref))`), `trace_id`, `revision`, `status` (`ready` | `needs_clarification`), `created_at`, `updated_at`.
   * **Normalized Input:** `project`, `title`, `description`, `note`, `kind` (`bug` | `feature` | `refactor` | `audit_fix` | `ops`), `source`, `source_ref`, `affected_paths` (POSIX-normalized, repo-relative, `..` and personal paths rejected), `paths_confidence` (`explicit` | `inferred_from_query` | `none`), `impact_flags` (`contract` | `schema` | `db` | `auth` | `security` | `governance`).
   * **Triage Verdict & Trace:** `lane` (`ASDD` | `FTD` | `""`), `suggested_template` (`modulo_nuevo` | `correccion_hallazgos` | `auditoria_e2e` | `mapeo_estatico`), `risk_level` (`NONE` | `ALTO` | `CRITICO`), `requires_autor_hitl` (boolean), `risk_hits[]` (`id`, `project`, `level`, `matched_paths[]`, `mandatory_gate`), `triage_reasons[]` (`code`, `rule_kind`: `"deterministic"` | `"heuristic"`, `ruleset_version`, `detail`, `matched_terms[]`).
   * **G4 & `0_Brain` Evidence (`G1` / `G5`):** `sanitization` (`strict_g4`, `sanitized_payload_sha256`, `g4_redactions[]` with `rule`, `field`, `count`), `brain_evidence` (`vault_path`, `brain_db_mtime`, `risk_zones_sha256`, `extracted_query_tokens[]`, `invocations[]` with `command`, `exit_code`, `duration_ms`, `matched_docs[]`), `quarantine_info` (`missing_fields[]`, `reason_codes[]`, `invalid_path_reason`, `resolve_hint`).
2. **Deterministic Execution Sequence (`intake ingest` / `intake resolve`):**
   * **Step 1 (Tier 1 G4 Raw Gate & Input Parsing):** Scan raw input for the 8 Tier 1 hard-secret rules (and the 3 Tier 2 rules if `--strict-g4` is set), respecting `pem_private_key` line suppression over `literal_private_key`. On any match, write 0 files to `workitems/` or `quarantine/`, append a payload-free `REJECTED_G4` event to `intake_events.jsonl`, write the structured rejection to `stderr`, and exit `3`.
   * **Step 2 (Tier 2 In-Memory Subgroup Redaction & Path Hygiene):** Parse JSON or Markdown+YAML frontmatter into memory. Apply delimiter-preserving Tier 2 redaction (`[REDACTED:G4:<rule>]`) exclusively to parsed free-text values (`title`, `description`, `note`). Scan `affected_paths` for personal/out-of-repo paths or `..` traversal; if detected, discard the offending path value in memory and mark the item for quarantine (`invalid_path_reason: "G4_personal_path_in_affected_paths"`). Compute `sanitized_payload_sha256`.
   * **Step 3 (`0_Brain` Preflight & Project Validation):** Verify `.index/brain.db` exists and no pending `.brain-journal.json` exists (running `brain.exe verify --json` on anomaly; exit `4` if locked, missing, or corrupt). Validate `project` against `type: project` docs with fallback to `## Proyectos Registrados` in `0_Brain/vault/index.md`. If required fields are missing, `affected_paths` triggered path quarantine, or `project` is unregistered, persist to `quarantine/` (`exit 2`).
   * **Step 4 (`0_Brain` Consultation & Locatability Gate):** If `affected_paths` is omitted, extract 3–5 technical identifier tokens (stripped of stop-words and FTS5 reserved words `AND`/`OR`/`NOT`/`NEAR`) and call `brain.exe query --json` (handling `null` or `[]`). If 0 documents with `covers_paths` match, persist to `quarantine/` (`UNCERTAIN_SCOPE`, `exit 2`). If explicit `affected_paths` are present, call `brain.exe context --json` and `brain.exe query --json`, plus parse `## Catálogo de Zonas de Riesgo` in `wiki/risk_zones.md`.
   * **Step 5 (Lane Assignment — `ASDD` vs. `FTD`):** Route to `ASDD` if any deterministic trigger fires (`CRITICO` risk zone; `ALTO` risk zone with `kind == feature` or `impact_flags`; `kind == feature` or `ops`; any `impact_flags` present; `NO_BRAIN_COVERAGE` in `adr`/`wiki`; `>1` root module or `>5` files; `paths_confidence != explicit`) or if the versioned bilingual heuristic lexicon matches `title`/`description`. Otherwise route to `FTD`. Persist `WI-<hash12>.json` atomically under `.intake.lock` and log to `intake_log.md` and `intake_events.jsonl`.
3. **CLI Contract (`intake.exe`), Exit Codes (`0..5`), and Verification Metrics:**
   * Subcommands: `ingest`, `resolve`, `list`, `show` (supports `--format json|md`), `verify` (validates runtime JSON schemas, G4 cleanliness, `.gitignore` Option i via `git check-ignore`, `intake_log.md` consistency, and `brain.exe verify --json`).
   * Exit codes: `0` (ready / idempotent hit / command OK), `1` (CLI usage, format, or I/O error), `2` (quarantined in `quarantine/` under `G6`), `3` (G4 fail-closed rejection, 0 payload files created), `4` (`0_Brain` unavailable, locked, or corrupt journal), `5` (idempotency conflict on existing `ready` `dedupe_key` with modified payload when `--update` is not set).
   * Target verification metrics: 0% false negatives on Tier 1 G4 rules and `RZ-001` (`CRITICO`) detection; 100% regex/glob parity with `0_Brain/engine/validator.go` and `index.go`; `<500ms p95` local triage latency on Windows (~50–100 vault docs); `CGO_ENABLED=0 go test -count=1 ./...` passing 100% on `t.TempDir()` (`G3`).
