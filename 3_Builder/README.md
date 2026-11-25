# Equipo 3: `3_Builder` (Construcción TDD Aislada / Orquestador sobre `GengiSWAP`)

## Overview
Responsable de transformar las tareas compiladas por `2_Spec_Writer` en código funcional, probado y auditable sobre ramas dedicadas (`build/<task_id>`) y `git worktree` aislados bajo disciplina **TDD** (*Red → Green → Refactor*), consultando obligatoriamente `0_Brain` en modo solo lectura (*Read-Before-Act*) e integrando el anillo determinista de [`GengiSWAP`](../GengiSWAP/gengis_swap_common.md) (`gengis.exe` como subproceso, manteniendo `RZ-001` 100% intacto) para control de `Scope`, evidencia literal `G1`, commits atómicos y reversión segura.

Al superar la fase Green, empaqueta entregas inmutables (*DeliveryBundle* v1.0) para los 4 subagentes en contexto limpio de [`4_Revisores`](../4_Revisores/README.md) (Seguridad, UX, UI y Código/Arquitectura) y aplica las órdenes consolidadas (*BounceOrder* v1.0: `BOUNCE` | `APPROVE`) de [`5_Cycle_Approver`](../5_Cycle_Approver/README.md) mediante `builder.exe apply-order` (hasta 3 ciclos de remediación antes de activar freno `exit 5`).

> Especificación técnica completa EGM: [`Docs/designs/3_builder.md`](../Docs/designs/3_builder.md)

## Arquitectura y Artefactos
- [`engine/`](./engine/README.md): módulo Go `blackfactory/builder` (`CGO_ENABLED=0`) + binario CLI [`engine/cmd/builder/`](./engine/cmd/builder/README.md) -> `3_Builder/builder.exe` (ignorado en `.gitignore`).
- [`schemas/`](./schemas/delivery_bundle.schema.json): contratos JSON Schema Draft 2020-12 v1.0 (`delivery_bundle.schema.json` y `bounce_order.schema.json`).
- [`skills/`](./skills/README.md): guía progresiva [`builder-execution`](./skills/builder-execution/SKILL.md) y perfiles agnósticos [`profiles.example.json`](./skills/builder-execution/profiles.example.json).
- `runtime/` (ignorado en `.gitignore`, `Opción i`): cerrojos `.session.lock` / `.repo-<sha16>.lock` / `.events.lock`, `builder_events.jsonl`, `<task_id>/session.json`, `bundles/*.{bundle.json,diff}` y `bounces/*.order.json`.

## Flujo Operativo (`builder.exe`)

```text
2_Spec_Writer (gengis_task/<ID>)
        │
        ▼
builder init (rama build/<ID> + git worktree + bind 1 línea index.md + 0_Brain)
        │  + [AUTOR] gengis.exe plan
        ▼
Bucle Interno por Chunk:
  builder context -> TDD Red (builder tdd-red, ExitCode != 0) -> Green/Refactor -> builder package
        │
        ▼
  DeliveryBundle v1.0 ──► 4_Revisores (4 lentes en contexto limpio, aplicabilidad UX/UI determinista)
        ▲                                          │
        │  BounceOrder (BOUNCE 1..3)               ▼
        └──── builder apply-order ◄────── 5_Cycle_Approver
                     │ (APPROVE)
                     ▼
              builder commit (anti-bypass crudo + subproceso gengis.exe check)
                     │
                     ▼
              [AUTOR] gengis.exe decide -> builder cleanup
```

## Subcomandos y Códigos de Salida (`0..6`)
- **Subcomandos:** `init`, `context`, `tdd-red`, `package`, `apply-order`, `commit`, `cleanup`, `verify` (ver matriz completa de banderas en [`engine/cmd/builder/README.md`](./engine/cmd/builder/README.md)).
- **Códigos de salida:** `0` (`OK`), `1` (`Fixable`: alcance, TDD Red/Green, orden inválida, deriva CRLF/BOM/modo post-aprobar), `2` (`Usage`: banderas, tarea o `--cmd-index` inválidos), `3` (`G4_VIOLATION` fail-closed), `4` (`ExternalDecision`: `[AUTOR]`, `STALE_BRAIN_CONTEXT`, `TDD_RED_REQUIRED`), `5` (`MAX_BOUNCE_CYCLES` tras 3 rebotes), `6` (`InfraOrRefusal`: cerrojo, reparse point, `GITIGNORE_DRIFT`, `GENGIS_ENGINE_DRIFT`, estado corrupto).

## Verificación y Compilación
```powershell
Set-Location "C:\Black Factory\3_Builder\engine"
$env:CGO_ENABLED = "0"
$env:GENGIS_CONTRACT_STRICT = "1"
$env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
go vet ./...
go test -count=1 -cover ./...
go test -count=1 -v -run TestLatencyBudgetP95 ./...
go build -o ..\builder.exe .\cmd\builder
& "C:\Black Factory\3_Builder\builder.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\3_Builder\runtime" --json
git -C "C:\Black Factory" check-ignore -q 3_Builder/runtime/
git -C "C:\Black Factory" check-ignore -q 3_Builder/builder.exe
```

## Estado
- **Motor determinista base:** operativo en [`GengiSWAP/`](../GengiSWAP/README.md) (`gengis.exe`, `RZ-001` intacto).
- **Diseño técnico EGM (`Docs/designs/3_builder.md`):** `Approved` (`2026-11-24`), implementación y contrato `builder.exe` / `GengiSWAP` completados en `bf_03_builder_v1`.
