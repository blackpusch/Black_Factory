# Black Factory Blueprint — Software Factory sobre GengiSWAP & RSC

Repositorio oficial: `https://github.com/blackpusch/Black_Factory`

## System Overview

**Black Factory** es una factoría de software repetible, auditable y paralelizable que combina la metodología de arnés de **`ericrisco/rsc-harness`** (*Open Knowledge Format*, `constitution.md`, `AGENTS.md`, `SKILL.md` progresivos y carriles `ASDD`/`SDD`, `FTD`, `TDD`) con el motor determinista de **`GengiSWAP` (`gengis`)** (*las reglas las hace cumplir el motor, no la buena voluntad de ningún modelo*).

## Subdirectories & System Boundaries

- `0_Brain`: [./0_Brain/README.md](./0_Brain/README.md) — Memoria persistente (*The Brain*): estándar OKF/RSC + motor Go `brain.exe` con índice SQLite FTS5, validación `G4`/enlaces, detección de *drift* Git y gobernanza criptográfica Ed25519.
- `1_Intake`: [./1_Intake/README.md](./1_Intake/README.md) — Recepción y normalización de incidencias/peticiones (`WorkItem`) y triaje de carril (`ASDD` / `FTD`).
- `2_Spec_Writer`: [./2_Spec_Writer/README.md](./2_Spec_Writer/README.md) — Generación de especificaciones técnicas consultando obligatoriamente `0_Brain` antes de construir.
- `3_Builder`: [./3_Builder/README.md](./3_Builder/README.md) — Construcción y remediación bajo TDD en ramas/worktrees aislados, integrando el anillo determinista de `GengiSWAP/` (`gengis`).
- `4_Revisores`: [./4_Revisores/README.md](./4_Revisores/README.md) — Subagentes independientes con contexto limpio (Seguridad, Calidad/Arquitectura vs Brain + G5, UX/Flujo, UI).
- `5_Cycle_Approver`: [./5_Cycle_Approver/README.md](./5_Cycle_Approver/README.md) — Consolidación de reportes de revisores y rebote estructurado hacia `3_Builder` hasta un límite de ciclos.
- `6_Filtro_de_Riesgo`: [./6_Filtro_de_Riesgo/README.md](./6_Filtro_de_Riesgo/README.md) — Compuerta híbrida (reglas deterministas en Go + clasificador) entre vía rápida de bajo riesgo y PR con aprobación humana `[AUTOR]` para cambios críticos (`G3`, BD, auth, core).
- `Docs`: [./Docs/BLACK_FACTORY_BRAIN.md](./Docs/BLACK_FACTORY_BRAIN.md) — Documentación técnica transversal de arquitectura y protocolos ([`PROYECTO_TUNGSTENO.md`](./Docs/PROYECTO_TUNGSTENO.md)).
- `GengiSWAP`: [./GengiSWAP/README.md](./GengiSWAP/README.md) — Núcleo de ejecución determinista por chunks (`gengis.exe`), protocolos v2 y plantillas canónicas.

## Global Invariants

- **Reglas Globales G1–G6 innegociables:**
  - **G1 (Evidencia Literal):** registrar comando ejecutado, código de salida y salida literal en reportes de ejecución.
  - **G2 (Trazabilidad):** respetar encabezados de ubicación y formatos estándar para diagramas y artefactos generados.
  - **G3 (Integridad de Datos):** prohibida la mutación directa de bases de datos productivas; toda prueba se ejecuta en entornos efímeros aislados.
  - **G4 (Cero Fugas de Secretos):** escaneo determinista regex de secretos, tokens, llaves privadas PEM/SSH, rutas de usuarios y correos sensibles.
  - **G5 (Rigor Técnico):** citar `archivo:línea` reales verificables y separar hechos observados de hipótesis.
  - **G6 (Principio de Parada):** detenerse y solicitar confirmación al `[AUTOR]` ante cualquier ambigüedad, enlace roto o violación de alcance.
- **Gobernanza Criptográfica:** Toda alteración de documentos nucleares (`constitution.md`, `wiki/risk_zones.md`) exige firma Ed25519 del `[AUTOR]` y diario transaccional fail-closed (`.brain-journal.json`).
- **Aislamiento de Alcance (*Scope*):** Restricción estricta de rutas modificables (`CanModify`), lectura exclusiva (`ReadOnly`) y bloqueo total (`Forbidden`) con reversión automática en caso de violación.

## Global Side Effects

- **Persistencia e Índices:** Creación y actualización de bases de datos SQLite FTS5 locales (`.index/brain.db`) y control de concurrencia mediante cerrojo de SO (`.brain.lock`).
- **Trazabilidad Inmutable:** Registro append-only de eventos en `GengiSWAP/gengis_global_log.md` y `0_Brain/vault/log.md`.
- **Control de Versiones:** Automatización de `git commit`, `git stash` y `git revert` aislada por tarea o chunk en ramas dedicadas.

## System Verification

- `Shared Core (pkg/bfcore)`:
  ```powershell
  Set-Location "C:\Black Factory\pkg\bfcore"
  $env:CGO_ENABLED = "0"
  go test -count=1 ./...
  ```
