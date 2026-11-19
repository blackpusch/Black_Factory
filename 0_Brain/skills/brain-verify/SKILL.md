---
name: brain-verify
description: Verificación determinista de integridad OKF, enlaces internos, escaneo G4 y detección de drift Git con brain.exe verify e index, cumpliendo G1-G6 y Opción i.
version: "1.0"
owner: 0_Brain
---

# Skill: `brain-verify` (Validación OKF, Guardia G4 y Detección de Drift)

## Cuándo Cargar Esta Skill

Cargar al finalizar un cambio en código cubierto por `0_Brain`, antes de aprobar entregas en `4_Revisores` o `5_Cycle_Approver`, tras aplicar un `BrainPatch` y como chequeo periódico de salud e integridad de la bóveda.

## Comandos Operativos (`brain.exe`)

### Validación Completa de Bóveda y Drift Git

```text
0_Brain\brain.exe verify --vault <RUTA_VAULT> [--repo <PROYECTO>=<RUTA_REPO>] [--json]
```

### Reconstrucción Atómica del Índice

```text
0_Brain\brain.exe index --vault <RUTA_VAULT>
```

## Controles Deterministas Ejecutados

El motor `brain.exe verify` evalúa de forma determinista:

1. **Estructura base de la bóveda:** existencia de `AGENTS.md`, `index.md`, `constitution.md` y `log.md`.
2. **Esquema frontmatter OKF:** presencia de campos mínimos obligatorios (`id`, `title`, `type`, `status`) y unicidad estricta de `id` en toda la bóveda.
3. **Integridad de enlaces internos:** resolución de enlaces Markdown relativos (`[texto](ruta.md#ancla)`), ignorando URLs `http(s)://` y bloques fenced.
4. **Escaneo determinista `G4`:** detección de tokens de autorización HTTP, API keys, claves privadas PEM/SSH, cadenas de conexión con credenciales, correos sensibles y rutas absolutas de perfiles personales de usuario.
5. **Detección de *drift* Git (opcional con `--repo`):** comparación `verified_head..HEAD` mediante `git diff --name-only` contra `covers_paths` para identificar documentación obsoleta frente a commits reales.

## Protocolo de Respuesta ante Hallazgos

- **Violación `G4` o enlace roto:** detener inmediatamente la aprobación (`G6`), emitir evidencia literal (`G1`) con archivo y línea afectados, y corregir antes de reintentar.
- **Detección de *drift*:** no ignorar documentos desactualizados; abrir una propuesta gobernada siguiendo `brain-update` para alinear el documento con el estado real del código (`G5`) y actualizar `verified_head`.

## Cumplimiento Obligatorio (`G1–G6`) y Privacidad (`Opción i`)

- **G1:** registrar literalmente el comando `brain.exe verify`, su código de salida (`0` esperado en verde) y el resumen o JSON de salida.
- **G2:** respetar convenciones de reporte en artefactos de verificación si la ejecución forma parte de una tarea `gengis`.
- **G3:** `verify` es de solo lectura sobre el repositorio evaluado y la bóveda; nunca altera código fuente ni bases de datos productivas.
- **G4:** si `verify` reporta una fuga potencial, citar únicamente el archivo, línea y tipo de hallazgo sin transcribir el valor sensible.
- **G5:** contrastar cada aviso de *drift* revisando los archivos modificados en el diff real.
- **G6:** ante fallo de repositorio o ambigüedad en `verified_head`, detener y escalar al `[AUTOR]`.
- **Privacidad `Opción i`:** verificar periódicamente con `git status --ignored -s` que `0_Brain/vault/`, `0_Brain/brain.exe` y `*.db` permanezcan ignorados (`!!`) y fuera de Git.
