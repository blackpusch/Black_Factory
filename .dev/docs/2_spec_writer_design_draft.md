# Equipo 2 de Black Factory (`2_Spec_Writer`): Compuerta Determinista de Especificaciones y Compilador hacia `GengiSWAP`

*   **Author:** blackpusch
*   **Status:** Draft
*   **Date:** 2026-11-23

--------------------------------------------------------------------------------

## Overview

`2_Spec_Writer` (`2_Spec_Writer/engine` → `2_Spec_Writer/specwriter.exe`) es la compuerta determinista de especificaciones técnicas y compilador hacia `GengiSWAP` de **Black Factory**. Compilado como binario pure-Go autónomo (`CGO_ENABLED=0`, objetivo principal Windows con fallback POSIX, sin SQLite y sin llamadas HTTP a LLM, compartiendo el paquete `pkg/bfcore` que ya consume `1_Intake/engine/go.mod:5-10` con `golang.org/x/sys v0.49.0`), conecta `1_Intake` (`1_Intake/runtime/workitems/WI-<hash12>.json`, esquema v1.0, mantenido inmutable como foto del triaje inicial) con `3_Builder` (`GengiSWAP/gengis.exe` sobre `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\`) consultando `0_Brain` (`0_Brain/brain.exe`) en modo estrictamente de solo lectura (*Read-Before-Act*).

Antes de detallar los flujos, esta especificación se apoya en las definiciones base del repositorio ([`README.md`](../../README.md), [`Docs/BLACK_FACTORY_BRAIN.md`](../BLACK_FACTORY_BRAIN.md) y [`Docs/designs/1_intake.md`](1_intake.md)):
*   **Invariantes Globales (`G1–G6`):** `G1` (Evidencia literal: comandos, códigos de salida y salidas), `G2` (Trazabilidad y diagramas ASCII/Unicode Box-Drawing, prohibiendo bloques Mermaid), `G3` (Integridad de datos: pruebas sobre `t.TempDir()` sin mutar datos reales), `G4` (Cero fugas de secretos o rutas personales: en `2_Spec_Writer` las **11 reglas de `pkg/bfcore/g4.go:48-115` son fail-closed con exit `3`, sin auto-redacción Tier 2**, permitiendo limpiamente los marcadores `[REDACTED:G4:<rule>]` heredados de `1_Intake`), `G5` (Rigor técnico: citas `archivo:línea` reales verificables en disco), y `G6` (Principio de parada ante ambigüedad o incumplimiento de precondiciones).
*   **Convenciones Arquitectónicas (`Opción 3C`, `Opción i`, `[AUTOR]`, `HITL`, `RZ-001`):** `Opción 3C` combina esquemas/plantillas embebidas (`go:embed`), skills de carga progresiva y un CLI Go determinista; `Opción i` mantiene los directorios operativos locales (`0_Brain/vault/`, `1_Intake/runtime/`, `2_Spec_Writer/runtime/`) y binarios `.exe` fuera de Git vía `.gitignore`; `[AUTOR]` (`blackpusch`) es la autoridad humana de aprobación (`HITL`), ejercida en v1 en una única parada informada en `gengis.exe plan`; `RZ-001` (`GengiSWAP/engine/`) es zona `CRITICO` y permanece intacta en v1.

`2_Spec_Writer` es el primer equipo del pipeline donde participa un LLM —siempre desde el harness externo (Antigravity / CLI), nunca invocado desde Go— operando bajo una **arquitectura "sandwich" delgada** de cinco subcomandos (`prepare`, `validate`, `render`, `compile` y `verify`):
1. **`prepare`:** escanea el `WorkItem` con `ScanG4All` (exit `3`), verifica `status: "ready"`, ejecuta los checks de solo lectura compartidos con `verify` contra `0_Brain` (exit `6`), preserva `requires_autor_hitl: true` sin bloquear, congela `spec_context.json` (hashes SHA-256 sobre bytes normalizados CRLF/BOM, `risk_zones_sha256`, `HEAD` informativo, `trace_id`), inicializa `attempts.json` en `0` y genera el esqueleto `spec.json` bajo `2_Spec_Writer/runtime/<project>/<WI-id>/`. Sobre un WorkItem ya preparado, `--refresh-context` actualiza únicamente `spec_context.json` e invalida los hashes de validación previa sin sobrescribir `spec.json` ni reiniciar contadores; el desbloqueo tras `exit 5` ocurre únicamente vía `--reset-attempts --reason "<motivo>"` (auditado con `G4` y evento `ATTEMPTS_RESET`).
2. **Autoría externa (LLM en harness) + bucle corto con `validate [--stage draft|final]`:** el agente edita exclusivamente `spec.json` (`additionalProperties: false`, única fuente de verdad), registrando `observed_files[]` (`state: "present" | "absent"`, exclusivamente rutas de archivo concretas) y `decisions[]` acotado al alcance aprobado (`approved_paths[]` + `scope_sha256`). `validate` ejecuta la precedencia fija `2 → 5 → 3 → 6 → 4 → 1` con presupuestos independientes por etapa (`draft`: máx. 3, `final`: máx. 3) y `external_cycles` acumulativo (máx. 3), devolviendo errores JSON Pointer, campo booleano `retryable` y persistiendo `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json` al pasar (`exit 0`). `--stage final` es obligatorio únicamente para `ASDD` antes de `compile`; `FTD` compila tras superar `--stage draft`.
3. **`render`, `compile` y `verify`:** `render` valida `G4` antes de proyectar Markdown determinista sellado con `spec_sha256` canónico (nunca re-ingestado). `compile` ejecuta precedencia propia (`2 → 5 → 3 → 6 → 4 → 1`), verifica bloqueo (`stage_attempts >= 3` o `external_cycles >= 3`), impide bypass exigiendo coincidencia con `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json` (`VALIDATION_REQUIRED` vs. `VALIDATOR_BYPASS_DEFECT`), valida el superconjunto de `validateChunks` + allowlist estricto de ejecutables/subcomandos + `G4` en memoria antes de escribir fuera del repo, protege `.ring/state.json` bajo cerrojo SO (exit `6`, `LIVE_TASK_STATE`, `retryable: false`), genera `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\` e imprime el resumen previo a `gengis.exe plan`. `verify` es 100% solo lectura (separando `--factory-root` de `--repo-root`) y mapea deriva de `.gitignore` (`GITIGNORE_DRIFT`) y deriva de `GengiSWAP/engine/{bridge,ring}.go` (`GENGIS_ENGINE_DRIFT`) a exit `6` (`retryable: false`).

### Mapa Comparado de Códigos de Salida en el Pipeline (`1_Intake` vs. `2_Spec_Writer`)

| Código | `1_Intake` (`intake.exe`, `0..5`) | `2_Spec_Writer` (`specwriter.exe`, `0..6`) | Consume intento (`validate`) | Consume ciclo externo | `retryable` (JSON) |
| :---: | :--- | :--- | :---: | :---: | :---: |
| `0` | `ready` / éxito | Éxito (`prepare`, `validate`, `render`, `compile`, `verify`) | No (en `validate`, persiste `last_passed_sha256` + `last_passed_stage` sin resetear `final_attempts`) | No | `false` (N/A) |
| `1` | Error técnico o de uso CLI | Errores corregibles en `spec.json` (`validate`), `VALIDATION_REQUIRED` (`compile`) o `VALIDATOR_BYPASS_DEFECT` (`compile`) | **Sí** en `validate` (`draft` o `final`); **No** en `compile` | No | `true` (`validate`, `VALIDATION_REQUIRED`); `false` (`VALIDATOR_BYPASS_DEFECT`) |
| `2` | Cuarentena `G6` (`needs_clarification`) | Error de uso CLI o argumento sintácticamente mal formado (incl. `--task-id` inválido o `--reset-attempts` sin `--reason`) | No | No | `false` |
| `3` | Rechazo Tier 1 `G4` | Rechazo fail-closed `G4` (las **11 reglas**, sin auto-redacción Tier 2) | **Sí** en `validate`; No en otros subcomandos | No | `false` |
| `4` | `0_Brain` no disponible / lock | Decisión externa requerida (`STALE_CONTEXT`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`) | No | **Sí** (acumulativo máx. 3; el 4.º devuelve `5`) | `false` |
| `5` | Conflicto de deduplicación 48-bit | Tope alcanzado (`MAX_VALIDATE_ATTEMPTS` por etapa o `MAX_EXTERNAL_CYCLES`; incluye `last_exit` y `last_reason` sin payload; desbloqueo con `prepare --reset-attempts --reason`) | No (resultado del tope) | No (resultado del tope) | `false` |
| `6` | *(no usado)* | Infraestructura/entorno (`BRAIN_UNAVAILABLE`, `BRAIN_JOURNAL_ACTIVE`, `LOCK_BUSY`, `IO_ERROR`) **o** negativa deliberada / deriva (`LIVE_TASK_STATE`, `TASK_DIR_EXISTS`, `ATTEMPTS_STATE_CORRUPT`, `ATTEMPTS_STATE_MISSING`, `REPARSE_POINT_DETECTED`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT`) — **ramificar por `reason` y `retryable`** | No | No | `true` (infra transitoria) / `false` (negativa deliberada, estado corrupto o deriva) |

## The Problem

Una vez completados `0_Brain` (memoria persistente OKF/FTS5 y gobernanza Ed25519) y `1_Intake` (puerta perimetral `G4` y triaje determinista `ASDD`/`FTD`, ya implementado en `1_Intake/engine/` y cerrado en `bf_01_intake_v1`), `2_Spec_Writer` existe solo como esqueleto de 5 líneas ([`2_Spec_Writer/README.md`](../../2_Spec_Writer/README.md)), y el traspaso entre especificaciones y `GengiSWAP` depende hoy de redacción manual guiada por prompt ([`.agents/skills/egm-to-gengis/SKILL.md`](../../.agents/skills/egm-to-gengis/SKILL.md)). Sin un motor determinista en `2_Spec_Writer`, la factoría enfrenta seis brechas verificadas en el código:

1. **Verificación dependiente de la buena voluntad del modelo (`constitution.md:22`):**
   Un agente redactando especificaciones libremente puede alucinar citas `archivo:línea` (`G5`), omitir compuertas heredadas de `wiki/risk_zones.md`, introducir bloques Mermaid (`G2`) o colar rutas personales/secretos (`G4`), haciendo que `3_Builder` desperdicie ciclos sobre especificaciones inválidas.
2. **Deriva entre prosa revisada y plan compilado (doble fuente de verdad):**
   Cuando un agente escribe por separado un Markdown de diseño y los archivos `index.md` / `chunks/NN_*.md` de `GengiSWAP`, `[AUTOR]` puede revisar una narrativa que diverge de los alcances o comandos realmente entregados a `gengis.exe`.
3. **Trampas de parseo y desajustes del motor `GengiSWAP` (`bridge.go` / `ring.go` / `runner_windows.go`):**
   La compilación manual en Markdown tropieza con restricciones reales verificadas en disco dentro de `GengiSWAP/engine/`:
   - `gengis.exe status <tmpDir>` (`ShowStatus`, `bridge.go:1455-1488`) llama `ParseTaskFromDir` y `SyncFiles` (sobrescribiendo `<tmpDir>/status.md`), pero **omite `validateChunks`** (`ring.go:393-426`, solo invocado por `PlanTask` en `bridge.go:1227`).
   - `Scope.Allows` (`ring.go:101-124`) evalúa prefijos puros (`under`) y nunca coincide con globs de extensión (`*.go`).
   - `ShellRunner` en Windows (`runner_windows.go:13-25`) ejecuta `cmd.exe /S /C` con timeout fijo de `5 * time.Minute` (`bridge.go:1178`) y fallo fijo cuando `ExitCode != 0` (`ring.go:678-680`). Una denylist de banderas (`-exec`, `-c`) es insuficiente frente a `go build -toolexec`, `go vet -vettool`, `go run`, `go env -w`, `python -c`, `node -e`, `powershell -Command`, `cmd /c` o `git -c core.sshCommand`.
   - `parseChunkFile` (`bridge.go:458-474`) malinterpreta viñetas con `:` cuando no se usan encabezados `##` H2 (`inCoreH2 = true`), y `parseChunksDir` (`bridge.go:331-360`) carga todos los `*.md` de `chunks/`, pudiendo ejecutar chunks huérfanos si se reescribe parcialmente una carpeta.
4. **Riesgo de evasión de carril (`FTD` → `ASDD`), expansión silenciosa de riesgo y desfase en conteo de tests:**
   Un `WorkItem` puede entrar en `FTD` con 1 archivo en `affected_paths` y requerir archivos adicionales que crucen otro módulo raíz, superen el umbral de 5 archivos (`1_Intake/engine/model.go:20`, `triage.go:212`) o toquen `RZ-001` (`CRITICO`). Sin re-consulta obligatoria a `0_Brain`, recálculo compartido en `pkg/bfcore` y vinculación de `decisions[]` al alcance aprobado (`approved_paths`/`scope_sha256`), el agente podría ampliar alcance sin re-disparar escalado. Al mismo tiempo, contar cada `*_test.go` `1:1` degrada correcciones pequeñas en Go a `ASDD`, mientras eximir tests sin tope abriría un hueco de abuso.
5. **Contexto obsoleto en archivos añadidos/nuevos y bypass hacia `compile`:**
   `prepare` solo conoce las rutas del `WorkItem`; cualquier archivo nuevo (`state: "absent"`) o existente (`state: "present"`) añadido en `can_modify` o citado en `G5` durante la redacción quedaría sin control de deriva salvo que `spec.json` registre `observed_files[]` (normalizado CRLF/BOM). Asimismo, sin persistir `(last_passed_sha256, last_passed_stage)` en `attempts.json`, un agente podría editar `spec.json` e invocar `compile` saltándose `validate`.
6. **Falta de trazabilidad `trace_id`, aislamiento `Opción i` y vigilancia de deriva sobre `RZ-001`:**
   `.gitignore` aún no excluye `2_Spec_Writer/runtime/` ni `2_Spec_Writer/specwriter.exe`, no existe registro JSONL con `trace_id` para Equipo 2, y no hay comprobación que detecte cambios en `GengiSWAP/engine/{bridge,ring}.go`.

**Solución Propuesta:**
Construir `2_Spec_Writer/engine` (`module blackfactory/specwriter`, `CGO_ENABLED=0`, cero SQLite, cero llamadas LLM) compartiendo `pkg/bfcore` con `1_Intake` (regla 5B de tests Go con tope duro, higiene de rutas parametrizada y cliente read-only de `0_Brain`), con cinco subcomandos (`prepare`, `validate`, `render`, `compile`, `verify`), `spec.json` (`schema_version: "1.0"`, `additionalProperties: false`) como único SSOT con `observed_files[]` (`present`/`absent`) y `decisions[]` acotado a `scope_sha256`, allowlist cerrado de ejecutables/subcomandos, precedencias explícitas + anti-bypass en `compile`, protección TOCTOU con cerrojo SO sobre `--task-root`, test de contrato obligatorio con `GENGIS_CONTRACT_STRICT=1` contra `gengis.exe plan`, y privacidad total `Opción i`.

## Goals

*   **SG1 — Núcleo Pure-Go sin LLM, sin SQLite y Reutilización en `pkg/bfcore`:** Compilar `2_Spec_Writer/specwriter.exe` (`CGO_ENABLED=0`, integrado en `go.work`, solo `golang.org/x/sys` vía `pkg/bfcore`, plantillas embebidas con `go:embed`) pasando `go vet ./...` y `go test -count=1 -cover ./...` con cobertura `>= 85%` sobre `t.TempDir()` (`G3`). Centralizar en `pkg/bfcore` la función única de evaluación FTD bajo **Opción 5B** (`DefaultMaxFTDFiles = 5` efectivos, `*_test.go` en el mismo directorio que un archivo productivo modificado exento hasta tope duro de `5` tests exentos / `10` archivos físicos totales, `<= 1` módulo raíz sin excepciones, otros lenguajes `1:1` explícito), higiene de rutas parametrizada y cliente read-only de `0_Brain`.
*   **SG2 — Fuente Única de Verdad (`spec.json`) y Proyección Determinista (`render`):** Fijar `spec.json` (`schema_version: "1.0"`, `additionalProperties: false`, sin `extensions`) como único artefacto editado por el agente, incluyendo `observed_files[]` (`present` | `absent`) y `decisions[]` vinculado a `approved_paths[]` + `scope_sha256`, `spec_sha256` sobre serialización JSON canónica, errores con JSON Pointer (RFC 6901), y vista Markdown determinista generada por `specwriter render` (tras escaneo `G4`) sellada con `spec_sha256` en cabecera.
*   **SG3 — Compuerta Estática Fail-Closed (`validate`) con Precedencia Fija, Presupuestos por Etapa y Allowlist de Comandos:** Nunca ejecutar comandos de la spec dentro de `validate`. Aplicar precedencia fija **`2` → `5` (presupuesto independiente `draft`: máx. 3, `final`: máx. 3; `external_cycles` acumulativo máx. 3; `last_exit`/`last_reason` sin payload; fail-closed exit `6` si `attempts.json` falta o está corrupto) → `3` (`G4` fail-closed en las 11 reglas doble fase raw+decodificado) → `6` (infra `0_Brain`/OS) → `4` (decisión externa: `STALE_CONTEXT` CRLF/BOM sobre `spec_context.json` + `observed_files[]` incl. aparición inesperada de `"absent"`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`) → `1` (errores corregibles: esquema, `G2`, `G5` 1-based + ancla `±5`, `risk_mitigations[]`, allowlist cerrado de ejecutables/subcomandos + claves `env`, rechazo de `& | < > ^ % ! " \r \n \x00`, `goldfish_review.json` en `--stage final` para `ASDD` como higiene de consistencia)**. Al pasar (`exit 0`), persistir `(last_passed_sha256, last_passed_stage)` en `attempts.json`.
*   **SG4 — Compilador Determinista (`compile`) con Anti-Bypass, Precedencia Propia, Promoción Atómica Create-Only en Windows y Test de Contrato CI:** Ejecutar precedencia propia **`2` → `5` (rechaza si `stage_attempts >= 3` o `external_cycles >= 3`) → `3` (`G4` en memoria antes de escribir fuera del repo) → `6` (`0_Brain`, reparse points bajo `--task-root`, cerrojo SO, `LIVE_TASK_STATE` si existe `.ring/state.json` o `TASK_DIR_EXISTS` si `<task-root>/<task-id>/` ya existe — modo *create-only* en v1, ambos `retryable: false`) → `4` (`STALE_CONTEXT`/escalado al revalidar) → `1` (`VALIDATION_REQUIRED` `retryable: true` si `spec_sha256`, etapa o `goldfish_sha256` no coinciden con `attempts.json`; `VALIDATOR_BYPASS_DEFECT` `retryable: false` sin consumir intento si falla tras pase válido del mismo hash, registrado en `specwriter_events.jsonl` para generar fixture de regresión)**. Incluir test de contrato contra `gengis.exe plan` obligatorio con `GENGIS_CONTRACT_STRICT=1` antes de merge.
*   **SG5 — Freno Honesto contra Bucles, Desbloqueo Auditado, Una Sola Parada `[AUTOR]`, `verify` Solo Lectura y `Opción i`:** Inicializar `attempts.json` en `prepare`; desbloqueo tras exit `5` únicamente mediante `specwriter prepare --reset-attempts --reason "<motivo>"` (escaneado con `G4`, registrando `ATTEMPTS_RESET` en `specwriter_events.jsonl`), documentando `attempts.json` como freno contra bucles y no control de seguridad. Mantener `gengis.exe plan` como única parada `[AUTOR]` en v1, excluir `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` en `.gitignore`, y exponer `specwriter verify [--json]` 100% solo lectura devolviendo exit `6` (`retryable: false`) ante `GITIGNORE_DRIFT` o `GENGIS_ENGINE_DRIFT`.
*   **SG6 — Observabilidad de Extremo a Extremo y Presupuesto de Latencia:** Propagar `WorkItem.trace_id` hacia `spec_context.json`, `specwriter_events.jsonl` y `gengis_task/<ID>/index.md`, completando `prepare`, `validate`, `render` y `compile` locales en `< 500 ms` p95 en Windows.

## Non-Goals

*   **NG1 — Llamadas a APIs de LLM o Ejecución de Comandos de la Spec dentro de Go:** `specwriter.exe` nunca realiza llamadas de red ni gestiona claves de API, y `validate` nunca ejecuta comandos escritos en `spec.json`.
*   **NG2 — `specwriter approve` y Firma Criptográfica Ed25519 en v1 (Diferido a v2):** Diferido a v2 *(disparador: ejecución desatendida o más de un operador)*. En v1, `gengis.exe plan` es la única parada `[AUTOR]`.
*   **NG3 — `specwriter publish` y `publish_allowlist` en v1 (Diferido a v2):** En v1 todo permanece privado en `2_Spec_Writer/runtime/` (`Opción i`). Diferido a v2 *(disparador: segunda necesidad real de publicación controlada)*.
*   **NG4 — DSL Declarativo Externo de Plantillas y Campo Opaco `extensions` en v1:** Plantillas embebidas con `go:embed`; evolución del esquema solo vía `schema_version`.
*   **NG5 — Wrapper de `BrainPatch` / `brain.exe propose` y Contador FTD de `NO_BRAIN_COVERAGE` en v1:** `1_Intake` ya enruta `NO_BRAIN_COVERAGE` a `ASDD` (`1_Intake/engine/triage.go:222-235`). `spec.json` declara solo `brain_followups[]` post-merge; wrapper diferido a v2 *(disparador: resolución del WorkItem independiente en `0_Brain` por falta de `.brain.lock` y `ScanG4` en `0_Brain/engine/vault.go:244-418`)*.
*   **NG6 — Modificaciones a `GengiSWAP/engine/` (`RZ-001`) o Mutación de `1_Intake` `WI-*.json` / `workitem.schema.json` v1.0:** `GengiSWAP/engine/` y `workitem.schema.json` v1.0 permanecen intactos, y `1_Intake/runtime/workitems/WI-*.json` queda inmutable (`Q4`).

## Open Questions

Ninguna abierta para v1. Las decisiones `Q1–Q4` fueron ratificadas por `[AUTOR]` (`blackpusch`) e incorporadas en el Plan Técnico:
*   **Q1 (Resuelta):** Presupuestos independientes por etapa (`draft`: máx. 3, `final`: máx. 3; pasar `draft` no resetea `final` si se re-ejecuta `draft`), `external_cycles` acumulativo (máx. 3), desbloqueo con `specwriter prepare --reset-attempts --reason "<motivo>"` (escaneado con `G4` + evento `ATTEMPTS_RESET`).
*   **Q2 (Resuelta):** `LIVE_TASK_STATE` permanece bajo `exit 6` con `retryable: false` y catálogo explícito de `reason` (contrato `0..6`).
*   **Q3 (Resuelta):** Opción `5B` en `pkg/bfcore` (`*_test.go` en mismo directorio que un archivo productivo modificado exento hasta tope duro de `5` tests exentos / `10` archivos físicos totales; `<= 1` módulo raíz sin excepciones; otros lenguajes `1:1` explícito).
*   **Q4 (Resuelta):** `decisions[]` local en `spec.json` acotado a `approved_paths[]` + `scope_sha256`, manteniendo `1_Intake/runtime/workitems/WI-*.json` inmutable.

*(Seguimiento fuera de alcance de v1: WorkItem independiente en `0_Brain` para añadir `.brain.lock` y `ScanG4` en `0_Brain/engine/vault.go:244-418`.)*

## Critical User Journeys (CUJs)

### CUJ 1: Flujo Completo `FTD` Ejercitando Opción 5B (5 Productivos + 2 Tests Hermanos) hasta `compile`

*   **Given**: `0_Brain/vault/` está indexado y sano, `1_Intake/runtime/workitems/WI-a1b2c3d4e5f6.json` está en `status: "ready"`, `project: "black-factory"`, `lane: "FTD"`, `risk_level: "ALTO"` (`RZ-002` sobre `0_Brain/engine/`), `affected_paths` contiene `5` archivos productivos dentro de `0_Brain/engine/` (`validator.go`, `index.go`, `query.go`, `drift.go`, `frontmatter.go`), y `2_Spec_Writer/runtime/` está ignorado en `.gitignore`.
*   **When**:
    1. El agente ejecuta `specwriter.exe prepare --workitem "1_Intake/runtime/workitems/WI-a1b2c3d4e5f6.json" --repo-root "C:\Black Factory" --json`.
    2. El agente completa `2_Spec_Writer/runtime/black-factory/WI-a1b2c3d4e5f6/spec.json` con `1` `work_unit` modificando esos `5` archivos productivos más `2` tests del mismo directorio (`validator_test.go`, `index_test.go` — `7` archivos físicos `<= 10`, `2` tests exentos `<= 5`), registra los `7` archivos en `observed_files[]` (`state: "present"` + SHA-256 normalizado CRLF/BOM), enlaza `RZ-002` en `risk_mitigations[]` a un `verify_step` permitido (`cwd: "0_Brain/engine"`, `argv: ["go", "test", "-count=1", "./..."]`, `expect_exit: 0`, `timeout: "2m"`) + `review_gate`, declara `failure_proof` (`expect_exit: 1`) y citas `G5`.
    3. Ejecuta `validate --stage draft --json`, luego `render`, y finalmente `compile --task-root "C:\Desarrollo\GengiSWAP_data\gengis_task" --task-id "bf_fix_brain_104" --json`.
*   **Then**:
    1. `prepare` valida `G4` en el `WorkItem`, ejecuta el preflight compartido de `verify`, congela `spec_context.json` e inicializa `attempts.json` en `0` (exit `0`).
    2. `validate --stage draft` verifica `2 → 5 → 3 → 6 → 4 → 1`: `0` hallazgos `G4`, hashes normalizados intactos, `bfcore.EvaluateFTDBounds` calcula `5` efectivos (`<= 5`), `2` tests exentos (`<= 5`), `7` físicos (`<= 10`), `1` módulo raíz (`<= 1`) → mantiene `FTD`, persiste `(last_passed_sha256, "draft")` en `attempts.json` y sale con exit `0`.
    3. `render` valida `G4` y proyecta Markdown con `spec_sha256` en cabecera (exit `0`).
    4. `compile` verifica `attempts.json` (no bloqueado y `spec_sha256 == last_passed_sha256` con etapa `"draft"` válida para `FTD`), sanea `--task-id`, verifica `G4` en memoria, adquiere cerrojo de carpeta bajo `--task-root`, confirma ausencia de `.ring/state.json`, valida el superconjunto de `validateChunks`, emite atómicamente `index.md`, `chunks/01_*.md` y `status.md` inicial, imprime el resumen previo a `gengis.exe plan` y sale con exit `0`.

### CUJ 2: Escalado de Carril (`ESCALATION_REQUIRED`), `decisions[]` Acotado y `--stage final` Goldfish (`ASDD`)

*   **Given**: Un `WorkItem` llega en `lane: "FTD"` sobre `1_Intake/engine/triage.go`. Durante la redacción, el agente necesita editar también `pkg/bfcore/paths.go` (segundo módulo raíz).
*   **When**:
    1. El agente ejecuta `validate --stage draft --json` con `lane: "FTD"`.
    2. `[AUTOR]` aprueba la escalación registrando una entrada en `spec.json.decisions[]` vinculada al `scope_sha256` de `["1_Intake/engine/triage.go", "pkg/bfcore/paths.go"]` y cambia `lane` a `"ASDD"`. El agente completa las secciones `ASDD`, pasa `--stage draft` (exit `0`), genera `goldfish_review.json` ligado a `spec_sha256`, y ejecuta `validate --stage final --json`.
    3. Si el agente añadiera posteriormente un tercer archivo en `can_modify` fuera de `approved_paths`, ejecuta `validate --stage final --json`.
*   **Then**:
    1. La primera corrida devuelve exit `4` (`ESCALATION_REQUIRED`, `retryable: false`, `external_cycles: 1/3`, sin tocar `draft_attempts`).
    2. Al pasar `--stage draft` (`exit 0`), `final_attempts` arranca limpio (`0/3`). Al pasar `--stage final` (`exit 0`), `attempts.json` guarda `(last_passed_sha256, "final")`, habilitando `compile`.
    3. Si el agente añade un archivo fuera de `decisions[].approved_paths` / `scope_sha256`, `validate` vuelve a disparar exit `4` (`ESCALATION_REQUIRED`). Si intentara ejecutar `compile` sin revalidar el nuevo `spec_sha256`, `compile` devuelve exit `1` (`VALIDATION_REQUIRED`, `retryable: true`).

### CUJ 3: Freno contra Bucles (`G4` → Exit `5`), Desbloqueo Auditado y Protección TOCTOU (`LIVE_TASK_STATE` → Exit `6`)

*   **Given**: (a) Un agente intenta validar cuatro veces seguidas un `spec.json` con ruta personal Windows (`C:\Users\dev\...`); y (b) `C:\Desarrollo\GengiSWAP_data\gengis_task\bf_active_task\.ring\state.json` existe en ejecución por `3_Builder`.
*   **When**:
    1. El agente invoca `validate --stage draft --json` 4 veces sin eliminar la ruta personal, y luego `[AUTOR]` desbloquea con `specwriter.exe prepare --workitem "..." --reset-attempts --reason "Ruta personal eliminada tras revision" --json`.
    2. Un operador invoca `compile --spec "..." --task-id "bf_active_task" --json`.
*   **Then**:
    1. Los intentos 1, 2 y 3 fallan en el paso `3 (G4)` devolviendo exit `3` (`retryable: false`, `0` payload escrito) e incrementando `draft_attempts` al terminar con `last_exit: 3, last_reason: "G4"`. El 4.º intento corta en el paso `5` devolviendo exit `5` (`MAX_VALIDATE_ATTEMPTS`, `last_exit: 3`, `last_reason: "G4"`). El comando `--reset-attempts` valida `--reason` con `G4`, reinicia contadores en `attempts.json`, registra `ATTEMPTS_RESET` en `specwriter_events.jsonl` y devuelve exit `0`.
    2. `compile` adquiere el cerrojo de carpeta bajo `--task-root`, detecta `.ring/state.json`, no toca ningún archivo y sale con exit `6` (`LIVE_TASK_STATE`, `retryable: false`).

### CUJ 4: `specwriter verify` Solo Lectura, Deriva `GITIGNORE_DRIFT` / `GENGIS_ENGINE_DRIFT` y Archivo Nuevo Inesperado (`absent`)

*   **Given**: `2_Spec_Writer/runtime/` contiene specs previas, un archivo declarado en `observed_files[]` con `state: "absent"` aparece inesperadamente en disco antes de `validate`, o se modifica `GengiSWAP/engine/bridge.go` / `.gitignore`.
*   **When**:
    1. Se ejecuta `specwriter.exe verify --vault "..." --data-dir "..." --repo-root "C:\Black Factory" --json`.
    2. Se ejecuta `specwriter.exe validate --spec "..." --stage draft --json` tras aparecer el archivo marcado `"absent"`.
*   **Then**:
    1. `verify` lee sin cerrojos exclusivos ni creación de archivos: si falta `.gitignore` devuelve exit `6` (`GITIGNORE_DRIFT`, `retryable: false`); si cambió el hash de `GengiSWAP/engine/{bridge,ring}.go` devuelve exit `6` (`GENGIS_ENGINE_DRIFT`, `retryable: false`); si halla un secreto en `runtime/` devuelve exit `3`; si todo está sano devuelve exit `0`.
    2. `validate` detecta que el archivo `"absent"` ahora existe en disco y devuelve exit `4` (`STALE_CONTEXT`, `retryable: false`), obligando a refrescar contexto con `prepare`.

--------------------------------------------------------------------------------

## Technical Plan (Architecture)

### 1. Topología del Sistema y Límites de Módulos (`go.work` + `pkg/bfcore` + `2_Spec_Writer`)

`2_Spec_Writer` sigue exactamente la arquitectura híbrida `Opción 3C` + `Opción i` de `0_Brain` y `1_Intake`, añadiendo `./2_Spec_Writer/engine` al archivo raíz [`go.work`](../../go.work) y extrayendo en **Fase 0** las primitivas transversales hacia `pkg/bfcore` (sin SQLite, `CGO_ENABLED=0`, única dependencia externa `golang.org/x/sys v0.49.0`):

```text
Black Factory\
├── go.work                                (Añade ./2_Spec_Writer/engine junto a 0_Brain, 1_Intake, pkg/bfcore)
├── .gitignore                             (Añade 2_Spec_Writer/runtime/ y 2_Spec_Writer/specwriter.exe)
├── pkg\bfcore\                            (Módulo compartido blackfactory/pkg/bfcore — Fase 0)
│   ├── g4.go                              (Existente: 11 reglas G4, ScanG4All, ScanTier1, RedactTier2Text)
│   ├── paths.go                           (Ampliado: SanitizeRelativePaths parametrizado + ComputeNormalizedSHA256)
│   ├── ftd.go                             (NUEVO: Opción 5B EvaluateFTDBounds + CountRootModules + constantes FTD)
│   ├── brain_client.go                    (NUEVO: BrainClient read-only extraído de 1_Intake + ErrBrainJournalActive)
│   ├── lock_windows.go / lock_other.go    (Existente: cerrojo exclusivo SO con backoff)
│   └── reparse_windows.go / reparse_other (Existente: CheckNoReparseChain vía os.Lstat)
├── 1_Intake\engine\                       (Fase 0: delega FTD/CountRootModules/Sanitize/BrainClient a pkg/bfcore)
└── 2_Spec_Writer\                         (Módulo blackfactory/specwriter — Fase 1)
    ├── README.md                          (Documentación de límites, invariantes, comandos y códigos 0..6)
    ├── schemas\
    │   └── spec.schema.json               (JSON Schema Draft 2020-12 v1.0, additionalProperties: false)
    ├── skills\
    │   ├── README.md                      (Catálogo de skills de Equipo 2)
    │   └── spec-authoring\
    │       └── SKILL.md                   (Guía progresiva del agente para el bucle prepare -> validate -> compile)
    ├── engine\
    │   ├── go.mod / go.sum / README.md
    │   ├── embed.go                       (//go:embed de spec.schema.json y plantillas GengiSWAP index/chunk/status)
    │   ├── model.go                       (Estructuras Spec, SpecContext, AttemptsState, GoldfishReview, JSON canonical SHA-256)
    │   ├── state.go                       (Gestión atómica de attempts.json bajo .wi.lock, telemetría JSONL bajo .events.lock)
    │   ├── prepare.go                     (Flujo prepare + --reset-attempts --reason + congelación spec_context.json)
    │   ├── validate.go                    (Pipeline 2->5->3->6->4->1, JSON Pointer, G5 ±5, allowlist comandos, STALE_CONTEXT, Goldfish)
    │   ├── render.go                      (Proyección Markdown ASCII determinista sellada con spec_sha256)
    │   ├── compile.go                     (Pipeline 2->5->3->6->4->1, anti-bypass, validateChunks superset, TOCTOU lock, resumen)
    │   ├── verify.go                      (Chequeos 100% solo lectura compartidos con prepare/compile + hashes RZ-001)
    │   ├── specwriter_test.go             (Tests unitarios TDD: FTD 5B, allowlist, G5, STALE_CONTEXT, TOCTOU, bypass)
    │   ├── contract_gengis_test.go        (Test de contrato contra gengis.exe plan en t.TempDir(); GENGIS_CONTRACT_STRICT=1)
    │   └── cmd\specwriter\
    │       ├── README.md
    │       ├── main.go                    (CLI specwriter.exe: prepare, validate, render, compile, verify; exits 0..6)
    │       └── main_test.go               (Tests E2E de todos los CUJs sobre t.TempDir())
    ├── specwriter.exe                     (Binario compilado local; ignorado en .gitignore)
    └── runtime\                           (Estado operativo Opción i; ignorado en .gitignore)
        ├── .events.lock                   (Cerrojo breve exclusivamente para appends en specwriter_events.jsonl)
        ├── specwriter_events.jsonl        (Bitácora append-only con trace_id, duration_ms y reason_codes)
        └── <project>\
            └── <WI-hash12>\
                ├── .wi.lock               (Cerrojo granular por WorkItem; permite paralelismo por proyecto)
                ├── spec_context.json      (Snapshot inmutable de prepare)
                ├── attempts.json          (Contadores draft/final/external + last_passed_sha256/stage)
                ├── spec.json              (Única fuente de verdad editada por el agente)
                └── goldfish_review.json   (Evidencia requerida en ASDD --stage final; higiene de consistencia)
```

### 2. Contratos Compartidos en `pkg/bfcore` (Fase 0)

Para eliminar duplicación y deriva entre `1_Intake` y `2_Spec_Writer` sin añadir dependencias externas:

1. **Evaluación FTD única (`pkg/bfcore/ftd.go`, Opción 5B):**
   - Constantes canónicas: `DefaultMaxFTDFiles = 5`, `MaxFTDExemptGoTests = 5`, `MaxFTDPhysicalFiles = 10`, `MaxFTDRootModules = 1`.
   - `CountRootModules(paths []string) []string`: extraído de `1_Intake/engine/paths.go:137-160` (preserva la semántica exacta donde archivos raíz sin `/` mapean a `"."`, deduplicados case-insensitively).
   - `EvaluateFTDBounds(paths []string, maxEffective int) FTDEvaluation`:
     - Normaliza y deduplica rutas (case-insensitive en Windows).
     - Clasifica como **test Go candidato** exclusivamente archivos cuyo nombre base termina en `_test.go` (`strings.HasSuffix(strings.ToLower(base), "_test.go")`). Todos los archivos de otros lenguajes (`.py`, `.js`, `.ts`, etc.) se clasifican como **no-exentos (`1:1`)** de forma explícita.
     - Recopila el conjunto de directorios padre que contienen al menos un archivo **productivo (no `_test.go`)** en `paths`.
     - Un `*_test.go` queda exento únicamente si su directorio padre pertenece a ese conjunto **y** `exemptCount < MaxFTDExemptGoTests (5)`. Cualquier `*_test.go` sin archivo productivo hermano en el mismo directorio, o por encima del 5.º test exento, cuenta `1:1` en `EffectiveCount`.
     - Calcula `RootModules := CountRootModules(paths)` sobre **todos** los archivos físicos (incluidos los tests exentos, sin excepciones).
     - Devuelve `FTDEvaluation{PhysicalCount, EffectiveCount, ExemptTestCount, RootModules, WithinBounds}` donde `WithinBounds` exige simultáneamente: `len(RootModules) <= 1 && EffectiveCount <= maxEffective && ExemptTestCount <= MaxFTDExemptGoTests && PhysicalCount <= MaxFTDPhysicalFiles`.
   - `1_Intake/engine/triage.go` reemplaza la verificación inline (`L211-220`) por `bfcore.EvaluateFTDBounds(wi.AffectedPaths, maxFTDFiles)` con test dedicado en `1_Intake/engine/intake_test.go`.

2. **Higiene de rutas parametrizada y hashing normalizado (`pkg/bfcore/paths.go`):**
   - `SanitizeRelativePaths(rawPaths []string, repoRoot string, opts PathSanitizeOpts) ([]string, string)`: unifica el algoritmo pre-`path.Clean` de `1_Intake/engine/paths.go:20-118` con `PathSanitizeOpts{AllowDir bool, AllowGlob bool, RejectPersonalG4 bool}`. `1_Intake` invoca `{AllowDir: false, AllowGlob: false, RejectPersonalG4: true}`; `2_Spec_Writer` usa `{AllowDir: false, AllowGlob: false, RejectPersonalG4: true}` para `FTD` / `observed_files` / `g5_citations` y `{AllowDir: true, AllowGlob: false, RejectPersonalG4: true}` para prefijos `can_modify`/`forbidden` en `ASDD` (rechazando siempre globs `*?[`).
   - `ComputeNormalizedFileSHA256(repoRoot, relPath string) (state string, sha256Hex string, err error)`: verifica `CheckNoReparseChain(repoRoot, absPath)`; si `os.Lstat` devuelve `os.ErrNotExist`, retorna `("absent", "", nil)`; si es directorio o archivo irregular devuelve error; si es archivo regular lee bytes, elimina UTF-8 BOM (`\xef\xbb\xbf`), normaliza `\r\n` y `\r` a `\n` y retorna `("present", hexSHA256, nil)`.

3. **Cliente read-only de `0_Brain` (`pkg/bfcore/brain_client.go`):**
   - Extrae `BrainClient`, `SubprocessBrainClient`, `Preflight`, `Context`, `Query`, `IsProjectActiveOnDisk` y DTOs (`BrainPreflight`, `BrainContextResponse`, `BrainDocMatch`, `RiskHit`, `BrainInvocation`) desde `1_Intake/engine/brain_client.go`.
   - Define sentinels independientes (`ErrBrainUnavailable`, `ErrBrainIndexMissing`, `ErrBrainJournalActive`) y retorna errores multi-wrap de Go (`fmt.Errorf("%w: %w: %s", ErrBrainUnavailable, ErrBrainJournalActive, detail)` y análogo para `ErrBrainIndexMissing`), garantizando simultáneamente `errors.Is(err, ErrBrainJournalActive) == true` **y** `errors.Is(err, ErrBrainUnavailable) == true` (mientras `errors.Is(err, ErrBrainIndexMissing) == false`). Así `1_Intake` sigue detectando `ErrBrainUnavailable` (exit `4`) sin cambios, y `2_Spec_Writer` distingue exactamente `BRAIN_JOURNAL_ACTIVE` vs. `BRAIN_UNAVAILABLE` bajo exit `6` (`retryable: true`).

### 3. Esquema Canónico `spec.json` (`schema_version: "1.0"`) y Mapa Campo por Campo hacia `GengiSWAP` (`bridge.go`)

`spec.json` aplica `"additionalProperties": false` en todos los niveles (sin `extensions`). La siguiente tabla fija tipo, obligatoriedad por carril y destino exacto en los archivos compilados para [`GengiSWAP/engine/bridge.go`](../../GengiSWAP/engine/bridge.go), garantizando cobertura del 100% de las secciones requeridas por `parseIndexFile`, `parseChunkFile` y `validateChunks`:

| Ruta JSON (`spec.json`) | Tipo | `FTD` | `ASDD` | Destino compilado en `GengiSWAP` / Uso determinista |
| :--- | :--- | :---: | :---: | :--- |
| `/schema_version` | `string` (`"1.0"`) | Req | Req | Fail-closed en `validate`/`compile`/`render`. |
| `/work_item_ref` | `object` (`id`, `revision`, `sanitized_payload_sha256`, `trace_id`, `source_workitem_path` relativo a `--factory-root`) | Req | Req | Trazabilidad inmutable vs `spec_context.json` sin rutas absolutas de host (seguro para `t.TempDir()` en Windows bajo `G4`); `trace_id` e `id` en cabecera de `index.md` y `specwriter_events.jsonl`. |
| `/project` | `string` (slug) | Req | Req | Debe coincidir con `spec_context.json.project`; usado en `brain.exe context --project`. |
| `/lane` | `enum` (`"FTD"` \| `"ASDD"`) | Req | Req | Recalculado por `validate` (solo ascenso `FTD → ASDD` con `decisions[]`). |
| `/template` | `enum` (`modulo_nuevo` \| `correccion_hallazgos` \| `auditoria_e2e` \| `mapeo_estatico`) | Req | Req | Línea 2 de `index.md`: `Plantilla: <template> v1` (`templates/README.md:20`). |
| `/target_branch` | `string` (sin espacios ni backticks) | Req | Req | `## Directorio de Destino Real` (`rama <target_branch>` sin backticks en `## Alcance` para evitar falso parseo en `bridge.go:630`). |
| `/title` | `string` (`1..160` chars) | Req | Req | H1 de `index.md`: `# Mapa de Tarea: <task_id> — <title>`. |
| `/g3_data_mutation` | `string` (non-empty) | Req | Req | `## Alcance` → `- Mutación de datos (G3): <texto>` (`templates/*/index.md:14`). |
| `/requires_autor_hitl` | `bool` | Req | Req | Invariante monotónica (`true` si `WorkItem.requires_autor_hitl` o algún `risk_hit` es `CRITICO` o `ESCALATION_REQUIRED`); impreso en resumen previo a `gengis.exe plan`. |
| `/stack_assumptions` | `string[]` (`>= 1`) | Req | Req | `## Supuestos y Stack Tecnológico` de `index.md` + `- **Máx. intentos por chunk:** 3` (`bridge.go:19`). |
| `/read_only_paths` | `string[]` (prefijos/archivos relativos, sin globs) | Req | Req | `## Alcance` → `- Solo lectura:` en `index.md` (+ entregables `[x] hecho`). |
| `/global_forbidden` | `string[]` (prefijos/archivos relativos, sin globs) | Req | Req | `## Alcance` → `- Prohibido:` en `index.md` (unido automáticamente con rutas de `wiki/risk_zones.md` que NO intersectan `can_modify`). |
| `/dod` | `string[]` (`>= 1`) | Req | Req | `## Definición de Terminado (DoD)` en `index.md`. |
| `/narrative/problem` | `string` (non-empty) | Req | Req | `**Objetivo:**` en `index.md` y sección `## Problema` en `render`. |
| `/narrative/root_cause` | `string` (non-empty) | Req | Req | Sección `## Causa Raíz` en `render` (referenciando `g5_citations[]`). |
| `/narrative/proposed_change` | `string` (non-empty) | Req | Req | Sección `## Cambio Propuesto` en `render`. |
| `/narrative/alternatives_considered` | `string[]` (`>= 1` en `ASDD`) | Opc (`[]`) | Req | Sección `## Alternativas Consideradas` en `render` (`ASDD`). |
| `/narrative/interfaces_and_contracts` | `string` (non-empty en `ASDD`) | Opc (`""`) | Req | Sección `## Interfaces y Contratos` en `render` (`ASDD`). |
| `/narrative/edge_cases` | `string[]` (`>= 1` en `ASDD`) | Opc (`[]`) | Req | Sección `## Casos Borde` en `render` (`ASDD`). |
| `/narrative/rollback_plan` | `string` (non-empty en `ASDD`) | Opc (`""`) | Req | Sección `## Plan de Rollback` en `render` (`ASDD`). |
| `/observed_files[]` | `{path, state: "present"\|"absent", sha256}` | Req | Req | Cubre **únicamente rutas de archivo concretas** (`affected_paths ∪ g5_citations[].path ∪ archivos concretos de can_modify`, nunca prefijos de directorio); comparado contra disco normalizado CRLF/BOM para `STALE_CONTEXT`. Para prefijos de directorio declarados en `ASDD` `can_modify` (terminados en `/`): (a) si el directorio **aún no existe**, `validate` ejecuta `CheckNoReparseUpToNearestExisting` hasta el ancestro existente más cercano y evalúa el prefijo (+ sonda sintética `<prefijo>/__new__`) contra `0_Brain` y `wiki/risk_zones.md` sin fallar; (b) si el directorio **ya existe**, recorre `WalkDir` omitiendo `.git/`, `node_modules/`, `runtime/` y `.dev/`, rechaza cualquier reparse point interno y aplica un tope duro de **`MaxDirExpansionFiles = 128`** archivos ordenados lexicográficamente al invocar `brain.exe context --files` (complementado con cruce bidireccional de prefijos contra `wiki/risk_zones.md` para no exceder límites de argumentos Windows ni `< 500 ms` sin perder zonas de riesgo). |
| `/g5_citations[]` | `{path, line, symbol_anchor, claim}` | Req (`>= 1`) | Req (`>= 1`) | Validado en disco (1-based CRLF/BOM + ancla literal `±5` para extensiones de código); proyectado en `render`. |
| `/decisions[]` | `{reason, decided_by, rationale, timestamp, lane_from, lane_to, approved_paths[], scope_sha256}` | Opc (`[]`) | Req si hubo escalado/upgrade | Vincula resolución `[AUTOR]` de exit `4` al `scope_sha256` exacto; impreso en resumen pre-`plan`. |
| `/risk_mitigations[]` | `{risk_id, level, verify_step_ids[], review_gates[], requires_autor_hitl}` | Req si `len(risk_hits)>0` | Req si `len(risk_hits)>0` | Cubre 100% de `risk_hits` (originales + re-consultados); `CRITICO` exige `>=1 review_gate` y `requires_autor_hitl: true`. |
| `/brain_followups[]` | `{target_hint, summary, trigger}` | Opc (`[]`) | Opc (`[]`) | Proyectado en `render` y en notas post-merge de `index.md` (sin invocar `brain.exe propose` en v1). |
| `/work_units[]` | `object[]` (`1..2` en `FTD`; `>= 1` en `ASDD`) | Req | Req | Compilado 1-a-1 a `chunks/01_<id>.md`..`NN_<id>.md` (orden topológico determinista según `depends_on`) y `## Contratos entre Pasos` en `index.md`. |
| `/work_units[]/id` | `string` (`^[a-z0-9][a-z0-9_]{1,47}$`, único) | Req | Req | Sufijo de archivo `NN_<id>.md` y `Chunk.ID` (`ring.go:403-406`). |
| `/work_units[]/title` | `string` (`1..120` chars) | Req | Req | H1 `# Chunk NN: <title>` y viñeta inicial de `status.md`. |
| `/work_units[]/objective` | `string` (non-empty) | Req | Req | `## Objetivo` → `Chunk.Spec` (`bridge.go:501`, `ring.go:407`). |
| `/work_units[]/inputs` | `string[]` | Req | Req | `## Entradas` en `chunks/NN_<id>.md`. |
| `/work_units[]/can_modify` | `string[]` (`>= 1`; archivos concretos en `FTD`; archivos concretos o prefijos de directorio terminados en `/` sin globs en `ASDD`) | Req | Req | `## Salidas` + unión en `index.md` `Puede modificar`; disjunto entre `work_units` sin dependencia mutua. |
| `/work_units[]/scope_expansions` | `{path, rationale}[]` | Opc (`[]`) | Opc (`[]`) | Documenta rutas de `can_modify` fuera de `WorkItem.affected_paths`; resumen pre-`plan`. |
| `/work_units[]/forbidden` | `string[]` (prefijos/archivos sin globs) | Req | Req | `## Fuera de Alcance` (`bridge.go:417`) — solo rutas en backticks, unido con `can_modify` de otros chunks y `risk_zones` no intersectadas. |
| `/work_units[]/depends_on` | `string[]` (IDs previos, DAG acíclico) | Opc (`[]`) | Req (`[]` en el 1.º) | Valida orden topológico y genera `## Contratos entre Pasos` en `index.md`. |
| `/work_units[]/acceptance_criteria[]` | `{id, kind: "command"\|"review_gate", description, verify_step_id?, executor?}` | Req (`>= 1`) | Req (`>= 1`) | `## Criterios de Aceptación` (`Chunk.Acceptance`, `ring.go:409`). Si `kind == "command"`, debe referenciar un `verify_step.id`; si `kind == "review_gate"`, `executor` obligatorio (`4_Revisores` \| `Modo_QA` \| `AUTOR`). |
| `/work_units[]/verify_steps[]` | `{id, cwd, env, argv[], expect_exit: 0, timeout_sec: 1..300}` | Req (`>= 1`) | Req (`>= 1`) | `## Comando de Verificación` en bloque fenced (`Chunk.VerifyCommands`, `ring.go:420`), compilado a línea segura `cmd.exe`. Todo `work_unit` exige `>= 1` `verify_step` real. |
| `/work_units[]/failure_proof` | `{description, expect_exit: 1..255, expect_output_contains?}` | Req | Req | `## Prueba de que puede fallar` (`Chunk.FailureProof`, `bridge.go:504`, `ring.go:411`). **Nunca** se inyecta en `VerifyCommands` (evitando fallo en `ring.go:678-680`). |

*(Nota de canonización: `spec_sha256` se calcula serializando la estructura `Spec` canonizada con `json.Marshal` compacto, claves en orden determinista de struct, slices vacíos serializados como `[]` y nunca `null`, sin indentación ni CRLF. `scope_sha256` se calcula como SHA-256 sobre la unión única, normalizada en minúsculas y ordenada lexicográficamente de `work_units[].can_modify` unida por `\n`.)*

### 4. Estructura de Artefactos de Estado en `2_Spec_Writer/runtime/`

1. **`spec_context.json` (inmutable tras `prepare`, actualizable únicamente vía `prepare --refresh-context`):**
   - Contiene `schema_version: "1.0"`, `created_at`, `trace_id`, `repo_root_basename` y `repo_root_sha256` (**nunca rutas absolutas de host**, evitando falsos positivos `win_user_path` bajo `C:\Users\...` en tests `t.TempDir()` de Windows; toda ruta física se resuelve desde flags CLI), `git_head` (informativo), `work_item` (`id`, `revision`, `sanitized_payload_sha256`, `project`, `kind`, `lane`, `suggested_template`, `risk_level`, `requires_autor_hitl`, `affected_paths[]`, `impact_flags[]`, `risk_hits[]`), `brain_snapshot` (`brain_db_mtime`, `risk_zones_sha256`, `matched_docs[]`, `invocations[]`), y `baseline_hashes[]` (`{path, state, sha256}` para `affected_paths` y documentos activos de bóveda referenciados).
2. **`attempts.json` (inicializado por `prepare`, cerrojo `.wi.lock`):**
   - Esquema: `{schema_version: "1.0", wi_id, project, draft_attempts, final_attempts, external_cycles, last_exit, last_reason, last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256, updated_at}`.
   - Reglas de transición (sin vías de evasión del freno):
     - **`prepare` inicial (WorkItem nuevo):** crea `spec_context.json`, inicializa `attempts.json` en cero (`draft_attempts: 0, final_attempts: 0, external_cycles: 0, last_passed_sha256: "", last_passed_stage: "", last_passed_goldfish_sha256: ""`) y escribe el esqueleto inicial `spec.json`. Si `spec.json` o `spec_context.json` **ya existen** y no se pasa `--refresh-context` ni `--reset-attempts`, `prepare` falla cerrado (exit `2`) sin sobrescribir `spec.json` ni reiniciar contadores.
     - **`prepare --refresh-context` (ante `STALE_CONTEXT` o nueva revisión del `WorkItem`):** comportamiento estrictamente **no destructivo**: actualiza **únicamente** `spec_context.json`, invalida `last_passed_sha256 = ""`, `last_passed_stage = ""` y `last_passed_goldfish_sha256 = ""` en `attempts.json`, registra `CONTEXT_REFRESHED` en `specwriter_events.jsonl`, y **NO** sobrescribe `spec.json` ni reinicia `draft_attempts`, `final_attempts` ni `external_cycles` (impidiendo evadir el límite de `external_cycles` o `exit 5`).
     - **`prepare --reset-attempts --reason "<motivo>"` (única vía oficial tras `exit 5`):** `--reason` obligatorio (`len >= 8`, escaneado con `ScanG4All` → exit `3` si falla); reinicia `draft_attempts = 0`, `final_attempts = 0`, `external_cycles = 0`, invalida `last_passed_sha256`/`last_passed_stage`/`last_passed_goldfish_sha256`, **NO** sobrescribe `spec.json` (actualiza `spec_context.json` solo si se combina con `--refresh-context`) y registra `ATTEMPTS_RESET` con los contadores previos en `specwriter_events.jsonl`.
     - **En `validate --stage draft`:** si `draft_attempts >= 3` → exit `5` (`MAX_VALIDATE_ATTEMPTS`). Si falla con exit `1` o `3`, incrementa `draft_attempts` al terminar y actualiza `last_exit`/`last_reason` (sin payload). Si pasa (`exit 0`), actualiza `last_passed_sha256` y `last_passed_stage = "draft"` (`last_passed_goldfish_sha256 = ""`) **sin** resetear `final_attempts`.
     - **En `validate --stage final`:** si `final_attempts >= 3` → exit `5` (`MAX_VALIDATE_ATTEMPTS`). Si falla con exit `1` o `3`, incrementa `final_attempts` al terminar. Si pasa (`exit 0`), actualiza `last_passed_sha256`, `last_passed_stage = "final"` y `last_passed_goldfish_sha256` (SHA-256 normalizado de `goldfish_review.json` en `ASDD`, o `""` en `FTD`).
     - **Ante decisión externa (paso `4` de `validate`):** si `external_cycles >= 3`, actualiza `last_exit = 4`, `last_reason = <reason>` y devuelve **exit `5` (`MAX_EXTERNAL_CYCLES`)**; si `external_cycles < 3`, incrementa `external_cycles` y devuelve **exit `4`**.
     - **Si `attempts.json` falta o está corrupto en `validate` o `compile`:** fail-closed **exit `6`** (`ATTEMPTS_STATE_MISSING` / `ATTEMPTS_STATE_CORRUPT`, `retryable: false`).
3. **`goldfish_review.json` (requerido en `ASDD` `--stage final`):**
   - Ubicación fija: `2_Spec_Writer/runtime/<project>/<WI-id>/goldfish_review.json`.
   - Esquema (`additionalProperties: false`): `{schema_version: "1.0", spec_sha256, lenses_run[], unresolved_blockers, report_path, reviewed_at}`.
   - Lentes requeridos calculados determinísticamente por `validate`:
     - 3 lentes fijos obligatorios en toda spec `ASDD`: `["egm-design", "egm-codebase-details", "egm-implementation-readiness"]`.
     - Lentes condicionales: añadir `"egm-structural"` si `WorkItem.impact_flags` contiene `contract`, `schema` o `db` (o `risk_level == "CRITICO"`); añadir `"egm-zoom-out"` si `len(work_units) > 3`.
   - Criterio de pase: `spec_sha256 == canonicalSHA256(spec.json)`, `unresolved_blockers == 0`, todos los lentes requeridos presentes en `lenses_run[]`, y `report_path` limpio bajo `G4`. Documentado explícitamente como **higiene de consistencia, no prueba de ejecución**.

### 5. Flujo y Precedencia Exacta de `validate` y `compile`

```text
validate --spec <path> --stage <draft|final> [--json]
  [2] Uso CLI / flags inválidos / ruta sintácticamente mal formada ──► Exit 2 (retryable: false)
  [5] Leer attempts.json bajo .wi.lock:
      • Ausente / JSON corrupto                                    ──► Exit 6 (ATTEMPTS_STATE_*, retryable: false)
      • stage_attempts >= 3                                        ──► Exit 5 (MAX_VALIDATE_ATTEMPTS + last_exit/reason)
  [3] G4 Fail-Closed (11 reglas, 0 redacción Tier 2):
      • Fase-A sobre bytes crudos de spec.json (+ goldfish_review si final)
      • Fase-B sobre todos los strings JSON decodificados (\u0067hp_...)
      • Si hallazgo: incrementar stage_attempts, log REJECTED_G4   ──► Exit 3 (G4_VIOLATION, retryable: false)
  [6] Infraestructura / Preflight (checks compartidos con verify):
      • CheckNoReparseChain sobre repo/runtime/vault               ──► Exit 6 (REPARSE_POINT_DETECTED, retryable: false)
      • 0_Brain Preflight (.index/brain.db y .brain-journal.json)  ──► Exit 6 (BRAIN_UNAVAILABLE | BRAIN_JOURNAL_ACTIVE, retryable: true)
      • Errores OS de lectura / lock                               ──► Exit 6 (IO_ERROR | LOCK_BUSY, retryable: true)
  [4] Decisión Externa (si dispara y external_cycles >= 3 ──► Exit 5 MAX_EXTERNAL_CYCLES; si < 3 ──► inc + Exit 4):
      • STALE_CONTEXT: comparar SHA-256 normalizado CRLF/BOM de spec_context.json,
        risk_zones_sha256, brain_db_mtime y spec.observed_files[] (solo archivos concretos,
        incl. aparición inesperada de "absent")
      • Re-consulta 0_Brain context --files sobre archivos concretos de can_modify
        (+ expansión en disco + cruce bidireccional de risk_zones para prefijos de dir en ASDD,
        filtrando status=="active"):
        - ESCALATION_REQUIRED: sube risk_level, toca nueva zona CRITICO, o len(RootModules) > 1
          sin entrada válida en decisions[] que cubra approved_paths[] y scope_sha256 actual
        - LANE_UPGRADE_REQUIRED: spec.lane=="FTD" pero bfcore.EvaluateFTDBounds falla (>5 efectivos,
          >5 tests exentos, >10 físicos, >1 módulo) o hay ruta sin cobertura activa wiki/adr en 0_Brain
  [1] Errores Corregibles (reportados con JSON Pointer RFC 6901; si falla ──► inc stage_attempts + Exit 1, retryable: true):
      • Esquema estricto (DisallowUnknownFields), obligatoriedad FTD vs ASDD, DAG depends_on,
        can_modify disjunto entre unidades independientes, 1..2 work_units en FTD
      • G2 (cero bloques ```mermaid), G5 (1-based CRLF/BOM + ancla literal ±5 en código,
        rechazando tokens boilerplate len < 4 o palabras reservadas err/func/return/if/for/nil)
      • risk_mitigations[] completo, requires_autor_hitl monotónico, CRITICO con review_gate
      • Allowlist cerrado de comandos/subcomandos/banderas normalizadas/claves env en verify_steps
      • En --stage final (ASDD): verificación de goldfish_review.json
  [0] OK: persistir (last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)
      en attempts.json + evento VALIDATE_PASS                      ──► Exit 0
```

**Precedencia exacta de `compile` (`2 → 5 → 3 → 6 → 4 → 1`):**
1. **`[2]` Uso y saneamiento de `--task-id`:** valida flags obligatorios y `--task-id` contra `^[a-z0-9][a-z0-9_-]{2,63}$` (rechaza `..`, separadores, nombres reservados Windows `CON..LPT9`).
2. **`[5]` Estado de freno (`attempts.json`):** si falta/corrupto → `exit 6` (`ATTEMPTS_STATE_*`, `retryable: false`); si **`stage_attempts >= 3`** (etapa requerida: `draft` para `FTD`, `final` para `ASDD`) **o `external_cycles >= 3`** → **`exit 5`** (`MAX_VALIDATE_ATTEMPTS` / `MAX_EXTERNAL_CYCLES`).
3. **`[3]` Barrera `G4` previa a cualquier escritura externa:** escanea `spec.json` (raw + decodificado) con las 11 reglas.
4. **`[6]` Infraestructura, seguridad de destino y cerrojo TOCTOU:**
   - Ejecuta checks compartidos de `verify`: `0_Brain` preflight, `CheckNoReparseChain` sobre `--task-root` y destino (`REPARSE_POINT_DETECTED`, `retryable: false`).
   - Adquiere cerrojo exclusivo `<task-root>/.<task-id>.compile.lock` vía `bfcore.AcquireFileLock` (`LOCK_BUSY`, `retryable: true`).
   - **Dentro del cerrojo**, inspecciona `<task-root>/<task-id>` con `os.Lstat` (**política estricta *create-only* en v1**):
     - Si `<task-root>/<task-id>/.ring/state.json` existe → aborta sin modificar nada con **`exit 6`, `reason: "LIVE_TASK_STATE"`, `retryable: false`**.
     - Si `<task-root>/<task-id>` ya existe en disco (aunque no tenga `.ring/state.json`, p. ej. compilación previa) → aborta sin sobrescribir con **`exit 6`, `reason: "TASK_DIR_EXISTS"`, `retryable: false`** (exigiendo un `--task-id` nuevo o limpieza manual explícita; elimina chunks huérfanos y evita el fallo de `os.Rename` sobre directorios existentes en Windows NTFS).
5. **`[4]` Revalidación de contexto/alcance en vivo:** re-ejecuta el bloque `[4]` de `validate` sin consumir intentos (`STALE_CONTEXT`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`).
6. **`[1]` Anti-bypass completo y superconjunto `validateChunks`:**
   - Comprueba: (a) `canonicalSHA256(spec.json) == attempts.last_passed_sha256`, (b) `last_passed_stage` satisface la etapa requerida (`"draft"` o `"final"` para `FTD`; estrictamente `"final"` para `ASDD`), y (c) en `ASDD`, `goldfish_review.json` existe y su SHA-256 normalizado coincide con `attempts.last_passed_goldfish_sha256`. Si cualquiera falla (p. ej. edición de `spec.json` o edición/borrado posterior de `goldfish_review.json`) → **`exit 1`, `reason: "VALIDATION_REQUIRED"`, `retryable: true`** (sin consumir intento; nunca un falso `VALIDATOR_BYPASS_DEFECT`).
   - Ejecuta el bloque `[1]` completo + el superconjunto de `validateChunks` (`ring.go:393-426`: IDs únicos, `Spec`/`Acceptance`/`FailureProof`/`VerifyCommands` no vacíos, prefijos sin globs, renderizado `##` H2 + bloques fenced sin colisiones de `:` ni backticks no-ruta en `## Fuera de Alcance`) + `ScanG4All` sobre los buffers renderizados en memoria (`index.md`, `chunks/NN_*.md`, `status.md`). Si alguna comprobación falla pese a tener pase válido para el mismo `(spec_sha256, stage, goldfish_sha256)` → **`exit 1`, `reason: "VALIDATOR_BYPASS_DEFECT"`, `retryable: false`** (sin consumir intento; registrado en `specwriter_events.jsonl` para generar fixture de regresión).
