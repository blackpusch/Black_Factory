# Equipo 4 de Black Factory (`4_Revisores`): Revisión Especializada Multi-Lente en Contexto Limpio

*   **Author:** blackpusch
*   **Status:** Draft (pendiente de confirmación final Goldfish y promoción a `Docs/designs/4_revisores.md`)
*   **Date:** 2026-11-25

--------------------------------------------------------------------------------

## Overview

`4_Revisores` (`4_Revisores/engine` -> `4_Revisores/reviewer.exe`) es la compuerta determinista de aislamiento de contexto, verificación de dictámenes y sellado multi-lente de **Black Factory**, ubicada entre [`3_Builder`](../3_Builder/README.md) (`DeliveryBundle` v1.0) y [`5_Cycle_Approver`](../5_Cycle_Approver/README.md) (`BounceOrder` v1.0). Compilado como binario pure-Go autónomo (`CGO_ENABLED=0`, Windows principal con fallback POSIX, sin SQLite, sin llamadas HTTP a LLM y con `0` bytes modificados en `0_Brain/**`, `1_Intake/**`, `2_Spec_Writer/**`, `3_Builder/**`, `pkg/bfcore/**` y `GengiSWAP/engine/**`), reutiliza `blackfactory/pkg/bfcore` (`golang.org/x/sys v0.49.0`) bajo la arquitectura híbrida **Opción 3C** y privacidad **Opción i**.

En el flujo de la Software Factory, cuando `3_Builder` termina de programar e implementar las pruebas (`builder.exe package`), delega la validación a **cuatro subagentes revisores independientes** ejecutados en el harness externo con **contexto totalmente limpio** (aislados de `3_Builder/runtime/`, `session.json` e historial del Builder) y **diversidad estricta de modelos** para eliminar el sesgo de autoevaluación:

1. **Revisor de Seguridad (`security`):** **Sol** (OpenAI, vía licencia Codex) — audita vulnerabilidades, inyecciones, permisos, `G4`, reparse points y `wiki/risk_zones.md` sin el sesgo de Opus revisándose a sí mismo.
2. **Revisor de UX (`ux`):** **Sol** (OpenAI, vía licencia Codex) — evalúa el flujo de usuario, cantidad de pasos, cumplimiento de directrices de `0_Brain` y claridad para el usuario final.
3. **Revisor de UI (`ui`):** **Astra** (RL enfocado en *computer use*) — valida visualmente la interfaz gráfica real, estados, diseño responsive y accesibilidad.
4. **Revisor de Código y Arquitectura (`code_architecture`):** **Sonnet** (modelo más ligero) — contrasta código, comentarios y evidencia TDD frente a la arquitectura registrada en `0_Brain` con rigor `G5`.

El motor Go `reviewer.exe` expone cuatro subcomandos deterministas (`prepare`, `validate`, `seal`, `verify`) que garantizan mecánicamente: integridad del `DeliveryBundle` y del `.diff`, copia aislada de insumos dentro de `4_Revisores/runtime/` (sin exponer `3_Builder/runtime/` a los revisores), pre-emisión determinista completa de `NA` para ejes con `applicable == false`, barrera `G4` doble fase (`ScanRawAndDecodedG4` implementado localmente en `4_Revisores/engine/state.go` sobre las 11 reglas de `pkg/bfcore/g4.go` — sujeto a consolidación previa en `TD-006`), verificación real en disco del `worktree` para cada cita `file:line` y `evidence_quote` (`G5`), control anti-TOCTOU de `worktree_tree_sha256`, auditoría declarativa de `model_family`, y sellado inmutable `ReviewSet` v1.0 para `5_Cycle_Approver`.

### Glosario Base y Convenciones del Repositorio

*   **Separación Estricta Motor Go vs. Harness Externo (`Opción 3C` + `NG1`):** `reviewer.exe` nunca realiza llamadas HTTP a LLM ni lee `profiles.example.json` en runtime para decidir APIs; las constantes declarativas de política (`conceptual_reference`, `required_model_family`, `checklist`) están embebidas en Go y el enrutamiento real de modelos reside en [`4_Revisores/skills/`](./skills/README.md).
*   **Invariantes Globales (`G1–G6`), `Opción i` y `RZ-001`:**
    - **`G1`:** Evidencia literal capturada en bitácora JSONL y sobres CLI.
    - **`G2`:** Diagramas exclusivamente ASCII / Unicode Box-Drawing (cero Mermaid).
    - **`G3`:** Pruebas sobre directorios temporales `t.TempDir()` (`0` mutaciones sobre la bóveda real o código revisado).
    - **`G4`:** Cero secretos ni rutas personales de host mediante las 11 reglas de `pkg/bfcore/g4.go` (reglas `win_user_path`, `unix_user_path`, tokens, llaves PEM/SSH, correos); doble fase `ScanRawAndDecodedG4` fail-closed (`exit 3`).
    - **`G5`:** Verificación mecánica contra disco del `--worktree` (y líneas `-` de `bundle.diff` en archivos borrados) de cada `file:line` y `evidence_quote` no vacío citado en `findings[]`.
    - **`G6`:** Parada explícita ante deriva del bundle, `0_Brain`, `worktree` o incumplimiento de contrato.
    - **`Opción i`:** `4_Revisores/runtime/` y `4_Revisores/reviewer.exe` excluidos en `.gitignore` (en Fase 2 al integrar `./4_Revisores/engine` en `go.work`).
    - **`RZ-001`:** `GengiSWAP/engine/` permanece 100% intacto (`0` bytes modificados; 5 hashes SHA-256 normalizados vigilados en `reviewer verify`).

### Mapa de Códigos de Salida (`reviewer.exe`, `0..6`)

