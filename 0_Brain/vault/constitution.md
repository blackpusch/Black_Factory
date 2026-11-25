---
id: brain-constitution
title: "Constitución de Ingeniería y Gobernanza (0_Brain)"
type: constitution
project: black-factory
status: active
tags:
  - constitution
  - governance
  - invariants
covers_paths: []
verified_head: ""
updated_at: "2026-11-19"
---

# Constitución de Ingeniería y Gobernanza (`0_Brain`)

Este documento define las invariantes arquitectónicas, de seguridad y de verificación que rigen a todos los equipos de **Black Factory** y a cualquier proyecto gestionado dentro de la bóveda.

## Principios Innegociables

1. **Determinismo del motor sobre buena voluntad del modelo:** las reglas de alcance, compilación, pruebas, validación OKF y escaneo `G4` se hacen cumplir mediante herramientas deterministas (`gengis.exe` y `brain.exe`), nunca por afirmaciones sin respaldo.
2. **Evidencia literal (`G1`):** ningún cambio ni verificación se considera válido sin registrar el comando ejecutado, el código de salida y la salida literal pertinente.
3. **Disciplina de pruebas (`TDD` y carriles):**
   - Toda funcionalidad nueva o corrección de defecto sigue el ciclo *Red → Green → Refactor* dentro de su carril asignado (`ASDD`/`SDD`, `FTD` o `TDD`).
   - Las pruebas operan exclusivamente sobre directorios y bases de datos temporales efímeras.
4. **Separación de responsabilidades entre equipos:** `0_Brain` preserva memoria y gobernanza; `1_Intake` clasifica; `2_Spec_Writer` especifica tras consultar el contexto; `3_Builder` ejecuta en alcance acotado; `4_Revisores` audita en contexto limpio; `5_Cycle_Approver` consolida rebotes; `6_Filtro_de_Riesgo` aplica compuerta híbrida HITL.

## Seguridad, Aislamiento y Privacidad (`G3`, `G4` y `Opción i`)

- **No mutar datos reales (`G3`):** está prohibido alterar bases de datos reales o ejecutar operaciones destructivas sobre entornos productivos fuera de autorización explícita.
- **Cero secretos y cero rutas personales (`G4`):** ningún documento, plantilla, propuesta o código versionado puede contener credenciales, contraseñas en cadenas de conexión, claves privadas PEM/SSH, tokens de autorización HTTP, API keys, correos electrónicos sensibles ni rutas absolutas de perfiles personales de usuario del sistema operativo.
- **Privacidad estructural (`Opción i`):** la bóveda operativa `0_Brain/vault/`, el binario compilado `0_Brain/brain.exe` y los archivos de índice local (`*.db`, `.index/`) deben permanecer ignorados en `.gitignore` y nunca ser versionados en Git.

## Rigor de Evidencia y Manejo de Ambigüedad (`G5` y `G6`)

- **Rigor `G5`:** toda afirmación sobre código, arquitectura o comportamiento debe citar `archivo:línea` reales y separar explícitamente los `HECHOS` observados de las `HIPÓTESIS` inferidas. Está prohibido inventar rutas, símbolos, tablas o resultados.
- **Detención ante ambigüedad (`G6`):** si una especificación entra en conflicto con esta constitución, con [wiki/risk_zones.md](wiki/risk_zones.md) o presenta un enlace roto o brecha de seguridad, la ejecución se detiene de inmediato y se solicita resolución al `[AUTOR]`.

## Gobernanza de Modificaciones

Este documento y [wiki/risk_zones.md](wiki/risk_zones.md) son artefactos gobernados de alto impacto:

- Queda prohibido editarlos de forma directa o unilateral por un agente constructor.
- Todo cambio debe formularse como una propuesta `BrainPatch` dentro de `_proposals/` mediante `brain.exe propose`.
- La aplicación exige validación OKF y `G4` limpia más aprobación explícita del `[AUTOR]` (`brain.exe apply --approve-autor`), generando automáticamente una entrada *append-only* en [log.md](log.md) y la reindexación de `.index/brain.db`.

## Referencias Núcleo

- Enrutador operativo: [AGENTS.md](AGENTS.md)
- Índice maestro: [index.md](index.md)
- Mapa de zonas de riesgo: [wiki/risk_zones.md](wiki/risk_zones.md)
- Bitácora *append-only*: [log.md](log.md)
