---
name: brain-update
description: Flujo gobernado de actualización de la bóveda OKF mediante init-vault, propose (BrainPatch en _proposals/) y apply (con --approve-autor en documentos críticos), cumpliendo G1-G6 y Opción i.
version: "1.0"
owner: 0_Brain
---

# Skill: `brain-update` (Actualización Gobernada de la Bóveda)

## Cuándo Cargar Esta Skill

Cargar cuando un equipo necesite inicializar la bóveda local, registrar una nueva decisión de arquitectura (`adr/`), documentar o ampliar conocimiento técnico (`wiki/`, `projects/`), o resolver un *drift* detectado por `brain-verify`.

## Regla de Oro de Gobernanza

- Nunca sobrescribir de forma directa documentos existentes de la bóveda sin pasar por el flujo de propuesta (`propose`) y validación (`apply`).
- Las modificaciones sobre `constitution.md` o `wiki/risk_zones.md` quedan bloqueadas por el motor a menos que exista aprobación explícita del `[AUTOR]` (`--approve-autor`).

## Comandos Operativos (`brain.exe`)

### 1. Inicializar Bóveda Base (Idempotente)

Instancia la estructura inicial (`AGENTS.md`, `index.md`, `constitution.md`, `log.md`, `wiki/risk_zones.md`, `adr/`, `wiki/`, `projects/`, `_proposals/`, `.index/`) sin sobrescribir archivos existentes:

```text
0_Brain\brain.exe init-vault --vault <RUTA_VAULT> --schemas <RUTA_SCHEMAS>
```

### 2. Proponer Actualización (`BrainPatch` en `_proposals/`)

Genera un archivo de propuesta estructurado en `_proposals/` sin mutar todavía los documentos activos:

```text
0_Brain\brain.exe propose --vault <RUTA_VAULT> --target <RUTA_RELATIVA> --title "<título>" --project <PROYECTO> [--body-file <RUTA_CONTENIDO>]
```

- Para propuestas que cubran código real, actualizar `covers_paths` (rutas relativas con `/`) y `verified_head` con el hash verificado en el repositorio objetivo.

### 3. Aplicar Propuesta Validada (`apply`)

Valida el patch contra el esquema OKF y el escáner determinista **G4**; si aprueba, aplica el cambio, añade una línea *append-only* en `log.md` y reconstruye `.index/brain.db`:

```text
0_Brain\brain.exe apply --vault <RUTA_VAULT> --proposal <RUTA_PROPUESTA> [--approve-autor]
```

- **Compuerta `[AUTOR]`:** si `--target` apunta a `constitution.md` o `wiki/risk_zones.md`, `brain.exe apply` rechazará la operación si no se incluye `--approve-autor` tras la decisión humana correspondiente.

## Cumplimiento Obligatorio (`G1–G6`) y Privacidad (`Opción i`)

- **G1:** adjuntar en la propuesta o reporte de cierre el comando `propose` / `apply`, su código de salida y las líneas finales de salida.
- **G2:** mantener consistencia con las rutas e identificadores declarados en `index.md` y metadatos OKF.
- **G3:** las pruebas de `propose` y `apply` fuera de la bóveda operativa se ejecutan únicamente en directorios temporales efímeros.
- **G4:** antes de proponer o aplicar, asegurar que el texto esté 100% libre de contraseñas en cadenas de conexión, tokens de autorización HTTP, API keys, claves privadas PEM/SSH, correos sensibles y rutas absolutas de perfiles personales de usuario.
- **G5:** todo `adr/` o `wiki/` propuesto debe citar `archivo:línea` verificados en el código real y distinguir `HECHOS` de `HIPÓTESIS`.
- **G6:** si existe ambigüedad sobre si un cambio impacta zonas de riesgo o la constitución, detener la aplicación y solicitar revisión del `[AUTOR]`.
- **Privacidad `Opción i`:** las propuestas en `_proposals/`, el contenido de `0_Brain/vault/` y la bitácora `log.md` permanecen locales e ignorados en `.gitignore`.