7. **`[0]` Promoción atómica *create-only* en Windows y resumen informado:** dentro del `.compile.lock`:
   - Crea un directorio hermano de staging en el **mismo volumen NTFS** (`<task-root>/.<task-id>.staging-<rand>`) con `defer os.RemoveAll(stagingDir)` si cualquier paso falla antes del rename.
   - Escribe el árbol completo dentro del staging (`index.md`, `chunks/01_<id>.md`..`NN_<id>.md`, `status.md` inicial y `workspace/qa/`), ejecutando `Write` + `Sync` + `Close` explícitos en cada archivo antes de promover.
   - Re-verifica por `os.Lstat` que `<task-root>/<task-id>` sigue inexistente (`os.ErrNotExist`) y ejecuta **un único `os.Rename(stagingDir, destDir)` atómico** (con reintento acotado Windows de 3×25 ms ante bloqueos transitorios de antivirus/indexador; como `destDir` no existe y ambos residen en `<task-root>`, la operación es un rename atómico único en NTFS).
   - Registra `COMPILED` en `specwriter_events.jsonl`, imprime el resumen informado (`spec_sha256`, `trace_id`, `lane`, `scope_expansions`, `risk_mitigations`, `decisions`, lentes Goldfish y comando exacto `gengis.exe plan`) y sale con `exit 0`.

