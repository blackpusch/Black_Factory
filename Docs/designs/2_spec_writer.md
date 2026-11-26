# Equipo 2 de Black Factory (`2_Spec_Writer`): Compuerta Determinista de Especificaciones y Compilador hacia `GengiSWAP`

*   **Author:** blackpusch
*   **Status:** Approved
*   **Date:** 2026-11-23

--------------------------------------------------------------------------------

## Overview

`2_Spec_Writer` (`2_Spec_Writer/engine` → `2_Spec_Writer/specwriter.exe`) es la compuerta determinista de especificaciones técnicas y compilador hacia `GengiSWAP` de **Black Factory**. Compilado como binario pure-Go autónomo (`CGO_ENABLED=0`, objetivo principal Windows con fallback POSIX, sin SQLite y sin llamadas HTTP a LLM, compartiendo el paquete `pkg/bfcore` que ya consume `1_Intake/engine/go.mod:5-10` con `golang.org/x/sys v0.49.0`), conecta `1_Intake` (`1_Intake/runtime/workitems/WI-<hash12>.json`, esquema v1.0, mantenido como registro inmutable del triaje inicial) con `3_Builder` (`GengiSWAP/gengis.exe` sobre `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\`) consultando `0_Brain` (`0_Brain/brain.exe`) en modo estrictamente de solo lectura (*Read-Before-Act*).

### Glosario Base y Convenciones del Repositorio

Esta especificación se apoya en las definiciones base del repositorio ([`README.md`](../../README.md), [`Docs/BLACK_FACTORY_BRAIN.md`](../BLACK_FACTORY_BRAIN.md) y [`Docs/designs/1_intake.md`](1_intake.md)):
*   **Carriles de Trabajo (`FTD` y `ASDD`):**
    - **`FTD` (*Fast-Track Development*):** carril rápido para cambios acotados y de riesgo controlado (`<= 5` archivos efectivos en `<= 1` módulo raíz, con cobertura activa en `0_Brain`, `WorkItem.kind` acotado a `bug|audit_fix|refactor` —excluyendo `feature|ops`—, sin impacto `CRITICO` ni `impact_flags`). Requiere narrativa básica, `1..2` unidades de trabajo (`work_units[]`) y supera `validate --stage draft` antes de compilar.
    - **`ASDD` (*Agentic / Spec-Driven Development*):** carril completo para cambios multi-módulo, nuevas funcionalidades, riesgo `CRITICO` o áreas sin cobertura documental. Exige narrativa extendida (`alternatives_considered`, `interfaces_and_contracts`, `edge_cases`, `rollback_plan`), revisión independiente **Goldfish** y superar `validate --stage final` antes de compilar.
*   **`EGM` (*Elephant-Goldfish Model*) y Revisión `Goldfish`:** protocolo de auditoría donde el agente principal con historial (*Elephant*) somete la especificación a subagentes revisores lanzados por el harness externo en contexto limpio sin historial previo (*Goldfish*, ej. lentes `egm-design`, `egm-codebase-details`, `egm-implementation-readiness`, `egm-structural`, `egm-zoom-out`). Cada corrida de revisión genera un informe Markdown en `report_path` y el harness registra el resumen estructurado en `goldfish_review.json`.
*   **Invariantes Globales (`G1–G6`):**
    - **`G1`:** Evidencia literal (comandos exactos, códigos de salida y salidas).
    - **`G2`:** Trazabilidad y diagramas ASCII/Unicode Box-Drawing (prohibiendo bloques Mermaid).
    - **`G3`:** Integridad de datos (pruebas sobre `t.TempDir()` sin mutar datos reales).
    - **`G4`:** Cero fugas de secretos o rutas personales mediante las **11 reglas** de `pkg/bfcore/g4.go:48-115`: **Tier 1** (8 reglas de secretos duros: claves privadas, tokens GitHub/AWS/OpenAI/Slack, contraseñas en URI) y **Tier 2** (3 reglas: rutas personales `C:\Users\<user>\...` / `/home/<user>/...` / `/Users/<user>/...` y correos electrónicos). En `2_Spec_Writer`, las 11 reglas operan en modo **fail-closed (`exit 3`, sin auto-redacción Tier 2)**, admitiendo los marcadores `[REDACTED:G4:<rule>]` heredados de `1_Intake`.
    - **`G5`:** Rigor técnico (citas `archivo:línea` verificables en disco).
    - **`G6`:** Principio de parada ante ambigüedad o incumplimiento de precondiciones.
*   **Convenciones de la Factoría (`OKF`, `Opción 3C`, `Opción i`, `[AUTOR]`, `HITL`, `RZ`):**
    - **`OKF` (*Open Knowledge Format*):** bóveda Markdown + índice SQLite FTS5 gestionados por `0_Brain`.
    - **`Opción 3C`:** esquemas/plantillas embebidas (`go:embed`), skills de carga progresiva y CLI Go determinista.
    - **`Opción i`:** directorios operativos locales (`0_Brain/vault/`, `1_Intake/runtime/`, `2_Spec_Writer/runtime/`) y binarios `.exe` mantenidos fuera de Git vía `.gitignore`.
    - **`[AUTOR]` y `HITL` (*Human-in-the-Loop*):** autoridad humana (`blackpusch`), ejercida en v1 en una única parada informada en `gengis.exe plan`.
    - **`RZ` (*Risk Zone*):** zonas catalogadas en `0_Brain/vault/wiki/risk_zones.md` (`RZ-001` = `GengiSWAP/engine/` nivel `CRITICO`, intacta en v1; `RZ-002` = `0_Brain/engine/` nivel `ALTO`).

`2_Spec_Writer` es el primer equipo del pipeline donde participa un LLM —siempre desde el harness externo (Antigravity / CLI), nunca invocado desde Go— operando bajo una **arquitectura en tres fases** (pre-validación Go → autoría LLM externa → compuerta/compilación Go) de cinco subcomandos (`prepare`, `validate`, `render`, `compile` y `verify`):
1. **`prepare`:** valida `WorkItem` con `ScanG4All` (`exit 3`), verifica esquema v1.0 y `status: "ready"` (`exit 2` si es inválido o está en `needs_clarification`), ejecuta el preflight compartido (`CheckReparseSafety` + `CheckBrainPreflight`, `exit 6`), preserva `requires_autor_hitl: true` sin bloquear, congela `spec_context.json` (hashes SHA-256 sobre bytes normalizados CRLF/BOM, `risk_zones_sha256`, `matched_doc_hashes[]`, `HEAD` informativo, `trace_id`), inicializa `attempts.json` en `0` y genera el esqueleto `spec.json` bajo `2_Spec_Writer/runtime/<project>/<WI-id>/`. Sobre un WorkItem ya preparado, `--refresh-context` actualiza únicamente `spec_context.json` e invalida los hashes de validación previa sin sobrescribir `spec.json` ni reiniciar contadores; el desbloqueo tras `exit 5` ocurre únicamente vía `--reset-attempts --reason "<motivo>"` (auditado con `G4` y evento `ATTEMPTS_RESET`).
2. **Autoría externa (LLM en harness) + bucle corto con `validate [--stage draft|final]`:** el agente edita exclusivamente `spec.json` (`additionalProperties: false`, única fuente de verdad), registrando `observed_files[]` (`state: "present" | "absent"`, rutas de archivo concretas) y `decisions[]` acotado al alcance aprobado (`approved_paths[]` + `scope_sha256`). `validate` ejecuta la precedencia fija `2 → 5 → 3 → 6 → 4 → 1` con presupuestos independientes por etapa (`draft`: máx. 3, `final`: máx. 3) y `external_cycles` acumulativo (máx. 3), devolviendo errores JSON Pointer (RFC 6901), `reason`, campo booleano `retryable` y persistiendo `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json` al pasar (`exit 0`). `--stage final` es obligatorio únicamente para `ASDD` antes de `compile`; `FTD` compila tras superar `--stage draft`.
3. **`render`, `compile` y `verify`:** `render` valida `G4` antes de proyectar Markdown determinista hacia `stdout` (cero escrituras en disco) sellado con `spec_sha256` canónico (nunca re-ingestado). `compile` ejecuta precedencia propia (`2 → 5 → 3 → 6 → 4 → 1`), impide bypass exigiendo coincidencia con `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json` (`VALIDATION_REQUIRED` vs. `VALIDATOR_BYPASS_DEFECT`), reutiliza el validador compartido + superconjunto de `validateChunks` + allowlist cerrado de comandos/banderas + `G4` en memoria (aplicando frontera de dos zonas para inyectar `--repo-root` únicamente en el `index.md` externo tras `ScanG4All`), protege `.ring/state.json` bajo cerrojo `<task-root>/.compile.lock` (`LIVE_TASK_STATE`, `exit 6`, `retryable: false`) y aplica promoción atómica *create-only* en Windows NTFS (`TASK_DIR_EXISTS`, `exit 6`, `retryable: false`), genera `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\` e imprime el resumen previo a `gengis.exe plan`. `verify` es 100% solo lectura (separando `--factory-root` de `--repo-root`) y ejecuta exclusivamente los chequeos de deriva de `.gitignore` (`GITIGNORE_DRIFT`) y deriva de `GengiSWAP/engine/{bridge,ring}.go` (`GENGIS_ENGINE_DRIFT`) bajo `exit 6` (`retryable: false`).

### Mapa Comparado de Códigos de Salida en el Pipeline (`1_Intake` vs. `2_Spec_Writer`)

| Código | `1_Intake` (`intake.exe`, `0..5`) | `2_Spec_Writer` (`specwriter.exe`, `0..6`) | Consume intento (`validate`) | Consume ciclo externo | `retryable` (JSON) |
| :---: | :--- | :--- | :---: | :---: | :---: |
| `0` | `ready` / éxito | Éxito (`prepare`, `validate`, `render`, `compile`, `verify`; `reason: "OK"`) | No (en `validate`, persiste `last_passed_sha256` + `last_passed_stage` + `last_passed_goldfish_sha256` sin resetear `final_attempts`) | No | `false` |
| `1` | Error técnico o de uso CLI | Errores corregibles en `spec.json` o `goldfish_review.json` (`VALIDATION_FAILED`, `INVALID_JSON`, `SPEC_TOO_LARGE` en `validate`; `INVALID_JSON`, `SPEC_TOO_LARGE`, `VALIDATION_FAILED` [`schema_version`] en `render`; `INVALID_JSON`, `SPEC_TOO_LARGE`, `VALIDATION_REQUIRED` en `compile`), o defecto de invariante en `compile` (`VALIDATOR_BYPASS_DEFECT`) | **Sí** en `validate` (`draft` o `final`); **No** en `render`/`compile` | No | `true` (`VALIDATION_FAILED`, `INVALID_JSON`, `SPEC_TOO_LARGE`, `VALIDATION_REQUIRED`); `false` (`VALIDATOR_BYPASS_DEFECT`) |
| `2` | Cuarentena `G6` (`needs_clarification`) | Error de uso CLI, argumento mal formado, `--spec` inexistente, `WorkItem` inválido/`needs_clarification` o `spec` ya existente sin flags (`INVALID_USAGE`, `SPEC_ALREADY_EXISTS`) | No | No | `false` |
| `3` | Rechazo Tier 1 `G4` | Rechazo fail-closed `G4` en las **11 reglas** sin auto-redacción Tier 2 (`G4_VIOLATION`, `G4_VIOLATION_ON_DISK`) | **Sí** en `validate`; No en otros subcomandos | No | `false` |
| `4` | `0_Brain` no disponible / lock | Decisión externa requerida (`STALE_CONTEXT`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`) | No | **Sí** en `validate` (acumulativo máx. 3; el 4.º devuelve `5`); **No** en `compile` (solo lectura sobre `attempts.json`) | `false` |
| `5` | Conflicto de deduplicación (`dedupe_key`) | Tope alcanzado (`MAX_VALIDATE_ATTEMPTS` por etapa o `MAX_EXTERNAL_CYCLES`; incluye `last_exit` y `last_reason` sin transcribir secretos ni cadenas disparadoras; desbloqueo con `prepare --reset-attempts --reason`) | No (resultado del tope) | No (resultado del tope) | `false` |
| `6` | *(no usado)* | Infraestructura transitoria (`BRAIN_UNAVAILABLE`, `BRAIN_JOURNAL_ACTIVE`, `LOCK_BUSY`, `IO_ERROR` → `retryable: true`) **o** negativa deliberada / estado corrupto / deriva (`LIVE_TASK_STATE`, `TASK_DIR_EXISTS`, `ATTEMPTS_STATE_MISSING`, `ATTEMPTS_STATE_CORRUPT`, `ARTIFACT_STATE_CORRUPT`, `REPARSE_POINT_DETECTED`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT` → `retryable: false`) | No | No | `true` (infra transitoria) / `false` (negativa deliberada, estado corrupto o deriva) |

## The Problem

Una vez completados `0_Brain` (memoria persistente OKF/FTS5 y gobernanza Ed25519) y `1_Intake` (puerta perimetral `G4` y triaje determinista `ASDD`/`FTD`, implementado en `1_Intake/engine/` y cerrado en la tarea `bf_01_intake_v1`), `2_Spec_Writer` existe solo como esqueleto de 5 líneas ([`2_Spec_Writer/README.md`](../../2_Spec_Writer/README.md)), y el traspaso entre especificaciones y `GengiSWAP` depende hoy de redacción manual guiada por prompt ([`.agents/skills/egm-to-gengis/SKILL.md`](../../.agents/skills/egm-to-gengis/SKILL.md)). Sin un motor determinista en `2_Spec_Writer`, la factoría enfrenta seis brechas verificadas en el código:

1. **Verificación dependiente de la buena voluntad del modelo (`constitution.md:22`):**
   Un agente redactando especificaciones libremente puede alucinar citas `archivo:línea` (`G5`), omitir compuertas heredadas de `wiki/risk_zones.md`, introducir bloques Mermaid (`G2`) o colar rutas personales/secretos (`G4`), haciendo que `3_Builder` desperdicie ciclos sobre especificaciones inválidas.
2. **Deriva entre prosa revisada y plan compilado (doble fuente de verdad):**
   Cuando un agente escribe por separado un Markdown de diseño y los archivos `index.md` / `chunks/NN_*.md` de `GengiSWAP`, `[AUTOR]` puede revisar una narrativa que diverge de los alcances o comandos realmente entregados a `gengis.exe`.
3. **Trampas de parseo y desajustes del motor `GengiSWAP` (`bridge.go` / `ring.go` / `runner_windows.go`):**
   La compilación manual en Markdown tropieza con restricciones reales verificadas en disco dentro de `GengiSWAP/engine/`:
   - `gengis.exe status <tmpDir>` (`ShowStatus`, `bridge.go:1455-1488`) llama `ParseTaskFromDir` y `SyncFiles` (sobrescribiendo `<tmpDir>/status.md`), pero **omite `validateChunks`** (`ring.go:393-426`, solo invocado por `PlanTask` en `bridge.go:1227`).
   - `Scope.Allows` (`ring.go:101-124`) evalúa prefijos puros (`under`) y nunca coincide con globs de extensión (`*.go`), además evalúa `Forbidden` y `ReadOnly` **antes** que `CanModify` (`ring.go:116-132`). En los chunks, `classifyChunkField` (`bridge.go:523-528`) mapea `Puede modificar` a `Chunk.Scope.CanModify`, mientras ignora `Salidas` (`"other"`).
   - `ShellRunner` en Windows (`runner_windows.go:13-25`) ejecuta `cmd.exe /S /C` con timeout fijo de `5 * time.Minute` (`bridge.go:1178`) y considera fallo todo `ExitCode != 0` (`ring.go:678-680`). Una denylist de banderas es insuficiente frente a `-ldflags=-extld=...`, `-toolexec`, `-test.cpuprofile`, `go build` escribiendo en `cwd`, `git diff --output` o escapes `cmd.exe`.
   - `parseChunkFile` (`bridge.go:458-474`) requiere encabezados `##` H2 (`inCoreH2 = true`) para proteger viñetas `- ` con `:`, pero líneas de prosa sin viñeta que contengan `Clave: valor` pueden reasignar el campo activo; además `parseChunksDir` (`bridge.go:331-360`) carga todos los `*.md` de `chunks/` ordenados lexicográficamente (`sort.Strings`, máximo 99 chunks de 2 dígitos).
4. **Riesgo de evasión de carril (`FTD` → `ASDD`), expansión silenciosa de riesgo y desfase en conteo de tests:**
   Un `WorkItem` puede entrar en `FTD` con 1 archivo en `affected_paths` y requerir archivos adicionales que crucen otro módulo raíz, superen el umbral de 5 archivos (`1_Intake/engine/model.go:20`, `triage.go:212`) o toquen `RZ-001` (`CRITICO`). Sin re-consulta obligatoria a `0_Brain`, recálculo compartido en `pkg/bfcore`, bloqueo explícito de descenso `ASDD → FTD` y vinculación de `decisions[]` al alcance aprobado (`approved_paths`/`scope_sha256`), el agente podría ampliar o degradar alcance sin escalado. Al mismo tiempo, contar cada `*_test.go` `1:1` degrada correcciones pequeñas en Go a `ASDD`, mientras eximir tests sin tope abriría una vía de evasión.
5. **Contexto obsoleto en archivos añadidos/nuevos y bypass hacia `compile`:**
   `prepare` solo conoce las rutas del `WorkItem`; cualquier archivo nuevo (`state: "absent"`) o existente (`state: "present"`) añadido en `can_modify` o citado en `G5` durante la redacción quedaría sin control de deriva salvo que `spec.json` registre `observed_files[]` (normalizado CRLF/BOM). Asimismo, sin persistir `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json`, un agente podría editar `spec.json` o `goldfish_review.json` e invocar `compile` saltándose `validate`.
6. **Falta de trazabilidad `trace_id`, aislamiento `Opción i` y vigilancia de deriva sobre `RZ-001`:**
   `.gitignore` aún no excluye `2_Spec_Writer/runtime/` ni `2_Spec_Writer/specwriter.exe`, no existe registro JSONL con `trace_id` para Equipo 2, y no hay comprobación que detecte cambios en `GengiSWAP/engine/{bridge,ring}.go`.

**Solución Propuesta:**
Construir `2_Spec_Writer/engine` (`module blackfactory/specwriter`, `CGO_ENABLED=0`, cero SQLite, cero llamadas LLM) compartiendo `pkg/bfcore` con `1_Intake` (evaluación FTD **Opción 5B**, higiene de rutas parametrizada y cliente read-only de `0_Brain` con multi-wrap de errores). El binario expone cinco subcomandos (`prepare`, `validate`, `render`, `compile`, `verify`) sobre una única fuente de verdad (`spec.json`, `schema_version: "1.0"`, `additionalProperties: false`) con rutas de archivo concretas en `can_modify` (v1), allowlist cerrado de comandos y banderas por subcomando, serialización determinista para Windows `cmd.exe`, validador compartido entre `validate` y `compile`, frontera de dos zonas `G4` para inyectar `--repo-root` únicamente en el `index.md` externo, promoción atómica *create-only* en NTFS bajo `.compile.lock`, test de contrato obligatorio (`GENGIS_CONTRACT_STRICT=1`) contra `gengis.exe plan`, y aislamiento en `.gitignore` (`Opción i`).

## Goals

*   **SG1 — Núcleo Pure-Go sin LLM, sin SQLite y Reutilización en `pkg/bfcore`:** Compilar `2_Spec_Writer/specwriter.exe` (`CGO_ENABLED=0`, integrado en `go.work`, solo `golang.org/x/sys` vía `pkg/bfcore`, plantillas `templates/*.md.tmpl` embebidas con `go:embed`) pasando `go vet ./...` y `go test -count=1 -cover ./...` con cobertura `>= 85%` sobre `t.TempDir()` (`G3`). Centralizar en `pkg/bfcore` la evaluación FTD bajo **Opción 5B** (`DefaultMaxFTDFiles = 5` efectivos, `*_test.go` en el mismo directorio que un archivo productivo modificado exento hasta tope duro de `5` tests exentos / `10` archivos físicos totales, `<= 1` módulo raíz sin excepciones, resto de lenguajes `1:1`), higiene de rutas parametrizada y cliente read-only de `0_Brain`.
*   **SG2 — Fuente Única de Verdad (`spec.json`) y Proyección Determinista (`render`):** Fijar `spec.json` (`schema_version: "1.0"`, `additionalProperties: false`, sin `extensions`) como único artefacto editado por el agente, con rutas de archivo concretas en `can_modify` y `observed_files[]` (`present` | `absent`), `decisions[]` vinculado a `approved_paths[]` + `scope_sha256`, `spec_sha256` sobre serialización JSON canónica, errores con JSON Pointer (RFC 6901), y vista Markdown determinista en `stdout` vía `specwriter render` (tras escaneo `G4`, cero archivos creados) sellada con `spec_sha256`.
*   **SG3 — Compuerta Estática Fail-Closed (`validate`) con Precedencia Fija, Presupuestos por Etapa y Allowlist Cerrado de Comandos/Banderas:** Nunca ejecutar comandos de la spec dentro de `validate`. Aplicar precedencia fija **`2` → `5` (presupuesto independiente `draft`: máx. 3, `final`: máx. 3; `external_cycles` acumulativo máx. 3; `last_exit`/`last_reason` sin transcribir secretos; fail-closed `exit 6` si `attempts.json` falta o está corrupto) → `3` (`G4` fail-closed en las 11 reglas doble fase raw+decodificado; JSON sintácticamente inválido tras Fase-A devuelve `exit 1` `INVALID_JSON` sin entrar a `[4]`) → `6` (preflight compartido `CheckReparseSafety` + `CheckBrainPreflight`) → `4` (decisión externa en orden `STALE_CONTEXT → ESCALATION_REQUIRED → LANE_UPGRADE_REQUIRED`) → `1` (validador compartido: esquema estricto, no descenso `ASDD → FTD`, `G2`, `G5`, `risk_mitigations[]`, allowlist cerrado de ejecutables/subcomandos/banderas/`env`, paridad `GengiSWAP` en memoria con `## Puede modificar` y protección `parseChunkFile`, y `goldfish_review.json` en `ASDD --stage final` como higiene de consistencia, no prueba de ejecución)**. Al pasar (`exit 0`), persistir `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256)` en `attempts.json`.
*   **SG4 — Compilador Determinista (`compile`) con Anti-Bypass, Frontera `G4` de Dos Zonas, Promoción Atómica *Create-Only* y Test de Contrato CI:** Ejecutar precedencia **`2` → `5` (rechaza si `draft_attempts >= 3 || final_attempts >= 3`, o si `external_cycles >= 3` sin pase `validate` posterior) → `3` (`G4` sobre `spec.json` y `goldfish_review.json` en `ASDD`) → `6` (preflight compartido, `--repo-root` validado, `.compile.lock`, `LIVE_TASK_STATE` si existe `.ring/state.json` o `TASK_DIR_EXISTS` si `<task-root>/<task-id>` ya existe, ambos `retryable: false`) → `4` (revalidación de contexto/alcance sin mutar `attempts.json`, devolviendo `exit 5` solo si ocurre un 4.º ciclo externo) → `1` (`VALIDATION_REQUIRED` `retryable: true` si `(spec_sha256, stage, goldfish_sha256)` difieren de `attempts.json`; invocación del validador compartido + `ScanG4All` en memoria previo al enlace `<TARGET_REPO>` → `VALIDATOR_BYPASS_DEFECT` `retryable: false` si fallara tras pase válido del mismo hash)**. Incluir test de contrato contra `gengis.exe plan` obligatorio con `GENGIS_CONTRACT_STRICT=1` antes de merge.
*   **SG5 — Freno Operativo contra Bucles, Desbloqueo Auditado, Una Sola Parada `[AUTOR]`, `verify` Solo Lectura y `Opción i`:** Inicializar `attempts.json` en `prepare`; desbloqueo tras `exit 5` únicamente mediante `specwriter prepare --reset-attempts --reason "<motivo>"` (escaneado con `G4`, registrando `ATTEMPTS_RESET`), documentando `attempts.json` explícitamente como freno operativo contra bucles y no control de seguridad. Mantener `gengis.exe plan` como única parada `[AUTOR]` en v1 (inyectando etiquetas compactas de riesgo/escalado en `Objetivo` y 1.ª línea de `Chunk.Spec`), excluir `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` en `.gitignore`, y exponer `specwriter verify [--json]` 100% solo lectura devolviendo `exit 6` (`retryable: false`) ante `GITIGNORE_DRIFT` o `GENGIS_ENGINE_DRIFT`.
*   **SG6 — Observabilidad de Extremo a Extremo y Presupuesto de Latencia:** Propagar `WorkItem.trace_id` hacia `spec_context.json`, `specwriter_events.jsonl` (sanitizado con `RedactAllG4` + verificado con `ScanG4All` antes de escribir) e `index.md`, completando `prepare`, `validate`, `render`, `compile` y `verify` (hasta 50 artefactos en `runtime/`) en `< 500 ms` p95 en Windows con un máximo de 2 invocaciones read-only a `0_Brain/brain.exe`.

## Non-Goals

*   **NG1 — Llamadas a APIs de LLM o Ejecución de Comandos de la Spec dentro de Go:** `specwriter.exe` nunca realiza llamadas de red ni gestiona claves de API, y `validate` nunca ejecuta comandos escritos en `spec.json`.
*   **NG2 — `specwriter approve` y Firma Criptográfica Ed25519 en v1 (Diferido a v2):** Diferido a v2 *(disparador: ejecución desatendida o más de un operador)*. En v1, `gengis.exe plan` es la única parada `[AUTOR]`.
*   **NG3 — `specwriter publish` y `publish_allowlist` en v1 (Diferido a v2):** En v1 todo permanece privado en `2_Spec_Writer/runtime/` (`Opción i`). Diferido a v2 *(disparador: segunda necesidad real de publicación controlada)*.
*   **NG4 — DSL Declarativo Externo de Plantillas, Campo Opaco `extensions` y Prefijos de Directorio en `can_modify` (Diferidos a v2):** Plantillas embebidas con `go:embed`; evolución del esquema solo vía `schema_version`; `work_units[].can_modify` admite únicamente rutas de archivo concretas en v1 (prefijos de directorio en `can_modify` diferidos a v2 hasta compartir un único parser `ParseRiskZonesCatalog` con `0_Brain`).
*   **NG5 — Wrapper de `BrainPatch` / `brain.exe propose` y Soporte de Stacks No-Go en `verify_steps` en v1:** `1_Intake` ya enruta `NO_BRAIN_COVERAGE` a `ASDD` (`1_Intake/engine/triage.go:222-235`). `spec.json` declara solo `brain_followups[]` post-merge; wrapper diferido a v2 *(disparador: resolución del WorkItem independiente en `0_Brain` por falta de `.brain.lock` y `ScanG4` en `0_Brain/engine/vault.go:244-418`)*. El allowlist de `verify_steps` en v1 cubre exclusivamente el monorepo Black Factory y repositorios Go (`go test`/`vet`, `git` y herramientas read-only de Black Factory).
*   **NG6 — Modificaciones a `0_Brain/**`, `GengiSWAP/engine/` (`RZ-001`) o `1_Intake/schemas/workitem.schema.json` v1.0:** `0_Brain/**`, `GengiSWAP/engine/` y `workitem.schema.json` v1.0 permanecen intactos en v1, y `1_Intake/runtime/workitems/WI-*.json` queda inmutable (`Q4`).

## Open Questions

Ninguna abierta para v1. Las decisiones `Q1–Q4` fueron ratificadas por `[AUTOR]` (`blackpusch`) e incorporadas en el Plan Técnico:
*   **Q1 (Resuelta):** Presupuestos independientes por etapa (`draft`: máx. 3, `final`: máx. 3; pasar `draft` no resetea `final` si se re-ejecuta `draft`), `external_cycles` acumulativo (máx. 3), desbloqueo con `specwriter prepare --reset-attempts --reason "<motivo>"` (escaneado con `G4` + evento `ATTEMPTS_RESET`).
*   **Q2 (Resuelta):** `LIVE_TASK_STATE` (y `TASK_DIR_EXISTS` *create-only*) permanecen bajo `exit 6` con `retryable: false` y catálogo explícito de `reason` (contrato `0..6`).
*   **Q3 (Resuelta):** Opción `5B` en `pkg/bfcore` (`*_test.go` en mismo directorio que un archivo productivo modificado exento hasta tope duro de `5` tests exentos / `10` archivos físicos totales; `<= 1` módulo raíz sin excepciones; otros lenguajes `1:1`).
*   **Q4 (Resuelta):** `decisions[]` local en `spec.json` acotado a `approved_paths[]` + `scope_sha256`, manteniendo `1_Intake/runtime/workitems/WI-*.json` inmutable.

*(Seguimiento fuera de alcance de v1: WorkItem independiente en `0_Brain` para añadir `.brain.lock` y `ScanG4` en `0_Brain/engine/vault.go:244-418`.)*

## Critical User Journeys (CUJs)

### CUJ 1: Flujo Completo `FTD` Ejercitando Opción 5B (5 Productivos + 2 Tests Hermanos) hasta `compile`

*   **Given**: `0_Brain/vault/` está indexado y sin `.brain-journal.json` pendiente, `1_Intake/runtime/workitems/WI-a1b2c3d4e5f6.json` está en `status: "ready"`, `project: "black-factory"`, `lane: "FTD"`, `risk_level: "ALTO"` (`RZ-002` sobre `0_Brain/engine/`), `affected_paths` contiene `5` archivos productivos dentro de `0_Brain/engine/` (`validator.go`, `index.go`, `query.go`, `drift.go`, `frontmatter.go`), y `2_Spec_Writer/runtime/` está ignorado en `.gitignore`.
*   **When**:
    1. El agente ejecuta `specwriter.exe prepare --workitem "1_Intake/runtime/workitems/WI-a1b2c3d4e5f6.json" --repo-root "C:\Black Factory" --json`.
    2. El agente completa `2_Spec_Writer/runtime/black-factory/WI-a1b2c3d4e5f6/spec.json` con `1` `work_unit` modificando esos `5` archivos productivos más `2` tests del mismo directorio (`validator_test.go`, `index_test.go` — `7` archivos físicos `<= 10`, `2` tests exentos `<= 5`), registra los `7` archivos en `observed_files[]` (`state: "present"` + SHA-256 normalizado CRLF/BOM) y los `2` archivos añadidos en `scope_expansions[]`, enlaza `RZ-002` en `risk_mitigations[]` a un `verify_step` permitido (`cwd: "0_Brain/engine"`, `argv: ["go", "test", "-count=1", "./..."]`, `expect_exit: 0`, `timeout_sec: 120`) + `review_gate` (`acceptance_criteria[].id`), declara `failure_proof` (`expect_exit: 1`) y citas `G5`.
    3. Ejecuta `specwriter.exe validate --spec "2_Spec_Writer/runtime/black-factory/WI-a1b2c3d4e5f6/spec.json" --repo-root "C:\Black Factory" --stage draft --json`, luego `specwriter.exe render --spec "2_Spec_Writer/runtime/black-factory/WI-a1b2c3d4e5f6/spec.json"`, y finalmente `specwriter.exe compile --spec "2_Spec_Writer/runtime/black-factory/WI-a1b2c3d4e5f6/spec.json" --repo-root "C:\Black Factory" --task-root "C:\Desarrollo\GengiSWAP_data\gengis_task" --task-id "bf_fix_brain_104" --json`.
*   **Then**:
    1. `prepare` valida `G4` en el `WorkItem`, ejecuta el preflight compartido (`CheckReparseSafety` + `CheckBrainPreflight`), congela `spec_context.json` e inicializa `attempts.json` en `0` (`exit 0`).
    2. `validate --stage draft` verifica `2 → 5 → 3 → 6 → 4 → 1`: `0` hallazgos `G4`, hashes normalizados coinciden con disco, `bfcore.EvaluateFTDBounds` calcula `5` efectivos (`<= 5`), `2` tests exentos (`<= 5`), `7` físicos (`<= 10`), `1` módulo raíz (`<= 1`) → mantiene `FTD`, verifica renderizado `GengiSWAP` en memoria (`## Puede modificar`), persiste `(last_passed_sha256, "draft", "")` en `attempts.json` y sale con `exit 0`.
    3. `render` valida `G4` y emite en `stdout` el Markdown determinista con `spec_sha256` en cabecera sin crear archivos (`exit 0`).
    4. `compile` verifica `attempts.json` (no bloqueado y `spec_sha256 == last_passed_sha256` con etapa `"draft"` válida para `FTD`), valida `--repo-root` contra `repo_root_sha256`, sanea `--task-id`, verifica `G4` sobre el buffer con `<TARGET_REPO>`, adquiere `<task-root>/.compile.lock`, confirma que `<task-root>/bf_fix_brain_104` no existe, sustituye `<TARGET_REPO>` únicamente en el `index.md` externo, crea el árbol en staging hermano y ejecuta un único `os.Rename` atómico en NTFS, imprime el resumen previo a `gengis.exe plan` y sale con `exit 0`.

### CUJ 2: Escalado de Carril (`ESCALATION_REQUIRED`), `decisions[]` Acotado y `--stage final` Goldfish (`ASDD`)

*   **Given**: Un `WorkItem` llega en `lane: "FTD"` sobre `1_Intake/engine/triage.go`. Durante la redacción, el agente necesita editar también `pkg/bfcore/paths.go` (segundo módulo raíz).
*   **When**:
    1. El agente ejecuta `specwriter.exe validate --spec "..." --repo-root "C:\Black Factory" --stage draft --json` con `lane: "FTD"`.
    2. `[AUTOR]` aprueba la escalación registrando una entrada en `spec.json.decisions[]` vinculada al `scope_sha256` de `["1_Intake/engine/triage.go", "pkg/bfcore/paths.go"]` y cambia `lane` a `"ASDD"`. El agente completa las secciones `ASDD`, pasa `--stage draft` (`exit 0`), el harness ejecuta los lentes Goldfish y escribe `goldfish_review.json` ligado a `spec_sha256`, y el agente ejecuta `validate --spec "..." --repo-root "C:\Black Factory" --stage final --json`.
    3. Si el agente añadiera posteriormente un tercer archivo en `can_modify` fuera de `approved_paths`, ejecuta `validate --spec "..." --repo-root "C:\Black Factory" --stage final --json`.
*   **Then**:
    1. La primera corrida devuelve `exit 4` (`ESCALATION_REQUIRED` por precedencia sobre `LANE_UPGRADE_REQUIRED`, `retryable: false`, `external_cycles: 1/3`, sin tocar `draft_attempts`).
    2. Al pasar `--stage draft` (`exit 0`), `final_attempts` arranca limpio (`0/3`). Al pasar `--stage final` (`exit 0`), `attempts.json` guarda `(last_passed_sha256, "final", last_passed_goldfish_sha256)`, habilitando `compile`.
    3. Si el agente añade un archivo fuera de `decisions[].approved_paths` / `scope_sha256`, `validate` vuelve a disparar `exit 4` (`ESCALATION_REQUIRED`). Si intentara ejecutar `compile` sin revalidar el nuevo `spec_sha256` (o tras editar/borrar `goldfish_review.json`), `compile` devuelve `exit 1` (`VALIDATION_REQUIRED`, `retryable: true`).

### CUJ 3: Freno Operativo contra Bucles (`G4_VIOLATION` → `exit 5`), Desbloqueo Auditado y Protección TOCTOU (`LIVE_TASK_STATE` / `TASK_DIR_EXISTS` → `exit 6`)

*   **Given**: (a) Un agente intenta validar cuatro veces seguidas un `spec.json` con ruta personal Windows (`C:\Users\dev\...`); y (b) `C:\Desarrollo\GengiSWAP_data\gengis_task\bf_active_task\.ring\state.json` existe en ejecución por `3_Builder`.
*   **When**:
    1. El agente invoca `validate --spec "..." --repo-root "C:\Black Factory" --stage draft --json` 4 veces sin eliminar la ruta personal, y luego `[AUTOR]` desbloquea con `specwriter.exe prepare --workitem "..." --repo-root "C:\Black Factory" --reset-attempts --reason "Ruta personal eliminada tras revision" --json`.
    2. Un operador invoca `specwriter.exe compile --spec "..." --repo-root "C:\Black Factory" --task-root "C:\Desarrollo\GengiSWAP_data\gengis_task" --task-id "bf_active_task" --json`.
*   **Then**:
    1. Los intentos 1, 2 y 3 fallan en el paso `3 (G4)` devolviendo `exit 3` (`reason: "G4_VIOLATION"`, `retryable: false`, sin transcribir la cadena disparadora) e incrementando `draft_attempts` al terminar con `last_exit: 3, last_reason: "G4_VIOLATION"`. El 4.º intento corta en el paso `5` devolviendo `exit 5` (`MAX_VALIDATE_ATTEMPTS`, `last_exit: 3`, `last_reason: "G4_VIOLATION"`). El comando `--reset-attempts` valida `--reason` con `G4`, reinicia contadores en `attempts.json`, registra `ATTEMPTS_RESET` en `specwriter_events.jsonl` y devuelve `exit 0`.
    2. `compile` adquiere `<task-root>/.compile.lock`, detecta `.ring/state.json`, no toca ningún archivo y sale con `exit 6` (`LIVE_TASK_STATE`, `retryable: false`). Si la carpeta existiera sin `.ring/state.json`, devuelve `exit 6` (`TASK_DIR_EXISTS`, `retryable: false`).

### CUJ 4: `specwriter verify` Solo Lectura, Deriva `GITIGNORE_DRIFT` / `GENGIS_ENGINE_DRIFT` y Archivo Nuevo Inesperado (`absent`)

*   **Given**: `2_Spec_Writer/runtime/` contiene specs previas, un archivo declarado en `observed_files[]` con `state: "absent"` aparece inesperadamente en disco antes de `validate`, o se modifica `GengiSWAP/engine/bridge.go` / `.gitignore`.
*   **When**:
    1. Se ejecuta `specwriter.exe verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\2_Spec_Writer\runtime" --json`.
    2. Se ejecuta `specwriter.exe validate --spec "..." --repo-root "C:\Black Factory" --stage draft --json` tras aparecer el archivo marcado `"absent"`.
*   **Then**:
    1. `verify` lee sin cerrojos exclusivos ni creación de archivos: si falta `2_Spec_Writer/runtime/` o `2_Spec_Writer/specwriter.exe` en `.gitignore` (comprobados individualmente) devuelve `exit 6` (`GITIGNORE_DRIFT`, `retryable: false`); si cambió el hash de `GengiSWAP/engine/{bridge,ring}.go` devuelve `exit 6` (`GENGIS_ENGINE_DRIFT`, `retryable: false`); si halla un secreto en `runtime/` devuelve `exit 3` (`G4_VIOLATION_ON_DISK`); si todas las comprobaciones pasan devuelve `exit 0`.
    2. `validate` detecta que el archivo `"absent"` ahora existe en disco y devuelve `exit 4` (`STALE_CONTEXT`, `retryable: false`), indicando refrescar contexto con `prepare --refresh-context` y actualizar `observed_files[]` en `spec.json` antes de revalidar.

--------------------------------------------------------------------------------

## Technical Plan (Architecture)

### 1. Topología del Sistema y Límites de Módulos (`go.work` + `pkg/bfcore` + `2_Spec_Writer`)

`2_Spec_Writer` sigue la arquitectura híbrida `Opción 3C` + `Opción i` de `0_Brain` y `1_Intake`, añadiendo `./2_Spec_Writer/engine` al archivo raíz [`go.work`](../../go.work) y extrayendo en **Fase 0** las primitivas transversales hacia `pkg/bfcore` (sin SQLite, `CGO_ENABLED=0`, única dependencia externa `golang.org/x/sys v0.49.0`):

```text
Black Factory\
├── go.work                                (Añade ./2_Spec_Writer/engine junto a 0_Brain, 1_Intake, pkg/bfcore)
├── .gitignore                             (Añade 2_Spec_Writer/runtime/ y 2_Spec_Writer/specwriter.exe)
├── pkg\bfcore\                            (Módulo compartido blackfactory/pkg/bfcore — Fase 0)
│   ├── g4.go                              (Existente: 11 reglas G4, ScanG4All, ScanTier1, RedactTier2Text, RedactAllG4, MatchesPersonalPathG4)
│   ├── paths.go                           (Ampliado: NormalizeCoverPath/MatchCoverPathFold existentes + SanitizeRelativePaths + CheckNoReparseUpToNearestExisting + ComputeNormalizedFileSHA256)
│   ├── ftd.go                             (NUEVO: Opción 5B EvaluateFTDBounds + CountRootModules + constantes FTD)
│   ├── brain_client.go                    (NUEVO: BrainClient read-only + ExecBrain + FindUncoveredPaths + RiskLevelRank + ErrBrainJournalActive multi-wrap)
│   ├── lock_windows.go / lock_other.go    (Existente: cerrojo exclusivo SO con backoff)
│   └── reparse_windows.go / reparse_other.go (Existente: CheckNoReparseChain vía os.Lstat)
├── 1_Intake\engine\                       (Fase 0: delega FTD/CountRootModules/Sanitize/BrainClient a pkg/bfcore)
└── 2_Spec_Writer\                         (Módulo blackfactory/specwriter — Fase 1)
    ├── README.md                          (Documentación de límites, invariantes, comandos y códigos 0..6)
    ├── schemas\
    │   └── spec.schema.json               (JSON Schema Draft 2020-12 v1.0, additionalProperties: false; contrato único)
    ├── skills\
    │   ├── README.md                      (Catálogo de skills de Equipo 2)
    │   └── spec-authoring\
    │       └── SKILL.md                   (Guía progresiva del agente para el bucle prepare -> validate -> compile)
    ├── engine\
    │   ├── go.mod / go.sum / README.md
    │   ├── embed.go                       (//go:embed exclusivo de templates/*.md.tmpl)
    │   ├── templates\                     (index.md.tmpl, chunk.md.tmpl, status.md.tmpl)
    │   ├── model.go                       (Structs Spec, SpecContext, AttemptsState, GoldfishReview, VerifyReport, CLIResult, SHA-256 canónico)
    │   ├── state.go                       (Persistencia atómica de attempts.json bajo .wi.lock, telemetría JSONL RedactAllG4+ScanG4All bajo .events.lock)
    │   ├── prepare.go                     (Flujo prepare + --refresh-context + --reset-attempts --reason)
    │   ├── validate.go                    (Pipeline 2->5->3->6->4->1 + ValidateSpecAndRenderCheck compartido con compile)
    │   ├── render.go                      (Proyección Markdown ASCII determinista hacia stdout sellada con spec_sha256)
    │   ├── compile.go                     (Pipeline 2->5->3->6->4->1, anti-bypass, frontera G4 2 zonas, create-only NTFS rename bajo .compile.lock)
    │   ├── verify.go                      (Preflight compartido CheckReparseSafety+CheckBrainPreflight y checks exclusivos verify: gitignore + RZ-001)
    │   ├── specwriter_test.go             (Tests unitarios TDD + paridad spec.schema.json + Scope.Allows + benchmark < 500 ms p95)
    │   ├── contract_gengis_test.go        (Test de contrato contra gengis.exe plan en t.TempDir(); GENGIS_CONTRACT_STRICT=1)
    │   └── cmd\specwriter\
    │       ├── README.md
    │       ├── main.go                    (CLI specwriter.exe: prepare, validate, render, compile, verify; exits 0..6)
    │       └── main_test.go               (Tests E2E de todos los CUJs sobre t.TempDir())
    ├── specwriter.exe                     (Binario compilado local; ignorado en .gitignore)
    └── runtime\                           (Estado operativo Opción i; ignorado en .gitignore)
        ├── .events.lock                   (Cerrojo breve exclusivamente para appends en specwriter_events.jsonl)
        ├── specwriter_events.jsonl        (Bitácora append-only con trace_id, duration_ms, reason y retryable)
        └── <project>\
            └── <WI-hash12>\
                ├── .wi.lock               (Cerrojo granular por WorkItem; permite paralelismo por proyecto)
                ├── spec_context.json      (Snapshot congelado de prepare)
                ├── attempts.json          (Contadores draft/final/external + last_passed_sha256/stage/goldfish)
                ├── spec.json              (Única fuente de verdad editada por el agente)
                └── goldfish_review.json   (Evidencia requerida en ASDD --stage final; higiene de consistencia)
```

### 2. Contratos Compartidos en `pkg/bfcore` (Fase 0)

Para eliminar duplicación y deriva entre `1_Intake` y `2_Spec_Writer` sin añadir dependencias externas ni modificar `0_Brain/**`:

1. **Evaluación FTD única (`pkg/bfcore/ftd.go`, Opción 5B):**
   - Constantes canónicas: `DefaultMaxFTDFiles = 5`, `MaxFTDExemptGoTests = 5`, `MaxFTDPhysicalFiles = 10`, `MaxFTDRootModules = 1`.
   - `CountRootModules(paths []string) []string`: extraído de `1_Intake/engine/paths.go:137-160` (preserva el mapeo de archivos raíz sin `/` a `"."` y deduplicación case-insensitive).
   - `EvaluateFTDBounds(paths []string, maxEffective int) FTDEvaluation`:
     - Si `maxEffective <= 0`, aplica `DefaultMaxFTDFiles (5)`. Si `len(paths) == 0`, retorna `WithinBounds: false` (todo cambio válido requiere al menos 1 archivo).
     - Normaliza y deduplica rutas de forma insensible a mayúsculas.
     - Clasifica como **test Go candidato** exclusivamente archivos cuyo nombre base termina en `_test.go` (sin distinguir mayúsculas). Todo otro archivo (`.py`, `.js`, `.ts` y demás archivos no `*_test.go`) cuenta `1:1`.
     - Recopila el conjunto de directorios padre que contienen al menos un archivo **productivo (no `_test.go`)** en `paths`.
     - Un `*_test.go` queda exento únicamente si su directorio padre pertenece a ese conjunto **y** `exemptCount < MaxFTDExemptGoTests` (`5`). Todo `*_test.go` sin archivo productivo hermano en el mismo directorio, o por encima del 5.º test exento, suma `1:1` en `EffectiveCount`.
     - Calcula `RootModules := CountRootModules(paths)` sobre **todos** los archivos físicos (incluidos los tests exentos, sin excepciones).
     - Devuelve `FTDEvaluation{PhysicalCount, EffectiveCount, ExemptTestCount, RootModules, WithinBounds}` donde `WithinBounds` exige simultáneamente: `PhysicalCount > 0 && len(RootModules) <= 1 && EffectiveCount <= maxEffective && ExemptTestCount <= MaxFTDExemptGoTests && PhysicalCount <= MaxFTDPhysicalFiles`.
   - `1_Intake/engine/triage.go` reemplaza la verificación inline (`L211-220`) por `bfcore.EvaluateFTDBounds(wi.AffectedPaths, maxFTDFiles)` actualizando el detalle de triaje (`L218, L243`) para informar `EffectiveCount`/`PhysicalCount`.

2. **Higiene de rutas parametrizada y hashing normalizado (`pkg/bfcore/paths.go`):**
   - `SanitizeRelativePaths(rawPaths []string, repoRoot string, opts PathSanitizeOpts) (cleanPaths []string, rejectReason string)`: unifica las validaciones pre-`path.Clean` de `1_Intake/engine/paths.go:20-118` con `PathSanitizeOpts{AllowDir bool, SplitCommas bool, FoldCaseDedupe bool}`, rechazando siempre globs `*?[` y rutas personales `G4` de forma incondicional. Devuelve siempre `cleanPaths` no-nil (`[]string{}`) y `rejectReason == ""` en éxito, o los códigos exactos `"G4_personal_path_in_affected_paths"` / `"PATH_TRAVERSAL_OR_INVALID_SCOPE"` en rechazo. Rechaza siempre `"."`/`"./"`, y registra si la entrada terminaba en `/` o `\` antes de limpiar la ruta para preservar `/` final únicamente cuando `AllowDir == true`. Cuando `SplitCommas == true` (`1_Intake`) omite segmentos vacíos tras dividir por `,`; cuando `SplitCommas == false` (`2_Spec_Writer`) rechaza cualquier entrada vacía con `"PATH_TRAVERSAL_OR_INVALID_SCOPE"`. `1_Intake` invoca `{AllowDir: false, SplitCommas: true, FoldCaseDedupe: false}` (conservando bit a bit `sanitized_payload_sha256` y los tests existentes); `2_Spec_Writer` invoca `{AllowDir: false, SplitCommas: false, FoldCaseDedupe: true}` para `can_modify` / `observed_files` / `g5_citations` (archivos concretos en v1) y `{AllowDir: true, SplitCommas: false, FoldCaseDedupe: true}` para prefijos restrictivos `read_only_paths` / `global_forbidden` / `work_units[].forbidden`.
   - `CheckNoReparseUpToNearestExisting(rootPath, targetPath string) error`: sube desde `targetPath` hasta el ancestro existente más cercano dentro de `rootPath` (o desde la raíz del volumen cuando `rootPath == ""`, p. ej. `--task-root` externo al repositorio) y ejecuta `CheckNoReparseChain`.
   - `ComputeNormalizedFileSHA256(repoRoot, relPath string) (state string, sha256Hex string, err error)`: valida primero contención de archivo relativo único dentro de `repoRoot` mediante `SanitizeRelativePaths(..., PathSanitizeOpts{AllowDir: false, SplitCommas: false, FoldCaseDedupe: false})` antes de invocar `CheckNoReparseChain(repoRoot, absPath)` o `os.Lstat`; si devuelve `os.ErrNotExist`, retorna `("absent", "", nil)`; si es directorio o archivo irregular devuelve error; si es archivo regular lee en streaming (eliminando UTF-8 BOM `\xef\xbb\xbf` y normalizando `\r\n` / `\r` a `\n` hasta `MaxObservedFileBytes = 10 MiB`) y retorna `("present", hexSHA256, nil)`.

3. **Cliente read-only de `0_Brain` (`pkg/bfcore/brain_client.go`):**
   - Extrae el transporte read-only compartido (`BrainClient` con `{Preflight, Context}`, `SubprocessBrainClient` con `NewSubprocessBrainClient`, `Preflight`, `Context` y `ExecBrain(args []string) ([]byte, BrainInvocation, error)`), sus DTOs compartidos (`BrainPreflight`, `BrainContextResponse`, `BrainDocMatch`, `RiskHit`, `BrainInvocation`) desde `1_Intake/engine/brain_client.go` y `1_Intake/engine/model.go:84-122`, más `FindUncoveredPaths` (`1_Intake/engine/triage.go:254-289`) y `RiskLevelRank` (`1_Intake/engine/model.go:324-329`) para garantizar paridad exacta de cobertura y severidad sin duplicación. En `1_Intake/engine/brain_client.go`, `intake.BrainClient` embebe `bfcore.BrainClient` + `Query(...)` y `intake.SubprocessBrainClient` envuelve `*bfcore.SubprocessBrainClient` en un struct local para definir el método `Query(...)` (usando `c.ExecBrain`) junto a `ExtractQueryTokens`, `BuildDisjunctiveFTSQuery` e `IsProjectActiveOnDisk`.
   - Define sentinels independientes (`ErrBrainUnavailable`, `ErrBrainIndexMissing`, `ErrBrainJournalActive`) y retorna errores multi-wrap en `Preflight` de modo que `errors.Is` reconozca simultáneamente `ErrBrainUnavailable` y el sentinel específico (`ErrBrainJournalActive` o `ErrBrainIndexMissing`, sin confundirse entre sí). Así `1_Intake` sigue detectando `ErrBrainUnavailable` (`exit 4`) sin cambios, y `2_Spec_Writer` distingue exactamente `BRAIN_JOURNAL_ACTIVE` vs. `BRAIN_UNAVAILABLE` bajo `exit 6` (`retryable: true`). No duplica el parser de `wiki/risk_zones.md` (`0_Brain/engine/query.go:186-332` permanece como único parser).

### 3. Esquema Canónico `spec.json` (`schema_version: "1.0"`) y Mapa Campo por Campo hacia `GengiSWAP` (`bridge.go`)

`spec.json` aplica `"additionalProperties": false` en todos los niveles (sin `extensions`). La siguiente tabla fija tipo, obligatoriedad por carril y destino exacto en los archivos compilados para [`GengiSWAP/engine/bridge.go`](../../GengiSWAP/engine/bridge.go):

| Ruta JSON (`spec.json`) | Tipo | `FTD` | `ASDD` | Destino compilado en `GengiSWAP` / Uso determinista |
| :--- | :--- | :---: | :---: | :--- |
| `/schema_version` | `string` (`"1.0"`) | Req | Req | Fail-closed en `validate`/`compile`/`render`. |
| `/work_item_ref` | `object` (`id`, `revision`, `sanitized_payload_sha256`, `trace_id`, `source_workitem_path` relativo a `--factory-root`) | Req | Req | Validado campo a campo en `[1]` (`id`, `revision`, `sanitized_payload_sha256` contra `spec_context.work_item`; `trace_id` contra `spec_context.trace_id`; `source_workitem_path` contra `spec_context.source_workitem_path`, sin rutas absolutas de host); `trace_id` e `id` en `## Notas Post-Merge y Trazabilidad` de `index.md` y en `specwriter_events.jsonl`. |
| `/project` | `string` (slug) | Req | Req | Debe coincidir con `spec_context.json.project`; usado en `brain.exe context --project`. |
| `/lane` | `enum` (`"FTD"` \| `"ASDD"`) | Req | Req | Verificado por `validate` (solo ascenso `FTD → ASDD` con `decisions[]`; `ASDD → FTD` prohibido en `[1]`; `FTD` fuera de límites falla en `[4]` con `LANE_UPGRADE_REQUIRED`). |
| `/template` | `enum` (`modulo_nuevo` \| `correccion_hallazgos` \| `auditoria_e2e` \| `mapeo_estatico`) | Req | Req | Línea 2 de `index.md`: `Plantilla: <template> v1` (`GengiSWAP/templates/README.md:20`). |
| `/target_branch` | `string` (una línea, sin espacios, `\r`/`\n` ni backticks) | Req | Req | `## Directorio de Destino Real` (`- **Rama destino:** <target_branch>` sin backticks) junto a `- **Repositorio destino:** <absRepoRoot>` enlazado al final de `compile`. |
| `/title` | `string` (`1..160` chars, sin `\r`/`\n`) | Req | Req | H1 de `index.md`: `# Mapa de Tarea: <task_id> — <title>`. |
| `/g3_data_mutation` | `string` (non-empty, una línea sin `\r`/`\n`) | Req | Req | `## Alcance` → `- Mutación de datos (G3): <texto>` (`GengiSWAP/templates/modulo_nuevo/index.md:14` y `GengiSWAP/templates/{auditoria_e2e,correccion_hallazgos,mapeo_estatico}/index.md:12`). |
| `/requires_autor_hitl` | `bool` | Req | Req | Transición unidireccional `false → true` (`true` si `WorkItem.requires_autor_hitl`, `CRITICO` o `ESCALATION_REQUIRED`); inyecta etiquetas compactas (`[HITL]`/`[CRITICO]`/`[ESCALATED]` en `index.md` `**Objetivo:**`; `[HITL]`/`[CRITICO]`/`[ESCALATED]`/`[EXPANSIONS:N]` en 1.ª línea de `Chunk.Spec` para `gengis.exe plan`, `bridge.go:1508`). |
| `/stack_assumptions` | `string[]` (`>= 1`) | Req | Req | `## Supuestos y Stack Tecnológico` de `index.md` + `- **Máx. intentos por chunk:** 3` (`bridge.go:19`). |
| `/read_only_paths` | `string[]` (archivos o prefijos `/` relativos, sin globs) | Req | Req | `## Alcance` → `- Solo lectura:` (únicamente rutas entre backticks, sin `/` en prosa para evitar falso parseo en `cleanScopeItems`, `bridge.go:203-207, 630`). |
| `/global_forbidden` | `string[]` (archivos o prefijos `/` relativos, sin globs) | Req | Req | `## Alcance` → `- Prohibido:` (únicamente rutas entre backticks de `global_forbidden` en v1; `Scope.Allows` en `ring.go:116-124` ya deniega por defecto toda ruta fuera de `CanModify`; el cruce automático con zonas no intersectadas de `wiki/risk_zones.md` queda diferido a v2 junto a `ParseRiskZonesCatalog`). |
| `/dod` | `string[]` (`>= 1`) | Req | Req | `## Definición de Terminado (DoD)` en `index.md`. |
| `/narrative/problem` | `string` (non-empty) | Req | Req | `**Objetivo:**` en `index.md` (prefijado con `[HITL]`/`[CRITICO]`/`[ESCALATED]` si aplica) y `## Problema` en `render`. |
| `/narrative/root_cause` | `string` (non-empty) | Req | Req | Sección `## Causa Raíz` en `render` (referenciando `g5_citations[]`). |
| `/narrative/proposed_change` | `string` (non-empty) | Req | Req | Sección `## Cambio Propuesto` en `render`. |
| `/narrative/alternatives_considered` | `string[]` (`>= 1` en `ASDD`) | Opc (`[]`) | Req | Sección `## Alternativas Consideradas` en `render` (`ASDD`). |
| `/narrative/interfaces_and_contracts` | `string` (non-empty en `ASDD`) | Opc (`""`) | Req | Sección `## Interfaces y Contratos` en `render` (`ASDD`). |
| `/narrative/edge_cases` | `string[]` (`>= 1` en `ASDD`) | Opc (`[]`) | Req | Sección `## Casos Borde` en `render` (`ASDD`). |
| `/narrative/rollback_plan` | `string` (non-empty en `ASDD`) | Opc (`""`) | Req | Sección `## Plan de Rollback` en `render` (`ASDD`). |
| `/observed_files[]` | `{path, state, sha256}` (`state`: `"present"` o `"absent"`) | Req | Req | Cubre exactamente la unión de rutas de archivo concretas `affected_paths ∪ g5_citations[].path ∪ work_units[].can_modify`. Completitud validada en `[1]` (`exit 1`); deriva contra disco (hash CRLF/BOM cambiado o aparición inesperada de `"absent"`) detectada en `[4]` (`STALE_CONTEXT`). |
| `/g5_citations[]` | `{path, line, symbol_anchor, claim}` | Req (`>= 1`) | Req (`>= 1`) | Validado en disco (1-based CRLF/BOM + ancla literal `±5` líneas en archivos de texto UTF-8 y código, rechazando tokens triviales). Proyectado en `render`. |
| `/decisions[]` | `{reason, decided_by, rationale, timestamp, lane_from, lane_to, approved_paths[], scope_sha256}` | Opc (`[]`) | Req si hubo escalado o ascenso `FTD → ASDD` | Vincula resolución `[AUTOR]` al `scope_sha256` exacto (`ComputeScopeSHA256(approved_paths) == scope_sha256 == current_scope_sha256`, exigiendo igualdad exacta de conjuntos normalizados entre `approved_paths` y la unión de `work_units[].can_modify`); etiqueta `[ESCALATED]` en `gengis.exe plan`. |
| `/risk_mitigations[]` | `{risk_id, level, verify_step_ids[], review_gates[], requires_autor_hitl}` | Req si `len(risk_hits)>0` | Req si `len(risk_hits)>0` | Cubre 100% de `risk_hits` (originales + re-consultados). `verify_step_ids[]` referencia `work_units[].verify_steps[].id` (únicos globalmente en `spec.json`); `review_gates[]` (`string[]`) referencia `work_units[].acceptance_criteria[].id` con `kind == "review_gate"`. `CRITICO` exige `>= 1` `review_gate` y `requires_autor_hitl: true`. |
| `/brain_followups[]` | `{target_hint, summary, trigger}` | Opc (`[]`) | Opc (`[]`) | Proyectado en `render` y en notas post-merge de `index.md` (sin invocar `brain.exe propose` en v1). |
| `/work_units[]` | `object[]` (`1..2` en `FTD`; `1..99` en `ASDD`) | Req | Req | Compilado 1-a-1 a `chunks/01_<id>.md`..`NN_<id>.md` (orden topológico determinista, máx. 99 por orden lexicográfico en `bridge.go:347`) y `## Contratos entre Pasos` en `index.md`. |
| `/work_units[]/id` | `string` (`^[a-z0-9][a-z0-9_]{1,47}$`, único) | Req | Req | Sufijo de archivo `NN_<id>.md` (`Chunk.ID = NN_<id>` en `bridge.go:369-370`, `ring.go:403-406`). |
| `/work_units[]/title` | `string` (`1..120` chars, sin `\r`/`\n`) | Req | Req | H1 `# Chunk NN: <title>` y viñeta inicial de `status.md`. |
| `/work_units[]/objective` | `string` (non-empty) | Req | Req | `## Objetivo` → `Chunk.Spec` (`bridge.go:501`, `ring.go:407`; 1.ª línea prefijada con etiquetas `[HITL]`/`[CRITICO]`/`[ESCALATED]`/`[EXPANSIONS:N]` si aplica). |
| `/work_units[]/inputs` | `string[]` (cada ítem de una línea sin `\r`/`\n`) | Req | Req | `## Entradas` en `chunks/NN_<id>.md`. |
| `/work_units[]/can_modify` | `string[]` (`>= 1`; **rutas de archivo concretas** en `FTD` y `ASDD` en v1, sin prefijos `/` ni globs) | Req | Req | **`## Puede modificar`** en `chunks/NN_<id>.md` (puebla `Chunk.Scope.CanModify` en `bridge.go:418-419, 523`) + unión en `index.md` `Puede modificar`. Disjunto entre unidades sin relación transitiva `depends_on`, y estrictamente disjunto de `read_only_paths`, `global_forbidden` y `forbidden`. |
| `/work_units[]/scope_expansions` | `{path, rationale}[]` | Req si añade rutas fuera de `affected_paths` | Req si añade rutas fuera de `affected_paths` | Obligatorio para cada ruta en `can_modify \ WorkItem.affected_paths`; etiqueta `[EXPANSIONS:N]` en `gengis.exe plan`. |
| `/work_units[]/forbidden` | `string[]` (archivos o prefijos `/` sin globs) | Req | Req | `## Fuera de Alcance` (`bridge.go:417`) — solo rutas en backticks, unido con `can_modify` de otros chunks **excluyendo** rutas iguales o solapadas con el `can_modify` del chunk actual (evitando bloqueo de `depends_on` en `ring.go:120`). |
| `/work_units[]/depends_on` | `string[]` (IDs previos, DAG acíclico) | Opc (`[]`) | Req (`[]` en el 1.º) | Valida orden topológico y genera `## Contratos entre Pasos` en `index.md`. |
| `/work_units[]/acceptance_criteria[]` | `{id, kind, description, verify_step_id?, executor?}` (`kind`: `"command"` o `"review_gate"`, `description` de una línea sin `\r`/`\n`) | Req (`>= 1`) | Req (`>= 1`) | `## Criterios de Aceptación` en viñetas `- ` (`Chunk.Acceptance`, `ring.go:409`). `id` único globalmente en `spec.json`. Si `kind == "command"`, referencia `work_units[].verify_steps[].id`; si `kind == "review_gate"`, `executor` obligatorio (`4_Revisores` = equipo de revisión independiente \| `Modo_QA` = auditoría QA de `GengiSWAP` \| `AUTOR` = aprobación humana). |
| `/work_units[]/verify_steps[]` | `{id, cwd, env, argv[], expect_exit: 0, timeout_sec: 1..300}` | Req (`>= 1`) | Req (`>= 1`) | `## Comando de Verificación` en bloque fenced (`Chunk.VerifyCommands`, `ring.go:420`), compilado determinísticamente a `cmd.exe`. `id` único globalmente en `spec.json`. Todo `work_unit` exige `>= 1` `verify_step` válido según el allowlist. |
| `/work_units[]/failure_proof` | `{description, expect_exit: 1..255, expect_output_contains?}` | Req | Req | `## Prueba de que puede fallar` (`Chunk.FailureProof`, `bridge.go:504`, `ring.go:411`). **Nunca** se inyecta en `VerifyCommands` (evitando fallo en `ring.go:678-680`). |

*(Nota de canonización: `CanonicalSpecSHA256` serializa `Spec` canonizado con `json.Marshal` compacto, claves en orden determinista de struct, slices `nil` normalizados a `[]` y `VerifyStep.Env == nil` normalizado a `{}` —nunca `null`—, sin indentación ni CRLF. `ComputeScopeSHA256` calcula SHA-256 sobre la unión única, normalizada en minúsculas y ordenada lexicográficamente de `work_units[].can_modify` unida por `\n`.)*

### 4. Estructura de Artefactos de Estado en `2_Spec_Writer/runtime/`

1. **`spec_context.json` (congelado en `prepare`, actualizable únicamente vía `prepare --refresh-context`):**
   - Contiene `schema_version: "1.0"`, `created_at`, `trace_id`, `source_workitem_path` (relativo a `--factory-root`), `repo_root_basename` y `repo_root_sha256` (**nunca rutas absolutas de host**, evitando falsos positivos `win_user_path` bajo `C:\Users\...` en tests `t.TempDir()` de Windows; toda ruta física se resuelve desde flags CLI), `git_head` (informativo), `work_item` (`id`, `revision`, `sanitized_payload_sha256`, `project`, `kind`, `lane`, `suggested_template`, `risk_level`, `requires_autor_hitl`, `affected_paths[]`, `impact_flags[]`, `triage_reasons[]`, `risk_hits[]`), `brain_snapshot` (`brain_db_mtime` informativo, `risk_zones_sha256`, `matched_doc_hashes[]` `{rel_path, sha256}` con `rel_path` relativo a `--vault` sobre documentos activos relevantes, `invocations[]`), y `baseline_hashes[]` (`{path, state, sha256}` exclusivamente para `WorkItem.affected_paths`).
2. **`attempts.json` (inicializado por `prepare`, cerrojo `.wi.lock`):**
   - Esquema: `{schema_version: "1.0", wi_id, project, draft_attempts, final_attempts, external_cycles, last_exit, last_reason, last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256, updated_at}`.
   - Reglas de transición y orden global de cerrojos (`.wi.lock → .compile.lock → .events.lock`; `.wi.lock` se mantiene durante `[5]..[0]` en `validate` y `compile`):
     - **`prepare` inicial (WorkItem nuevo):** crea `spec_context.json`, inicializa `attempts.json` en cero y escribe el esqueleto inicial `spec.json`. Si `spec.json` o `spec_context.json` **ya existen** sin `--refresh-context` ni `--reset-attempts`, falla cerrado (`exit 2`, `SPEC_ALREADY_EXISTS`) sin sobrescribir `spec.json` ni reiniciar contadores.
     - **`prepare --refresh-context`:** estrictamente **no destructivo**: actualiza únicamente `spec_context.json`, invalida `last_passed_sha256 = ""`, `last_passed_stage = ""`, `last_passed_goldfish_sha256 = ""` en `attempts.json`, registra `CONTEXT_REFRESHED`, y **NO** sobrescribe `spec.json` ni reinicia `draft_attempts`, `final_attempts` ni `external_cycles`.
     - **`prepare --reset-attempts --reason "<motivo>"`:** única vía oficial tras `exit 5`; `--reason` obligatorio (`len >= 8`, `exit 2` si falta, `exit 3` si falla `ScanG4All`), reinicia `draft_attempts = 0`, `final_attempts = 0`, `external_cycles = 0`, limpia `last_passed_*`, **NO** sobrescribe `spec.json` (actualiza `spec_context.json` solo si se combina con `--refresh-context`) y registra `ATTEMPTS_RESET`.
     - **En `validate --stage draft`:** si `draft_attempts >= 3` → `exit 5` (`MAX_VALIDATE_ATTEMPTS`). Si falla con `exit 1` o `3`, incrementa `draft_attempts` y actualiza `last_exit`/`last_reason`. Si pasa (`exit 0`), actualiza `last_passed_sha256`, `last_passed_stage = "draft"`, `last_passed_goldfish_sha256 = ""`, `last_exit = 0`, `last_reason = "OK"` **sin** resetear `final_attempts`.
     - **En `validate --stage final`:** si `final_attempts >= 3` → `exit 5` (`MAX_VALIDATE_ATTEMPTS`). Si falla con `exit 1` o `3`, incrementa `final_attempts` y actualiza `last_exit`/`last_reason`. Si pasa (`exit 0`), actualiza `last_passed_sha256`, `last_passed_stage = "final"`, `last_passed_goldfish_sha256` (SHA-256 normalizado de `goldfish_review.json` en `ASDD`, o `""` en `FTD`), `last_exit = 0`, `last_reason = "OK"`.
     - **Ante decisión externa (paso `[4]` de `validate`):** si `external_cycles >= 3`, actualiza `last_exit = 4`, `last_reason = <reason>` y devuelve **`exit 5` (`MAX_EXTERNAL_CYCLES`)**; si `external_cycles < 3`, incrementa `external_cycles`, actualiza `last_exit = 4`, `last_reason = <reason>` y devuelve **`exit 4`**.
     - **Si `attempts.json` falta o está corrupto en `validate` o `compile`:** fail-closed **`exit 6`** (`ATTEMPTS_STATE_MISSING` / `ATTEMPTS_STATE_CORRUPT`, `retryable: false`).
3. **`goldfish_review.json` (requerido en `ASDD` `--stage final`):**
   - Ubicación fija: `2_Spec_Writer/runtime/<project>/<WI-id>/goldfish_review.json` (límite `MaxGoldfishBytes = 256 KiB`). Producido por el harness externo tras ejecutar los subagentes Goldfish del protocolo EGM sobre la spec y guardar el informe en `report_path`.
   - Esquema (`additionalProperties: false`): `{schema_version: "1.0", spec_sha256, lenses_run[], unresolved_blockers, report_path, reviewed_at}`.
   - Lentes requeridos calculados determinísticamente por `validate`:
     - 3 lentes fijos en `ASDD`: `egm-design` (crítica arquitectónica y de seguridad), `egm-codebase-details` (verificación literal de citas y símbolos en disco) y `egm-implementation-readiness` (ausencia de ambigüedades ejecutables).
     - Lentes condicionales: añadir `egm-structural` (estructura y simetría de contratos) si `WorkItem.impact_flags` contiene `contract`, `schema` o `db` (o `risk_level == "CRITICO"`); añadir `egm-zoom-out` (simplicidad y YAGNI) si `len(work_units) > 3`.
   - Criterio de pase: archivo presente (si falta `os.ErrNotExist` en `ASDD --stage final` → `exit 1` `VALIDATION_FAILED`, nunca `IO_ERROR` `exit 6`), `spec_sha256 == CanonicalSpecSHA256(spec)`, `unresolved_blockers == 0`, todos los lentes requeridos presentes en `lenses_run[]`, y `report_path` relativo a `--factory-root` limpio bajo `G4` (sin exigir existencia del archivo Markdown en disco para mantener el rol de higiene de consistencia). Documentado explícitamente como **higiene de consistencia, no prueba criptográfica de ejecución**.

### 5. Contrato CLI, Flujo y Precedencia Exacta de `validate` y `compile`

#### Matriz de Subcomandos y Flags CLI

| Subcomando | Flags Obligatorios | Flags Opcionales y Valores por Defecto | Salida (`stdout`) |
| :--- | :--- | :--- | :--- |
| `prepare` | `--workitem <path>` | `--repo-root` (`.`), `--factory-root` (`--repo-root`), `--vault` (`<factory-root>/0_Brain/vault`), `--data-dir` (`<factory-root>/2_Spec_Writer/runtime`), `--brain-bin` (`<factory-root>/0_Brain/brain.exe`), `--refresh-context`, `--reset-attempts`, `--reason <texto>`, `--json` | Resumen de estado o `PrepareResult` JSON (`--json`). |
| `validate` | `--spec <path>`, `--stage <draft\|final>` | `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin` (mismos defaults que `prepare`), `--json` | Resumen o `CLIResult` JSON (`--json`). |
| `render` | `--spec <path>` | `--json` *(precedencia: `[2]` `os.Lstat` + `CheckNoReparseChain` → `[3]` `MaxSpecBytes` + `ScanG4All` raw + parse JSON `INVALID_JSON` + `ScanG4All` decodificado + `schema_version == "1.0"` [`VALIDATION_FAILED`] → `[0]` proyección + `ScanG4All` salida; sin `0_Brain` ni escrituras)* | Markdown puro por defecto; `CLIResult` (`ok`, `exit_code`, `reason`, `spec_sha256`, `trace_id`, `markdown`) con `--json`. |
| `compile` | `--spec <path>`, `--task-root <dir>`, `--task-id <id>` | `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin` (mismos defaults que `prepare`), `--json` | Resumen informado + comando `gengis.exe plan`, o `CLIResult` JSON (`--json`). |
| `verify` | *(ninguno)* | `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin` (mismos defaults que `prepare`), `--json` | Resumen o `VerifyReport` JSON (`--json`). |

#### Precedencia Exacta de `validate` (`2 → 5 → 3 → 6 → 4 → 1`)

```text
validate --spec <path> --stage <draft|final> [--repo-root <dir>] [--json]
  [2] Uso CLI / flags inválidos / os.Lstat(--spec) inexistente        ──► Exit 2 (INVALID_USAGE)
      • Error OS distinto de ErrNotExist al inspeccionar --spec       ──► Exit 6 (IO_ERROR, retryable: true)
      • CheckNoReparseChain previo sobre dataDir / wiDir / specPath   ──► Exit 6 (REPARSE_POINT_DETECTED, retryable: false)
  [5] Adquirir .wi.lock (mantenido hasta [0]) y leer attempts.json:
      • Ausente / JSON corrupto                                       ──► Exit 6 (ATTEMPTS_STATE_*, retryable: false)
      • stage_attempts >= 3 (seleccionado por --stage sin leer spec)  ──► Exit 5 (MAX_VALIDATE_ATTEMPTS + last_exit/reason)
  [3] Cuotas de tamaño + G4 Fail-Closed (11 reglas, 0 redacción Tier 2) + parseo JSON:
      • spec.json > MaxSpecBytes (1 MiB) o (stage==final y goldfish presente > 256 KiB):
        inc stage_attempts                                            ──► Exit 1 (SPEC_TOO_LARGE, retryable: true)
      • Fase-A sobre bytes crudos de spec.json (+ goldfish_review si final y presente)
      • Parsear JSON para Fase-B (si sintaxis JSON inválida tras Fase-A ──► inc stage_attempts + Exit 1 INVALID_JSON)
      • Fase-B sobre todos los strings JSON decodificados (\u0067hp_...)
      • Si hallazgo G4: inc stage_attempts, log REJECTED_G4           ──► Exit 3 (G4_VIOLATION, retryable: false)
  [6] Preflight Compartido (CheckReparseSafety + CheckBrainPreflight):
      • CheckNoReparseChain sobre repo/runtime/vault                  ──► Exit 6 (REPARSE_POINT_DETECTED, retryable: false)
      • 0_Brain Preflight (.index/brain.db y .brain-journal.json)     ──► Exit 6 (BRAIN_UNAVAILABLE | BRAIN_JOURNAL_ACTIVE, retryable: true)
      • Errores OS de lectura / lock                                  ──► Exit 6 (IO_ERROR | LOCK_BUSY, retryable: true)
  [4] Decisión Externa (orden fijo: STALE_CONTEXT -> ESCALATION_REQUIRED -> LANE_UPGRADE_REQUIRED;
      si dispara y external_cycles >= 3 ──► Exit 5 MAX_EXTERNAL_CYCLES; si < 3 ──► inc + Exit 4):
      • Pre-saneamiento en memoria (sin I/O) con SanitizeRelativePaths sobre observed_files[] y can_modify[]:
        rutas inválidas en observed_files[] se difieren a [1]; si len(can_modify)==0 o can_modify falla higiene,
        [4] evalúa solo STALE_CONTEXT sobre spec_context.json y omite (2)/(3) delegando a [1] (Exit 1 VALIDATION_FAILED)
      • (1) STALE_CONTEXT: sha256(abs(repoRoot)) == spec_context.repo_root_sha256,
            risk_zones_sha256, matched_doc_hashes[], baseline_hashes[], y cada entrada saneada de
            spec.observed_files[] en disco ("present" hash CRLF/BOM o "absent" ausente)
      • Re-consulta 0_Brain context --files sobre rutas concretas saneadas de can_modify (status=="active"):
      • (2) ESCALATION_REQUIRED: sube risk_level respecto al baseline, nueva zona CRITICO, o
            len(RootModules) > 1 cuando (work_item.lane=="FTD" o se añade módulo raíz fuera de
            affected_paths), sin decisión activa válida en decisions[len(decisions)-1]
            (SanitizeRelativePaths + ComputeScopeSHA256(approved_paths) == decision.scope_sha256 ==
            current_scope_sha256, igualdad exacta de conjuntos normalizados)
      • (3) LANE_UPGRADE_REQUIRED: spec.lane=="FTD" y dispara cualquier regla ASDD de 1_Intake
            (kind feature/ops, impact_flags, CRITICO, risk_zones_parse_error != "" devuelto en vivo por
            0_Brain context --json o RISK_ZONES_UNPARSEABLE_FAILSAFE en spec_context.work_item.triage_reasons[],
            !bfcore.EvaluateFTDBounds.WithinBounds, o ruta sin cobertura activa wiki/adr)
  [1] Validador Compartido ValidateSpecAndRenderCheck (JSON Pointer RFC 6901; si falla ──► inc stage_attempts + Exit 1):
      • DisallowUnknownFields(), coincidencia explícita de work_item_ref (id/revision/sanitized_payload_sha256 contra
        spec_context.work_item, trace_id contra spec_context.trace_id, source_workitem_path contra
        spec_context.source_workitem_path), completitud observed_files[], prohibición descenso ASDD -> FTD (/lane),
        decisions[] obligatorio si ascenso FTD -> ASDD y, siempre que len(decisions) > 0, decisión activa
        (decisions[len(decisions)-1]) con scope_sha256 == current_scope_sha256, DAG acíclico depends_on,
        can_modify disjunto (salvo dependencia transitiva) y no sombreado por read_only_paths/forbidden,
        scope_expansions obligatorio para rutas nuevas, 1..2 units FTD / 1..99 ASDD
      • G2 (cero bloques Mermaid), G5 (1-based CRLF/BOM + ancla literal ±5 en texto UTF-8 y código),
        risk_mitigations[] completo con IDs globales, requires_autor_hitl unidireccional, CRITICO con review_gate
      • Allowlist cerrado de ejecutables/subcomandos/banderas/env en verify_steps
      • Render en memoria de plantillas GengiSWAP (<TARGET_REPO>, ## Puede modificar) + validateChunks +
        guarda parseIndexFile/parseChunkFile (campos de línea única sin \r/\n: title, target_branch,
        g3_data_mutation, work_units[].title, inputs[], acceptance_criteria[].description; sin encabezados # ni
        fences ni reMaxRetries ni parseFieldLabel de alcance en prosa de index.md; sin encabezados # ni fences en
        objective/failure_proof, sin Clave: val sin viñeta en objective/failure_proof, y sin Clave: val en inputs[])
      • En --stage final (ASDD): verificación de goldfish_review.json (ausente => Exit 1 VALIDATION_FAILED)
  [0] OK: persistir (last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256,
      last_exit=0, last_reason="OK") en attempts.json + evento VALIDATE_PASS ──► Exit 0
```

#### Precedencia Exacta de `compile` (`2 → 5 → 3 → 6 → 4 → 1`)

1. **`[2]` Uso, reparse previo y saneamiento de `--task-id`:** valida flags obligatorios (`--spec`, `--task-root`, `--task-id`), `os.Lstat(--spec)` (`INVALID_USAGE` → `exit 2`; otro error OS → `exit 6` `IO_ERROR`), `CheckNoReparseChain` sobre `dataDir`/`wiDir`/`specPath` (`REPARSE_POINT_DETECTED` → `exit 6`) y `--task-id` contra `^[a-z0-9][a-z0-9_-]{2,63}$` (sin dispositivos reservados Windows `CON..LPT9`).
2. **`[5]` Estado de freno (`attempts.json` bajo `.wi.lock` mantenido hasta `[0]`):** si falta/corrupto → `exit 6` (`ATTEMPTS_STATE_*`, `retryable: false`); si `draft_attempts >= 3 || final_attempts >= 3` → `exit 5` (`MAX_VALIDATE_ATTEMPTS`); si `external_cycles >= 3 && (last_passed_sha256 == "" || last_exit == 4)` → `exit 5` (`MAX_EXTERNAL_CYCLES`). *(No bloquea si el 3.er ciclo externo fue resuelto por un pase posterior de `validate` con `exit 0`.)*
3. **`[3]` Cuotas y barrera `G4` previa a cualquier escritura externa:** comprueba `MaxSpecBytes = 1 MiB` (y `MaxGoldfishBytes = 256 KiB` si `ASDD` y presente; `SPEC_TOO_LARGE` → `exit 1`, sin consumir intento en `compile`), luego escanea `spec.json` y, si `ASDD` y presente, `goldfish_review.json` (Fase-A raw + parseo JSON → `INVALID_JSON` `exit 1` si falla + Fase-B decodificado) con las 11 reglas (`exit 3`, `G4_VIOLATION`).
4. **`[6]` Preflight compartido, validación de `--repo-root` y cerrojo TOCTOU (`<task-root>/.compile.lock`):**
   - Ejecuta `CheckReparseSafety` (sobre `repoRoot`, `dataDir`, `vaultRoot`) y `CheckBrainPreflight`, más `bfcore.ScanTier1` sobre `--repo-root` (`exit 3` si contuviera un secreto Tier 1).
   - Ejecuta `bfcore.CheckNoReparseUpToNearestExisting` sobre `--task-root`, crea `--task-root` si no existe (`os.MkdirAll(taskRoot, 0o755)`), verifica `CheckNoReparseChain` sobre `<task-root>` y `<task-root>/.compile.lock`, y adquiere el cerrojo estático único `<task-root>/.compile.lock` vía `bfcore.AcquireFileLock` (`LOCK_BUSY`, `retryable: true`; nunca se elimina el dotfile de cerrojo en Windows).
   - **Dentro del cerrojo**, verifica `CheckNoReparseChain` e inspecciona `<task-root>/<task-id>` y `<task-root>/<task-id>/.ring/state.json` con `os.Lstat` fail-closed (error distinto de `os.ErrNotExist` → `exit 6` `IO_ERROR`) bajo **política estricta *create-only* en v1**:
     - Si `<task-root>/<task-id>/.ring/state.json` existe → aborta sin modificar nada con **`exit 6`, `reason: "LIVE_TASK_STATE"`, `retryable: false`**.
     - Si `<task-root>/<task-id>` ya existe en disco (aunque no tenga `.ring/state.json`) → aborta sin sobrescribir con **`exit 6`, `reason: "TASK_DIR_EXISTS"`, `retryable: false`**.
5. **`[4]` Revalidación de contexto/alcance en vivo (solo lectura sobre `attempts.json`):** re-ejecuta el bloque `[4]` de `validate` sin mutar `attempts.json` (no incrementa `stage_attempts` ni `external_cycles`); si se dispara una nueva decisión externa y `external_cycles >= 3`, devuelve **`exit 5` (`MAX_EXTERNAL_CYCLES`)**; si `external_cycles < 3`, devuelve **`exit 4`**.
6. **`[1]` Anti-bypass completo, validador compartido y frontera `G4` de dos zonas:**
   - Comprueba: (a) `CanonicalSpecSHA256(spec) == attempts.last_passed_sha256`, (b) `last_passed_stage` satisface la etapa requerida (`"draft"` o `"final"` para `FTD`; estrictamente `"final"` para `ASDD`), y (c) en `ASDD`, `goldfish_review.json` existe y su SHA-256 normalizado coincide con `attempts.last_passed_goldfish_sha256`. Si cualquiera falla → **`exit 1`, `reason: "VALIDATION_REQUIRED"`, `retryable: true`** (sin consumir intento).
   - Ejecuta el mismo `ValidateSpecAndRenderCheck` de `validate` (que genera en memoria `index.md` con marcador `<TARGET_REPO>`, `chunks/NN_<id>.md` con `## Puede modificar` y etiquetas HITL, y `status.md`, y valida el superconjunto de `validateChunks`) + `bfcore.ScanG4All` sobre todos esos buffers en memoria **antes** de enlazar `<TARGET_REPO>`. Si fallara tras un pase previo válido para el mismo `(spec_sha256, stage, goldfish_sha256)` → **`exit 1`, `reason: "VALIDATOR_BYPASS_DEFECT"`, `retryable: false`** (guardia defensiva de invariante sin consumir intento, registrada en `specwriter_events.jsonl`).
7. **`[0]` Enlace efímero de `--repo-root`, promoción atómica *create-only* en Windows y resumen informado:** dentro de `.compile.lock`:
   - Sustituye únicamente la línea `- **Repositorio destino:** <TARGET_REPO>` en el buffer de `index.md` por `filepath.Clean(absRepoRoot)` (ya verificado contra `repo_root_sha256`, `CheckNoReparseChain` y `ScanTier1`).
   - Crea staging hermano en el **mismo volumen NTFS** (`<task-root>/.<task-id>.staging-<rand>`) con `defer os.RemoveAll(stagingDir)` si cualquier paso falla antes del rename.
   - Escribe con `Write` + `Sync` + `Close` los archivos `index.md`, `chunks/01_<id>.md`..`NN_<id>.md` y `status.md` inicial, y crea el directorio vacío `workspace/qa/`.
   - Re-verifica con `os.Lstat` que `<task-root>/<task-id>` sigue inexistente (`os.ErrNotExist`) y ejecuta **un único `os.Rename(stagingDir, destDir)` atómico** (con reintento Windows acotado ante bloqueos transitorios de antivirus/indexador).
   - Registra `COMPILED` en `specwriter_events.jsonl` (bajo `.events.lock` con `RedactAllG4` + `ScanG4All`), imprime el resumen informado (`spec_sha256`, `trace_id`, `lane`, `scope_expansions`, `risk_mitigations`, `decisions`, lentes Goldfish y comando exacto `gengis.exe plan`) y sale con `exit 0`.

### 6. Allowlist Cerrado de Comandos y Banderas (`verify_steps`) y Serialización Determinista `cmd.exe`

`validate` y `compile` **nunca ejecutan** `verify_steps`; los auditan contra una tabla cerrada de **(ejecutable, subcomando, banderas permitidas)** embebida en Go (en lugar de una denylist abierta), acotada en v1 al monorepo Black Factory y repositorios Go:

| Ejecutable permitido (`argv[0]`) | Subcomando (`argv[1]`) | Banderas permitidas (normalizadas `-flag` / `--flag` / `-flag=val`) | Restricción de `cwd` y argumentos |
| :--- | :--- | :--- | :--- |
| `go` | `test` | `-count=1`, `-v`, `-short`, `-race`, `-cover`, `-run=<patrón>` (`^[A-Za-z0-9_$./-]+$`), `-timeout=<1s..300s>` | `cwd` relativo opcional; argumentos solo paquetes relativos (`./...`, `./sub/...`). *(Para verificar compilación sin ejecutar pruebas ni escribir `.exe`, usar `go test -run=NoTestsMatch ./...`; `go build` queda excluido porque escribe `.exe` en `cwd`.)* |
| `go` | `vet` | `-v` | `cwd` relativo opcional; argumentos solo paquetes relativos. |
| `git` | `status` | `-s`, `--short`, `--porcelain`, `--ignored` | `cwd` relativo opcional; rutas relativas limpias. |
| `git` | `diff` | `--stat`, `--name-only`, `--check`, `--cached`, `--staged` | `cwd` relativo opcional; rutas relativas limpias (sin `--output`, `--ext-diff`, `--no-index`, `-C`). |
| `git` | `rev-parse` | `--short`, `--abbrev-ref` | `cwd` relativo opcional; argumento opcional `HEAD`. |
| `git` | `check-ignore` | `-q`, `-v` | `cwd` relativo opcional; rutas relativas limpias. |
| `0_Brain/brain.exe` | `query`, `context` | `query`: `--vault`, `--db`, `--project`, `--type`, `--tag`, `--q`, `--limit`, `--json` (`0_Brain/engine/cmd/brain/main.go:205-212`); `context`: `--vault`, `--db`, `--project`, `--file`, `--files`, `--json` (`0_Brain/engine/cmd/brain/main.go:254-259`) | `cwd == ""` obligatorio; **`verify` excluido** porque ejecuta `RecoverJournal` (`0_Brain/engine/cmd/brain/main.go:313-323`); todo valor de ruta estrictamente relativo (sin `..`, `:`, `\\` ni `/` inicial). |
| `1_Intake/intake.exe` | `list`, `show` | `list`: `--data-dir`, `--project`, `--status`, `--lane`, `--risk`, `--json`; `show`: `--data-dir`, `--id`, `--format`, `--json` | `cwd == ""` obligatorio; **`verify` excluido** porque adquiere `.intake.lock` y escribe `VERIFIED` en `intake_events.jsonl` (`1_Intake/engine/cmd/intake/main.go:697-700`); `ingest`/`resolve` excluidos; rutas estrictamente relativas. |
| `2_Spec_Writer/specwriter.exe` | `verify`, `render` | `verify`: `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json`; `render`: `--spec`, `--json` | `cwd == ""` obligatorio; solo lectura (`render` emite a `stdout` sin flag `--out`); rutas estrictamente relativas (para `--factory-root`/`--repo-root` se admite `"."` explícito u omitirlos para usar el default `"."`). |

*(Nota: `GengiSWAP/gengis.exe status` queda excluido porque `ShowStatus` invoca `SyncFiles` y muta `status.md` en `bridge.go:1487`; todo intérprete o shell —`cmd`, `powershell`, `pwsh`, `sh`, `bash`, `python`, `node`, `npm`, `npx`, scripts `.bat`/`.cmd`/`.ps1`— y toda bandera fuera de la columna de permitidas, incluidas `-ldflags`, `-toolexec`, `-exec`, `-o`, `-c`, `-test.*profile`, `-C`, se rechazan con `exit 1` y JSON Pointer `/work_units/<i>/verify_steps/<j>/argv/<k>`.)*

*   **Validación estricta de `env` (claves y valores):**
    - `CGO_ENABLED` (`"0"` únicamente), `GENGIS_CONTRACT_STRICT` (`"0"` | `"1"`), `GOOS` (`"windows"` | `"linux"` | `"darwin"`), `GOARCH` (`"amd64"` | `"arm64"`), `GOAMD64` (`"v1"` | `"v2"` | `"v3"` | `"v4"`), `PYTHONUTF8` (`"0"` | `"1"`), `PYTHONIOENCODING` (`"utf-8"`).
    - **`GENGIS_BIN` queda explícitamente excluida** de `spec.json` (solo proviene del entorno del operador/CI), al igual que `PATH`, `COMSPEC`, `GOFLAGS`, `PYTHONPATH`, `NODE_OPTIONS` y `LD_PRELOAD`.
*   **Caracteres rechazados sin escape en `cwd`, `env` y cada `argv[k]`:** cadenas vacías `""`, barra invertida final `\`, acento grave final `` ` `` (tratado como continuación de línea cuando va precedido de espacio en `extractFenceCommands`, `bridge.go:607`; rechazar cualquier `` ` `` final es un superconjunto seguro), controles/espaciadores `\t`, `\v`, `\f`, `\r`, `\n`, `\x00`, y metacaracteres `&`, `|`, `<`, `>`, `^`, `%`, `!`, `"`, `'`, `(`, `)`, `;`.
*   **Serialización determinista para Windows `cmd.exe /S /C` (`runner_windows.go:23`, con `cmd.Dir = TargetRepo` en `bridge.go:1175-1178`):**
    1. Ordenar las claves de `env` lexicográficamente y anteponer `set K=V&&` por cada par.
    2. Si `cwd != ""`, convertir `/` → `\` y anteponer **siempre entrecomillado** `cd /d "<cwd>" && `.
    3. Convertir `/` → `\` en el ejecutable `argv[0]` (de modo que `0_Brain/brain.exe` se serialice como `0_Brain\brain.exe`, evitando que `cmd.exe` interprete `/` como modificador).
    4. Entrecomillar cada token `argv[k]` que contenga espacio o `,` (seguro porque `"`, `^` y `\`/`` ` `` finales fueron rechazados).
*   **Límites de ejecución:** `expect_exit` fijado a `0` y `timeout_sec` en `1..300` (`Timeout: 5 * time.Minute` en `bridge.go:1178`). Pasos `> 5 min` se declaran como `review_gate`; expectativas `expect_exit != 0` viven exclusivamente en `failure_proof`.

### 7. `specwriter verify` (Solo Lectura), Separación `--factory-root` vs. `--repo-root` y Deriva `RZ-001`

`specwriter verify [--factory-root <dir>] [--repo-root <dir>] [--vault <dir>] [--data-dir <dir>] [--brain-bin <path>] [--json]` opera sin efectos secundarios (`0` archivos creados, `0` cerrojos exclusivos; usa `ReadFileWithShortRetry` ante bloqueos momentáneos de Windows). Si `dataDir` (`2_Spec_Writer/runtime`) aún no existe en un repositorio limpio, la comprobación de `runtime/` pasa con `ok: true`, `g4_issues: 0`, `artifacts_checked: 0` y `0` archivos creados.

*   **Preflight compartido vs. checks exclusivos de `verify`:**
    - **Compartidos (`prepare`, `validate`, `compile` y `verify`):** `CheckReparseSafety` (`REPARSE_POINT_DETECTED`, `exit 6`, `retryable: false`) y `CheckBrainPreflight` (`BRAIN_UNAVAILABLE` / `BRAIN_JOURNAL_ACTIVE`, `exit 6`, `retryable: true`).
    - **Exclusivos de `verify` (sobre `--factory-root`, default igual a `--repo-root`):**
      1. **Aislamiento `Opción i` (comprobación individual por ruta):** ejecuta dos llamadas separadas `git -C <factoryRoot> check-ignore -q 2_Spec_Writer/runtime/` **y** `git -C <factoryRoot> check-ignore -q 2_Spec_Writer/specwriter.exe`, exigiendo código `0` en **cada una** → `exit 6`, `reason: "GITIGNORE_DRIFT"`, `retryable: false`.
      2. **Detección de deriva en `RZ-001`:** compara `bfcore.ComputeNormalizedFileSHA256` sobre `<factoryRoot>/GengiSWAP/engine/bridge.go` y `<factoryRoot>/GengiSWAP/engine/ring.go` contra `PinnedBridgeGoSHA256` y `PinnedRingGoSHA256` en `verify.go` → `exit 6`, `reason: "GENGIS_ENGINE_DRIFT"`, `retryable: false`.
      3. **Integridad y `G4` sobre `2_Spec_Writer/runtime/`:** recorre todos los archivos existentes (`spec_context.json`, `attempts.json`, `spec.json`, `goldfish_review.json`, `specwriter_events.jsonl`), ejecuta `bfcore.ScanG4All` (`exit 3`, `G4_VIOLATION_ON_DISK`, `retryable: false`) y valida sintaxis JSON (`attempts.json` corrupto → `exit 6`, `ATTEMPTS_STATE_CORRUPT`, `retryable: false`; otros JSON/JSONL corruptos → `exit 6`, `ARTIFACT_STATE_CORRUPT`, `retryable: false`; error OS transitorio → `exit 6`, `IO_ERROR`, `retryable: true`).

### 8. Test de Contrato Real contra `gengis.exe plan`

Sin importar código de `GengiSWAP/engine/` (`RZ-001`):
*   Lee la ruta del binario desde la variable de entorno del operador/CI `GENGIS_BIN` (por defecto `../../GengiSWAP/gengis.exe`; nunca configurable desde `spec.json`).
*   Si el binario no existe: en local ejecuta `t.Skipf(...)`; si `GENGIS_CONTRACT_STRICT=1` (obligatorio antes de merge), falla con `t.Fatalf`.
*   **Casos positivos (`FTD` con `failure_proof.expect_exit: 1` y `ASDD` multi-chunk sobre `t.TempDir()` con `git init`):** compila en `t.TempDir()` (verificando en vivo la frontera `G4` de dos zonas con ruta real `C:\Users\...`), ejecuta `gengis.exe plan <tmpTask> --root <tmpRoot>` con `"n\n"` en `stdin` (sin tocar `C:\Black Factory\GengiSWAP\gengis_global_log.md`) y verifica: `exit 1`, resumen del plan en `stdout` con todos los chunks, etiquetas HITL/escalado, alcance `Modificar` poblado y el `TargetRepo` real sin `ADVERTENCIA: destino sin Git`, `<tmpTask>/.ring/state.json` creado con `"status": "PLAN_PENDING"` y `Chunk.Scope.CanModify` no vacío (validando paridad `## Puede modificar`), y `stderr` con `"plan no aprobado por el Autor"` (`ring.go:28, 570`).
*   **Casos negativos de paridad:** fixtures con `FailureProof` vacío, `VerifyCommands` vacío o ID duplicado rechazados tanto por el validador compartido como por `gengis.exe plan` (`exit 1`, `stderr` `"chunk inválido"`, sin resumen de plan y sin crear `.ring/state.json`).

--------------------------------------------------------------------------------

## Alternatives Considered

### Alternative A: Llamar API de LLM Directamente desde `specwriter.exe` (Go)

*   **Pros:** Permite un comando único autocontenido sin depender de un agente/harness externo.
*   **Cons:** Introduce credenciales de API, dependencias HTTP, reintentos y manejo de streaming/tool-calling en el binario Go; rompe la operación offline/sin secretos y duplica el harness de Antigravity y EGM.
*   **Reason for Rejection:** Rechazada por acoplamiento de red y credenciales: el binario Go permanece 0% LLM y sin secretos; la ejecución desatendida futura se resuelve mediante orquestación externa sobre el CLI del agente (`prepare → agente → validate → compile`).

### Alternative B: Implementar `2_Spec_Writer` Únicamente como `SKILL.md` sin Binario Go

*   **Pros:** Cero código Go que mantener; cambio rápido de formato editando Markdown.
*   **Cons:** Viola `constitution.md:22` (*determinismo del motor sobre buena voluntad del modelo*); deja sin verificación mecánica `G2`, `G4`, `G5`, conservación de `risk_hits`, allowlist de comandos y compatibilidad con `bridge.go`.
*   **Reason for Rejection:** Rechazada por requisito de determinismo: las comprobaciones objetivas sin juicio deben ejecutarse de forma determinista en Go para evitar ciclos fallidos en `3_Builder`.

### Alternative C: Delegar la Partición de Chunks a `3_Builder` o a un Divisor Mecánico por Archivo/Tamaño

*   **Pros:** Mantiene `spec.json` más simple al omitir `work_units[]`.
*   **Cons:** La experiencia en tareas previas de Black Factory (como `bf_01_intake_v1`, 43 archivos agrupados en 2 chunks disjuntos por contrato compartido) demostró que la partición requiere juicio arquitectónico sobre el código. Delegarla a `3_Builder` obliga a releer el repositorio y mezcla responsabilidades de diseño (`2_Spec_Writer`) y ejecución (`3_Builder`).
*   **Reason for Rejection:** Rechazada por separación de responsabilidades: la partición en `work_units[]` es juicio de diseño y pertenece a `2_Spec_Writer`, mientras la serialización hacia `index.md`/`chunks/*.md` es mecánica y pertenece al compilador determinista.

### Alternative D: Autoría Dual de Markdown + `spec.json` y Campo Opaco `extensions`

*   **Pros:** Mayor libertad de prosa para el agente y punto de extensión sin versionar el esquema.
*   **Cons:** Duplica tokens de salida, permite deriva semántica entre lo que revisa `[AUTOR]` y lo que compila `compile` (violando el precedente `NG5` de `Docs/designs/1_intake.md`), e introduciría un vector de fuga `G4` y de mantenimiento en `extensions`.
*   **Reason for Rejection:** Rechazada por deriva de doble fuente de verdad: `spec.json` (`additionalProperties: false`) es la única fuente de verdad con proyección Markdown determinista (`render`); toda evolución futura se gestiona mediante `schema_version`.

### Alternative E: Validar la Salida de `compile` Mediante `gengis.exe status <tmpDir>`

*   **Pros:** Evita duplicar validaciones de `GengiSWAP` en `2_Spec_Writer`.
*   **Cons:** Verificado en `GengiSWAP/engine/bridge.go:1455-1488`: `ShowStatus` omite `validateChunks` (`ring.go:393-426`, solo ejecutado por `PlanTask`) y llama `SyncFiles`, sobrescribiendo `<tmpDir>/status.md`. Aceptaría chunks inválidos silenciosamente.
*   **Reason for Rejection:** Rechazada por omisión de validación y efectos secundarios verificados: reemplazada por el validador compartido `ValidateSpecAndRenderCheck` (superconjunto de `validateChunks`) más un test de contrato real contra `gengis.exe plan` en `t.TempDir()` (`GENGIS_CONTRACT_STRICT=1`).

### Alternative F: Máquina de Estados Completa, `specwriter approve`, `publish`, DSL Declarativo Externo, Prefijos de Directorio en `can_modify` y Wrapper `BrainPatch` en v1

*   **Pros:** Cobertura anticipada para exportación pública, múltiples operadores, directorios completos en `ASDD` y propuestas automáticas en `0_Brain`.
*   **Cons:** Sin firma criptográfica Ed25519, `approve` en un workspace editable por el agente no aporta aislamiento real y duplica la parada de `gengis.exe plan`; el DSL externo añade complejidad para 2 carriles; admitir prefijos de directorio en `ASDD` `can_modify` sin modificar `0_Brain` obliga a duplicar el parser de `wiki/risk_zones.md` (`0_Brain/engine/query.go:186-332`), truncar expansiones en disco y abrir riesgos de omisión de zonas `CRITICO`; y `0_Brain/engine/vault.go:244-418` (`ProposeUpdate`) carece hoy de `.brain.lock` y `ScanG4`.
*   **Reason for Rejection:** Rechazada por YAGNI y límites de seguridad: diferidos explícitamente a v2 con disparadores concretos; v1 se acota a un núcleo mínimo (`prepare`, `validate`, `render`, `compile`, `verify`), rutas de archivo concretas en `can_modify`, `go:embed` solo para plantillas y `attempts.json` documentado explícitamente como freno operativo contra bucles.

### Alternative G: Modificar `GengiSWAP/engine/` (`RZ-001`) para Ejecutar `argv` Estructurado u Omitir la Confirmación de `gengis.exe plan`

*   **Pros:** Eliminaría la traducción a `cmd.exe /S /C` y permitiría timeouts o `expect_exit` arbitrarios.
*   **Cons:** `GengiSWAP/engine/` es zona `CRITICO` (`RZ-001`); abrir cambios allí amplía el radio de impacto y retrasa la entrega de `2_Spec_Writer`.
*   **Reason for Rejection:** Rechazada por radio de impacto en `RZ-001`: `RZ-001` permanece intacto en v1; el compilador garantiza compatibilidad segura mediante allowlist cerrado de comandos/banderas, serialización determinista `cmd.exe` y límites fijos (`expect_exit == 0`, `timeout <= 5 min`).

### Alternative H: Políticas Alternativas para Tests en `FTD=5` (Plan B `1:1`, Opción `5A` por Nombre Exacto o Regla Multi-Lenguaje por Directorio)

*   **Pros:** Plan B (`1:1`) no toca `1_Intake`; `5A` (`foo_test.go ↔ foo.go`) es más restrictiva; la regla multi-lenguaje cubriría otros stacks.
*   **Cons:** Plan B degrada correcciones pequeñas en Go a `ASDD` por añadir tests; `5A` penaliza archivos de prueba consolidados del paquete (como `brain_test.go` o `intake_test.go`); la regla "mismo directorio" no encaja con Python (`tests/`) ni TS/JS (`__tests__`).
*   **Reason for Rejection:** Rechazada en favor de **Opción `5B`** en `pkg/bfcore` (`*_test.go` en el mismo directorio que un archivo productivo modificado con tope duro de `5` tests exentos / `10` archivos físicos totales, `<= 1` módulo raíz estricto, y resto de lenguajes `1:1`).

### Alternative I: Mutar `1_Intake/runtime/workitems/WI-*.json` en Escalados o Añadir `visibility` a `workitem.schema.json` v1.0

*   **Pros:** Mantendría un único estado de carril en Equipo 1 y permitiría visibilidad por ticket.
*   **Cons:** Rompe el esquema cerrado `workitem.schema.json` v1.0 (`additionalProperties: false`), mezcla responsabilidad de escritura entre Equipo 1 y Equipo 2, y hace que la visibilidad sea manipulable por ticket individual.
*   **Reason for Rejection:** Rechazada por inmutabilidad de esquema y límites de propiedad: `WI-*.json` permanece como registro inmutable del triaje inicial, las resoluciones de escalado viven en `spec.json.decisions[]` acotadas a `approved_paths[]` + `scope_sha256`, y todo v1 es privado en `2_Spec_Writer/runtime/`.

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Seguridad y Privacidad (`G4`, `Opción i`, Anti-Inyección y Anti-TOCTOU):**
    - Las 11 reglas `G4` de `pkg/bfcore/g4.go:48-115` se aplican fail-closed (`exit 3`, cero auto-redacción Tier 2) en `prepare`, `validate`, `render`, `compile` y `verify`, sobre bytes crudos y strings JSON decodificados, sin transcribir secretos ni cadenas disparadoras en `attempts.json`, `stderr` ni `specwriter_events.jsonl`. En `compile`, la frontera de dos zonas ejecuta `ScanG4All` sobre `index.md` con `<TARGET_REPO>` antes de sustituir `--repo-root` (verificado con `ScanTier1`, `CheckNoReparseChain` y `repo_root_sha256`) exclusivamente en `gengis_task/<ID>/index.md`.
    - Todo registro de `specwriter_events.jsonl` aplica `bfcore.RedactAllG4` sobre campos de texto/detalle y verifica `len(bfcore.ScanG4All(line)) == 0` en memoria antes de escribir bajo `.events.lock`.
    - `validate` nunca ejecuta comandos de la spec; `verify_steps` exige allowlist cerrado de ejecutables/subcomandos/banderas/valores `env` (excluyendo `0_Brain/brain.exe verify`, `go build`, `GENGIS_BIN`, `gengis.exe status`, intérpretes y shells) y serialización determinista `cmd.exe`.
    - Cuotas de lectura: `MaxSpecBytes = 1 MiB`, `MaxGoldfishBytes = 256 KiB`, `MaxObservedFileBytes = 10 MiB` (hashing en streaming).
    - Orden estricto de cerrojos: `.wi.lock → .compile.lock → .events.lock`. Las escrituras en `runtime/<project>/<WI-id>/` mantienen `.wi.lock` durante `[5]..[0]`; `compile` usa el cerrojo estático único `<task-root>/.compile.lock` + staging atómico *create-only* en el mismo volumen NTFS + `CheckNoReparseChain` + comprobación de `.ring/state.json` y existencia de destino dentro del cerrojo.
    - `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` permanecen excluidos en `.gitignore`, verificado individualmente por `specwriter verify` (`GITIGNORE_DRIFT` → `exit 6`, `retryable: false`).
*   **Rendimiento y Latencia:**
    - Presupuesto `< 500 ms` p95 en Windows para `prepare`, `validate`, `render`, `compile` y `verify` locales (sobre bóvedas de 50–100 documentos indexados en `0_Brain` y hasta `50` artefactos en `2_Spec_Writer/runtime/`), con un máximo de `2` invocaciones read-only a `0_Brain/brain.exe`.
    - Ejecutar `validate --stage draft` (0 tokens) antes de lanzar revisores Goldfish en `ASDD` para evitar consumo de tokens sobre citas `G5` o esquemas rotos.
*   **Observabilidad y Trazabilidad:**
    - Propagación íntegra de `WorkItem.trace_id` a `spec_context.json`, `specwriter_events.jsonl` (eventos `PREPARED`, `CONTEXT_REFRESHED`, `ATTEMPTS_RESET`, `VALIDATE_PASS`, `VALIDATE_FAIL`, `REJECTED_G4`, `EXTERNAL_DECISION`, `ATTEMPTS_BLOCKED`, `COMPILED`, `VALIDATOR_BYPASS_DEFECT`, `LIVE_TASK_REFUSED`, `TASK_DIR_REFUSED`, con `duration_ms`, `reason`, `retryable` y `spec_sha256`) e `index.md`.
*   **Despliegue, Compatibilidad y Rollback:**
    - Compilación reproducible `CGO_ENABLED=0`; cero cambios a `0_Brain/**`, `GengiSWAP/engine/` (`RZ-001`) ni `workitem.schema.json` v1.0.
    - El cambio en `1_Intake/engine/triage.go` (adopción de `bfcore.EvaluateFTDBounds` Opción `5B`) es retrocompatible para todas las peticiones existentes de `<= 5` archivos y aplica una relajación estrictamente acotada para `*_test.go` hermanos en Go, validado con la suite completa de `pkg/bfcore`, `0_Brain/engine` y `1_Intake/engine`.
    - Rollback mediante `git revert` del commit del chunk sin mutar `0_Brain/vault/`, `1_Intake/runtime/` ni tareas activas en `C:\Desarrollo\GengiSWAP_data\gengis_task\`.

--------------------------------------------------------------------------------

## Risks, Limits & Mitigations

| Riesgo / Límite Operativo (v1) | Impacto Potencial | Mitigación en v1 y Disparador v2 |
| :--- | :--- | :--- |
| **`attempts.json` en workspace editable por el agente** | Un agente con acceso al sistema de archivos podría alterar manualmente el contador si ignora la skill. | Documentado explícitamente como **freno operativo contra bucles**, no control de seguridad. `validate`/`compile` fallan cerrados (`exit 6`) si falta o está corrupto. *(v2: firma Ed25519 si hay ejecución desatendida.)* |
| **`goldfish_review.json` como higiene de consistencia** | El binario Go verifica formato, `spec_sha256` y `unresolved_blockers == 0`, no ejecuta los subagentes LLM. | La verificación real de fondo ocurre en la parada humana `[AUTOR]` en `gengis.exe plan` con etiquetas `[HITL]`/`[CRITICO]`/`[ESCALATED]` visibles. |
| **Frontera `G4` de dos zonas en `index.md` externo** | `--repo-root` bajo `C:\Users\...` (p. ej. `t.TempDir()`) debe aparecer en `gengis_task/<ID>/index.md` para `extractTargetRepo` (`GengiSWAP/engine/bridge.go:236-308`). | Solo se sustituye el marcador único `<TARGET_REPO>` tras pasar `ScanG4All` sobre el resto del buffer y validar `repo_root_sha256` + `ScanTier1` + `CheckNoReparseChain`. `2_Spec_Writer/runtime/` queda libre de rutas absolutas de `--repo-root` y de rutas personales `G4`. |
| **Límite de `5 min` y `ExitCode == 0` de `ShellRunner` (`GengiSWAP`)** | Pasos lentos o pruebas negativas (`expect_exit != 0`) no son expresables como `VerifyCommands`. | `verify_steps` exige `timeout_sec in 1..300` y `expect_exit == 0`; verificaciones lentas se modelan como `review_gate` y pruebas negativas viven exclusivamente en `failure_proof`. |
| **Rutas concretas en `can_modify` y allowlist Go/Black Factory en v1** | Cambios masivos en directorios completos requieren enumerar archivos o escalar; stacks Python/JS no tienen comandos de test en `verify_steps` v1. | Elimina bypasses `CRITICO` y parsers duplicados de `risk_zones.md` manteniendo `0_Brain/**` intacto en v1. *(v2: compartir `ParseRiskZonesCatalog` en `pkg/bfcore` y abrir allowlist por stack cuando existan repos reales.)* |
| **Pinning SHA-256 de `GengiSWAP/engine/{bridge,ring}.go`** | Cualquier edición de comentario/formato en `RZ-001` dispara `GENGIS_ENGINE_DRIFT` en `specwriter verify`. | Comportamiento intencional para vigilar `RZ-001` sin correr `go test`, acotado exclusivamente a `specwriter verify` (no bloquea `validate` ni `compile`). |

--------------------------------------------------------------------------------

## Additional Resources

*   Arquitectura base de `0_Brain`: [`Docs/BLACK_FACTORY_BRAIN.md`](../BLACK_FACTORY_BRAIN.md)
*   Especificación aprobada de `1_Intake`: [`Docs/designs/1_intake.md`](1_intake.md) y [`1_Intake/schemas/workitem.schema.json`](../../1_Intake/schemas/workitem.schema.json)
*   Motor determinista `GengiSWAP`: [`GengiSWAP/engine/bridge.go`](../../GengiSWAP/engine/bridge.go), [`GengiSWAP/engine/ring.go`](../../GengiSWAP/engine/ring.go), [`GengiSWAP/engine/runner_windows.go`](../../GengiSWAP/engine/runner_windows.go) y [`GengiSWAP/templates/README.md`](../../GengiSWAP/templates/README.md)
*   Constitución y zonas de riesgo: `0_Brain/vault/constitution.md` y `0_Brain/vault/wiki/risk_zones.md` *(archivos locales privados `Opción i`)*

--------------------------------------------------------------------------------

## Future Improvements

*   **`specwriter approve` + firma criptográfica Ed25519 (v2):** *(Disparador: ejecución desatendida en CI o múltiples operadores).* Autenticar aprobaciones y desbloqueos `--reset-attempts` mediante `.autor/autor.pub` (mismo esquema que `0_Brain`).
*   **`specwriter publish` + `publish_allowlist` (v2):** *(Disparador: segunda necesidad real de exportar specs a directorios versionados).* Exportación `private`-by-default que remueve metadatos de runtime conservando `source_spec_sha256` y `G4`.
*   **Prefijos de directorio en `ASDD` `can_modify` con parser compartido (v2):** *(Disparador: necesidad recurrente de modificar directorios enteros en `ASDD`).* Migrar `ParseRiskZonesCatalog` de `0_Brain/engine/query.go` a `pkg/bfcore` para cruce bidireccional único sin duplicación.
*   **DSL declarativo externo de plantillas (v2):** *(Disparador: divergencia real entre `>= 3` proyectos).* Sustituir plantillas `go:embed` por contratos declarativos versionados.
*   **Wrapper `BrainPatch` / `brain.exe propose` para ADRs en `ASDD` (v2):** *(Disparador: resolución del WorkItem independiente en `0_Brain` que añada `.brain.lock` y `ScanG4` a `ProposeUpdate` en `0_Brain/engine/vault.go:244-418`).*
*   **Reglas de archivo hermano y allowlist `verify_steps` para stacks adicionales (v2):** *(Disparador: adopción activa de repositorios Python o TS/JS en Black Factory).* Definir semántica explícita por lenguaje en `pkg/bfcore/ftd.go` y subcomandos read-only permitidos en `validate.go`.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

La implementación se estructura en **dos fases con alcances estrictamente disjuntos** para facilitar la posterior compilación a dos chunks de `GengiSWAP` con una única parada `[AUTOR]` en `gengis.exe plan` (`Fase 0`: núcleo compartido `pkg/bfcore` + delegación en `1_Intake/engine`; `Fase 1 + Integración`: `2_Spec_Writer/**` + cableado raíz `go.work`, `.gitignore`, `README.md`). No se modifica ningún archivo bajo `0_Brain/**`, `GengiSWAP/**` (`RZ-001`) ni `1_Intake/schemas/workitem.schema.json`.

### Target Files to Create/Modify

#### Fase 0 — Primitivas Compartidas en `pkg/bfcore` y Delegación en `1_Intake/engine`

#### 1. `pkg/bfcore/ftd.go` (Create)

*   **Type / Scope**: Go Library (`blackfactory/pkg/bfcore`, `CGO_ENABLED=0`, stdlib-only).
*   **Proposed Changes**:
    - Definir constantes canónicas: `DefaultMaxFTDFiles = 5`, `MaxFTDExemptGoTests = 5`, `MaxFTDPhysicalFiles = 10`, `MaxFTDRootModules = 1`.
    - Exportar `type FTDEvaluation struct { PhysicalCount, EffectiveCount, ExemptTestCount int; RootModules []string; WithinBounds bool }`.
    - Exportar `CountRootModules(paths []string) []string` (migrado desde `1_Intake/engine/paths.go:137-160`, preservando mapeo de archivos raíz sin `/` a `"."` y deduplicación case-insensitive).
    - Exportar `EvaluateFTDBounds(paths []string, maxEffective int) FTDEvaluation` implementando la **Opción 5B**:
      - Si `maxEffective <= 0`, usar `DefaultMaxFTDFiles`. Si `len(paths) == 0`, devolver `WithinBounds: false`.
      - Normalizar y deduplicar rutas case-insensitively (`strings.ToLower(NormalizeCoverPath(p))`).
      - Identificar directorios con al menos un archivo productivo (basename que NO termine en `_test.go`).
      - Eximir `*_test.go` en un directorio presente en ese conjunto hasta un máximo de `MaxFTDExemptGoTests` (`5`); todo test `*_test.go` sin hermano productivo en el mismo directorio, todo test por encima del 5.º exento y todo archivo no `*_test.go` suman `1:1` en `EffectiveCount`.
      - Calcular `RootModules` sobre todos los archivos físicos incluidos los tests exentos (`<= 1` módulo raíz sin excepciones).
      - Fijar `WithinBounds = PhysicalCount > 0 && len(RootModules) <= MaxFTDRootModules && EffectiveCount <= maxEffective && ExemptTestCount <= MaxFTDExemptGoTests && PhysicalCount <= MaxFTDPhysicalFiles`.

#### 2. `pkg/bfcore/paths.go` (Modify)

*   **Type / Scope**: Go Library (`blackfactory/pkg/bfcore`).
*   **Proposed Changes**:
    - Definir constante `MaxObservedFileBytes = 10 << 20`.
    - Exportar `type PathSanitizeOpts struct { AllowDir bool; SplitCommas bool; FoldCaseDedupe bool }` (globs `*?[` y rutas personales `G4` rechazados incondicionalmente).
    - Exportar `SanitizeRelativePaths(rawPaths []string, repoRoot string, opts PathSanitizeOpts) (cleanPaths []string, rejectReason string)` unificando las validaciones pre-`path.Clean` de `1_Intake/engine/paths.go:20-118`:
      - Retorna siempre `cleanPaths` no-nil (`[]string{}`) y `rejectReason == ""` en éxito, o `"G4_personal_path_in_affected_paths"` / `"PATH_TRAVERSAL_OR_INVALID_SCOPE"` en rechazo.
      - Si `SplitCommas == true`, divide entradas por `,` y omite segmentos vacíos/blancos (paridad exacta con `1_Intake`); si `SplitCommas == false`, rechaza cualquier entrada vacía `""` con `"PATH_TRAVERSAL_OR_INVALID_SCOPE"`.
      - Rechaza rutas personales (`MatchesPersonalPathG4`), control bytes/NUL, `:`, prefijos absolutos/UNC/NT, segmentos `.`/`..`, `"."`/`"./"` (incluso si `AllowDir == true`), nombres reservados Windows `CON..LPT9` y globs `*?[`.
      - Registra si la entrada terminaba en `/` o `\` antes de `path.Clean` y re-añade `/` final únicamente cuando `AllowDir == true` (rechazando `/` final si `AllowDir == false`).
    - Exportar `CheckNoReparseUpToNearestExisting(rootPath, targetPath string) error`: sube desde `targetPath` hasta el ancestro existente más cercano dentro de `rootPath` (admitiendo `rootPath == ""` para caminar desde la raíz del volumen en `--task-root` externo) y aplica `CheckNoReparseChain`.
    - Exportar `ComputeNormalizedFileSHA256(repoRoot, relPath string) (state string, sha256Hex string, err error)`: ejecuta primero `SanitizeRelativePaths([]string{relPath}, repoRoot, PathSanitizeOpts{AllowDir: false, SplitCommas: false, FoldCaseDedupe: false})` para garantizar contención relativa dentro de `repoRoot`, luego verifica `CheckNoReparseChain`; devuelve `("absent", "", nil)` ante `os.ErrNotExist`; rechaza directorios o reparse points con error; sobre archivos regulares lee en streaming (tope `MaxObservedFileBytes`), elimina UTF-8 BOM (`\xef\xbb\xbf`), normaliza `\r\n` y `\r` a `\n`, y retorna `("present", hexSHA256, nil)`.

#### 3. `pkg/bfcore/brain_client.go` (Create)

*   **Type / Scope**: Go Library (`blackfactory/pkg/bfcore`, solo stdlib + primitivas de `bfcore`).
*   **Proposed Changes**:
    - Extraer el transporte read-only compartido desde `1_Intake/engine/brain_client.go` y los DTOs compartidos desde `1_Intake/engine/model.go:84-122` (`BrainPreflight`, `BrainContextResponse`, `BrainDocMatch`, `RiskHit`, `BrainInvocation`, interfaz `BrainClient` con `{Preflight, Context}`, `SubprocessBrainClient` con `NewSubprocessBrainClient`, `Preflight`, `Context` y método exportado `ExecBrain(args []string) ([]byte, BrainInvocation, error)`).
    - Exportar también `FindUncoveredPaths(paths []string, matchedDocs []BrainDocMatch) []string` (extraído de `1_Intake/engine/triage.go:254-289`) y `RiskLevelRank(level string) int` (extraído de `1_Intake/engine/model.go:324-329`) para evitar deriva entre `1_Intake` y `2_Spec_Writer`. Mantener los helpers exclusivos de triaje (`ExtractQueryTokens`, `BuildDisjunctiveFTSQuery`, `IsProjectActiveOnDisk`, `Query`, `BrainQueryOpts`) en `1_Intake/engine/brain_client.go`.
    - Definir sentinels independientes (`ErrBrainUnavailable = errors.New(...)`, `ErrBrainIndexMissing = errors.New(...)`, `ErrBrainJournalActive = errors.New(...)`) y en `Preflight` devolver errores multi-wrap (`fmt.Errorf("%w: %w: %s", ErrBrainUnavailable, ErrBrainJournalActive, detail)` y `fmt.Errorf("%w: %w: %s", ErrBrainUnavailable, ErrBrainIndexMissing, detail)`), de modo que `errors.Is` reconozca ambas identidades (`ErrBrainUnavailable` para `exit 4` en `1_Intake`, y `ErrBrainJournalActive` / `ErrBrainIndexMissing` para distinguir `BRAIN_JOURNAL_ACTIVE` vs. `BRAIN_UNAVAILABLE` en `exit 6` de `2_Spec_Writer`).

#### 4. `pkg/bfcore/bfcore_test.go` (Modify) & `pkg/bfcore/README.md` (Modify)

*   **Type / Scope**: Unit Tests & Package Docs (`pkg/bfcore`).
*   **Proposed Changes**:
    - Añadir pruebas TDD en `bfcore_test.go`: `EvaluateFTDBounds` (5 productivos + 5 tests Go hermanos = 10 PASS; 5 productivos + 6 tests Go = FAIL; test Go huérfano sin hermano = `1:1`; tests Python/TS = `1:1`; test Go en segundo módulo raíz = FAIL; archivos raíz `.` + subdirectorio = 2 módulos; `maxEffective <= 0` default; `len(paths) == 0` → `WithinBounds: false`), `SanitizeRelativePaths` (combinaciones `AllowDir`/`SplitCommas`/`FoldCaseDedupe`, rechazo `"."`, rechazo `""` con `SplitCommas: false`, preservación `/` final), `ComputeNormalizedFileSHA256` (CRLF vs LF con mismo hash, BOM strip, `"absent"` en inexistente, rechazo de `../` fuera de `repoRoot`, error en directorio), `CheckNoReparseUpToNearestExisting` (incl. `rootPath == ""`), `FindUncoveredPaths`, `RiskLevelRank`, y `SubprocessBrainClient.Preflight` verificando ambas identidades con `errors.Is` (`ErrBrainJournalActive` + `ErrBrainUnavailable` y `!ErrBrainIndexMissing`; `ErrBrainIndexMissing` + `ErrBrainUnavailable` y `!ErrBrainJournalActive`).
    - Actualizar `pkg/bfcore/README.md` documentando los nuevos símbolos e invariantes.

#### 5. `1_Intake/engine/paths.go`, `brain_client.go`, `triage.go`, `model.go`, `intake_test.go` y `README.md` (Modify)

*   **Type / Scope**: Go Library, Unit Tests & Docs (`blackfactory/intake`).
*   **Proposed Changes**:
    - `1_Intake/engine/paths.go`: delegar `SanitizeAffectedPaths` a `bfcore.SanitizeRelativePaths(..., PathSanitizeOpts{AllowDir: false, SplitCommas: true, FoldCaseDedupe: false})` y `CountRootModules` a `bfcore.CountRootModules`.
    - `1_Intake/engine/brain_client.go` & `model.go`: re-exportar alias de DTOs/constantes/errores (`BrainPreflight`, `BrainContextResponse`, `BrainDocMatch`, `RiskHit`, `BrainInvocation`, `ErrBrainUnavailable`, `ErrBrainIndexMissing`, `DefaultMaxFTDFiles = bfcore.DefaultMaxFTDFiles`, delegando `RiskLevelRank` a `bfcore.RiskLevelRank`), definir `intake.BrainClient` embebiendo `bfcore.BrainClient` + `Query(...)`, y envolver `type SubprocessBrainClient struct { *bfcore.SubprocessBrainClient }` (con `NewSubprocessBrainClient` envolviendo `bfcore.NewSubprocessBrainClient`) para implementar el método local `Query(...)` vía `c.ExecBrain(args)` junto a `ExtractQueryTokens`/`BuildDisjunctiveFTSQuery`/`IsProjectActiveOnDisk`, conservando 100% intacta la serialización JSON de `WorkItem` (`workitem.schema.json` v1.0 sin cambios) y `sanitized_payload_sha256`.
    - `1_Intake/engine/triage.go`: reemplazar la comprobación inline (`L211-220`) por `bfcore.EvaluateFTDBounds(wi.AffectedPaths, maxFTDFiles)` (Opción `5B`), delegar `findUncoveredPaths` a `bfcore.FindUncoveredPaths` y actualizar el texto de detalle (`L218, L243`) para informar `EffectiveCount`/`PhysicalCount`.
    - `1_Intake/engine/intake_test.go`: añadir casos TDD para Opción `5B` en `TriageWorkItem` (5 productivos + 2 `*_test.go` hermanos → `FTD`; 5 productivos + 6 `*_test.go` → `ASDD`; `test_app.py` → `1:1`; test Go en segundo módulo → `ASDD`), verificando 0 regresiones en toda la suite previa (`L326-356`).
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
    - `2_Spec_Writer/README.md`: reemplazar el esqueleto de 5 líneas con la documentación del Equipo 2 (Overview, límites, arquitectura en tres fases, tabla comparada de códigos `0..6` vs `1_Intake 0..5`, matriz CLI, invariantes `G1–G6`/`Opción i`, efectos secundarios y comandos de verificación).
    - `2_Spec_Writer/schemas/spec.schema.json`: único archivo canónico de esquema JSON Draft 2020-12 (`schema_version: "1.0"`, `additionalProperties: false` en todos los niveles, cero `extensions`) cubriendo todos los campos de la Sección 3 del Plan Técnico, condicionales `if/then` para `FTD` vs `ASDD`, `observed_files[]` (`state: "present" | "absent"`), `decisions[]` con `approved_paths[]` + `scope_sha256`, `risk_mitigations[].review_gates[]` (`string[]`), `verify_steps[]` (`expect_exit: 0`, `timeout_sec: 1..300`) y `failure_proof` (`expect_exit: 1..255`).
    - `2_Spec_Writer/skills/README.md` & `2_Spec_Writer/skills/spec-authoring/SKILL.md`: guía progresiva del bucle operativo para agentes externos (`prepare → editar spec.json → validate --stage draft → [ASDD Goldfish + validate --stage final] → render → compile → [AUTOR] gengis.exe plan`), reglas de `observed_files[]`, `decisions[]`, generación de `goldfish_review.json` y recuperación ante exits `1..6`.

#### 8. `2_Spec_Writer/engine/go.mod`, `go.sum`, `README.md`, `embed.go` y `templates/{index,chunk,status}.md.tmpl` (Create)

*   **Type / Scope**: Go Module Configuration (`blackfactory/specwriter`, `CGO_ENABLED=0`, sin SQLite) & Embedded Templates (`go:embed`).
*   **Proposed Changes**:
    - `go.mod` / `go.sum`: `module blackfactory/specwriter`, `go 1.26.0`, requiriendo únicamente `blackfactory/pkg/bfcore` (`replace => ../../pkg/bfcore`) y `golang.org/x/sys v0.49.0`.
    - `embed.go`: embeber **exclusivamente** `//go:embed templates/*.md.tmpl` (sin copiar `spec.schema.json`; `validate.go` aplica el esquema vía `json.Decoder.DisallowUnknownFields()` + comprobaciones programáticas, con paridad verificada en `specwriter_test.go`, patrón idéntico a `1_Intake/engine`).
    - Plantillas deterministas `GengiSWAP` v1 usando exclusivamente encabezados `##` H2 (`inCoreH2 = true` en `bridge.go:458`), cero bloques Mermaid, y encabezados exactos:
      - `templates/index.md.tmpl`: `# Mapa de Tarea: <task_id> — <title>`, línea 2 `Plantilla: <template> v1`, `**Objetivo:**` (prefijado con `[HITL]`/`[CRITICO]`/`[ESCALATED]` si aplica + `<narrative.problem>`), `## Directorio de Destino Real` (`- **Repositorio destino:** <TARGET_REPO>` y `- **Rama destino:** <target_branch>` sin backticks), `## Alcance` (`- Puede modificar:`, `- Solo lectura:`, `- Prohibido:` con rutas exclusivamente en backticks sin `/` en prosa, `- Mutación de datos (G3):`), `## Supuestos y Stack Tecnológico` (`- **Máx. intentos por chunk:** 3`), `## Contratos entre Pasos`, `## Definición de Terminado (DoD)`, `## Notas Post-Merge y Trazabilidad` (`spec_sha256`, `trace_id`, `brain_followups[]`).
      - `templates/chunk.md.tmpl`: `# Chunk <NN>: <title>`, `## Objetivo` (1.ª línea prefijada con etiquetas `[HITL]`/`[CRITICO]`/`[ESCALATED]`/`[EXPANSIONS:N]` si aplica), `## Entradas`, **`## Puede modificar`** (rutas en backticks de `can_modify`, poblando `Chunk.Scope.CanModify` en `bridge.go:418-419, 523`), `## Fuera de Alcance` (solo rutas en backticks, excluyendo rutas del propio chunk), `## Criterios de Aceptación` (viñetas `- `), `## Comando de Verificación` (bloque fenced `cmd`), `## Prueba de que puede fallar` (separada estrictamente de `Comando de Verificación`).
      - `templates/status.md.tmpl`: estado inicial de chunks `[ ] pendiente` con `spec_sha256` y `trace_id`.
    - `2_Spec_Writer/engine/README.md`: índice de símbolos exportados, invariantes, efectos secundarios y comando de verificación.

#### 9. `2_Spec_Writer/engine/model.go` (Create)

*   **Type / Scope**: Go Domain Model, Closed Reason Catalog & Canonical Hashing (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Constantes de salida (`ExitOK = 0`, `ExitFixable = 1`, `ExitUsage = 2`, `ExitG4 = 3`, `ExitExternalDecision = 4`, `ExitAttemptsBlocked = 5`, `ExitInfraOrRefusal = 6`), límites (`MaxValidateAttemptsPerStage = 3`, `MaxExternalCycles = 3`, `MaxVerifyTimeoutSec = 300`, `MaxWorkUnitsASDD = 99`, `MaxSpecBytes = 1 << 20`, `MaxGoldfishBytes = 256 << 10`, `MaxObservedFileBytes = bfcore.MaxObservedFileBytes`).
    - Catálogo cerrado de constantes `Reason*` y función `IsRetryableReason(code int, reason string) bool`:
      - Exit `0`: `OK` (`false`)
      - Exit `1`: `VALIDATION_FAILED` (`true`), `INVALID_JSON` (`true`), `SPEC_TOO_LARGE` (`true`), `VALIDATION_REQUIRED` (`true`), `VALIDATOR_BYPASS_DEFECT` (`false`)
      - Exit `2`: `INVALID_USAGE` (`false`), `SPEC_ALREADY_EXISTS` (`false`)
      - Exit `3`: `G4_VIOLATION` (`false`), `G4_VIOLATION_ON_DISK` (`false`)
      - Exit `4`: `STALE_CONTEXT` (`false`), `ESCALATION_REQUIRED` (`false`), `LANE_UPGRADE_REQUIRED` (`false`)
      - Exit `5`: `MAX_VALIDATE_ATTEMPTS` (`false`), `MAX_EXTERNAL_CYCLES` (`false`)
      - Exit `6`: `BRAIN_UNAVAILABLE` (`true`), `BRAIN_JOURNAL_ACTIVE` (`true`), `LOCK_BUSY` (`true`), `IO_ERROR` (`true`); `LIVE_TASK_STATE` (`false`), `TASK_DIR_EXISTS` (`false`), `ATTEMPTS_STATE_MISSING` (`false`), `ATTEMPTS_STATE_CORRUPT` (`false`), `ARTIFACT_STATE_CORRUPT` (`false`), `REPARSE_POINT_DETECTED` (`false`), `GITIGNORE_DRIFT` (`false`), `GENGIS_ENGINE_DRIFT` (`false`).
    - Structs con tags JSON exactos: `Spec`, `WorkItemRef` (`source_workitem_path` relativo a `--factory-root`), `Narrative`, `ObservedFile`, `G5Citation`, `DecisionRecord`, `RiskMitigation` (`ReviewGates []string`), `BrainFollowup`, `WorkUnit`, `ScopeExpansion`, `AcceptanceCriterion`, `VerifyStep`, `FailureProof`, `SpecContext` (`source_workitem_path`, `repo_root_basename`, `repo_root_sha256`, `brain_snapshot.matched_doc_hashes[]` con `rel_path` relativo a `--vault`), `AttemptsState`, `GoldfishReview`, `ValidationIssue{Pointer, Code, Message}`, `RenderedChunk{Filename, Content string}`, `RenderedTaskFiles{IndexMD, StatusMD string; Chunks []RenderedChunk}`, `PrepareOptions`, `VerifyOptions`, `SpecWriterEvent{Timestamp, Event, TraceID, Project, WorkItemID, Stage, ExitCode, Reason, Retryable, SpecSHA256, DurationMS, Detail}`, `CLIResult{OK, ExitCode, Reason, Retryable, TraceID, SpecSHA256, LastExit, LastReason, TaskDir, Markdown, Issues}` (con tags `omitempty` en los campos específicos de subcomando `LastExit`, `LastReason`, `TaskDir`, `Markdown`, `Issues`), `PrepareResult{OK, ExitCode, Reason, Retryable, TraceID, WorkItemID, SpecPath, ContextPath, AttemptsPath, Refreshed, Reset}`, y `VerifyReport{OK, ExitCode, Reason, Retryable, GitIgnoreOK [json:"gitignore_ok"], GengisHashesOK [json:"gengis_hashes_ok"], BrainOK [json:"brain_ok"], G4Issues [json:"g4_issues"], ArtifactsChecked [json:"artifacts_checked"], Issues}`.
    - Exportar `CanonicalSpecSHA256(s Spec) (string, error)` (normalizando slices `nil -> []` y `VerifyStep.Env == nil -> map[string]string{}`, serialización compacta determinista sin indentación/CRLF) y `ComputeScopeSHA256(paths []string) string`.

#### 10. `2_Spec_Writer/engine/state.go` (Create)

*   **Type / Scope**: Go Runtime Persistence & Concurrency (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Gestionar rutas de runtime `runtime/<project>/<WI-id>/` con `CheckNoReparseChain`, cerrojo granular `.wi.lock` (`bfcore.AcquireFileLock`, backoff acotado 250 ms) y cerrojo breve `.events.lock` para appends en `runtime/specwriter_events.jsonl`, respetando orden estricto `.wi.lock → .compile.lock → .events.lock`.
    - Implementar `WriteJSONAtomicG4(path string, v any) error`: serializa, verifica `len(bfcore.ScanG4All(bytes)) == 0` antes de escribir, escribe temporal hermano + `Sync` + reemplazo atómico con reintentos Windows.
    - Implementar `LoadAttemptsStrict(wiDir string) (AttemptsState, error)` (fail-closed `ATTEMPTS_STATE_MISSING` / `ATTEMPTS_STATE_CORRUPT`) y `ReadFileWithShortRetry(path string) ([]byte, error)` (3×25 ms, exclusivamente solo lectura para `verify`).
    - Implementar `AppendEvent(dataDir string, ev SpecWriterEvent) error`: aplica `bfcore.RedactAllG4` sobre todos los campos de texto libre (`Detail`, `Reason`), verifica `len(bfcore.ScanG4All(line)) == 0` en memoria y escribe atómicamente bajo `.events.lock` propagando `trace_id`.

#### 11. `2_Spec_Writer/engine/verify.go` (Create)

*   **Type / Scope**: Go Read-Only Verification & Shared Preflight (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Fijar constantes `PinnedBridgeGoSHA256` y `PinnedRingGoSHA256` calculadas sobre `GengiSWAP/engine/{bridge,ring}.go`.
    - Separar explícitamente `FactoryRoot` (`--factory-root`, default `RepoRoot`) y `RepoRoot` (`--repo-root`, default `.`).
    - Exportar comprobaciones compartidas (`prepare`, `validate`, `compile`, `verify`):
      - `CheckReparseSafety(paths ...string) error` → `REPARSE_POINT_DETECTED` (`exit 6`, `retryable: false`).
      - `CheckBrainPreflight(client bfcore.BrainClient, vaultRoot, dbPath string) (bfcore.BrainPreflight, string, error)` → distingue `BRAIN_JOURNAL_ACTIVE` vs `BRAIN_UNAVAILABLE` (`exit 6`, `retryable: true`).
    - Exportar comprobaciones exclusivas de `specwriter verify` (100% solo lectura, 0 cerrojos exclusivos, 0 creación de archivos):
      - `CheckGitIgnore(factoryRoot string) error`: dos invocaciones separadas (`git -C <factoryRoot> check-ignore -q 2_Spec_Writer/runtime/` y `git -C <factoryRoot> check-ignore -q 2_Spec_Writer/specwriter.exe`), exigiendo `exit 0` en cada una → `GITIGNORE_DRIFT` (`exit 6`, `retryable: false`).
      - `CheckGengisEngineHashes(factoryRoot string) error`: compara `ComputeNormalizedFileSHA256` de `GengiSWAP/engine/bridge.go` y `ring.go` contra las constantes ancladas → `GENGIS_ENGINE_DRIFT` (`exit 6`, `retryable: false`).
      - `VerifyRuntime(opts VerifyOptions) (VerifyReport, int)`: tras `CheckReparseSafety`, evalúa todos los checks restantes para poblar `VerifyReport` (escaneo `ScanG4All` y sintaxis JSON sobre archivos existentes bajo `dataDir`; si `dataDir` no existe en checkout limpio, pasa con `ok: true`, `artifacts_checked: 0`, `g4_issues: 0`), y selecciona `(ExitCode, Reason)` con prioridad determinista fija: `REPARSE_POINT_DETECTED` (`6`) > `G4_VIOLATION_ON_DISK` (`3`) > `GITIGNORE_DRIFT` (`6`) > `GENGIS_ENGINE_DRIFT` (`6`) > `ATTEMPTS_STATE_CORRUPT` (`6`) > `ARTIFACT_STATE_CORRUPT` (`6`) > `BRAIN_JOURNAL_ACTIVE` (`6`) > `BRAIN_UNAVAILABLE` (`6`) > `IO_ERROR` (`6`).

#### 12. `2_Spec_Writer/engine/prepare.go` (Create)

*   **Type / Scope**: Go Preparation & Context Freezing (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Exportar `Prepare(opts PrepareOptions) (PrepareResult, int)`:
      - Validar `WorkItem` de entrada: `ScanG4All` raw+decodificado (`exit 3`, `G4_VIOLATION`); `schema_version == "1.0"`, `status == "ready"`, `lane in {"FTD","ASDD"}` y rutas `affected_paths` limpias (`exit 2`, `INVALID_USAGE` si es inválido o `needs_clarification`).
      - Ejecutar preflight compartido (`CheckReparseSafety` + `CheckBrainPreflight`) y `brain.Context` sobre `WorkItem.AffectedPaths` filtrando `status == "active"`; calcular `matched_doc_hashes[]` y `risk_zones_sha256`.
      - Calcular `repo_root_basename` + `repo_root_sha256` (sobre ruta absoluta normalizada de `--repo-root`, nunca persistiendo la ruta absoluta) y `source_workitem_path` relativo a `--factory-root` (o basename si queda fuera).
      - Bajo `.wi.lock`:
        - **WorkItem nuevo:** crear `spec_context.json` (`source_workitem_path` relativo a `--factory-root`, `baseline_hashes[]` con `ComputeNormalizedFileSHA256` sobre `WorkItem.AffectedPaths`), inicializar `attempts.json` en `0` y escribir esqueleto inicial `spec.json` (`schema_version: "1.0"`, `work_item_ref`, `project`, `lane`, `template: WorkItem.SuggestedTemplate`, `target_branch` detectada de `git rev-parse --abbrev-ref HEAD` o `""`, `title: WorkItem.Title`, `g3_data_mutation: ""`, `requires_autor_hitl: WorkItem.RequiresAutorHITL`, `narrative: {alternatives_considered: [], edge_cases: []}`, `observed_files[]` pre-poblado **exclusivamente desde `WorkItem.AffectedPaths`** —nunca docs de bóveda—, y slices vacíos `[]` para `stack_assumptions`, `read_only_paths`, `global_forbidden`, `dod`, `g5_citations`, `decisions`, `risk_mitigations`, `brain_followups`, `work_units`).
        - **WorkItem ya existente sin flags:** fallar cerrado (`exit 2`, `SPEC_ALREADY_EXISTS`) sin sobrescribir `spec.json` ni tocar contadores.
        - **`--refresh-context`:** actualizar únicamente `spec_context.json`, invalidar `last_passed_*` en `attempts.json` sin sobrescribir `spec.json` ni reiniciar contadores, y emitir `CONTEXT_REFRESHED`.
        - **`--reset-attempts --reason "<motivo>"`:** exigir `--reason` (`len >= 8`, `exit 2` si falta, `exit 3` si dispara `ScanG4All`), reiniciar `draft_attempts`, `final_attempts`, `external_cycles` a `0`, limpiar `last_passed_*` sin sobrescribir `spec.json`, y emitir `ATTEMPTS_RESET`.

#### 13. `2_Spec_Writer/engine/validate.go` (Create)

*   **Type / Scope**: Go Static Validation Engine (`blackfactory/specwriter`, 0 ejecución de comandos).
*   **Proposed Changes**:
    - Implementar la precedencia estricta `2 → 5 → 3 → 6 → 4 → 1` manteniendo `.wi.lock` durante `[5]..[0]`:
      - **`[2]`:** verificar `os.Lstat(--spec)` (`os.ErrNotExist` → `exit 2` `INVALID_USAGE`; otro error OS → `exit 6` `IO_ERROR`) y `CheckNoReparseChain` previo sobre `dataDir`/`wiDir`/`specPath` (`exit 6` `REPARSE_POINT_DETECTED`).
      - **`[5]`:** `LoadAttemptsStrict` (`exit 6` si falta/corrupto); seleccionar contador por `--stage` sin decodificar `spec.json`; si `stage_attempts >= 3` → `exit 5` (`MAX_VALIDATE_ATTEMPTS` + `last_exit`/`last_reason`).
      - **`[3]`:** verificar cuotas `MaxSpecBytes` (`1 MiB`) y, si `--stage final` y `goldfish_review.json` presente, `MaxGoldfishBytes` (`256 KiB`) — ante exceso, incrementar `stage_attempts` y devolver `exit 1` (`SPEC_TOO_LARGE`, `retryable: true`); Fase-A `ScanG4All` sobre bytes crudos de `spec.json` (y `goldfish_review.json` si `--stage final` y presente; si ausente `os.ErrNotExist`, continuar hacia `[1]`); parsear JSON para Fase-B — si sintaxis JSON inválida tras pasar Fase-A, incrementar `stage_attempts` y devolver `exit 1` (`INVALID_JSON`, `retryable: true`) sin entrar a `[4]`; Fase-B `ScanG4All` sobre todos los strings decodificados (ante hallazgo, incrementar `stage_attempts`, emitir `REJECTED_G4` y devolver `exit 3` `G4_VIOLATION`).
      - **`[6]`:** `CheckReparseSafety` + `CheckBrainPreflight` (`exit 6`).
      - **`[4]` (orden fijo `STALE_CONTEXT → ESCALATION_REQUIRED → LANE_UPGRADE_REQUIRED`; si dispara y `external_cycles >= 3` → `exit 5` `MAX_EXTERNAL_CYCLES`; si `< 3` → incrementar y `exit 4`):**
        - Pre-saneamiento en memoria (sin I/O): pasar `spec.observed_files[].path` y `can_modify[]` por `SanitizeRelativePaths(..., AllowDir: false, SplitCommas: false, FoldCaseDedupe: true)`; las entradas inválidas en `observed_files[]` no se pasan a `ComputeNormalizedFileSHA256` (se difieren a `[1]`); si `len(can_modify) == 0` o `can_modify` falla higiene, `[4]` evalúa únicamente `STALE_CONTEXT` sobre `spec_context.json` (consultando `brain.Context` solo con `spec_context.work_item.affected_paths`) y omite `ESCALATION_REQUIRED` y `LANE_UPGRADE_REQUIRED` para que `[1]` devuelva `exit 1` (`VALIDATION_FAILED`) sin consumir `external_cycles`.
        - `STALE_CONTEXT`: verificar `repo_root_sha256`, `risk_zones_sha256`, `matched_doc_hashes[]`, `baseline_hashes[]` y cada entrada saneada de `spec.observed_files[]` contra disco (`"present"` mantiene hash CRLF/BOM; `"absent"` sigue ausente).
        - Re-consultar `0_Brain` `context --files` sobre las rutas concretas saneadas de `can_modify` filtrando `status == "active"`:
        - `ESCALATION_REQUIRED`: sube `risk_level` vs baseline, nueva zona `CRITICO`, o `len(RootModules) > 1` cuando (`spec_context.work_item.lane == "FTD"` o se añade un módulo raíz fuera de `affected_paths`), salvo que la decisión activa (`decisions[len(decisions)-1]`) sea válida (`SanitizeRelativePaths(approved_paths)` y `ComputeScopeSHA256(approved_paths) == decision.scope_sha256 == current_scope_sha256`, exigiendo igualdad exacta de conjuntos normalizados).
        - `LANE_UPGRADE_REQUIRED`: `spec.lane == "FTD"` y dispara cualquier regla `ASDD` de `1_Intake` (`kind in {"feature","ops"}`, `len(impact_flags) > 0`, `CRITICO`, `risk_zones_parse_error != ""` devuelto en vivo por `0_Brain context --json` o `RISK_ZONES_UNPARSEABLE_FAILSAFE` en `spec_context.work_item.triage_reasons[]`, `!bfcore.EvaluateFTDBounds.WithinBounds`, o ruta sin cobertura activa `wiki`/`adr` vía `bfcore.FindUncoveredPaths`).
      - **`[1]` — Exportar `ValidateSpecAndRenderCheck(specBytes []byte, spec Spec, ctx SpecContext, wiDir, repoRoot, stage string, reQueriedRiskHits []bfcore.RiskHit) ([]ValidationIssue, RenderedTaskFiles, error)` (compartida con `compile.go`; si falla en `validate`, incrementa `stage_attempts` y devuelve `exit 1` `VALIDATION_FAILED`):**
        - Decodificación estricta `DisallowUnknownFields()`, verificación explícita de `work_item_ref` (`id`, `revision`, `sanitized_payload_sha256` contra `ctx.WorkItem`; `trace_id` contra `ctx.TraceID`; `source_workitem_path` contra `ctx.SourceWorkItemPath`, `/work_item_ref`), completitud de `observed_files[]` (`affected_paths ∪ g5_citations[].path ∪ can_modify`, `/observed_files`), obligatoriedad `FTD` vs `ASDD`, bloqueo `ctx.WorkItem.Lane == "ASDD" && spec.Lane != "ASDD"` (`/lane`), `decisions[]` obligatorio cuando `ctx.WorkItem.Lane == "FTD" && spec.Lane == "ASDD"` (`/decisions`) y, siempre que `len(spec.Decisions) > 0`, verificación de que la decisión activa (`spec.Decisions[len(spec.Decisions)-1]`) satisface `ComputeScopeSHA256(approved_paths) == decision.scope_sha256 == current_scope_sha256` (`/decisions`), `1..2` `work_units` en `FTD` y `1..99` en `ASDD`.
        - DAG acíclico en `depends_on`, `can_modify` rutas de archivo concretas (`AllowDir: false`), rechazo del carácter `` ` `` en toda ruta de alcance/archivo (`can_modify`, `read_only_paths`, `global_forbidden`, `forbidden`, `observed_files`, `g5_citations`) para evitar cierre prematuro de `reBackticked` (`bridge.go:21`), verificación de capitalización exacta de segmentos en disco para rutas `"present"` en Windows NTFS (paridad con `ring.go:101-124`), `can_modify` disjunto entre unidades sin dependencia transitiva y estrictamente no sombreado por `read_only_paths`, `global_forbidden` ni `work_units[].forbidden`; `scope_expansions[]` obligatorio para toda ruta en `can_modify \ affected_paths`.
        - `G2` (cero bloques Mermaid), `G5` (existencia, 1-based CRLF/BOM, ancla literal sensible a mayúsculas `±5` líneas en todo archivo de texto UTF-8 y código —extensiones `.go`, `.py`, `.js`, `.ts`, `.sql`, `.sh`, `.ps1`, `.md`, `.json` y cualquier otro archivo de texto plano—, rechazando tokens `len < 4` o palabras reservadas `err`, `func`, `return`, `if`, `for`, `nil`, `true`, `false`).
        - `work_units[].verify_steps[].id` y `work_units[].acceptance_criteria[].id` únicos globalmente; `risk_mitigations[]` cubriendo 100% de `ctx.WorkItem.RiskHits ∪ reQueriedRiskHits` con `review_gates[]` referenciando `acceptance_criteria[].id` (`kind == "review_gate"`), `requires_autor_hitl` unidireccional, `CRITICO` con `>= 1` `review_gate` y ejecutor cerrado (`4_Revisores` | `Modo_QA` | `AUTOR`), `>= 1` `verify_step` válido por unidad.
        - **Allowlist cerrado de `verify_steps[]`** (tabla de Sección 6: ejecutables, subcomandos, banderas permitidas por subcomando —incl. `brain.exe query --q`, exclusión de `brain.exe verify` e `intake.exe verify`, `-run=` con `^[A-Za-z0-9_$./-]+$` sin `^`—, `cwd == ""` obligatorio para binarios Black Factory, valores `env` validados sin `GENGIS_BIN`, rechazo de caracteres inseguros incl. `` ` `` final, `expect_exit == 0`, `timeout_sec in 1..300`).
        - Renderizado en memoria de `index.md` (`<TARGET_REPO>`), `chunks/NN_<id>.md` (`## Puede modificar`, etiquetas HITL) y `status.md`, ejecutando el superconjunto de `validateChunks` (`ring.go:393-426`) y las guardas `parseIndexFile`/`parseChunkFile` (`bridge.go:115-158, 196-212, 431, 446, 450-474, 527-528`): prohibir `\r`/`\n` en todos los campos de línea única (`title`, `target_branch`, `g3_data_mutation`, `work_units[].title`, `work_units[].inputs[]`, `work_units[].acceptance_criteria[].description`), prohibir `^#{1,6}\s`, fences ```` ``` ````/`~~~`, `reMaxRetries` y claves de alcance `parseFieldLabel` en la prosa de `index.md` (`narrative.problem`, `stack_assumptions[]`, `dod[]`, `brain_followups[]`), prohibir `^#{1,6}\s` y fences en `objective`/`failure_proof`, prohibir `parseFieldLabel` (`classifyChunkField != ""`) en líneas sin viñeta de `objective`/`failure_proof` y en todo ítem de `inputs[]`.
        - En `--stage final` (`ASDD`): validar `goldfish_review.json` (si falta `os.ErrNotExist` → `exit 1` `VALIDATION_FAILED`; `spec_sha256` coincidente, `unresolved_blockers == 0`, lentes base + condicionales presentes, `report_path` relativo a `--factory-root` limpio bajo `G4`).
      - **`[0]`:** persistir `(last_passed_sha256, last_passed_stage, last_passed_goldfish_sha256, last_exit=0, last_reason="OK")` en `attempts.json` (sin resetear `final_attempts` al pasar `draft`) y emitir `VALIDATE_PASS`.

#### 14. `2_Spec_Writer/engine/render.go` (Create)

*   **Type / Scope**: Go Deterministic Markdown Renderer (`blackfactory/specwriter`, stdout-only, 0 escrituras en disco).
*   **Proposed Changes**:
    - Exportar `RenderSpecMarkdown(specBytes []byte) (string, Spec, int, error)`: ejecuta `MaxSpecBytes` (`SPEC_TOO_LARGE` → `exit 1`), `ScanG4All` raw (`G4_VIOLATION` → `exit 3`), parseo JSON (`INVALID_JSON` → `exit 1`), `ScanG4All` decodificado (`exit 3`), verifica `spec.SchemaVersion == "1.0"` (`VALIDATION_FAILED` → `exit 1`), genera Markdown determinista (cabecera `AUTO-GENERATED DERIVED VIEW: DO NOT EDIT` con `spec_sha256` y `trace_id`, secciones `FTD`/`ASDD`, tabla de `work_units`, `risk_mitigations`, `decisions`, `g5_citations`, `brain_followups` y diagrama de dependencias ASCII/Unicode Box-Drawing, cero Mermaid) y vuelve a ejecutar `ScanG4All` sobre la salida renderizada (`exit 3` si halla fuga).

#### 15. `2_Spec_Writer/engine/compile.go` (Create)

*   **Type / Scope**: Go Deterministic `GengiSWAP` Adapter (`blackfactory/specwriter`).
*   **Proposed Changes**:
    - Implementar la serialización determinista Windows `cmd.exe` de `verify_steps` (claves `env` ordenadas, `cd /d "<filepath.FromSlash(cwd)>" &&` siempre entrecomillado si `cwd != ""`, `filepath.FromSlash(argv[0])`, comillas en `argv[k]` con espacio o `,`).
    - Implementar la precedencia `2 → 5 → 3 → 6 → 4 → 1` manteniendo `.wi.lock` durante `[5]..[0]` y respetando orden `.wi.lock → .compile.lock → .events.lock`:
      - `[2]`: validar `os.Lstat(--spec)` (`INVALID_USAGE` → `exit 2`; otro error OS → `exit 6` `IO_ERROR`), `CheckNoReparseChain` sobre `dataDir`/`wiDir`/`specPath` (`REPARSE_POINT_DETECTED` → `exit 6`) y `--task-id` (`^[a-z0-9][a-z0-9_-]{2,63}$` sin reservados Windows).
      - `[5]`: `LoadAttemptsStrict`; rechazar con `exit 5` solo si `draft_attempts >= 3 || final_attempts >= 3` (`MAX_VALIDATE_ATTEMPTS`) o si `external_cycles >= 3 && (last_passed_sha256 == "" || last_exit == 4)` (`MAX_EXTERNAL_CYCLES`).
      - `[3]`: comprobar `MaxSpecBytes` / `MaxGoldfishBytes` (`SPEC_TOO_LARGE` → `exit 1`, sin consumir intento), luego `G4` fail-closed sobre `spec.json` y, si `ASDD` y presente, `goldfish_review.json` (`INVALID_JSON` → `exit 1` si sintaxis rota tras Fase-A).
      - `[6]`: `CheckReparseSafety`, `CheckBrainPreflight`, `ScanTier1` sobre `--repo-root`, `CheckNoReparseUpToNearestExisting("", taskRoot)` + `os.MkdirAll(taskRoot, 0o755)` + `CheckNoReparseChain` sobre `<task-root>` y `<task-root>/.compile.lock`, adquirir `<task-root>/.compile.lock` (`LOCK_BUSY`), verificar destino *create-only* bajo `CheckNoReparseChain` + `os.Lstat` fail-closed: `<dest>/.ring/state.json` → `LIVE_TASK_STATE` (`exit 6`, `retryable: false`); `<dest>` existente sin `.ring/state.json` → `TASK_DIR_EXISTS` (`exit 6`, `retryable: false`).
      - `[4]`: revalidación de contexto/alcance en vivo **sin mutar `attempts.json`** (con el mismo pre-saneamiento de `observed_files[]`/`can_modify[]` de `validate`; si dispara una nueva decisión externa, devuelve `exit 5` si `external_cycles >= 3` o `exit 4` si `external_cycles < 3`).
      - `[1]`: verificar `(CanonicalSpecSHA256(spec) == last_passed_sha256)`, etapa requerida (`draft`/`final` en `FTD`, `final` en `ASDD`) y SHA-256 de `goldfish_review.json` en `ASDD` → si falla, `exit 1` (`VALIDATION_REQUIRED`, `retryable: true`). Invocar el validador compartido `ValidateSpecAndRenderCheck` (pasando `reQueriedRiskHits` de `[4]`) y ejecutar `ScanG4All` sobre los buffers renderizados en memoria (`index.md` con `<TARGET_REPO>`, `chunks/NN_<id>.md`, `status.md`); cualquier fallo aquí tras pase válido emite `exit 1` (`VALIDATOR_BYPASS_DEFECT`, `retryable: false`, sin consumir intento) registrado en `specwriter_events.jsonl`.
      - `[0]`: sustituir únicamente `- **Repositorio destino:** <TARGET_REPO>` en `index.md` por `filepath.Clean(absRepoRoot)`, crear staging hermano `<task-root>/.<task-id>.staging-<rand>` con `defer os.RemoveAll(stagingDir)`, ejecutar `Write` + `Sync` + `Close` sobre `index.md`, `chunks/NN_<id>.md` y `status.md` inicial, crear el directorio vacío `workspace/qa/`, re-comprobar `os.Lstat(<dest>)` (`os.ErrNotExist`), ejecutar un único `os.Rename(stagingDir, dest)` atómico (con reintento Windows 3×25 ms), registrar `COMPILED` en `specwriter_events.jsonl`, e imprimir el resumen informado y comando `gengis.exe plan` para `[AUTOR]`.

#### 16. `2_Spec_Writer/engine/specwriter_test.go` (Create) & `2_Spec_Writer/engine/contract_gengis_test.go` (Create)

*   **Type / Scope**: Unit Tests, Security Regression Tests, Latency Benchmark & Real `gengis.exe plan` Contract Tests (`t.TempDir()`, `G3`).
*   **Proposed Changes**:
    - `specwriter_test.go`: tests TDD cubriendo paridad de tags JSON de `model.go` contra `2_Spec_Writer/schemas/spec.schema.json`, cero rutas absolutas de host bajo `t.TempDir()` en Windows (`runtime/` limpio + frontera `G4` de dos zonas en `index.md`), `prepare` + `--refresh-context` + `--reset-attempts --reason` (G4 + `ATTEMPTS_RESET`), precedencia `2 → 5 → 3 → 6 → 4 → 1` (incluyendo `SPEC_TOO_LARGE` en `[3]` consumiendo intento en `validate`, `CheckNoReparseChain` previo a `.wi.lock`, JSON sintácticamente inválido tras Fase-A → `INVALID_JSON` sin consumir `external_cycles`, esqueleto `work_units: []` → `exit 1` `VALIDATION_FAILED` sin disparar falso `exit 4`, y `goldfish_review.json` ausente → `VALIDATION_FAILED`), presupuestos `draft`/`final` + resolución exitosa del 3.er ciclo externo compilable sin bloqueo permanente en `compile` + 4.º ciclo → `exit 5`, `ESCALATION_REQUIRED` sin falsos positivos en `ASDD` multi-módulo inicial, `decisions[]` con `scope_sha256` obsoleto tras ascenso `FTD → ASDD`, bloqueo `ASDD → FTD` y `CRITICO` en `FTD`, `observed_files[]` `"present"`/`"absent"` + CRLF/BOM + `matched_doc_hashes[]`, allowlist cerrado de comandos/banderas/`env` + serialización `cmd.exe` (`0_Brain\brain.exe query --q`, `go test -run=NoTestsMatch ./...`, `cd /d "..."`, `env` ordenado, rechazo `^`, `` ` `` final, `brain.exe verify`, `intake.exe verify`, `go build`, `-ldflags`, `-test.cpuprofile`, `GENGIS_BIN`, `gengis.exe status`), `## Puede modificar` + no sombreado `read_only`/`forbidden` + `depends_on` + guardas `parseIndexFile`/`parseChunkFile` (rechazo `\n` en `acceptance_criteria[].description`/`inputs[]`/`title`, rechazo `## Alcance` en `narrative.problem`/`dod`), anti-bypass `VALIDATION_REQUIRED` vs `VALIDATOR_BYPASS_DEFECT`, `.compile.lock` *create-only* (`LIVE_TASK_STATE` / `TASK_DIR_EXISTS`), `RedactAllG4` en `specwriter_events.jsonl`, `verify` solo lectura (checkout limpio sin `runtime/`, prioridad de códigos, caso `*.exe` ignorado pero `runtime/` no → `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT`, `ARTIFACT_STATE_CORRUPT`, `G4_VIOLATION_ON_DISK`) y `TestLatencyBudgetP95` (`< 500 ms` p95).
    - `contract_gengis_test.go`: invoca `gengis.exe plan <tmpTask> --root <tmpRoot>` con `"n\n"` sobre un repo `git init` en `t.TempDir()` (`GENGIS_BIN` desde entorno del operador/CI; `t.Skip` en local; `t.Fatalf` bajo `GENGIS_CONTRACT_STRICT=1`), cubriendo fixtures positivos (`FTD` con `failure_proof.expect_exit: 1` y `ASDD` multi-chunk verificando el resumen del plan en `stdout` con todos los chunks, etiquetas HITL y `TargetRepo` real sin `ADVERTENCIA: destino sin Git`, `.ring/state.json` en `PLAN_PENDING` con `Chunk.Scope.CanModify` poblado, y `"plan no aprobado por el Autor"` en `stderr`) y negativos de paridad (`FailureProof` vacío, `VerifyCommands` vacío, ID duplicado → `"chunk inválido"` sin crear `.ring/state.json`).

#### 17. `2_Spec_Writer/engine/cmd/specwriter/README.md`, `main.go` y `main_test.go` (Create)

*   **Type / Scope**: CLI Entry Point (`specwriter.exe`) & End-to-End Tests (`t.TempDir()`).
*   **Proposed Changes**:
    - `main.go`: implementa los 5 subcomandos (`prepare`, `validate`, `render`, `compile`, `verify`) respetando la Matriz de Subcomandos y Flags CLI de la Sección 5 (incluidos defaults y `--brain-bin`), devolviendo códigos `0..6` y salida JSON estructurada (`CLIResult`, `PrepareResult`, `VerifyReport`).
    - `main_test.go`: pruebas E2E reproduciendo `CUJ 1` (`FTD` 5 productivos + 2 tests Go hermanos), `CUJ 2` (`ESCALATION_REQUIRED` → `decisions[]` → `ASDD` `--stage final`), `CUJ 3` (`G4_VIOLATION` reincidente → `exit 5` → `--reset-attempts`, y `LIVE_TASK_STATE`/`TASK_DIR_EXISTS` `exit 6`), y `CUJ 4` (`verify` solo lectura con `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT` y archivo `"absent"` que aparece en disco).
    - `cmd/specwriter/README.md`: contrato de uso CLI, matriz de banderas/defaults, códigos `0..6` y verificación.

### Verification Plan

*   **Compilation / Build (`CGO_ENABLED=0`, cero SQLite en `pkg/bfcore`, `1_Intake` y `2_Spec_Writer`):**
    ```powershell
    $env:CGO_ENABLED = "0"
    Set-Location "C:\Black Factory\2_Spec_Writer\engine"
    go build -o ..\specwriter.exe .\cmd\specwriter
    ```
    Verificar que `pkg/bfcore/go.mod`, `1_Intake/engine/go.mod` y `2_Spec_Writer/engine/go.mod` no contienen `modernc.org/sqlite`. Si `C:\Black Factory\GengiSWAP\gengis.exe` no existiera en un checkout limpio, compilarlo previamente con `go build -o ..\gengis.exe .\cmd\gengis` desde `C:\Black Factory\GengiSWAP\engine`.
*   **Static Analysis & Linting (`go vet` en todos los módulos del workspace, sintaxis `cmd.exe` compatible con `GengiSWAP`):**
    ```cmd
    set CGO_ENABLED=0&& cd /d "C:\Black Factory\pkg\bfcore" && go vet ./... && cd /d "C:\Black Factory\0_Brain\engine" && go vet ./... && cd /d "C:\Black Factory\1_Intake\engine" && go vet ./... && cd /d "C:\Black Factory\2_Spec_Writer\engine" && go vet ./...
    ```
*   **Unit Tests, Coverage (`>= 85%`) y Contrato Obligatorio contra `gengis.exe plan` (`GENGIS_CONTRACT_STRICT=1`):**
    Antes de cualquier merge a `main`, la suite debe ejecutarse con **`GENGIS_CONTRACT_STRICT=1`** para garantizar que `contract_gengis_test.go` corra realmente contra `GengiSWAP/gengis.exe` (nunca `t.Skip`):
    ```cmd
    set CGO_ENABLED=0&& set GENGIS_CONTRACT_STRICT=1&& set GENGIS_BIN=C:\Black Factory\GengiSWAP\gengis.exe&& cd /d "C:\Black Factory\pkg\bfcore" && go test -count=1 -cover ./... && cd /d "C:\Black Factory\0_Brain\engine" && go test -count=1 ./... && cd /d "C:\Black Factory\1_Intake\engine" && go test -count=1 -cover ./... && cd /d "C:\Black Factory\2_Spec_Writer\engine" && go test -count=1 -cover ./...
    ```
    *Requisito de cobertura:* `>= 85%` de cobertura de sentencias en `blackfactory/pkg/bfcore`, `blackfactory/specwriter` y `blackfactory/specwriter/cmd/specwriter`, y cero regresiones en `0_Brain/engine` y `1_Intake/engine`.
*   **E2E / Health & Privacy Verification (`Opción i`):**
    ```powershell
    & "C:\Black Factory\2_Spec_Writer\specwriter.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\2_Spec_Writer\runtime" --json
    git -C "C:\Black Factory" check-ignore -v 2_Spec_Writer/runtime/
    git -C "C:\Black Factory" check-ignore -v 2_Spec_Writer/specwriter.exe
    git -C "C:\Black Factory" status --ignored -s 2_Spec_Writer/
    ```
    *Resultado esperado:* `specwriter.exe verify` `exit 0` (`ok: true`, `gitignore_ok: true`, `gengis_hashes_ok: true`, `brain_ok: true`, `g4_issues: 0`, `0` archivos creados en `runtime/`); ambas llamadas individuales a `git check-ignore -v` salen con código `0` (independientemente de si `runtime/` está vacío), y `git status --ignored -s 2_Spec_Writer/` confirma `!! 2_Spec_Writer/specwriter.exe` y cero artefactos rastreados bajo `runtime/`.
*   **Latency Budget Verification:**
    ```cmd
    set CGO_ENABLED=0&& cd /d "C:\Black Factory\2_Spec_Writer\engine" && go test -count=1 -v -run TestLatencyBudgetP95 ./...
    ```
    *Resultado esperado:* p95 `< 500 ms` local en Windows para `prepare`, `validate`, `render`, `compile` y `verify` (hasta `50` artefactos en `runtime/`).

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
