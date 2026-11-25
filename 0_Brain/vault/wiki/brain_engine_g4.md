---
id: brain-engine-g4
title: "Wiki: Motor Determinista 0_Brain, Guardia G4 y Primitivas pkg/bfcore"
type: wiki
project: black-factory
status: active
tags:
  - wiki
  - g4
  - bfcore
  - sqlite-fts5
covers_paths:
  - 0_Brain/engine/
  - pkg/bfcore/
verified_head: ""
updated_at: "2026-11-22"
---

# Wiki: Motor Determinista `0_Brain`, Guardia `G4` y Primitivas `pkg/bfcore`

## Propósito y Alcance

Documenta la arquitectura interna del motor `0_Brain/engine` (`blackfactory/brain`) y su delegación de primitivas de seguridad y sistema de archivos en el paquete compartido `pkg/bfcore` (`blackfactory/pkg/bfcore`).

## Componentes Clave

| Componente | Archivo:Línea (`G5`) | Responsabilidad |
| :--- | :--- | :--- |
| `ScanG4All` / `ScanTier1` / `RedactTier2Text` | `pkg/bfcore/g4.go:118` | Registro canónico de las 11 reglas `G4` (8 Tier 1 secretos duros + 3 Tier 2 rutas personales y correos) |
| `MatchCoverPathFold` | `pkg/bfcore/paths.go:38` | Normalización de rutas y coincidencia de prefijos/globs insensible a mayúsculas para Windows |
| `AcquireFileLock` | `pkg/bfcore/lock_windows.go:24` | Cerrojo exclusivo de kernel del SO (`LockFileEx` / `flock`) con reintentos acotados cada 25 ms |
| `CheckNoReparseChain` | `pkg/bfcore/reparse_windows.go:37` | Verificación `os.Lstat` de toda la cadena de ancestros contra symlinks y junctions NTFS |
| `ContextForFiles` / `ParseRiskZonesCatalog` | `0_Brain/engine/query.go:100` | Cruce de archivos contra `covers_paths` y emisión estructurada de `risk_hits` desde `wiki/risk_zones.md` |

## Referencias Núcleo

- Enrutador operativo: [../AGENTS.md](../AGENTS.md)
- Constitución: [../constitution.md](../constitution.md)
- Zonas de riesgo: [risk_zones.md](risk_zones.md)
