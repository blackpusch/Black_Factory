# Black Factory — Documentación Técnica de `0_Brain`

> **Versión verificada:** `2026-11-19` (`bf_00_brain_v1`, rama `feat/0-brain`)  
> **Alcance del documento:** Arquitectura **Opción 3C**, modelo de privacidad **Opción i**, estándar documental OKF/RSC, motor determinista Go (`0_Brain/engine/` → `0_Brain/brain.exe`) con SQLite FTS5 sin CGO, validación `G4`/enlaces, detección de *drift* Git, flujo gobernado (`propose`/`apply`) y hoja de ruta **MCP** (Fase 2).

---

## 1. Visión General y Arquitectura Híbrida (`Opción 3C`)

[`0_Brain`](../0_Brain/README.md) es el módulo de memoria persistente, contexto rápido y gobernanza de **Black Factory**. Combina:

1. **Capa Documental OKF/RSC (`ericrisco/rsc-harness`):** plantillas estructuradas en Markdown + frontmatter YAML (`0_Brain/schemas/`), enrutamiento progresivo *Read-Before-Act* (`AGENTS.md`, `index.md`, `constitution.md`, `wiki/risk_zones.md`, `log.md`, `adr/`, `wiki/`, `projects/`) y *skills* modulares (`0_Brain/skills/`).
2. **Motor Determinista en Go (`0_Brain/engine/` → `0_Brain/brain.exe`):** parser OKF sin dependencias YAML pesadas, verificador de enlaces relativos y guardas **G4**, índice local **SQLite FTS5** (BM25 + `covers_paths`) vía driver *pure-Go* `modernc.org/sqlite` (`CGO_ENABLED=0`), detector de *drift* contra `git diff` y compuerta humana `[AUTOR]` para aplicar propuestas sobre documentos críticos.

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│                EQUIPOS CONSUMIDORES (1_Intake .. 6_Filtro_de_Riesgo)         │
│   Read-Before-Act: brain.exe query / context / verify ─► propose ─► apply    │
└───────────────────────────────────────┬──────────────────────────────────────┘
                                        │
                                        ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                 MOTOR DETERMINISTA GO (0_Brain/engine/ -> brain.exe)         │
│                                                                              │
│  frontmatter.go + validator.go ──► Parser OKF, enlaces relativos y guardia G4│
│  index.go + query.go           ──► SQLite FTS5 (modernc.org/sqlite, BM25)    │
│  drift.go                      ──► git diff --name-only (verified_head..HEAD)│
│  vault.go                      ──► init-vault, propose (_proposals/), apply  │
│  cmd/brain/main.go             ──► BrainService / LocalService + CLI         │
└───────────────────┬───────────────────────────────────────┬──────────────────┘
                    │                                       │
                    ▼                                       ▼
┌───────────────────────────────────────┐   ┌──────────────────────────────────┐
│   VERSIONADO EN GIT (Trackeado)       │   │  PRIVADO / LOCAL (Opción i)      │
│  • 0_Brain/README.md                  │   │  Ignorado en .gitignore (!!):    │
│  • 0_Brain/schemas/ (7 plantillas)    │   │  • 0_Brain/vault/                │
│  • 0_Brain/skills/  (README + 3 SKILL)│   │    (AGENTS.md, constitution.md,  │
│  • 0_Brain/engine/  (Go + tests)      │   │     wiki/risk_zones.md, .index/  │
│  • Docs/BLACK_FACTORY_BRAIN.md        │   │     brain.db, _proposals/, log)  │
└───────────────────────────────────────┘   │  • 0_Brain/brain.exe             │
                                            └──────────────────────────────────┘
