---
name: intake-triage
description: Normalización determinista de solicitudes e incidencias, frontera G4 en dos niveles, resolución de cuarentena G6 y triaje de carril ASDD/FTD mediante 1_Intake/intake.exe.
version: "1.0"
owner: 1_Intake
---

# Skill: `intake-triage` (Recepción Determinista, Frontera G4 y Triaje ASDD/FTD)

## Cuándo Cargar Esta Skill

Cargar **siempre que ingrese una nueva solicitud, reporte de bug, hallazgo de auditoría o cambio operativo** a **Black Factory**, o cuando se deba promover un `WorkItem` retenido en cuarentena (`status: "needs_clarification"`).

## Precondiciones (*Read-Before-Act* con `0_Brain`)

`1_Intake/intake.exe` verifica automáticamente en modo solo lectura (`NG2`) que:
1. `0_Brain/vault/.index/brain.db` exista en disco.
2. No exista un diario pendiente `0_Brain/vault/.brain-journal.json` (si existe, `intake.exe` aborta con código `4` sin mutar la bóveda e indica ejecutar `0_Brain/brain.exe verify`).

## Comandos Operativos (`1_Intake/intake.exe`)

### 1. Ingestar y Clasificar una Solicitud (`ingest`)

Desde un archivo JSON local (`<= 1 MiB`) o `stdin` (`--input -`), con soporte de sobrescritura por banderas CLI:

```text
1_Intake\intake.exe ingest --input <archivo.json|-> [--project <slug>] [--kind <bug|feature|refactor|audit_fix|ops>] [--source <origen>] [--source-ref <ref>] [--title "<titulo>"] [--description "<desc>"] [--path <ruta/relativa.go>] [--impact-flag <contract|schema|db|auth|security|governance>] [--strict-g4] [--update] [--dry-run] [--json]
```

### 2. Resolver un WorkItem en Cuarentena (`resolve`)

Completa campos faltantes o reemplaza `affected_paths` para promover atómicamente de `runtime/quarantine/` a `runtime/workitems/`:

```text
1_Intake\intake.exe resolve --id WI-<hash12> [--project <slug>] [--source-ref <ref>] [--path <ruta/relativa.go>] [--note "<nota>"] [--update] [--dry-run] [--json]
```

### 3. Listar, Consultar y Verificar Integridad (`list`, `show`, `verify`)

```text
1_Intake\intake.exe list [--status <ready|needs_clarification>] [--lane <ASDD|FTD>] [--project <slug>] [--risk <NONE|BAJO|MEDIO|ALTO|CRITICO>] [--json]
1_Intake\intake.exe show --id WI-<hash12> [--format <json|md>] [--json]
1_Intake\intake.exe verify --vault 0_Brain\vault --data-dir 1_Intake\runtime --repo-root . --json
```

## Contrato de Códigos de Salida (`0..5`)

| Código | Nombre Simbólico | Significado Operativo | Acción del Agente |
| :---: | :--- | :--- | :--- |
| **`0`** | `ExitOK` | `WorkItem` clasificado como `ready` (`ASDD` o `FTD`), hit idempotente, o `list`/`show`/`verify` exitoso. | Entregar `WI-<hash12>.json` a `2_Spec_Writer` (`ASDD`) o `3_Builder` (`FTD`). |
| **`1`** | `ExitUsageOrError` | Banderas inválidas, JSON malformado, entrada `> 1 MiB`, symlink/reparse point, ID inexistente o fallo en `verify`. | Corregir sintaxis o invocación CLI. |
| **`2`** | `ExitQuarantined` | Retenido en `runtime/quarantine/WI-<hash12>.json` (`needs_clarification`: campos faltantes, ruta inválida/G4, alcance vacío o proyecto inactivo). | Revisar `quarantine_info.resolve_hint`, solicitar aclaración (`G6`) y ejecutar `intake.exe resolve`. |
| **`3`** | `ExitRejectedG4` | Secreto duro Tier 1 detectado en bytes crudos o strings JSON decodificados (o Tier 2 con `--strict-g4`). `0` archivos creados en `workitems/` o `quarantine/`. | Eliminar la credencial/secreto del payload original antes de reintentar (`G4`). |
| **`4`** | `ExitBrainOrLockUnavailable` | `0_Brain` no disponible (`.index/brain.db` ausente, `.brain-journal.json` activo o fallo de `brain.exe`) o `.intake.lock` ocupado. | Ejecutar `0_Brain/brain.exe index` o `verify` y reintentar. |
| **`5`** | `ExitDedupeConflict` | Ya existe `workitems/WI-<hash12>.json` con el mismo `dedupe_key` (`project + source + source_ref`) pero distinto payload y no se pasó `--update`. | Confirmar si corresponde actualizar la revisión existente con `--update`. |

## Cumplimiento Obligatorio (`G1–G6`) y Privacidad (`Opción i`)

- **G1 / G5:** conservar la salida JSON de `intake.exe` y las invocaciones literales de `brain_evidence`.
- **G2:** usar `intake.exe show --id <WI-ID> --format md` para vistas legibles con tablas ASCII/Unicode Box-Drawing (nunca Mermaid).
- **G3 / NG2:** usar `--dry-run` cuando se requiera simular el triaje con `0` escrituras en disco.
- **G4 / Opción i:** nunca versionar en Git `1_Intake/runtime/` ni `1_Intake/intake.exe`; confirmar siempre con `intake.exe verify --json` (`git_ignored: true`, `ok: true`).
