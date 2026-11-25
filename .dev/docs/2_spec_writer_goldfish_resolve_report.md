# Goldfish Annotated Review Report (`egm-resolve`)

**Target:** `Docs/designs/2_spec_writer.md` (`Lines 1–691`)  
**Source Audit:** `consolidated_review_report.md` (Stages 5–9, `10/10` reviewers `completed`, `unresolved_blockers: 8`)  
**Status:** resolved (Accepted: 8 Blockers [B-04 Option A] + 15 Major PROPOSE FIX + 10 Nit PROPOSE FIX; Contested/Maintained: M-06 part a, M-17 two stages; Declined: N-07)

---

## Summary of Elephant Categorization

* **🔴 Blockers (`8`):** `8 PROPOSE FIX` (`0 CONTEST`, `0 ESCALATE`)
* **🟡 Majors (`17`):** `15 PROPOSE FIX`, `2 PARTIAL CONTEST / PROPOSE FIX` (`M-06` keeping `GENGIS_ENGINE_DRIFT` per ratified `SG5`, `M-17` keeping 2 stages `draft|final` per ratified `Q1`)
* **🟢 Nits (`11`):** `10 PROPOSE FIX`, `1 DECLINE` (`N-07`)

---

## Prioritized Findings & AI Suggestions

### [Issue B-01] [`egm-inventive-catalyst`, `egm-design`, `egm-implementation-readiness`, `egm-codebase-details`, `egm-zoom-out (S8)`] - Severity: Blocker
* **Affected Scope:** [Lines 14-649](Docs/designs/2_spec_writer.md#L14-L649) (`Overview`, `SG4`, `§3`, `§5 compile`, `§8`, `Item 8`, `Item 15`)
* **Discrepancy Description:** `gengis.exe plan` has no `--repo` flag (`GengiSWAP/engine/cmd/gengis/main.go:67-105`) and reads `TargetRepo` solely from `index.md` via `extractTargetRepo` (`bridge.go:164-167, 236-308`). `index.md.tmpl` omits rendering `--repo-root` (and `compile` never checks `--repo-root` vs. `spec_context.repo_root_sha256`), while running `ScanG4All` over final `index.md` collides with Windows `t.TempDir()` (`C:\Users\<user>\...`, triggering `reWinUserPath` in `pkg/bfcore/g4.go:41`), deadlocking `contract_gengis_test.go` and `main_test.go`.
* **Rationale:** Real empirical deadlock verified in `GengiSWAP/engine/bridge.go` and `pkg/bfcore/g4.go`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Formalizing a strict two-zone `G4` boundary keeps all repo-side artifacts (`runtime/`, `spec.json`, `spec_context.json`, `render`, `specwriter_events.jsonl`) 100% free of host paths while allowing `compile` to bind the verified CLI `--repo-root` into the external `gengis_task/<ID>/index.md` without failing `ScanG4All` on Windows `t.TempDir()`.
* ***Proposed Edit:***
  * Location: `SG4` (`L63`), `§3` (`L227`), `§5 compile` (`L323-341`), `§8` (`L379`), `Item 8` (`L558`), `Item 15` (`L641-642`).
  * Change:
    1. Require `--repo-root` on `validate` and `compile` (default `.`), verifying `sha256(normalized_abs(repoRoot)) == spec_context.repo_root_sha256` (exit `4` `STALE_CONTEXT` if mismatch), `bfcore.CheckNoReparseChain`, and `bfcore.ScanTier1` (zero Tier 1 hard secrets) on `--repo-root`.
    2. In `index.md.tmpl` (`## Directorio de Destino Real`), emit `- **Repositorio destino:** <TARGET_REPO>` (plus `- **Rama destino:** <target_branch>` without backticks).
    3. In `compile` `[1]`/`[0]`, run `bfcore.ScanG4All` (all 11 rules) on rendered `index.md` **with placeholder `<TARGET_REPO>`**, `chunks/NN_<id>.md`, and `status.md` in memory; only after `ScanG4All == 0`, substitute `<TARGET_REPO>` with verified `filepath.Clean(absRepoRoot)` solely in the external `gengis_task/<ID>/index.md` written under `--task-root`.

---

### [Issue B-02] [`egm-comprehension`, `egm-design`, `egm-zoom-out (S5 & S8)`] - Severity: Blocker
* **Affected Scope:** [Lines 20-637](Docs/designs/2_spec_writer.md#L20-L637) (`Overview`, `SG4`, `§4.2`, `§5 compile`, `Item 15`)
* **Discrepancy Description:** `validate` (`L30, L274, L300, L612`) permits 3 external cycles (`external_cycles: 1 → 2 → 3`, locking out on the 4th), allowing a spec that resolved its 3rd external cycle (`external_cycles == 3`, preserved by `--refresh-context`) to pass `validate` (`exit 0`). However, `compile` step `[5]` (`L20, L63, L325, L637`) unconditionally rejects `external_cycles >= 3` with `exit 5`, deadlocking a valid spec and shrinking the budget to 2 cycles.
* **Rationale:** Direct state-machine contradiction between `validate` and `compile`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Removing the unconditional `external_cycles >= 3` gate from `compile` `[5]` restores exact parity with `validate` and `L30` ("acumulativo máx. 3; el 4.º devuelve `5`").
* ***Proposed Edit:***
  * Location: `L20`, `L63`, `L325`, `L637`.
  * Change: In `compile` `[5]`, reject with `exit 5` ONLY when: (a) `draft_attempts >= 3 || final_attempts >= 3` (`MAX_VALIDATE_ATTEMPTS`), OR (b) `external_cycles >= 3 && (last_passed_sha256 == "" || last_exit == 4)` (`MAX_EXTERNAL_CYCLES` when no newer passing validation resolved the 3rd cycle). In `compile` `[4]`, if a new external decision triggers while `external_cycles >= 3`, return `exit 5` (`MAX_EXTERNAL_CYCLES`) without mutating `attempts.json`.

---

### [Issue B-03] [`egm-comprehension`, `egm-zoom-out (S8)`, `egm-implementation-readiness`] - Severity: Blocker
* **Affected Scope:** [Lines 286-336, 608-641](Docs/designs/2_spec_writer.md#L286-L336) (`§5`, `Item 13`, `Item 15`)
* **Discrepancy Description:** Precedence places `[5]` (`attempts.json`), `[3]` (`G4` Fase-B over decoded JSON strings), and `[4]` (reading typed `Spec` fields) before `[1]` (`DisallowUnknownFields()`), yet `compile` `[5]` depended on `spec.lane` prior to JSON parsing, and malformed JSON syntax / missing `--spec` before `[1]` had no specified exit code.
* **Rationale:** Real execution-order inversion that would force ad-hoc pre-decoding or falsely burn `external_cycles` on broken JSON.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Defining an exact pre-`[4]` parse boundary cleanly preserves `2 → 5 → 3 → 6 → 4 → 1` without pre-decode hacks.
* ***Proposed Edit:***
  * Location: `§5` (`L286-336`), `Item 13` (`L608-617`), `Item 15` (`L635-641`).
  * Change:
    1. `[2]`: Check CLI flags and verify `--spec` exists (`os.ErrNotExist` → `exit 2`, `INVALID_USAGE`, `retryable: false`; other OS read errors → `exit 6`, `IO_ERROR`, `retryable: true`). Enforce `MaxSpecBytes = 1 MiB` (`exit 1`, `SPEC_TOO_LARGE`, `retryable: true`).
    2. `[5]`: In `validate`, `--stage draft|final` selects the stage counter without reading `spec.json`. In `compile`, check `draft_attempts >= 3 || final_attempts >= 3` (and `B-02`'s `external_cycles` rule) before touching `spec.json`.
    3. `[3]`: Run Fase-A `ScanG4All` on raw bytes of `spec.json` (and `goldfish_review.json` if `--stage final` / `ASDD` and present), then parse JSON for Fase-B `ScanG4All`. If JSON syntax is malformed after passing raw Fase-A `G4`, return immediately `exit 1` (`INVALID_JSON`, `retryable: true`; increments `stage_attempts` in `validate`, never enters `[4]` or increments `external_cycles`).
    4. `[4]` operates on parsed `Spec`; `[1]` enforces `DisallowUnknownFields()` and all schema/semantic rules.

---

### [Issue B-04] [`egm-design`, `egm-implementation-readiness`, `egm-zoom-out (S5 & S8)`, `egm-inventive-catalyst`, `egm-codebase-details`, `egm-style`] - Severity: Blocker
* **Affected Scope:** [Lines 208, 242, 252, 304-306, 478, 504, 513, 614-615](Docs/designs/2_spec_writer.md#L208-L615) (`§2`, `§3`, `§5`, `Item 2`, `Item 3`, `Item 13`)
* **Discrepancy Description:** Supporting trailing-`/` directory prefixes in `ASDD` `can_modify` while freezing `0_Brain/**` introduced a 5-layer workaround (`ParseRiskZonesCoverPatterns` duplicating `0_Brain/engine/query.go:186-332` contrary to `1_intake.md:236`, undefined `RiskZonePattern` & bidirectional glob algorithm, `WalkDir(128)` silent truncation / Windows 32 KiB CLI overflow across units, `path.Clean` stripping trailing `/`, basename `runtime/` exclusion skipping `pkg/runtime/` or `1_Intake/runtime/`), leaving a real `CRITICO` bypass when a parent dir prefix covers an unexpanded or non-existent risk-zone subdir.
* **Rationale:** 7 independent Goldfish reviewers proved the current hybrid is both unimplementable as written and unsafe against `CRITICO` bypass.
* ***AI Suggestion:*** **PROPOSE FIX (Option A — Concrete File Paths in `can_modify` for v1; Option B documented for Author choice)**
* ***Justification:***
  * **Option A (Primary Proposed Fix — Zero `0_Brain` changes, zero dual parser, zero `CRITICO` bypass):** Require concrete file paths in `work_units[].can_modify` for BOTH `FTD` and `ASDD` in v1 (matching `1_Intake`'s `affected_paths` and `observed_files[]`), while keeping directory prefixes allowed on restrictive-only fields (`read_only_paths`, `global_forbidden`, `work_units[].forbidden` — which can never expand scope into `CRITICO`). Remove `ParseRiskZonesCoverPatterns`, `WalkDir(128)`, `MaxDirExpansionFiles`, `<prefijo>/__new__`, and `CheckNoReparseUpToNearestExisting` (deferring `ASDD` directory prefixes to v2 with a single shared parser in `0_Brain`/`pkg/bfcore`). Note that `GengiSWAP/engine/ring.go:101-124` (`under`) matches exact file paths cleanly.
  * **Option B (Alternative if Author requires `ASDD` directory prefixes in v1):** Lift the `0_Brain/**` freeze in Phase 0, move `RiskZoneEntry` + `ParseRiskZonesCatalog` from `0_Brain/engine/query.go:25-332` into `pkg/bfcore` (single parser shared by `0_Brain` and `2_Spec_Writer`), specify exact bidirectional prefix/glob matching, enforce a global cap of `128` files / `16 KiB` returning `exit 4` (`DIR_EXPANSION_TRUNCATED`) on overflow (never silent), preserve `IsDirPrefix` before `path.Clean`, and anchor `WalkDir` exclusions to exact repo-root paths (`.git/`, `node_modules/`, `.dev/`, `1_Intake/runtime/`, `2_Spec_Writer/runtime/`).
* ***Proposed Edit (under Option A):***
  * Location: `L208-210`, `L242`, `L252`, `L304-306`, `L503-504`, `L513`, `L565`, `L614`, `L648`, `Future Improvements`.
  * Change: Restrict `work_units[].can_modify` in v1 to concrete file paths (files to modify or new files to create, both tracked in `observed_files[]` as `"present"`/`"absent"`), allow `AllowDir: true` exclusively on `read_only_paths`, `global_forbidden`, and `work_units[].forbidden`, delete `ParseRiskZonesCoverPatterns`, `MaxDirExpansionFiles`, `WalkDir(128)`, `<prefijo>/__new__`, and `CheckNoReparseUpToNearestExisting`, and add `ASDD` directory prefixes to `Future Improvements` (v2 trigger: shared catalog parser with `0_Brain`).

---

### [Issue B-05] [`egm-implementation-readiness`, `egm-codebase-details`, `egm-inventive-catalyst`, `egm-design`, `egm-zoom-out (S8)`] - Severity: Blocker
* **Affected Scope:** [Lines 345-362, 446, 620, 641-642](Docs/designs/2_spec_writer.md#L345-L362) (`§6`, `NFRs`, `Item 13`, `Item 15`)
* **Discrepancy Description:** On Windows `cmd.exe /S /C` (`GengiSWAP/engine/runner_windows.go:23`), unquoted forward-slash paths (`0_Brain/brain.exe`, `1_Intake/intake.exe`, `2_Spec_Writer/specwriter.exe`) and unquoted `cd /d 0_Brain/engine` fail (`/` parsed as switch); `env` map iteration is non-deterministic; `cwd != ""` breaks repo-root-relative executables. (Coupled directly with `M-02` `brain.exe verify` mutating vault and `M-03` flag-denylist bypasses `-ldflags`, `-test.cpuprofile`, `go build` writing binaries to `cwd`, `git diff --output`/`--no-index`, `\t`, `( ) ; '`, trailing `\`, unvalidated `env` values.)
* **Rationale:** Verified against `GengiSWAP/engine/runner_windows.go:23` and `0_Brain/engine/cmd/brain/main.go:313-323`.
* ***AI Suggestion:*** **PROPOSE FIX** (resolves `B-05`, `M-02`, and `M-03` together)
* ***Justification:*** Replacing the open-ended flag denylist with a closed per-subcommand **flag allowlist** + strict safe-character validation + deterministic Windows `cmd.exe` serializer closes every Windows failure and injection vector cleanly.
* ***Proposed Edit:***
  * Location: `§6` (`L345-362`), `NFRs` (`L446`), `Item 13` (`L620`), `Item 15` (`L641-642`).
  * Change:
    1. **Closed `(executable, subcommand, allowed_flags)` allowlist:**
       - `go test`: allowed flags `-count=1`, `-v`, `-short`, `-race`, `-cover`, `-run=<safe_pattern>`, `-timeout=<1s..300s>` (use `-run=^$` to verify compilation of `cmd/...` without writing binaries; drops mutating `go build`). Relative Go package specs only (`./...`, `./subpkg/...`).
       - `go vet`: allowed flags `-v`; relative package specs only.
       - `git status`: allowed flags `-s`, `--short`, `--porcelain`, `--ignored`; relative paths only.
       - `git diff`: allowed flags `--stat`, `--name-only`, `--check`, `--cached`, `--staged`; relative paths only.
       - `git rev-parse`: allowed flags `--short`, `--abbrev-ref`, `HEAD`.
       - `git check-ignore`: allowed flags `-q`, `-v`; relative paths only.
       - `0_Brain/brain.exe` (`query`, `context` — **`verify` removed** because it calls mutating `RecoverJournal`), `1_Intake/intake.exe` (`verify`, `list`, `show`), `2_Spec_Writer/specwriter.exe` (`verify`, `render` — stdout-only): **require `cwd == ""`**, allow only documented read-only flags (`--vault`, `--data-dir`, `--factory-root`, `--repo-root`, `--project`, `--files`, `--type`, `--spec`, `--json`, `--limit`), and reject `..`, `:`, `\\`, or leading `/` in every path argument.
    2. **Token & `env` validation:** Reject empty `argv[k]`, trailing `\`, whitespace control chars (`\t \v \f \r \n \x00`), and shell metacharacters `& | < > ^ % ! " ' ( ) ;`. Validate `env` values strictly: `CGO_ENABLED` MUST be `"0"`, `GENGIS_CONTRACT_STRICT` in `{"0","1"}`, `GOOS` in `{"windows","linux","darwin"}`, `GOARCH` in `{"amd64","arm64"}`, `GOAMD64` in `{"v1","v2","v3","v4"}`, `PYTHONUTF8` in `{"0","1"}`, `PYTHONIOENCODING == "utf-8"`.
    3. **Deterministic Windows `cmd.exe` serialization in `compile.go`:** Sort `env` keys lexicographically (`set K=V&&`); if `cwd != ""` emit `cd /d "<filepath.FromSlash(cwd)>" && ` (always quoted); convert `argv[0]` with `filepath.FromSlash`; quote any `argv[k]` containing space or `,`.

---

### [Issue B-06] [`egm-implementation-readiness`, `egm-codebase-details`, `egm-zoom-out (S5 & S8)`, `egm-comprehension`, `egm-style`] - Severity: Blocker
* **Affected Scope:** [Lines 155-164, 549, 552-558](Docs/designs/2_spec_writer.md#L155-L164) (`§1`, `Item 8`)
* **Discrepancy Description:** `//go:embed` in `2_Spec_Writer/engine/embed.go` cannot traverse `../schemas/spec.schema.json`, and `"(o copia sincronizada)"` leaves an unresolved file choice outside `Target Files`, while `go.mod` has no JSON Schema library (`validate.go` validates via Go structs + `DisallowUnknownFields()`, matching `1_Intake`).
* **Rationale:** Compile error in Go and violation of the Zero-Decision Rule.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Embedding only `templates/*.md.tmpl` matches `1_Intake/engine`, eliminates duplicate schema drift, and keeps `2_Spec_Writer/schemas/spec.schema.json` as the sole canonical schema artifact verified by unit tests.
* ***Proposed Edit:***
  * Location: `L163`, `L557`, `L648`.
  * Change: Update `embed.go` to `//go:embed templates/*.md.tmpl` ONLY (removing `spec.schema.json (o copia sincronizada)`), state explicitly that `validate.go` enforces `2_Spec_Writer/schemas/spec.schema.json` via `json.Decoder.DisallowUnknownFields()` + programmatic checks, and add a struct-tag / schema parity test in `specwriter_test.go`.

---

### [Issue B-07] [`egm-implementation-readiness`, `egm-design`, `egm-structural`, `egm-comprehension`] - Severity: Blocker
* **Affected Scope:** [Lines 103-109, 225, 244, 307-310, 615](Docs/designs/2_spec_writer.md#L307-L312) (`CUJ 2`, `§3`, `§5`, `Item 13`)
* **Discrepancy Description:** (1) `len(RootModules) > 1` without `decisions[]` falsely blocks initial multi-module `ASDD` WorkItems from `1_Intake` with zero expansion; (2) `ESCALATION_REQUIRED` and `LANE_UPGRADE_REQUIRED` overlap without declared precedence when `FTD` adds a 2nd root module; (3) `ASDD → FTD` downgrade and `FTD` expansion into a covered `CRITICO` file are not blocked; (4) `decisions[]` hash validation (`ComputeScopeSHA256(approved_paths) == scope_sha256` and `can_modify ⊆ approved_paths`) is unspecified.
* **Rationale:** Directly causes false rejections on valid `ASDD` WorkItems and allows security downgrades.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Makes Step `[4]` and Step `[1]` lane/escalation semantics airtight and consistent with `1_Intake/engine/triage.go`.
* ***Proposed Edit:***
  * Location: `L225`, `L244`, `L307-312`, `L615-617`.
  * Change:
    1. Precedence inside Step `[4]`: `1. STALE_CONTEXT → 2. ESCALATION_REQUIRED → 3. LANE_UPGRADE_REQUIRED`.
    2. `ESCALATION_REQUIRED` fires ONLY when: (a) `RiskLevelRank(current) > RiskLevelRank(baseline)` or a new `CRITICO` `risk_id` is hit; OR (b) `len(RootModules) > 1` **and** (`spec_context.work_item.lane == "FTD"` OR `RootModules(can_modify) \ RootModules(baseline_affected_paths) != ∅`), unless satisfied by a valid `decisions[]` entry whose `approved_paths` passes `SanitizeRelativePaths`, satisfies `ComputeScopeSHA256(approved_paths) == decision.scope_sha256 == current_scope_sha256`, and satisfies `can_modify ⊆ approved_paths`.
    3. `LANE_UPGRADE_REQUIRED` fires whenever `spec.lane == "FTD"` and ANY `1_Intake` `ASDD` rule fires (`kind in {"feature","ops"}`, non-empty `impact_flags`, any `CRITICO` `risk_hit`, `RISK_ZONES_UNPARSEABLE_FAILSAFE`, `!EvaluateFTDBounds.WithinBounds`, or any path lacking active `wiki`/`adr` coverage in `0_Brain`).
    4. Step `[1]` explicitly blocks: (a) `spec_context.work_item.lane == "ASDD" && spec.lane != "ASDD"` (exit `1` at `/lane`), and (b) `spec_context.work_item.lane == "FTD" && spec.lane == "ASDD"` without a matching `decisions[]` entry (exit `1` at `/decisions`).

---

### [Issue B-08] [`egm-comprehension` (cross-referenced with `egm-zoom-out` `ZO-3` / `S8-M4`)] - Severity: Blocker
* **Affected Scope:** [Lines 13-19, 62, 106, 276-282, 318, 341, 451, 621](Docs/designs/2_spec_writer.md#L276-L282) (`Overview`, `§4.3`, `Item 13`)
* **Discrepancy Description:** `ASDD` requires `goldfish_review.json` with specific `egm-*` lenses, yet the document never defines `EGM`, `Goldfish`, what each lens audits, or who creates `goldfish_review.json` / `report_path`.
* **Rationale:** Per `egm-resolve` rules, never contest a comprehension gap; a reader must be able to understand the `ASDD` gate from the document alone.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Adding a concise glossary in `Overview` (defining `FTD`, `ASDD`, `OKF`, `RZ`, `HITL`, `EGM / Goldfish`, and `G4` Tier 1/2) plus documenting the `Goldfish` actor/workflow in `§4.3` eliminates all comprehension blockers cleanly.
* ***Proposed Edit:***
  * Location: `Overview` (`L13-16`) and `§4.3` (`L276-282`).
  * Change: Add concise definitions of `FTD` (Fast-Track Deterministic — small low-risk changes `<= 5` effective files in `1` root module), `ASDD` (Architecture-Specified Deterministic Design — multi-module, high-risk, or architectural changes requiring full narrative + independent design review), `EGM / Goldfish` (Elephant-Goldfish Model: clean-context adversarial reviewer subagents spawned by the external agent harness to audit `spec.json` / rendered Markdown before `--stage final`, writing a Markdown report at `report_path` and summarizing counts/lenses in `goldfish_review.json`), `OKF`, `RZ` (Risk Zone), `HITL` (Human-in-the-Loop), and `G4` Tier 1 (hard secrets) vs. Tier 2 (personal paths/PII). In `§4.3`, briefly explain the responsibility of each required lens (`egm-design`, `egm-codebase-details`, `egm-implementation-readiness`, plus conditional `egm-structural`, `egm-zoom-out`) and explicitly document the harness actor that writes `goldfish_review.json`.

---

### [Issue M-01] [`egm-codebase-details`, `egm-design`, `egm-inventive-catalyst`, `egm-comprehension`, `egm-zoom-out (S8)`, `egm-implementation-readiness`] - Severity: Major
* **Affected Scope:** [Lines 232-233, 252, 254, 312-318, 379, 558](Docs/designs/2_spec_writer.md#L252-L254) (`§3`, `§5`, `§8`, `Item 8`)
* **Discrepancy Description:** `GengiSWAP/engine/bridge.go:523-527` maps `## Salidas` to `"other"` and reads `Chunk.Scope.CanModify` exclusively from `## Puede modificar` (`L523`), so emitting `## Salidas` silently disables per-chunk scope narrowing (`ring.go:127-132`). Additionally, merging other chunks' `can_modify` into `## Fuera de Alcance` conflicts with `depends_on` shared files (`ring.go:120`), `validate` did not reject `can_modify` shadowed by `read_only_paths`/`forbidden`, `index.md` `## Alcance` (`cleanScopeItems`) parses non-backticked `/` prose, and `contract_gengis_test.go` did not assert `Chunk.Scope.CanModify`.
* **Rationale:** Empirically verified at `GengiSWAP/engine/bridge.go:417-419, 523-528, 630` and `ring.go:116-132`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Critical `GengiSWAP` contract correctness fix.
* ***Proposed Edit:***
  * Location: `L232-233`, `L252`, `L254`, `L312-313`, `L379`, `L558`, `L617`, `L649`.
  * Change: Emit `## Puede modificar` (backticked paths from `work_units[].can_modify`) in `templates/chunk.md.tmpl`; in `## Fuera de Alcance`, add other chunks' `can_modify` **excluding** any path equal to or prefix-overlapping the current chunk's own `can_modify`; in `validate` `[1]`, verify transitive DAG reachability for shared `can_modify` and reject any `can_modify` path shadowed by `read_only_paths`, `global_forbidden`, or `work_units[].forbidden`; in `index.md` `## Alcance`, emit strictly backticked paths without `/` in surrounding prose; verify parsed `Chunk.Scope` / `Scope.Allows` in `specwriter_test.go` & `contract_gengis_test.go`.

---

### [Issue M-02 & M-03] [`egm-design`, `egm-style`, `egm-inventive-catalyst`, `egm-zoom-out (S8)`] - Severity: Major
* **Affected Scope:** [Lines 345-362, 446, 620](Docs/designs/2_spec_writer.md#L345-L362) (`§6`, `NFRs`, `Item 13`)
* **Discrepancy Description:** `0_Brain/brain.exe verify` invokes mutating `RecoverJournal` (`0_Brain/engine/cmd/brain/main.go:313-323`), contradicting read-only classification (`1_intake.md:52, 253`); flag denylist misses `-ldflags`, `-gcflags`, `-asmflags`, `-modfile`, `-C`, `go test -test.cpuprofile`, `go build` writing to `cwd`, `git diff --output`/`--ext-diff`/`--no-index`/`-C`, `\t`, `( ) ; '`, trailing `\`, and unvalidated `env` values.
* **Rationale:** Verified directly against `0_Brain/engine/cmd/brain/main.go:313-323` and `GengiSWAP/engine/runner_windows.go:23`.
* ***AI Suggestion:*** **PROPOSE FIX** (consolidated into `[Issue B-05]` edit above).

---

### [Issue M-04] [`egm-style`, `egm-comprehension`, `egm-inventive-catalyst`, `egm-design`] - Severity: Major
* **Affected Scope:** [Lines 99, 180-185, 210, 329, 337, 447, 639-642](Docs/designs/2_spec_writer.md#L329-L340) (`CUJ 1`, `§5`, `NFRs`, `Item 15`)
* **Discrepancy Description:** Contradictory lock filename `.compile.lock` vs `.<task-id>.compile.lock` (Windows `LockFileEx` unlink race/orphan dotfiles); missing global lock ordering (`.wi.lock` → `.compile.lock` → `.events.lock`), missing `<task-root>` `MkdirAll` before `AcquireFileLock`, missing `CheckNoReparseChain` + fail-closed `os.Lstat` on `.ring/state.json`, and missing byte quotas (`MaxSpecBytes`).
* **Rationale:** Verified against `pkg/bfcore/lock_windows.go:27`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Eliminates lock-name conflict, Windows unlink races, and TOCTOU windows.
* ***Proposed Edit:***
  * Location: `L99`, `L329`, `L337`, `L447`, `L565`, `L573`, `L639-642`.
  * Change: Standardize on `<task-root>/.compile.lock` (single static dotfile, never unlinked); define strict lock acquisition order `.wi.lock → .compile.lock → .events.lock` holding `.wi.lock` across `[5]..[0]`; in `compile` `[6]`, run `CheckNoReparseUpToNearestExisting` + `os.MkdirAll(taskRoot, 0o755)` + `CheckNoReparseChain` on `<task-root>` and lock file before `AcquireFileLock`, and check `<dest>` and `<dest>/.ring/state.json` via `CheckNoReparseChain` + fail-closed `os.Lstat` (non-`ErrNotExist` → `exit 6` `IO_ERROR`); specify quotas `MaxSpecBytes = 1 MiB`, `MaxGoldfishBytes = 256 KiB`, and streaming SHA-256 on `observed_files[]`.

---

### [Issue M-05] [`egm-inventive-catalyst`, `egm-implementation-readiness`, `egm-comprehension`, `egm-style`, `egm-design`, `egm-zoom-out (S8)`] - Severity: Major
* **Affected Scope:** [Lines 18, 125-131, 242, 265, 270, 301-303, 599, 601, 613](Docs/designs/2_spec_writer.md#L265-L303) (`§4.1`, `§5`, `Item 12`, `Item 13`)
* **Discrepancy Description:** (1) Global `brain_db_mtime` causes unrelated `0_Brain` updates to burn `external_cycles`; (2) `L599` pre-populates `observed_files[]` from `baseline_hashes[]`, which (`L265`) includes vault docs; (3) `work_item_ref` mismatch after `--refresh-context` and missing `observed_files[]` entries were ambiguous between `exit 4` and `exit 1`.
* **Rationale:** Prevents `STALE_CONTEXT` loops and spurious `external_cycles` burn.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Clean ownership separation between frozen context drift (`exit 4`) and author-fixable `spec.json` mismatches (`exit 1`).
* ***Proposed Edit:***
  * Location: `L265`, `L301-303`, `L599`, `L613`, `L617`.
  * Change: Replace `brain_db_mtime` in `STALE_CONTEXT` with content hashes of matched active `0_Brain` documents (`matched_doc_hashes[]`) + `risk_zones_sha256` (retaining `brain_db_mtime` purely as informational metadata); pre-populate `spec.json.observed_files[]` in `prepare` exclusively from `WorkItem.AffectedPaths` (never vault docs); check `spec.work_item_ref == spec_context.work_item` and completeness of `observed_files[]` (every `affected_paths ∪ g5_citations[].path ∪ can_modify` present) in Step `[1]` (`exit 1` with JSON Pointer); reserve Step `[4]` `STALE_CONTEXT` (`exit 4`) strictly for disk-state/hash drift vs. `spec_context.json` (`baseline_hashes[]`, `risk_zones_sha256`, `matched_doc_hashes[]`) or vs. declared `spec.observed_files[]` (`"present"` hash changed or `"absent"` appeared on disk — noting in `CUJ 4` that recovery is `prepare --refresh-context` + updating `spec.json.observed_files[]`).

---

### [Issue M-06] [`egm-zoom-out (S5 & S8)`, `egm-style`] - Severity: Major
* **Affected Scope:** [Lines 18, 20, 32, 64, 171, 296, 328, 368-372, 587, 634, 683](Docs/designs/2_spec_writer.md#L368-L372) (`SG5`, `§5`, `§7`, `Item 11`, `Item 15`)
* **Discrepancy Description:** (a) Hardcoding `PinnedBridgeGoSHA256`/`PinnedRingGoSHA256` (`GENGIS_ENGINE_DRIFT`) couples `verify` to comments/formatting in `GengiSWAP/engine/{bridge,ring}.go` despite `GENGIS_CONTRACT_STRICT=1`; (b) ambiguous which subcommands run `CheckGitIgnore` and `CheckGengisEngineHashes` vs. the shared preflight checks.
* **Rationale:** Part (a) conflicts with explicit `SG5` / `§7` contract ratified in drafting (`L64, L371`); Part (b) is a real scope ambiguity.
* ***AI Suggestion:*** **CONTEST (Part a) / PROPOSE FIX (Part b)**
* ***Justification:***
  * **Part (a) — CONTEST:** Keeping `GENGIS_ENGINE_DRIFT` in `specwriter verify` is an explicit recorded requirement in `2_spec_writer.md` (`SG5` `L64`, `§7` `L371`) providing fast offline drift detection on `RZ-001` (`bridge.go`, `ring.go`, plus `runner_windows.go`) when running `specwriter verify` without invoking `go test`. *(Author can overrule and drop `GENGIS_ENGINE_DRIFT` in favor of `GENGIS_CONTRACT_STRICT=1` alone if desired.)*
  * **Part (b) — PROPOSE FIX:** Move `PinnedBridgeGoSHA256`/`PinnedRingGoSHA256` constants from `compile.go` to `verify.go`, explicitly enumerate the shared preflight subset (`CheckReparseSafety` + `CheckBrainPreflight`) for `prepare`/`validate`/`compile`, and state explicitly that `CheckGitIgnore` (`GITIGNORE_DRIFT`) and `CheckGengisEngineHashes` (`GENGIS_ENGINE_DRIFT`) execute exclusively in `specwriter verify` on `--factory-root`.

---

### [Issue M-07] [`egm-zoom-out (S5 & S8)`, `egm-inventive-catalyst`, `egm-implementation-readiness`] - Severity: Major
* **Affected Scope:** [Lines 20, 27, 63, 336, 558, 617, 641](Docs/designs/2_spec_writer.md#L336-L336) (`§5`, `Item 13`, `Item 15`)
* **Discrepancy Description:** `validate` and `compile` duplicated validation logic, deferring `GengiSWAP` `parseChunkFile` Markdown collisions (verified at `GengiSWAP/engine/bridge.go:463-474`: `inCoreH2` only protects `- ` bullet lines, not unbulleted `Clave: val` prose) to `compile`'s `VALIDATOR_BYPASS_DEFECT`.
* **Rationale:** Real parser trap in `bridge.go:463-474` and avoidable code duplication.
* ***AI Suggestion:*** **PROPOSE FIX (shared validator + `bridge.go` prose guard) / CONTEST (removing `VALIDATOR_BYPASS_DEFECT`)**
* ***Justification:*** Extract ONE shared function `ValidateSpecAndRenderCheck` called by both `validate` `[1]` and `compile` `[1]` (checking `parseChunkFile` collisions: forbidding `^#{1,6}\s`, fenced ```` ``` ````/`~~~`, and unbulleted `parseFieldLabel` `Key: val` lines in `objective`/`failure_proof`, plus `len(work_units) <= 99`), so agents receive fixable `exit 1` JSON Pointers during `validate`. Keep `VALIDATOR_BYPASS_DEFECT` solely as a defensive invariant guard in `compile` (`SG4` `L63`, `L336`) if `compile` ever fails after a verified matching `(last_passed_sha256, stage, goldfish_sha256)`.

---

### [Issue M-08] [`egm-inventive-catalyst`] - Severity: Major
* **Affected Scope:** [Lines 15, 64, 70, 230, 244-245, 253, 341, 419, 558](Docs/designs/2_spec_writer.md#L341-L341) (`§5 compile`, `Item 8`)
* **Discrepancy Description:** `requires_autor_hitl`, `decisions[]`, `risk_mitigations[]`, and `scope_expansions` appear only in `compile` stdout and are absent from `gengis.exe plan` (`bridge.go:1500-1519` `printPlanSummary`), the sole `[AUTOR]` stop in v1.
* **Rationale:** Verified at `GengiSWAP/engine/bridge.go:1500-1519`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Justification:*** Improves `[AUTOR]` visibility at `gengis.exe plan` with zero changes to `GengiSWAP/engine/`.
* ***Proposed Edit:***
  * Location: `L341`, `L558`, `L642`.
  * Change: Deterministically prefix `index.md` `Objetivo` and the first line of `Chunk.Spec` (printed by `bridge.go:1508`) with compact governance tags (`[HITL]`, `[CRITICO]`, `[ESCALATED]`, `[EXPANSIONS:N]`) whenever applicable, without absolute paths.

---

### [Issue M-09] [`egm-design`] - Severity: Major
* **Affected Scope:** [Lines 181, 271, 372, 445, 453, 576](Docs/designs/2_spec_writer.md#L576-L576) (`NFRs`, `Item 10`)
* **Discrepancy Description:** `specwriter_events.jsonl` lacked an explicit `bfcore.RedactAllG4` + `ScanG4All == 0` guard before appending, allowing OS/lock errors containing `C:\Users\...` to permanently break `specwriter verify` with `exit 3`.
* **Rationale:** Consistent with `1_intake.md:232` and prevents self-inflicted `G4_VIOLATION_ON_DISK`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L445`, `L453`, `L576`.
  * Change: Mandate applying `bfcore.RedactAllG4` to every free-text/detail field of `SpecWriterEvent` and verifying `len(bfcore.ScanG4All(line)) == 0` in memory before appending under `.events.lock`.

---

### [Issue M-10] [`egm-structural`, `egm-style`, `egm-implementation-readiness`, `egm-comprehension`] - Severity: Major
* **Affected Scope:** [Lines 24-32, 120, 295, 299, 372, 565-566, 588, 655, 683](Docs/designs/2_spec_writer.md#L24-L32) (`Overview`, `CUJ 3`, `§5`, `§7`, `Item 9`, `Item 11`, `Item 17`)
* **Discrepancy Description:** `IO_ERROR` had conflicting `retryable` semantics (`true` at `L32, 299` vs `false` at `L372`); `G4` reason alternated between `"G4"` (`L120`) and `"G4_VIOLATION"` (`L295`); exit `1`/`2` lacked named reasons; `CLIResult` omitted `last_exit`/`last_reason`; `PrepareResult`, `VerifyReport`, and `SpecWriterEvent` field lists were incomplete.
* **Rationale:** Eliminates contract ambiguity for callers branching on `(exit_code, reason, retryable)`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L24-32`, `L120`, `L372`, `L565-566`, `L588`, `L594`, `L655`.
  * Change: Standardize `"G4_VIOLATION"` (`L120`, `L295`) and `"G4_VIOLATION_ON_DISK"` (`L372`); add `ARTIFACT_STATE_CORRUPT` (`exit 6`, `retryable: false`) keeping `IO_ERROR` strictly `retryable: true`; name exit `1`/`2` reasons (`VALIDATION_FAILED`, `INVALID_JSON`, `SPEC_TOO_LARGE`, `INVALID_USAGE`, `SPEC_ALREADY_EXISTS`); consolidate exact struct fields for `CLIResult` (including `last_exit`, `last_reason`), `PrepareResult`, `VerifyReport` (`ok`, `gitignore_ok`, `gengis_hashes_ok`, `brain_ok`, `g4_issues`, `artifacts_checked`, `issues[]`), and `SpecWriterEvent` in `Item 9` (`model.go`), specifying that `verify --json` outputs `VerifyReport` embedding `exit_code`, `reason`, `retryable`.

---

### [Issue M-11] [`egm-structural`, `egm-style`, `egm-comprehension`, `egm-implementation-readiness`, `egm-design`] - Severity: Major
* **Affected Scope:** [Lines 18-20, 92-94, 117-118, 284-373, 628, 639-640, 655](Docs/designs/2_spec_writer.md#L628-L656) (`CUJs`, `§5`, `Item 14`, `Item 15`, `Item 17`)
* **Discrepancy Description:** Missing per-subcommand CLI flag/default matrix (including `--task-root`, `--brain-bin`), undefined `render` stdout/`--json` contract, missing `prepare` exit code for invalid/`needs_clarification` `WorkItem`, and ambiguous `"sin consumir intentos"` in `compile` `[4]`.
* **Rationale:** Necessary for unambiguous CLI implementation and E2E testing.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `§5` (new compact CLI Flag Contract table), `CUJ 1` (`L94`), `CUJ 3` (`L117-118`), `Item 12` (`L595`), `Item 14` (`L628`), `Item 15` (`L640`), `Item 17` (`L655`).
  * Change: Add exact per-subcommand CLI flag matrix with defaults (`--repo-root` default `.`, `--factory-root` default `--repo-root`, `--vault` default `<factory-root>/0_Brain/vault`, `--data-dir` default `<factory-root>/2_Spec_Writer/runtime`, `--brain-bin` default `<factory-root>/0_Brain/brain.exe`, `--task-root` mandatory on `compile`, aligned in `CUJ 1`/`CUJ 3`); define `render` as stdout-only (`0` files written; raw Markdown default, `{ok, exit_code, spec_sha256, trace_id, markdown}` with `--json`); map invalid/`needs_clarification` `WorkItem` in `prepare` to `exit 2` (`INVALID_USAGE`); state explicitly that `compile` `[4]` never mutates `attempts.json` (neither `stage_attempts` nor `external_cycles`).

---

### [Issue M-12] [`egm-codebase-details`, `egm-implementation-readiness`, `egm-design`, `egm-structural`] - Severity: Major
* **Affected Scope:** [Lines 245, 276-282, 293, 326, 610, 619, 621](Docs/designs/2_spec_writer.md#L245-L326) (`§3`, `§4.3`, `§5`, `Item 13`, `Item 15`)
* **Discrepancy Description:** Missing `os.ErrNotExist` handling for `goldfish_review.json` in `validate --stage final` (`exit 1` vs `exit 6`), `compile` `[3]` omitted scanning `goldfish_review.json` with `G4`, and `risk_mitigations[].review_gates[]` element type + ID uniqueness scope across `work_units` were unspecified.
* **Rationale:** Required for `spec.schema.json` (`additionalProperties: false`) and consistent `G4`/error semantics.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L245`, `L256-257`, `L293`, `L326`, `L610`, `L619`, `L621`, `L638`.
  * Change: Specify missing `goldfish_review.json` in `ASDD --stage final` returns `exit 1` (`VALIDATION_FAILED`, incrementing `final_attempts`, never `IO_ERROR` `exit 6`); scan `goldfish_review.json` (when `ASDD` and present) in `compile` `[3]`; enforce globally unique `verify_steps[].id` and `acceptance_criteria[].id` across `spec.json`, and specify `risk_mitigations[].review_gates[]` as `string[]` referencing existing `acceptance_criteria[].id` entries with `kind == "review_gate"`.

---

### [Issue M-13] [`egm-implementation-readiness`, `egm-style`] - Severity: Major
* **Affected Scope:** [Lines 208, 503, 599](Docs/designs/2_spec_writer.md#L503-L599) (`§2`, `Item 2`, `Item 12`)
* **Discrepancy Description:** Unnamed second `string` return value on `SanitizeRelativePaths`, and unspecified initial skeleton field defaults written by `prepare` to `spec.json`.
* **Rationale:** Removes guesswork during `bfcore` and `prepare.go` implementation.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L208`, `L503`, `L599`.
  * Change: Signature `SanitizeRelativePaths(rawPaths []string, repoRoot string, opts PathSanitizeOpts) (cleanPaths []string, rejectReason string)` returning non-nil `[]string{}` and `""` on success (preserving `"G4_personal_path_in_affected_paths"` / `"PATH_TRAVERSAL_OR_INVALID_SCOPE"` on rejection); specify exact `prepare` skeleton defaults (`schema_version: "1.0"`, `work_item_ref`, `project`, `lane`, `template: suggested_template`, `target_branch: ""` or detected git branch, `title: WorkItem.Title`, `g3_data_mutation: ""`, `requires_autor_hitl`, `observed_files[]` from `AffectedPaths`, empty slices `[]` everywhere else).

---

### [Issue M-14] [`egm-style`] - Severity: Major
* **Affected Scope:** [Lines 681, 683](Docs/designs/2_spec_writer.md#L681-L683) (`Verification Plan`)
* **Discrepancy Description:** `git status --ignored -s 2_Spec_Writer/` will NOT display `!! 2_Spec_Writer/runtime/` when `runtime/` is empty/non-existent after a read-only `verify` run.
* **Rationale:** Verified Git behavior on empty directories.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L681-683`.
  * Change: Replace/supplement `git status --ignored -s 2_Spec_Writer/` with two individual commands `git -C "C:\Black Factory" check-ignore -v 2_Spec_Writer/runtime/` and `git -C "C:\Black Factory" check-ignore -v 2_Spec_Writer/specwriter.exe` (both must exit `0`), plus `git status --ignored -s 2_Spec_Writer/` confirming `!! 2_Spec_Writer/specwriter.exe` and zero tracked artifacts under `runtime/`.

---

### [Issue M-15] [`egm-structural`, `egm-style`] - Severity: Major
* **Affected Scope:** [Lines 242, 442-473](Docs/designs/2_spec_writer.md#L442-L473) (`§3`, `NFRs`, `Additional Resources`, `Future Improvements`)
* **Discrepancy Description:** `### Additional Resources` and `### Future Improvements` were nested as H3 under `## Non-Functional Requirements`; no consolidated `Risks, Limits & Mitigations` section; `/observed_files[]` cell mixed in `can_modify` directory rules.
* **Rationale:** Aligns directly with `feature-spec-standard.md`.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L242`, `L442-473`.
  * Change: Keep `L242` strictly about `/observed_files[]`; add `## Risks, Limits & Mitigations` summarizing trust-boundary limits (`attempts.json` loop brake, `goldfish_review.json` consistency hygiene, `5 min` `ShellRunner` timeout, single-operator v1, concrete file paths in v1); promote `## Additional Resources` and `## Future Improvements` to `H2` with standard horizontal rules.

---

### [Issue M-16] [`egm-comprehension`, `egm-style`] - Severity: Major
* **Affected Scope:** [Lines 14-17, 36, 64-65, 225, 401, 419-420, 450](Docs/designs/2_spec_writer.md#L14-L65) (`Overview`, `SG5`, `SG6`, `§3`, `Alternatives`, `NFRs`)
* **Discrepancy Description:** Undefined acronyms (`FTD`, `ASDD`, `OKF`, `RZ`, `EGM`, `G4` Tier 1/2), metaphorical/subjective phrasing (`Freno Honesto`, `teatro de seguridad`, `Preparador y Chef`, `arquitectura "sandwich" delgada`), ambiguous `Recalculado por validate` (`L225`), and `SG6` vs NFR latency scope/phrasing mismatch (`verify` unbounded, `repositorios de tamaño medio`, `sin superar <= 2`).
* **Rationale:** Improves precision and objective technical tone.
* ***AI Suggestion:*** **PROPOSE FIX**
* ***Proposed Edit:***
  * Location: `L14-17`, `L64-65`, `L225`, `L401`, `L419-420`, `L450`.
  * Change: Add concise glossary (`B-08`); replace metaphorical labels with dry engineering terms (`Freno Operativo contra Bucles`, `arquitectura en tres fases`, `responsabilidades de diseño y ejecución`, `no aporta aislamiento real...`); change `L225` to `Verificado por validate`; align `SG6` and `L450` latency scope (`prepare`, `validate`, `render`, `compile`, and `verify` on `<= 50` runtime artifacts) with `un máximo de 2 invocaciones`.

---

### [Issue M-17] [`egm-zoom-out (S5 & S8)`] - Severity: Major
* **Affected Scope:** [Lines 19, 62, 79, 272-282, 621](Docs/designs/2_spec_writer.md#L79-L79) (`Overview`, `Open Questions Q1`, `§4.3`, `Item 13`)
* **Discrepancy Description:** Suggests collapsing `--stage draft|final` into a single stage and removing hardcoded `egm-*` plugin lens names from Go.
* **Rationale:** Two-stage `draft|final` with independent attempt budgets is an explicitly ratified decision (`Q1` recorded at `L79`), ensuring 0-token `validate --stage draft` runs before spending tokens on Goldfish reviews. Decoupling `egm-*` lens names from Go vs. keeping them is an architectural choice for `[AUTOR]`.
* ***AI Suggestion:*** **CONTEST (keeping `--stage draft|final` per ratified `Q1` at `L79`) / PROPOSE FIX (documenting Goldfish workflow in `B-08`)**
* ***Justification:*** `Q1` (`L79`) explicitly documents the ratified two-stage budget (`draft`: 3, `final`: 3) to prevent Goldfish token waste on broken drafts (`L451`). Regarding `lenses_run[]`, we keep the documented list unless `[AUTOR]` chooses to validate non-empty `lenses_run[]` generically in Go and keep specific `egm-*` names in `SKILL.md`.

---

### [Issues N-01 to N-11] [Multiple Reviewers] - Severity: Nit
* **N-01 (`L229`):** Cite `templates/modulo_nuevo/index.md:14` and `templates/{auditoria_e2e,correccion_hallazgos,mapeo_estatico}/index.md:12` → **PROPOSE FIX**.
* **N-02 (`L198-208, 491-503, 526-528`):** Specify `maxEffective <= 0` → `DefaultMaxFTDFiles`, `len(paths) == 0` → `WithinBounds: false`, `SplitCommas`/`FoldCaseDedupe` in `PathSanitizeOpts` preserving `1_Intake` test/hash parity, reject `"."` → **PROPOSE FIX**.
* **N-03 (`L243, 247, 253, 314-315, 618`):** Unify `G5` code extensions/reserved words, define `G5` on `.md`/`.json` (1-based line bounds + non-empty `symbol_anchor ±5`), cap `work_units <= 99` (`bridge.go:347`), require `scope_expansions` when `can_modify \ affected_paths != ∅` → **PROPOSE FIX**.
* **N-04 (`L390..438`):** Replace `Rejected by author:` with direct technical rationale (`Rejected due to ...`) → **PROPOSE FIX**.
* **N-05 (`L314`):** Replace literal ```` ```mermaid ```` in diagram with `G2 (cero bloques Mermaid)` → **PROPOSE FIX**.
* **N-06 (`L366, 567, 575, 642, 649, 667, 675, 685`):** Align symbol casing (`ReadFileWithShortRetry`, `CanonicalSpecSHA256`), separate `workspace/qa/` dir creation from `Write+Sync+Close`, clarify `contract_gengis_test.go` stdout assertion, check `1_Intake/engine/go.mod` for 0 SQLite, add concrete `go test -run TestLatencyBudgetP95` command and `gengis.exe` build note → **PROPOSE FIX**.
* **N-07 (`L147, 209-210, 504-505`):** Move `ComputeNormalizedFileSHA256` out of `pkg/bfcore` → **DECLINE** (negligible churn; keeping normalized SHA-256 alongside `paths.go` in `pkg/bfcore` avoids re-exporting normalization helpers).
* **N-08 (`L347-354, 367`):** Clarify in `§7` / `Non-Goals` that v1 `verify_steps` targets the Black Factory monorepo and Go repositories (`--factory-root` split kept for clean path resolution) → **PROPOSE FIX**.
* **N-09 (`L14, 46, 56, 62, 90, 97, 120, 130, 139, 151, 200, 230, 257, 273, 456-457`):** Fix `reparse_other.go`, split `L56`, tighten filler adverbs and `"sin payload"` phrasing → **PROPOSE FIX**.
* **N-10 (`L588, 680`):** State `VerifyRuntime` passes (`ok: true`, `0` files created) when `runtime/` does not exist yet on a clean checkout → **PROPOSE FIX**.
* **N-11 (`L558`):** Enumerate exact H2 headings emitted in `templates/{index,chunk,status}.md.tmpl` → **PROPOSE FIX**.