```

### Integración con el Pipeline de Black Factory

- [`1_Intake`](../1_Intake/README.md): consulta `0_Brain` con `brain-query` durante el triaje de incidencias (`WorkItem`) y clasificación de carril (`ASDD`/`SDD` o `FTD`).
- [`2_Spec_Writer`](../2_Spec_Writer/README.md): aplica *Read-Before-Act* (`AGENTS.md` → `index.md` → `constitution.md` → `wiki/risk_zones.md` → `query`/`context`) antes de especificar, y abre propuestas `BrainPatch` con `brain-update`.
- [`3_Builder`](../3_Builder/README.md): consulta `brain.exe context --files ...` sobre los archivos a modificar dentro del anillo de [`GengiSWAP`](../GengiSWAP/gengis_swap_common.md) y verifica ausencia de *drift* con `brain-verify`.
- [`4_Revisores`](../4_Revisores/README.md) y [`5_Cycle_Approver`](../5_Cycle_Approver/README.md): validan entregas en contexto limpio frente a `constitution.md`, `wiki/risk_zones.md` y salidas literales `brain.exe verify`.
- [`6_Filtro_de_Riesgo`](../6_Filtro_de_Riesgo/README.md): cruza los archivos tocados con `wiki/risk_zones.md` y `covers_paths` para activar compuerta HITL `[AUTOR]` ante cambios de alto impacto.

---

## 2. Modelo de Privacidad (`Opción i`) y Control en `.gitignore`

Para mantener los procesos operativos internos, la constitución real, los mapas de riesgo y los índices locales fuera del repositorio público, [`.gitignore`](../.gitignore#L22-L27) excluye explícitamente:

- `0_Brain/vault/` (documentos vivos de la bóveda, propuestas `_proposals/`, bitácora `log.md`, `README_PRIVADO.md` e índice `.index/brain.db`).
- `0_Brain/brain.exe` (binario local compilado de forma reproducible desde `0_Brain/engine/`).
- `*.db`, `*.db-wal`, `*.db-shm`.

**Verificación observada con `git status --ignored -s` (2026-11-19):**

```text
!! 0_Brain/brain.exe
!! 0_Brain/vault/
!! GengiSWAP/gengis.exe
!! GengiSWAP/gengis_global_log.md
```

---

## 3. Estándar Documental OKF (`schemas/`) y Skills Modulares (`skills/`)

### 3.1 Plantillas OKF en `0_Brain/schemas/`

Todas las plantillas inician con frontmatter YAML plano con los **9 campos estructurados** (`id`, `title`, `type`, `project`, `status`, `tags`, `covers_paths`, `verified_head`, `updated_at`):

| Plantilla | Destino en bóveda / Uso | Propósito |
| :--- | :--- | :--- |
| [`AGENTS.template.md`](../0_Brain/schemas/AGENTS.template.md) | `AGENTS.md` | Enrutador operativo *Read-Before-Act* en 6 pasos, tabla por equipo y guardas `G1–G6` |
| [`index.template.md`](../0_Brain/schemas/index.template.md) | `index.md` | Índice maestro hacia documentos núcleo, proyectos, ADRs, wiki y `_proposals/` |
| [`constitution.template.md`](../0_Brain/schemas/constitution.template.md) | `constitution.md` | Principios innegociables, reglas `G3`–`G6` y gobernanza (protegido por `[AUTOR]`) |
| [`risk_zones.template.md`](../0_Brain/schemas/risk_zones.template.md) | `wiki/risk_zones.md` | Catálogo de zonas críticas y compuertas HITL (protegido por `[AUTOR]`) |
| [`adr.template.md`](../0_Brain/schemas/adr.template.md) | `adr/ADR-NNNN-slug.md` | Registro de decisiones de arquitectura separando `HECHOS (G5)` de `HIPÓTESIS` |
| [`wiki.template.md`](../0_Brain/schemas/wiki.template.md) | `wiki/<dominio>.md` | Documentación viva de módulos con `covers_paths`, `verified_head` y diagramas `text` |
| [`brain_patch.template.md`](../0_Brain/schemas/brain_patch.template.md) | `_proposals/patch-*.md` | Esquema `BrainPatch` con `target_path`, `operation`, `requires_autor_approval` y `author_role` |

### 3.2 Skills Modulares de Carga Progresiva (`0_Brain/skills/`)

Catálogo centralizado en [`0_Brain/skills/README.md`](../0_Brain/skills/README.md):

- [`brain-query/SKILL.md`](../0_Brain/skills/brain-query/SKILL.md): protocolo *Read-Before-Act*, `brain.exe index`, `brain.exe query` y `brain.exe context`.
- [`brain-update/SKILL.md`](../0_Brain/skills/brain-update/SKILL.md): flujo gobernado `init-vault` → `propose` → `apply` (exigiendo `--approve-autor` en documentos críticos).
- [`brain-verify/SKILL.md`](../0_Brain/skills/brain-verify/SKILL.md): ejecución y respuesta ante hallazgos de `brain.exe verify` (esquema OKF, enlaces, `G4` y *drift* Git).

---

## 4. Arquitectura del Motor Go (`0_Brain/engine/`)

Módulo `blackfactory/brain` ([`go.mod`](../0_Brain/engine/go.mod), `go 1.26.0`, compilado con `CGO_ENABLED=0` usando toolchain `go1.27.4 windows/amd64`) respaldado por [`go.sum`](../0_Brain/engine/go.sum) y la única dependencia externa `modernc.org/sqlite v1.62.0`.

| Archivo | Tamaño | Símbolos principales (`archivo:línea`) | Responsabilidad verificada |
| :--- | :---: | :--- | :--- |
| [`frontmatter.go`](../0_Brain/engine/frontmatter.go) | `609` L | `ParseDocument` (L99), `ParseFile` (L72), `ExtractRelativeLinks` (L414), `ListVaultMarkdownFiles` (L538) | Parser OKF puro (escalares, listas YAML de bloque/inline, extracción de enlaces relativos ignorando URLs `http(s)://`, bloques fenced y código inline) |
| [`validator.go`](../0_Brain/engine/validator.go) | `340` L | `ValidateVault` (L182), `ValidateDocumentMeta` (L116), `ScanG4` (L152), `g4Rules` (L49–L105) | Valida archivos núcleo (`AGENTS.md`, `index.md`, `constitution.md`, `log.md`), campos mínimos (`id`, `title`, `type`, `status`), unicidad de `id`, enlaces internos y 11 reglas **G4** sin transcribir valores sensibles |
| [`validator_test.go`](../0_Brain/engine/validator_test.go) | `499` L | `TestParseDocument_*`, `TestValidateVault_*` | Suite unitaria TDD incluyendo validación sobre las plantillas reales del Chunk 01 y 9 subtests **G4** |
| [`index.go`](../0_Brain/engine/index.go) | `405` L | `BuildIndex` (L96), `BuildIndexAt` (L101), `NormalizeCoverPath` (L32), `MatchCoverPath` (L54), `replaceFileAtomic` (L392) | Construcción atómica de `.index/brain.db` (archivo temporal en `.index/`, `PRAGMA journal_mode = DELETE`, `db.Close()` y `os.Rename` con reintentos en Windows), tablas `docs`, `doc_tags`, `doc_paths` y FTS5 `docs_fts` (`unicode61 remove_diacritics 2`) |
| [`query.go`](../0_Brain/engine/query.go) | `503` L | `Search` (L40), `ContextForFiles` (L81), `buildFTS5Query` (L408) | Búsqueda FTS5 con `bm25(docs_fts, 0.0, 0.0, 5.0, 2.0, 1.0)`, `snippet()`, filtros (`Project`, `Type`, `Tag`, `Limit`) y cruce de rutas contra `covers_paths` + transversales (`constitution`, `risk_zones`) |
| [`index_test.go`](../0_Brain/engine/index_test.go) | `388` L | `TestBuildIndex_*`, `TestSearch_*`, `TestContextForFiles_*`, `TestIndexAndSearch_Scale50DocsPerformance` | Pruebas TDD de idempotencia, ranking BM25, insensibilidad a tildes, globs Windows/POSIX y rendimiento sobre 62 documentos |
| [`drift.go`](../0_Brain/engine/drift.go) | `207` L | `CheckDrift` (L37), `GitHead` (L27), `gitDiffSince` (L172) | Compara `verified_head..HEAD` vía `git diff --name-only -z` contra `covers_paths` usando `MatchCoverPath` |
| [`vault.go`](../0_Brain/engine/vault.go) | `750` L | `InitVault` (L73), `ProposeUpdate` (L147), `ApplyUpdate` (L325), `IsProtectedVaultTarget` (L62) | Bootstrap idempotente, generación de `BrainPatch` en `_proposals/`, validación OKF/`G4`, compuerta `[AUTOR]` (`ErrAutorApprovalRequired`), escritura atómica, bitácora *append-only* en `log.md` y reindexación |
| [`cmd/brain/main.go`](../0_Brain/engine/cmd/brain/main.go) | `606` L | `BrainService` (L18–L28), `LocalService` (L30–L65), `run` (L85) | CLI con `init-vault`, `index`, `query`, `context`, `verify`, `propose`, `apply` y reserva `mcp` (L114–L116) |
| [`cmd/brain/main_test.go`](../0_Brain/engine/cmd/brain/main_test.go) | `384` L | `TestCLI_E2E_FullLifecycle`, `TestCLI_ApplyRejectsRiskZonesWithoutAutorAndG4Violations` | Pruebas E2E completas con repositorio Git temporal, *drift*, rechazo sin `--approve-autor`, aprobación y bloqueo **G4** |

