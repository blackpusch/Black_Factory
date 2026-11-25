# Informe Consolidado de Revisión Goldfish (Etapas 5–9) — `Docs/designs/2_spec_writer.md`

* **Documento auditado:** `C:\Black Factory\Docs\designs\2_spec_writer.md` (`Status: Proposed`, 691 líneas)
* **División semántica:** `FeatureRange = 1..475` | `DetailedRange = 476..691`
* **Modo ejecutado:** Flujo completo ininterrumpido (Etapas 5, 6, 7, 8, 9 — 10 subagentes Goldfish independientes en contexto limpio sobre `flash`)
* **Estado de revisores:** **10/10 `completed`** (0 `not run`)
* **Veredicto:** **FAIL — NO PROMOCIONAR A `Approved`**
* **`unresolved_blockers`: `8`** (más `17` 🟡 Majors consolidados y `11` 🟢 Nits consolidados)

---

## Resumen de Cobertura de Revisores

| Etapa | Revisor (`TypeName`) | Alcance | Estado | 🔴 Blocker | 🟡 Major | 🟢 Nit |
| :---: | :--- | :--- | :---: | :---: | :---: | :---: |
| 5 | `egm-zoom-out` (`41de3795`) | `1..691` | `completed` | 0 | 5 | 2 |
| 5 | `egm-inventive-catalyst` (`efc5413d`) | `1..691` | `completed` | 1 | 6 | 1 |
| 6 | `egm-comprehension` (`abf71761`) | `1..475`, `476..691` (sin contexto externo) | `completed` | 3 | 8 | 1 |
| 7 | `egm-design` (`7ad3b644`) | `1..475` | `completed` | 3 | 7 | 2 |
| 7 | `egm-structural` (`933c1e0e`) | `1..475` | `completed` | 0 | 5 | 3 |
| 7 | `egm-style` (`0f18ae14`) | `1..475` | `completed` | 0 | 9 | 2 |
| 8 | `egm-zoom-out` (`db83cc82`) | `1..691` + informes 5–7 | `completed` | 3 | 5 | 1 |
| 9 | `egm-implementation-readiness` (`45d59da1`) | `476..691` (+ refs `1..475`) | `completed` | 5 | 5 | 1 |
| 9 | `egm-codebase-details` (`d63d4eb1`) | `476..691` (+ citas `1..691` en disco) | `completed` | 3 | 3 | 2 |
| 9 | `egm-style` (`e8185cfc`) | `476..691` | `completed` | 0 | 7 | 6 |

---

## 🔴 Bloqueantes Consolidados (`unresolved_blockers: 8`)

### B-01 — Colisión `G4` (`win_user_path`) vs. `TargetRepo` (`extractTargetRepo`) en `index.md` bajo Windows `t.TempDir()` / `C:\Users\...`
* **Fuentes:** `IC-1`, `DC-B1`, `IR-B1`, `CD-B1`, `S8-B1` (5 revisores independientes)
* **Líneas:** `L14, 63, 227, 265, 326, 336, 379, 445, 558, 641-642, 649`
* **Defecto:** `GengiSWAP/engine/cmd/gengis/main.go:67-105` no tiene flag `--repo`; `gengis.exe plan` obtiene `TargetRepo` únicamente desde `index.md` vía `extractTargetRepo` (`GengiSWAP/engine/bridge.go:164-167, 236-308`). `contract_gengis_test.go` (`L379, L649`) exige compilar sobre `t.TempDir()` y verificar `TargetRepo` real sin `ADVERTENCIA: destino sin Git`. Sin embargo: (a) `index.md.tmpl` (`L558`) no especifica escribir `--repo-root` en `## Directorio de Destino Real` ni `compile` valida `--repo-root` contra `spec_context.json.repo_root_sha256`; y (b) en Windows `t.TempDir()` cae bajo `C:\Users\<user>\...`, disparando `reWinUserPath` (`pkg/bfcore/g4.go:41, 62-66`) cuando `compile` ejecuta `ScanG4All` sobre el buffer renderizado de `index.md` (`L336, L641`). Esto produce un bloqueo determinista en los tests de contrato y en cualquier repo bajo `C:\Users\...`.
* **Corrección requerida:** Formalizar la frontera en dos zonas: (1) exigir `--repo-root` en `validate` y `compile` verificando `repo_root_sha256 == spec_context.repo_root_sha256` y `ScanTier1` + `CheckNoReparseChain` sobre `--repo-root`; (2) ejecutar `ScanG4All` sobre todos los artefactos en `runtime/` y sobre `index.md` renderizado con marcador `<TARGET_REPO>` **antes** de sustituir la única línea de cabecera `- **Repositorio destino:** <absRepoRoot>` en `## Directorio de Destino Real` del archivo externo `gengis_task/<ID>/index.md`.

