---
id: adr-0000-titulo-corto
title: "ADR-0000: Título de la Decisión de Arquitectura"
type: adr
project: black-factory
status: proposed
tags:
  - adr
  - architecture
covers_paths:
  - ruta/relativa/modulo/
verified_head: ""
updated_at: "YYYY-MM-DD"
---

# ADR-0000: Título de la Decisión de Arquitectura

## Contexto y Problema

Describe el problema arquitectónico o restricción operativa que motiva esta decisión.

### HECHOS (G5)

- Registrar únicamente observaciones verificadas citando `ruta/archivo.ext:línea` y comandos reales con su código de salida (`G1`).
- Ejemplo de cita válida (en texto plano o código inline): `0_Brain/engine/validator.go:1`.

### HIPÓTESIS

- Documentar explícitamente cualquier suposición pendiente de verificación empírica antes de cerrar el ADR como `accepted`.

## Decisión

Explicar de forma concreta la opción seleccionada, su alcance exacto y los contratos que impone sobre otros módulos o equipos.

## Alternativas Consideradas

| Alternativa | Ventajas | Desventajas | Motivo de descarte |
| :--- | :--- | :--- | :--- |
| Opción A (descartada) | Simplicidad inicial | No cumple invariante requerida | Superada por la opción elegida |
| Opción B (elegida) | Determinismo y trazabilidad | Mayor disciplina inicial | Cumple `G1–G6` y requisitos |

## Consecuencias

- **Positivas:** beneficios medibles en mantenibilidad, latencia o determinismo.
- **Trade-offs:** costos operativos, restricciones o deuda técnica aceptada.
- **Impacto en `G3`, `G4` y zonas de riesgo:** indicar si involucra áreas cubiertas por `wiki/risk_zones.md` o requiere compuerta `[AUTOR]`.

## Cobertura y Verificación de Drift

- Mantener `covers_paths` usando separadores `/` y rutas relativas al repositorio del proyecto (sin rutas absolutas personales).
- Actualizar `verified_head` con el hash corto o completo del commit verificado en el repositorio objetivo para habilitar la detección de *drift* mediante `brain.exe verify`.

## Referencias

- Documentos relacionados en la bóveda (anotar rutas como código inline hasta que existan físicamente, por ejemplo `` `wiki/ejemplo-modulo.md` `` o `` `adr/ADR-0001-ejemplo.md` ``).
- Código fuente cubierto por esta decisión (citar rutas reales relativas al repositorio).
