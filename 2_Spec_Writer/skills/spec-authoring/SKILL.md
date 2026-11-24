---
name: spec-authoring
description: Bucle determinista de autoría, validación estática, revisión Goldfish (ASDD) y compilación hacia GengiSWAP mediante 2_Spec_Writer/specwriter.exe.
version: "1.0"
owner: 2_Spec_Writer
---

# Skill: `spec-authoring` (Autoría Determinista de `spec.json` y Compilación a `GengiSWAP`)

## Cuándo Cargar Esta Skill

Cargar **siempre que exista un `WorkItem` con `status: "ready"` emitido por `1_Intake`** que deba convertirse en una tarea compilada y verificable para `GengiSWAP` (`3_Builder`).

## Bucle Operativo Estándar

1. **Preparar contexto congelado (`prepare`):**
   ```text
   2_Spec_Writer\specwriter.exe prepare --workitem 1_Intake\runtime\workitems\WI-<hash12>.json --repo-root . --json
   ```
   Crea `2_Spec_Writer/runtime/<project>/<WI-id>/{spec_context.json, attempts.json, spec.json}`.

2. **Completar `spec.json` (única fuente de verdad, `additionalProperties: false`):**
   - `work_units[].can_modify`: rutas de archivo concretas relativas al repositorio (sin globs `*?[`, sin sufijo `/`, sin backticks).
   - `observed_files[]`: cubrir exactamente `WorkItem.affected_paths ∪ g5_citations[].path ∪ work_units[].can_modify` indicando `state: "present"` (con SHA-256 normalizado CRLF/BOM) o `state: "absent"` (`sha256: ""`).
   - `g5_citations[]`: `>= 1` cita verificable en disco (`symbol_anchor` literal `>= 4` caracteres no triviales dentro de `±5` líneas).
   - `risk_mitigations[]`: cubrir 100% de los `risk_hits` del `WorkItem` y de la re-consulta en vivo a `0_Brain`; cualquier zona `CRITICO` exige `requires_autor_hitl: true` y `>= 1` `review_gate` enlazado a `acceptance_criteria[].id`.
   - `verify_steps[]`: solo ejecutables/subcomandos/banderas del allowlist cerrado (`go test`, `go vet`, `git status|diff|rev-parse|check-ignore`, `0_Brain/brain.exe query|context`, `1_Intake/intake.exe list|show`, `2_Spec_Writer/specwriter.exe verify|render`); `expect_exit: 0`, `timeout_sec: 1..300`. Para compilar Go sin escribir `.exe`, usar `go test -run=NoTestsMatch ./...`.
   - `decisions[]`: obligatorio ante escalación o ascenso `FTD → ASDD`, con `approved_paths[]` coincidiendo exactamente con la unión de `work_units[].can_modify` y `scope_sha256` igual a `ComputeScopeSHA256(approved_paths)`.

3. **Validar borrador (`validate --stage draft`):**
   ```text
   2_Spec_Writer\specwriter.exe validate --spec 2_Spec_Writer\runtime\<project>\<WI-id>\spec.json --repo-root . --stage draft --json
   ```
   - Si `lane == "FTD"` y sale `0`, la spec está lista para `render` y `compile`.

4. **Revisión Goldfish y validación final (solo `ASDD`):**
   - Ejecutar los lentes requeridos en contexto limpio (`egm-design`, `egm-codebase-details`, `egm-implementation-readiness`, más `egm-structural` si `impact_flags` incluye `contract|schema|db` o `risk_level == "CRITICO"`, y `egm-zoom-out` si `len(work_units) > 3`).
   - Registrar `2_Spec_Writer/runtime/<project>/<WI-id>/goldfish_review.json` con `schema_version: "1.0"`, `spec_sha256`, `lenses_run[]`, `unresolved_blockers: 0`, `report_path` (relativo a `--factory-root`, libre de `G4`) y `reviewed_at`.
   - Ejecutar:
     ```text
     2_Spec_Writer\specwriter.exe validate --spec 2_Spec_Writer\runtime\<project>\<WI-id>\spec.json --repo-root . --stage final --json
     ```

5. **Proyectar vista legible (`render`, `stdout-only`) y compilar (`compile`):**
   ```text
   2_Spec_Writer\specwriter.exe render --spec 2_Spec_Writer\runtime\<project>\<WI-id>\spec.json
   2_Spec_Writer\specwriter.exe compile --spec 2_Spec_Writer\runtime\<project>\<WI-id>\spec.json --repo-root . --task-root C:\Desarrollo\GengiSWAP_data\gengis_task --task-id <task_id> --json
   ```
   Entregar el comando `gengis.exe plan` emitido al `[AUTOR]` (única parada HITL en v1).

## Recuperación ante Códigos de Salida (`1..6`)

| Código | Razón (`reason`) | Acción requerida |
| :---: | :--- | :--- |
| `1` | `VALIDATION_FAILED`, `INVALID_JSON`, `SPEC_TOO_LARGE`, `VALIDATION_REQUIRED` | Corregir los JSON Pointers en `issues[]` y re-ejecutar `validate` (`draft` o `final`). |
| `2` | `INVALID_USAGE`, `SPEC_ALREADY_EXISTS` | Corregir flags CLI o usar `--refresh-context` / `--reset-attempts --reason`. |
| `3` | `G4_VIOLATION`, `G4_VIOLATION_ON_DISK` | Eliminar cualquier secreto o ruta personal (`C:\Users\...`, `/home/...`, correo) del archivo afectado. |
| `4` | `STALE_CONTEXT` | Ejecutar `prepare --refresh-context` y actualizar `observed_files[]` en `spec.json`. |
| `4` | `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED` | Solicitar aprobación `[AUTOR]`, registrar `decisions[]` con `approved_paths` + `scope_sha256` y ascender a `ASDD` si corresponde. |
| `5` | `MAX_VALIDATE_ATTEMPTS`, `MAX_EXTERNAL_CYCLES` | Detener bucle; desbloquear con `[AUTOR]` vía `prepare --reset-attempts --reason "<motivo >= 8 chars>"`. |
| `6` | `BRAIN_JOURNAL_ACTIVE` / `BRAIN_UNAVAILABLE` | Ejecutar `0_Brain/brain.exe verify` o `index` manualmente y reintentar. |
| `6` | `LIVE_TASK_STATE` / `TASK_DIR_EXISTS` | Nunca sobrescribir tareas activas/previas; elegir un nuevo `--task-id` o concluir la tarea previa en `GengiSWAP`. |
| `6` | `GITIGNORE_DRIFT` / `GENGIS_ENGINE_DRIFT` | Restaurar `.gitignore` (`Opción i`) o revisar deriva en `GengiSWAP/engine/{bridge,ring}.go`. |
