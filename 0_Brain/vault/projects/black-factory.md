---
id: project-black-factory
title: "Proyecto: Black Factory (Orquestación de Factoría de Software)"
type: project
project: black-factory
status: active
tags:
  - project
  - black-factory
  - governance
covers_paths:
  - 0_Brain/
  - 1_Intake/
  - GengiSWAP/
  - pkg/bfcore/
verified_head: ""
updated_at: "2026-11-22"
---

# Proyecto: Black Factory (`black-factory`)

## Propósito y Alcance

Repositorio monorepo principal de **Black Factory**, organizado en siete equipos especializados (`0_Brain` a `6_Filtro_de_Riesgo`), el paquete compartido `pkg/bfcore` y el anillo determinista de ejecución `GengiSWAP` (`gengis.exe`).

## Componentes Registrados

| Equipo / Módulo | Ruta Relativa | Rol Principal |
| :--- | :--- | :--- |
| `0_Brain` | `0_Brain/` | Memoria persistente OKF, índice SQLite FTS5, guardia `G4` y gobernanza Ed25519 |
| `1_Intake` | `1_Intake/` | Normalización determinista de solicitudes, frontera `G4` en dos niveles y triaje `ASDD`/`FTD` |
| `pkg/bfcore` | `pkg/bfcore/` | Primitivas compartidas pure-Go (`CGO_ENABLED=0`, sin SQLite) para `G4`, globs, locks y reparse points |
| `GengiSWAP` | `GengiSWAP/` | Anillo de ejecución determinista por chunks con verificación `G1–G6` y compuerta `[AUTOR]` |

## Referencias Núcleo

- Enrutador operativo: [../AGENTS.md](../AGENTS.md)
- Índice maestro: [../index.md](../index.md)
- Constitución: [../constitution.md](../constitution.md)
- Zonas de riesgo: [../wiki/risk_zones.md](../wiki/risk_zones.md)