| Código | Significado | `Reason` exactos | Disparador determinista | `retryable` |
| :---: | :--- | :--- | :--- | :---: |
| `0` | Éxito (`OK`) | `OK` | Operación completada o `seal` idempotente | `false` |
| `1` | Error corregible de dictamen, contrato, diversidad, `G5` o TOCTOU | `VERDICT_VALIDATION_FAILED`, `INVALID_JSON`, `PACKET_TOO_LARGE`, `VERDICT_TOO_LARGE`, `BUNDLE_TOO_LARGE`, `DIFF_TOO_LARGE`, `BUNDLE_HASH_MISMATCH`, `DIFF_HASH_MISMATCH`, `WORKTREE_DRIFT`, `NA_NOT_ALLOWED`, `G5_CITATION_INVALID`, `MODEL_DIVERSITY_VIOLATION`, `UI_EVIDENCE_REQUIRED`, `MISSING_VERDICT`, `VERDICT_SET_MISMATCH` | Fallo de esquema/reglas cruzadas, tamaño excedido, hash `--bundle-sha256` o `.diff` distinto, deriva de `worktree` respecto a `worktree_tree_sha256`/`base_ref`, `NA` inválido o `security`/`code_architecture` con `applicable: false` o LLM sobreescribiendo `NA` determinista, cita `file:line`/`evidence_quote` inválida o vacía tras `TrimSpace`, familia prohibida/`"deterministic"` en eje aplicable, evidencia UI faltante o set incompleto/inconsistente | `true` |
| `2` | Uso CLI inválido o artefacto faltante/duplicado | `INVALID_USAGE`, `BUNDLE_NOT_FOUND`, `PACKET_NOT_FOUND`, `PACKET_ALREADY_EXISTS`, `WORKTREE_REQUIRED`, `UI_EVIDENCE_MISSING` | Banderas faltantes/inválidas (incluyendo `--verdict` ausente o dentro de `absDataDir`, `len(--ui-evidence) > 16`, `--ui-evidence`/`--ui-live-surface` cuando `!ui.applicable`), rutas inexistentes, `prepare` sobre `<bundle_id>` ya existente sin `--force`, `--worktree` ausente o no directorio, `--ui-evidence` ausente o no raster válido | `false` |
| `3` | Rechazo fail-closed `G4` | `G4_VIOLATION` | Violación de cualquiera de las 11 reglas `G4` en `DeliveryBundle`, `.diff`, `ReviewPacket`, `--verdict`, `--builder-family`, rutas `--ui-evidence` o archivos en `runtime/` | `false` |
| `4` | Precondición externa (`0_Brain` obsoleto) | `STALE_BRAIN_CONTEXT` | `risk_zones_sha256`, `highest_risk_level`, `matched_docs` o `risk_hits` difieren entre consulta en vivo `0_Brain` (`bundle.Scope.CanModify`) y `DeliveryBundle.brain_context` | `false` |
| `5` | *(reservado para paridad con Equipos 2 y 3)* | *(no emitido en v1)* | Reservado | `false` |
| `6` | Infraestructura transitoria (`true`) o corrupción/deriva (`false`) | **Transitoria (`true`):** `BRAIN_UNAVAILABLE`, `BRAIN_JOURNAL_ACTIVE`, `LOCK_BUSY`, `IO_ERROR`, `GIT_ERROR` <br>**Negativa/deriva (`false`):** `REPARSE_POINT_DETECTED`, `ARTIFACT_STATE_CORRUPT`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT` | Subproceso `brain.exe`/`git` fallido, diario `.brain-journal.json`, lock ocupado `> 250 ms`, E/S, symlink/junction/reparse point, artefacto en `runtime/` corrupto o deriva en `.gitignore` / `RZ-001` | `true` / `false` |

--------------------------------------------------------------------------------

## The Problem

Una vez que `3_Builder` emite `DeliveryBundle` v1.0 y `.diff` mediante `builder.exe package`, y `5_Cycle_Approver` espera consolidar los cuatro ejes en `BounceOrder` v1.0 (`3_Builder/schemas/bounce_order.schema.json`), quedan **seis brechas operativas** sin cubrir entre Equipos 3 y 5:

1. **Ausencia de contratos formales (`ReviewPacket`, `ReviewVerdict`, `ReviewSet`):** `3_Builder` solo define el bundle de entrega y la orden consolidada final; no existen esquemas propios para paquetes aislados por eje, dictámenes individuales ni manifiesto sellado.
2. **Fuga de aislamiento al leer `3_Builder/runtime/`:** si los subagentes leen el `.diff` directamente desde `3_Builder/runtime/<task_id>/bundles/`, quedan expuestos al directorio hermano que contiene `session.json` y `bounces/*.order.json`, rompiendo la regla de contexto limpio.
3. **Sesgo de autoevaluación y falta de trazabilidad de modelo:** sin auditoría determinista sobre `model_id` / `model_family` y `disallowed_model_families`, el mismo modelo del Builder (`Opus`) podría aprobar su propio código.
4. **Desperdicio o abuso de `NA` en ejes condicionales (`ux`/`ui`):** sin pre-emisión determinista desde `reviewer_applicability`, los modelos pueden inventar `NA` cuando sí hay superficie de usuario o gastar llamadas innecesarias en librerías internas.
5. **Citas `G5` no verificadas, riesgo TOCTOU del `worktree` y fugas `G4`:** un subagente puede alucinar líneas inexistentes, usar `evidence_quote` en blanco, citar un `worktree` modificado tras `package` o introducir rutas personales en `evidence_quote` / `summary` o archivos poison-pill en `runtime/`.
6. **Falta de evidencia visual segura para UI (`Astra`):** `DeliveryBundle` no transporta capturas raster validadas ni distingue sesiones *computer use* en vivo.

**Solución Propuesta:**
Publicar los contratos Draft 2020-12 en `4_Revisores/schemas/`, las cuatro skills progresivas + perfiles en `4_Revisores/skills/`, y el motor determinista `4_Revisores/engine` (`reviewer.exe`, `CGO_ENABLED=0`) con aislamiento físico de `.diff` dentro de `4_Revisores/runtime/`, staging seguro en `validate`, verificación `G5` real en disco + `worktree_tree_sha256` anti-TOCTOU, y sellado `ReviewSet` v1.0.

--------------------------------------------------------------------------------

## Goals & Non-Goals

### Goals
*   **SG1 — Aislamiento Físico y Lógico de Contexto (`ReviewPacket` v1.0):** `prepare` valida `DeliveryBundle` + `.diff` (SHA-256, `SanitizeRelativePaths`, `CheckNoReparseChain` y `G4`), consulta `0_Brain` read-only (`CanModify`), copia el `.diff` a `4_Revisores/runtime/<task_id>/<bundle_id>/bundle.diff` para que ningún subagente toque `3_Builder/runtime/`, emite 4 paquetes aislados y pre-emite `NA` completo válido contra esquema para ejes `ux`/`ui` con `applicable == false`.
*   **SG2 — Validación Staging de Dictámenes (`ReviewVerdict` v1.0) y Rigor `G5` + Anti-TOCTOU:** `validate` lee un archivo staging `--verdict` externo a `absDataDir`, verifica procedencia canónica de `--packet`, re-verifica el `worktree` (`HEAD == base_ref` y `worktree_tree_sha256`), ejecuta `ScanRawAndDecodedG4`, exige `DisallowUnknownFields()` + campos `required` no nulos/no vacíos, prohíbe sobreescribir `NA` determinista, verifica en disco (incluyendo UTF-16LE/BE y `diff --git` para borrados) cada `file:line` y `evidence_quote` (`G5`), y solo si todo pasa promueve atómicamente a `verdicts/<axis>.verdict.json`.
*   **SG3 — Diversidad Declarada de Modelos y Evidencia UI Segura (`Astra`):** `validate`/`seal` normalizan y exigen `model_family` distinto de `disallowed_model_families` y distinto de `"deterministic"` en ejes aplicables (y coincidencia exacta con `required_model_family` si `--strict-model-policy`), y para `ui` aceptan únicamente capturas raster (`.png`, `.jpg`, `.jpeg`, `.webp` validadas por magic bytes) o sesión `--ui-live-surface`.
*   **SG4 — Sellado `ReviewSet` v1.0 Re-Validado para `5_Cycle_Approver`:** `seal` adquiere `.review.lock`, re-valida `bundle.diff`, los 4 paquetes, los 4 dictámenes, `ui_evidence/` y el `worktree`, y escribe `review_set.json` inmutable (idempotente si ya existe idéntico y válido).
*   **SG5 — Privacidad `Opción i`, `verify` Solo Lectura, Cobertura `>= 85%` y `< 500 ms` p95:** `0` rutas absolutas de host en `4_Revisores/runtime/`, `reviewer verify` con `0` escrituras y `0` cerrojos, cobertura Go `>= 85%` y `< 500 ms` p95 local en Windows.

### Non-Goals
*   **NG1 — Llamadas HTTP a LLMs desde Go:** `reviewer.exe` jamás realiza llamadas de red ni almacena claves API; el harness ejecuta a **Sol**, **Astra** y **Sonnet** leyendo `4_Revisores/skills/`.
*   **NG2 — Consolidación, Deduplicación o Emisión de `BounceOrder`:** responsabilidad exclusiva de `5_Cycle_Approver`.
*   **NG3 — Mutar Código del `worktree`, `0_Brain`, `1_Intake`, `2_Spec_Writer`, `3_Builder`, `pkg/bfcore` o `GengiSWAP` (`RZ-001`):** `0` bytes modificados fuera de `4_Revisores/` (salvo el registro de `./4_Revisores/engine` en `go.work` y `4_Revisores/{runtime/,reviewer.exe}` en `.gitignore` durante Fase 2; cualquier consolidación en `pkg/bfcore` sigue `TD-006`).

--------------------------------------------------------------------------------

## Critical User Journeys (CUJs)

### CUJ 1: Flujo Completo en Contexto Limpio (Librería Interna sin UI/UX)
*   **Given:** `3_Builder` emitió `BND-demo-01_core-a1-r0-c0.bundle.json` + `.diff` con `security.applicable = true`, `code_architecture.applicable = true`, `ux.applicable = false`, `ui.applicable = false`.
*   **When:**
    1. Se ejecuta `reviewer.exe prepare --bundle <bundle.json> --builder-data-dir <builder_rt> --worktree <wt> --project black-factory --builder-family anthropic-opus --json`.
    2. El harness lanza en contexto limpio a **Sol** (`security`) con `packets/security.packet.json` y a **Sonnet** (`code_architecture`) con `packets/code_architecture.packet.json`, escribiendo en staging externo `sec.staging.json` y `code.staging.json`.
    3. Se ejecuta `reviewer.exe validate --packet <packets/security.packet.json> --verdict sec.staging.json --worktree <wt> --json` (y análogo para `code_architecture`) y finalmente `reviewer.exe seal --bundle <bundle.json> --worktree <wt> --builder-family anthropic-opus --json`.
*   **Then:**
    1. `prepare` verifica `diff_sha256`, `G4`, `worktree_tree_sha256` y `0_Brain`, copia `.diff` a `runtime/demo/<bundle_id>/bundle.diff`, genera los 4 paquetes y pre-emite completos `verdicts/ux.verdict.json` y `verdicts/ui.verdict.json` en `status: "NA"` (`exit 0`).
    2. `validate` verifica `G4 == 0`, `worktree_tree_sha256`, citas `G5` reales, diversidad `model_family` y promueve atómicamente cada dictamen a `verdicts/<axis>.verdict.json` (`exit 0`).
    3. `seal` re-valida todo el set y emite `review_set.json` (`exit 0`).

### CUJ 2: Revisión Condicional Activa con UI (`Astra`) + UX (`Sol`) y Hallazgo `FAIL` Verificado (`G5`)
*   **Given:** Un `DeliveryBundle` modifica `web/login.tsx` (`ui.applicable = true`, `ux.applicable = true`) y existe `shots/ui_login.png` dentro del `--worktree`.
*   **When:** `prepare ... --ui-evidence shots/ui_login.png` copia la captura raster verificada a `runtime/<task_id>/<bundle_id>/ui_evidence/shots/ui_login.png`; **Astra** detecta solapamiento visual en `web/login.tsx:42` y emite `FAIL` con `ui_evidence_used: ["ui_evidence/shots/ui_login.png"]`.
*   **Then:** `validate` verifica que `web/login.tsx` tenga `>= 42` líneas, que `evidence_quote` coincida en ventana `±3` líneas y que `ui_evidence_used` pertenezca a `ui_evidence_refs`; `seal` emite `review_set.json` con `any_fail: true` (`exit 0`).

### CUJ 3: Bloqueos Anti-Bypass (`G4_VIOLATION`, `G5_CITATION_INVALID`, `WORKTREE_DRIFT`, `MODEL_DIVERSITY_VIOLATION`, `NA_NOT_ALLOWED`)
*   **Given:** (a) Un archivo staging `--verdict` contiene una ruta de usuario (`win_user_path`/`unix_user_path`) en `evidence_quote`; (b) `file: "foo.go", line: 9999` excede líneas reales o `evidence_quote` es blanco o no coincide; (c) el `--worktree` fue alterado tras `package`; (d) `model_family` es `"Anthropic-Opus "` o `"deterministic"` en eje aplicable; o (e) un LLM emite `NA` en `ux` cuando `applicable == true` (o `PASS` cuando `applicable == false`).
*   **When:** Se invoca `reviewer.exe validate`.
*   **Then:** `exit 3` (`G4_VIOLATION`) en (a) sin escribir nada en `runtime/`, y `exit 1` (`G5_CITATION_INVALID`, `WORKTREE_DRIFT`, `MODEL_DIVERSITY_VIOLATION`, `NA_NOT_ALLOWED`) en (b)–(e) manteniendo limpio `verdicts/`.

### CUJ 4: `reviewer verify` 100% Solo Lectura (`Opción i`)
*   **Given:** Se ejecuta `reviewer.exe verify --factory-root . --repo-root . --json`.
*   **When:** `4_Revisores/runtime/` está ausente o contiene paquetes, dictámenes, `.diff`, imágenes raster y sets de múltiples tareas.
*   **Then:** Con `0` escrituras y `0` cerrojos exclusivos, valida reparse points, `.gitignore` (incluyendo línea literal `4_Revisores/reviewer.exe` y `4_Revisores/runtime/`), los 5 hashes `RZ-001` de `GengiSWAP/engine/`, `0_Brain` `Preflight` e integridad/`G4` de cada `.json`, `.jsonl` y `.diff` en `runtime/` (`exit 0`).

--------------------------------------------------------------------------------

## Technical Plan (Architecture)

### 1. Topología del Módulo (`4_Revisores/`)

```text
4_Revisores\
├── README.md                                  (Resumen, límites, CLI y verificación)
├── DESIGN.md                                  (Esta especificación técnica)
├── schemas\
│   ├── review_packet.schema.json              (Entrada aislada por lente v1.0)
│   ├── review_verdict.schema.json             (Dictamen individual v1.0)
│   └── review_set.schema.json                 (Set sellado de 4 dictámenes v1.0)
├── skills\
│   ├── README.md                              (Catálogo y protocolo de contexto limpio)
│   ├── profiles.example.json                  (Opus builder / Sol / Astra / Sonnet)
│   ├── reviewer-security\SKILL.md
│   ├── reviewer-ux\SKILL.md
│   ├── reviewer-ui\SKILL.md
│   └── reviewer-code\SKILL.md
├── engine\                                    (Fase 2: module blackfactory/reviewers)
│   ├── go.mod / go.sum / README.md            (solo blackfactory/pkg/bfcore + golang.org/x/sys v0.49.0)
│   ├── model.go                               (Structs exactos, cuotas, Reason*, IDs acotados, Canonical*SHA256)
│   ├── state.go                               (.review.lock -> .events.lock [250ms], ScanRawAndDecodedG4, WriteJSONAtomicG4)
│   ├── bundle_loader.go                       (Carga + CanonicalBundleSHA256 + diff_sha256 + worktree_tree_sha256)
│   ├── prepare_cmd.go                         (0_Brain read-only + copia .diff/ui_evidence + 4 packets + NA determinista)
│   ├── validate_cmd.go                        (Staging --verdict + G4 + G5 real en disco + diversidad + promoción atómica)
│   ├── seal_cmd.go                            (Re-validación completa + ReviewSet v1.0 inmutable/idempotente)
│   ├── verify.go                              (verify 100% read-only + RZ-001 + .gitignore + runtime audit)
│   ├── reviewers_test.go                      (Suite TDD unitaria de Chunk 1)
│   └── cmd\reviewer\
│       ├── README.md
│       ├── main.go                            (CLI: prepare, validate, seal, verify)
│       └── main_test.go                       (Tests E2E de CUJ 1..4 + TestLatencyBudgetP95 en t.TempDir())
├── reviewer.exe                               (Binario local Opción i; ignorado en .gitignore)
└── runtime\                                   (Estado privado Opción i; ignorado en .gitignore; 0 rutas host)
    ├── .events.lock
    ├── reviewer_events.jsonl                  (Bitácora append-only G4-sanitizada)
    └── <task_id>\<bundle_id>\
        ├── .review.lock
        ├── bundle.diff                        (Copia aislada verificada del .diff del bundle)
        ├── packets\{security,code_architecture,ux,ui}.packet.json
        ├── ui_evidence\<relPath>              (Copias raster locales para Astra)
        ├── verdicts\{security,code_architecture,ux,ui}.verdict.json
        └── review_set.json                    (Salida sellada hacia 5_Cycle_Approver)
```

### 2. Modelos Go, Cuotas, IDs Deterministas, Checklist y Paridad (`model.go`, `bundle_loader.go`)

1. **Cuotas estrictas contra agotamiento de memoria/disco:**
   * `MaxBundleBytes = 4 << 20` (`4 MiB`), `MaxDiffBytes = 2 << 20` (`2 MiB`), `MaxPacketBytes = 4 << 20` (`4 MiB`), `MaxVerdictBytes = 512 << 10` (`512 KiB`), `MaxUIEvidenceFiles = 16`, `MaxUIEvidenceBytes = 10 << 20` (`10 MiB` por imagen raster), `MaxObservedFileBytes = bfcore.MaxObservedFileBytes` (`10 MiB`).
2. **Fórmulas deterministas de IDs acotados (evitan desbordamiento sobre `task_id`/`chunk_id` largos):**
   * Sea `b16 = bundleSHA256[:16]` (16 hex minúsculas):
   * `packet_id`: `fmt.Sprintf("PKT-%s-%s", b16, axis)` (`23..38` chars, patrón `^PKT-[0-9a-f]{16}-(security|code_architecture|ux|ui)$`).
   * `verdict_id` (para `NA` determinista; los subagentes pueden usar el mismo patrón o cualquiera válido `^[A-Za-z0-9][A-Za-z0-9_-]{2,63}$`): `fmt.Sprintf("VRD-%s-%s", b16, axisShort)` donde `axisShort ∈ {"sec","code","ux","ui"}` (`23..25` chars).
   * `set_id`: `fmt.Sprintf("SET-%s", b16)` (`20` chars, patrón `^SET-[0-9a-f]{16}$`).
3. **Paridad exacta de `CanonicalBundleSHA256` y `worktree_tree_sha256` (`bundle_loader.go`):**
   * **`CanonicalBundleSHA256`:** los structs espejo de `DeliveryBundle` (`BrainDocSummary`, `ChangedFileEntry`, `GitBundleInfo`, `ScopeInfo`, `BundleBrainInfo`, `RedEvidence`, `CommandEvidence`, `TDDEvidence`, `AxisApplicability`, `ReviewerApplicability`, `DeliveryBundle`) conservan exactamente el mismo orden de declaración de campos, tags JSON y normalización `nil -> []` (incluyendo `RiskHit.MatchedPaths`) que `3_Builder/engine/model.go:149-281, 532-580`.
   * **`worktree_tree_sha256`:** replica exactamente `3_Builder/engine/git_worktree.go:213-444`:
     1. `CheckNoReparseChain` sobre `repoRoot`, `repoRoot/.git`, `worktree`, `worktree/.git` y cada ruta cambiada (`REPARSE_POINT_DETECTED` `6`).
     2. Rechaza modos Git `120000` (symlink) y `160000` (gitlink) en `git diff --raw -z <baseRef>` y `git ls-files --stage -z`.
     3. Parsea `git status --porcelain=v1 -z --untracked-files=all`: `R` emite destino `"renamed"` + `OrigPath` `"deleted"` (`git_worktree.go:324-330`), mientras `C` y `A` emiten `"added"` sobre `Path` sin marcar `OrigPath` como `"deleted"` (`git_worktree.go:331-334`).
     4. Calcula SHA-256 sobre **bytes crudos** en streaming (sin normalizar BOM ni CRLF, cuota `MaxObservedFileBytes = 10 MiB`), precedencia de modo `rawModes -> stageModes -> fi.Mode()&0o111 -> "0644"|"0755"|"0000"`, ordena por `(Path, Status)` y computa `sha256(baseRef + "\n" + join("status\tmode\tpath\tsha256\n"))`.
4. **Política embebida de modelos y checklists en Go (`TargetModelPolicyForAxis`, `ChecklistForAxis`):**
   * **Política:** `security` (`"Sol (OpenAI / Codex)"`, `"openai-sol"`), `ux` (`"Sol (OpenAI / Codex)"`, `"openai-sol"`), `ui` (`"Astra (RL computer use)"`, `"astra-cu"`), `code_architecture` (`"Sonnet"`, `"anthropic-sonnet"`), con `disallowed_model_families: [normalizedBuilderFamily]` (unificado con `"anthropic-opus"` si distinto).
   * **Checklists embebidos (copiados de `4_Revisores/skills/reviewer-*/SKILL.md`, `ScanG4All == 0`):**
     * `security`: `["Inyecciones shell/SQL/traversal/cuotas", "Regla G4 sin secretos ni rutas de usuario", "Reparse/symlinks, locks y TOCTOU", "Fronteras de confianza y wiki/risk_zones.md", "Cumplimiento estricto de scope read_only/forbidden"]`.
     * `ux`: `["Flujo sin pasos redundantes ni estados ambiguos", "Claridad de mensajes de error, flags y --help", "Consistencia con directrices de 0_Brain y principio G6", "Sin friccion oculta ni supuestos implicitos"]`.
     * `ui`: `["Validacion visual real (alineacion, jerarquia, errores render)", "Estados completos: vacio, carga, exito y error", "Diseno responsive desktop y movil sin desbordes", "Accesibilidad a11y: contraste, etiquetas, foco y teclado", "Adherencia a directrices visuales de 0_Brain"]`.
     * `code_architecture`: `["Alineacion arquitectonica y ADRs de 0_Brain (reuso pkg/bfcore)", "Modularidad sin duplicacion ni manejo silencioso de errores", "Comentarios y docstrings coherentes con el codigo real", "Rigor TDD real en Red y Green sin falsos verdes", "Trazabilidad G5 archivo:linea verificable en disco"]`.
5. **Envelopes JSON CLI (`model.go`):**
   ```go
   type ValidationIssue struct {
   	Pointer string `json:"pointer"`
   	Code    string `json:"code"`
   	Message string `json:"message"`
   }

   type CLIResult struct {
   	OK              bool              `json:"ok"`
   	ExitCode        int               `json:"exit_code"`
   	Reason          string            `json:"reason"`
   	Retryable       bool              `json:"retryable"`
   	TraceID         string            `json:"trace_id,omitempty"`
   	TaskID          string            `json:"task_id,omitempty"`
   	ChunkID         string            `json:"chunk_id,omitempty"`
   	BundleID        string            `json:"bundle_id,omitempty"`
   	BundleSHA256    string            `json:"bundle_sha256,omitempty"`
   	Axis            string            `json:"axis,omitempty"`
   	PacketPaths     map[string]string `json:"packet_paths,omitempty"`     // relativas a dataDir
   	VerdictPath     string            `json:"verdict_path,omitempty"`     // relativa a dataDir
   	NAVerdictPaths  map[string]string `json:"na_verdict_paths,omitempty"` // relativas a dataDir
   	SetID           string            `json:"set_id,omitempty"`
   	ReviewSetPath   string            `json:"review_set_path,omitempty"` // relativa a dataDir
   	AnyFail         bool              `json:"any_fail,omitempty"`
   	RequiresRedTest bool              `json:"requires_red_test_any,omitempty"`
   	Issues          []ValidationIssue `json:"issues,omitempty"`
   }

   type VerifyReport struct {
   	OK               bool              `json:"ok"`
   	ExitCode         int               `json:"exit_code"`
   	Reason           string            `json:"reason"`
   	Retryable        bool              `json:"retryable"`
   	GitIgnoreOK      bool              `json:"gitignore_ok"`
   	GengisHashesOK   bool              `json:"gengis_hashes_ok"`
   	BrainOK          bool              `json:"brain_ok"`
   	G4Issues         int               `json:"g4_issues"`
   	ArtifactsChecked int               `json:"artifacts_checked"`
   	Issues           []ValidationIssue `json:"issues"`
   }

   type ReviewerEvent struct {
   	Timestamp    string `json:"timestamp"`
   	Event        string `json:"event"` // PREPARE | VALIDATE | SEAL | VERIFY
   	TraceID      string `json:"trace_id,omitempty"`
   	TaskID       string `json:"task_id,omitempty"`
   	BundleID     string `json:"bundle_id,omitempty"`
   	BundleSHA256 string `json:"bundle_sha256,omitempty"`
   	Axis         string `json:"axis,omitempty"`
   	ExitCode     int    `json:"exit_code"`
   	Reason       string `json:"reason"`
   	Retryable    bool   `json:"retryable"`
   	DurationMS   int64  `json:"duration_ms"`
   	Detail       string `json:"detail,omitempty"`
   }
   ```
   Sin `--json`, `reviewer.exe` imprime un resumen humano conciso de 1–3 líneas (`[OK|FAIL] <reason> ...`) más `issues` sanitizados `G4`.

### 3. Matriz de Subcomandos CLI, Banderas y Precedencias

Resolución de raíces comunes:
* `--factory-root <dir>` (default `--repo-root` si se pasa, o `.`), `--repo-root <dir>` (default `.`), `--data-dir <dir>` (default `<factory-root>\4_Revisores\runtime`), `--builder-data-dir <dir>` (default `<factory-root>\3_Builder\runtime`), `--vault <dir>` (default `<factory-root>\0_Brain\vault`), `--brain-bin <exe>` (default `<factory-root>\0_Brain\brain.exe`, fallback POSIX `brain`).

| Subcomando | Flags Obligatorios | Flags Opcionales | Efecto en Disco |
| :--- | :--- | :--- | :--- |
| `prepare` | `--bundle <path>`, `--worktree <dir>` | `--builder-family <str>` (default `"anthropic-opus"`), `--bundle-sha256 <hex64>`, `--project <str>` (fallback `risk_hits[0].project` o `""`; recomendado si `risk_hits` está vacío), `--ui-evidence <relPath>` *(repetible)*, `--ui-live-surface`, `--force`, `--builder-data-dir`, `--factory-root`, `--repo-root`, `--data-dir`, `--vault`, `--brain-bin`, `--json` | Copia `bundle.diff`, `ui_evidence/*`, escribe 4 `packets/*.packet.json` y `verdicts/{ux,ui}.verdict.json` si `applicable == false` bajo `.review.lock`. |
| `validate` | `--packet <path>`, `--verdict <staging-path>`, `--worktree <dir>` | `--strict-model-policy` *(exige coincidencia exacta con `required_model_family`)*, `--factory-root`, `--repo-root`, `--data-dir`, `--json` | Lee `--verdict` staging (fuera de `absDataDir`) sin mutarlo; si todo pasa promueve atómicamente a `<data-dir>/<task_id>/<bundle_id>/verdicts/<axis>.verdict.json`. |
| `seal` | `--bundle <path>`, `--worktree <dir>` | `--builder-family <str>` (default `"anthropic-opus"`), `--strict-model-policy`, `--factory-root`, `--repo-root`, `--data-dir`, `--json` | Re-valida `bundle.diff`, los 4 paquetes y 4 dictámenes bajo `.review.lock` y escribe `review_set.json` inmutable (idempotente si es idéntico). |
| `verify` | *(ninguno)* | `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json` | `0` escrituras, `0` cerrojos exclusivos (100% solo lectura). |

**Orden de cerrojos y persistencia (`state.go`):**
* Cerrojo por bundle `.review.lock` en `<dataDir>/<task_id>/<bundle_id>/.review.lock` -> cerrojo de eventos `.events.lock` en `<dataDir>/.events.lock`, backoff acotado `250 ms` vía `bfcore.AcquireFileLock`.
* **Ownership de `ScanRawAndDecodedG4` y `WriteJSONAtomicG4`:** implementados localmente en `4_Revisores/engine/state.go` (reutilizando `bfcore.ScanG4All`, `bfcore.CheckNoReparseUpToNearestExisting`, `bfcore.CheckNoReparseChain` sin modificar `pkg/bfcore/**`; sujetos a unificación futura `TD-006`). `AppendEvent` verifica `CheckNoReparseChain` antes de `O_APPEND` (`0o600`) y valida `ScanG4All == 0`.
* **Saneamiento de rutas en `runtime/` (`SanitizeBundleRelSegments`):** antes de `os.MkdirAll`, valida `bundle.TaskID` (`^[a-z0-9][a-z0-9_-]{2,63}$` sin nombres reservados Windows), `bundle.BundleID` (`^BND-[a-z0-9][a-z0-9_-]{2,63}-.+?-a[1-9][0-9]*-r[0-9]+-c[0-3]$` y `bfcore.SanitizeRelativePaths` como único segmento sin `/` ni `\`), y ejecuta `bfcore.CheckNoReparseUpToNearestExisting(dataDir, bundleDir)`.

**Precedencia paso a paso en `prepare` (`[2] -> [6a] -> [6b] -> [4] -> [3] -> [1] -> [0]`):**
1. **`[2]`** Valida flags (`WORKTREE_REQUIRED` si falta `--worktree` o no es directorio; `INVALID_USAGE` si `--builder-family` vacío, `--bundle-sha256` malformado o `len(--ui-evidence) > MaxUIEvidenceFiles (16)`), `CheckNoReparseChain` en `--bundle`, `--worktree`, `--builder-data-dir`, `--data-dir`, `--vault`, `--factory-root`, `--repo-root` y `worktree/.git`. Verifica existencia/regularidad/tamaño `<= MaxBundleBytes` de `--bundle` (`BUNDLE_NOT_FOUND` `2` / `BUNDLE_TOO_LARGE` `1`). Valida cada `--ui-evidence <relPath>` con `bfcore.SanitizeRelativePaths(..., repoRoot="", FoldCaseDedupe: true)` (`INVALID_USAGE` `2` si hay colisiones o traversal).
2. **`[6a]`** Decodifica `--bundle` con `DisallowUnknownFields()` + campos `required` no nulos, sanea `TaskID`/`BundleID`, sanea `bundle.Git.DiffRelPath` y `bundle.Git.ChangedFiles[].Path` con `bfcore.SanitizeRelativePaths`, comprueba `Security.Applicable && CodeArchitecture.Applicable` (`NA_NOT_ALLOWED` `1` si alguno es `false`), y adquiere `.review.lock` (`LOCK_BUSY` `6`). Si `packets/` ya existe sin `--force` -> `exit 2` (`PACKET_ALREADY_EXISTS`).
3. **`[6b]`** `CheckBrainPreflight` (`BRAIN_JOURNAL_ACTIVE` / `BRAIN_UNAVAILABLE` `6`); resuelve `diffSrcPath = filepath.Join(builderDataDir, filepath.FromSlash(diffRelPath))`, valida `bfcore.CheckNoReparseChain(builderDataDir, diffSrcPath)` + `os.Lstat` regular (`DIFF_HASH_MISMATCH` `1` si falta; `DIFF_TOO_LARGE` `1` si `> MaxDiffBytes`) antes de leer `diffBytes`; y verifica `worktree_tree_sha256` + `HEAD == base_ref` (`WORKTREE_DRIFT` `1`).
4. **`[4]`** Consulta `bfcore.SubprocessBrainClient.Context(vault, "", project, bundle.Scope.CanModify)` (paridad exacta con `3_Builder`) y comprueba `risk_zones_sha256`, `highest_risk_level`, `matched_docs` y `risk_hits` contra `bundle.BrainContext` (`STALE_BRAIN_CONTEXT` `4`). *(Enriquecimiento opcional sobre `nonDeletedChanged` nunca dispara `STALE_BRAIN_CONTEXT`).*
5. **`[3]`** `ScanRawAndDecodedG4(bundleRaw)` + `ScanG4All(diffBytes)` + `ScanG4All(builderFamily)` + `ScanG4All(uiEvidencePaths)` (`exit 3`, `G4_VIOLATION`).
6. **`[1]`** Verifica `CanonicalBundleSHA256(bundle) == --bundle-sha256` (si se pasó) (`BUNDLE_HASH_MISMATCH` `1`) y `sha256(diffBytes) == bundle.Git.DiffSHA256` (`DIFF_HASH_MISMATCH` `1`). Si `!ui.applicable` y se pasó `--ui-evidence` o `--ui-live-surface` -> `exit 2` (`INVALID_USAGE`). Si `ui.applicable == true`: exige al menos un `--ui-evidence` o `--ui-live-surface` (`UI_EVIDENCE_REQUIRED` `1`); cada `--ui-evidence <relPath>` dentro de `--worktree` exige `CheckNoReparseChain`, archivo regular `Size <= MaxUIEvidenceBytes`, extensión `.png`/`.jpg`/`.jpeg`/`.webp` y cabecera de magic bytes raster (`UI_EVIDENCE_MISSING` `2` si falta/inválida).
7. **`[0]`** Si `--force`, limpia antes `verdicts/`, `ui_evidence/` y `review_set.json` dentro de `<bundleDir>`. Copia `bundle.diff`, copia `ui_evidence/<relPath>`, escribe 4 `packets/<axis>.packet.json` (`<= MaxPacketBytes`, `UIEvidenceRefs` con prefijo `"ui_evidence/" + cleanRelPath`) y para `ux`/`ui` con `applicable == false` escribe el veredicto `NA` completo:
   * `verdict_id: "VRD-" + b16 + "-" + axisShort`, `status: "NA"`, `na_reason: cleanNA` (sin `\r/\n`, `<= 300` runas), `model_id: "deterministic-applicability-gate"`, `model_family: "deterministic"`, `requires_red_test: false`, `summary: "NA determinista: " + cleanNA` (truncado `<= 240` runas), `findings_count: 0`, `findings: []`, `checks_performed: [{check_id: "applicability_gate", result: "NA", evidence: cleanNA}]`.

**Precedencia paso a paso en `validate` (`[2] -> [6a] -> [3] -> [1] -> [0]`):**
1. **`[2]`** Valida flags (`WORKTREE_REQUIRED`, `INVALID_USAGE`), ejecuta `CheckNoReparseChain` sobre `--packet`, `--verdict`, `--worktree`, `--data-dir`, `--factory-root`, `--repo-root` y `worktree/.git`; exige que `--verdict` exista, sea archivo regular (`INVALID_USAGE` `2`), no resida dentro de `absDataDir` (`INVALID_USAGE` `2`), y tenga tamaño `<= MaxVerdictBytes` (`VERDICT_TOO_LARGE` `1`); verifica `--packet` `<= MaxPacketBytes` (`PACKET_TOO_LARGE` `1`).
2. **`[6a]`** Lee y escanea `--packet` (`ScanRawAndDecodedG4`), sanea `SanitizeBundleRelSegments(packet.TaskID, packet.BundleID)`, verifica que `filepath.Abs(opts.PacketPath)` coincida exactamente con `<absDataDir>/<task_id>/<bundle_id>/packets/<axis>.packet.json` (`PACKET_NOT_FOUND` `2` / `ARTIFACT_STATE_CORRUPT` `6` si alterado), y adquiere `.review.lock`.
3. **`[3]`** Ejecuta `ScanRawAndDecodedG4(verdictRaw)` (`exit 3`, `G4_VIOLATION` sin escribir nada en `runtime/`).
4. **`[1]`** Valida sintaxis JSON estricta (`DisallowUnknownFields()` + mapa crudo verificando que toda propiedad `required` exista y `v != nil`), `schema_version == "1.0"`, patrón `verdict_id`, `trace_id`/`task_id`/`chunk_id`/`bundle_id`/`bundle_sha256`/`axis` idénticos a `packet`, `worktree` (`HEAD == base_ref` y `worktree_tree_sha256 == packet.Git.WorktreeTreeSHA256`, `WORKTREE_DRIFT` `1`), reglas de aplicabilidad:
   * Si `!packet.Applicability.Applicable`: **solo** válido si `status == "NA" && model_family == "deterministic" && TrimSpace(na_reason) != "" && findings_count == 0 && len(findings) == 0 && !requires_red_test && len(ui_evidence_used) == 0 && all(checks.result == "NA")` (`NA_NOT_ALLOWED` `1`).
   * Si `packet.Applicability.Applicable`: prohíbe `status == "NA"` (`NA_NOT_ALLOWED` `1`); exige `TrimSpace(model_id) != ""` y `TrimSpace(model_family) != ""`; normaliza `fam = strings.ToLower(strings.TrimSpace(model_family))`; rechaza `fam == "deterministic"` o `fam ∈ packet.TargetModel.DisallowedModelFamilies` (y si `--strict-model-policy`, exige `fam == packet.TargetModel.RequiredModelFamily`, `MODEL_DIVERSITY_VIOLATION` `1`).
   * Coherencia de veredicto: `summary` (`1..240` runas sin `\r/\n`, `TrimSpace != ""`), `len(checks_performed) >= 1` (`TrimSpace(check_id/evidence) != ""`; `PASS` prohíbe checks en `FAIL`), `findings_count == len(findings)`, IDs únicos no vacíos en `findings[]`; si `PASS` => `findings_count == 0 && !requires_red_test`; si `FAIL` => `findings_count >= 1`.
   * Coherencia UI: si `axis != "ui"` => `len(ui_evidence_used) == 0`; si `axis == "ui"`: verifica unicidad (`len <= 17`), exige que cada ítem sea elemento de `packet.UIEvidenceRefs` (verificando además su existencia regular no-reparse y magic bytes raster en `<bundleDir>/ui_evidence/`) o `"live_computer_use_session"` (solo si `packet.UILiveSurface == true`), y exige `>= 1` elemento cuando `len(packet.UIEvidenceRefs) > 0 || packet.UILiveSurface` (`UI_EVIDENCE_REQUIRED` `1`).
   * **Verificación `G5` (`verifyFindingG5`):**
     * Rechaza `strings.TrimSpace(f.EvidenceQuote) == ""` (`G5_CITATION_INVALID` `1`). Normaliza `\r\n` / `\r` -> `\n` en cita y contenido.
     * Si `f.File == ""`: solo válido si `f.Line == 0 && f.SymbolAnchor == ""` (`G5_CITATION_INVALID` `1`).
     * Si `f.File != ""`: debe pasar `bfcore.SanitizeRelativePaths`; si figura en `packet.Git.ChangedFiles` con `status == "deleted"`, **no** debe existir en `--worktree` (`G5_CITATION_INVALID` `1`), y si `f.Line > 0` exige validar `CheckNoReparseChain` en `bundle.diff` y que `TrimSpace(f.EvidenceQuote)` aparezca en las líneas `-` de la sección del archivo borrado (`diff --git a/<file> ` / `--- a/<file>` + `+++ /dev/null`); en otro caso (`added`/`modified`/`renamed` o archivo existente), debe existir como archivo regular no-reparse en `--worktree` (`Size <= MaxObservedFileBytes`), decodificar BOM UTF-16LE/BE (`FF FE` / `FE FF`) antes de detectar binario (`0x00` en primeros 8 KiB) -> binario exige `f.Line == 0`, texto `0` bytes exige `f.Line == 0`, texto `> 0` bytes exige `1 <= f.Line <= totalLines` (contando líneas reales tras normalizar `\r\n` sin línea fantasma tras `\n` final) **y** que `strings.TrimSpace(f.EvidenceQuote)` esté contenido en la ventana `[line-3, line+3]`.
5. **`[0]`** Promueve atómicamente vía `WriteJSONAtomicG4` a `<dataDir>/<task_id>/<bundle_id>/verdicts/<axis>.verdict.json` y registra evento `VALIDATE`.

**Precedencia en `seal`:**
* `[2]` flags/reparse (`WORKTREE_REQUIRED`, `INVALID_USAGE`) -> `[6a]` carga y valida `--bundle` idéntico a `prepare` (`BUNDLE_NOT_FOUND` `2`, `BUNDLE_TOO_LARGE` `1`, `INVALID_JSON` `1`, `G4_VIOLATION` `3`), ejecuta `SanitizeBundleRelSegments`, adquiere `.review.lock`, verifica existencia de `bundle.diff`, los 4 `packets/` y 4 `verdicts/` (`MISSING_VERDICT` `1` si falta alguno) -> `[3]` `ScanRawAndDecodedG4` / `ScanG4All` sobre `bundle.diff`, packets y verdicts -> `[1]` verifica `bundle.diff` (`sha256 == DiffSHA256`), verifica consistencia de cada packet contra `--bundle` (`CanonicalBundleSHA256`, `Applicability`, `Git`, `BuilderModelFamily == normalizedBuilderFamily`), re-ejecuta `verifyWorktree` (`WORKTREE_DRIFT`) y todas las reglas de `validate` sobre los 4 ejes + `ui_evidence/`. Si `review_set.json` ya existe: lo decodifica estrictamente (`ScanRawAndDecodedG4`, `DisallowUnknownFields()`, `builder_model_family`), retorna idempotente (`exit 0`) si `bundle_sha256`, `worktree_tree_sha256` y los 4 `verdict_sha256` coinciden exactamente, o `VERDICT_SET_MISMATCH` (`exit 1`) si difieren.

**Prioridad en `reviewer verify` (`0` escrituras, `0` cerrojos):**
* `REPARSE_POINT_DETECTED` (`6`) > `G4_VIOLATION` (`3`) > `GITIGNORE_DRIFT` (`6`) > `GENGIS_ENGINE_DRIFT` (`6`) > `ARTIFACT_STATE_CORRUPT` (`6`) > `BRAIN_JOURNAL_ACTIVE` (`6`) > `BRAIN_UNAVAILABLE` (`6`) > `IO_ERROR` (`6`).
* Comprueba `git check-ignore -q 4_Revisores/runtime/` y `4_Revisores/reviewer.exe` + presencia literal de ambas entradas en `.gitignore` (evitando falso positivo de `*.exe`).
* Comprueba los 5 hashes SHA-256 normalizados `RZ-001` (`GengiSWAP/engine/{bridge.go,ring.go,runner.go,runner_windows.go,cmd/gengis/main.go}`: `8c72badfb4fba74c5f81fd0e5c14687532ea7b5e95b8bc1ec0331390d54beb82`, `8bb4c7bda1ec71536c95e2c57e771d25a0856de7ee9d06327d66dc81fa57b26f`, `776a2858c7e93861539f78361cb9cae56a9682b824e8792688a20ec7`, `d5c289942c01722dd2ab4e92ecafea15f366aaf2c803b1c49c627f9fe1c20d62`, `4ba109ad252503325f726f39dd36166beec3693bcd134f5fc8df051c2cb523a8`).
* Escanea todo `.json`, `.jsonl`, `.diff` y magic bytes raster en `ui_evidence/` bajo `4_Revisores/runtime/` (`G4` + validez JSON/contrato).

--------------------------------------------------------------------------------

## Alternatives Considered

### Alternative A: Delegar en subagentes sin motor Go (`reviewer.exe`) usando solo `SKILL.md`
*   **Pros:** Cero código Go nuevo.
*   **Cons:** Nada garantiza aislamiento de `3_Builder/runtime/`, detección `G4`, verificación real de citas `file:line` (`G5`) ni rechazo de `NA` arbitrarios.
*   **Reason for Rejection:** Rompe el principio base de Black Factory (*las reglas las hace cumplir el motor, no la buena voluntad de ningún modelo*).

### Alternative B: Leer `.diff` directo desde `3_Builder/runtime/` y validar dictámenes en el mismo archivo de salida
*   **Pros:** Evita copiar `bundle.diff` y evita `--verdict` staging.
*   **Cons:** Expone `3_Builder/runtime/<task_id>/session.json` a subagentes de contexto limpio y deja artefactos poison-pill `G4` dentro de `4_Revisores/runtime/` cuando `validate` rechaza un dictamen.
*   **Reason for Rejection:** Copiar `bundle.diff` a `4_Revisores/runtime/` y exigir staging `--verdict` externo a `absDataDir` cierra ambas brechas limpiamente.

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Seguridad y Aislamiento:** `0` rutas absolutas de host en `4_Revisores/runtime/`; `CheckNoReparseChain` obligatorio; raster-only en `ui_evidence` validado por magic bytes; orden estricto `.review.lock -> .events.lock` (`250 ms` backoff).
*   **Presupuesto de Latencia:** `< 500 ms` p95 local en Windows para `prepare` (con `BrainClient` mockeado), `validate`, `seal` y `verify` con `50` artefactos en `runtime/`.
*   **Observabilidad:** Bitácora append-only `4_Revisores/runtime/reviewer_events.jsonl` (`0o600`) sanitizada `G4`.

--------------------------------------------------------------------------------

## Open Questions

Ninguna abierta para Fase 1; la promoción a `Docs/designs/4_revisores.md` y la Fase 2 (`4_Revisores/engine`) esperan la integración de `feat/3-builder` y la consolidación `TD-006` en `pkg/bfcore` si aplica.

--------------------------------------------------------------------------------

## Risks, Limits & Mitigations

| Riesgo / Límite | Impacto | Mitigación Determinista |
| :--- | :---: | :--- |
| Subagente alucina `archivo:línea` o cita vacía/falsa (`G5`) | Alto | `validate` rechaza `TrimSpace(evidence_quote) == ""`, soporta UTF-16LE/BE, comprueba rango real y coincidencia en `±3` líneas (o líneas `-` de `bundle.diff` si `deleted`) |
| Auto-evaluación usando el mismo modelo del Builder (`Opus`) o `"deterministic"` | Alto | `TargetModelPolicy` + `disallowed_model_families` en `validate` y `seal` rechazan coincidencia normalizada con `--builder-family` o `"deterministic"` en ejes activos |
| Fuga de rutas personales o secretos en dictámenes | Crítico | Staging obligatorio externo a `absDataDir` + `ScanRawAndDecodedG4` fail-closed (`exit 3`, `G4_VIOLATION`) antes de escribir en `runtime/` |
| Mutación del `worktree` entre `package` y `seal` (TOCTOU) | Crítico | `prepare`, `validate` y `seal` recomputan `worktree_tree_sha256` y `HEAD == base_ref` (`WORKTREE_DRIFT`, `exit 1`) |

--------------------------------------------------------------------------------

## Detailed Implementation Plan & Verification Plan (Fase 2)

### Chunks Disjuntos para `GengiSWAP` (`bf_04_revisores_v1`)
1. **Chunk 1 (`01_core_packets_and_validate`):** actualizar `go.work` (`./4_Revisores/engine`) y `.gitignore` (`4_Revisores/runtime/`, `4_Revisores/reviewer.exe`); crear `4_Revisores/engine/{go.mod,go.sum,model.go,state.go,bundle_loader.go,prepare_cmd.go,validate_cmd.go,reviewers_test.go}` (`module blackfactory/reviewers`, `go 1.26.0`, solo `blackfactory/pkg/bfcore` + `golang.org/x/sys v0.49.0`, `0` bytes tocados en módulos cerrados).
2. **Chunk 2 (`02_seal_verify_and_cli`):** crear `4_Revisores/engine/{seal_cmd.go,verify.go,README.md,cmd/reviewer/main.go,cmd/reviewer/main_test.go,cmd/reviewer/README.md}` cubriendo CUJs `1..4` en `t.TempDir()`, `.gitignore` + 5 archivos `RZ-001` efímeros en `t.TempDir()` (`G3`) y `TestLatencyBudgetP95` (50 artefactos en `runtime/`).

### Verification Plan (Fase 2)
*   **Compilación y cero deriva en módulos cerrados (`CGO_ENABLED=0`):**
    ```powershell
    $env:CGO_ENABLED = "0"
    Set-Location "C:\Black Factory\4_Revisores\engine"
    go build -o ..\reviewer.exe .\cmd\reviewer
    git -C "C:\Black Factory" diff --name-only 0_Brain/ 1_Intake/ 2_Spec_Writer/ 3_Builder/ pkg/bfcore/ GengiSWAP/engine/
    ```
*   **Análisis estático, cobertura (`>= 85%`) y regresión:**
    ```powershell
    Set-Location "C:\Black Factory\4_Revisores\engine"
    $env:CGO_ENABLED = "0"
    go vet ./...
    go test -count=1 -cover ./...
    go test -count=1 -v -run TestLatencyBudgetP95 ./...
    ```
*   **Salud, privacidad `Opción i` y `RZ-001`:**
    ```powershell
    & "C:\Black Factory\4_Revisores\reviewer.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\4_Revisores\runtime" --json
    git -C "C:\Black Factory" check-ignore -v 4_Revisores/runtime/
    git -C "C:\Black Factory" check-ignore -v 4_Revisores/reviewer.exe
    ```

--------------------------------------------------------------------------------

*Drafted with EGM*
