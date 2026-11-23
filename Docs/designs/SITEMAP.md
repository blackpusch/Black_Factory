# Black Factory — Design Sitemap (EGM)

Índice maestro de documentos de diseño técnico producidos y auditados bajo el **Elephant-Goldfish Model (EGM)** en `Black Factory`.

## Especificaciones de Equipos (`Docs/designs/`)

| Equipo | Documento | Estado | Resumen |
| :--- | :--- | :--- | :--- |
| `0_Brain` | [../BLACK_FACTORY_BRAIN.md](../BLACK_FACTORY_BRAIN.md) | Implementado (`main` `8316011`, endurecimiento en `fix/0-brain-hardening`) | Memoria persistente OKF/RSC, índice SQLite FTS5, validación G4, drift Git y gobernanza Ed25519 |
| `1_Intake` | [1_intake.md](1_intake.md) | `Approved` (implementado en `feat/1-intake`, `ee16ab4`) | Puerta perimetral determinista, `pkg/bfcore`, alineación Fase 0 en `0_Brain`, G4 de dos niveles y triaje `ASDD`/`FTD` |
| `2_Spec_Writer` | [2_spec_writer.md](2_spec_writer.md) | `Approved` (2026-11-23, `blackpusch`; 4 rondas Goldfish, `unresolved_blockers: 0`) | Compuerta determinista de especificaciones (`spec.json` SSOT), `pkg/bfcore` Fase 0 (Opción `5B` + `BrainClient`), compilador *create-only* hacia `GengiSWAP` y test de contrato `gengis.exe plan` |
| `3_Builder` | [../PROYECTO_TUNGSTENO.md](../PROYECTO_TUNGSTENO.md) | Motor base operativo (`GengiSWAP/`) | Construcción TDD aislada sobre el anillo determinista `gengis.exe` |
| `4_Revisores` | *(pendiente)* | Planificado | Auditoría multi-lente en contexto limpio (Seguridad, Calidad/G5, UX, UI) |
| `5_Cycle_Approver` | *(pendiente)* | Planificado | Consolidación de hallazgos y rebote controlado hacia `3_Builder` |
| `6_Filtro_de_Riesgo` | *(pendiente)* | Planificado | Compuerta híbrida HITL (`[AUTOR]`) sobre diffs reales y `wiki/risk_zones.md` |

## Flujo EGM → GengiSWAP

Todo diseño en estado `Approved` se compila en una tarea formal de `GengiSWAP` mediante la skill de workspace [egm-to-gengis](../../.agents/skills/egm-to-gengis/SKILL.md).