### B-02 — Bloqueo (`deadlock`) de `external_cycles == 3` en `compile` paso `[5]`
* **Fuentes:** `C-1`, `DC-B3`, `S8-B3`, `ZO-4` (4 revisores independientes)
* **Líneas:** `L20, 30, 63, 270, 274, 291, 300, 325, 609, 612, 637`
* **Defecto:** `validate` (`L30, L274, L300, L612`) permite resolver hasta 3 decisiones externas (`external_cycles: 1 → 2 → 3`, bloqueando únicamente el **4.º** disparo con exit `5`). Tras resolver el 3.er ciclo externo (y sin que `--refresh-context` reinicie contadores, `L270`), `validate` supera `[5]` (`L291`) y `[4]`, devuelve exit `0` y persiste `last_passed_sha256`. No obstante, `compile` paso `[5]` (`L20, L63, L325, L637`) rechaza incondicionalmente cuando `external_cycles >= 3` devolviendo exit `5`, bloqueando una spec que acaba de aprobar `validate` y reduciendo el presupuesto efectivo de `external_cycles` de 3 a 2.
* **Corrección requerida:** Retirar el corte incondicional `external_cycles >= 3` de `compile` paso `[5]` (o limitarlo únicamente a `last_passed_sha256 == ""` con `last_exit == 4`), y disparar exit `5` (`MAX_EXTERNAL_CYCLES`) en `compile` paso `[4]` únicamente si ocurre un **4.º** evento externo (`external_cycles >= 3`), alineando exactamente `compile` con `validate` y `L30`.

### B-03 — Inversión de parseo JSON de `spec.json` respecto a la precedencia `2 → 5 → 3 → 6 → 4 → 1`
* **Fuentes:** `C-2`, `S8-B3`, `IR-M3` (3 revisores independientes)
* **Líneas:** `L286-320, 323-336, 608-621, 635-641`
* **Defecto:** La precedencia coloca `[5]` (`attempts.json`), `[3]` (`G4` Fase-B sobre strings JSON decodificados) y `[4]` (`observed_files[]`, `lane`, `can_modify`, `decisions[]`) antes de `[1]` (`DisallowUnknownFields()`). En `compile` paso `[5]` (`L325, L637`), se necesita `spec.lane` (`draft` vs. `final`) antes de decodificar `spec.json` (y leer `spec_context.work_item.lane` falla si hubo ascenso `FTD → ASDD`). En `validate`, tampoco se define qué código de salida ocurre si `--spec` no existe en disco o si `spec.json` / `goldfish_review.json` tiene JSON sintácticamente inválido al intentar `[3]` Fase-B o `[4]`.
* **Corrección requerida:** Definir el contrato exacto de lectura y parseo: (1) `--spec` inexistente → exit `2` (o `6` `IO_ERROR` si falla permiso); (2) en `compile` `[5]`, comprobar `draft_attempts >= 3 || final_attempts >= 3` (o inferir etapa tras `[3]` si la comprobación es por etapa); (3) en `[3]`, correr Fase-A sobre bytes crudos y luego decodificar JSON — si la sintaxis JSON es inválida, devolver inmediatamente exit `1` (`INVALID_JSON`, incrementando `stage_attempts`) sin ejecutar `[4]` ni consumir `external_cycles`; y en `[1]` aplicar `DisallowUnknownFields()` y reglas de esquema/semántica.