### 6. Allowlist Cerrado de Comandos (`verify_steps`) y Regla de Espacios en Windows (`C:\Black Factory`)

`validate` y `compile` **nunca ejecutan** `verify_steps`; los auditan contra una tabla cerrada embebida en Go:

| Ejecutable permitido (`argv[0]`) | Subcomandos permitidos (`argv[1]`) | Banderas explícitamente bloqueadas (normalizadas `-flag` / `--flag` / `-flag=val`) |
| :--- | :--- | :--- |
| `go` | `test`, `vet`, `build` | `-exec`, `-toolexec`, `-vettool`, `-overlay`, `-pkgdir`, **`-o`**, **`-c`**, **`-outputdir`**, **`-coverprofile`**, **`-cpuprofile`**, **`-memprofile`**, **`-blockprofile`**, **`-mutexprofile`**, **`-trace`** *(bloquea toda bandera de ejecución externa o escritura de archivos/binarios en disco; subcomandos `run`, `env`, `generate`, `install`, `tool` bloqueados)* |
| `git` | `status`, `diff`, `rev-parse`, `check-ignore` | `-c`, `-config-env`, `-exec-path`, `-git-dir`, `-work-tree`, `-upload-pack` |
| `0_Brain/brain.exe` | `verify`, `query`, `context` | *(solo lectura; `apply`, `propose`, `init-vault`, `index` excluidos)* |
| `1_Intake/intake.exe` | `verify`, `list`, `show` | *(solo lectura; `ingest`, `resolve` excluidos)* |
| `2_Spec_Writer/specwriter.exe` | `verify`, `render` | *(solo lectura)* |

