# Equipo 3: `3_Builder` (Construcción TDD Aislada / Orquestador sobre `GengiSWAP`)

## Overview
Responsable de transformar las tareas compiladas por `2_Spec_Writer` en código funcional, probado y auditable sobre ramas dedicadas (`build/<task_id>`) y `git worktree` aislados bajo disciplina **TDD** (*Red → Green → Refactor*), consultando obligatoriamente `0_Brain` en modo solo lectura (*Read-Before-Act*) e integrando el anillo determinista de [`GengiSWAP`](../GengiSWAP/gengis_swap_common.md) (`gengis.exe` como subproceso, manteniendo `RZ-001` 100% intacto) para control de `Scope`, evidencia literal `G1`, commits atómicos y reversión segura.

Al superar la fase Green, empaqueta entregas inmutables (*DeliveryBundle* v1.0) para los 4 subagentes en contexto limpio de [`4_Revisores`](../4_Revisores/README.md) (Seguridad, UX, UI y Código/Arquitectura) y aplica las órdenes consolidadas (*BounceOrder* v1.0: `BOUNCE` | `APPROVE`) de [`5_Cycle_Approver`](../5_Cycle_Approver/README.md) mediante `builder.exe apply-order` (hasta 3 ciclos de remediación antes de activar freno `exit 5`).

> Especificación técnica completa EGM: [`Docs/designs/3_builder.md`](../Docs/designs/3_builder.md)

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

## Estado
- **Motor determinista base:** operativo en [`GengiSWAP/`](../GengiSWAP/README.md) (`gengis.exe`, `RZ-001` intacto).
- **Diseño técnico EGM (`Docs/designs/3_builder.md`):** `Approved` (`2026-11-24`), compilado en 4 chunks disjuntos (`bf_03_builder_v1`).
