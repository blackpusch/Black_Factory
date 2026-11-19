# Equipo 0: `0_Brain` (Memoria Persistente y Contexto)

Responsable de mantener la memoria persistente, trazable y rápida de **Black Factory** y de cada proyecto gestionado, siguiendo la estrategia híbrida **Opción 3C** y privacidad **Opción i**.

## Diseño (Opción 3C + Opción i)

- **Estándar documental (`rsc-harness`):** *Open Knowledge Format* (Markdown + frontmatter YAML), `AGENTS.md`, `index.md`, `constitution.md`, `log.md`, `adr/`, `wiki/` y skills modulares `SKILL.md` de carga progresiva.
- **Motor determinista en Go (`engine/` → `brain.exe`):**
  - Parser OKF y validación estricta de esquemas/enlaces internos.
  - Guardia determinista **G4** (escaneo de secretos, tokens, cadenas de conexión o rutas personales).
  - Índice local **SQLite FTS5** (BM25, `modernc.org/sqlite` pure Go) + cruce por `covers_paths` para consultas en milisegundos.
  - Detección de *drift* (obsolescencia) comparando `verified_head` contra `git diff`.
  - Flujo gobernado de propuestas (`propose` / `apply`, con aprobación `[AUTOR]` para `constitution.md` y `risk_zones.md`).
  - Preparado para exponer servidor **MCP** local en fase posterior.
- **Privacidad (Opción i):**
  - **Versionado en Git:** `README.md`, `schemas/`, `skills/` y `engine/`.
  - **Untracked (`.gitignore`):** `vault/` (procesos privados, constitución real, brains de proyectos e índice `.index/brain.db`) y `brain.exe`.
