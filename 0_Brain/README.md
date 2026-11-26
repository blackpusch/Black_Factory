# Equipo 0: `0_Brain` (Memoria Persistente y Contexto)

## Overview
Responsable de mantener la memoria persistente, trazable y rápida de **Black Factory** y de cada proyecto gestionado, siguiendo la arquitectura híbrida **Opción 3C** y privacidad **Opción i**.

> Documentación técnica completa: [`Docs/BLACK_FACTORY_BRAIN.md`](../Docs/BLACK_FACTORY_BRAIN.md)

## Subdirectories & System Boundaries
- `engine`: [./engine/README.md](./engine/README.md) - Motor determinista en Go (`blackfactory/brain`) y CLI `brain.exe` (validación OKF, FTS5, drift Git, gobernanza Ed25519).
- `skills`: [./skills/README.md](./skills/README.md) - Catálogo de skills modulares de carga progresiva (`brain-query`, `brain-update`, `brain-verify`).
- `schemas`: [./schemas/AGENTS.template.md](./schemas/AGENTS.template.md) - 7 plantillas canónicas OKF (AGENTS, index, constitution, adr, wiki, risk_zones, brain_patch).

## Estructura del Módulo

```text
0_Brain\
├── README.md                  (este documento)
├── schemas\                   (7 plantillas OKF: AGENTS, index, constitution, adr, wiki, risk_zones, brain_patch)
├── skills\                    (catálogo README.md + brain-query, brain-update, brain-verify)
├── engine\                    (módulo Go blackfactory/brain con modernc.org/sqlite, CGO_ENABLED=0)
│   ├── go.mod / go.sum
│   ├── frontmatter.go         (parser OKF YAML plano y enlaces relativos)
│   ├── validator.go           (ValidateVault, campos obligatorios, unicidad ID, enlaces y guardia G4)
│   ├── index.go / query.go    (índice atómico SQLite FTS5/BM25 y ContextForFiles por covers_paths)
│   ├── drift.go / vault.go    (CheckDrift vía git diff, InitVault, ProposeUpdate, ApplyUpdate, SignProposal, diario .brain-journal.json)
│   ├── lock_*.go / reparse_*.go (cerrojo de SO .brain.lock y protección contra enlaces/reparse points)
│   └── cmd\brain\main.go      (CLI brain.exe con subcomando sign y BrainService preparado para MCP)
├── brain.exe                  (compilado localmente; ignorado por .gitignore)
└── vault\                     (bóveda privada local e índice .index\brain.db; ignorada por .gitignore)
```

## Invariants
- Toda propuesta sobre documentos gobernados (`constitution.md`, `wiki/risk_zones.md`) exige firma Ed25519 del `[AUTOR]` (`--autor-sig` / `--approve-autor`).
- Validación determinista G4 innegociable: detección de secretos, credenciales, tokens y rutas personales sin transcribir valores sensibles.
- Bloqueo exclusivo a nivel de SO sobre `.brain.lock` antes de verificar o aplicar propuestas.
- Integridad transaccional y recuperación fail-closed mediante diario atómico `.brain-journal.json`.

## Side Effects
- Mutación atómica de documentos y registro append-only en `log.md` dentro de la bóveda.
- Creación y actualización del índice SQLite FTS5 en `.index/brain.db`.
- Bloqueo de archivos a nivel de kernel mediante Windows `LockFileEx` / POSIX `flock`.

## Diseño (`Opción 3C` + `Opción i`)

- **Estándar documental (`rsc-harness`):** *Open Knowledge Format* (Markdown + frontmatter YAML), `AGENTS.md`, `index.md`, `constitution.md`, `log.md`, `adr/`, `wiki/` y skills modulares [`SKILL.md`](skills/README.md) de carga progresiva (*Read-Before-Act*).
- **Motor determinista en Go ([`engine/`](engine/README.md) → `brain.exe`):**
  - Parser OKF y validación estricta de esquemas, unicidad de `id` y enlaces internos ([`frontmatter.go`](engine/frontmatter.go), [`validator.go`](engine/validator.go)).
  - Guardia determinista **G4** (escaneo de secretos, tokens, cadenas de conexión, correos y rutas personales sin transcribir valores sensibles).
  - Índice local **SQLite FTS5** (BM25, `modernc.org/sqlite` *pure-Go* sin CGO) + cruce por `covers_paths` e inclusión de transversales ([`index.go`](engine/index.go), [`query.go`](engine/query.go)).
  - Detección de *drift* comparando `verified_head..HEAD` vía `git diff --name-only` ([`drift.go`](engine/drift.go)).
  - Flujo gobernado de propuestas (`propose` / `sign` / `apply`, exigiendo firma Ed25519 `--autor-sig` / `--approve-autor` contra `.autor/autor.pub` para `constitution.md` y `wiki/risk_zones.md`, cerrojo `.brain.lock`, diario *fail-closed* `.brain-journal.json`, registro *append-only* en `log.md` y reindexación) ([`vault.go`](engine/vault.go)).
  - Interfaz [`BrainService`](engine/cmd/brain/main.go#L18-L28) preparada para acoplar un servidor **MCP** local (`brain mcp`) en Fase 2.
- **Privacidad (`Opción i`):**
  - **Versionado en Git:** `README.md`, [`schemas/`](schemas/AGENTS.template.md), [`skills/`](skills/README.md) y [`engine/`](engine/README.md).
  - **Untracked ([`.gitignore`](../.gitignore#L22-L27)):** `vault/` (bóveda local, `_proposals/`, `log.md` e índice `.index/brain.db`) y `brain.exe` (confirmado con `!!` en `git status --ignored -s`).

## Verification
- Run: `go test -count=1 ./...` (en `0_Brain/engine`)

## Compilación, Pruebas y Uso CLI Verificado

```powershell
# Compilación y suite de regresión (CGO_ENABLED=0)
Set-Location "C:\Black Factory\0_Brain\engine"
$env:CGO_ENABLED = "0"
go vet ./...
go test -count=1 ./...
go build -o ..\brain.exe .\cmd\brain

# Inicializar, indexar, consultar y verificar la bóveda privada local
& "C:\Black Factory\0_Brain\brain.exe" init-vault --vault "C:\Black Factory\0_Brain\vault" --schemas "C:\Black Factory\0_Brain\schemas"
& "C:\Black Factory\0_Brain\brain.exe" index --vault "C:\Black Factory\0_Brain\vault" --json
& "C:\Black Factory\0_Brain\brain.exe" query --vault "C:\Black Factory\0_Brain\vault" "gobernanza" --json
& "C:\Black Factory\0_Brain\brain.exe" context --vault "C:\Black Factory\0_Brain\vault" --project black-factory --files "0_Brain/engine/validator.go" --json
& "C:\Black Factory\0_Brain\brain.exe" verify --vault "C:\Black Factory\0_Brain\vault" --json
```
