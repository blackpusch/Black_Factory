# Equipo 3 de Black Factory (`3_Builder`): Construcción TDD Aislada, Orquestación sobre `GengiSWAP` y Handoff a Revisores

*   **Author:** blackpusch
*   **Status:** Approved
*   **Date:** 2026-11-24

--------------------------------------------------------------------------------

## Overview

`3_Builder` (`3_Builder/engine` → `3_Builder/builder.exe`) es el motor determinista de construcción aislada y puente operativo hacia `GengiSWAP` y `4_Revisores` de **Black Factory**. Compilado como binario pure-Go autónomo (`CGO_ENABLED=0`, objetivo principal Windows con fallback POSIX, sin SQLite, sin llamadas HTTP a LLM y con `0` bytes modificados en `0_Brain/**`, `1_Intake/**`, `2_Spec_Writer/**`, `pkg/bfcore/**` y `GengiSWAP/engine/**`), reutiliza el paquete compartido `blackfactory/pkg/bfcore` (`golang.org/x/sys v0.49.0`) bajo la arquitectura híbrida **Opción 3C** y privacidad **Opción i**.

`3_Builder` recibe una carpeta de tarea `GengiSWAP` compilada por `2_Spec_Writer` (`C:\Desarrollo\GengiSWAP_data\gengis_task\<task_id>\` o equivalente del protocolo), crea una **rama dedicada `build/<task_id>` y un `git worktree` aislado** fuera del repositorio principal, vincula la línea `- **Repositorio destino:**` de `<taskDir>/index.md` hacia dicho worktree, impone consulta obligatoria de solo lectura a `0_Brain` (*Read-Before-Act*), exige evidencia literal `G1` del ciclo **TDD** (*Red → Green → Refactor*), aplica una puerta fail-closed `G4` (11 reglas) y validación de alcance sobre el diff completo, emite paquetes inmutables (`DeliveryBundle` v1.0) hacia `4_Revisores`, procesa las órdenes consolidadas (`BounceOrder` v1.0: `BOUNCE` | `APPROVE`) de `5_Cycle_Approver` con freno determinista (`MaxBounceCycles = 3`), y sella cada chunk aprobado invocando `GengiSWAP/gengis.exe check` como subproceso — preservando intactas las compuertas humanas `[AUTOR]` (`gengis.exe plan` y `gengis.exe decide`).

### Glosario Base y Convenciones del Repositorio

Esta especificación se apoya en [`README.md`](../../README.md), [`Docs/BLACK_FACTORY_BRAIN.md`](../BLACK_FACTORY_BRAIN.md), [`Docs/designs/1_intake.md`](1_intake.md), [`Docs/designs/2_spec_writer.md`](2_spec_writer.md) y `0_Brain/vault/constitution.md` *(archivo local privado `Opción i`)*:

*   **Separación Estricta Motor Go vs. Harness Externo (`Opción 3C` + `NG1`):** `builder.exe` jamás invoca APIs LLM ni codifica proveedores en Go; valida mecánicamente invariantes Git, TDD, `G4`, alcance y contratos JSON. El agente constructor (referencia conceptual: **Opus**), los 4 subagentes en contexto limpio de `4_Revisores` (Seguridad/**Sol**, UX/**Sol**, UI/**Astra**, Código-Arquitectura/**Sonnet**) y el consolidador `5_Cycle_Approver` (**Opus**) corren en el harness externo guiados por perfiles agnósticos declarados en `3_Builder/skills/`.
*   **Invariantes Globales (`G1–G6`), `Opción i` y `RZ-001`:**
    - **`G1`:** Evidencia literal capturada por el motor (comando, código de salida, duración y extracto real).
    - **`G2`:** Diagramas exclusivamente ASCII / Unicode Box-Drawing (cero bloques Mermaid).
    - **`G3`:** Pruebas sobre directorios y repositorios Git efímeros en `t.TempDir()` (incluyendo `--gengis-root` temporal para no mutar `GengiSWAP/gengis_global_log.md`).
    - **`G4`:** Cero secretos ni rutas personales mediante las **11 reglas** de `pkg/bfcore/g4.go:48-115`: fail-closed en las 11 reglas sobre rutas cambiadas, líneas añadidas `+`, archivos nuevos sin rastrear, órdenes JSON y todo artefacto en `3_Builder/runtime/`; frontera de dos zonas para el enlace externo de `<worktree_path>` en `<taskDir>/index.md` (`ScanTier1` sin secretos duros); y sanitización en salidas de comandos/contexto heredado de diffs (`ScanTier1` fail-closed primero, `RedactTier2Text` después).
    - **`G5`:** Citas `archivo:línea` verificadas directamente en disco.
    - **`G6`:** Parada explícita ante ambigüedad, deriva o precondición incumplida.
    - **`Opción i`:** `3_Builder/runtime/` y `3_Builder/builder.exe` excluidos en `.gitignore`.
    - **`RZ-001`:** `GengiSWAP/engine/` permanece 100% intacto en v1 (`0` bytes modificados; 5 archivos vigilados con SHA-256 normalizado en `builder verify`).

`3_Builder` expone ocho subcomandos deterministas (`init`, `context`, `tdd-red`, `package`, `apply-order`, `commit`, `cleanup`, `verify`):
1. **`init`:** rechaza sesiones duplicadas (`SESSION_ALREADY_EXISTS`, comprobado en seco y re-verificado dentro de `.session.lock`), valida `index.md`, `VerifyCommands` (contra `git ls-tree <baseRef40>`), rutas no solapadas, referencias Git (`git rev-parse --verify <ref>^{commit}`) y `0_Brain` (`Preflight` + `Context`) **antes** de mutar nada; bajo `.repo-<sha16>.lock` crea la rama aislada `build/<task_id>` + `git worktree` en `<worktree-root>/<task_id>`, vincula la única línea `repositorio destino` en `<taskDir>/index.md` (con rollback completo ante cualquier fallo previo al cierre) y persiste un único estado atómico `3_Builder/runtime/<task_id>/session.json` sin rutas absolutas de host.
2. **`context`:** sincroniza el chunk e intento activos desde `<taskDir>/.ring/state.json` (o lee el primer chunk de `chunks/*.md` con `plan_approved: false` en Secuencia A pre-`plan`), consulta `0_Brain context --files` en solo lectura y emite el contexto JSON autocontenido del chunk (incluyendo `active_bounce` si aplica) para el agente constructor (`CLIResult` en errores previos).
3. **`tdd-red`:** exige `PlanApproved == true`, `HEAD == base_ref`, cambios dentro de alcance, `0` fugas `G4` y archivo de prueba modificado cuando aplique; ejecuta `VerifyCommands[--cmd-index]` dentro del worktree exigiendo `ExitCode != 0` y congela `red_worktree_tree_sha256` + evidencia literal `G1` en `session.json`.
4. **`package`:** verifica `bounce_brake_active == false`, frescura de `0_Brain` (`risk_zones_sha256` + `matched_doc_hashes`), `G4`, alcance y evolución real del árbol (`worktree_tree_sha256` crudo distinto de `red` y del último rebote), ejecuta **todos** los `VerifyCommands` exigiendo `ExitCode == 0`, re-verifica el snapshot post-comando, limpia aprobaciones/rebotes previos pendientes y escribe el `DeliveryBundle` v1.0 inmutable + `.diff` unificado completo hacia `4_Revisores`.
5. **`apply-order`:** valida `BounceOrder` v1.0 (`APPROVE` | `BOUNCE`) contra `bounce_order.schema.json` y el último `bundle_sha256` (exigiendo resolución única por bundle y `order_id` no repetido): en `APPROVE` sella `approved_bundle_sha256` + `approved_worktree_tree_sha256`; en `BOUNCE` permite hasta `3` ciclos de remediación (`bounce_cycle` `1..3`) y activa el freno `bounce_brake_active = true` (`exit 5`, `MAX_BOUNCE_CYCLES`) ante un 4.º `BOUNCE` (desbloqueable solo con `--reset-bounces --reason`, que incrementa `reset_epoch`).
6. **`commit`:** verifica ausencia total de deriva post-aprobación (`current_worktree_tree_sha256 == approved_worktree_tree_sha256`, `HEAD == base_ref`, `index.md` apuntando al worktree), re-valida `G4` y alcance, invoca `gengis.exe check <taskDir> --root <gengisRoot>`, comprueba que `HEAD` del repo principal no cambió y emite el comando entrecomillado de 1 línea `gengis.exe decide` para `[AUTOR]`.
7. **`cleanup`:** retira el worktree físico (`git worktree remove` + `prune`) bajo `.repo-<sha16>.lock` cuando la tarea está en `DONE`/`ABORTED`, preservando `build/<task_id>` por defecto.
8. **`verify`:** auditoría 100% solo lectura (`0` cerrojos, `0` escrituras) que comprueba reparse points, `.gitignore`, los 5 hashes `RZ-001`, `0_Brain` e integridad/`G4` de `3_Builder/runtime/`.

### Mapa Comparado de Códigos de Salida (`1_Intake` vs. `2_Spec_Writer` vs. `3_Builder`)

| Código | `1_Intake` (`0..5`) | `2_Spec_Writer` (`0..6`) | `3_Builder` (`builder.exe`, `0..6`) | Consume rebote (`bounce_cycle`) | `retryable` (JSON) |
| :---: | :--- | :--- | :--- | :---: | :---: |
| `0` | `ready` / éxito | Éxito (`OK`) | Éxito (`reason: "OK"`; en `apply-order` con `BOUNCE` `1..3` incrementa `bounce_cycle` y devuelve `0`) | **Sí** solo en `apply-order` (`decision=="BOUNCE"` y `bounce_cycle < 3`); No en el resto | `false` |
| `1` | Error técnico o CLI | Errores corregibles o defecto de invariante | Errores corregibles de construcción/alcance/TDD/contrato (`SCOPE_VIOLATION`, `TDD_RED_PASSED_UNEXPECTEDLY`, `TDD_RED_NO_CHANGES`, `TDD_RED_MISSING_TEST_CHANGE`, `TDD_GREEN_FAILED`, `TDD_GREEN_UNCHANGED_FROM_RED`, `INVALID_JSON`, `ORDER_VALIDATION_FAILED`, `BUNDLE_MISMATCH`, `APPROVAL_REQUIRED`, `WORKTREE_DRIFT_AFTER_APPROVAL`, `WORKTREE_DIRTY`, `DIFF_TOO_LARGE`) | No | `true` |
| `2` | Cuarentena `G6` | Uso CLI inválido / `SPEC_ALREADY_EXISTS` | Uso CLI inválido, tarea incompatible o sesión faltante/duplicada (`INVALID_USAGE`, `SESSION_ALREADY_EXISTS`, `SESSION_NOT_FOUND`, `TASK_DIR_INVALID`, `CMD_INDEX_OUT_OF_RANGE`) | No | `false` |
| `3` | Rechazo Tier 1 `G4` | Rechazo fail-closed `G4` | Rechazo fail-closed `G4` (`G4_VIOLATION`, con ubicación exacta en `issues[].pointer`) | No | `false` |
| `4` | `0_Brain` / lock | Decisión externa requerida | Precondición o decisión `[AUTOR]` requerida (`PLAN_NOT_APPROVED`, `TASK_TERMINAL_STATE`, `STALE_BRAIN_CONTEXT`, `TDD_RED_REQUIRED`, `UNEXPECTED_COMMIT_IN_WORKTREE`, `BRANCH_COLLISION`, `GENGIS_DECIDE_REQUIRED`) | No | `false` |
| `5` | Conflicto `dedupe_key` | Tope de intentos / ciclos externos | Freno operativo de rebotes activo (`MAX_BOUNCE_CYCLES`, al recibir un 4.º `BOUNCE` cuando `bounce_cycle == 3` o mientras `bounce_brake_active == true`; desbloqueo con `apply-order --reset-bounces --reason`) | No (resultado del tope) | `false` |
| `6` | *(no usado)* | Infra transitoria (`true`) o negativa/corrupción/deriva (`false`) | **Transitoria (`true`):** `BRAIN_UNAVAILABLE`, `BRAIN_JOURNAL_ACTIVE`, `LOCK_BUSY`, `GIT_ERROR`, `GENGIS_UNAVAILABLE`, `IO_ERROR` <br>**Negativa/corrupción/deriva (`false`):** `TASK_ALREADY_ACTIVE`, `WORKTREE_PATH_MISMATCH`, `WORKTREE_MISSING`, `SESSION_STATE_CORRUPT`, `ARTIFACT_STATE_CORRUPT`, `REPARSE_POINT_DETECTED`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT`, `INDEX_BIND_INVARIANT_BROKEN` | No | `true` (infra transitoria) / `false` (negativa, corrupción o deriva) |

## The Problem

Completados `0_Brain` (`main` `8316011` + `fix/0-brain-hardening` `6eed341`), `1_Intake` (`feat/1-intake` `ee16ab4`) y `2_Spec_Writer` (`feat/2-spec-writer` `00c32c7`), la factoría compila tareas deterministas en `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\`. Sin embargo, operar directamente con `GengiSWAP/gengis.exe` sin un motor determinista en `3_Builder` deja **siete brechas verificadas en el código (`G5`)**:

1. **Acoplamiento del CLI `gengis.exe` a `index.md` y falta de `git worktree` aislado:**
   Desde CLI, `GengiSWAP/engine/cmd/gengis/main.go:94-99` construye `ring.CLIConfig` con `TargetRepo == ""` (sin bandera `--repo`), por lo que `PlanTask` (`bridge.go:1232-1236`), `CheckChunk` (`bridge.go:1301-1302`) y `DecideChunk` (`bridge.go:1372-1373`) resuelven el repositorio destino leyendo `<taskDir>/index.md` y priorizando `- **Repositorio destino:**` (`bridge.go:164-167, 236-290, 1148-1156`). Si `index.md` conserva la ruta del repositorio principal (`C:\Black Factory`), cualquier construcción ensucia el árbol principal, impide concurrencia entre tareas y en `DecideChunk` calcularía `base_ref` o `git revert` sobre la rama equivocada. Asimismo, `2_Spec_Writer` permite `VerifySteps` que invocan binarios locales `.gitignore`d (`0_Brain/brain.exe`, `1_Intake/intake.exe`, `2_Spec_Writer/specwriter.exe` en `2_Spec_Writer/engine/validate.go:1524-1575`), los cuales no existen dentro de un `git worktree` limpio salvo validación explícita previa.
2. **Bloqueo de `gengis.exe check` tras el primer intento que impide rebotes autónomos con `4_Revisores` / `5_Cycle_Approver`:**
   En `GengiSWAP/engine/bridge.go:1342-1349`, una vez ejecutado `gengis.exe check` sobre un intento (`att.QADone == true && att.Verdict == nil`), el anillo rechaza cualquier re-ejecución hasta que `[AUTOR]` corra `gengis.exe decide`, habiendo creado ya un commit en `qaStep` (`ring.go:661-665`) con `cliPreparador.Verify` fijo en `true` (`bridge.go:1620-1622`). Sin un bucle interno previo a `gengis.exe check`, cada rebote de `4_Revisores` bloquearía al humano `[AUTOR]` y contaminaría el historial Git.
3. **Ausencia de verificación mecánica de TDD (*Red → Green*):**
   `GengiSWAP/engine/ring.go:667-681` solo verifica `ExitCode == 0` al entregar; nunca exige evidencia `G1` de fallo inicial (`ExitCode != 0`) en fase *Red* ni comprueba que los bytes del worktree cambiaron realmente entre *Red* y *Green*.
4. **Falta de consulta obligatoria a `0_Brain` (*Read-Before-Act*) y control de deriva documental:**
   `gengis.exe` no consulta `0_Brain/brain.exe` ni detecta cambios en `wiki/risk_zones.md` o documentos cubiertos (`matched_doc_hashes`) durante la construcción de cada chunk.
5. **Falta de puerta `G4` previa sobre diffs y riesgo de commits manuales fuera del motor:**
   `GitWorkspace.Commit` (`GengiSWAP/engine/runner.go:199-229`) commitea sin escanear `G4`, y si el agente ejecuta `git commit` manualmente (`Docs/PROYECTO_TUNGSTENO.md` §5.6 ítem 7), el motor no puede des-commitear archivos fuera de alcance.
6. **Inexistencia de contratos formales (`DeliveryBundle` / `BounceOrder`) ni sello anti-bypass post-aprobación:**
   No existen esquemas JSON estrictos entre `3_Builder`, `4_Revisores` y `5_Cycle_Approver`, ni reglas deterministas de aplicabilidad `UX`/`UI`, ni un hash crudo del árbol (`worktree_tree_sha256`) que impida editar archivos entre `APPROVE` y `gengis.exe check` o re-empaquetar cambios vacíos tras un rebote.
7. **Falta de privacidad `Opción i`, telemetría `trace_id` y comando `verify` en Equipo 3:**
   `.gitignore` aún no lista explícitamente `3_Builder/runtime/` ni `3_Builder/builder.exe`, y no existe auditoría read-only de deriva para Equipo 3.

**Solución Propuesta:**
Construir `3_Builder/engine` (`module blackfactory/builder`, `CGO_ENABLED=0`, cero SQLite, cero LLM, `0` bytes tocados en `0_Brain`, `1_Intake`, `2_Spec_Writer`, `pkg/bfcore` y `GengiSWAP/engine`) exponiendo `builder.exe` con ocho subcomandos (`init`, `context`, `tdd-red`, `package`, `apply-order`, `commit`, `cleanup`, `verify`), un único archivo de estado atómico por tarea (`session.json`), cerrojo de repositorio para operaciones `git worktree`, validación de comandos compatibles con worktree, ciclo TDD *Red → Green* verificado con hash crudo de bytes, contratos Draft 2020-12 (`delivery_bundle.schema.json` y `bounce_order.schema.json`), freno de 3 ciclos de remediación y sellado final mediante subproceso `gengis.exe check`.

## Goals

*   **SG1 — Núcleo Pure-Go Sin LLM, Sin SQLite y Cero Mutación de Módulos Cerrados (`RZ-001` Intacto):** Compilar `3_Builder/builder.exe` (`CGO_ENABLED=0`, integrado en `go.work`, requiriendo únicamente `blackfactory/pkg/bfcore` y `golang.org/x/sys v0.49.0`) con cobertura `>= 85%` sobre `t.TempDir()` (`G3`), dejando `0_Brain/**`, `1_Intake/**`, `2_Spec_Writer/**`, `pkg/bfcore/**` y `GengiSWAP/engine/**` con `0` bytes modificados.
*   **SG2 — Aislamiento `git worktree` Concurrente y Puente Transparente con `gengis.exe`:** Validar todas las precondiciones antes de mutar en `init` (no solapamiento de `--worktree-root`, compatibilidad de `VerifyCommands` con worktree sin binarios `Opción i` no rastreados, resolución a SHA de 40 hex vía `git rev-parse --verify <ref>^{commit}`, `0_Brain` `Preflight` + `Context`); serializar `git worktree add/remove/prune` bajo `.repo-<sha16>.lock`; soportar Secuencia A (pre-`plan`) y Secuencia B (post-`plan` con `0` intentos); vincular únicamente la línea `repositorio destino` en `<taskDir>/index.md` con rollback completo ante fallo; y mantener `0` rutas absolutas de host en `3_Builder/runtime/`.
*   **SG3 — *Read-Before-Act* (`0_Brain`) y TDD Determinista (*Red → Green → Refactor*):** Consultar `0_Brain` en `init` y `context` (verificando `risk_zones_sha256` y `matched_doc_hashes[]` en `package`), exigir en `tdd-red` `HEAD == base_ref`, `Scope` válido, `G4` limpio, cambio en archivo de prueba cuando aplique y `ExitCode != 0` sobre `VerifyCommands[--cmd-index]`, y exigir en `package` cambio real de bytes crudos respecto a `red` y al último rebote más `ExitCode == 0` en todos los `VerifyCommands` (re-verificando el snapshot post-comando).
*   **SG4 — Contratos Canónicos (`DeliveryBundle` v1.0 y `BounceOrder` v1.0), Aplicabilidad Aditiva UX/UI y Freno de 3 Ciclos:** Publicar `delivery_bundle.schema.json` y `bounce_order.schema.json` (`additionalProperties: false`), calcular aplicabilidad `auto` por extensión y segmentos de ruta (permitiendo únicamente activación aditiva `--include-ui` / `--include-ux`), soportar hasta `3` ciclos de remediación `BOUNCE` (`bounce_cycle` `1..3`) y activar `bounce_brake_active = true` (`exit 5`, `MAX_BOUNCE_CYCLES`) ante un 4.º `BOUNCE` (desbloqueo auditado `apply-order --reset-bounces --reason`).
*   **SG5 — Sellado Anti-Bypass en `commit` sobre `gengis.exe check` y Contrato CI Estricto:** Exigir en `commit` `approved_bundle_sha256`, `current_worktree_tree_sha256 == approved_worktree_tree_sha256` (hash crudo + modo de archivo), `extractTargetRepo(index.md) == absWorktreeClean` y `HEAD == base_ref`, ejecutar `gengis.exe check`, verificar que `HEAD` de `--repo-root` no cambió, manejar `QAPassed == false` como `exit 4` (`GENGIS_DECIDE_REQUIRED`, `retryable: false`) y validar el flujo completo con `GENGIS_CONTRACT_STRICT=1` contra `gengis.exe` real.
*   **SG6 — Privacidad `Opción i`, `verify` Solo Lectura, Telemetría `trace_id` y Latencia `< 500 ms` p95:** Excluir explícitamente `3_Builder/runtime/` y `3_Builder/builder.exe` en `.gitignore`; exponer `builder verify` 100% read-only verificando `.gitignore`, los 5 archivos clave de `GengiSWAP/engine/`, `0_Brain` y todos los `.json`/`.jsonl`/`.diff` de `runtime/`; y cumplir `< 500 ms` p95 de overhead propio en Windows.

## Non-Goals

*   **NG1 — Llamadas a APIs LLM o Bandera de Diversidad de Modelos en Go:** `builder.exe` nunca realiza llamadas de red ni valida identidades de modelos en el CLI; el enrutamiento y diversidad de modelos residen en `3_Builder/skills/builder-execution/SKILL.md`.
*   **NG2 — Modificar `GengiSWAP/engine/` (`RZ-001`) o Absorber `TD-004` en v1:** `GengiSWAP/engine/` se mantiene 100% intacto (`0` bytes modificados) e invocado solo como subproceso `gengis.exe`.
*   **NG3 — Implementar Motores de `4_Revisores`, `5_Cycle_Approver` o `6_Filtro_de_Riesgo` en `bf_03_builder_v1`:** Alcance acotado estrictamente a `3_Builder` y sus esquemas JSON de frontera.
*   **NG4 — Bypass de `[AUTOR]` (`plan` / `decide`) o Degradación Manual de Revisores UX/UI:** Prohibido invocar `gengis.exe plan`/`decide` desde `builder.exe`, prohibido `--skip-review-gate` y prohibido forzar `ui_applicable=false` o `ux_applicable=false` cuando la regla determinista `auto` dio `true`.
*   **NG5 — Modificar `0_Brain/**`, `1_Intake/**`, `2_Spec_Writer/**`, `pkg/bfcore/**` o Ejecutar `git merge`/`push`:** Todos los módulos previos permanecen intactos en v1 y no existe bandera `--spec` acoplada a `2_Spec_Writer/runtime/`.
*   **NG6 — Comandos Arbitrarios por CLI o Tareas sin Git:** Solo se ejecutan los `VerifyCommands` de `.ring/state.json` aprobados por `[AUTOR]`, rechazando comandos que apunten a binarios locales `Opción i` ausentes en el worktree.

## Open Questions

Ninguna abierta para v1. Las cinco decisiones de encuadre (`Q1–Q5`) fueron ratificadas por `[AUTOR]` (`blackpusch`) el `2026-11-24`:
*   **Q1 (Resuelta):** Crear `feat/3-builder` directamente desde `feat/2-spec-writer` (`00c32c7`) llevando la documentación actualizada sin tocar `main` todavía.
*   **Q2 (Resuelta):** Mantener `GengiSWAP/engine/` (`RZ-001`) 100% intacto en v1 envolviendo `gengis.exe` como subproceso y difiriendo `TD-004`.
*   **Q3 (Resuelta):** Construir estrictamente `3_Builder` (`builder.exe` + skill) definiendo en `3_Builder/schemas/` los contratos `DeliveryBundle` y `BounceOrder` para Equipos 4 y 5.
*   **Q4 (Resuelta):** Perfiles agnósticos por rol en `3_Builder/skills/` (ref. Opus/Sol/Astra/Sonnet) + aplicabilidad determinista `ui`/`ux` con estado `NA` justificado cuando no exista GUI o superficie de usuario.
*   **Q5 (Resuelta):** Completar directamente la especificación técnica de `Docs/designs/3_builder.md` y abordar `6_Filtro_de_Riesgo` en su fase correspondiente.

--------------------------------------------------------------------------------

## Critical User Journeys (CUJs)

### CUJ 1: Flujo Completo Aislado (`init → plan → context → tdd-red → package → apply-order(APPROVE) → commit → decide → cleanup`)

*   **Given**: `2_Spec_Writer` compiló `C:\Desarrollo\GengiSWAP_data\gengis_task\bf_demo_101\` (`index.md` con `WorkItem: WI-a1b2c3d4e5f6 (rev 1)`, `Trace ID: trc-20261124-a1b2c3d4`, `Carril: FTD`, `Spec SHA-256`, `Rama destino: feat/3-builder`, `Repositorio destino: C:\Black Factory` y 1 chunk Go con `go vet` + `go test` sobre paquete interno sin GUI ni CLI). `0_Brain/vault/` está indexado y limpio.
*   **When**:
    1. El agente ejecuta `builder.exe init --task-dir "C:\Desarrollo\GengiSWAP_data\gengis_task\bf_demo_101" --repo-root "C:\Black Factory" --project black-factory --json`.
    2. `[AUTOR]` aprueba el plan: `gengis.exe plan "C:\Desarrollo\GengiSWAP_data\gengis_task\bf_demo_101" --root "C:\Black Factory\GengiSWAP"`.
    3. El agente ejecuta `builder.exe context --task-dir "..." --repo-root "C:\Black Factory" --json`, escribe primero la prueba fallida `foo_test.go` dentro de `C:\Desarrollo\GengiSWAP_data\worktrees\bf_demo_101\` y ejecuta `builder.exe tdd-red --task-dir "..." --repo-root "C:\Black Factory" --json`.
    4. El agente implementa `foo.go` dentro del worktree y ejecuta `builder.exe package --task-dir "..." --repo-root "C:\Black Factory" --json`.
    5. `4_Revisores` (Seguridad y Código activos; UX y UI en `NA` justificado) y `5_Cycle_Approver` emiten `approve.json` (`decision: "APPROVE"`). El agente ejecuta `builder.exe apply-order --task-dir "..." --repo-root "C:\Black Factory" --order approve.json --json` y luego `builder.exe commit --task-dir "..." --repo-root "C:\Black Factory" --json`.
    6. `[AUTOR]` ejecuta el comando entrecomillado `gengis.exe decide` (`GO_NEXT` → tarea `DONE`), y el agente ejecuta `builder.exe cleanup --task-dir "..." --repo-root "C:\Black Factory" --json`.
*   **Then**:
    1. `init` valida en seco `index.md`, `VerifyCommands` y `0_Brain`, adquiere `.repo-<sha16>.lock`, crea `build/bf_demo_101` y el worktree `C:\Desarrollo\GengiSWAP_data\worktrees\bf_demo_101`, reescribe únicamente `- **Repositorio destino:**` en `index.md` y guarda `3_Builder/runtime/bf_demo_101/session.json` con `0` rutas absolutas de host (`exit 0`).
    2. `gengis.exe plan` bloquea `.ring/base_ref` sobre el `HEAD` del worktree y marca `PlanApproved: true`.
    3. `tdd-red` verifica `HEAD == base_ref`, alcance, `G4` y presencia de `foo_test.go`, corre `VerifyCommands[0]` confirmando `ExitCode != 0` y guarda `tdd.red` en `session.json` (`exit 0`).
    4. `package` comprueba `worktree_tree_sha256 != red_worktree_tree_sha256`, corre todos los `VerifyCommands` (`ExitCode == 0`), re-valida el snapshot post-comando y escribe `bundles/01_core_a1_r0_c0.{bundle.json,diff}` con `ui.applicable = false` y `ux.applicable = false` (`exit 0`).
    5. `apply-order` archiva `bounces/01_core_a1_r0_c0_approve.order.json` y fija `approved_bundle_sha256` y `approved_worktree_tree_sha256` (`exit 0`). `commit` verifica cero deriva de bytes crudos, ejecuta `gengis.exe check`, confirma que `HEAD` de `C:\Black Factory` no cambió e imprime el comando `gengis.exe decide` para `[AUTOR]` (`exit 0`).
    6. `cleanup` elimina el worktree físico bajo `.repo-<sha16>.lock` preservando la rama `build/bf_demo_101` (`exit 0`).

### CUJ 2: Tres Ciclos de Rebote (`BOUNCE` `1..3`), Freno en el 4.º `BOUNCE` (`exit 5`) y Desbloqueo Auditado

*   **Given**: El chunk `01_core` (intento `1`) emitió el bundle inicial (`bounce_cycle: 0`).
*   **When**:
    1. `5_Cycle_Approver` emite `bounce_1.json` (`decision: "BOUNCE"`, `requires_red_test: true`) y el agente ejecuta `builder.exe apply-order --task-dir "..." --repo-root "..." --order bounce_1.json --json`.
    2. El agente intenta correr `package` sin nuevo `tdd-red`; luego ejecuta `tdd-red` → corrige → `package` (`bounce_cycle: 1`).
    3. Si ocurren un 2.º `BOUNCE` (`bounce_cycle: 2`, `requires_red_test: false`) y el agente intenta re-ejecutar `package` **sin editar ningún archivo**; luego edita, empaqueta ciclo `2`, recibe un 3.er `BOUNCE` (`bounce_cycle: 3`), corrige y empaqueta ciclo `3`.
    4. Si `5_Cycle_Approver` emite `APPROVE` en ciclo `3`, avanza a `commit`; pero si emite un **4.º `BOUNCE`**, `apply-order` corta a freno y posteriormente `[AUTOR]` desbloquea con `builder.exe apply-order --task-dir "..." --repo-root "..." --reset-bounces --reason "Criterio alineado con AUTOR" --json`.
*   **Then**:
    1. El 1.er `BOUNCE` guarda `bounces/01_core_a1_r0_b1.order.json`, sube `bounce_cycle` a `1`, guarda `last_bounced_worktree_tree_sha256`, limpia `tdd.red` (`requires_red_test: true`) y sale con `exit 0`.
    2. `package` sin `tdd-red` falla cerrado con `exit 4` (`TDD_RED_REQUIRED`); tras nuevo `tdd-red` y fix, emite `bundles/01_core_a1_r0_c1.bundle.json` (`exit 0`).
    3. En `requires_red_test: false`, re-ejecutar `package` sin cambios falla cerrado con `exit 1` (`TDD_GREEN_UNCHANGED_FROM_RED`) al coincidir con `last_bounced_worktree_tree_sha256`. El ciclo `3` permite `tdd-red`, `package` y `apply-order(APPROVE)` + `commit` con éxito (`exit 0`).
    4. Un **4.º `BOUNCE`** (cuando `bounce_cycle == 3`) archiva `bounces/01_core_a1_r0_b4_brake.order.json`, activa `bounce_brake_active = true` en `session.json` y devuelve `exit 5` (`MAX_BOUNCE_CYCLES`, `retryable: false`), bloqueando `tdd-red`, `package`, `apply-order` y `commit` con `exit 5` hasta que `--reset-bounces --reason` valide `G4`, incremente `reset_epoch` (`1`), reinicie `bounce_cycle = 0` y `bounce_brake_active = false`, limpie `tdd.red` y aprobaciones pendientes, registre `BOUNCES_RESET` y salga con `exit 0` (conservando intactos todos los artefactos `r0` previos).

### CUJ 3: Guardias Anti-Bypass (`G4_VIOLATION`, `WORKTREE_DRIFT_AFTER_APPROVAL`, `UNEXPECTED_COMMIT_IN_WORKTREE` y `GENGIS_DECIDE_REQUIRED`)

*   **Given**: Un worktree activo tiene: (a) una línea añadida o ruta con secreto/ruta personal `G4`; (b) un cambio de fin de línea CRLF/LF o BOM UTF-8 introducido tras `APPROVE` antes de `commit`; (c) un `git commit` manual hecho por el agente; o (d) un fallo inesperado durante `gengis.exe check` que deja `att.QADone == true && att.QAPassed == false`.
*   **When**: Se invoca `package` en (a), `commit` en (b), `tdd-red`/`package` en (c), o `commit` en (d).
*   **Then**:
    1. En (a), `package` rechaza antes de correr comandos con `exit 3` (`G4_VIOLATION`, `issues[0].pointer` señalando `/diff/<archivo>`). Si solo una línea de contexto sin modificar (` `) o borrada (`-`) contenía un rastro Tier 2 preexistente, el motor redacta Tier 2 únicamente en esa línea de contexto/`-` del `.diff` persistido para no bloquear limpiezas ni romper `builder verify`.
    2. En (b), `worktree_tree_sha256` (calculado sobre **bytes crudos** y modo de archivo, sin normalizar CRLF/BOM) cambia y `commit` bloquea con `exit 1` (`WORKTREE_DRIFT_AFTER_APPROVAL`) antes de invocar `gengis.exe check`.
    3. En (c), `git rev-parse HEAD != base_ref` dispara `exit 4` (`UNEXPECTED_COMMIT_IN_WORKTREE`).
    4. En (d), `commit` detecta `att.QADone && !att.QAPassed` en `.ring/state.json` y devuelve `exit 4` (`GENGIS_DECIDE_REQUIRED`, `retryable: false`) imprimiendo el comando entrecomillado `gengis.exe decide --qa-pass=false ...` para `[AUTOR]`.

### CUJ 4: `builder verify` Solo Lectura, Secuencia B (`plan` Previo a `init`) y Rechazo de Binarios `Opción i` en `VerifyCommands`

*   **Given**: (a) `[AUTOR]` ejecutó `gengis.exe plan` antes de `builder init` (`PlanApproved: true`, `Index: 0`, `0` intentos); (b) una tarea incluye `0_Brain\brain.exe query ...` relativo en `VerifyCommands`; (c) se ejecuta `builder verify` en checkout limpio o con deriva en `.gitignore` / `GengiSWAP/engine/`.
*   **When**: Se ejecuta `builder init` en (a) y (b), y `builder verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --json` en (c).
*   **Then**:
    1. En (a), `init` toma el commit exacto de `.ring/base_ref`, crea el worktree sobre ese commit y vincula `index.md` (`exit 0`); si hubiera intentos previos devuelve `exit 6` (`TASK_ALREADY_ACTIVE`).
    2. En (b), `init` detecta el binario local `Opción i` no rastreado en `HEAD` antes de crear worktree ni tocar `index.md` y falla cerrado con `exit 2` (`TASK_DIR_INVALID`, pointer `/chunks/01_.../verify_commands/0`).
    3. En (c), `verify` opera con `0` cerrojos y `0` escrituras: devuelve `exit 0` en checkout limpio sin `runtime/`, `exit 6` ante `GITIGNORE_DRIFT` o `GENGIS_ENGINE_DRIFT` (en cualquiera de los 5 archivos vigilados de `GengiSWAP/engine/`), y `exit 3` (`G4_VIOLATION`) ante cualquier secreto en `.json`, `.jsonl` o `.diff` dentro de `runtime/`.

--------------------------------------------------------------------------------

## Technical Plan (Architecture)

### 1. Topología del Sistema y Límites de Módulos (`go.work` + `pkg/bfcore` + `3_Builder`)

`3_Builder` añade `./3_Builder/engine` a [`go.work`](../../go.work) y deja **100% intactos** `0_Brain/**`, `1_Intake/**`, `2_Spec_Writer/**`, `pkg/bfcore/**` y `GengiSWAP/engine/**`:

```text
Black Factory\
├── go.work                                  (Añade ./3_Builder/engine)
├── .gitignore                               (Añade 3_Builder/runtime/ y 3_Builder/builder.exe)
├── pkg\bfcore\                              (0 bytes modificados; reutiliza g4, paths, brain_client, lock, reparse)
├── GengiSWAP\                               (RZ-001 — 0 bytes modificados; invocado solo vía gengis.exe)
│   ├── gengis.exe
│   └── engine\{bridge,ring,runner,runner_windows,cmd/gengis/main}.go
└── 3_Builder\                               (Módulo blackfactory/builder)
    ├── README.md                            (Límites, arquitectura, matriz CLI, exits 0..6, verificación)
    ├── schemas\
    │   ├── delivery_bundle.schema.json      (JSON Schema Draft 2020-12 v1.0, additionalProperties: false)
    │   └── bounce_order.schema.json         (JSON Schema Draft 2020-12 v1.0, additionalProperties: false)
    ├── skills\
    │   ├── README.md
    │   └── builder-execution\
    │       ├── SKILL.md                     (Bucle operativo + perfiles agnósticos por rol + reglas UX/UI)
    │       └── profiles.example.json        (Plantilla declarativa para harness externo; nunca leída por Go)
    ├── engine\
    │   ├── go.mod / go.sum / README.md      (module blackfactory/builder; solo bfcore + golang.org/x/sys)
    │   ├── model.go                         (Structs exactos, códigos 0..6, catálogo Reason*, CanonicalBundleSHA256)
    │   ├── state.go                         (Locks .session.lock -> .repo.lock -> .events.lock, JSON atómico G4)
    │   ├── task_parser.go                   (Parseo index.md/.ring/state.json, Scope paridad ring.go, bind index.md)
    │   ├── git_worktree.go                  (GitClient, worktree add/remove/prune, snapshot crudo, diff + G4)
    │   ├── runner.go / runner_windows.go / runner_unix.go (Ejecución VerifyCommands paridad GengiSWAP + G4 stdout)
    │   ├── verify.go                        (Preflight compartido + builder verify 100% solo lectura)
    │   ├── init_cmd.go                      (Validación en seco + worktree + bind index.md + rollback completo)
    │   ├── context_cmd.go                   (Sincronización .ring/state.json + Read-Before-Act 0_Brain)
    │   ├── tdd_cmd.go                       (Fase Red determinista: Scope + G4 + test file + ExitCode != 0)
    │   ├── package_cmd.go                   (Fase Green + re-snapshot post-comando + DeliveryBundle inmutable)
    │   ├── bounce_cmd.go                    (Subcomando apply-order: APPROVE/BOUNCE + --reset-bounces --reason)
    │   ├── commit_cmd.go                    (Anti-bypass crudo + gengis.exe check + reconciliación post-check)
    │   ├── cleanup_cmd.go                   (git worktree remove + prune preservando rama build/<task_id>)
    │   ├── core_test.go / lifecycle_test.go / workflow_test.go (Suites TDD unitarias y benchmark < 500 ms p95)
    │   ├── contract_gengis_test.go          (Contrato real gengis.exe + git worktree; GENGIS_CONTRACT_STRICT=1)
    │   └── cmd\builder\
    │       ├── README.md
    │       ├── main.go                      (CLI builder.exe: 8 subcomandos, exits 0..6)
    │       └── main_test.go                 (Tests E2E de CUJ 1..4 sobre t.TempDir())
    ├── builder.exe                          (Binario compilado local; ignorado en .gitignore)
    └── runtime\                             (Estado privado Opción i; ignorado en .gitignore; 0 rutas host)
        ├── .repo-<repo_sha16>.lock          (Cerrojo por repositorio para git worktree add/remove/prune/branch -D)
        ├── .events.lock                     (Cerrojo breve exclusivamente para appends en builder_events.jsonl)
        ├── builder_events.jsonl             (Bitácora append-only G4-sanitizada con trace_id)
        └── <task_id>\
            ├── .session.lock                (Cerrojo granular por tarea)
            ├── session.json                 (Único archivo mutable de estado: sesión + brain_context + tdd)
            ├── bundles\
            │   ├── <chunk>_a<N>_r<E>_c<C>.bundle.json  (DeliveryBundle inmutable hacia 4_Revisores)
            │   └── <chunk>_a<N>_r<E>_c<C>.diff         (Diff unificado -U3 + untracked, ScanG4All == 0)
            └── bounces\
                ├── <chunk>_a<N>_r<E>_b<K>.order.json         (Órdenes BOUNCE 1..3 y b4_brake inmutables)
                └── <chunk>_a<N>_r<E>_c<C>_approve.order.json (Orden APPROVE inmutable sin colisión)
```

### 2. Aislamiento `git worktree`, Validación Pre-Mutación y Puente sin Tocar `RZ-001`

```text
  C:\Black Factory (--repo-root — NUNCA mutado por 3_Builder)
       │
       │ [1] Validación 100% read-only (rutas, VerifyCommands, SHA 40-hex, 0_Brain)
       │ [2] Bajo .session.lock -> .repo-<sha16>.lock:
       │     git worktree add -b build/<task_id> <worktree_path> <base_ref_40hex>
       ▼
  <worktree-root>\<task_id>\  (rama: build/<task_id>, HEAD == <base_ref_40hex>)
       ▲
       │ <taskDir>\index.md -> "- **Repositorio destino:** <absWorktreeClean>"
       │ (reescritura atómica de 1 sola línea; rollback total si falla antes de session.json)
       ▼
  GengiSWAP\gengis.exe (plan | check | decide) ──► GitWorkspace + ShellRunner operan en el worktree
```

1. **No solapamiento de raíces (`--worktree-root`):**
   - Default: `filepath.Join(filepath.Dir(filepath.Dir(absTaskDir)), "worktrees")`.
   - `init` rechaza con `exit 2` (`INVALID_USAGE`) si `<worktree-root>` o `<worktree-root>/<task_id>` coincide, contiene o está contenido dentro de `--repo-root`, `--factory-root`, `--data-dir` o `--task-dir` (comparación insensible a mayúsculas sobre rutas limpias absolutas).
2. **Extracción determinista de metadatos de `<taskDir>/index.md` (`task_parser.go`):**
   - `task_id`: `filepath.Base(absTaskDir)` validado contra `^[a-z0-9][a-z0-9_-]{2,63}$` y sin nombres reservados Windows (`exit 2`, `TASK_DIR_INVALID`).
   - Bajo `## Notas Post-Merge y Trazabilidad` (plantilla `2_Spec_Writer/engine/templates/index.md.tmpl:26-30`):
     - `WorkItem:` recorta sufijos `(rev N)` y backticks → valida `^WI-[0-9a-f]{12}$` (o `""` en tareas manuales).
     - `Trace ID:` recorta backticks; si está ausente en tareas manuales, genera deterministamente `"trc-builder-" + taskID + "-" + sha256Hex(taskID + "\n" + baseRef)[:8]`.
     - `Carril:` `"FTD" | "ASDD" | ""`.
     - `Spec SHA-256:` recorta backticks; si no está vacío debe cumplir `^[0-9a-f]{64}$` (`exit 2`, `TASK_DIR_INVALID`).
   - Bajo `## Directorio de Destino Real`:
     - Verifica que exista **exactamente 1 línea** (fuera de fences) cuya etiqueta normalizada sea `repositorio destino` (si `!= 1` → `exit 6`, `INDEX_BIND_INVARIANT_BROKEN`). Si su valor actual no es `<TARGET_REPO>` ni ya `<absWorktreeClean>`, debe coincidir (`EqualFold` sobre `filepath.Clean`) con `absRepoClean` (`exit 2`, `TASK_DIR_INVALID`).
     - Extrae `Rama destino:` como `target_branch` (recortando espacios/backticks).
3. **Compatibilidad de `VerifyCommands` con `git worktree` (bloqueo de rutas `Opción i`):**
   - Antes de crear el worktree (y tras resolver `baseRef40`), `init` comprueba que `<dataDir>/<task_id>/session.json` no exista aún (`exit 2`, `SESSION_ALREADY_EXISTS`), carga los chunks desde `.ring/state.json` (Secuencia B) o `chunks/*.md` (Secuencia A), exige `len(chunks) >= 1` y `len(c.VerifyCommands) >= 1` por chunk (`exit 2`, `TASK_DIR_INVALID`), y escanea cada comando buscando referencias relativas a binarios o directorios locales `.gitignore`d (`0_Brain/brain.exe`, `1_Intake/intake.exe`, `2_Spec_Writer/specwriter.exe`, `3_Builder/builder.exe`, `GengiSWAP/gengis.exe`, `0_Brain/vault`, `1_Intake/runtime`, `2_Spec_Writer/runtime`, `3_Builder/runtime` con `/` o `\`). Si la ruta referenciada no está rastreada en `git -C <repoRoot> ls-tree <baseRef40>`, `init` rechaza con `exit 2` (`TASK_DIR_INVALID`, pointer `/chunks/<id>/verify_commands/<idx>`).
4. **Resolución de `base_ref` a 40 hex, Secuencias A/B y Rollback Total:**
   - **Secuencia A (sin `.ring/state.json`):** elige referencia en orden `--base-ref` (si se pasó) → `target_branch` (si no está vacía) → `"HEAD"`. Resuelve obligatoriamente con `git -C <repoRoot> rev-parse --verify <ref>^{commit}` a un SHA de 40 hex en minúsculas; si `--base-ref` o `target_branch` explícita no resuelven, falla cerrado con `exit 2` (`TASK_DIR_INVALID`) sin hacer fallback silencioso a `HEAD`.
   - **Secuencia B (con `.ring/state.json`):** exige `PlanApproved == true && Status == "RUNNING" && Index == 0 && len(Chunks) > 0 && len(Chunks[0].Attempts) == 0` (en cualquier otro caso → `exit 6`, `TASK_ALREADY_ACTIVE`); lee `.ring/base_ref` y lo resuelve con `rev-parse --verify <ref>^{commit}` (`exit 6`, `ARTIFACT_STATE_CORRUPT` si es inválido).
   - **Rama aislada (`--branch`, default `build/<task_id>`):** validada con `git check-ref-format --branch`; si ya existe localmente o aparece en `git worktree list --porcelain` o coincide con `HEAD` de `--repo-root` → `exit 4` (`BRANCH_COLLISION`).
   - **Orden transaccional en `init`:** (i) todas las lecturas/validaciones + `0_Brain` `Preflight` & `Context` ocurren **antes** de mutar disco; (ii) adquiere `.session.lock` → re-verifica inexistencia de `session.json` (`SESSION_ALREADY_EXISTS` `2`) → adquiere `.repo-<sha16>.lock`; (iii) guarda en memoria `origIndexBytes`; (iv) ejecuta `git worktree add -b <buildBranch> <absWorktreeClean> <baseRef40>`; (v) reescribe la única línea `repositorio destino` en `<taskDir>/index.md` y verifica `extractTargetRepo == absWorktreeClean`; (vi) escribe `session.json` con `WriteJSONAtomicG4`. Si los pasos (iv), (v) o (vi) fallan, un `defer` restaura `origIndexBytes` en `index.md`, ejecuta `git worktree remove --force <absWorktreeClean>` y `git branch -D <buildBranch>` antes de retornar error.

### 3. Ciclo TDD Determinista, Snapshot Crudo Anti-Bypass y Reglas `G4` en Diff/Stdout

```text
  [context] ──► Sincroniza (chunk_id, attempt) desde .ring/state.json + 0_Brain Context
      │
      ▼ (Agente escribe test que falla)
  [tdd-red] ──► Verifica index.md==worktree, HEAD==base_ref, Scope, G4, has_test_file
      │         Corre VerifyCommands[cmd_index] -> EXIGE ExitCode != 0
      │         Guarda session.tdd.red (red_worktree_tree_sha256 crudo + evidencia G1)
      ▼ (Agente implementa código mínimo y refactoriza)
  [package] ──► Verifica Brain fresco, G4, Scope, len(changed)>=1,
      │         tree_sha256 != red && tree_sha256 != last_bounced
      │         Corre TODOS VerifyCommands (ExitCode == 0) + RE-VERIFICA snapshot/Scope/G4
      │         Escribe bundles/<chunk>_a<N>_c<C>.{bundle.json,diff} inmutables
      ▼
  [apply-order] ◄── BounceOrder v1.0 (4_Revisores -> 5_Cycle_Approver)
      ├── BOUNCE (ciclos 1..3 -> exit 0; 4.º BOUNCE -> bounce_brake_active=true, exit 5)
      └── APPROVE (sella approved_bundle_sha256 + approved_worktree_tree_sha256)
              │
              ▼
          [commit] ──► Verifica tree_sha256 == approved (bytes crudos + modo), HEAD==base_ref,
                       index.md==worktree, G4, Scope -> invoca gengis.exe check
                       Verifica HEAD de --repo-root intacto y CommitRef en build_branch
                       Imprime comando entrecomillado gengis.exe decide para [AUTOR]
```

1. **Snapshot determinista sobre bytes crudos (`worktree_tree_sha256`):**
   - Inspecciona `git status --porcelain=v1 -z --untracked-files=all` y `git diff --raw -z <baseRef>`:
     - Rechaza entradas Git con modo `120000` (symlink) o `160000` (gitlink/submodule) y verifica `bfcore.CheckNoReparseChain` sobre `<repoRoot>`, `<repoRoot>/.git`, `<worktree>`, `<worktree>/.git`, `<taskDir>` y cada ruta tocada (`exit 6`, `REPARSE_POINT_DETECTED`).
     - En renombrados/copias `R`/`C` (pares NUL `runner.go:136-153`), registra el destino como `"renamed"` y el origen como `"deleted"` para que `Scope.Allows` + `Scope.Narrows` (`ring.go:88-142`) evalúen **ambas** rutas en exacta paridad con `GengiSWAP`.
     - Para cada archivo existente en disco: verifica `os.Lstat` regular (`!IsDir && Mode().IsRegular()`), exige `Size() <= bfcore.MaxObservedFileBytes (10 MiB)` (`exit 1`, `DIFF_TOO_LARGE`), lee en streaming calculando **SHA-256 de bytes crudos sin normalizar CRLF ni BOM** y captura el modo regular normalizado (`"0755"` si `Mode()&0o111 != 0`, `"0644"` en caso contrario; `"0000"` y `"DELETED"` para borrados).
     - `worktree_tree_sha256 = sha256(baseRef + "\n" + joinSorted(status + "\t" + mode + "\t" + relPath + "\t" + rawSHA256 + "\n"))`. Cualquier cambio de 1 byte, BOM o CRLF/LF entre `APPROVE` y `commit` invalida el hash (`WORKTREE_DRIFT_AFTER_APPROVAL`).
2. **Higiene `G4` sin contradicción entre `package` y `verify` (`.diff` + UTF-16 + salidas):**
   - **Rutas y cambios nuevos (fail-closed 11 reglas, `exit 3` `G4_VIOLATION`):** escanea cada `relPath` en `changed_files[]`, cada línea añadida `+` de `git diff` (excluyendo `+++ b/...`) y el contenido en disco de todo archivo regular añadido o modificado (tanto rastreado como `??`) cuando tenga BOM UTF-16LE (`FF FE`) o UTF-16BE (`FE FF`) —decodificándolo a UTF-8 en memoria— o cuando sea archivo `??` de texto, antes de clasificar binarios con `0x00` o parches `GIT binary patch`.
   - **Buffer `.diff` persistido (`bundles/<chunk>_a<N>_r<E>_c<C>.diff`):** concatena `git -C <wt> diff --no-ext-diff --binary -U3 <baseRef>` con hunks unified-diff generados en pure-Go para archivos `??` (sin tocar `.git/index`), verifica tamaño total `<= MaxDiffBytes (2 MiB)` (`exit 1`, `DIFF_TOO_LARGE`), ejecuta `bfcore.ScanTier1` fail-closed (`exit 3`) sobre **todo** el buffer (incluyendo contexto y líneas `-`), aplica `bfcore.RedactTier2Text` **únicamente** a líneas de contexto (` `) y líneas eliminadas (`-`) para no bloquear limpiezas de deuda Tier 2 preexistente, y exige `len(bfcore.ScanG4All(finalDiff)) == 0` antes de calcular `diff_sha256` y guardar `<chunk>.diff`.
   - **Salida `stdout`/`stderr` de `VerifyCommands` (`SanitizeCommandOutputG4`):** aplica `bfcore.RedactAllG4` sobre el string `Command`, ejecuta `bfcore.ScanTier1` fail-closed (`exit 3`, pointer `/command_output`) sobre la salida cruda completa, aplica `bfcore.RedactTier2Text` **antes** de truncar estrictamente dentro del tope `MaxCommandOutputBytes = 8000` bytes UTF-8 seguros (`3991` B inicio + `18` B de `"\n…[truncado]…\n"` + `3991` B final, ajustados a frontera de runa UTF-8) y verifica `len(bfcore.ScanG4All(excerpt)) == 0` retornando error fail-closed si quedara algún hallazgo.
3. **Reglas de `tdd-red`, re-verificación en `package` y sincronización `.ring/state.json`:**
   - Re-ejecutar `tdd-red` dentro del mismo ciclo está permitido y sobrescribe `session.tdd.red` fijando `BounceCycle == InheritedFromCycle == session.BounceCycle`. Cuando un `BOUNCE` con `requires_red_test: false` hereda el `Red` previo, actualiza `Red.BounceCycle = session.BounceCycle` conservando intacto `Red.InheritedFromCycle`.
   - En `tdd-red`, `has_test_file` es obligatorio (`TDD_RED_MISSING_TEST_CHANGE`, `exit 1`) siempre que el alcance efectivo (`chunk.Scope.CanModify`, o `task.Scope.CanModify` si el del chunk es vacío) incluya algún patrón de test (`*_test.go`, `test_*.py`, `*_test.py`, `*.test.*`, `*.spec.*`) o cuando `changed_files[]` modifique archivos productivos `.go`/`.py`/`.js`/`.ts` y el alcance permita crear/editar su test hermano.
   - En `package`, tras pasar todos los `VerifyCommands` con `ExitCode == 0`, se vuelve a ejecutar `ComputeWorktreeSnapshot` + `Scope` + `ScanWorktreeChangesG4` para sellar el estado exacto post-comando en el `DeliveryBundle` (detectando cualquier artefacto sucio generado por un test) y se limpian `ActiveBounceOrderID`, `ApprovedBundleID`, `ApprovedBundleSHA256` y `ApprovedWorktreeTreeSHA256`.
   - **Auto-sincronización transversal:** `context` (cuando `.ring/state.json` ya existe), `tdd-red`, `package`, `apply-order` (sin `--reset-bounces`) y `commit` ejecutan al inicio `syncSessionWithRingState`:
     - Si `st.Status ∈ {"DONE", "ABORTED"}` → `exit 4` (`TASK_TERMINAL_STATE`).
     - Si el último `Attempt` del chunk activo tiene `QADone == true && Verdict == nil` → `exit 4` (`GENGIS_DECIDE_REQUIRED`, poblando `NextAutorCommand`).
     - Si el último `Attempt` tiene `QADone == false && Verdict == nil` → `exit 6` (`ARTIFACT_STATE_CORRUPT`).
     - Si `(chunk.ID, len(c.Attempts)+1)` avanzó respecto a `(session.ActiveChunkID, session.ActiveAttempt)` tras un `GO_NEXT`, `RETRY_CHUNK` o `RESPLIT_CHUNK` de `[AUTOR]`, reinicia limpiamente la ventana activa (`ResetEpoch = 0`, `BounceCycle = 0`, `BounceBrakeActive = false`, `PreviousBounceIDs = []`, `SeenOrderIDs = []`, `LastBouncedWorktreeTreeSHA256 = ""`, `LatestBundle* = ""`, `Approved* = ""`, `ActiveBounceOrderID = ""`, `TDD.Red = nil`, `ActiveBaseRef = .ring/base_ref`) y re-consulta `0_Brain Context` cuando cambió `chunk.ID`.

### 4. Artefactos Persistentes en `runtime/` y Contratos Canónicos (`DeliveryBundle` y `BounceOrder`)

Todos los JSON persistidos en `3_Builder/runtime/` se validan al leer con `json.Decoder.DisallowUnknownFields()` y al escribir con `WriteJSONAtomicG4` (que ejecuta `ScanRawAndDecodedG4(raw)` —Fase-A cruda + Fase-B decodificada sobre claves y strings— garantizando `0` discrepancias con `builder verify`):

#### 4.1 Estado Unificado de Tarea (`3_Builder/runtime/<task_id>/session.json`)

```go
type SessionState struct {
	SchemaVersion                 string               `json:"schema_version"` // "1.0"
	TaskID                        string               `json:"task_id"`
	Project                       string               `json:"project"`
	WorkItemID                    string               `json:"work_item_id"`
	TraceID                       string               `json:"trace_id"`
	SpecSHA256                    string               `json:"spec_sha256"`
	Lane                          string               `json:"lane"`
	RepoRootBasename              string               `json:"repo_root_basename"`
	RepoRootSHA256                string               `json:"repo_root_sha256"`
	WorktreeBasename              string               `json:"worktree_basename"`
	WorktreePathSHA256            string               `json:"worktree_path_sha256"`
	TargetBranch                  string               `json:"target_branch"`
	BuildBranch                   string               `json:"build_branch"`
	InitialBaseRef                string               `json:"initial_base_ref"`
	ActiveChunkID                 string               `json:"active_chunk_id"`
	ActiveAttempt                 int                  `json:"active_attempt"`
	ActiveBaseRef                 string               `json:"active_base_ref"`
	ResetEpoch                    int                  `json:"reset_epoch"`
	BounceCycle                   int                  `json:"bounce_cycle"`
	BounceBrakeActive             bool                 `json:"bounce_brake_active"`
	PreviousBounceIDs             []string             `json:"previous_bounce_ids"`
	SeenOrderIDs                  []string             `json:"seen_order_ids"`
	ActiveBounceOrderID           string               `json:"active_bounce_order_id"`
	RequiresRedTest               bool                 `json:"requires_red_test"`
	LastBouncedWorktreeTreeSHA256 string               `json:"last_bounced_worktree_tree_sha256"`
	LatestBundleID                string               `json:"latest_bundle_id"`
	LatestBundleSHA256            string               `json:"latest_bundle_sha256"`
	LatestWorktreeTreeSHA256      string               `json:"latest_worktree_tree_sha256"`
	ApprovedBundleID              string               `json:"approved_bundle_id"`
	ApprovedBundleSHA256          string               `json:"approved_bundle_sha256"`
	ApprovedWorktreeTreeSHA256    string               `json:"approved_worktree_tree_sha256"`
	WorktreeStatus                string               `json:"worktree_status"` // "active" | "removed"
	BrainContext                  BrainContextSnapshot `json:"brain_context"`
	TDD                           TDDState             `json:"tdd"`
	CreatedAt                     string               `json:"created_at"`
	UpdatedAt                     string               `json:"updated_at"`
}

type MatchedDocHash struct {
	RelPath string `json:"rel_path"`
	SHA256  string `json:"sha256"`
}

type BrainDocSummary struct {
	ID           string `json:"id"`
	RelPath      string `json:"rel_path"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	VerifiedHead string `json:"verified_head"`
}

type BrainContextSnapshot struct {
	ChunkID          string                   `json:"chunk_id"`
	QueriedFiles     []string                 `json:"queried_files"`
	BrainDBMTime     string                   `json:"brain_db_mtime"`
	RiskZonesSHA256  string                   `json:"risk_zones_sha256"`
	HighestRiskLevel string                   `json:"highest_risk_level"` // vía bfcore.RiskLevelRank
	MatchedDocs      []BrainDocSummary        `json:"matched_docs"`
	MatchedDocHashes []MatchedDocHash         `json:"matched_doc_hashes"` // vía bfcore.ComputeNormalizedFileSHA256
	RiskHits         []bfcore.RiskHit         `json:"risk_hits"`
	Invocations      []bfcore.BrainInvocation `json:"invocations"`
	RefreshedAt      string                   `json:"refreshed_at"`
}

type TDDState struct {
	Red *RedEvidence `json:"red"` // nil cuando aún no se corrió tdd-red en el ciclo vigente
}

type RedEvidence struct {
	ChunkID               string `json:"chunk_id"`
	GengisAttempt         int    `json:"gengis_attempt"`
	BounceCycle           int    `json:"bounce_cycle"`
	InheritedFromCycle    int    `json:"inherited_from_cycle"`
	CmdIndex              int    `json:"cmd_index"`
	Command               string `json:"command"`
	ExitCode              int    `json:"exit_code"`
	DurationMS            int64  `json:"duration_ms"`
	OutputExcerpt         string `json:"output_excerpt"`
	Truncated             bool   `json:"truncated"`
	RedWorktreeTreeSHA256 string `json:"red_worktree_tree_sha256"`
	HasTestFile           bool   `json:"has_test_file"`
	RecordedAt            string `json:"recorded_at"`
}

type CommandEvidence struct {
	CmdIndex      int    `json:"cmd_index"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exit_code"`
	DurationMS    int64  `json:"duration_ms"`
	OutputExcerpt string `json:"output_excerpt"`
	Truncated     bool   `json:"truncated"`
}
```

#### 4.2 Contrato de Salida hacia `4_Revisores`: `DeliveryBundle` (`3_Builder/schemas/delivery_bundle.schema.json`)

Hash canónico `CanonicalBundleSHA256(b)` calculado sobre JSON compacto con slices normalizados `nil → []`:

| Campo | Tipo JSON / Formato | Reglas y Contenido |
| :--- | :--- | :--- |
| `schema_version` | `"1.0"` | Constante estricta |
| `bundle_id` | `string` | `"BND-" + task_id + "-" + chunk_id + "-a" + attempt + "-r" + reset_epoch + "-c" + cycle` |
| `created_at` | `string` (`date-time`) | UTC RFC 3339 |
| `trace_id` | `string` (`minLength: 1`) | Heredado de `index.md` o generado deterministamente |
| `work_item_id` | `string` (`^$|^WI-[0-9a-f]{12}$`) | Vacío `""` en tareas `GengiSWAP` manuales |
| `spec_sha256` | `string` (`^([0-9a-f]{64})?$`) | Vacío `""` en tareas `GengiSWAP` manuales |
| `lane` | `"FTD" \| "ASDD" \| ""` | Carril de la tarea |
| `task_id`, `chunk_id` | `string` (`minLength: 1`) | Identificadores de tarea y chunk activo |
| `gengis_attempt`, `reset_epoch`, `bounce_cycle` | `int >= 1`, `int >= 0`, `int` (`0..3`) | Intento `GengiSWAP`, época de reset y ciclo de remediación |
| `git` | `object` (`GitBundleInfo`) | `{target_branch, build_branch, base_ref (40-hex), worktree_tree_sha256 (64-hex), changed_files: []ChangedFileEntry{path, status: "added"\|"modified"\|"deleted"\|"renamed", mode: ^(0644\|0755\|0000)$, sha256: ^([0-9a-f]{64}\|DELETED)$}, diff_rel_path (relativo a 3_Builder/runtime/, ej. "<task_id>/bundles/<chunk>_a1_r0_c0.diff"), diff_sha256 (64-hex), diff_bytes (0..2097152)}` |
| `scope` | `object` (`ScopeInfo`) | `{can_modify: []string, read_only: []string, forbidden: []string}` |
| `brain_context` | `object` (`BundleBrainInfo`) | `{brain_db_mtime, risk_zones_sha256, highest_risk_level, matched_docs: []BrainDocSummary, risk_hits: []bfcore.RiskHit}` |
| `tdd_evidence` | `object` (`TDDEvidence`) | `{red: RedEvidence (exit_code != 0), green: []CommandEvidence (minItems: 1, todos exit_code == 0)}` |
| `reviewer_applicability` | `object` (`ReviewerApplicability`) | `{security: AxisApplicability, code_architecture: AxisApplicability, ux: AxisApplicability, ui: AxisApplicability}` donde `AxisApplicability = {applicable: bool, mode: "always"\|"auto"\|"included", reason: string, profile_role: string}` |
| `previous_bounces` | `[]string` | Lista de `order_id` previos resueltos en este intento (`[]` en ciclo `0`) |

**Reglas deterministas de `reviewer_applicability` (evaluadas sobre `strings.Split(bfcore.NormalizeCoverPath(p), "/")`):**
*   `security` (`profile_role: "reviewer_security"`) y `code_architecture` (`profile_role: "reviewer_code"`): siempre `{applicable: true, mode: "always", reason: "always_required"}`.
*   `ui` (`profile_role: "reviewer_ui"`): `auto` da `true` (`reason: "gui_surface_detected"`) si algún `changed_files[]` no borrado tiene extensión `.html`, `.htm`, `.css`, `.scss`, `.sass`, `.less`, `.jsx`, `.tsx`, `.vue`, `.svelte`, `.xaml`, `.qml`, `.ui` o contiene alguno de los segmentos de directorio `"ui"`, `"frontend"`, `"web"`, `"views"`, `"components"`, `"pages"`, `"static"`. Si `--include-ui` está presente y `auto` era `false`, pasa a `{applicable: true, mode: "included", reason: "cli_include_ui"}`; si no, `{applicable: false, mode: "auto", reason: "no_gui_files_in_changed_files"}`.
*   `ux` (`profile_role: "reviewer_ux"`): `auto` da `true` (`reason: "user_surface_detected"`) si `ui.applicable == true`, o si algún `changed_files[]` contiene los segmentos `"cmd"`, `"cli"`, `"skills"`, o su nombre base es `"README.md"` (insensible a mayúsculas) o termina en `".schema.json"`. Si `--include-ux` está presente y `auto` era `false`, pasa a `{applicable: true, mode: "included", reason: "cli_include_ux"}`; si no, `{applicable: false, mode: "auto", reason: "internal_library_no_user_surface"}`.

#### 4.3 Contrato de Entrada desde `5_Cycle_Approver`: `BounceOrder` (`3_Builder/schemas/bounce_order.schema.json`)

| Campo | Tipo JSON / Formato | Restricciones e Invariantes Cruzados en `apply-order` |
| :--- | :--- | :--- |
| `schema_version` | `"1.0"` | Constante estricta |
| `order_id` | `string` (`^[A-Za-z0-9][A-Za-z0-9_-]{2,63}$`) | Identificador único de la orden |
| `created_at` | `string` (`date-time`) | UTC RFC 3339 |
| `trace_id`, `task_id`, `chunk_id`, `bundle_id` | `string` (`minLength: 1`) | Deben coincidir exactamente con el último `DeliveryBundle` activo (`exit 1`, `BUNDLE_MISMATCH`) |
| `bundle_sha256` | `string` (`^[0-9a-f]{64}$`) | Debe coincidir con `session.LatestBundleSHA256` (`exit 1`, `BUNDLE_MISMATCH`) |
| `decision` | `"APPROVE" \| "BOUNCE"` | Veredicto consolidado de `5_Cycle_Approver` |
| `reviewer_verdicts` | `{security, code_architecture, ux, ui}` | Cada eje es `AxisVerdict{status, findings_count (int >= 0), summary (1..240 chars sin \r/\n), na_reason (string, omitempty), model_id (string, omitempty)}`. `security` y `code_architecture` solo admiten `"PASS"\|"FAIL"`; `ux` y `ui` admiten `"NA"` **solo** si `bundle.reviewer_applicability.<axis>.applicable == false` y `na_reason != ""` (`exit 1`, `ORDER_VALIDATION_FAILED`) |
| `requires_red_test` | `boolean` | En `"APPROVE"` debe ser `false` (`ORDER_VALIDATION_FAILED`). En `"BOUNCE"`, `true` invalida `session.tdd.red` para exigir nuevo `tdd-red`; `false` conserva `session.tdd.red` (con `inherited_from_cycle`) |
| `remediation_items` | `[]RemediationItem` | En `"APPROVE"`: `len == 0`, ningún eje `"FAIL"` y todos los ejes (`"PASS"` y `"NA"`) tienen `findings_count == 0`. En `"BOUNCE"`: `len >= 1` y al menos un eje tiene `"FAIL"`. Cada ítem: `{id, axis ∈ {"security","ux","ui","code_architecture"}, severity ∈ {"BLOCKER","MAJOR","MINOR"}, file ("" o ruta relativa saneada con bfcore.SanitizeRelativePaths), line (int >= 0), description (1..500 chars), required_action (1..500 chars)}` |
| `summary` | `string` (`1..200` chars, sin `\r`/`\n`/`"`/`\`/`` ` ``/`$`/`%`) | Resumen limpio `G4` y seguro frente a escapes Windows/shell reutilizado en `--qa-notes` |

#### 4.4 Sobres de Salida CLI (`CLIResult`, `ContextResult`, `VerifyReport`, `ValidationIssue`)

```go
type ValidationIssue struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CLIResult struct {
	OK               bool              `json:"ok"`
	ExitCode         int               `json:"exit_code"`
	Reason           string            `json:"reason"`
	Retryable        bool              `json:"retryable"`
	TraceID          string            `json:"trace_id,omitempty"`
	TaskID           string            `json:"task_id,omitempty"`
	ChunkID          string            `json:"chunk_id,omitempty"`
	Attempt          int               `json:"attempt,omitempty"`
	BounceCycle      int               `json:"bounce_cycle,omitempty"`
	BuildBranch      string            `json:"build_branch,omitempty"`
	WorktreePath     string            `json:"worktree_path,omitempty"` // solo en stdout CLI, nunca en runtime/
	BundleID         string            `json:"bundle_id,omitempty"`
	BundleSHA256     string            `json:"bundle_sha256,omitempty"`
	BundlePath       string            `json:"bundle_path,omitempty"`
	DiffPath         string            `json:"diff_path,omitempty"`
	Decision         string            `json:"decision,omitempty"`
	CommitRef        string            `json:"commit_ref,omitempty"`
	NextAutorCommand string            `json:"next_autor_command,omitempty"`
	Reset            bool              `json:"reset,omitempty"`
	Issues           []ValidationIssue `json:"issues,omitempty"`
}

type ContextResult struct {
	OK                 bool                 `json:"ok"`
	ExitCode           int                  `json:"exit_code"`
	Reason             string               `json:"reason"`
	Retryable          bool                 `json:"retryable"`
	TraceID            string               `json:"trace_id"`
	TaskID             string               `json:"task_id"`
	WorktreePath       string               `json:"worktree_path"`
	BuildBranch        string               `json:"build_branch"`
	TargetBranch       string               `json:"target_branch"`
	PlanApproved       bool                 `json:"plan_approved"`
	ChunkID            string               `json:"chunk_id"`
	Attempt            int                  `json:"attempt"`
	BounceCycle        int                  `json:"bounce_cycle"`
	BaseRef            string               `json:"base_ref"`
	Spec               string               `json:"spec"`
	Acceptance         []string             `json:"acceptance"`
	VerifyCommands     []string             `json:"verify_commands"`
	FailureProof       string               `json:"failure_proof"`
	EffectiveCanModify []string             `json:"effective_can_modify"`
	EffectiveReadOnly  []string             `json:"effective_read_only"`
	EffectiveForbidden []string             `json:"effective_forbidden"`
	GengisFeedback     []string             `json:"gengis_feedback"`
	BrainContext       BrainContextSnapshot `json:"brain_context"`
	ActiveBounce       *BounceOrder         `json:"active_bounce,omitempty"`
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
	ArtifactsChecked int               `json:"artifacts_checked"` // cuenta cada .json, .jsonl y .diff bajo dataDir
	Issues           []ValidationIssue `json:"issues"`
}
```

### 5. Matriz de Subcomandos CLI, Banderas Acotadas y Precedencias Paso a Paso

Resolución de raíces y binarios:
*   `--repo-root <dir>` (default `.`), `--factory-root <dir>` (default `--repo-root`), `--data-dir <dir>` (default `<factory-root>\3_Builder\runtime`).
*   `--vault <dir>` (default `<factory-root>\0_Brain\vault`) y `--brain-bin <exe>` (default `<factory-root>\0_Brain\brain.exe`, con fallback POSIX `<factory-root>/0_Brain/brain`) solo en los subcomandos que consultan `0_Brain` (`init`, `context`, `package`, `commit`, `verify`).
*   `--gengis-root <dir>` (default `<factory-root>\GengiSWAP`): en `init`, `commit`: binario resuelto desde `os.Getenv("GENGIS_BIN")` si no está vacío, o `<gengis-root>\gengis.exe` en Windows (`<gengis-root>/gengis` fallback en POSIX).
*   `--worktree-root <dir>` (default `<parent-of-task-root>\worktrees`): aceptado en los subcomandos con `--task-dir` para resolver deterministamente `<worktree-root>\<task_id>` y verificarlo contra `session.WorktreePathSHA256`.

| Subcomando | Flags Obligatorios | Flags Opcionales | Matriz `0_Brain` |
| :--- | :--- | :--- | :--- |
| `init` | `--task-dir <dir>` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--gengis-root`, `--vault`, `--brain-bin`, `--project`, `--branch`, `--base-ref`, `--json` | `Preflight` + `Context` (sobre unión de `can_modify`) |
| `context` | `--task-dir <dir>` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--vault`, `--brain-bin`, `--refresh-brain`, `--json` | `Preflight` + `Context` (en cambio de chunk o `--refresh-brain`; si `can_modify` del chunk es vacío usa `task.Scope.CanModify`) |
| `tdd-red` | `--task-dir <dir>` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--cmd-index <N>` (`0`), `--timeout <1..300>` (`300`), `--json` | *(usa snapshot vigente)* |
| `package` | `--task-dir <dir>` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--vault`, `--brain-bin`, `--include-ui`, `--include-ux`, `--timeout <1..300>` (`300`), `--json` | `Preflight` + `Context` en vivo verificando `risk_zones_sha256` y `matched_doc_hashes[]` (`STALE_BRAIN_CONTEXT`) |
| `apply-order` | `--task-dir <dir>` y (`--order <path>` XOR `--reset-bounces --reason <str>`) | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--json` | *(sin consulta)* |
| `commit` | `--task-dir <dir>` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--gengis-root`, `--vault`, `--brain-bin`, `--timeout <1..300>` (`300`), `--json` | `Preflight` read-only |
| `cleanup` | `--task-dir <dir>` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--force`, `--delete-branch`, `--json` | *(sin consulta)* |
| `verify` | *(ninguno)* | `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json` | `Preflight` read-only (`0` subprocesos `brain.exe`) |

**Precedencia unificada por etapas en subcomandos sobre tarea existente (`context`, `tdd-red`, `package`, `apply-order`, `commit`, `cleanup`):**

1. **`[2]` Uso CLI y reparse previo al cerrojo:** valida banderas obligatorias (`INVALID_USAGE` `2`), exclusión mutua (`--order` vs `--reset-bounces`), rangos `--timeout ∈ 1..300`, `CheckReparseSafety` sobre `--factory-root`, `--repo-root`, `--data-dir`, `--task-dir`, `--vault` (`REPARSE_POINT_DETECTED` → `exit 6`) y, en `apply-order --order`, `CheckNoReparseChain` + `os.Lstat` regular sobre `--order` (`os.ErrNotExist` → `exit 2` `INVALID_USAGE`; symlink/reparse → `exit 6` `REPARSE_POINT_DETECTED`).
2. **`[6a]` Cerrojo `.session.lock` e integridad de sesión/worktree:** adquiere `<dataDir>/<task_id>/.session.lock` (`LOCK_BUSY` `6`), decodifica `session.json` (`os.ErrNotExist` → `exit 2` `SESSION_NOT_FOUND`; JSON inválido/campos fuera de contrato → `exit 6` `SESSION_STATE_CORRUPT`). En comandos distintos de `cleanup` y `--reset-bounces`: verifica `session.WorktreeStatus == "active"`, existencia de `<absWorktree>` (`WORKTREE_MISSING` `6`), coincidencia de `ComputePathIdentity(absWorktree)` con `session.WorktreePathSHA256`, `ComputePathIdentity(absRepo)` con `session.RepoRootSHA256`, y `extractTargetRepo(<taskDir>/index.md) == absWorktreeClean` (`WORKTREE_PATH_MISMATCH` `6`).
3. **`[5]` Freno operativo de rebotes (`tdd-red`, `package`, `apply-order --order`, `commit`):** si `session.BounceBrakeActive == true`, retorna de inmediato `exit 5` (`MAX_BOUNCE_CYCLES`, `retryable: false`).
4. **`[6b]` Preflight de infraestructura externa:** ejecuta `CheckBrainPreflight` cuando corresponde (`BRAIN_JOURNAL_ACTIVE` / `BRAIN_UNAVAILABLE` `6`); en `commit`, comprueba con `CheckNoReparseChain` + `os.Lstat` que `gengis.exe` existe y es archivo regular (`REPARSE_POINT_DETECTED` o `GENGIS_UNAVAILABLE` `6`).
5. **`[4]` Precondiciones del anillo, auto-sincronización y contexto:**
   - En `tdd-red`, `package`, `apply-order --order`, `commit`: exige `.ring/state.json` con `PlanApproved == true` (`PLAN_NOT_APPROVED` `4`), ejecuta `syncSessionWithRingState` (`TASK_TERMINAL_STATE` `4`, `GENGIS_DECIDE_REQUIRED` `4`, `ARTIFACT_STATE_CORRUPT` `6`), verifica `git -C <wt> rev-parse HEAD == session.ActiveBaseRef` (`UNEXPECTED_COMMIT_IN_WORKTREE` `4`).
   - En `package`: re-consulta `0_Brain Context` y verifica igualdad de `risk_zones_sha256` y de cada `matched_doc_hashes[]` contra disco vía `bfcore.ComputeNormalizedFileSHA256` (`STALE_BRAIN_CONTEXT` `4`), y exige `session.TDD.Red != nil` (`TDD_RED_REQUIRED` `4`).
   - En `cleanup`: sin `--force`, exige `.ring/state.json` en `DONE` o `ABORTED` (`TASK_TERMINAL_STATE` `4`); si se pasa `--delete-branch` con estado `DONE` sin `--force`, rechaza antes de borrar nada con `exit 4` (`TASK_TERMINAL_STATE`) para impedir pérdida accidental de los commits aprobados.
6. **`[3]` Barrera `G4` fail-closed (11 reglas):**
   - En `apply-order --order`: verifica cuota `MaxOrderBytes (256 KiB)` (`ORDER_VALIDATION_FAILED` `1`), ejecuta `ScanRawAndDecodedG4` (`G4_VIOLATION` `3`; JSON inválido tras Fase-A → `INVALID_JSON` `1`). En `--reset-bounces`: verifica `ScanG4All(reason) == 0` (`G4_VIOLATION` `3`).
   - En `tdd-red`, `package`, `commit`: calcula `ComputeWorktreeSnapshot` y ejecuta `ScanWorktreeChangesG4` (`G4_VIOLATION` `3`) **antes** de ejecutar cualquier `VerifyCommand` o `gengis.exe check`.
7. **`[1]` Reglas de dominio y ejecución:**
   - `tdd-red`: valida `--cmd-index` (`CMD_INDEX_OUT_OF_RANGE` `2`), `Scope` (`SCOPE_VIOLATION` `1`), `len(changed) >= 1` (`TDD_RED_NO_CHANGES` `1`), `has_test_file` (`TDD_RED_MISSING_TEST_CHANGE` `1`); ejecuta `VerifyCommands[cmdIndex]`; si `ExitCode == 0` → `exit 1` (`TDD_RED_PASSED_UNEXPECTEDLY`); sanitiza salida (`G4_VIOLATION` `3` si Tier 1).
   - `package`: valida `Scope` (`SCOPE_VIOLATION` `1`), `len(changed) >= 1` (`TDD_RED_NO_CHANGES` `1`), `treeSHA != red.RedWorktreeTreeSHA256` y (si `BounceCycle > 0`) `treeSHA != session.LastBouncedWorktreeTreeSHA256` (`TDD_GREEN_UNCHANGED_FROM_RED` `1`), `diffBytes <= MaxDiffBytes` (`DIFF_TOO_LARGE` `1`); ejecuta todos los `VerifyCommands` (si alguno `ExitCode != 0` → `TDD_GREEN_FAILED` `1`); re-ejecuta `ComputeWorktreeSnapshot` + `Scope` + `ScanWorktreeChangesG4` sobre el estado post-comando.
   - `apply-order --order`: exige `session.LatestBundleSHA256 != ""`, `session.ActiveBounceOrderID == ""` y `session.ApprovedBundleSHA256 == ""` (un bundle solo puede resolverse una vez; re-envío sobre un bundle ya rebotado o aprobado → `ORDER_VALIDATION_FAILED` `1`), rechaza cualquier `order_id` ya presente en `session.SeenOrderIDs` (`ORDER_VALIDATION_FAILED` `1`), y valida `DisallowUnknownFields()` e invariantes de §4.3 (`BUNDLE_MISMATCH` `1` / `ORDER_VALIDATION_FAILED` `1`). Si `APPROVE`, archiva `bounces/<chunk>_a<N>_r<E>_c<C>_approve.order.json`. Si `BOUNCE` y `session.BounceCycle >= 3`: archiva `bounces/<chunk>_a<N>_r<E>_b4_brake.order.json`, marca `BounceBrakeActive = true`, guarda `session.json` y devuelve `exit 5` (`MAX_BOUNCE_CYCLES`). En `--reset-bounces`, incrementa `ResetEpoch++`, reinicia `BounceCycle = 0`, `BounceBrakeActive = false`, `ActiveBounceOrderID = ""`, `RequiresRedTest = true`, `TDD.Red = nil` y `Approved* = ""` conservando `SeenOrderIDs`, `PreviousBounceIDs` y `LastBouncedWorktreeTreeSHA256`.
   - `commit`: exige `session.ApprovedBundleSHA256 != ""` (`APPROVAL_REQUIRED` `1`) y `currentTreeSHA == session.ApprovedWorktreeTreeSHA256` (`WORKTREE_DRIFT_AFTER_APPROVAL` `1`), valida `Scope` (`SCOPE_VIOLATION` `1`), captura `mainHeadBefore`, ejecuta `gengis.exe check <taskDir> --root <gengisRoot>` (error de arranque/ejecución sin actualizar `state.json` → `GENGIS_UNAVAILABLE` `6`), verifica `mainHeadAfter == mainHeadBefore` (`GIT_ERROR` `6`), recarga `.ring/state.json`: si `att.QADone && !att.QAPassed` → `exit 4` (`GENGIS_DECIDE_REQUIRED`, `retryable: false`); verifica `att.CommitRef != ""` y `git -C <wt> rev-parse HEAD == att.CommitRef` (`GIT_ERROR` `6`), e imprime el comando entrecomillado `"` + gengisBin + `" decide "` + absTaskDir + `" --root "` + absGengisRoot + `" --qa-pass=true --qa-notes="..."`.
   - `cleanup`: sin `--force`, verifica worktree limpio (`WORKTREE_DIRTY` `1`); bajo `.repo-<sha16>.lock` ejecuta `git worktree remove` + `prune` (+ `branch -D` si `--delete-branch`).
8. **`[0]` Persistencia atómica (`session.json`, bundle/order si aplica), telemetría `builder_events.jsonl` y salida `exit 0`.**

### 6. Prioridad Fija en `builder verify` y Hashes `RZ-001` Anclados

`builder verify` opera sin `--task-dir`, con **0 cerrojos exclusivos y 0 escrituras en disco**, evaluando todos los chequeos y seleccionando `(ExitCode, Reason)` con prioridad determinista fija:
`REPARSE_POINT_DETECTED` (`6`) > `G4_VIOLATION` (`3`) > `GITIGNORE_DRIFT` (`6`) > `GENGIS_ENGINE_DRIFT` (`6`) > `SESSION_STATE_CORRUPT` (`6`) > `ARTIFACT_STATE_CORRUPT` (`6`) > `BRAIN_JOURNAL_ACTIVE` (`6`) > `BRAIN_UNAVAILABLE` (`6`) > `IO_ERROR` (`6`).

*   **`.gitignore` individual (`CheckGitIgnore`):** ejecuta `git -C <factoryRoot> check-ignore -q 3_Builder/runtime/` y `git -C <factoryRoot> check-ignore -q 3_Builder/builder.exe` **más** verificación de que `.gitignore` declara explícitamente la línea `3_Builder/builder.exe` (evitando falso positivo por la regla global `*.exe` de `.gitignore:3`).
*   **Hashes SHA-256 normalizados CRLF/BOM (`bfcore.ComputeNormalizedFileSHA256`) anclados para `RZ-001`:**
    1. `GengiSWAP/engine/bridge.go`: `8c72badfb4fba74c5f81fd0e5c14687532ea7b5e95b8bc1ec0331390d54beb82`
    2. `GengiSWAP/engine/ring.go`: `8bb4c7bda1ec71536c95e2c57e771d25a0856de7ee9d06327d66dc81fa57b26f`
    3. `GengiSWAP/engine/runner.go`: `776a2858c7e938b5e6ec3861539f78361cb9cae56a9682b824e8792688a20ec7`
    4. `GengiSWAP/engine/runner_windows.go`: `d5c289942c01722dd2ab4e92ecafea15f366aaf2c803b1c49c627f9fe1c20d62`
    5. `GengiSWAP/engine/cmd/gengis/main.go`: `4ba109ad252503325f726f39dd36166beec3693bcd134f5fc8df051c2cb523a8`
*   **Escaneo de `dataDir` (`3_Builder/runtime/`):** si no existe en un checkout limpio, pasa con `ok: true, artifacts_checked: 0`. Si existe, ignora `*.lock`, incrementa `artifacts_checked` por cada `.json`, `.jsonl` y `.diff`:
    - `session.json`: `ScanRawAndDecodedG4` (`G4_VIOLATION` `3`) + `parseSessionStrict` (`SESSION_STATE_CORRUPT` `6`).
    - Otros `*.json` (`bundles/*.bundle.json`, `bounces/*.order.json`): `ScanRawAndDecodedG4` (`G4_VIOLATION` `3`) + sintaxis/contrato estricto (`ARTIFACT_STATE_CORRUPT` `6`).
    - `*.jsonl`: `ScanG4All` crudo y por línea decodificada (`G4_VIOLATION` `3`) + JSON válido por línea (`ARTIFACT_STATE_CORRUPT` `6`).
    - `*.diff`: `ScanG4All` sobre el archivo completo (`G4_VIOLATION` `3`).

### 7. Perfiles Agnósticos de Modelos en `3_Builder/skills/`

`3_Builder/skills/builder-execution/profiles.example.json` define una plantilla declarativa para el harness externo (`schema_version: "1.0"`, objeto `roles` con claves `builder`, `reviewer_security`, `reviewer_ux`, `reviewer_ui`, `reviewer_code`, `cycle_approver`, cada una con `{conceptual_reference, family_constraint, clean_context_required}`), nunca leída por `builder.exe` (`NG1`):

| Rol (`profile_role`) | Equipo | Referencia conceptual | Restricción de contexto y diversidad en el harness |
| :--- | :---: | :--- | :--- |
| `builder` | `3_Builder` | Opus | Recibe salida de `builder context` y opera exclusivamente en `<worktree_path>` |
| `reviewer_security` | `4_Revisores` | Sol / Codex | Contexto 100% limpio (`DeliveryBundle` + `.diff` + `0_Brain`); familia distinta a `builder` |
| `reviewer_ux` | `4_Revisores` | Sol | Contexto 100% limpio; se marca `status: "NA"` si `ux.applicable == false` |
| `reviewer_ui` | `4_Revisores` | Astra (multimodal) | Contexto 100% limpio; se marca `status: "NA"` si `ui.applicable == false` |
| `reviewer_code` | `4_Revisores` | Sonnet | Contexto 100% limpio; audita arquitectura, modularidad y rigor `G5` frente a `0_Brain` |
| `cycle_approver` | `5_Cycle_Approver` | Opus | Consolida los 4 dictámenes y emite `BounceOrder` v1.0 ligado a `bundle_sha256` |

--------------------------------------------------------------------------------

## Alternatives Considered

### Alternative A: Modificar `GengiSWAP/engine/` (`RZ-001`) para añadir `--repo`, `git worktree`, TDD y bucle de revisores
*   **Pros:** Un único binario concentraría el anillo de commits y el aislamiento de worktree.
*   **Cons:** Viola directamente la frontera `CRITICO` de `RZ-001` (`0_Brain/vault/wiki/risk_zones.md`), rompe `PinnedBridgeGoSHA256` y `PinnedRingGoSHA256` en `2_Spec_Writer/engine/verify.go:18-22` y mezcla en una sola entrega la construcción de Equipo 3 con la deuda `TD-004`.
*   **Reason for Rejection:** Rechazada en Decisión `Q2`; la vinculación de una sola línea en `<taskDir>/index.md` logra que `gengis.exe plan`, `check` y `decide` operen 100% sobre el worktree con `0` bytes modificados en `RZ-001`.

### Alternative B: Invocar `gengis.exe check` antes de `4_Revisores` y usar `gengis.exe decide` (`RETRY_CHUNK`) para cada rebote
*   **Pros:** Reutilizaría el contador de intentos de `GengiSWAP` como contador de rebotes.
*   **Cons:** `CheckChunk` (`GengiSWAP/engine/bridge.go:1342-1349`) bloquea toda nueva corrida hasta que `[AUTOR]` ejecute manualmente `gengis.exe decide` por consola y ya habrá creado commits intermedios en `qaStep` (`ring.go:661-665`). Cualquier hallazgo de `4_Revisores` interrumpiría al humano y ensuciaría el historial Git.
*   **Reason for Rejection:** Separar el ciclo interno (`tdd-red → package → apply-order` hasta `3` rebotes autónomos sin commits) del sellado final (`commit → gengis.exe check`) preserva la autonomía entre Equipos 3–4–5 y deja a `[AUTOR]` una sola decisión limpia por chunk.

### Alternative C: Acoplar `builder init --spec <spec.json>` y dividir el estado en `session.json` + `brain_context.json` + `tdd_state.json` + `receipts/`
*   **Pros:** Permitiría leer `failure_proof.expect_exit` directo desde `2_Spec_Writer/runtime/` y separar sub-estados en archivos distintos.
*   **Cons:** Obliga a duplicar en `3_Builder` los 20 campos y reglas de `CanonicalSpecSHA256` de `2_Spec_Writer` (`NG5`), introduce fragilidad de mapeo `01_<id>` vs `<id>` tras `RESPLIT_CHUNK`, multiplica por 3 los `fsync`/`Rename` en Windows con riesgo de estado desgarrado, y duplica en `receipts/` lo que ya persiste `.ring/state.json`.
*   **Reason for Rejection:** Consolidar todo el estado mutable bajo un único `session.json` atómico y parametrizar `--project` elimina acoplamiento entre equipos y reduce superficie de fallo.

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Seguridad, `G4`, Reparse Safety y Cuotas:**
    - Cero rutas absolutas de host dentro de `3_Builder/runtime/` (`ComputePathIdentity` guarda únicamente basename saneado + SHA-256).
    - `CheckNoReparseChain` obligatorio sobre raíces, `.git`, worktree, archivos tocados y `--order`; rechazo de modos Git `120000` y `160000`.
    - Cuotas estrictas contra agotamiento de memoria/disco: `MaxObservedFileBytes = 10 MiB` por archivo, `MaxDiffBytes = 2 MiB` por diff unificado, `MaxOrderBytes = 256 KiB` para `BounceOrder`, `MaxCommandOutputBytes = 8000` bytes y `MaxVerifyTimeoutSec = 300` segundos.
    - Orden global estricto de cerrojos: `.session.lock` → `.repo-<sha16>.lock` → `.events.lock` (backoff `250 ms` vía `bfcore.AcquireFileLock`).
*   **Presupuesto de Latencia:**
    - Overhead propio de `builder.exe` (`context`, `apply-order`, `verify` hasta `50` artefactos en `runtime/` y validaciones estáticas previas a subprocesos) `< 500 ms` p95 local en Windows, con máximo 1 subproceso read-only a `0_Brain/brain.exe` por comando (`0` en `verify`).
*   **Observabilidad y Auditoría:**
    - Bitácora append-only `3_Builder/runtime/builder_events.jsonl` propagando `trace_id`, `task_id`, `chunk_id`, `attempt`, `bounce_cycle`, `exit_code`, `reason`, `retryable`, `bundle_sha256` y `duration_ms`, sanitizada con `bfcore.RedactAllG4` y validada con `bfcore.ScanG4All == 0`.
*   **Despliegue y Rollback:**
    - Binario único autocontenido `3_Builder/builder.exe` (`CGO_ENABLED=0`). Ante fallo parcial en `init`, rollback automático completo de worktree, rama e `index.md`. Para desinstalar o revertir Equipo 3 basta retirar `./3_Builder/engine` de `go.work` y revertir la rama `feat/3-builder` sin impacto alguno en `0_Brain`, `1_Intake`, `2_Spec_Writer` ni `GengiSWAP`.

--------------------------------------------------------------------------------

## Risks, Limits & Mitigations

| Riesgo / Límite en v1 | Impacto | Mitigación Determinista en v1 | Disparador de Evolución (v2) |
| :--- | :---: | :--- | :--- |
| `VerifyCommands` que invocan binarios locales `Opción i` (`brain.exe`, `intake.exe`, `specwriter.exe`) ausentes en el worktree | Alto | `init` inspecciona todos los `VerifyCommands` antes de crear el worktree y falla cerrado con `exit 2` (`TASK_DIR_INVALID`) si referencian binarios `Opción i` no rastreados en `HEAD` | Inyección controlada de variables `BF_FACTORY_ROOT` en el allowlist de `2_Spec_Writer` v2 |
| Manipulación externa de `<taskDir>/index.md` tras `init` redirigiendo `gengis.exe` al repo principal | Crítico | `context`, `tdd-red`, `package` y `commit` re-verifican `extractTargetRepo(index.md) == absWorktreeClean` (`WORKTREE_PATH_MISMATCH` `6`), y `commit` verifica `mainHeadAfter == mainHeadBefore` | Adición de bandera `--repo` nativa en `GengiSWAP` cuando `[AUTOR]` aborde `TD-004` (`RZ-001`) |
| Edición sutil (CRLF/LF, BOM o permisos) entre `APPROVE` y `commit` | Crítico | `worktree_tree_sha256` hashea bytes crudos en disco + modo octal regular, bloqueando `commit` con `WORKTREE_DRIFT_AFTER_APPROVAL` (`exit 1`) | Firma criptográfica Ed25519 sobre `DeliveryBundle` y `BounceOrder` en entornos multi-operador |
| Carreras Git entre tareas paralelas sobre `.git/worktrees` y `.git/refs` | Alto | Cerrojo por repositorio `<dataDir>/.repo-<sha16>.lock` alrededor de `worktree add/remove/prune` y `branch -D`, respetando `.session.lock → .repo.lock → .events.lock` | Daemon coordinador de cola si se superan `> 8` worktrees simultáneos |
| `git stash` compartido entre worktrees durante `DecideChunk` con revert en `GengiSWAP` | Medio | `package` y `commit` exigen `HEAD == base_ref` y solo invocan `gengis.exe check` cuando `ScopeViolations == 0`, evitando disparos accidentales de `stash` por archivos fuera de alcance | Aislamiento sin `git stash` al abordar `TD-004` en `GengiSWAP` |
| Freno `session.json` editable por un operador con acceso directo al sistema de archivos local | Bajo | Documentado explícitamente como **freno operativo determinista contra bucles de agentes** (paridad con `2_Spec_Writer` `attempts.json`), no como frontera contra el administrador local del host | Estado firmado por servicio externo si se despliega ejecución desatendida multi-tenant |

--------------------------------------------------------------------------------

## Additional Resources

*   [`Docs/designs/2_spec_writer.md`](2_spec_writer.md) — Especificación aprobada de `2_Spec_Writer` (contratos de compilación `index.md`/`chunks/*.md` y códigos `0..6`).
*   [`Docs/PROYECTO_TUNGSTENO.md`](../PROYECTO_TUNGSTENO.md) — Auditoría línea por línea de `GengiSWAP/engine/` (§5.1–§5.7).
*   [`Docs/TECH_DEBT.md`](../TECH_DEBT.md) — Registro fechado de deuda técnica diferida (`TD-001`..`TD-005`).
*   [`GengiSWAP/gengis_swap_common.md`](../../GengiSWAP/gengis_swap_common.md) — Protocolo común del anillo determinista y semántica de `.ring/state.json` / `.ring/base_ref`.
*   [`pkg/bfcore/README.md`](../../pkg/bfcore/README.md) — Primitivas compartidas pure-Go (`G4`, higiene de rutas, cliente `0_Brain`, locks y reparse safety).

--------------------------------------------------------------------------------

## Future Improvements

*   **Extracción compartida de persistencia JSON `G4` a `pkg/bfcore` (`v2`):** Cuando `[AUTOR]` programe el cierre conjunto de `TD-001`..`TD-003`, unificar `ScanRawAndDecodedG4` y `WriteJSONAtomicG4` de `2_Spec_Writer` y `3_Builder` en `pkg/bfcore`.
*   **Bandera `--repo` y `QA_OVERRIDE` en `GengiSWAP` (`TD-004`):** Al abrir una ventana autorizada sobre `RZ-001`, permitir pasar `--repo <worktree>` directamente a `gengis.exe` eliminando la reescritura de la línea `Repositorio destino:` en `index.md`.
*   **Integración directa con binarios de `4_Revisores`, `5_Cycle_Approver` y `6_Filtro_de_Riesgo`:** Conectar los artefactos `DeliveryBundle` v1.0 y `BounceOrder` v1.0 con los motores deterministas de los Equipos 4, 5 y 6 sin cambiar los esquemas publicados en `3_Builder/schemas/`.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

### Target Files to Create/Modify

La implementación se divide en **4 chunks disjuntos y cohesivos** para `GengiSWAP` (`bf_03_builder_v1`), manteniendo `0` bytes modificados en `0_Brain/**`, `1_Intake/**`, `2_Spec_Writer/**`, `pkg/bfcore/**` y `GengiSWAP/engine/**`:

#### Chunk 1 (`01_core_schemas_and_git`) — Módulo, Esquemas Canónicos, Estado Atómico, Parser, Git Worktree y Runner

#### 1. `go.work` (Modify) y `.gitignore` (Modify)
*   **Type / Scope**: Root Workspace & Option i Configuration.
*   **Proposed Changes**:
    - `go.work`: añadir `./3_Builder/engine` en `use (...)`.
    - `.gitignore`: añadir explícitamente `3_Builder/runtime/` y `3_Builder/builder.exe` bajo la sección `Opción i`.

#### 2. `3_Builder/schemas/delivery_bundle.schema.json` (Create) y `3_Builder/schemas/bounce_order.schema.json` (Create)
*   **Type / Scope**: JSON Schema Draft 2020-12 Contracts (`schema_version: "1.0"`, `additionalProperties: false` en todos los niveles).
*   **Proposed Changes**:
    - `delivery_bundle.schema.json`: definir todos los campos, patrones hex/regex y tipos de §4.2.
    - `bounce_order.schema.json`: definir todos los campos de §4.3 con bloques `if/then` para `decision == "APPROVE"` (`requires_red_test: false`, `remediation_items` `maxItems: 0`, `security`/`code_architecture` `"PASS"`) y `decision == "BOUNCE"` (`remediation_items` `minItems: 1`).

#### 3. `3_Builder/engine/{go.mod,go.sum,model.go,state.go,task_parser.go,git_worktree.go,runner.go,runner_windows.go,runner_unix.go,core_test.go}` (Create)
*   **Type / Scope**: Go Core Library & Unit Tests (`blackfactory/builder`, `CGO_ENABLED=0`).
*   **Proposed Changes**:
    - `go.mod` / `go.sum`: `module blackfactory/builder`, `go 1.26.0`, solo `blackfactory/pkg/bfcore` (`replace => ../../pkg/bfcore`) y `golang.org/x/sys v0.49.0`.
    - `model.go`: constantes `ExitOK`..`ExitInfraOrRefusal` (`0..6`), cuotas de NFR, catálogo cerrado `Reason*`, `IsRetryableReason`, todos los structs de §4.1–§4.4 y `CanonicalBundleSHA256`.
    - `state.go`: cerrojos `.session.lock`, `.repo-<sha16>.lock` y `.events.lock` en orden estricto, `ScanRawAndDecodedG4`, `WriteJSONAtomicG4` (invocando `ScanRawAndDecodedG4`), `ReadFileWithShortRetry`, `ComputePathIdentity`, `LoadSessionStrict` y `AppendEvent`.
    - `task_parser.go`: parseo de `index.md`, `chunks/*.md` y `.ring/{state.json,base_ref}`, validación de `VerifyCommands` contra binarios `Opción i`, `Scope.Allows`/`Narrows` (paridad `GengiSWAP/engine/ring.go:88-142`) y `BindIndexTargetRepo` + `ExtractIndexTargetRepo` (paridad `bridge.go:236-290`).
    - `git_worktree.go`: `GitClient` sobre `git` CLI, `ComputeWorktreeSnapshot` (bytes crudos + modo octal, rechazo `120000`/`160000`, manejo de pares `R`/`C`, cuota `10 MiB`/`2 MiB`, diff `-U3` + hunks `??`) y `ScanWorktreeChangesG4` (UTF-16 BOM decode, 11 reglas fail-closed en rutas/`+`/`??`, `ScanTier1` fail-closed + `RedactTier2Text` en contexto/`-` del `.diff` persistido).
    - `runner{,_windows,_unix}.go`: `CommandRunner` + `ShellRunner` (`cmd.exe /S /C`, `PYTHONUTF8=1`, `WaitDelay = 3s`, timeout `1..300s`) y `SanitizeCommandOutputG4` (`RedactAllG4` en comando, `ScanTier1` fail-closed en salida cruda, `RedactTier2Text` previo al truncado UTF-8 de `8000` B, verificación final `ScanG4All == 0` con retorno `error`).
    - `core_test.go`: tests TDD de paridad de tags JSON contra ambos `.schema.json`, `ScanRawAndDecodedG4` (`\uXXXX`), orden de locks, `Scope` case-sensitive, `R`/`C` porcelain, rechazo `120000`/`160000`, UTF-16 BOM `G4`, redacción Tier 2 en contexto de `.diff` y `SanitizeCommandOutputG4`.

---

#### Chunk 2 (`02_lifecycle_and_verify`) — `verify`, `init`, `context` y `cleanup`

#### 4. `3_Builder/engine/{verify.go,init_cmd.go,context_cmd.go,cleanup_cmd.go,lifecycle_test.go}` (Create)
*   **Type / Scope**: Lifecycle Subcommands, Read-Only Auditor & Unit Tests (`blackfactory/builder`).
*   **Proposed Changes**:
    - `verify.go`: `ResolveCommonRoots`, `CheckReparseSafety`, `CheckBrainPreflight`, `CheckGitIgnore` (2 llamadas `git check-ignore -q` + presencia literal de `3_Builder/builder.exe` en `.gitignore`), `CheckGengisEngineHashes` (5 hashes anclados de §6 vía `bfcore.ComputeNormalizedFileSHA256`) y `VerifyRuntime` (`0` cerrojos, `0` escrituras, escaneando `.json`, `.jsonl` y `.diff`).
    - `init_cmd.go`: validaciones 100% read-only previas a mutar (no solapamiento de `--worktree-root`, `index.md` dry-check, `VerifyCommands` sin binarios `Opción i` no rastreados, `rev-parse --verify <ref>^{commit}` 40-hex sin fallback silencioso, `BRANCH_COLLISION`, `0_Brain` `Preflight` + `Context` con `--project`), creación bajo `.repo-<sha16>.lock` en Secuencias A y B y rollback completo (`index.md` + worktree + rama) ante cualquier fallo antes de cerrar `session.json`.
    - `context_cmd.go`: `syncSessionWithRingState`, consulta read-only `0_Brain` (registrando en evento JSONL cambios de `HighestRiskLevel` en `--refresh-brain`), fallback a `task.Scope.CanModify` cuando `chunk.Scope.CanModify` es vacío y emisión de `ContextResult`.
    - `cleanup_cmd.go`: validación de estado terminal (`DONE`/`ABORTED`), guardia `--delete-branch` sobre `DONE` sin `--force` (`exit 4`, `TASK_TERMINAL_STATE`), `WORKTREE_DIRTY` (`exit 1`), y `git worktree remove` + `prune` bajo `.repo-<sha16>.lock`.
    - `lifecycle_test.go`: tests TDD de Secuencias A y B, rechazo de `TASK_ALREADY_ACTIVE`, bloqueo de binarios `Opción i` en `VerifyCommands`, no solapamiento `--worktree-root`, rollback completo en `init` ante fallo simulado de escritura, `context` tras `RETRY_CHUNK`/`GO_NEXT`, `cleanup` preservando rama y `verify` en todos sus códigos de prioridad.

---

#### Chunk 3 (`03_tdd_review_and_commit`) — `tdd-red`, `package`, `apply-order` y `commit`

#### 5. `3_Builder/engine/{tdd_cmd.go,package_cmd.go,bounce_cmd.go,commit_cmd.go,workflow_test.go}` (Create)
*   **Type / Scope**: TDD Engine, Bundle Packager, Review Gate, GengiSWAP Subprocess Sealer & Security/Latency Tests (`blackfactory/builder`).
*   **Proposed Changes**:
    - `tdd_cmd.go`: precedencia §5, verificación `HEAD == ActiveBaseRef`, `Scope`, `G4`, `has_test_file`, ejecución `VerifyCommands[cmdIndex]` exigiendo `ExitCode != 0` y actualización de `session.TDD.Red` (permitiendo re-ejecución que sobrescribe `Red`).
    - `package_cmd.go`: precedencia §5, verificación `STALE_BRAIN_CONTEXT` (`risk_zones_sha256` + `matched_doc_hashes[]`), `TDD_RED_REQUIRED`, `TDD_GREEN_UNCHANGED_FROM_RED` (contra `Red` y contra `LastBouncedWorktreeTreeSHA256`), ejecución de todos los `VerifyCommands` (`ExitCode == 0`), **re-snapshot + `Scope` + `G4` post-comando**, aplicabilidad `auto` por segmentos + `--include-ui`/`--include-ux`, y escritura atómica de `bundles/<chunk>_a<N>_c<C>.{bundle.json,diff}`.
    - `bounce_cmd.go`: implementa `ApplyOrder` (`--order` XOR `--reset-bounces --reason`), invariantes cruzados de §4.3, ciclos `1..3` (`exit 0`), activación de `BounceBrakeActive = true` en el 4.º `BOUNCE` (`exit 5`, `MAX_BOUNCE_CYCLES`) y desbloqueo auditado `BOUNCES_RESET`.
    - `commit_cmd.go`: anti-bypass crudo (`WORKTREE_DRIFT_AFTER_APPROVAL`), `extractTargetRepo(index.md) == absWorktreeClean`, `HEAD == ActiveBaseRef`, invocación de `gengis.exe check`, comprobación `mainHeadAfter == mainHeadBefore`, manejo de `QAPassed == false` (`exit 4`, `GENGIS_DECIDE_REQUIRED`) y emisión del comando entrecomillado `gengis.exe decide`.
    - `workflow_test.go`: tests TDD cubriendo `TDD_RED_PASSED_UNEXPECTEDLY`, `TDD_RED_MISSING_TEST_CHANGE`, re-snapshot post-comando sucio en `package`, ciclos `1..3` + corte al 4.º `BOUNCE` (`exit 5`) + bloqueo transversal `BounceBrakeActive` + `--reset-bounces`, `WORKTREE_DRIFT_AFTER_APPROVAL` por cambio CRLF/LF y cambio de modo, `UNEXPECTED_COMMIT_IN_WORKTREE`, `WORKTREE_PATH_MISMATCH` por edición de `index.md`, `GENGIS_DECIDE_REQUIRED` y `TestLatencyBudgetP95` (`< 500 ms` p95 con 50 artefactos).

---

#### Chunk 4 (`04_cli_contract_and_docs`) — CLI `builder.exe`, Test de Contrato Real `gengis.exe` + Documentación y Skill

#### 6. `3_Builder/engine/cmd/builder/{README.md,main.go,main_test.go}`, `3_Builder/engine/{contract_gengis_test.go,README.md}`, `3_Builder/README.md`, `3_Builder/skills/{README.md,builder-execution/SKILL.md,builder-execution/profiles.example.json}` y `README.md` (Create/Modify)
*   **Type / Scope**: CLI Binary Entry Point, Real `gengis.exe` Contract Suite, Team 3 Docs, Progressive-Load Skill & Root README.
*   **Proposed Changes**:
    - `cmd/builder/main.go` & `main_test.go`: despacho de los 8 subcomandos con banderas acotadas de §5, códigos `0..6`, salida JSON determinista y pruebas E2E de `CUJ 1..4` en `t.TempDir()`.
    - `contract_gengis_test.go`: resuelve `GENGIS_BIN` desde entorno con fallback a `../../GengiSWAP/gengis.exe` (`t.Skip` local solo si el binario no existe; `t.Fatalf` bajo `GENGIS_CONTRACT_STRICT=1`), pasa `--gengis-root` apuntando dentro de `t.TempDir()` (`G3`, sin tocar `GengiSWAP/gengis_global_log.md` real) y ejecuta el ciclo real completo (`init` → `gengis.exe plan` → `context` → `tdd-red` → `package` → `apply-order(BOUNCE)` → remediación → `package` → `apply-order(APPROVE)` → `commit` [`gengis.exe check`] → `gengis.exe decide` [`GO_NEXT`] → `cleanup`), verificando commits en `build/<task_id>`, árbol principal intacto y `.ring/state.json` en `DONE`.
    - `3_Builder/skills/README.md`, `3_Builder/skills/builder-execution/{SKILL.md,profiles.example.json}`, `3_Builder/engine/README.md`, `3_Builder/engine/cmd/builder/README.md`, `3_Builder/README.md` y `README.md`: documentación completa alineada con los 8 subcomandos, restricción de `VerifyCommands` compatibles con worktree, perfiles agnósticos de modelos y comandos de verificación.

### Verification Plan

*   **Compilation / Build (`CGO_ENABLED=0`, cero SQLite y cero cambios en módulos cerrados):**
    ```powershell
    $env:CGO_ENABLED = "0"
    Set-Location "C:\Black Factory\3_Builder\engine"
    go build -o ..\builder.exe .\cmd\builder
    git -C "C:\Black Factory" diff --name-only 0_Brain/ 1_Intake/ 2_Spec_Writer/ pkg/bfcore/ GengiSWAP/engine/
    ```
    *Resultado esperado:* `builder.exe` compila limpio; `3_Builder/engine/go.mod` no contiene `modernc.org/sqlite` ni `gengiswap/ring`; `git diff --name-only` sobre los 5 directorios cerrados devuelve salida vacía.
*   **Static Analysis & Linting (`go vet` en todos los módulos del workspace, sintaxis `cmd.exe`):**
    ```cmd
    set CGO_ENABLED=0&& cd /d "C:\Black Factory\pkg\bfcore" && go vet ./... && cd /d "C:\Black Factory\0_Brain\engine" && go vet ./... && cd /d "C:\Black Factory\1_Intake\engine" && go vet ./... && cd /d "C:\Black Factory\2_Spec_Writer\engine" && go vet ./... && cd /d "C:\Black Factory\3_Builder\engine" && go vet ./...
    ```
*   **Unit Tests, Coverage (`>= 85%`) y Contrato Obligatorio contra `gengis.exe` (`GENGIS_CONTRACT_STRICT=1`):**
    ```cmd
    set CGO_ENABLED=0&& set GENGIS_CONTRACT_STRICT=1&& set GENGIS_BIN=C:\Black Factory\GengiSWAP\gengis.exe&& cd /d "C:\Black Factory\pkg\bfcore" && go test -count=1 -cover ./... && cd /d "C:\Black Factory\0_Brain\engine" && go test -count=1 ./... && cd /d "C:\Black Factory\1_Intake\engine" && go test -count=1 -cover ./... && cd /d "C:\Black Factory\2_Spec_Writer\engine" && go test -count=1 -cover ./... && cd /d "C:\Black Factory\3_Builder\engine" && go test -count=1 -cover ./...
    ```
    *Requisito de cobertura:* `>= 85%` en `blackfactory/builder` y `blackfactory/builder/cmd/builder`, cero regresiones en `0_Brain`, `pkg/bfcore`, `1_Intake` y `2_Spec_Writer`, y `contract_gengis_test.go` ejecutado realmente (`0` skips).
*   **E2E / Health & Privacy Verification (`Opción i` + `RZ-001`):**
    ```powershell
    & "C:\Black Factory\3_Builder\builder.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\3_Builder\runtime" --json
    & "C:\Black Factory\2_Spec_Writer\specwriter.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\2_Spec_Writer\runtime" --json
    git -C "C:\Black Factory" check-ignore -v 3_Builder/runtime/
    git -C "C:\Black Factory" check-ignore -v 3_Builder/builder.exe
    git -C "C:\Black Factory" status --ignored -s 3_Builder/
    ```
    *Resultado esperado:* ambos `verify` devuelven `exit 0` (`ok: true`, `gitignore_ok: true`, `gengis_hashes_ok: true`, `brain_ok: true`, `g4_issues: 0`, `0` archivos creados en `runtime/`).
*   **Latency Budget Verification:**
    ```cmd
    set CGO_ENABLED=0&& cd /d "C:\Black Factory\3_Builder\engine" && go test -count=1 -v -run TestLatencyBudgetP95 ./...
    ```
    *Resultado esperado:* p95 `< 500 ms` local en Windows para `context`, `apply-order` y `verify` (hasta `50` artefactos en `runtime/`).

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
