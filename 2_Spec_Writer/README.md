# Equipo 2: `2_Spec_Writer` (Compuerta Determinista de Especificaciones y Compilador hacia `GengiSWAP`)

## Overview
Responsable de conectar los `WorkItem` listos de `1_Intake` (`schema_version: "1.0"`, inmutables) con el anillo determinista de ejecución `GengiSWAP` (`gengis.exe`), consultando `0_Brain` en modo estrictamente de solo lectura (*Read-Before-Act*). Compilado como binario autónomo pure-Go (`2_Spec_Writer/specwriter.exe`, `CGO_ENABLED=0`, cero SQLite, cero llamadas HTTP a LLM, compartiendo `blackfactory/pkg/bfcore` con `golang.org/x/sys v0.49.0`), sigue la arquitectura híbrida **Opción 3C** y privacidad **Opción i**.

> Especificación técnica aprobada: [`Docs/designs/2_spec_writer.md`](../Docs/designs/2_spec_writer.md)

## Subdirectories & System Boundaries
- `engine`: [./engine/README.md](./engine/README.md) — Motor determinista pure-Go (`blackfactory/specwriter`, plantillas `GengiSWAP` embebidas con `go:embed`) y CLI `specwriter.exe` ([`./engine/cmd/specwriter/README.md`](./engine/cmd/specwriter/README.md)).
- `schemas`: [./schemas/spec.schema.json](./schemas/spec.schema.json) — Esquema canónico JSON Schema Draft 2020-12 (`schema_version: "1.0"`, `additionalProperties: false`, fuente única de verdad).
- `skills`: [./skills/README.md](./skills/README.md) — Guía progresiva del bucle operativo para agentes externos (`spec-authoring`).

