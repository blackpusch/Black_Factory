---
name: egm-to-gengis
description: >-
  Compiles an approved EGM design specification (Docs/designs/*.md) into a
  deterministic GengiSWAP task (index.md, chunks/, status.md), runs autonomous
  TDD implementation with `gengis.exe check`, and performs clean-context
  Goldfish QA over the real git diff before requesting `[AUTOR]` decision.
  Use when implementing an approved EGM design doc in Black Factory.
---

# EGM → GengiSWAP Execution Bridge (`egm-to-gengis`)

Bridge between **EGM** (upstream design & multi-lens review) and **GengiSWAP** (`GengiSWAP/gengis.exe`, downstream deterministic scope/evidence execution) for **Black Factory**, minimizing human intervention while strictly preserving the `[AUTOR]` governance gate.

## Prerequisites & Hard Invariants

1. **Approved Spec Required:** Target document in `Docs/designs/<module>.md` must have `Status: Approved` and a `## Detailed Implementation Plan` + `### Verification Plan`.
2. **Read Canonical Protocols First:** Read `GengiSWAP/gengis_swap_common.md`, `GengiSWAP/gengis_swap_prep_protocol.md`, and `GengiSWAP/gengis_swap_chef_protocol.md`.
3. **Never Bypass `[AUTOR]` Gates:** Agents **MUST NOT** pipe synthetic stdin (`echo s | gengis plan` or `echo GO_NEXT | gengis decide`) to bypass human approval. Agents run `gengis.exe check` and `gengis.exe status` autonomously; `[AUTOR]` owns `gengis.exe plan` (`PLAN_APROBADO`) and `gengis.exe decide` (`GO_NEXT` / `RETRY_CHUNK` / `RESPLIT_CHUNK` / `ABORT`).
4. **Global Rules `G1–G6` & Privacy `Option i`:** No secrets or personal paths (`G4`), no Mermaid blocks (`G2`, use ASCII/Box-Drawing), tests on `t.TempDir()` (`G3`), and never stage/track `0_Brain/vault/`, `1_Intake/runtime/`, `*.exe`, `*.db*`, or `.dev/`.

---

## Phase A — Compile EGM Spec into a GengiSWAP Task (Preparador)

1. **Branch Hygiene:** Ensure the target design doc is committed on the dedicated feature branch (`feat/<module>`) so `git status` is clean before `gengis.exe plan` records `.ring/base_ref`.
2. **Minimize Human Stops (Coarse Disjoint Chunks):** Group the `Detailed Implementation Plan` into the fewest coherent chunks (typically **2–3 chunks**) whose tracked file scopes are **strictly disjoint**, so closed `[x] hecho` deliverables naturally remain `Solo lectura` for later chunks.
3. **Create Task Folder:** `C:\Desarrollo\GengiSWAP_data\gengis_task\<TASK_ID>\` containing:
   - `index.md` (`Plantilla: modulo_nuevo v1` or `correccion_hallazgos v1`), with complete `## Alcance` (`Puede modificar`, `Mutación de datos (G3)`, `Solo lectura`, `Prohibido`).
     - **`bridge.go` Pitfall Guard:** Do **NOT** wrap branch names inside `Puede modificar:` or allowed paths inside `Prohibido:` in backticks (`` `...` ``), because `cleanScopeItems` parses backticked tokens as scope paths.
   - `chunks/01_...md`..`NN_...md` (last chunk = Integration), each with **Entradas**, **Salidas**, **Criterios de Aceptación**, **Comando de Verificación** (valid Windows `cmd.exe` syntax — e.g. `set CGO_ENABLED=0&& cd /d "..." && ...`, **not** PowerShell-only syntax), **Prueba de que puede fallar**, and **Fuera de Alcance** (backticked paths strictly forbidden for that chunk).
   - `status.md` (Protocolo v2 5-state format + `[-] Parcial` footer).
   - `workspace/qa/` directory.
4. **Dry-Validate Parsing:** Run:
   ```powershell
   & "C:\Black Factory\GengiSWAP\gengis.exe" status "C:\Desarrollo\GengiSWAP_data\gengis_task\<TASK_ID>" --root "C:\Black Factory\GengiSWAP"
   ```
   Confirm all chunks parse cleanly without errors.
5. **Hand Off for Single Plan Approval:** Give `[AUTOR]` the exact 1-line command to register `PLAN_PENDIENTE` + `PLAN_APROBADO` and lock `.ring/base_ref`:
   ```powershell
   & "C:\Black Factory\GengiSWAP\gengis.exe" plan "C:\Desarrollo\GengiSWAP_data\gengis_task\<TASK_ID>" --root "C:\Black Factory\GengiSWAP"
   ```

---

## Phase B — Autonomous Execution per Chunk (Chef + `gengis.exe check`)

Once `[AUTOR] PLAN_APROBADO` exists in `GengiSWAP/gengis_global_log.md`:

1. **TDD First:** Write failing tests covering the chunk's acceptance criteria and edge cases before implementing production code.
2. **Implement Within Strict Scope:** Modify **only** files permitted by `index.md` `Puede modificar` and the active chunk's `Salidas` (never touching `[x] hecho` deliverables or `Fuera de Alcance`).
3. **Run `gengis.exe check` Autonomously (No Human Required):**
   ```powershell
   & "C:\Black Factory\GengiSWAP\gengis.exe" check "C:\Desarrollo\GengiSWAP_data\gengis_task\<TASK_ID>" --root "C:\Black Factory\GengiSWAP"
   ```
   `gengis.exe check` automatically verifies scope, commits in-scope files (`"gengis: <TASK_ID>/<chunk> intento <N>"`), runs every `Comando de Verificación`, writes `workspace/qa/<chunk>.md`, updates `.ring/state.json`, regenerates `status.md`, and logs `[CHEF] ENTREGADO` + `[MOTOR] VERIFICACION`.
4. **Stop at `[~] entregado`:** Do not start the next chunk until QA + `[AUTOR] GO_NEXT`.

---

## Phase C — Goldfish Diff QA & `[AUTOR]` Decision (Modo QA)

When a chunk reaches `[~] entregado`:

1. **Read-Only Invariant:** Never edit source code or deliverables during Modo QA.
2. **Check (a) Independent Re-run & Check (b) Scope:** Re-run the chunk's `Comando de Verificación` and inspect `git show --name-only --pretty="" <commit>` against `## Alcance` + `Fuera de Alcance`.
3. **Check (c) G5 + Clean-Context Goldfish Diff Audit:**
   - Capture the chunk diff (`git diff <base>..HEAD`).
   - Spawn fresh-context Goldfish reviewers (`egm-codebase-details` and `egm-design` via `invoke_subagent`) passing **only** the approved design doc (`Docs/designs/<module>.md`), the active chunk file, and the real `git diff` (no parent conversation history).
   - Verify at least 3 `archivo:línea` claims directly against disk (`G5`) and confirm zero `G4` leaks and zero Option i tracking violations.
4. **Write QA Report (`G2` Header):** Update `workspace/qa/<chunk>.md` with `Repo`, `Rama`, `HEAD`, verdict (`PASS` or `FAIL`), checks `(a)`, `(b)`, `(c)`, Goldfish findings, and literal `G1` evidence.
5. **Hand Off Single-Line `[AUTOR]` Command:** Provide the exact single-line `gengis.exe decide` command (never split `--qa-notes` across lines):
   ```powershell
   & "C:\Black Factory\GengiSWAP\gengis.exe" decide "C:\Desarrollo\GengiSWAP_data\gengis_task\<TASK_ID>" --root "C:\Black Factory\GengiSWAP" --qa-pass=true --qa-notes="Modo QA PASS: ..."
   ```

---

## Phase D — Post-Task Closure

After `[MOTOR] FIN | Tarea: <TASK_ID> | todos los chunks aprobados`:
1. Run `/egm:egm-drift-report Docs/designs/` and `0_Brain/brain.exe verify --vault "C:\Black Factory\0_Brain\vault" --repo "black-factory=C:\Black Factory" --json`.
2. Confirm Option i privacy (`git status --ignored -s`) before merging `feat/<module>` into `main`.
