---
name: builder-execution
description: Bucle operativo TDD para ejecutar builder.exe (init, context, tdd-red, package, apply-order, commit, cleanup, verify), orquestar los 4 revisores en contexto limpio + 5_Cycle_Approver con perfiles agnósticos de modelo, respetar aplicabilidad aditiva UX/UI y recuperarse ante códigos de salida 1..6.
---

# `builder-execution` — Operación Determinista de `3_Builder`

## 1. Principios de Ejecución
- **Aislamiento total:** Todo cambio se realiza únicamente dentro de `worktree_path` devuelto por `builder init` / `builder context` (rama `build/<task_id>`). Nunca edites el repositorio principal (`--repo-root`) directamente ni ejecutes `git commit` manual en el worktree (`UNEXPECTED_COMMIT_IN_WORKTREE`, `exit 4`).
- **Read-Before-Act (`0_Brain`):** Antes de programar en cualquier chunk ejecuta `builder context`. Si `0_Brain` cambia en disco tras `context`, `builder package` detiene con `STALE_BRAIN_CONTEXT` (`exit 4`) y exige `builder context --refresh-brain`.
- **TDD estricto:** Toda remediación o nueva funcionalidad requiere primero `builder tdd-red` (`VerifyCommands[cmd_index]` fallando con `ExitCode != 0`) antes de implementar y empaquetar con `builder package`.
- **Cero bypass de puertas:** `builder.exe` nunca invoca `gengis.exe plan` ni `gengis.exe decide`; esas decisiones pertenecen en exclusiva al `[AUTOR]`.

## 2. Perfiles Agnósticos por Rol
Las capacidades y asignaciones recomendadas residen en [`profiles.example.json`](./profiles.example.json) (referencia para el arnés externo):
- `builder` (`3_Builder`): implementa Red -> Green -> Refactor y corre `builder.exe`.
- `reviewer_security` (`4_Revisores`, contexto limpio, siempre aplicable): audita secretos `G4`, inyecciones, permisos, symlinks/reparse points, escapes shell y límites de alcance.
- `reviewer_code` (`4_Revisores`, contexto limpio, siempre aplicable): audita simplicidad, duplicación, cobertura, deuda técnica y cumplimiento del chunk.
- `reviewer_ux` (`4_Revisores`, contexto limpio, `auto` o `--include-ux`): audita flujos, mensajes de error, CLI, esquemas JSON y fricción operativa.
- `reviewer_ui` (`4_Revisores`, contexto limpio, `auto` o `--include-ui`): audita componentes visuales, accesibilidad y estilos si hay archivos UI.
- `cycle_approver` (`5_Cycle_Approver`, contexto limpio): consolida los 4 dictámenes y emite `BounceOrder` v1.0 (`APPROVE` o `BOUNCE`) vinculado exactamente a `bundle_id` y `bundle_sha256`.

## 3. Reglas de Aplicabilidad UX/UI
`builder package` evalúa `reviewer_applicability` de forma determinista sobre `changed_files[]`:
1. `security` y `code_architecture` son siempre `applicable: true` (`mode: "always"`).
2. `ui` pasa a `applicable: true` (`mode: "auto"`) ante extensiones GUI (`.html`, `.css`, `.scss`, `.tsx`, `.jsx`, `.vue`, `.svelte`, `.xaml`, `.qml`, `.ui`, etc.) o segmentos `ui`, `frontend`, `web`, `views`, `components`, `pages`, `static` (archivos no borrados).
3. `ux` pasa a `applicable: true` (`mode: "auto"`) si `ui.applicable == true` o si hay segmentos `cmd`, `cli`, `skills`, nombre `README.md` o archivos `*.schema.json`.
4. Las banderas `--include-ui` e `--include-ux` en `builder package` son **exclusivamente aditivas** (`mode: "included"`): nunca desactivan una aplicabilidad `auto: true`.
5. Un revisor solo puede emitir `"status": "NA"` en `BounceOrder` para `ux` o `ui` cuando `bundle.reviewer_applicability.<eje>.applicable == false`, indicando obligatoriamente `na_reason != ""`.

## 4. Bucle Completo Paso a Paso
1. **Inicializar tarea (Secuencia A pre-`plan` o Secuencia B post-`plan`):**
   ```powershell
   builder.exe init --task-dir "<taskDir>" --repo-root "<repo>" --json
   ```
   En Secuencia A, solicitar al `[AUTOR]` ejecutar `gengis.exe plan "<taskDir>" --root "<gengisRoot>"`.
2. **Sincronizar chunk y `0_Brain`:**
   ```powershell
   builder.exe context --task-dir "<taskDir>" --repo-root "<repo>" --json
   ```