### B-04 — Parche de 5 capas para prefijos de directorio en `ASDD` (`can_modify`): bypass de `CRITICO`, parser duplicado de `risk_zones.md`, contrato `RiskZonePattern` incompleto y desbordamiento CLI en Windows
* **Fuentes:** `DC-B2`, `IR-B4`, `S8-B2`, `ZO-1`, `IC-5`, `CD-M2`, `SD-M3` (7 revisores independientes)
* **Líneas:** `L208, 242, 252, 304-306, 478, 504, 513, 614-615`
* **Defecto:** Mantener `0_Brain/**` congelado mientras se admiten prefijos de directorio `/` en `ASDD` `can_modify` genera defectos estructurales y de seguridad:
  1. `0_Brain/engine/query.go:132-144` no devuelve filas no coincidentes de `wiki/risk_zones.md` (`GengiSWAP/__new__` no coincide con `GengiSWAP/engine/` `RZ-001` `CRITICO`, y >128 archivos ordenados alfabéticamente ocultan `0_Brain/engine/` `RZ-002`).
  2. `ParseRiskZonesCoverPatterns(vaultRoot)` (`L513`) duplica `0_Brain/engine/query.go:186-332` (`ParseRiskZonesCatalog`, violando `Docs/designs/1_intake.md:236`) y omite definir los campos de `RiskZonePattern`, verificación de frontmatter `type: risk_zones` / `status: active` y el algoritmo exacto de intersección bidireccional prefijo-vs-glob.
  3. `MaxDirExpansionFiles = 128` sin indicar tope global vs. por `work_unit` puede exceder el límite Windows `CreateProcess` de 32,767 caracteres en `--files` y trunca silenciosamente cobertura `wiki`/`adr`.
  4. `path.Clean` (`1_Intake/engine/paths.go:106`) borra `/` final (confundiendo directorio con archivo sin extensión), y omitir cualquier subdirectorio llamado `runtime/` (`L242, L614`) ignora rutas legítimas como `pkg/runtime/` o intentos sobre `1_Intake/runtime/`.
* **Corrección requerida:** Elegir explícitamente una de dos vías limpias:
  - **Vía A (Recomendada por YAGNI para v1):** Exigir rutas de archivo concretas en `work_units[].can_modify` tanto en `FTD` como en `ASDD` (igual que `1_Intake` `affected_paths`), eliminando `AllowDir`, `ParseRiskZonesCoverPatterns`, `WalkDir(128)`, `<prefijo>/__new__` y `CheckNoReparseUpToNearestExisting`.
  - **Vía B (Si prefijos de directorio en `ASDD` son obligatorios en v1):** Definir completamente `RiskZonePattern`, levantar el congelamiento de `0_Brain` en Fase 0 para compartir un único parser `ParseRiskZonesCatalog` en `pkg/bfcore`, especificar el algoritmo bidireccional exacto, fijar tope global `<= 128` archivos / `<= 16 KiB` con escalado explícito si trunca (nunca silencioso), registrar `IsDirPrefix` antes de `path.Clean` y anclar exclusiones `WalkDir` a rutas raíz exactas.

