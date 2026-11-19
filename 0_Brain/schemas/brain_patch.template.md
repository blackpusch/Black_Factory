---
id: patch-0000-descripcion-corta
title: "BrainPatch: Descripción del Cambio Propuesto"
type: brain_patch
project: black-factory
status: proposed
tags:
  - proposal
  - brain-patch
covers_paths: []
verified_head: ""
updated_at: "YYYY-MM-DD"
target_path: wiki/ejemplo.md
operation: update
requires_autor_approval: false
author_role: 2_Spec_Writer
---

# BrainPatch: Descripción del Cambio Propuesto

## Resumen del Cambio

Explicar qué archivo de la bóveda (`target_path`) se propone crear o actualizar, por qué es necesario (p. ej. nueva decisión arquitectónica, corrección de *drift* o registro de zona de riesgo) y qué impacto tiene en otros documentos.

## Evidencia y Trazabilidad (`G1` y `G5`)

### HECHOS (`G5` / `G1`)

- Citar cada archivo y línea observada en el repositorio objetivo (`ruta/archivo.ext:línea`).
- Indicar el hash de commit (`verified_head`) y el resultado literal de las verificaciones ejecutadas (comando, código de salida y salida clave).

### HIPÓTESIS

- Declarar cualquier punto inferido que requiera confirmación adicional durante la revisión.

## Checklist de Seguridad y Privacidad (`G3`, `G4` y `Opción i`)

- [ ] No contiene contraseñas, tokens HTTP, API keys ni cadenas de conexión (`G4`).
- [ ] No contiene claves privadas PEM/SSH, correos personales/corporativos ni rutas absolutas de perfiles personales de usuario (`G4`).
- [ ] No introduce mutaciones sobre bases de datos o datos reales (`G3`).
- [ ] Mantiene la bóveda y sus índices fuera del repositorio Git (`Opción i`).
- [ ] Todos los enlaces Markdown internos resuelven a rutas válidas dentro de la bóveda.

## Compuerta de Aprobación `[AUTOR]`

- Si `target_path` afecta `constitution.md` o `wiki/risk_zones.md`, el campo `requires_autor_approval` **debe** establecerse en `true`.
- El motor `brain.exe apply` rechazará aplicar cambios sobre dichos archivos críticos si no se especifica explícitamente la bandera `--approve-autor`.

## Contenido Propuesto

Incluir debajo de este encabezado (o en el cuerpo del patch según la operación `create`/`update`) el contenido Markdown completo con su frontmatter OKF válido, sin romper identificadores únicos ni enlaces internos.

```markdown
---
id: wiki-ejemplo
title: "Wiki: Ejemplo"
type: wiki
project: black-factory
status: active
tags:
  - wiki
covers_paths:
  - ruta/relativa/modulo/
verified_head: ""
updated_at: "YYYY-MM-DD"
---

# Wiki: Ejemplo
...
```
