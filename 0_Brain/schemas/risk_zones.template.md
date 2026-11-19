---
id: brain-risk-zones
title: "Mapa de Zonas de Riesgo y Compuertas HITL"
type: risk_zones
project: black-factory
status: active
tags:
  - risk-zones
  - security
  - hitl
covers_paths: []
verified_head: ""
updated_at: "YYYY-MM-DD"
---

# Mapa de Zonas de Riesgo y Compuertas HITL

## Propósito y Uso en Black Factory

Este documento centraliza las zonas de código, datos e infraestructura cuyo cambio conlleva alto impacto operativo o de seguridad. Es insumo obligatorio para:

- **`2_Spec_Writer`:** identificar restricciones antes de redactar especificaciones.
- **`4_Revisores`:** focalizar auditorías de seguridad, arquitectura y `G5`.
- **`6_Filtro_de_Riesgo`:** aplicar la compuerta determinista HITL (*Human-In-The-Loop*) exigiendo PR y aprobación explícita del `[AUTOR]` cuando un diff toque cualquier ruta aquí catalogada.

## Categorías de Riesgo Innegociables

1. **Bases de datos, migraciones y mutación de datos (`G3`):** esquemas DDL, semillas operativas, scripts de migración o endpoints que alteren estado persistente.
2. **Autenticación, autorización, CSRF y exposición de secretos (`G4`):** control de acceso, manejo de sesiones, validación de permisos y protección contra fugas de credenciales o datos personales.
3. **Gobernanza y núcleo determinista:** [../constitution.md](../constitution.md), este archivo (`wiki/risk_zones.md`) y el motor determinista de validación/anillo (`GengiSWAP` y `0_Brain/engine/`).
4. **Integraciones críticas e infraestructura:** configuración de despliegue, tuberías de CI/CD, puertos o reglas de aislamiento entre entornos.

## Catálogo de Zonas de Riesgo

| ID | Proyecto | Rutas cubiertas (`covers_paths`) | Nivel | Compuerta obligatoria | Evidencia `G5` |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `RZ-001` | `black-factory` | `GengiSWAP/engine/` | `CRITICO` | PR + revisión `4_Revisores` + aprobación `[AUTOR]` | `GengiSWAP/engine/ring.go:1` |
| `RZ-002` | `black-factory` | `0_Brain/engine/` | `ALTO` | `go vet` + `go test -count=1 ./...` + revisión `G4`/`G5` | `0_Brain/README.md:8` |

## Gobernanza del Mapa

Toda adición, modificación o retiro de una entrada en este catálogo exige un procedimiento gobernado estricto:

- Redactar una propuesta `BrainPatch` en `_proposals/` vía `brain.exe propose`.
- Validar esquema OKF y escaneo `G4` limpio.
- Aplicar únicamente con aprobación explícita del `[AUTOR]` (`brain.exe apply --approve-autor`), registrando el evento de forma *append-only* en [../log.md](../log.md).

## Referencias Núcleo

- Enrutador operativo: [../AGENTS.md](../AGENTS.md)
- Índice maestro: [../index.md](../index.md)
- Constitución: [../constitution.md](../constitution.md)
- Bitácora *append-only*: [../log.md](../log.md)