### B-05 — Fallo en Windows `cmd.exe` al serializar `verify_steps` con `/` (`0_Brain/brain.exe`, `cd /d <cwd>`) y orden no determinista de `env`
* **Fuentes:** `IR-B2`, `CD-B3`, `IC-3`, `DC-M2`, `S8-M3` (5 revisores independientes)
* **Líneas:** `L347-362, 620, 641-642`
* **Defecto:** `GengiSWAP/engine/runner_windows.go:23` ejecuta `cmd.exe /S /C "<cmd>"`. El allowlist (`L351-353, L620`) usa barras `/` (`0_Brain/brain.exe`, `1_Intake/intake.exe`, `2_Spec_Writer/specwriter.exe`) y `cwd` relativo con `/` (`"0_Brain/engine"`), pero `L361` solo envuelve en comillas si hay espacios y nunca convierte `/` → `\`. En `cmd.exe`, tanto `0_Brain/brain.exe` como `cd /d 0_Brain/engine` fallan (`/` se interpreta como modificador de comando). Además, iterar `map[string]string` en `env` sin ordenar rompe el determinismo, y combinar `cwd != ""` con ejecutables relativos a raíz (`0_Brain/brain.exe`) rompe la resolución de ruta tras `cd /d`.
* **Corrección requerida:** Especificar el algoritmo determinista de serialización `cmd.exe` en `compile.go`: ordenar claves `env` lexicográficamente (validando valores seguros), convertir `cwd` y `argv[0]` con `filepath.FromSlash` (`/` → `\`), entrecomillar **siempre** `cd /d "<cwd>" &&` cuando `cwd != ""`, prohibir `cwd != ""` con ejecutables relativos a raíz (`0_Brain/brain.exe`, etc.), y reforzar validación de argumentos/banderas (ver también `M-03`).

### B-06 — Directiva `//go:embed` inválida hacia `../schemas/spec.schema.json` y falta de decisión en `Target Files`
* **Fuentes:** `IR-B3`, `CD-B2`, `ZO-5`, `C-11`, `SD-M1`, `S8-N1` (6 revisores independientes)
* **Líneas:** `L155-164, 549, 552-558`
* **Defecto:** `2_Spec_Writer/engine/embed.go` no puede compilar `//go:embed ../schemas/spec.schema.json` (Go prohíbe `..` en `go:embed`). Dejar `"(o copia sincronizada)"` (`L557`) viola el criterio de cero decisiones abiertas en el Plan Detallado y omite la ruta exacta en `Target Files`; además `go.mod` (`L556`) no incluye librería JSON Schema porque `validate.go` valida con `DisallowUnknownFields()` sobre structs Go (igual que `1_Intake`).
* **Corrección requerida:** Cerrar la ambigüedad eligiendo una sola opción: (a) embeber **únicamente** `templates/*.md.tmpl` en `embed.go` (manteniendo `2_Spec_Writer/schemas/spec.schema.json` como contrato canónico verificado en tests, patrón idéntico a `1_Intake`), **o** (b) declarar explícitamente `2_Spec_Writer/engine/schemas/spec.schema.json (Create)` en `Target Files` y añadir test de paridad byte a byte contra `2_Spec_Writer/schemas/spec.schema.json`.

### B-07 — Falso `ESCALATION_REQUIRED` en `WorkItem` `ASDD` multi-módulo inicial, colisión sin precedencia con `LANE_UPGRADE_REQUIRED` y hueco de descenso `ASDD → FTD` / `CRITICO` en `FTD`
* **Fuentes:** `IR-B5`, `DC-M4`, `ST-M4`, `C-6` (4 revisores independientes)
* **Líneas:** `L103-109, 225, 244, 307-310, 615`
* **Defecto:**
  1. `L307` y `L615` disparan `ESCALATION_REQUIRED` cuando `len(RootModules) > 1` sin `decisions[]`, lo que bloquea erróneamente cualquier `WorkItem` que **ya entró como `ASDD`** desde `1_Intake` con `>= 2` módulos raíz en `affected_paths` y cero expansión de alcance.
  2. Cuando un `WorkItem` `FTD` añade un 2.º módulo raíz (`CUJ 2`, `L103-109`), coinciden simultáneamente `ESCALATION_REQUIRED` y `LANE_UPGRADE_REQUIRED` sin precedencia declarada en `[4]`.
  3. Nada en `[1]`/`[4]` impide degradar un `WorkItem` `ASDD` a `spec.lane == "FTD"`, ni impide compilar como `FTD` una expansión que toque `CRITICO` (`RZ-001`) una vez cubierta `decisions[]`, ni valida `ComputeScopeSHA256(approved_paths) == scope_sha256` y `can_modify ⊆ approved_paths`.
