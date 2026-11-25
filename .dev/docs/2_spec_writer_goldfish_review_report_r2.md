# Informe Consolidado de Re-Auditoría Goldfish — Ronda 2 (`Docs/designs/2_spec_writer.md`)

* **Documento auditado:** `C:\Black Factory\Docs\designs\2_spec_writer.md` (`Status: Proposed`, `783` líneas, tras aplicar los `PROPOSE FIX` de Ronda 1 con `B-04` Opción A)
* **División semántica:** `FeatureRange = 1..541` | `DetailedRange = 542..783`
* **Estado de revisores:** **8/8 `completed`** (0 `not run`)
* **Regresiones sobre los 8 bloqueantes de Ronda 1 (`B-01..B-08`):** **`0`** (todos los bloqueantes de Ronda 1 superaron la re-auditoría)
* **Hallazgos resueltos (`settled`) por `[AUTOR]` en Ronda 1 (no reabiertos):** `M-06a` (`GENGIS_ENGINE_DRIFT` en `verify.go`), `M-17` (`--stage draft|final` y `egm-*` en `goldfish_review.json`), `N-07` (`ComputeNormalizedFileSHA256` en `pkg/bfcore`)
* **Veredicto Ronda 2:** **FAIL — NO PROMOCIONAR A `Approved`**
* **`unresolved_blockers`: `3`** (más `5` 🟡 Majors consolidados y `10` 🟢 Nits consolidados)

---

## Cobertura de Revisores (Ronda 2)

| Revisor (`TypeName`) | Alcance | Estado | 🔴 Blocker | 🟡 Major | 🟢 Nit |
| :--- | :--- | :---: | :---: | :---: | :---: |
| `egm-zoom-out` (`b1fdb155`) | `1..783` | `completed` | 0 | 3 *(2 settled)* | 2 |
| `egm-comprehension` (`c1f70732`) | `1..541`, `542..783` (sin contexto externo) | `completed` | 0 | 2 | 3 |
| `egm-structural` (`c589a1b0`) | `1..541` | `completed` | 0 | 1 | 3 |
| `egm-style` (`b4bd7ac6`) | `1..783` | `completed` | 0 | 2 | 3 |
| `egm-implementation-readiness` (`4665bfc2`) | `542..783` (+ refs `1..541`) | `completed` | 1 | 2 | 1 |
| `egm-codebase-details` (`60b10a87`) | `542..783` (+ citas `1..783` en disco) | `completed` | 1 | 1 | 4 |
| `egm-design` (`ffe74767`) | `1..541` | `completed` | 1 | 4 | 1 |
| `egm-inventive-catalyst` (`a2fbd6fc`) | `1..783` | `completed` | 2 | 3 | 1 |

---

## 🔴 Bloqueantes Consolidados de Ronda 2 (`unresolved_blockers: 3`)

### R2-B1 — Contradicción `-run=^$` vs. prohibición de `^` en `verify_steps` y escape en `cmd.exe /S /C`
* **Fuentes:** `egm-implementation-readiness` (Blocker), `egm-design` (Blocker), `egm-inventive-catalyst` (Blocker)
* **Líneas:** `L386, 401, 406, 706, 721` (y `GengiSWAP/engine/runner_windows.go:23`)
* **Defecto:** `L386` permite `^` dentro de `go test -run=<patrón>` (`^[A-Za-z0-9_^$./-]+$`) y recomienda `go test -run=^$ ./...` para comprobar compilación sin escribir `.exe`, pero `L401` prohíbe incondicionalmente `^` en todo `argv[k]` (y `L406` no entrecomilla `^`, por lo que `cmd.exe /S /C` consumiría `^` como carácter de escape y ejecutaría `-run=$`, corriendo todos los tests).
* **Corrección propuesta:** Mantener `^` incondicionalmente prohibido en `L401` (cero excepciones en `cmd.exe`), actualizar la regex de `-run=` en `L386` a `^[A-Za-z0-9_$./-]+$` y recomendar `go test -run=NoTestsMatch ./...` para verificación de compilación sin ejecutar tests ni escribir `.exe`.

