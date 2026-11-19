---
id: wiki-tema-ejemplo
title: "Wiki: Nombre del Módulo o Dominio"
type: wiki
project: black-factory
status: draft
tags:
  - wiki
  - domain
covers_paths:
  - ruta/relativa/dominio/
verified_head: ""
updated_at: "YYYY-MM-DD"
---

# Wiki: Nombre del Módulo o Dominio

## Propósito y Alcance

Resumen claro de qué resuelve este módulo o dominio, qué equipos lo consumen y qué queda explícitamente fuera de su responsabilidad.

## Componentes Clave

Documentar únicamente componentes verificados en el código fuente (`G5`), distinguiendo hechos confirmados de hipótesis.

| Componente | Archivo:Línea (`G5`) | Responsabilidad |
| :--- | :--- | :--- |
| `NombreComponente` | `ruta/relativa/archivo.go:1` | Descripción breve de su función |

## Flujo Operativo

Usar exclusivamente diagramas en texto plano ASCII o Unicode Box-Drawing dentro de bloques `` ```text `` (queda prohibido el uso de bloques de diagramación externa no offline).

```text
┌─────────────────┐      ┌──────────────────────┐      ┌─────────────────┐
│ Entrada / Evento│ ───► │ Componente Principal │ ───► │ Salida / Estado │
└─────────────────┘      └──────────────────────┘      └─────────────────┘
```

## Invariantes, Contratos y Dependencias

- **Contratos de entrada/salida:** formatos, tipos o interfaces expuestas.
- **Dependencias internas/externas:** paquetes, servicios o documentos relacionados (citar rutas como código inline hasta validar su existencia en la bóveda, p. ej. `` `adr/ADR-0001-ejemplo.md` ``).

## Operación Segura (`G3` y `G4`)

- Confirmar cómo se ejecutan pruebas sin mutar datos reales (`G3`, uso de fixtures o directorios temporales efímeros).
- Confirmar ausencia absoluta de credenciales, tokens HTTP, API keys, claves privadas PEM/SSH, correos sensibles o rutas absolutas de perfiles personales (`G4`).

## Mantenimiento y Control de Drift

- Verificar que `covers_paths` liste todas las rutas o prefijos de directorio relevantes con `/`.
- Actualizar `verified_head` cada vez que un cambio en las rutas cubiertas sea contrastado con el código real.
- Ejecutar `brain.exe verify --vault <RUTA_VAULT> --repo <PROYECTO>=<RUTA_REPO>` para detectar obsolescencia respecto a commits recientes.