* **Corrección requerida:** Acotar `ESCALATION_REQUIRED` a incrementos respecto al baseline (`risk_level` mayor, nueva zona `CRITICO`, o `len(RootModules) > 1` cuando `work_item.lane == "FTD"` o se añade un módulo raíz fuera de `affected_paths`); fijar precedencia interna en `[4]`: `STALE_CONTEXT → ESCALATION_REQUIRED → LANE_UPGRADE_REQUIRED`; bloquear `work_item.lane == "ASDD" && spec.lane != "ASDD"` en `[1]` (`/lane`); exigir `LANE_UPGRADE_REQUIRED` si `spec.lane == "FTD"` y se dispara cualquier regla `ASDD` (incluido `CRITICO`); exigir `decisions[]` válido con verificación de `ComputeScopeSHA256(approved_paths) == scope_sha256 == current_scope_sha256` cuando `work_item.lane == "FTD" && spec.lane == "ASDD"`.

### B-08 — Proceso `Goldfish` y lentes `egm-*` obligatorios en `ASDD` sin definición autocontenida en el documento
* **Fuentes:** `C-3` (vinculado a `ZO-3` y `S8-M4`)
* **Líneas:** `L19, 62, 106, 276-282, 318, 341, 451, 621`
* **Defecto:** El flujo `ASDD` (`validate --stage final` y `compile`) exige `goldfish_review.json` con lentes específicos `egm-design`, `egm-codebase-details`, `egm-implementation-readiness`, `egm-structural` y `egm-zoom-out`, pero el documento nunca define qué es `EGM` / `Goldfish`, qué verifica cada lente, quién produce el informe `report_path` ni cómo se genera `goldfish_review.json`. Adicionalmente (`ZO-3`, `S8-M4`), codificar nombres internos de subagentes del plugin EGM en el binario Go acopla la CLI de la factoría a una herramienta externa.
* **Corrección requerida:** Definir explícitamente el proceso de revisión `EGM / Goldfish` (y `FTD` / `ASDD` en el glosario de `Overview`), y decidir si `validate.go` conserva la lista `egm-*` (documentada en la spec) o se desacopla en Go validando únicamente `{spec_sha256, unresolved_blockers == 0, lenses_run (non-empty), report_path}` mientras `SKILL.md` prescribe los lentes concretos.

---

## 🟡 Hallazgos Mayores Consolidados (`17`)

