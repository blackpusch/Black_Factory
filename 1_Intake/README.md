# Equipo 1: `1_Intake` (Recepción, Frontera G4 en Dos Niveles y Triaje Determinista)

## Overview
Responsable de recibir solicitudes de ingeniería y reportes de defectos desde banderas CLI o archivos/flujos JSON estructurados (`--input <archivo.json>` o `--input -` por `stdin` hasta `1 MiB`), aplicar la frontera de seguridad y sanitización `G4` en dos niveles (Tier 1 fail-closed con salida `3` y `0` archivos de payload + Tier 2 redacción en punto fijo `[REDACTED:G4:<rule>]`), consultar `0_Brain` en modo estrictamente de solo lectura (*Read-Before-Act*, `NG2`) y emitir un `WorkItem` JSON versionado (`schema_version: "1.0"`) asignando el carril metodológico (`ASDD` o `FTD`) o cuarentena segura (`needs_clarification`, salida `2`), siguiendo la arquitectura híbrida **Opción 3C** y privacidad **Opción i**.

> Especificación técnica aprobada: [`Docs/designs/1_intake.md`](../Docs/designs/1_intake.md)

## Subdirectories & System Boundaries
- `engine`: [./engine/README.md](./engine/README.md) - Motor determinista pure-Go (`blackfactory/intake`, `CGO_ENABLED=0`, sin SQLite, consumiendo `pkg/bfcore`) y CLI `intake.exe` ([`./engine/cmd/intake/README.md`](./engine/cmd/intake/README.md)).
- `schemas`: [./schemas/workitem.schema.json](./schemas/workitem.schema.json) - Esquema canónico JSON Schema Draft 2020-12 (`schema_version: "1.0"`) para `WorkItem`.
- `skills`: [./skills/README.md](./skills/README.md) - Catálogo de skills modulares de carga progresiva (`intake-triage`).

## Estructura del Módulo

```text
1_Intake\
├── README.md                          (este documento)
├── schemas\
│   └── workitem.schema.json           (JSON Schema Draft 2020-12 v1.0 para WorkItem)
├── skills\
│   ├── README.md                      (catálogo de skills para Equipo 1)
│   └── intake-triage\
│       └── SKILL.md                   (guía operativa de carga progresiva para intake.exe)
├── engine\                            (módulo Go blackfactory/intake, CGO_ENABLED=0, sin SQLite)
│   ├── go.mod / go.sum / README.md
│   ├── model.go                       (WorkItem v1.0, enums, dedupe_key, sanitized_payload_sha256)
│   ├── input.go                       (lector JSON/stdin <= 1 MiB, BOM strip, doble fase Tier 1 G4)
│   ├── paths.go                       (higiene pre-path.Clean de affected_paths y CountRootModules)
│   ├── brain_client.go                (BrainClient, preflight read-only os.Lstat, consulta OR FTS5)
│   ├── triage.go                      (compuerta de localizabilidad + matriz determinista 13 reglas)
│   ├── store.go                       (persistencia atómica bajo .intake.lock y telemetría JSONL)
│   ├── render.go                      (vista Markdown ASCII/Box-Drawing bajo demanda para show)
│   ├── intake_test.go                 (pruebas unitarias TDD y benchmark de latencia < 500 ms p95)
│   └── cmd\intake\
│       ├── README.md
│       ├── main.go                    (CLI intake.exe: ingest, resolve, list, show, verify; exits 0..5)
│       └── main_test.go               (pruebas E2E de ciclo completo sobre t.TempDir())
├── intake.exe                         (binario compilado localmente; ignorado por .gitignore)
└── runtime\                           (datos operativos Opción i; ignorado por .gitignore)
    ├── .intake.lock                   (cerrojo exclusivo de kernel del SO; persiste en disco)
    ├── workitems\                     (WorkItems listos: WI-<hash12>.json)
    ├── quarantine\                    (retenciones G6: WI-<hash12>.json)
    └── intake_events.jsonl            (bitácora append-only de eventos y rechazos REJECTED_G4)
```

