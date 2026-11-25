# CLI `builder.exe` (`blackfactory/builder/cmd/builder`)

Punto de entrada ejecutable para operar `3_Builder` desde arneses externos o terminal.

## Subcomandos

| Subcomando | Banderas obligatorias | Banderas opcionales principales | Descripción |
| :--- | :--- | :--- | :--- |
| `init` | `--task-dir` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--gengis-root`, `--vault`, `--brain-bin`, `--project`, `--branch`, `--base-ref`, `--json` | Crea rama `build/<task_id>` + worktree aislado, enlaza `index.md` y captura snapshot inicial de `0_Brain` con rollback total ante fallo. |
| `context` | `--task-dir` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--vault`, `--brain-bin`, `--refresh-brain`, `--json` | Sincroniza `.ring/state.json` / `.ring/base_ref` y retorna `ContextResult` con snapshot de `0_Brain` y orden `BOUNCE` activa. |
| `tdd-red` | `--task-dir` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--cmd-index`, `--timeout`, `--json` | Valida alcance, `G4` y archivo de prueba; exige fallo determinista (`ExitCode != 0`) de `VerifyCommands[cmd_index]` y guarda evidencia Red. |
| `package` | `--task-dir` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--vault`, `--brain-bin`, `--include-ui`, `--include-ux`, `--timeout`, `--json` | Verifica frescura `0_Brain`, corre todos `VerifyCommands`, re-ejecuta snapshot/alcance/`G4` post-comando y emite `DeliveryBundle` v1.0 + `.diff`. |
| `apply-order` | `--task-dir` y (`--order` XOR `--reset-bounces --reason`) | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--json` | Valida y archiva `BounceOrder` v1.0 (`APPROVE` o `BOUNCE` `1..3`, freno `exit 5` al 4.º rebote) o desbloquea `ResetEpoch++`. |
| `commit` | `--task-dir` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--gengis-root`, `--vault`, `--brain-bin`, `--timeout`, `--json` | Exige orden `APPROVE` y cero deriva de bytes crudos/modo, ejecuta `gengis.exe check` y emite comando `gengis.exe decide` para `[AUTOR]`. |
| `cleanup` | `--task-dir` | `--repo-root`, `--factory-root`, `--data-dir`, `--worktree-root`, `--force`, `--delete-branch`, `--json` | Retira worktree tras cierre `DONE`/`ABORTED` preservando rama `build/<task_id>` por defecto. |
| `verify` | *(ninguno)* | `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json` | Auditoría 100% solo lectura de `.gitignore`, 5 hashes `GengiSWAP/engine` (`RZ-001`), `0_Brain` preflight y `runtime/`. |

## Códigos de Salida (`0..6`)
- `0`: `OK` (en `apply-order` con `BOUNCE` `1..3` avanza ciclo y retorna `0`).
- `1`: fallo corregible (`SCOPE_VIOLATION`, `TDD_RED_PASSED_UNEXPECTEDLY`, `TDD_GREEN_FAILED`, `ORDER_VALIDATION_FAILED`, `BUNDLE_MISMATCH`, `WORKTREE_DRIFT_AFTER_APPROVAL`, `WORKTREE_DIRTY`).
- `2`: error de uso / entrada (`INVALID_USAGE`, `SESSION_ALREADY_EXISTS`, `SESSION_NOT_FOUND`, `TASK_DIR_INVALID`, `CMD_INDEX_OUT_OF_RANGE`).
- `3`: violación de higiene/secretos `G4` fail-closed (`G4_VIOLATION`).
- `4`: decisión externa / bloqueo de anillo (`PLAN_NOT_APPROVED`, `STALE_BRAIN_CONTEXT`, `TDD_RED_REQUIRED`, `GENGIS_DECIDE_REQUIRED`, `TASK_TERMINAL_STATE`, `UNEXPECTED_COMMIT_IN_WORKTREE`, `BRANCH_COLLISION`).
- `5`: freno operativo tras 3 ciclos `BOUNCE` (`MAX_BOUNCE_CYCLES`).
- `6`: infraestructura / integridad (`BRAIN_JOURNAL_ACTIVE`, `BRAIN_UNAVAILABLE`, `LOCK_BUSY`, `GIT_ERROR`, `GENGIS_UNAVAILABLE`, `REPARSE_POINT_DETECTED`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT`, `SESSION_STATE_CORRUPT`, `ARTIFACT_STATE_CORRUPT`, `WORKTREE_PATH_MISMATCH`, `WORKTREE_MISSING`, `TASK_ALREADY_ACTIVE`, `IO_ERROR`).