| ID | Tema | Líneas | Fuentes |
| :--- | :--- | :--- | :--- |
| **M-01** | `chunks/NN_<id>.md` emite `## Salidas` en vez de `## Puede modificar` — `GengiSWAP/engine/bridge.go:523-527` ignora `salidas` y desactiva `Chunk.Scope.CanModify` (`ring.go:127-132`); además `## Fuera de Alcance` mezcla `can_modify` de chunks con `depends_on` y `validate` no impide solapamiento con `read_only_paths` / `forbidden` (y `index.md` `cleanScopeItems` parsea `/` fuera de backticks). | `232-233, 252, 254, 312-318, 379, 558` | `CD-M1`, `DC-M3`, `IC-4`, `C-9`, `S8-M1`, `IR-M4` |
| **M-02** | `0_Brain/brain.exe verify` NO es de solo lectura (`0_Brain/engine/cmd/brain/main.go:313-323` llama `RecoverJournal`, contradiciendo `1_intake.md:52, 253`) y debe eliminarse del allowlist read-only de `verify_steps`. | `351, 355, 446, 620` | `DC-M1`, `SF-M2`, `IC-8`, `S8-M3` |
| **M-03** | Brechas de inyección en el allowlist `verify_steps`: `go build` sin `-o` escribe binarios en `cwd`; `-ldflags=-extld=...`, `-gcflags`, `-asmflags`, `-modfile`, `-C`, `go test -test.cpuprofile=...`, `git diff --output`/`--ext-diff`/`--no-index`/`-C` sin bloquear; `cmd.exe` falla con `\t`, `\` final, `""`, `( ) ; '`; valores `env` sin validar (`CGO_ENABLED=1`). Preferir **allowlist** de banderas/caracteres seguros. | `345-362, 446, 620` | `DC-M2`, `S8-M3` |
| **M-04** | Contradicción en nombre y ciclo de vida del cerrojo de `compile`: `.compile.lock` (`L99, 337, 447`) vs. `.<task-id>.compile.lock` (`L329, 639`, riesgo de huérfanos/unlink en Windows `LockFileEx`), más falta de orden global de cerrojos (`.wi.lock → .compile.lock → .events.lock`), creación previa de `<task-root>` y cuotas de tamaño (`MaxSpecBytes`). | `99, 180-185, 210, 329, 337, 447, 639-642` | `SF-M1`, `C-5`, `IC-7`, `DC-M6` |
| **M-05** | `STALE_CONTEXT` (`exit 4`) mezclado con `brain_db_mtime` global (invalidación por cambios ajenos en `0_Brain`), deriva `work_item_ref` tras `--refresh-context` y población/omisión de `observed_files[]` (`baseline_hashes[]` incluye docs de bóveda en `L265` vs `L599`; entrada omitida debe ser `exit 1`, no `exit 4`). | `18, 125-131, 242, 265, 270, 301-303, 599, 601, 613` | `IC-2`, `IR-M2`, `C-8`, `SD-M4`, `DC-M7`, `S8-M5` |
| **M-06** | `GENGIS_ENGINE_DRIFT` (pinning SHA-256 de `bridge.go`/`ring.go`) es acoplamiento frágil a comentarios/formato y redundante frente a `contract_gengis_test.go` (`GENGIS_CONTRACT_STRICT=1`), además de dejar ambiguo qué subcomandos lo ejecutan (`verify` vs `compile`). | `20, 32, 171, 371, 587, 634, 683` | `ZO-2`, `S8-M2`, `SF-M6` |
| **M-07** | Duplicación de lógica `validate` vs. `compile` y `parseChunkFile` (`bridge.go:463-474` `inCoreH2` solo protege líneas con viñeta `- `, no prosa libre con `Clave: val`), lo que difiere errores corregibles al `VALIDATOR_BYPASS_DEFECT` de `compile`. Compartir una única función `validate + chunk-render-check`. | `20, 27, 63, 336, 558, 641` | `ZO-4`, `IC-4`, `IR-M4`, `S8-B3` |
| **M-08** | Superficie HITL desacoplada: `requires_autor_hitl`, `decisions[]`, `risk_mitigations[]` y `scope_expansions` se imprimen solo en `compile` stdout y no aparecen en `gengis.exe plan` (`bridge.go:1500-1519` `printPlanSummary`), la única parada `[AUTOR]` en v1. Inyectar etiqueta compacta en `Objetivo` / 1.ª línea de `Chunk.Spec`. | `15, 64, 70, 230, 244-245, 253, 341, 419` | `IC-6` |
| **M-09** | `specwriter_events.jsonl` carece de guarda obligatoria `bfcore.RedactAllG4` + `ScanG4All` previa a escritura (errores OS/lock con `C:\Users\...` corromperían permanentemente `specwriter verify` con exit `3`). | `181, 271, 372, 445, 453` | `DC-M5` |
| **M-10** | Catálogo `reason`/`retryable` inconsistente e incompleto: `IO_ERROR` marcado `retryable: true` (`L32, 299`) y `retryable: false` (`L372`); `"G4"` (`L120`) vs `"G4_VIOLATION"` (`L295`); faltan `reason` para exit `1`/`2`; faltan definiciones de `VerifyReport`, `PrepareResult`, `SpecWriterEvent` y `last_exit`/`last_reason` en `CLIResult`. | `24-32, 120, 295, 299, 372, 565-566, 588, 655, 683` | `ST-M5`, `SF-M3`, `IR-M1`, `SD-M5`, `C-11` |
| **M-11** | Contrato CLI incompleto: matriz por subcomando (flags obligatorios/opcionales y defaults, `--task-root`, `--brain-bin`), contrato de salida de `render` (`stdout` vs `--json`), salida de `prepare` ante `WorkItem` inválido/`needs_clarification` (exit `2`), y semántica explícita de `compile` paso `[4]` sobre `external_cycles` (no mutar `attempts.json`). | `18-20, 92-94, 117-118, 284-373, 628, 639-640, 655` | `ST-M3`, `SF-M5`, `C-7`, `IR-M3`, `IR-M5`, `SD-M6`, `DC-M7` |
| **M-12** | Asimetrías de `goldfish_review.json` y `risk_mitigations[]`: falta manejo explícito de `goldfish_review.json` ausente en `validate --stage final` (`exit 1`, no `IO_ERROR` `exit 6`), `compile` `[3]` debe escanearlo con `G4`, y falta tipado/referencia de `risk_mitigations[].review_gates[]` y unicidad global de `verify_steps[].id` / `acceptance_criteria[].id`. | `245, 276-282, 293, 326, 610, 619, 621` | `CD-M3`, `IR-M4`, `DC-M7`, `ST-N3` |
| **M-13** | Esquema del esqueleto inicial `spec.json` en `prepare` no especificado (`target_branch`, `template`, `g3_data_mutation`, slices vacíos) y firma `SanitizeRelativePaths(...) ([]string, string)` con segundo retorno sin nombre ni contrato. | `208, 503, 599` | `IR-M2`, `SD-M2` |
| **M-14** | Plan de Verificación `git status --ignored -s 2_Spec_Writer/` (`L681, L683`) no mostrará `!! 2_Spec_Writer/runtime/` cuando el directorio esté vacío/inexistente tras `verify` solo lectura; sustituir/complementar con llamadas individuales `git check-ignore -v`. | `681, 683` | `SD-M7` |
| **M-15** | Estructura del documento: `### Additional Resources` (`L459`) y `### Future Improvements` (`L466`) mal anidados bajo `## Non-Functional Requirements`; falta subsección consolidada `Risks, Limits & Mitigations`; celda `/observed_files[]` (`L242`) sobrecargada con expansión de directorios `can_modify`. | `242, 442-473` | `ST-M1`, `ST-M2`, `SF-M9` |
| **M-16** | Definición incompleta de términos/acrónimos base (`FTD`, `ASDD`, `OKF`, `RZ`, `EGM`, `G4` Tier 1/2) y lenguaje metafórico/no calibrado (`Freno Honesto`, `teatro de seguridad`, `Preparador y Chef`, `arquitectura "sandwich" delgada`, `Recalculado por validate` en `L225`, `SG6` vs NFR `verify` `< 500 ms` + `repositorios de tamaño medio` + `sin superar <= 2`). | `14-17, 36, 64-65, 225, 401, 419-420, 450` | `C-4`, `C-10`, `SF-M4`, `SF-M7`, `SF-M8` |
| **M-17** | Acoplamiento de dos etapas `--stage draft|final` y nombres `egm-*` en Go (`ZO-3`, `S8-M4`, ligado a `B-08`): evaluar simplificación o confirmación explícita de mantener dos etapas por decisión `Q1`. | `19, 62, 79, 272-282, 621` | `ZO-3`, `S8-M4` |

