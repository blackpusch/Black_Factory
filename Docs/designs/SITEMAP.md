# Black Factory — Design Sitemap (EGM)

Índice maestro de documentos de diseño técnico producidos y auditados bajo el **Elephant-Goldfish Model (EGM)** en `Black Factory`.

## Especificaciones de Equipos (`Docs/designs/`)

| Equipo | Documento | Estado | Resumen |
| :--- | :--- | :--- | :--- |
| `0_Brain` | [../BLACK_FACTORY_BRAIN.md](../BLACK_FACTORY_BRAIN.md) | Implementado (`main` `8316011`, endurecimiento en `fix/0-brain-hardening`) | Memoria persistente OKF/RSC, índice SQLite FTS5, validación G4, drift Git y gobernanza Ed25519 |
| `1_Intake` | [1_intake.md](1_intake.md) | `Approved` (implementado en `feat/1-intake`, `ee16ab4`) | Puerta perimetral determinista, `pkg/bfcore`, alineación Fase 0 en `0_Brain`, G4 de dos niveles y triaje `ASDD`/`FTD` |
| `2_Spec_Writer` | [2_spec_writer.md](2_spec_writer.md) | `Approved` (implementado en `feat/2-spec-writer`, `00c32c7`; `bf_02_spec_writer_v1` `DONE`) | Compuerta determinista de especificaciones (`spec.json` SSOT), `pkg/bfcore` Fase 0 (Opción `5B` + `BrainClient`), compilador *create-only* hacia `GengiSWAP` y test de contrato `gengis.exe plan` |
| `3_Builder` | [3_builder.md](3_builder.md) | `Approved` (auditado por 5 lentes Goldfish `2026-11-24`; compilado en `bf_03_builder_v1`; motor base `GengiSWAP/` en [../PROYECTO_TUNGSTENO.md](../PROYECTO_TUNGSTENO.md)) | Construcción TDD aislada (`build/<ID>` + `git worktree` + `0_Brain` + `builder.exe` 8 subcomandos), contratos `DeliveryBundle`/`BounceOrder` v1.0 hacia `4_Revisores`/`5_Cycle_Approver` y sellado sobre `gengis.exe check` (`RZ-001` intacto) |
| `4_Revisores` | *(pendiente; contrato base en [3_builder.md](3_builder.md))* | Planificado | 4 subagentes en contexto limpio con diversidad de modelos (Seguridad, UX, UI, Código/Arquitectura vs `0_Brain`) |
| `5_Cycle_Approver` | *(pendiente; contrato base en [3_builder.md](3_builder.md))* | Planificado | Consolidación de los 4 dictámenes y ciclo de corrección controlado hacia `3_Builder` |
| `6_Filtro_de_Riesgo` | *(pendiente)* | Planificado | Compuerta híbrida HITL (`[AUTOR]`) sobre diffs reales y `wiki/risk_zones.md` (vía rápida vs. PR manual) |

## Deuda Técnica y Seguimientos

- Registro fechado y verificable (`G5`): [../TECH_DEBT.md](../TECH_DEBT.md) (`TD-001`..`TD-005`).
- Estado actual (`2026-11-24`, rama `feat/3-builder`): especificación técnica `Approved` en [3_builder.md](3_builder.md), compilada a `bf_03_builder_v1` (4 chunks disjuntos).

## Flujo EGM → GengiSWAP

Todo diseño en estado `Approved` se compila en una tarea formal de `GengiSWAP` mediante la skill de workspace [egm-to-gengis](../../.agents/skills/egm-to-gengis/SKILL.md).