## Flujo de Triaje en Cinco Etapas

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ ETAPA 1: Frontera Cruda (<= 1 MiB, BOM strip) + Fase-A Tier 1 G4 ──► EX 3  │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ETAPA 2: Decodificación JSON + Fase-B Tier 1 G4 + Redacción Punto Fijo     │
│          Tier 2 + Higiene Pre-Clean de affected_paths            ──► EX 3  │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ETAPA 3: Preflight Read-Only 0_Brain (os.Lstat, NG2) + Compuerta Metadatos │
│          y Alcance Explícito (Cuarentena G6 si incompleto)   ──► EX 4 / 2  │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ETAPA 4: Consulta 0_Brain (<= 2 subprocesos: context + query OR)           │
│          y Verificación de Proyecto Activo                       ──► EX 2  │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ ETAPA 5: Triaje Determinista (ASDD vs FTD), ScanG4All Pre-Escritura y      │
│          Commit Atómico bajo .intake.lock                    ──► EX 0 / 5  │
└────────────────────────────────────────────────────────────────────────────┘
```

## Invariants
- **Cero CGO y Cero SQLite (`IG1`):** `1_Intake/engine` compila con `CGO_ENABLED=0` compartiendo `blackfactory/pkg/bfcore` y permitiendo únicamente `golang.org/x/sys`.
- **Doble Fase Tier 1 G4 y Barrera Pre-Escritura `ScanG4All` (`IG2`, `G4`):** las 8 reglas Tier 1 bloquean secretos tanto en bytes crudos como en strings JSON decodificados (`\u0067hp_...`) con código `3` y `0` archivos en `workitems/` o `quarantine/`, registrando únicamente un evento `REJECTED_G4` libre de secretos en `intake_events.jsonl`. Ningún byte se escribe a disco sin pasar `bfcore.ScanG4All` con `0` hallazgos.
- **Consumidor Estrictamente de Solo Lectura de `0_Brain` (`NG2`):** `Preflight` inspecciona `.index/brain.db` y la ausencia de `.brain-journal.json` vía `os.Lstat` sin invocar `brain.exe verify` ni mutar `0_Brain/vault/`.
- **Identidad Unificada `WI-<hash12>` e Idempotencia (`IG4`):** `WI-<dedupe_key[:12]>` cuando `(project, source, source_ref)` están presentes, o `WI-<sanitized_payload_sha256[:12]>` con `dedupe_key: ""` cuando están incompletos en cuarentena, con promoción atómica en `resolve` o re-ingesta.
- **Cerrojo de Kernel `.intake.lock` y Privacidad `Opción i` (`IG6`):** toda escritura en `1_Intake/runtime/` adquiere `.intake.lock` con backoff acotado (`250 ms`); `1_Intake/runtime/` y `1_Intake/intake.exe` permanecen excluidos en `.gitignore`.

## Side Effects
- Creación y reemplazo atómico de archivos `WI-<hash12>.json` en `1_Intake/runtime/workitems/` y `1_Intake/runtime/quarantine/`.
- Registro append-only de eventos JSONL en `1_Intake/runtime/intake_events.jsonl` bajo el cerrojo `.intake.lock`.
- Ejecución de hasta 2 subprocesos de solo lectura `0_Brain/brain.exe` (`context --json` y `query --json`).

## Verification
- Run: `go test -v -count=1 -cover ./...` (en `1_Intake/engine`)

## Compilación, Pruebas y Uso CLI Verificado

```powershell
# Compilación y suite de pruebas con cobertura (CGO_ENABLED=0)
Set-Location "C:\Black Factory\1_Intake\engine"
$env:CGO_ENABLED = "0"
go vet ./...
go test -v -count=1 -cover ./...
go build -o ..\intake.exe .\cmd\intake

# Ingestar solicitud, listar, mostrar vista Markdown bajo demanda y verificar integridad Opción i
& "C:\Black Factory\1_Intake\intake.exe" ingest --project black-factory --kind bug --source cli --source-ref BUG-104 --title "Fix validator delimiter" --description "Check G4 rule" --path "0_Brain/engine/validator.go" --dry-run --json
& "C:\Black Factory\1_Intake\intake.exe" list --json
& "C:\Black Factory\1_Intake\intake.exe" verify --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\1_Intake\runtime" --repo-root "C:\Black Factory" --json
```
