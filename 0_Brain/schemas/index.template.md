---
id: brain-index
title: "Índice Maestro de Conocimiento (0_Brain)"
type: index
project: black-factory
status: active
tags:
  - index
  - navigation
  - okf
covers_paths: []
verified_head: ""
updated_at: "YYYY-MM-DD"
---

# Índice Maestro de Conocimiento (`0_Brain`)

Catálogo central de navegación de la bóveda OKF. Toda consulta debe iniciar por el enrutador y este índice antes de buscar detalles en documentos específicos.

## Documentos Núcleo

| Documento | Rol en la bóveda | Ruta |
| :--- | :--- | :--- |
| Enrutador operativo | Protocolo *Read-Before-Act* y enrutamiento por equipo | [AGENTS.md](AGENTS.md) |
| Constitución | Principios innegociables, guardas `G1–G6` y gobernanza | [constitution.md](constitution.md) |
| Zonas de riesgo | Catálogo de áreas críticas con compuerta HITL `[AUTOR]` | [wiki/risk_zones.md](wiki/risk_zones.md) |
| Bitácora operativa | Registro *append-only* de aplicaciones y cambios aprobados | [log.md](log.md) |

## Proyectos Registrados

Registrar cada proyecto gestionado bajo un archivo dedicado en `projects/<proyecto>.md` usando frontmatter OKF completo y `covers_paths` relativos al repositorio del proyecto.

| Proyecto | Ruta lógica | Estado | Resumen | Referencia en vault |
| :--- | :--- | :--- | :--- | :--- |
| `black-factory` | `C:/Black Factory` | `active` | Orquestación de factoría de software (`0_Brain`..`6_Filtro_de_Riesgo` y `GengiSWAP`) | `projects/black-factory.md` |

## Decisiones de Arquitectura (ADR)

Los registros de decisiones de arquitectura se almacenan en `adr/` siguiendo la convención `adr/ADR-NNNN-slug.md` a partir de la plantilla `adr.template.md`.

- Convención de identificadores: `adr-NNNN-slug` (único en toda la bóveda).
- Cada ADR debe declarar `covers_paths` normalizados con `/` y `verified_head` con el hash del commit validado.
- Ejemplo de ubicación futura: `adr/ADR-0001-arquitectura-okf-fts5.md`.

## Wiki Técnica y Dominios

La documentación viva de módulos, dominios y contratos reside en `wiki/`.

- Mapa transversal de zonas de riesgo: [wiki/risk_zones.md](wiki/risk_zones.md).
- Páginas de dominio y submódulos: crear en `wiki/<dominio>.md` a partir de `wiki.template.md` (ejemplo: `wiki/motor-indexacion.md`).

## Propuestas Pendientes (`_proposals/`)

Las actualizaciones propuestas por agentes se depositan como documentos `BrainPatch` dentro de `_proposals/` sin mutar los documentos activos hasta superar validación OKF/G4 y `brain.exe apply`.

- Las modificaciones a [constitution.md](constitution.md) o [wiki/risk_zones.md](wiki/risk_zones.md) requieren aprobación explícita del `[AUTOR]` (`--approve-autor`).

## Mantenimiento del Índice

Para reconstruir de forma atómica el índice local SQLite FTS5 (`.index/brain.db`) y comprobar integridad OKF, enlaces internos, reglas `G4` y *drift* Git:

```text
brain.exe index --vault <RUTA_VAULT>
brain.exe verify --vault <RUTA_VAULT> [--repo <PROYECTO>=<RUTA_REPO>]
```
