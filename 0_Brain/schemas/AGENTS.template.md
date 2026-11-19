---
id: brain-agents
title: "Enrutador Operativo de 0_Brain (AGENTS)"
type: agents
project: black-factory
status: active
tags:
  - agents
  - routing
  - okf
  - read-before-act
covers_paths: []
verified_head: ""
updated_at: "YYYY-MM-DD"
---

# Enrutador Operativo de `0_Brain` (Protocolo *Read-Before-Act*)

Este documento es el punto de entrada obligatorio para cualquier equipo o agente de **Black Factory** antes de consultar, planificar, construir, revisar o modificar memoria persistente en la bóveda local `0_Brain/vault/`.

## Protocolo Obligatorio *Read-Before-Act* (6 Pasos)

Ningún agente debe proponer especificaciones, editar código fuente o emitir dictámenes sin completar esta secuencia progresiva:

1. **Leer este enrutador (`AGENTS.md`)** para ubicar la topología de la bóveda, los skills aplicables y las guardas vigentes.
2. **Consultar [index.md](index.md)** para localizar proyectos registrados, ADRs relevantes, páginas de wiki y propuestas abiertas.
3. **Leer [constitution.md](constitution.md)** para cargar las invariantes de ingeniería, carriles metodológicos (`ASDD`/`SDD`, `FTD`, `TDD`) y reglas de gobernanza.
4. **Revisar [wiki/risk_zones.md](wiki/risk_zones.md)** para identificar zonas críticas que exigen verificación reforzada o compuerta humana `[AUTOR]`.
5. **Ejecutar consultas focalizadas con `brain.exe`** (`brain.exe query` y `brain.exe context`) usando la skill `brain-query` antes de abrir archivos masivos del repositorio.
6. **Proponer cambios solo por vía gobernada** mediante `brain.exe propose` y `brain.exe apply` (skill `brain-update`), dejando registro *append-only* en [log.md](log.md).

## Flujo de Arquitectura Híbrida (Opción 3C + Opción i)

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                    CARGA PROGRESIVA (Read-Before-Act)                   │
│                                                                         │
│  [1] AGENTS.md ─► [2] index.md ─► [3] constitution.md ─► [4] risk_zones │
└───────────────────────────────────┬─────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                 MOTOR DETERMINISTA GO (0_Brain/brain.exe)               │
│                                                                         │
│  • brain.exe query / context  ──► Índice local SQLite FTS5 (.index/)    │
│  • brain.exe verify           ──► Esquema OKF + Enlaces + G4 + Drift    │
│  • brain.exe propose / apply  ──► _proposals/ + [AUTOR] + log.md        │
└───────────────────────────────────┬─────────────────────────────────────┘
                                    │
          ┌─────────────────────────┴─────────────────────────┐
          ▼                                                   ▼
┌───────────────────────────┐                       ┌───────────────────────┐
│  VERSIONADO EN GIT        │                       │  UNTRACKED (Opción i) │
│  • 0_Brain/schemas/       │                       │  • 0_Brain/vault/     │
│  • 0_Brain/skills/        │                       │  • 0_Brain/brain.exe  │
│  • 0_Brain/engine/        │                       │  • *.db / .index/     │
└───────────────────────────┘                       └───────────────────────┘
```

## Tabla de Enrutamiento por Equipo de Black Factory

| Equipo | Responsabilidad | Skills a cargar | Documentos mínimos obligatorios |
| :--- | :--- | :--- | :--- |
| `1_Intake` | Triaje de incidencias (`WorkItem`) y asignación de carril | `brain-query` | `AGENTS.md`, [index.md](index.md), [constitution.md](constitution.md) |
| `2_Spec_Writer` | Redacción de especificaciones SDD/ASDD o FTD | `brain-query`, `brain-update` | [index.md](index.md), [constitution.md](constitution.md), [wiki/risk_zones.md](wiki/risk_zones.md) |
| `3_Builder` | Construcción TDD en worktrees aislados bajo `gengis` | `brain-query`, `brain-verify` | [constitution.md](constitution.md), [wiki/risk_zones.md](wiki/risk_zones.md), resultados de `brain.exe context` |
| `4_Revisores` | Auditoría en contexto limpio (Seguridad, Calidad, UX, UI) | `brain-query`, `brain-verify` | [constitution.md](constitution.md), [wiki/risk_zones.md](wiki/risk_zones.md), salida literal de `brain.exe verify` |
| `5_Cycle_Approver` | Consolidación de hallazgos y rebote controlado | `brain-query`, `brain-verify` | [constitution.md](constitution.md), [log.md](log.md) |
| `6_Filtro_de_Riesgo` | Compuerta híbrida HITL y clasificación de impacto | `brain-query`, `brain-verify` | [wiki/risk_zones.md](wiki/risk_zones.md), [constitution.md](constitution.md) |

## Guardas Obligatorias (`G1–G6`) y Privacidad (`Opción i`)

- **G1 (Evidencia literal):** toda validación o consulta usada como soporte debe conservar el comando ejecutado, el código de salida y las líneas relevantes de la salida.
- **G2 (Trazabilidad de artefactos):** todo reporte externo derivado en tareas `gengis` respeta los encabezados de ubicación vigentes.
- **G3 (No mutar datos reales):** ninguna prueba o indexación muta bases de datos productivas; las pruebas corren sobre directorios temporales efímeros.
- **G4 (Cero secretos o datos sensibles):** queda prohibido almacenar en la bóveda o en plantillas claves privadas PEM/SSH, tokens de autorización HTTP, API keys, contraseñas en cadenas de conexión, correos personales/corporativos o rutas absolutas de perfiles personales de usuario del sistema operativo.
- **G5 (Rigor de evidencia):** citar `archivo:línea` reales y separar explícitamente `HECHOS` observados de `HIPÓTESIS`.
- **G6 (Detención ante ambigüedad):** ante contradicción, enlace roto o riesgo no cubierto, detener el flujo y escalar al `[AUTOR]`.
- **Privacidad Opción i:** `0_Brain/vault/`, `.index/brain.db`, `*.db` y `0_Brain/brain.exe` permanecen excluidos del control de versiones en `.gitignore`.