3. **Fase Red (prueba que falla determinísticamente):** escribir o ampliar prueba en el worktree dentro de `effective_can_modify` y ejecutar:
   ```powershell
   builder.exe tdd-red --task-dir "<taskDir>" --repo-root "<repo>" --json
   ```
4. **Fase Green + Refactor + Empaquetado:** implementar lo mínimo, refactorizar sin salir de alcance y ejecutar:
   ```powershell
   builder.exe package --task-dir "<taskDir>" --repo-root "<repo>" --json
   ```
   Entregar el `.bundle.json` y `.diff` generados a los revisores aplicables en **contexto limpio** junto con sus prompts de rol.
5. **Aplicar orden de `5_Cycle_Approver`:**
   ```powershell
   builder.exe apply-order --task-dir "<taskDir>" --repo-root "<repo>" --order "<order.json>" --json
   ```
   - Si `decision == "BOUNCE"` (`1..3`, `exit 0`): si `requires_red_test == true`, volver al paso 3 (`tdd-red`); si `requires_red_test == false`, corregir en worktree y volver al paso 4 (`package`).
   - Si `decision == "APPROVE"` (`exit 0`): continuar al paso 6 sin editar un solo byte ni fin de línea del worktree.
6. **Sellar entrega con `GengiSWAP`:**
   ```powershell
   builder.exe commit --task-dir "<taskDir>" --repo-root "<repo>" --json
   ```
   Presentar `next_autor_command` literal al `[AUTOR]` para que ejecute `gengis.exe decide`. Tras `GO_NEXT`, repetir desde el paso 2 para el siguiente chunk.
7. **Limpieza terminal (`DONE` o `ABORTED`):**
   ```powershell
   builder.exe cleanup --task-dir "<taskDir>" --repo-root "<repo>" --json
   ```

## 5. Recuperación Determinista por Código de Salida (`1..6`)

| Código | Razones típicas | Acción de recuperación |
| :---: | :--- | :--- |
| `1` | `SCOPE_VIOLATION`, `TDD_RED_PASSED_UNEXPECTEDLY`, `TDD_RED_MISSING_TEST_CHANGE`, `TDD_GREEN_FAILED`, `TDD_GREEN_UNCHANGED_FROM_RED`, `ORDER_VALIDATION_FAILED`, `BUNDLE_MISMATCH`, `WORKTREE_DRIFT_AFTER_APPROVAL`, `WORKTREE_DIRTY` | Corregir en el worktree o en el `BounceOrder` y reintentar; ante drift post-`APPROVE` o test roto, re-empaquetar (`package`) y re-aprobar. |
| `2` | `INVALID_USAGE`, `SESSION_ALREADY_EXISTS`, `SESSION_NOT_FOUND`, `TASK_DIR_INVALID`, `CMD_INDEX_OUT_OF_RANGE` | Corregir banderas/rutas o verificar que `builder init` fue ejecutado y `VerifyCommands` sean compatibles con worktree. |
| `3` | `G4_VIOLATION` | Eliminar de inmediato el secreto, credencial, token o ruta personal en diff/stdout/order antes de reintentar (nunca consume ciclo de rebote). |
| `4` | `PLAN_NOT_APPROVED`, `STALE_BRAIN_CONTEXT`, `TDD_RED_REQUIRED`, `GENGIS_DECIDE_REQUIRED`, `TASK_TERMINAL_STATE`, `UNEXPECTED_COMMIT_IN_WORKTREE`, `BRANCH_COLLISION` | Escalar o sincronizar: esperar `[AUTOR]` (`plan`/`decide`), correr `builder context --refresh-brain`, ejecutar `builder tdd-red` o revertir commit manual no autorizado en el worktree. |
| `5` | `MAX_BOUNCE_CYCLES` | Límite de 3 rebotes alcanzado (`BounceBrakeActive == true`). Escalar al `[AUTOR]` y desbloquear explícitamente con `builder.exe apply-order --task-dir "<dir>" --reset-bounces --reason "<motivo >= 8 chars>"`. |
| `6` | `BRAIN_JOURNAL_ACTIVE`, `BRAIN_UNAVAILABLE`, `LOCK_BUSY`, `GIT_ERROR`, `GENGIS_UNAVAILABLE`, `REPARSE_POINT_DETECTED`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT`, `SESSION_STATE_CORRUPT`, `ARTIFACT_STATE_CORRUPT` | Detener ejecución. Resolver causa raíz de infraestructura (`0_Brain` journal, cerrojos colgados, integridad de `.gitignore` / `RZ-001`, symlinks/reparse points) y validar con `builder.exe verify --json`. |