*(Nota: `GengiSWAP/gengis.exe status` queda **excluido** del allowlist porque `ShowStatus` invoca `SyncFiles` y muta `status.md` en `bridge.go:1487`, rompiendo el criterio de solo lectura.)*

*   **Normalización de banderas:** antes de comparar contra la lista de banderas bloqueadas, el validador normaliza cada token `argv[k]` que empiece por `-` quitando 1 o 2 guiones iniciales y recortando desde el primer `=` (`-toolexec=foo`, `--toolexec`, `-o=..\a.exe`, `--o` se normalizan a `toolexec` y `o`), impidiendo evasiones por formato de bandera.
*   **Prohibición total de intérpretes y wrappers de shell:** cualquier `argv[0]` fuera de esta tabla (incluyendo `cmd`, `cmd.exe`, `powershell`, `pwsh`, `sh`, `bash`, `python`, `python3`, `node`, `npm`, `npx`, `echo`, `true` o scripts `.bat`/`.cmd`/`.ps1`) se rechaza con exit `1` y puntero JSON `/work_units/<i>/verify_steps/<j>/argv/0`.
*   **Claves `env` permitidas (allowlist):** `CGO_ENABLED`, `GOOS`, `GOARCH`, `GOAMD64`, `PYTHONUTF8`, `PYTHONIOENCODING`, `GENGIS_CONTRACT_STRICT`. **`GENGIS_BIN` queda explícitamente excluida** del allowlist de `spec.json` (solo proviene del entorno del operador/CI para impedir ejecución arbitraria a través de `contract_gengis_test.go`), al igual que `PATH`, `COMSPEC`, `GOFLAGS`, `PYTHONPATH`, `NODE_OPTIONS` o `LD_PRELOAD`.
*   **Metacaracteres rechazados (sin escape) en `cwd`, `env` y cada `argv[k]`:** `&`, `|`, `<`, `>`, `^`, `%`, `!`, `"`, `\r`, `\n` y `\x00` (`NUL`).
*   **Regla para espacios en la ruta del repo (`C:\Black Factory`):** `cwd` y `argv` deben ser **estrictamente relativos a la raíz del repositorio** (nunca contener `C:\Black Factory` ni comillas `"`). Al serializar hacia `cmd.exe` (`GengiSWAP/engine/runner_windows.go:23`), `compile` genera `set K=V&&` para `env`, antepone `cd /d "<cwd>" &&` únicamente cuando `cwd != ""` (envolviendo en comillas solo si `cwd` o un argumento relativo contiene espacios, seguro porque `"` fue rechazado previamente), apoyándose en que `ShellRunner` (`bridge.go:1175-1178`) ya establece `cmd.Dir = TargetRepo`.
*   **Límites de ejecución:** `expect_exit` fijado a `0` y `timeout_sec` en `1..300` (`Timeout: 5 * time.Minute` en `bridge.go:1178`). Pasos `> 5 min` se declaran como `review_gate`; expectativas `expect_exit != 0` viven exclusivamente en `failure_proof`.