## Arquitectura en Tres Fases

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ FASE 1 (Go): specwriter.exe prepare --workitem <WI.json>                   │
│   G4 (11 reglas) + Preflight 0_Brain read-only + spec_context.json +       │
│   attempts.json (0) + esqueleto spec.json                                  │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ FASE 2 (Agente Externo + Go): Autoría sobre spec.json + validate           │
│   Precedencia 2 -> 5 -> 3 -> 6 -> 4 -> 1 (presupuestos draft=3, final=3,   │
│   external_cycles=3; allowlist cerrado; paridad bridge.go/ring.go;         │
│   goldfish_review.json en ASDD --stage final)                              │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ FASE 3 (Go): render (stdout-only) + compile (create-only NTFS rename)      │
│   Anti-bypass (last_passed_sha256/stage/goldfish) + frontera G4 dos zonas  │
│   (<TARGET_REPO>) + .compile.lock -> GengiSWAP_data\gengis_task\<ID>\      │
│   -> única parada [AUTOR] en gengis.exe plan                               │
└────────────────────────────────────────────────────────────────────────────┘
```

## Mapa Comparado de Códigos de Salida (`1_Intake 0..5` vs. `2_Spec_Writer 0..6`)

| Código | `1_Intake` (`intake.exe`) | `2_Spec_Writer` (`specwriter.exe`) | `retryable` |
| :---: | :--- | :--- | :---: |
| `0` | `ready` / éxito | Éxito (`OK`) | `false` |
| `1` | Error técnico o CLI | Corregible en `spec.json` / `goldfish_review.json` (`VALIDATION_FAILED`, `INVALID_JSON`, `SPEC_TOO_LARGE`, `VALIDATION_REQUIRED`) o defecto de invariante (`VALIDATOR_BYPASS_DEFECT`) | `true` (`false` en `VALIDATOR_BYPASS_DEFECT`) |
| `2` | Cuarentena `G6` | Uso CLI inválido, `--spec` inexistente, `WorkItem` inválido/`needs_clarification` o `SPEC_ALREADY_EXISTS` | `false` |
| `3` | Rechazo Tier 1 `G4` | Rechazo fail-closed `G4` en las 11 reglas (`G4_VIOLATION`, `G4_VIOLATION_ON_DISK`) sin auto-redacción Tier 2 | `false` |
| `4` | `0_Brain` / lock no disponible | Decisión externa requerida (`STALE_CONTEXT`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`) | `false` |
| `5` | Conflicto `dedupe_key` | Tope de freno operativo (`MAX_VALIDATE_ATTEMPTS`, `MAX_EXTERNAL_CYCLES`; desbloqueo con `prepare --reset-attempts --reason`) | `false` |
| `6` | *(no usado)* | Infraestructura transitoria (`BRAIN_UNAVAILABLE`, `BRAIN_JOURNAL_ACTIVE`, `LOCK_BUSY`, `IO_ERROR` → `true`) o negativa deliberada / estado corrupto / deriva (`LIVE_TASK_STATE`, `TASK_DIR_EXISTS`, `ATTEMPTS_STATE_*`, `ARTIFACT_STATE_CORRUPT`, `REPARSE_POINT_DETECTED`, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT` → `false`) | `true` / `false` |

## Matriz de Subcomandos CLI

| Subcomando | Flags Obligatorios | Flags Opcionales | Efectos en Disco |
| :--- | :--- | :--- | :--- |
| `prepare` | `--workitem <path>` | `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin`, `--refresh-context`, `--reset-attempts`, `--reason`, `--json` | Escribe `spec_context.json`, `attempts.json`, `spec.json` y evento JSONL bajo `.wi.lock`/`.events.lock`. |
| `validate` | `--spec <path>`, `--stage <draft\|final>` | `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json` | Actualiza `attempts.json` y `specwriter_events.jsonl`; nunca ejecuta comandos de la spec. |
| `render` | `--spec <path>` | `--json` | `0` escrituras (proyección Markdown a `stdout`). |
| `compile` | `--spec <path>`, `--task-root <dir>`, `--task-id <id>` | `--repo-root`, `--factory-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json` | Promoción atómica *create-only* en `<task-root>/<task-id>` bajo `.wi.lock` → `.compile.lock` → `.events.lock`. |
| `verify` | *(ninguno)* | `--factory-root`, `--repo-root`, `--vault`, `--data-dir`, `--brain-bin`, `--json` | `0` escrituras, `0` cerrojos exclusivos (100% solo lectura). |

## Invariants
- **Cero CGO, Cero SQLite, Cero LLM (`SG1`, `NG1`):** `2_Spec_Writer/engine` depende únicamente de `blackfactory/pkg/bfcore` y `golang.org/x/sys v0.49.0`.
- **Fuente Única de Verdad (`SG2`):** `spec.json` (`additionalProperties: false`, rutas concretas de archivo en `can_modify` para v1) gobierna validación, `render` y `compile`.
- **G4 Fail-Closed en 11 Reglas y Frontera de Dos Zonas (`G4`):** cero rutas absolutas de host en `runtime/`; `<TARGET_REPO>` se sustituye únicamente tras `ScanG4All` en el `index.md` externo compilado.
- **Privacidad `Opción i` y Deriva `RZ-001` (`SG5`):** `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` excluidos en `.gitignore`; `verify` comprueba individualmente ambas rutas y los hashes normalizados de `GengiSWAP/engine/{bridge,ring}.go`.
- **Freno Operativo (`attempts.json`):** diseñado explícitamente como freno determinista contra bucles de reintento, no como control de seguridad frente a un operador con acceso local al sistema de archivos.

## Side Effects
- Mantiene estado operativo privado en `2_Spec_Writer/runtime/<project>/<WI-id>/` y bitácora `2_Spec_Writer/runtime/specwriter_events.jsonl` (sanitizada con `RedactAllG4` + verificada con `ScanG4All`).
- Compila carpetas de tarea `GengiSWAP` en `<task-root>/<task-id>` bajo política estricta *create-only* (`LIVE_TASK_STATE` / `TASK_DIR_EXISTS`).
- Invoca `0_Brain/brain.exe` (`context --json`) en modo estrictamente de solo lectura y `git check-ignore` en `verify`.

## Deuda Técnica Conocida

- **`TD-001` (`2026-11-24`, diferida tras `bf_02_spec_writer_v1`):** endurecimiento opcional de `WorkItem.TraceID` (`reTraceID` + `checkIndexProseSafety`), `TargetBranch`, paridad `\r`/`~~~`/`#` en `verifyRenderedGengisParity` y rechazo de `verify_step_id` no vacío en `review_gate`. Ver detalle con referencias `G5` en [`Docs/TECH_DEBT.md`](../Docs/TECH_DEBT.md#td-001--hardening-de-workitemtraceid-y-paridad-fina-en-2_spec_writer).

## Verification
```powershell
Set-Location "C:\Black Factory\2_Spec_Writer\engine"
$env:CGO_ENABLED = "0"
$env:GENGIS_CONTRACT_STRICT = "1"
$env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
go vet ./...
go test -count=1 -cover ./...
go build -o ..\specwriter.exe .\cmd\specwriter
& "C:\Black Factory\2_Spec_Writer\specwriter.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\2_Spec_Writer\runtime" --json
git -C "C:\Black Factory" check-ignore -q 2_Spec_Writer/runtime/
git -C "C:\Black Factory" check-ignore -q 2_Spec_Writer/specwriter.exe
```
