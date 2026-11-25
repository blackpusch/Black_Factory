# Equipo 2: `2_Spec_Writer` (Compuerta Determinista de Especificaciones y Compilador hacia `GengiSWAP`)

* **Author:** blackpusch
* **Date:** 2026-11-23
* **Nota:** Copia espejo local en `.dev/docs/` (ignorada por `.gitignore`) del artefacto `concepts_brief.md` de EGM Brainstorm.

## Overview

`2_Spec_Writer` es el segundo equipo de **Black Factory**, ubicado entre `1_Intake` (`1_Intake/runtime/workitems/WI-<hash12>.json`, esquema v1.0) y `3_Builder` (`GengiSWAP/gengis.exe` sobre `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\`), consumiendo `0_Brain` (`0_Brain/brain.exe`) en modo estrictamente de solo lectura (*Read-Before-Act*).

Es el primer equipo del pipeline donde participa un LLM (para leer código real, diseñar interfaces, partir `work_units` y redactar pruebas de fallo), operando bajo una **arquitectura "sandwich" delgada (`Opción 3C` + `Opción i`, `CGO_ENABLED=0`, cero SQLite, cero llamadas HTTP a LLM desde Go)**:

```text
[1_Intake WI-<hash12>.json (status: ready)]
                   │
                   ▼
   specwriter.exe prepare (Go, 0% LLM)
   • Verifica WorkItem ready + Preflight read-only 0_Brain (reutilizando verify)
   • Emite spec_context.json inmutable (HEAD, SHA-256, risk_zones, trace_id)
     + esqueleto spec.json en 2_Spec_Writer/runtime/<project>/<WI-id>/
                   │
                   ▼
   Agente LLM en Harness Externo (Antigravity / CLI + SKILL.md)
   • Lee código real y edita únicamente spec.json (SSOT canónico)
   • Bucle corto con specwriter.exe validate --stage draft (errores JSON Pointer)
   • En ASDD: revisores Goldfish en contexto limpio + validate --stage final
                   │
                   ▼
   specwriter.exe render & compile (Go, 0% LLM)
   • render: proyecta Markdown determinista sellado con spec_sha256
   • compile: valida superconjunto estricto de validateChunks, compila
     gengis_task/<ID>/ (index.md + chunks/NN_*.md + status.md inicial),
     protege .ring/state.json (exit 6 LIVE_TASK_STATE) e imprime resumen
                   │
                   ▼
   [AUTOR] ejecuta gengis.exe plan (única parada humana en v1; RZ-001 intacto)
```

## Esquema Canónico `spec.json` (Resumen de Alto Nivel)

- `schema_version` (`"1.0"`), `additionalProperties: false` (sin campo opaco `extensions`), `work_item_ref` (`id`, `revision`, `sanitized_payload_sha256`, `trace_id`), `project`, `lane` (`FTD` | `ASDD`), `narrative` (secciones según carril), `work_units[]` (`id`, `objective`, `can_modify`, `scope_expansions`, `forbidden`, `depends_on`, `acceptance_criteria[]` de tipo `command` | `review_gate` con ejecutor, `verify_steps[]` `{id, cwd, env, argv, expect_exit: 0, timeout <= 5m}`, `failure_proof` con `expect_exit` y `expect_output_contains` opcional), `risk_mitigations[]` (enlazando cada `risk_hit.id` a `verify_step_ids[]` y/o `review_gates[]` + `requires_autor_hitl`), `g5_citations[]`, `brain_followups[]`.
- `validate` recalcula determinísticamente el carril efectivo sobre la spec real (`can_modify` + `scope_expansions` + re-consulta `0_Brain` + `pkg/bfcore`), disparando `LANE_UPGRADE_REQUIRED` unidireccional (`FTD → ASDD`).

## Contrato Estable de Códigos de Salida (`0..6`)

| Código | Significado | Consume intento | `reason` en salida JSON / Comportamiento |
| :---: | :--- | :---: | :--- |
| `0` | Éxito (`prepare`, `validate`, `render`, `compile`, `verify`) | No | Operación completada conforme a contrato (`verify` es 100% solo lectura y comparte sus checks con `prepare` y `compile`). |
| `1` | Errores corregibles en `spec.json` (`validate` o `compile`) | Sí en `validate`; **No en `compile`** | En `validate`: esquema/JSON Pointer, `G2`, `G5`, globs/metacaracteres `cmd.exe`, flags `-exec`/`-c`, `risk_mitigations`, evidencia Goldfish en `--stage final` *(higiene de consistencia, no prueba de ejecución)*. En `compile`: fallo de globs/metacaracteres/`validateChunks` que `validate` debió atrapar (`reason: "VALIDATOR_BYPASS_DEFECT"`, sin consumir intento). |
| `2` | Error de uso CLI / argumento mal formado | No | Flags faltantes/inválidos, subcomando desconocido o argumento de ruta sintácticamente mal formado (error del invocador). |
| `3` | Rechazo `G4` (secreto o ruta personal) | Sí en `validate`; No en otros subcomandos | Fail-closed con `0` bytes de payload escritos; en `validate` incrementa el contador numérico en `attempts.json` (sin persistir valor sensible) para impedir bucles infinitos por reincidencia `G4`. |
| `4` | Decisión externa requerida (el agente no puede resolverlo reescribiendo) | No en intentos de `validate` (cuenta en ciclo externo máx. 3; **el 4.º ciclo devuelve `5`, no `4`**) | `STALE_CONTEXT`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`. |
| `5` | Bloqueado por tope de intentos | No (resultado del tope) | `MAX_VALIDATE_ATTEMPTS` (tras 3 intentos de `validate`, incluidos `G4`) o `MAX_EXTERNAL_CYCLES` (en el 4.º ciclo de decisión externa) en `attempts.json` *(documentado como freno contra bucles, no control de seguridad)*. |
| `6` | Entorno/infraestructura no disponible o negativa deliberada de seguridad en destino | No | **Los consumidores del exit `6` deben ramificar por `reason`.** Transitorios/entorno (ruta bien formada que el SO/entorno no puede leer/escribir o dependencia caída): `BRAIN_UNAVAILABLE` (en `prepare`, `validate`, `compile`, `verify`), `BRAIN_JOURNAL_ACTIVE`, `LOCK_BUSY` (reintentable), `IO_ERROR`. Negativa deliberada no reintentable: `LIVE_TASK_STATE` (`.ring/state.json` activo en destino de `compile`). |

## Alcance v1 vs. Diferido a v2

- **Incluido en v1:** `prepare`, `validate [--stage draft|final]`, `render`, `compile`, `verify [--json]` (solo lectura, lógica compartida con `prepare` y `compile`); `spec.json` SSOT (`schema_version: "1.0"`, `additionalProperties: false`, sin `extensions`) + `spec_sha256` canónico + JSON Pointer; `go:embed` para plantillas; superconjunto de `validateChunks` + test de contrato contra `gengis.exe plan` (`t.Skip` con mensaje explícito en local; fallo duro en modo estricto CI `GENGIS_CONTRACT_STRICT=1`, distinguiendo `ErrInvalidChunk` sin resumen/sin `.ring/state.json` frente a `ErrPlanNotApproved` `"plan no aprobado por el Autor"` con exit `1`); `attempts.json` por WorkItem (freno contra bucles, no control de seguridad); evidencia Goldfish en `--stage final` (higiene de consistencia, no prueba de ejecución); `brain_followups[]` post-merge; `specwriter_events.jsonl` con `trace_id` y `.gitignore` `Opción i`.
- **Diferido a v2 (con disparador explícito):**
  - `approve` + firma Ed25519 *(cuando haya ejecución desatendida o más de un operador)*.
  - `publish` + `publish_allowlist` *(cuando exista una segunda necesidad real de exportación; en v1 todo es privado en `runtime/`)*.
  - DSL declarativo externo de plantillas *(cuando ≥3 proyectos diverjan; en v1 `go:embed`)*.
  - Wrapper de `BrainPatch` / `brain.exe propose` *(cuando se resuelva el WorkItem aparte en `0_Brain` por falta de `.brain.lock` y `ScanG4` en `0_Brain/engine/vault.go:244-418`)*.

## Regla de Tests en Umbral `FTD=5` (`pkg/bfcore`)

- Función única en `pkg/bfcore` (`DefaultMaxFTDFiles = 5`, `1` módulo raíz) consumida por `1_Intake` y `2_Spec_Writer` (`validate` nunca mantiene copia propia).
- **Go (`*_test.go`) — única regla de hermano en v1:** un `*_test.go` no suma al límite de 5 solo si existe en el mismo conjunto un archivo no-test en el **mismo directorio**.
- **Otros lenguajes (Python, TS/JS, etc.):** **cuentan completos `1:1`** de forma explícita (sin regla implícita) hasta que exista un proyecto objetivo real y se defina su regla de hermano por lenguaje (p. ej. mismo módulo/paquete en Python).
- **Módulo raíz (`<= 1`):** aplica a todos los tests sin excepción.
- **Integración con `1_Intake`:** cambio propio con su test dedicado en `1_Intake/engine/triage.go`; Plan B si aún no aterriza: contar todo `1:1` en la misma función única de `pkg/bfcore` sin alterar el contrato.