### R2-B2 — Flag inexistente `--query` (en vez de `--q`) en el allowlist cerrado de `0_Brain/brain.exe`
* **Fuentes:** `egm-codebase-details` (Blocker), `egm-design` (Major), `egm-inventive-catalyst` (Major)
* **Líneas:** `L392, 706` (vs. `0_Brain/engine/cmd/brain/main.go:211, 257` y `1_Intake/engine/brain_client.go:199`)
* **Defecto:** `L392` incluye `--query` en el allowlist cerrado de `0_Brain/brain.exe` (`query`, `context`), pero `0_Brain/engine/cmd/brain/main.go:211` define únicamente **`--q`** (`fs.StringVar(&qFlag, "q", ...)`), por lo que `--query` fallaría en tiempo de ejecución y `--q` válido sería rechazado por `validate`. Además `cmdContext` (`main.go:257`) soporta `--file` junto a `--files`.
* **Corrección propuesta:** Sustituir `--query` por `--q` en `L392`, añadir `--file` para `context`, y particionar las banderas permitidas por subcomando (`query` vs. `context`). Adicionalmente (`egm-inventive-catalyst` Nit + `egm-codebase-details` Nit), retirar `verify` de `1_Intake/intake.exe` en `L393` (pues `1_Intake/engine/cmd/intake/main.go:697-700` adquiere `.intake.lock` y escribe evento `VERIFIED` en disco; dejar solo `list`, `show` con `--lane`, `--risk`, `--format`) y añadir `--brain-bin` a `specwriter.exe verify` en `L394`.

### R2-B3 — Paso `[4]` ejecuta `ComputeNormalizedFileSHA256`, `brain.Context` y `EvaluateFTDBounds` antes de validar higiene y no-vacío de `observed_files[]` y `can_modify[]`
* **Fuentes:** `egm-inventive-catalyst` (Blocker), `egm-design` (Major), `egm-implementation-readiness` (Major), `egm-comprehension` (Major)
* **Líneas:** `L216, 228, 331-354, 558, 682, 696-703, 727` (vs. `pkg/bfcore/reparse_windows.go:65-72`)
* **Defecto:** Bajo la precedencia `2 → 5 → 3 → 6 → 4 → 1`, el paso `[4]` corre antes que `[1]`:
  1. `ComputeNormalizedFileSHA256(repoRoot, obs.Path)` no sanea previamente `obs.Path` con `SanitizeRelativePaths` (y `reparse_windows.go:65-72` no valida contención dentro de `repoRoot` ante `../`), permitiendo `Lstat`/lectura fuera de `repoRoot`.
  2. Sobre el esqueleto inicial de `prepare` (`work_units: []`, `L682`) o un borrador `FTD` con `can_modify` vacío o con rutas inválidas, `[4]` llama a `EvaluateFTDBounds([], 5)` que devuelve `WithinBounds: false` (`L216, L558`), disparando un falso **`exit 4` (`LANE_UPGRADE_REQUIRED`)** que consume `external_cycles` en lugar de fallar en `[1]` con `exit 1` (`VALIDATION_FAILED`).
* **Corrección propuesta:**
  1. Exigir que `ComputeNormalizedFileSHA256` valide contención relativa vía `SanitizeRelativePaths` (archivo único dentro de `repoRoot`) antes de `CheckNoReparseChain` y `os.Lstat`.
  2. Antes de evaluar `[4]`, pre-sanear en memoria `observed_files[].path` y `can_modify[]` con `SanitizeRelativePaths`: si `observed_files[]` contiene rutas inválidas, omitir su hashing en `[4]` y dejar que `[1]` devuelva `exit 1`; si `len(can_modify) == 0` o `can_modify` falla `SanitizeRelativePaths`, `[4]` evalúa únicamente `STALE_CONTEXT` sobre `spec_context.json` (`repo_root_sha256`, `baseline_hashes[]`, `risk_zones_sha256`, `matched_doc_hashes[]`) y omite `ESCALATION_REQUIRED`/`LANE_UPGRADE_REQUIRED` para que `[1]` devuelva `exit 1` (`VALIDATION_FAILED`, JSON Pointer `/work_units`) sin consumir `external_cycles`.

---

## 🟡 Hallazgos Mayores Consolidados de Ronda 2 (`5`, excluidos `2` *settled*)

