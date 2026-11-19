# Skills Modulares de `0_Brain` (Carga Progresiva RSC)

Catálogo oficial de *skills* de memoria persistente para los equipos de **Black Factory**, inspirado en el patrón modular `SKILL.md` de `ericrisco/rsc-harness` y acoplado al motor determinista `0_Brain/brain.exe`.

## Índice de Skills

| Skill | Propósito principal | Archivo |
| :--- | :--- | :--- |
| `brain-query` | Carga progresiva *Read-Before-Act* y consulta rápida vía SQLite FTS5 (`query` / `context`) | [`brain-query/SKILL.md`](brain-query/SKILL.md) |
| `brain-update` | Ciclo gobernado `init-vault`, `propose` (`BrainPatch`) y `apply` con aprobación `[AUTOR]` | [`brain-update/SKILL.md`](brain-update/SKILL.md) |
| `brain-verify` | Validación determinista OKF, enlaces internos, guardia `G4` y detección de *drift* Git | [`brain-verify/SKILL.md`](brain-verify/SKILL.md) |

## Ciclo Operativo de Conocimiento

```text
┌───────────────────┐     ┌────────────────────┐     ┌────────────────────┐
│  1. brain-query   │ ──► │  Trabajo del equipo│ ──► │  2. brain-update   │
│  Read-Before-Act  │     │  (Spec / Build /   │     │  propose ─► apply  │
│  query / context  │     │   Review / Filtro) │     │  (_proposals/ +    │
└───────────────────┘     └────────────────────┘     │   [AUTOR] + log)   │
         ▲                                           └─────────┬──────────┘
         │                                                     │
         │                ┌────────────────────┐               │
         └─────────────── │  3. brain-verify   │ ◄─────────────┘
                          │  OKF + G4 + Drift  │
                          └────────────────────┘
```

## Matriz de Activación por Equipo

| Equipo | Carga obligatoria al iniciar | Al cerrar o auditar |
| :--- | :--- | :--- |
| `1_Intake` | `brain-query` | `brain-query` (registro de contexto en `WorkItem`) |
| `2_Spec_Writer` | `brain-query`, `brain-verify` | `brain-update` (si documenta ADR o dominio nuevo) |
| `3_Builder` | `brain-query` | `brain-verify` (antes de entregar cambios en código cubierto) |
| `4_Revisores` | `brain-query`, `brain-verify` | `brain-verify` (evidencia `G4`, `G5` y *drift*) |
| `5_Cycle_Approver` | `brain-query`, `brain-verify` | `brain-update` (propuestas tras cierre de ciclo) |
| `6_Filtro_de_Riesgo` | `brain-query`, `brain-verify` | Validación de impacto contra `wiki/risk_zones.md` |

## Reglas Globales (`G1–G6`) y Privacidad (`Opción i`)

- Toda operación de lectura, propuesta o verificación acata de forma obligatoria las reglas **G1–G6**:
  - **G1:** conservar comando ejecutado, código de salida y salida literal en reportes.
  - **G2:** respetar encabezados de ubicación en artefactos generados en tareas `gengis`.
  - **G3:** no mutar bases de datos reales ni datos productivos; pruebas siempre en entornos temporales efímeros.
  - **G4:** cero secretos, credenciales, tokens HTTP, API keys, claves privadas PEM/SSH, correos sensibles o rutas absolutas de perfiles personales de usuario.
  - **G5:** citar `archivo:línea` reales y separar `HECHOS` observados de `HIPÓTESIS`.
  - **G6:** detenerse y reportar al `[AUTOR]` ante cualquier ambigüedad, enlace roto o violación de alcance.
- **Privacidad `Opción i`:** las skills y los esquemas se versionan en Git (`0_Brain/skills/`, `0_Brain/schemas/`), mientras que la bóveda real `0_Brain/vault/`, el binario `0_Brain/brain.exe` y el índice `.index/brain.db` (`*.db`) permanecen excluidos mediante `.gitignore`.