### 7. `specwriter verify` (Solo Lectura), Separación `--factory-root` vs. `--repo-root` y Deriva `RZ-001`

`specwriter verify [--factory-root <dir>] [--repo-root <dir>] [--vault <dir>] [--data-dir <dir>] [--json]` opera sin efectos secundarios (`0` archivos creados, `0` cerrojos exclusivos; usa `readFileWithShortRetry` 3×25 ms ante bloqueos momentáneos de Windows):
*   **Separación de raíces:** `--factory-root` apunta a la raíz de **Black Factory** (por defecto igual a `--repo-root` cuando se trabaja sobre el propio monorepo `black-factory`), mientras `--repo-root` apunta al repositorio del proyecto destino (p. ej. un repositorio corporativo externo).
1. **Aislamiento `Opción i` (sobre `--factory-root`, comprobación individual por ruta):** ejecuta dos invocaciones separadas (`git -C <factoryRoot> check-ignore -q 2_Spec_Writer/runtime/` **y** `git -C <factoryRoot> check-ignore -q 2_Spec_Writer/specwriter.exe`), exigiendo código `0` en **cada una** (una sola llamada con ambos argumentos daría falso positivo `0` si solo `*.exe` estuviera ignorado). Si cualquiera falla → exit `6`, `reason: "GITIGNORE_DRIFT"`, `retryable: false`.
2. **Reparse points:** `bfcore.CheckNoReparseChain` sobre `factoryRoot`, `repoRoot`, `dataDir` y `vaultRoot` → exit `6`, `reason: "REPARSE_POINT_DETECTED"`, `retryable: false`.
3. **Preflight `0_Brain`:** `bfcore.SubprocessBrainClient.Preflight` (compartido con `prepare`, `validate` y `compile`) → exit `6`, `BRAIN_UNAVAILABLE` o `BRAIN_JOURNAL_ACTIVE`, `retryable: true`.
4. **Detección de deriva en `RZ-001` (sobre `--factory-root`):** calcula `bfcore.ComputeNormalizedFileSHA256` sobre `<factoryRoot>/GengiSWAP/engine/bridge.go` y `<factoryRoot>/GengiSWAP/engine/ring.go` y los compara contra las constantes ancladas en `compile.go`. Si difieren → exit `6`, `reason: "GENGIS_ENGINE_DRIFT"`, `retryable: false`.
5. **Integridad y `G4` sobre `2_Spec_Writer/runtime/`:** recorre todos los archivos existentes (`spec_context.json`, `attempts.json`, `spec.json`, `goldfish_review.json`, `specwriter_events.jsonl`), ejecuta `bfcore.ScanG4All` (si halla fuga residual en disco → exit `3`, `G4_VIOLATION_ON_DISK`, `retryable: false`) y valida sintaxis JSON de cada artefacto (si corrupto → exit `6`, `ATTEMPTS_STATE_CORRUPT` / `IO_ERROR`, `retryable: false`).

### 8. Test de Contrato Real contra `gengis.exe plan`

Sin importar código de `GengiSWAP/engine/` (`RZ-001`):
*   Lee la ruta del binario desde la variable de entorno del operador/CI `GENGIS_BIN` (por defecto `../../GengiSWAP/gengis.exe`; nunca configurable desde `spec.json`).
*   Si el binario no existe: en ejecución local ejecuta `t.Skipf("gengis.exe no encontrado en %s; define GENGIS_BIN o compila GengiSWAP/gengis.exe", binPath)`; si `GENGIS_CONTRACT_STRICT=1` (obligatorio en el Verification Plan antes de merge), falla con `t.Fatalf`.
*   **Casos positivos (FTD con `failure_proof.expect_exit: 1` y ASDD multi-chunk):** compila sobre `t.TempDir()` y ejecuta `gengis.exe plan <tmpTask> --root <tmpRoot>` con `"n\n"` en `stdin` (sin tocar jamás `C:\Black Factory\GengiSWAP\gengis_global_log.md`). Verifica: exit `1`, `stdout` contiene `printPlanSummary` con todos los chunks y el `TargetRepo` real (sin `ADVERTENCIA: destino sin Git`), `<tmpTask>/.ring/state.json` existe con `"status": "PLAN_PENDING"`, y `stderr` contiene literalmente `"plan no aprobado por el Autor"` (`ring.go:28, 570`).
*   **Casos negativos de paridad:** inyecta fixtures con `FailureProof` vacío, `VerifyCommands` vacío o ID duplicado y verifica que tanto el adaptador como `gengis.exe plan` los rechazan (en `gengis.exe plan`: exit `1`, `stderr` con `"chunk inválido"`, sin resumen de plan y sin crear `.ring/state.json`).

--------------------------------------------------------------------------------

## Alternatives Considered

### Alternative A: Llamar API de LLM Directamente desde `specwriter.exe` (Go)

*   **Pros:** Permite un comando único autocontenido sin depender de un agente/harness externo.
*   **Cons:** Introduce credenciales de API, dependencias HTTP, reintentos y manejo de streaming/tool-calling en el binario Go; rompe la operación offline/sin secretos y duplica el harness de Antigravity y EGM.
*   **Reason for Rejection:** Rejected by author: el binario Go debe mantenerse 0% LLM y sin secretos; la ejecución desatendida futura se resuelve mediante orquestación externa sobre el CLI del agente (`prepare → agente → validate → compile`).

### Alternative B: Implementar `2_Spec_Writer` Únicamente como `SKILL.md` sin Binario Go

*   **Pros:** Cero código Go que mantener; cambio rápido de formato editando Markdown.
*   **Cons:** Viola `constitution.md:22` (*determinismo del motor sobre buena voluntad del modelo*); deja sin verificación mecánica `G2`, `G4`, `G5`, conservación de `risk_hits`, allowlist de comandos y compatibilidad con `bridge.go`.
*   **Reason for Rejection:** Rejected by author: las comprobaciones objetivas sin juicio deben ejecutarse de forma determinista en Go para evitar ciclos fallidos en `3_Builder`.

### Alternative C: Delegar la Partición de Chunks a `3_Builder` o a un Divisor Mecánico por Archivo/Tamaño

*   **Pros:** Mantiene `spec.json` más simple al omitir `work_units[]`.
*   **Cons:** La experiencia real en `bf_01_intake_v1` (43 archivos agrupados en 2 chunks disjuntos por contrato compartido) y `fix_ri_hallazgos` demostró que la partición requiere juicio arquitectónico sobre el código. Delegarla a `3_Builder` obliga a releer el repositorio y mezcla los roles Preparador y Chef.
*   **Reason for Rejection:** Rejected by author: la partición en `work_units[]` es juicio de diseño y pertenece a `2_Spec_Writer`, mientras la serialización hacia `index.md`/`chunks/*.md` es mecánica y pertenece al compilador determinista.

### Alternative D: Autoría Dual de Markdown + `spec.json` y Campo Opaco `extensions`