| ID | Tema | Líneas | Fuentes |
| :--- | :--- | :--- | :--- |
| **R2-M1** | Resto en `L251`: `- Prohibido:` decía *"unido con zonas de `wiki/risk_zones.md` que no intersectan `can_modify`"*, lo cual es imposible sin duplicar `ParseRiskZonesCatalog` (`NG6`, `L232`). En v1 `- Prohibido:` compila únicamente `global_forbidden` (`Scope.Allows` ya deniega por defecto fuera de `CanModify`). | `251` | `egm-comprehension`, `egm-design`, `egm-inventive-catalyst` |
| **R2-M2** | `SPEC_TOO_LARGE` en `[2]` rompe precedencia `2 → 5 → 3 → 6 → 4 → 1` y contabilidad de intentos (`L44`), y `CheckNoReparseChain` debe correr sobre `dataDir`/`wiDir`/`specPath` antes de abrir `.wi.lock` o leer `attempts.json`. Mover `MaxSpecBytes`/`MaxGoldfishBytes` al inicio de `[3]` (tras `[5]`), y reflejar `INVALID_JSON`/`SPEC_TOO_LARGE` de `compile` en `L44`. | `44, 318-328, 360-364, 692-695, 723-726` | `egm-structural`, `egm-style`, `egm-design`, `egm-inventive-catalyst` |
| **R2-M3** | Guarda `parseChunkFile` (`bridge.go:458, 463-474, 527-528, 607`): `## Entradas` fija `inCoreH2 = false`, por lo que `parseFieldLabel` se ejecuta incluso en viñetas `- ` de `inputs[]`; extender el rechazo de `^#{1,6}\s`, fences y `parseFieldLabel` (`classifyChunkField != ""`) a `inputs[]` y `acceptance_criteria[].description`, y rechazar `` ` `` final en `L401`. | `353, 401, 707` | `egm-codebase-details` |
| **R2-M4** | Firma elidida `ValidateSpecAndRenderCheck(...)` y structs omitidos (`RenderedChunk`, `RenderedTaskFiles`, `PrepareOptions`, `VerifyOptions`), más `source_workitem_path` faltante en `spec_context.json` (`L283`) para la comparación explícita con `spec.work_item_ref` (`L345, L702`). | `241, 283, 345, 647, 671, 677, 680-682, 701-702` | `egm-implementation-readiness`, `egm-style`, `egm-comprehension`, `egm-structural` |
| **R2-M5** | Mantener los helpers FTS5 exclusivos de `1_Intake` (`ExtractQueryTokens`, `BuildDisjunctiveFTSQuery`, `IsProjectActiveOnDisk`, `Query`/`BrainQueryOpts`) en `1_Intake/engine/` y extraer a `pkg/bfcore/brain_client.go` únicamente el cliente/transporte compartido (`SubprocessBrainClient`, `Preflight`, `Context`, DTOs compartidos de `brain_client.go` y `model.go:84-122`, y sentinels multi-wrap). | `231, 582` | `egm-zoom-out`, `egm-codebase-details` |

---

## 🟢 Hallazgos Menores Consolidados de Ronda 2 (`10` Nits)

1. Eliminar toggles muertos `AllowGlob`/`RejectPersonalG4` en `PathSanitizeOpts` (`L226, 569, 589`) y especificar rechazo de `""` cuando `SplitCommas == false` (`L572-573`).
2. Promover encabezados `#### Precedencia Exacta de validate` y `#### Precedencia Exacta de compile` como hermanos de `#### Matriz de Subcomandos` (`L304-378`) y corregir `\|` dentro de backticks en `L260` y `L274`.
3. Aclarar `Chunk.ID = NN_<id>` en `L266` (`bridge.go:369-370`) y `CheckNoReparseUpToNearestExisting` admitiendo `rootPath == ""` en `L227, L575`.
4. Definir brevemente `RISK_ZONES_UNPARSEABLE_FAILSAFE`, `WorkItem.kind` enum, `4_Revisores`/`Modo_QA` y `extractTargetRepo` (`bridge.go`) (`L17, 274, 342, 515`), y precisar raíz relativa de `matched_doc_hashes[].rel_path` (`--vault`) y `report_path` (`--factory-root`), firma/precedencia de `render` (`L310, 715`), extensiones no listadas en `G5` (`L704`) y ubicación de `trace_id` en `index.md` (`L241` vs `L629`).
5. Eliminar la cláusula redundante `y can_modify ⊆ approved_paths` donde ya se exige igualdad exacta de `scope_sha256` (`L262, 339-340, 699`), y precisar `work_units[].verify_steps[].id` en `L263`.
6. Inicializar explícitamente `narrative.alternatives_considered: []` y `narrative.edge_cases: []` en el esqueleto de `prepare.go` (`L682`).
7. Definir `MaxObservedFileBytes = 10 << 20` en `pkg/bfcore/paths.go` (alias en `model.go`) y señalar tags `omitempty` en `CLIResult` (`L576, 638, 647`).
8. Reemplazar fraseo informal/mezclado (`foto del triaje`, `hueco de abuso`, `cwd MUST be ""`, `sin deadlock` → `L11, 66, 392-394, 453, 484, 735`) y mover expresiones literales de código de `§2`/`§6` a comportamiento (`L218, 232, 405`).