### Reglas de Validación `G4` y Seguridad Implementadas

`ScanG4` ([`validator.go:49-105,152-178`](../0_Brain/engine/validator.go#L49-L105)) escanea línea a línea todos los archivos `.md` de la bóveda (incluidos `log.md`, propuestas en `_proposals/` y documentos con frontmatter malformado) detectando claves privadas PEM/literales, rutas absolutas de perfiles personales Windows/Unix/macOS, tokens HTTP, API keys conocidas, tokens JWT, cadenas de conexión con credenciales, asignaciones de secretos y correos electrónicos, reportando código `G4_VIOLATION`, archivo y línea **sin transcribir el valor sensible**.

---

## 5. Operación CLI (`brain.exe`), Compilación y Verificación

### 5.1 Compilación y Suite de Regresión (`CGO_ENABLED=0`)

```powershell
Set-Location "C:\Black Factory\0_Brain\engine"
$env:CGO_ENABLED = "0"
go vet ./...
go test -count=1 ./...
go build -o ..\brain.exe .\cmd\brain
```

### 5.2 Subcomandos Verificados sobre `0_Brain/vault/`

```powershell
# 1. Inicializar bóveda local (no sobrescribe archivos existentes; preserva README_PRIVADO.md)
& "C:\Black Factory\0_Brain\brain.exe" init-vault --vault "C:\Black Factory\0_Brain\vault" --schemas "C:\Black Factory\0_Brain\schemas"

# 2. Construir/reconstruir índice atómico SQLite FTS5 (.index/brain.db)
& "C:\Black Factory\0_Brain\brain.exe" index --vault "C:\Black Factory\0_Brain\vault" --json

# 3. Búsqueda FTS5 + BM25 y filtros
& "C:\Black Factory\0_Brain\brain.exe" query --vault "C:\Black Factory\0_Brain\vault" "gobernanza" --json

# 4. Contexto por archivos modificados + transversales (constitution, risk_zones)
& "C:\Black Factory\0_Brain\brain.exe" context --vault "C:\Black Factory\0_Brain\vault" --project black-factory --files "0_Brain/engine/validator.go" --json

# 5. Validación integral OKF + enlaces + G4 + drift Git opcional
& "C:\Black Factory\0_Brain\brain.exe" verify --vault "C:\Black Factory\0_Brain\vault" [--repo "black-factory=C:\Black Factory"] [--fail-on-drift] [--json]

# 6. Flujo gobernado de cambios (propuesta sin mutar + aplicación validada)
& "C:\Black Factory\0_Brain\brain.exe" propose --vault "C:\Black Factory\0_Brain\vault" --target "wiki/ejemplo.md" --title "Módulo Ejemplo" --project black-factory --covers "ruta/modulo/"
& "C:\Black Factory\0_Brain\brain.exe" apply --vault "C:\Black Factory\0_Brain\vault" --proposal "<RUTA_PROPUESTA>" [--approve-autor]
```

**Resultados observados en la inicialización y verificación local (2026-11-19):**
- `0_Brain/brain.exe`: `12,078,080` bytes compilado limpio (`CGO_ENABLED=0`).
- `brain.exe index --vault "C:\Black Factory\0_Brain\vault" --json`: `docs_indexed: 4`, `.index/brain.db` (`126,976` bytes), `duration_ms: 24`, exit `0`.
- `brain.exe query --vault "C:\Black Factory\0_Brain\vault" "gobernanza" --json`: 4 documentos ordenados por BM25 (`brain-constitution`, `brain-risk-zones`, `brain-index`, `brain-agents`), exit `0`.
- `brain.exe verify --vault "C:\Black Factory\0_Brain\vault" --json`: `ok: true`, `files_checked: 6`, `docs_parsed: 6`, `issues: null`, `drift: []`, exit `0`.

---

## 6. Hoja de Ruta Fase 2 — Servidor MCP (`brain mcp`)

El punto de entrada CLI ya separa el transporte de la lógica de dominio mediante la interfaz [`BrainService`](../0_Brain/engine/cmd/brain/main.go#L18-L28) (implementada por [`LocalService`](../0_Brain/engine/cmd/brain/main.go#L30-L65)) y reserva el subcomando [`mcp`](../0_Brain/engine/cmd/brain/main.go#L114-L116):

1. **Transporte MCP (`stdio` / JSON-RPC 2.0):** implementar `cmdMCP` delegando en `BrainService` sin duplicar reglas de parser, SQLite FTS5, validación `G4` ni *drift*.
2. **Herramientas expuestas a agentes externos:**
   - Lectura directa (`brain_query`, `brain_context`, `brain_verify`): seguras e idempotentes sobre `.index/brain.db` en `PRAGMA query_only = ON`.
   - Propuesta (`brain_propose`): escribe únicamente en `0_Brain/vault/_proposals/` sin alterar documentos activos.
3. **Invariante HITL preservada en MCP:** `brain_apply` sobre `constitution.md` o `wiki/risk_zones.md` continuará bloqueado por [`ApplyUpdate`](../0_Brain/engine/vault.go#L325) (`ErrAutorApprovalRequired`, código de salida `2` en CLI) mientras no medie autorización explícita del `[AUTOR]`.