- `0_Brain Engine`:
  ```powershell
  Set-Location "C:\Black Factory\0_Brain\engine"
  $env:CGO_ENABLED = "0"
  go test -count=1 ./...
  ```
- `1_Intake Engine`:
  ```powershell
  Set-Location "C:\Black Factory\1_Intake\engine"
  $env:CGO_ENABLED = "0"
  go test -count=1 ./...
  ```
- `2_Spec_Writer Engine`:
  ```powershell
  Set-Location "C:\Black Factory\2_Spec_Writer\engine"
  $env:CGO_ENABLED = "0"
  $env:GENGIS_CONTRACT_STRICT = "1"
  $env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
  go vet ./...
  go test -count=1 -cover ./...
  ```
- `3_Builder Engine`:
  ```powershell
  Set-Location "C:\Black Factory\3_Builder\engine"
  $env:CGO_ENABLED = "0"
  $env:GENGIS_CONTRACT_STRICT = "1"
  $env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
  go vet ./...
  go test -count=1 -cover ./...
  ```
- `GengiSWAP Engine`:
  ```powershell
  Set-Location "C:\Black Factory\GengiSWAP\engine"
  go test -count=1 ./...
  ```

---

## Núcleo `GengiSWAP/` (`gengis`) y Separación Operativa

- `GengiSWAP/`: Protocolos v2 (`gengis_swap_common.md`, `gengis_swap_prep_protocol.md`, `gengis_swap_chef_protocol.md`), plantillas `v1` (`templates/`) y motor determinista en **Go 1.24** (`GengiSWAP/engine/`, módulo `gengiswap/ring`).
- [`Docs/PROYECTO_TUNGSTENO.md`](Docs/PROYECTO_TUNGSTENO.md): Referencia técnica del motor `gengis`.
- **Artefactos privados / locales (ignorados en `.gitignore`):**
  - `0_Brain/vault/`, `0_Brain/brain.exe`, `1_Intake/runtime/`, `1_Intake/intake.exe`, `2_Spec_Writer/runtime/`, `2_Spec_Writer/specwriter.exe`, `3_Builder/runtime/`, `3_Builder/builder.exe` y archivos `*.db` (`Opción i`).
  - `GengiSWAP/gengis.exe` y `GengiSWAP/gengis_global_log.md`.
  - `C:\Desarrollo\GengiSWAP_data\` para tareas activas/archivadas (`gengis_task\`) y respaldos.

### Verificación y Compilación de `GengiSWAP` (`gengis.exe`)

```powershell
Set-Location "C:\Black Factory\GengiSWAP\engine"
go vet ./...
go test -count=1 ./...
go build -o ..\gengis.exe .\cmd\gengis
```

*(Nota: en entornos Windows sin pila de red activa, `TestShellRunnerWaitDelay` puede omitirse con `-skip TestShellRunnerWaitDelay`.)*

### Uso rápido de `gengis.exe` desde `Black Factory`

```powershell
& "C:\Black Factory\GengiSWAP\gengis.exe" status "C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>" --root "C:\Black Factory\GengiSWAP"
```

### Verificación y Compilación de `1_Intake` (`intake.exe`)

```powershell
Set-Location "C:\Black Factory\1_Intake\engine"
$env:CGO_ENABLED = "0"
go vet ./...
go test -count=1 -cover ./...
go build -o ..\intake.exe .\cmd\intake
& "C:\Black Factory\1_Intake\intake.exe" verify --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\1_Intake\runtime" --repo-root "C:\Black Factory" --json
```

### Verificación y Compilación de `2_Spec_Writer` (`specwriter.exe`)

```powershell
Set-Location "C:\Black Factory\2_Spec_Writer\engine"
$env:CGO_ENABLED = "0"
$env:GENGIS_CONTRACT_STRICT = "1"
$env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
go vet ./...
go test -count=1 -cover ./...
go build -o ..\specwriter.exe .\cmd\specwriter
& "C:\Black Factory\2_Spec_Writer\specwriter.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\2_Spec_Writer\runtime" --json
git -C "C:\Black Factory" check-ignore -v 2_Spec_Writer/runtime/
git -C "C:\Black Factory" check-ignore -v 2_Spec_Writer/specwriter.exe
```

### Verificación y Compilación de `3_Builder` (`builder.exe`)

```powershell
Set-Location "C:\Black Factory\3_Builder\engine"
$env:CGO_ENABLED = "0"
$env:GENGIS_CONTRACT_STRICT = "1"
$env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
go vet ./...
go test -count=1 -cover ./...
go build -o ..\builder.exe .\cmd\builder
& "C:\Black Factory\3_Builder\builder.exe" verify --factory-root "C:\Black Factory" --repo-root "C:\Black Factory" --vault "C:\Black Factory\0_Brain\vault" --data-dir "C:\Black Factory\3_Builder\runtime" --json
git -C "C:\Black Factory" check-ignore -v 3_Builder/runtime/
git -C "C:\Black Factory" check-ignore -v 3_Builder/builder.exe
```
