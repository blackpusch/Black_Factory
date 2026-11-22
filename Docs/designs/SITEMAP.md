# Black Factory — Design Sitemap (EGM)

Índice maestro de documentos de diseño técnico producidos y auditados bajo el **Elephant-Goldfish Model (EGM)** en `Black Factory`.

## Especificaciones de Equipos (`Docs/designs/`)

| Equipo | Documento | Estado | Resumen |
| :--- | :--- | :--- | :--- |
| `0_Brain` | [../BLACK_FACTORY_BRAIN.md](../BLACK_FACTORY_BRAIN.md) | Implementado (`main` `8316011`, endurecimiento en `fix/0-brain-hardening`) | Memoria persistente OKF/RSC, índice SQLite FTS5, validación G4, drift Git y gobernanza Ed25519 |
| `1_Intake` | [1_intake.md](1_intake.md) | `Approved` | Puerta perimetral determinista, `pkg/bfcore`, alineación Fase 0 en `0_Brain`, G4 de dos niveles y triaje `ASDD`/`FTD` |
| `2_Spec_Writer` | *(pendiente)* | Planificado | Generación de especificaciones consultando `0_Brain` y consumiendo `WorkItem` de `1_Intake` |
| `3_Builder` | [../PROYECTO_TUNGSTENO.md](../PROYECTO_TUNGSTENO.md) | Motor base operativo (`GengiSWAP/`) | Construcción TDD aislada sobre el anillo determinista `gengis.exe` |
| `4_Revisores` | *(pendiente)* | Planificado | Auditoría multi-lente en contexto limpio (Seguridad, Calidad/G5, UX, UI) |
| `5_Cycle_Approver` | *(pendiente)* | Planificado | Consolidación de hallazgos y rebote controlado hacia `3_Builder` |
| `6_Filtro_de_Riesgo` | *(pendiente)* | Planificado | Compuerta híbrida HITL (`[AUTOR]`) sobre diffs reales y `wiki/risk_zones.md` |

## Flujo EGM → GengiSWAP

Todo diseño en estado `Approved` se compila en una tarea formal de `GengiSWAP` mediante la skill de workspace [egm-to-gengis](../../.agents/skills/egm-to-gengis/SKILL.md).