*   **Pros:** Mayor libertad de prosa para el agente y punto de extensión sin versionar el esquema.
*   **Cons:** Duplica tokens de salida, permite deriva semántica entre lo que revisa `[AUTOR]` y lo que compila `compile` (violando el precedente `NG5` de `1_Intake`), y abre un hueco `G4` y de mantenimiento en `extensions`.
*   **Reason for Rejection:** Rejected by author: `spec.json` (`additionalProperties: false`) es la única fuente de verdad con proyección Markdown determinista (`render`); toda evolución futura se gestiona mediante `schema_version`.

### Alternative E: Validar la Salida de `compile` Mediante `gengis.exe status <tmpDir>`

*   **Pros:** Evita duplicar validaciones de `GengiSWAP` en `2_Spec_Writer`.
*   **Cons:** Verificado en `GengiSWAP/engine/bridge.go:1455-1488`: `ShowStatus` omite `validateChunks` (`ring.go:393-426`, solo ejecutado por `PlanTask`) y llama `SyncFiles`, sobrescribiendo `<tmpDir>/status.md`. Aceptaría chunks inválidos silenciosamente.
*   **Reason for Rejection:** Rejected by author: reemplazado por un superconjunto estricto de `validateChunks` en `compile` más un test de contrato real contra `gengis.exe plan` en `t.TempDir()` (`GENGIS_CONTRACT_STRICT=1`).

### Alternative F: Máquina de Estados Completa, `specwriter approve`, `publish`, DSL Declarativo Externo y Wrapper `BrainPatch` en v1

*   **Pros:** Cobertura anticipada para exportación pública, múltiples operadores y creación automática de propuestas en `0_Brain`.
*   **Cons:** Sin firma criptográfica Ed25519, `approve` en un workspace donde escribe el agente es teatro de seguridad y duplica la parada real de `gengis.exe plan`; añadir Ed25519 es gestión de claves prematura; el DSL externo añade complejidad para 2 carriles y 1 monorepo; `0_Brain/engine/vault.go:244-418` (`ProposeUpdate`) carece hoy de `.brain.lock` y `ScanG4`.
*   **Reason for Rejection:** Rejected by author: diferidos explícitamente a v2 con disparadores concretos; v1 se acota a un núcleo delgado (`prepare`, `validate`, `render`, `compile`, `verify`) con `go:embed` y `attempts.json` documentado honestamente como freno contra bucles.

### Alternative G: Modificar `GengiSWAP/engine/` (`RZ-001`) para Ejecutar `argv` Estructurado u Omitir la Confirmación de `gengis.exe plan`

*   **Pros:** Eliminaría la traducción a `cmd.exe /S /C` y permitiría timeouts o `expect_exit` arbitrarios.
*   **Cons:** `GengiSWAP/engine/` es zona `CRITICO` (`RZ-001`); abrir cambios allí amplía el radio de impacto y retrasa la entrega de `2_Spec_Writer`.
*   **Reason for Rejection:** Rejected by author: `RZ-001` permanece intacto en v1; el compilador garantiza compatibilidad segura mediante allowlist cerrado, prohibición de metacaracteres y límites fijos (`expect_exit == 0`, `timeout <= 5 min`).

### Alternative H: Políticas Alternativas para Tests en `FTD=5` (Plan B `1:1`, Opción `5A` por Nombre Exacto o Regla Multi-Lenguaje por Directorio)

*   **Pros:** Plan B (`1:1`) no toca `1_Intake`; `5A` (`foo_test.go ↔ foo.go`) es más restrictiva; la regla multi-lenguaje cubriría otros stacks.
*   **Cons:** Plan B degrada correcciones pequeñas en Go a `ASDD` por añadir tests; `5A` penaliza archivos de prueba consolidados del paquete (como `brain_test.go` o `intake_test.go`); la regla "mismo directorio" no encaja con Python (`tests/`) ni TS/JS (`__tests__`), generando inconsistencias entre lenguajes.
*   **Reason for Rejection:** Rejected by author: elegida **Opción `5B`** en `pkg/bfcore` (`*_test.go` en el mismo directorio que un archivo productivo modificado con tope duro de `5` tests exentos / `10` archivos físicos totales, `<= 1` módulo raíz estricto, y otros lenguajes `1:1` explícito hasta que existan repos reales).

### Alternative I: Mutar `1_Intake/runtime/workitems/WI-*.json` en Escalados o Añadir `visibility` a `workitem.schema.json` v1.0

*   **Pros:** Mantendría un único estado de carril en Equipo 1 y permitiría visibilidad por ticket.
*   **Cons:** Rompe el esquema cerrado `workitem.schema.json` v1.0 (`additionalProperties: false`), mezcla responsabilidad de escritura entre Equipo 1 y Equipo 2, y hace que la visibilidad sea manipulable por ticket individual.
*   **Reason for Rejection:** Rejected by author: `WI-*.json` permanece inmutable como foto del triaje inicial, las resoluciones de escalado viven en `spec.json.decisions[]` acotadas a `approved_paths[]` + `scope_sha256`, y todo v1 es privado en `2_Spec_Writer/runtime/`.

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Seguridad y Privacidad (`G4`, `Opción i`, Anti-Inyección y Anti-TOCTOU):**
    - Las 11 reglas `G4` de `pkg/bfcore/g4.go:48-115` se aplican en modo fail-closed (exit `3`, cero auto-redacción Tier 2) en `prepare`, `validate`, `render`, `compile` y `verify`, tanto sobre bytes crudos como sobre strings JSON decodificados (`\u0067hp_...`), sin transcribir jamás el secreto en `attempts.json`, `stderr` ni `specwriter_events.jsonl`.
    - `validate` nunca ejecuta comandos de la spec; `verify_steps` exige allowlist cerrado de ejecutables/subcomandos/claves `env` (excluyendo `GENGIS_BIN`, intérpretes, shells, `-o` en `go build` y `gengis.exe status`) y rechaza metacaracteres `& | < > ^ % ! " \r \n \x00`.
    - Todas las escrituras en `2_Spec_Writer/runtime/<project>/<WI-id>/` usan cerrojo granular `.wi.lock` (permitiendo ejecución paralela entre WorkItems del mismo proyecto); los appends en `specwriter_events.jsonl` usan `.events.lock`; la escritura en `--task-root` usa `.compile.lock` + staging atómico + verificación `CheckNoReparseChain` + comprobación de `.ring/state.json` dentro del cerrojo.
    - `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` permanecen excluidos en `.gitignore`, verificado por `specwriter verify` (`GITIGNORE_DRIFT` → exit `6`, `retryable: false`).
*   **Rendimiento y Latencia:**
    - Presupuesto `< 500 ms` p95 en Windows para `prepare`, `validate`, `render`, `compile` y `verify` locales (sobre bóvedas de 50–100 documentos indexados en `0_Brain` y repositorios de tamaño medio), sin superar `<= 2` invocaciones read-only a `0_Brain/brain.exe`.
    - Ejecutar `validate --stage draft` (0 tokens) antes de lanzar revisores Goldfish en `ASDD` para evitar consumo de tokens sobre citas `G5` o esquemas rotos.
*   **Observabilidad y Trazabilidad:**
    - Propagación íntegra de `WorkItem.trace_id` a `spec_context.json`, `specwriter_events.jsonl` (eventos `PREPARED`, `CONTEXT_REFRESHED`, `ATTEMPTS_RESET`, `VALIDATE_PASS`, `VALIDATE_FAIL`, `REJECTED_G4`, `EXTERNAL_DECISION`, `ATTEMPTS_BLOCKED`, `COMPILED`, `VALIDATOR_BYPASS_DEFECT`, `LIVE_TASK_REFUSED`, con `duration_ms`, `reason`, `retryable` y `spec_sha256`) e `index.md`.