---

## 🟢 Hallazgos Menores Consolidados (`11` Nits)

1. **`templates/*/index.md:14` (`L229`):** Es línea `14` en `modulo_nuevo/index.md:14` y línea `12` en `auditoria_e2e`, `correccion_hallazgos` y `mapeo_estatico` (`CD-N1`).
2. **`EvaluateFTDBounds` y compatibilidad `1_Intake` (`L198-208, 491-503, 526-528`):** Especificar `maxEffective <= 0` → `DefaultMaxFTDFiles`, `len(paths) == 0` → `WithinBounds: false`, `SplitCommas`/`FoldCaseDedupe` en `PathSanitizeOpts` para preservar `sanitized_payload_sha256` y razones exactas de `1_Intake/engine/intake_test.go:326-356`, y rechazo de `"."` (`CD-N2`, `DC-N2`, `IR-N1`, `IC-8`).
3. **`G5` no-código y límite `<= 99` chunks (`L243, 247, 253, 314-315, 618`):** Unificar lista de extensiones/palabras reservadas entre `L315` y `L618`, definir `G5` en `.md`/`.json`, limitar `work_units <= 99` (orden `sort.Strings` en `bridge.go:347`) y exigir `scope_expansions` ante rutas nuevas (`DC-N1`, `C-12`).
4. **Atribución `Rejected by author:` sin enlace (`L390..438`):** Sustituir por redacción técnica directa (`ST-N1`).
5. **Subcadena literal ```` ```mermaid ```` en diagrama `text` (`L314`):** Reescribir como `G2 (cero bloques Mermaid)` (`ST-N2`).
6. **Nombres de símbolos y redacción en Detailed Plan (`L366, 567, 575, 642, 649, 667, 685`):** Unificar `ReadFileWithShortRetry`/`CanonicalSpecSHA256`, separar creación de `workspace/qa/` de `Write+Sync+Close`, aclarar aserción de resumen en `contract_gengis_test.go`, incluir `1_Intake/engine/go.mod` en verificación de 0 SQLite, añadir comando concreto de latencia y nota de compilar `gengis.exe` si falta (`SD-N1..N6`, `IR-N1`).
7. **Helpers de único consumidor en `pkg/bfcore` (`L147, 209-210, 504-505`):** Evaluar mantener `ComputeNormalizedFileSHA256` en `2_Spec_Writer/engine` y exportar solo `NormalizeLineEndings` desde `pkg/bfcore` (`ZO-6`).
8. **Alcance `--repo-root` externo vs. allowlist solo Go/Black Factory (`L347-354, 367`):** Aclarar en `Non-Goals`/Sección 7 que v1 cubre el monorepo Black Factory y repositorios Go (`ZO-7`).
9. **Fraseo y relleno menor en `FeatureRange` (`L14, 46, 56, 62, 90, 97, 120, 130, 139, 151, 200, 230, 257, 273, 456-457`):** Corregir `reparse_other.go`, quitar adverbios de relleno (`limpiamente`, `limpio`, `exactamente`), dividir `L56`, precisar `"sin payload"` (`SF-N1`, `SF-N2`, `C-12`).
10. **Comportamiento de `VerifyRuntime` con `runtime/` inexistente (`L588, 680`):** Indicar explícitamente que pasa (`ok: true`, `0` archivos creados) en checkout limpio (`IR-N1`).
11. **Encabezados H2 exactos en `templates/*.md.tmpl` (`L558`):** Enumerarlos explícitamente para eliminar cualquier ambigüedad frente a `bridge.go` (`IR-N1`).

---

## Matriz de Trazabilidad (Hallazgos Crudos → Ítems Consolidados)

| Ítem Consolidado | Severidad | Hallazgos Crudos de Revisores |
| :--- | :---: | :--- |
| **B-01** (`G4` vs `TargetRepo` en `index.md`) | 🔴 Blocker | `IC-1`, `DC-B1`, `IR-B1`, `CD-B1`, `S8-B1` |
| **B-02** (`external_cycles == 3` deadlock en `compile`) | 🔴 Blocker | `C-1`, `DC-B3`, `S8-B3`, `ZO-4` |
| **B-03** (Parseo JSON `spec.json` vs precedencia) | 🔴 Blocker | `C-2`, `S8-B3`, `IR-M3` |
| **B-04** (`ASDD` dir-prefix patchwork & bypass `CRITICO`) | 🔴 Blocker | `DC-B2`, `IR-B4`, `S8-B2`, `ZO-1`, `IC-5`, `CD-M2`, `SD-M3` |
| **B-05** (`cmd.exe` `/` en `verify_steps` + `env` orden) | 🔴 Blocker | `IR-B2`, `CD-B3`, `IC-3`, `DC-M2`, `S8-M3` |
| **B-06** (`//go:embed ../schemas/` inválido) | 🔴 Blocker | `IR-B3`, `CD-B2`, `ZO-5`, `C-11`, `SD-M1`, `S8-N1` |
| **B-07** (Falso `ESCALATION_REQUIRED` + `ASDD→FTD`) | 🔴 Blocker | `IR-B5`, `DC-M4`, `ST-M4`, `C-6` |
| **B-08** (`Goldfish`/`egm-*` no definido en doc) | 🔴 Blocker | `C-3` (cruzado con `ZO-3`, `S8-M4`) |
| **M-01..M-17** | 🟡 Major | Ver tabla de Mayores arriba (100% mapeados) |
| **N-01..N-11** | 🟢 Nit | Ver lista de Nits arriba (100% mapeados) |