*   **Despliegue, Compatibilidad y Rollback:**
    - Compilación reproducible `CGO_ENABLED=0`; cero cambios a `GengiSWAP/engine/` (`RZ-001`) ni a `workitem.schema.json` v1.0.
    - El cambio en `1_Intake/engine/triage.go` (adopción de `bfcore.EvaluateFTDBounds` Opción `5B`) es retrocompatible para todas las peticiones existentes de `<= 5` archivos y relajación estrictamente acotada para `*_test.go` hermanos en Go, validado con la suite completa de `pkg/bfcore`, `0_Brain/engine` y `1_Intake/engine`.
    - Rollback limpio vía `git revert` del commit del chunk sin mutar `0_Brain/vault/`, `1_Intake/runtime/` ni tareas activas en `C:\Desarrollo\GengiSWAP_data\gengis_task\`.

### Additional Resources

*   Arquitectura base de `0_Brain`: [`Docs/BLACK_FACTORY_BRAIN.md`](../BLACK_FACTORY_BRAIN.md)
*   Especificación aprobada de `1_Intake`: [`Docs/designs/1_intake.md`](1_intake.md) y [`1_Intake/schemas/workitem.schema.json`](../../1_Intake/schemas/workitem.schema.json)
*   Motor determinista `GengiSWAP`: [`GengiSWAP/engine/bridge.go`](../../GengiSWAP/engine/bridge.go), [`GengiSWAP/engine/ring.go`](../../GengiSWAP/engine/ring.go), [`GengiSWAP/engine/runner_windows.go`](../../GengiSWAP/engine/runner_windows.go) y [`GengiSWAP/templates/README.md`](../../GengiSWAP/templates/README.md)
*   Constitución y zonas de riesgo: `0_Brain/vault/constitution.md` y `0_Brain/vault/wiki/risk_zones.md` *(archivos locales privados `Opción i`)*

### Future Improvements

*   **`specwriter approve` + firma criptográfica Ed25519 (v2):** *(Disparador: ejecución desatendida en CI o múltiples operadores).* Autenticar aprobaciones y desbloqueos `--reset-attempts` mediante `.autor/autor.pub` (mismo esquema que `0_Brain`).
*   **`specwriter publish` + `publish_allowlist` (v2):** *(Disparador: segunda necesidad real de exportar specs a directorios versionados).* Exportación `private`-by-default que remueve metadatos de runtime conservando `source_spec_sha256` y `G4`.
*   **DSL declarativo externo de plantillas (v2):** *(Disparador: divergencia real entre `>= 3` proyectos).* Sustituir plantillas `go:embed` por contratos declarativos versionados.
*   **Wrapper `BrainPatch` / `brain.exe propose` para ADRs en `ASDD` (v2):** *(Disparador: resolución del WorkItem independiente en `0_Brain` que añada `.brain.lock` y `ScanG4` a `ProposeUpdate` en `0_Brain/engine/vault.go:244-418`).*
*   **Reglas de archivo hermano por lenguaje adicional (v2):** *(Disparador: adopción activa de repositorios Python o TS/JS en Black Factory).* Definir semántica explícita por lenguaje (p. ej. módulo/paquete en Python) en `pkg/bfcore/ftd.go`.


--------------------------------------------------------------------------------

## Detailed Implementation Plan

La implementación se estructura en **dos fases con alcances estrictamente disjuntos** para facilitar la posterior compilación a dos chunks de `GengiSWAP` con mínima intervención `[AUTOR]` (`Fase 0`: núcleo compartido `pkg/bfcore` + delegación en `1_Intake/engine`; `Fase 1 + Integración`: `2_Spec_Writer/**` + cableado raíz `go.work`, `.gitignore`, `README.md`). No se modifica ningún archivo bajo `0_Brain/**`, `GengiSWAP/**` (`RZ-001`) ni `1_Intake/schemas/workitem.schema.json`.

### Target Files to Create/Modify

#### Fase 0 — Primitivas Compartidas en `pkg/bfcore` y Delegación en `1_Intake/engine`

#### 1. `pkg/bfcore/ftd.go` (Create)

*   **Type / Scope**: Go Library (`blackfactory/pkg/bfcore`, `CGO_ENABLED=0`, stdlib-only).
*   **Proposed Changes**:
    - Definir constantes canónicas: `DefaultMaxFTDFiles = 5`, `MaxFTDExemptGoTests = 5`, `MaxFTDPhysicalFiles = 10`, `MaxFTDRootModules = 1`.
    - Exportar `type FTDEvaluation struct { PhysicalCount, EffectiveCount, ExemptTestCount int; RootModules []string; WithinBounds bool }`.
    - Exportar `CountRootModules(paths []string) []string` (migrado desde `1_Intake/engine/paths.go:137-160`, preservando mapeo de archivos raíz sin `/` a `"."` y deduplicación case-insensitive).
    - Exportar `EvaluateFTDBounds(paths []string, maxEffective int) FTDEvaluation` implementando la **Opción 5B**:
      - Normalizar y deduplicar rutas case-insensitively (`strings.ToLower(NormalizeCoverPath(p))`).
      - Identificar directorios con al menos un archivo productivo (todo archivo cuyo basename NO termine en `_test.go`).
      - Eximir archivos `*_test.go` ubicados en un directorio presente en ese conjunto hasta un máximo de `MaxFTDExemptGoTests (5)`; todo test `*_test.go` sin hermano productivo en el mismo directorio, todo test por encima del 5.º exento y todo archivo de test de otros lenguajes (`.py`, `.js`, `.ts`, etc.) suman `1:1` en `EffectiveCount`.
      - Calcular `RootModules` sobre todos los archivos físicos incluidos los tests exentos (`<= 1` módulo raíz sin excepciones).
      - Fijar `WithinBounds = len(RootModules) <= MaxFTDRootModules && EffectiveCount <= maxEffective && ExemptTestCount <= MaxFTDExemptGoTests && PhysicalCount <= MaxFTDPhysicalFiles`.

#### 2. `pkg/bfcore/paths.go` (Modify)

*   **Type / Scope**: Go Library (`blackfactory/pkg/bfcore`).
*   **Proposed Changes**:
    - Exportar `type PathSanitizeOpts struct { AllowDir bool; AllowGlob bool; RejectPersonalG4 bool }`.
    - Exportar `SanitizeRelativePaths(rawPaths []string, repoRoot string, opts PathSanitizeOpts) ([]string, string)` unificando las validaciones pre-`path.Clean` de `1_Intake/engine/paths.go:20-118` (rechazo de rutas personales `MatchesPersonalPathG4`, control bytes/NUL, `:`, prefijos absolutos/UNC/NT, segmentos `.`/`..`, nombres reservados Windows `CON..LPT9`, preservando `/` final únicamente cuando `AllowDir == true` y rechazando `*?[` cuando `AllowGlob == false`).
    - Exportar `CheckNoReparseUpToNearestExisting(rootPath, targetPath string) error`: sube desde `targetPath` hasta el ancestro existente más cercano dentro de `rootPath` y aplica `CheckNoReparseChain(rootPath, nearestExisting)` (necesario para prefijos de directorios nuevos en `ASDD` `can_modify`).
    - Exportar `ComputeNormalizedFileSHA256(repoRoot, relPath string) (state string, sha256Hex string, err error)`: verifica `CheckNoReparseChain`; devuelve `("absent", "", nil)` ante `os.ErrNotExist`; rechaza directorios o reparse points con error; sobre archivos regulares lee bytes, elimina UTF-8 BOM (`\xef\xbb\xbf`), normaliza `\r\n` y `\r` a `\n`, y retorna `("present", hexSHA256, nil)`.

#### 3. `pkg/bfcore/brain_client.go` (Create)

*   **Type / Scope**: Go Library (`blackfactory/pkg/bfcore`, solo stdlib + primitivas de `bfcore`).
*   **Proposed Changes**:
    - Extraer desde `1_Intake/engine/brain_client.go` los tipos compartidos (`BrainPreflight`, `BrainContextResponse`, `BrainDocMatch`, `RiskHit`, `BrainInvocation`, `BrainQueryOpts`, `BrainClient`, `SubprocessBrainClient`), constructores (`NewSubprocessBrainClient`), helpers FTS5 (`ExtractQueryTokens`, `BuildDisjunctiveFTSQuery`) y verificación en disco (`IsProjectActiveOnDisk`).
    - Definir sentinels independientes (`ErrBrainUnavailable = errors.New(...)`, `ErrBrainIndexMissing = errors.New(...)`, `ErrBrainJournalActive = errors.New(...)`) y en `Preflight` devolver errores multi-wrap (`fmt.Errorf("%w: %w: %s", ErrBrainUnavailable, ErrBrainJournalActive, detail)` y `fmt.Errorf("%w: %w: %s", ErrBrainUnavailable, ErrBrainIndexMissing, detail)`), de modo que `errors.Is` reconozca ambas identidades (`ErrBrainUnavailable` para exit `4` en `1_Intake`, y `ErrBrainJournalActive` / `ErrBrainIndexMissing` para distinguir `BRAIN_JOURNAL_ACTIVE` vs. `BRAIN_UNAVAILABLE` en exit `6` de `2_Spec_Writer`).
    - Incluir helper read-only `ParseRiskZonesCoverPatterns(vaultRoot string) ([]RiskZonePattern, error)` (verificando `CheckNoReparseChain`) para cruce bidireccional de prefijos de directorio en `ASDD` `can_modify` cuando un directorio excede el límite de expansión o aún no existe en disco.

#### 4. `pkg/bfcore/bfcore_test.go` (Modify) & `pkg/bfcore/README.md` (Modify)

*   **Type / Scope**: Unit Tests & Package Docs (`pkg/bfcore`).
*   **Proposed Changes**:
    - Añadir pruebas TDD exhaustivas en `bfcore_test.go`: `EvaluateFTDBounds` (5 productivos + 5 tests Go hermanos = 10 PASS; 5 productivos + 6 tests Go = FAIL; test Go huérfano sin hermano = `1:1`; tests Python/TS = `1:1`; test Go en segundo módulo raíz = FAIL; archivos raíz `.` + subdirectorio = 2 módulos), `SanitizeRelativePaths` (combinaciones `AllowDir`/`AllowGlob`), `ComputeNormalizedFileSHA256` (CRLF vs LF con mismo hash, BOM strip, `"absent"` en archivo inexistente, error en directorio), `CheckNoReparseUpToNearestExisting`, y `SubprocessBrainClient.Preflight` verificando ambas identidades con `errors.Is` (caso journal activo: `errors.Is(err, ErrBrainJournalActive) == true && errors.Is(err, ErrBrainUnavailable) == true && !errors.Is(err, ErrBrainIndexMissing)`; caso índice ausente: `errors.Is(err, ErrBrainIndexMissing) == true && errors.Is(err, ErrBrainUnavailable) == true && !errors.Is(err, ErrBrainJournalActive)`).
    - Actualizar `pkg/bfcore/README.md` documentando los nuevos símbolos e invariantes.

#### 5. `1_Intake/engine/paths.go`, `brain_client.go`, `triage.go`, `model.go`, `intake_test.go` y `README.md` (Modify)

*   **Type / Scope**: Go Library, Unit Tests & Docs (`blackfactory/intake`).
*   **Proposed Changes**:
    - `1_Intake/engine/paths.go`: delegar `SanitizeAffectedPaths` a `bfcore.SanitizeRelativePaths(..., PathSanitizeOpts{AllowDir: false, AllowGlob: false, RejectPersonalG4: true})` y `CountRootModules` a `bfcore.CountRootModules`.
    - `1_Intake/engine/brain_client.go` & `model.go`: delegar/re-exportar alias de tipos y errores desde `pkg/bfcore` (`DefaultMaxFTDFiles = bfcore.DefaultMaxFTDFiles`) manteniendo 100% intacta la serialización JSON de `WorkItem` (`workitem.schema.json` v1.0 sin cambios).
    - `1_Intake/engine/triage.go`: reemplazar la comprobación inline (`L211-220`) por `bfcore.EvaluateFTDBounds(wi.AffectedPaths, maxFTDFiles)` (Opción `5B`).
    - `1_Intake/engine/intake_test.go`: añadir casos TDD para Opción `5B` en `TriageWorkItem` (5 productivos + 2 `*_test.go` hermanos → `FTD`; 5 productivos + 6 `*_test.go` → `ASDD`; `test_app.py` → `1:1`; test Go en segundo módulo → `ASDD`), verificando 0 regresiones en toda la suite previa.
    - `1_Intake/engine/README.md`: actualizar referencias hacia las primitivas delegadas a `pkg/bfcore`.

---

#### Fase 1 + Integración — Módulo `2_Spec_Writer`, Esquema, Skills y Cableado Raíz

#### 6. `go.work` (Modify), `.gitignore` (Modify) y `README.md` (Modify)

*   **Type / Scope**: Root Workspace Configuration, Option i Privacy & Root Docs.
*   **Proposed Changes**:
    - `go.work`: añadir `./2_Spec_Writer/engine` al bloque `use (...)`.
    - `.gitignore`: añadir `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` en la sección `Opción i`.
    - `README.md`: añadir sección de verificación y compilación de `2_Spec_Writer Engine` (`CGO_ENABLED=0`, `GENGIS_CONTRACT_STRICT=1`, `specwriter.exe verify`).

#### 7. `2_Spec_Writer/README.md` (Modify), `2_Spec_Writer/schemas/spec.schema.json` (Create), `2_Spec_Writer/skills/README.md` (Create) y `2_Spec_Writer/skills/spec-authoring/SKILL.md` (Create)

*   **Type / Scope**: Team 2 Documentation, Canonical JSON Schema Draft 2020-12 & Progressive-Load Agent Skill.
*   **Proposed Changes**:
    - `2_Spec_Writer/README.md`: reemplazar el esqueleto de 5 líneas con la documentación completa del Equipo 2 (Overview, límites, arquitectura sandwich, tabla comparada de códigos `0..6` vs `1_Intake 0..5`, invariantes `G1–G6`/`Opción i`, efectos secundarios y comandos de verificación).
    - `2_Spec_Writer/schemas/spec.schema.json`: esquema JSON Draft 2020-12 (`schema_version: "1.0"`, `additionalProperties: false` en todos los niveles, cero `extensions`) cubriendo todos los campos de la Sección 3 del Plan Técnico, incluyendo condicionales `if/then` para `FTD` vs `ASDD`, `observed_files[]` (`state: "present" | "absent"`), `decisions[]` con `approved_paths[]` + `scope_sha256`, `verify_steps[]` (`expect_exit: 0`, `timeout_sec: 1..300`) y `failure_proof` (`expect_exit: 1..255`).
    - `2_Spec_Writer/skills/README.md` & `2_Spec_Writer/skills/spec-authoring/SKILL.md`: guía progresiva del bucle operativo para agentes externos (`prepare → editar spec.json → validate --stage draft → [ASDD Goldfish + validate --stage final] → render → compile → [AUTOR] gengis.exe plan`), reglas de `observed_files[]`, `decisions[]` y recuperación ante exits `1..6`.

#### 8. `2_Spec_Writer/engine/go.mod`, `go.sum`, `README.md`, `embed.go` y `templates/{index,chunk,status}.md.tmpl` (Create)

*   **Type / Scope**: Go Module Configuration (`blackfactory/specwriter`, `CGO_ENABLED=0`, sin SQLite) & Embedded Templates (`go:embed`).
*   **Proposed Changes**:
    - `go.mod` / `go.sum`: `module blackfactory/specwriter`, `go 1.26.0`, requiriendo únicamente `blackfactory/pkg/bfcore` (`replace => ../../pkg/bfcore`) y `golang.org/x/sys v0.49.0`.
    - `embed.go`: embeber `spec.schema.json` (o copia sincronizada) y `templates/*.md.tmpl` mediante `//go:embed`.
    - `templates/index.md.tmpl`, `templates/chunk.md.tmpl`, `templates/status.md.tmpl`: plantillas deterministas `GengiSWAP` v1 usando exclusivamente encabezados `##` H2 (`inCoreH2 = true` en `bridge.go:458`), bloques fenced para `Comando de Verificación`, separación estricta de `Prueba de que puede fallar`, cero backticks no-ruta en `## Alcance` / `## Fuera de Alcance`, línea de trazabilidad con `spec_sha256` y `trace_id` sin rutas absolutas que engañen a `extractTargetRepo`, y cero bloques Mermaid.
    - `2_Spec_Writer/engine/README.md`: índice de símbolos exportados, invariantes, efectos secundarios y comando de verificación.

#### 9. `2_Spec_Writer/engine/model.go` (Create)

*   **Type / Scope**: Go Domain Model & Canonical Hashing (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Definir constantes de salida (`ExitOK = 0`, `ExitFixable = 1`, `ExitUsage = 2`, `ExitG4 = 3`, `ExitExternalDecision = 4`, `ExitAttemptsBlocked = 5`, `ExitInfraOrRefusal = 6`), límites (`MaxValidateAttemptsPerStage = 3`, `MaxExternalCycles = 3`, `MaxDirExpansionFiles = 128`, `MaxVerifyTimeoutSec = 300`) y catálogo cerrado de `reason` + `retryable`.
    - Definir structs con tags JSON exactos: `Spec`, `WorkItemRef` (`source_workitem_path` relativo a `--factory-root`, nunca ruta absoluta de host), `Narrative`, `ObservedFile`, `G5Citation`, `DecisionRecord`, `RiskMitigation`, `BrainFollowup`, `WorkUnit`, `ScopeExpansion`, `AcceptanceCriterion`, `VerifyStep`, `FailureProof`, `SpecContext` (`repo_root_basename` + `repo_root_sha256`, nunca ruta absoluta de host), `AttemptsState` (con `draft_attempts`, `final_attempts`, `external_cycles`, `last_exit`, `last_reason`, `last_passed_sha256`, `last_passed_stage`, `last_passed_goldfish_sha256`), `GoldfishReview` (`report_path` relativo), `SpecWriterEvent`, `ValidationIssue{Pointer, Code, Message}` y `CLIResult`.
    - Exportar `CanonicalSpecSHA256(s Spec) (string, error)` (normalizando slices `nil -> []`, serialización compacta determinista sin indentación/CRLF) y `ComputeScopeSHA256(paths []string) string`.

#### 10. `2_Spec_Writer/engine/state.go` (Create)

*   **Type / Scope**: Go Runtime Persistence & Concurrency (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Gestionar rutas de runtime `runtime/<project>/<WI-id>/` con validación `CheckNoReparseChain`, cerrojo granular `.wi.lock` (`bfcore.AcquireFileLock` con backoff acotado 250 ms) y cerrojo breve `.events.lock` para appends en `runtime/specwriter_events.jsonl`.
    - Implementar `WriteJSONAtomicG4(path string, v any) error`: serializa, verifica `bfcore.ScanG4All` con `0` hallazgos antes de escribir, escribe archivo temporal hermano + `Sync` + reemplazo atómico con reintentos Windows.
    - Implementar `LoadAttemptsStrict(wiDir string) (AttemptsState, error)` (fail-closed `ATTEMPTS_STATE_MISSING` / `ATTEMPTS_STATE_CORRUPT` si falta o JSON inválido) y `ReadFileWithShortRetry(path string) ([]byte, error)` (3×25 ms, exclusivamente solo lectura para `verify`).
    - Implementar `AppendEvent(dataDir string, ev SpecWriterEvent) error` garantizando `ScanG4All == 0` y propagando `trace_id`.

#### 11. `2_Spec_Writer/engine/verify.go` (Create)

*   **Type / Scope**: Go Read-Only Shared Verification Engine (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Separar explícitamente `FactoryRoot` (`--factory-root`, default `RepoRoot`) y `RepoRoot` (`--repo-root`).
    - Exportar comprobaciones reutilizables (100% solo lectura, 0 cerrojos exclusivos, 0 creación de archivos):
      - `CheckGitIgnore(factoryRoot string) error`: invoca `git -C <factoryRoot> check-ignore -q 2_Spec_Writer/runtime/` y `git -C <factoryRoot> check-ignore -q 2_Spec_Writer/specwriter.exe` **por separado** (exigiendo exit `0` en cada llamada individual, evitando el falso positivo de `git check-ignore` multi-argumento → `GITIGNORE_DRIFT`, exit `6`, `retryable: false`).
      - `CheckReparseSafety(paths ...string) error` → `REPARSE_POINT_DETECTED`, exit `6`, `retryable: false`.
      - `CheckBrainPreflight(client bfcore.BrainClient, vaultRoot, dbPath string) (bfcore.BrainPreflight, string, error)` → distingue `BRAIN_JOURNAL_ACTIVE` vs `BRAIN_UNAVAILABLE` (exit `6`, `retryable: true`).
      - `CheckGengisEngineHashes(factoryRoot string) error` (compara `ComputeNormalizedFileSHA256` de `GengiSWAP/engine/bridge.go` y `GengiSWAP/engine/ring.go` contra las constantes ancladas `PinnedBridgeGoSHA256` / `PinnedRingGoSHA256` → `GENGIS_ENGINE_DRIFT`, exit `6`, `retryable: false`).
      - `VerifyRuntime(opts VerifyOptions) (VerifyReport, int)`: combina los 4 checks anteriores + escaneo `ScanG4All` y sintaxis JSON de todos los archivos existentes bajo `runtime/` (`G4_VIOLATION_ON_DISK` → exit `3`; `ATTEMPTS_STATE_CORRUPT` → exit `6`).

#### 12. `2_Spec_Writer/engine/prepare.go` (Create)

*   **Type / Scope**: Go Preparation & Context Freezing (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Exportar `Prepare(opts PrepareOptions) (PrepareResult, int)`:
      - Validar `WorkItem` de entrada: `ScanG4All` sobre bytes crudos y strings decodificados (exit `3`), `schema_version == "1.0"`, `status == "ready"`, `lane in {"FTD","ASDD"}`.
      - Ejecutar `CheckReparseSafety` y `CheckBrainPreflight` (exit `6` si falla `0_Brain` o hay `.brain-journal.json`), más `brain.Context` sobre `WorkItem.AffectedPaths` filtrando `status == "active"`.
      - Calcular `repo_root_basename` + `repo_root_sha256` (nunca rutas absolutas de host) y `source_workitem_path` relativo a `--factory-root` (o basename si queda fuera).
      - Comportamiento bajo `.wi.lock`:
        - **WorkItem nuevo:** crear `spec_context.json` (`baseline_hashes[]` con `ComputeNormalizedFileSHA256`), inicializar `attempts.json` en `0` y escribir esqueleto inicial `spec.json` (`observed_files[]` pre-poblado desde `baseline_hashes[]`, `requires_autor_hitl` heredado intacto).
        - **WorkItem ya existente sin flags:** fallar cerrado (`exit 2`) sin sobrescribir `spec.json` ni tocar contadores.
        - **`--refresh-context`:** actualizar **únicamente** `spec_context.json`, invalidar `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json` sin sobrescribir `spec.json` ni reiniciar `draft_attempts`, `final_attempts` ni `external_cycles`, y emitir evento `CONTEXT_REFRESHED`.
        - **`--reset-attempts --reason "<motivo>"`:** exigir `--reason` (`len >= 8`, exit `2` si falta, exit `3` si dispara `ScanG4All`), reiniciar `draft_attempts = 0`, `final_attempts = 0`, `external_cycles = 0`, limpiar `last_passed_*` sin sobrescribir `spec.json`, y emitir evento `ATTEMPTS_RESET`.

#### 13. `2_Spec_Writer/engine/validate.go` (Create)

*   **Type / Scope**: Go Static Validation Engine (`blackfactory/specwriter`, 0 ejecución de comandos).
*   **Proposed Changes**:
    - Implementar la precedencia estricta `2 → 5 → 3 → 6 → 4 → 1`:
      - **`[5]`:** `LoadAttemptsStrict` (exit `6` si falta/corrupto); si `stage_attempts >= 3` → exit `5` (`MAX_VALIDATE_ATTEMPTS` + `last_exit`/`last_reason` sin payload).
      - **`[3]`:** doble fase `G4` fail-closed sobre las 11 reglas (bytes crudos + todos los strings JSON decodificados de `spec.json` y, en `--stage final`, `goldfish_review.json`); ante hallazgo incrementar `stage_attempts`, registrar `REJECTED_G4` y devolver exit `3`.
      - **`[6]`:** `CheckReparseSafety` + `CheckBrainPreflight` (exit `6`).
      - **`[4]` (decisión externa — si dispara y `external_cycles >= 3` → exit `5` `MAX_EXTERNAL_CYCLES`; si `< 3` → incrementar y exit `4`):**
        - `STALE_CONTEXT`: verificar `work_item_ref` vs `spec_context.json`, `risk_zones_sha256`, `brain_db_mtime`, `baseline_hashes[]` y cada entrada concreta de `spec.observed_files[]` (`"present"` debe mantener hash CRLF/BOM idéntico; `"absent"` debe seguir ausente en disco).
        - Expansión controlada de prefijos de directorio `ASDD` en `can_modify`: (a) si el directorio no existe, `CheckNoReparseUpToNearestExisting` + sonda sintética `<prefijo>/__new__` + cruce bidireccional con `ParseRiskZonesCoverPatterns`; (b) si existe, `WalkDir` omitiendo `.git/`, `node_modules/`, `runtime/`, `.dev/`, rechazando reparse points internos y aplicando tope `MaxDirExpansionFiles = 128` archivos ordenados para `brain.Context` + cruce bidireccional de `risk_zones`.
        - Re-consulta `0_Brain` filtrando `status == "active"`: disparar `ESCALATION_REQUIRED` (subida de `risk_level`, nueva zona `CRITICO` o `len(RootModules) > 1` sin `decisions[]` válido cuyo `scope_sha256` y `approved_paths[]` cubran el `can_modify` actual) o `LANE_UPGRADE_REQUIRED` (`spec.lane == "FTD"` pero `bfcore.EvaluateFTDBounds` falla bajo Opción `5B` o hay ruta sin cobertura `wiki`/`adr` activa en `0_Brain`).
      - **`[1]` (errores con JSON Pointer RFC 6901; incrementa `stage_attempts` al fallar):**
        - Decodificación estricta `DisallowUnknownFields()`, obligatoriedad por carril (`FTD` vs `ASDD`), DAG acíclico en `depends_on`, `can_modify` disjunto entre unidades sin dependencia, `1..2` `work_units` en `FTD`.
        - `G2` (cero `` ```mermaid ``), `G5` (existencia, 1-based CRLF/BOM, ancla literal sensible a mayúsculas `±5` líneas para extensiones de código `.go`, `.py`, `.js`, `.ts`, `.sql`, `.sh`, `.ps1`, rechazando tokens `len < 4` o palabras reservadas `err`, `func`, `return`, `if`, `for`, `nil`, `true`, `false`).
        - `risk_mitigations[]` cubriendo 100% de `risk_hits`, `requires_autor_hitl` monotónico, `CRITICO` con `>= 1` `review_gate` y ejecutor cerrado (`4_Revisores` | `Modo_QA` | `AUTOR`), `>= 1` `verify_step` real por unidad.
        - **Allowlist cerrado de `verify_steps[]`:** tabla cerrada de ejecutables/subcomandos (`go test|vet|build`, `git status|diff|rev-parse|check-ignore`, `0_Brain/brain.exe verify|query|context`, `1_Intake/intake.exe verify|list|show`, `2_Spec_Writer/specwriter.exe verify|render`; excluyendo `GengiSWAP/gengis.exe status` e intérpretes/shells), normalización de banderas `-flag`/`--flag`/`-flag=val` bloqueando `-exec`, `-toolexec`, `-vettool`, `-overlay`, `-pkgdir`, `-o`, `-c`, `-outputdir`, `-coverprofile`, `-cpuprofile`, `-memprofile`, `-blockprofile`, `-mutexprofile`, `-trace` en `go` y `-c`/`-config-env`/`-exec-path`/`-git-dir`/`-work-tree`/`-upload-pack` en `git`, allowlist `env` sin `GENGIS_BIN`, rechazo sin escape de `& | < > ^ % ! " \r \n \x00`, `cwd` y rutas relativas, `expect_exit == 0`, `timeout_sec in 1..300`.
        - En `--stage final` (`ASDD`): validar `goldfish_review.json` (`spec_sha256` coincidente, `unresolved_blockers == 0`, lentes base + condicionales presentes, `report_path` relativo limpio bajo `G4`).
      - **`[0]`:** persistir `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json` (sin resetear `final_attempts` al pasar `draft`) y emitir `VALIDATE_PASS`.

#### 14. `2_Spec_Writer/engine/render.go` (Create)

*   **Type / Scope**: Go Deterministic Markdown Renderer (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Exportar `RenderSpecMarkdown(specBytes []byte, spec Spec) (string, int, error)`: ejecuta `ScanG4All` sobre entrada y salida renderizada (exit `3` si halla fuga), estampa cabecera `AUTO-GENERATED DERIVED VIEW: DO NOT EDIT` con `spec_sha256` y `trace_id`, proyecta secciones `FTD`/`ASDD`, tabla de `work_units`, `risk_mitigations`, `decisions`, `g5_citations`, `brain_followups` y diagrama de dependencias en texto ASCII/Unicode Box-Drawing (cero Mermaid).

#### 15. `2_Spec_Writer/engine/compile.go` (Create)

*   **Type / Scope**: Go Deterministic `GengiSWAP` Adapter (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Fijar constantes `PinnedBridgeGoSHA256` y `PinnedRingGoSHA256` calculadas sobre los archivos vigentes de `GengiSWAP/engine/{bridge,ring}.go`.
    - Implementar la precedencia `2 → 5 → 3 → 6 → 4 → 1`:
      - Saneamiento estricto de `--task-id` (`^[a-z0-9][a-z0-9_-]{2,63}$` sin dispositivos reservados Windows → exit `2`).
      - Check `[5]` sobre `attempts.json`: `stage_attempts >= 3` **o** `external_cycles >= 3` → exit `5`.
      - Check `[3]` `G4` fail-closed sobre `spec.json`.
      - Check `[6]`: `CheckBrainPreflight`, `CheckReparseSafety` sobre `--task-root` y destino, adquirir `<task-root>/.<task-id>.compile.lock` (`LOCK_BUSY`), y verificar destino en modo **estrictamente *create-only* en v1** dentro del cerrojo: si existe `<dest>/.ring/state.json` → `LIVE_TASK_STATE` (exit `6`, `retryable: false`); si `<dest>` ya existe aunque no tenga `.ring/state.json` → `TASK_DIR_EXISTS` (exit `6`, `retryable: false`).
      - Check `[4]`: revalidación de contexto/alcance en vivo sin consumir intentos.
      - Check `[1]` anti-bypass: verificar `(spec_sha256 == last_passed_sha256)` + etapa requerida (`draft`/`final` en `FTD`, `final` en `ASDD`) + en `ASDD` SHA-256 normalizado de `goldfish_review.json == last_passed_goldfish_sha256` → si falla, devolver exit `1` (`VALIDATION_REQUIRED`, `retryable: true`). Luego correr el bloque `[1]` + superconjunto de `validateChunks` (`ring.go:393-426`) + renderizado en memoria de `index.md`, `chunks/NN_<id>.md`, `status.md` + `ScanG4All` en memoria; cualquier fallo en este punto emite exit `1` (`VALIDATOR_BYPASS_DEFECT`, `retryable: false`, sin consumir intento) y lo registra en `specwriter_events.jsonl`.
      - Promoción atómica *create-only* en Windows dentro de `.compile.lock`: crear staging hermano en el mismo volumen NTFS (`<task-root>/.<task-id>.staging-<rand>`) con `defer os.RemoveAll(stagingDir)` ante cualquier fallo previo al rename, escribir y hacer `Write` + `Sync` + `Close` sobre `index.md`, `chunks/NN_<id>.md`, `status.md` inicial y `workspace/qa/`, re-comprobar con `os.Lstat` que `<dest>` sigue inexistente (`os.ErrNotExist`), ejecutar un único `os.Rename(stagingDir, dest)` atómico (con reintento acotado Windows 3×25 ms ante bloqueos transitorios de antivirus/indexador), registrar `COMPILED`, e imprimir resumen informado y comando `gengis.exe plan` para `[AUTOR]`.

#### 16. `2_Spec_Writer/engine/specwriter_test.go` (Create) & `2_Spec_Writer/engine/contract_gengis_test.go` (Create)

*   **Type / Scope**: Unit Tests, Security Regression Tests, Latency Benchmark & Real `gengis.exe plan` Contract Tests (`t.TempDir()`, `G3`).
*   **Proposed Changes**:
    - `specwriter_test.go`: tests TDD para todos los paths críticos: cero rutas absolutas de host bajo `t.TempDir()` en Windows (`C:\Users\...`), `prepare` + `--refresh-context` (no destructivo, conserva contadores) + `--reset-attempts --reason` (G4 + `ATTEMPTS_RESET`), precedencia `2 → 5 → 3 → 6 → 4 → 1`, presupuestos independientes `draft`/`final` + 4.º ciclo externo → exit `5`, `observed_files[]` `"present"`/`"absent"` + CRLF/BOM parity, prefijos de directorio `ASDD` existentes (tope `128`, omisión `.git/`) y no existentes (`CheckNoReparseUpToNearestExisting`), allowlist completo de comandos y banderas normalizadas (incl. `-o`, `-c`, `-coverprofile`, `-toolexec`, rechazo `GENGIS_BIN` y `gengis.exe status`), `decisions[]` con nueva expansión → re-dispara exit `4`, anti-bypass `VALIDATION_REQUIRED` (incl. edición/borrado de `goldfish_review.json`) vs `VALIDATOR_BYPASS_DEFECT`, destino *create-only* bajo `.compile.lock` (`.ring/state.json` → `LIVE_TASK_STATE`; directorio previo sin `.ring/state.json` → `TASK_DIR_EXISTS`; limpieza de staging en error), `verify` solo lectura (incluyendo caso donde `*.exe` está ignorado pero `2_Spec_Writer/runtime/` NO está ignorado → `GITIGNORE_DRIFT`, más `GENGIS_ENGINE_DRIFT` y `G4_VIOLATION_ON_DISK`) y benchmark `< 500 ms` p95.
    - `contract_gengis_test.go`: invoca `gengis.exe plan <tmpTask> --root <tmpRoot>` con `"n\n"` sobre `t.TempDir()` (`GENGIS_BIN` desde entorno del operador/CI; `t.Skip` explícito en local; `t.Fatalf` bajo `GENGIS_CONTRACT_STRICT=1`), cubriendo fixtures positivos (`FTD` con `failure_proof.expect_exit: 1` y `ASDD` multi-chunk verificando `printPlanSummary`, `PLAN_PENDING` y `"plan no aprobado por el Autor"`) y negativos de paridad (`FailureProof` vacío, `VerifyCommands` vacío, ID duplicado → `"chunk inválido"` sin crear `.ring/state.json`).

#### 17. `2_Spec_Writer/engine/cmd/specwriter/README.md`, `main.go` y `main_test.go` (Create)

*   **Type / Scope**: CLI Entry Point (`specwriter.exe`) & End-to-End Tests (`t.TempDir()`).
*   **Proposed Changes**:
    - `main.go`: subcomandos `prepare`, `validate`, `render`, `compile`, `verify`; flags `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--task-root`, `--task-id`, `--workitem`, `--spec`, `--stage`, `--refresh-context`, `--reset-attempts`, `--reason`, `--json`; códigos de salida deterministas `0..6` con salida JSON estructurada (`exit_code`, `reason`, `retryable`, `trace_id`, `spec_sha256`, `issues[]`).
    - `main_test.go`: pruebas E2E completas reproduciendo `CUJ 1` (`FTD` 5 productivos + 2 tests Go hermanos), `CUJ 2` (`ESCALATION_REQUIRED` → `decisions[]` → `ASDD` `--stage final`), `CUJ 3` (`G4` reincidente → exit `5` → `--reset-attempts`, y `LIVE_TASK_STATE` exit `6`), y `CUJ 4` (`verify` solo lectura con `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT` y archivo `"absent"` que aparece en disco).
    - `cmd/specwriter/README.md`: contrato de uso CLI, banderas, códigos `0..6` y verificación.

### Verification Plan

*   **Compilation / Build (`CGO_ENABLED=0`, cero SQLite en `pkg/bfcore`, `1_Intake` y `2_Spec_Writer`):**
    ```powershell
    $env:CGO_ENABLED = "0"
    Set-Location "C:\Black Factory\2_Spec_Writer\engine"
    go build -o ..\specwriter.exe .\cmd\specwriter
    ```
    Verificar que `2_Spec_Writer/engine/go.mod` y `pkg/bfcore/go.mod` no contienen `modernc.org/sqlite`.
*   **Static Analysis & Linting (`go vet` en todos los módulos del workspace):**
    ```powershell
    $env:CGO_ENABLED = "0"
    cd /d "C:\Black Factory\pkg\bfcore" && go vet ./... && cd /d "C:\Black Factory\0_Brain\engine" && go vet ./... && cd /d "C:\Black Factory\1_Intake\engine" && go vet ./... && cd /d "C:\Black Factory\2_Spec_Writer\engine" && go vet ./...
    ```
*   **Unit Tests, Coverage (`>= 85%`) y Contrato Obligatorio contra `gengis.exe plan` (`GENGIS_CONTRACT_STRICT=1`):**
    Antes de cualquier merge a `main`, la suite debe ejecutarse con **`GENGIS_CONTRACT_STRICT=1`** para garantizar que `contract_gengis_test.go` corra realmente contra `GengiSWAP/gengis.exe` (nunca `t.Skip`):
    ```powershell
    $env:CGO_ENABLED = "0"
    $env:GENGIS_CONTRACT_STRICT = "1"
    $env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
    cd /d "C:\Black Factory\pkg\bfcore" && go test -count=1 -cover ./...
    cd /d "C:\Black Factory\0_Brain\engine" && go test -count=1 ./...
    cd /d "C:\Black Factory\1_Intake\engine" && go test -count=1 -cover ./...
    cd /d "C:\Black Factory\2_Spec_Writer\engine" && go test -count=1 -cover ./...
    ```
    *Requisito de cobertura:* `>= 85%` de cobertura de sentencias en `blackfactory/pkg/bfcore`, `blackfactory/specwriter` y `blackfactory/specwriter/cmd/specwriter`, y cero regresiones en `0_Brain/engine` y `1_Intake/engine`.
*   **E2E / Health & Privacy Verification (`Opción i`):**
    ```powershell
    & "C:\Black Factory\2_Spec_Writer\specwriter.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\2_Spec_Writer\runtime" --json
    git -C "C:\Black Factory" status --ignored -s 2_Spec_Writer/
    ```
    *Resultado esperado:* `specwriter.exe verify` exit `0` (`ok: true`, `gitignore_ok: true`, `gengis_hashes_ok: true`, `brain_ok: true`, `g4_issues: 0`, `0` archivos creados en `runtime/`); `git status --ignored -s 2_Spec_Writer/` confirma `!! 2_Spec_Writer/specwriter.exe` y `!! 2_Spec_Writer/runtime/` ignorados.
*   **Latency Budget Verification:**
    Benchmark/test dedicado confirmando `< 500 ms` p95 local en Windows para `prepare`, `validate`, `render` y `compile`.

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
