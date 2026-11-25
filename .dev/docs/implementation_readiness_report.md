# Revisión de Preparación para Implementación Consolidada — Etapa 9 (`DetailedRange`: líneas `501..720` de `Docs/designs/1_intake.md`)

Este documento consolida la auditoría de preparación para implementación ejecutada en la **Etapa 9 (Implementation Readiness Test)** sobre `DetailedRange` (`501..720`) de [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md) aplicando `implementation-readiness-standard` a través de tres lentes especializados:
1. **Tech Lead Pragmático (`egm-implementation-readiness`)**: Verificación de instrucciones de ejecución sin ambigüedades ni decisiones de diseño ocultas para el implementador.
2. **Auditor Pedante de Detalles del Código (`egm-codebase-details`)**: Contraste exacto de llamadas de sistema, firmas Go, banderas CLI, esquemas y números de línea contra `0_Brain` y `GengiSWAP`.
3. **Editor de Estilo Técnico (`egm-style`)**: Consistencia de nombres de símbolos entre `FeatureRange` y `DetailedRange`, formato de enlaces en encabezados, calibración de métricas y claridad de viñetas.

---

## Parte I — Preparación para Ejecución (`egm-implementation-readiness`)

* **Alcance auditado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L501-L720) (`DetailedRange`: líneas `501..720`)
* **Resultado:** **5 `🔴 Blocker`**, **7 `🟡 Major`**, **2 `🟢 Nit`**

### 🔴 Blockers (`egm-implementation-readiness`)

#### IR-B01 — `lock_windows.go` referencia símbolos inexistentes en la stdlib `syscall` de Windows
* **Alcance afectado:** Ítem 17 (`1_Intake/engine/lock_windows.go`, [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L626)) vs. Ítem 6 (`1_Intake/engine/go.mod`, línea `538`) y [0_Brain/engine/lock_windows.go](file:///C:/Black%20Factory/0_Brain/engine/lock_windows.go#L11).
* **Hallazgo:** La línea `626` instruye implementar `AcquireIntakeLock` y `(*FileLock).Unlock` *"using Windows `syscall.CreateFile` + `LockFileEx` (`LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY`) in `lock_windows.go` ... (matching `0_Brain/engine/lock_windows.go`)"*.
  * En la realidad del código (`0_Brain/engine/lock_windows.go:11,30,36,54`), `0_Brain` importa el paquete externo `golang.org/x/sys/windows` (declarado en `0_Brain/engine/go.mod:13`) y usa `os.OpenFile` + `windows.LockFileEx` / `windows.UnlockFileEx`.
  * El Ítem 6 (línea `538`) prohíbe cualquier `require` externo en `1_Intake/engine/go.mod`.
  * En la stdlib `syscall` de Go para Windows **no existen** `syscall.LockFileEx`, `syscall.UnlockFileEx`, `syscall.LOCKFILE_EXCLUSIVE_LOCK`, `syscall.LOCKFILE_FAIL_IMMEDIATELY` ni `syscall.ERROR_LOCK_VIOLATION`. Si el implementador sigue literalmente la línea `626`, `lock_windows.go` **no compilará**.
* **Corrección concreta:** Permitir `golang.org/x/sys/windows` en `1_Intake/engine/go.mod` (igual que `0_Brain/engine/go.mod:13`), **o** actualizar la línea `626` para especificar explícitamente el uso de stdlib pura mediante `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)`, `syscall.NewLazySystemDLL("kernel32.dll")` con `NewProc("LockFileEx")` y `NewProc("UnlockFileEx")`, constantes locales (`lockfileExclusiveLock = 0x00000002`, `lockfileFailImmediately = 0x00000001`, `errorLockViolation = syscall.Errno(33)`), y `syscall.Overlapped`.

#### IR-B02 — `TestMatchCoverPathParityWith0Brain` en `parity_test.go` no puede ejecutarse dinámicamente contra `0_Brain/engine/index.go` y contradice el case-sensitivity
* **Alcance afectado:** Ítem 19 (`1_Intake/engine/parity_test.go`, [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L638-L641)) vs. Ítem 13 (`1_Intake/engine/risk_zones.go`, línea `594`) y [0_Brain/engine/index.go](file:///C:/Black%20Factory/0_Brain/engine/index.go#L32-L93).
* **Hallazgo:**
  1. La línea `641` exige que `TestMatchCoverPathParityWith0Brain` ejecute vectores compartidos contra `MatchCoverPath` *"and compares against `0_Brain/engine/index.go`"*. Como `1_Intake/engine` (`blackfactory/intake`) no puede importar `blackfactory/brain` (por `modernc.org/sqlite`, línea `538`) y `go/parser`/`go/ast` solo inspecciona sintaxis pero no ejecuta funciones de otro módulo, **no está definido cómo se compara dinámicamente** contra `0_Brain/engine/index.go`.
  2. La línea `594` exige usar el algoritmo de `0_Brain/engine/index.go:32-93` *"with case-insensitive path matching for Windows compatibility"*. Sin embargo, `0_Brain/engine/index.go:54-93` (`NormalizeCoverPath`, `MatchCoverPath`, `matchPathSegments`) es **estrictamente case-sensitive**. Si `1_Intake` hace `strings.ToLower` y `0_Brain` no, los vectores con mayúsculas/minúsculas fallarán la paridad exacta.
* **Corrección concreta:**
  * Definir explícitamente en el Ítem 13 (línea `594`) que `MatchCoverPath` mantiene paridad case-sensitive exacta con `0_Brain/engine/index.go` y que `MatchCoverPathFold(pattern, filePath string) bool` envuelve `MatchCoverPath(strings.ToLower(pattern), strings.ToLower(filePath))` para la evaluación de zonas de riesgo en Windows.
  * Definir en el Ítem 19 (línea `641`) el mecanismo verificable sin importación cruzada (p. ej. extraer las funciones puras a un paquete compartido sin SQLite, o verificar por AST la estructura de `NormalizeCoverPath`/`MatchCoverPath`/`matchPathSegments` en `0_Brain/engine/index.go` junto con los vectores canónicos compartidos de `0_Brain/engine/index_test.go`).

#### IR-B03 — `input.go` omite detección de formato en `stdin`, comportamiento de flags CLI puros en Stage 1 y reglas de precedencia de `ParseMarkdownIntake`
* **Alcance afectado:** Ítem 9 (`1_Intake/engine/input.go`, [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L555-L560)) vs. Ítem 8 (`model.go`, línea `550`), Ítem 10 (`g4.go`, línea `566`) y `FeatureRange` (`130–149`, `413–415`).
* **Hallazgo:**
  1. `IngestSource.ReadRaw() ([]byte, string, error)` (línea `556`) no define el significado ni los valores válidos del segundo retorno `string` (¿formato `"json"` | `"md"`? ¿nombre de origen?).
  2. No se especifica cómo se detecta el formato JSON vs. Markdown+YAML al leer desde `--input -` (`stdin`), ni qué ocurre si el archivo `--input` no tiene extensión `.json`/`.md`.
  3. Cuando se invoca `intake.exe ingest` solo con flags CLI (sin `--input`), la línea `131` indica que Stage 1 escanea los bytes crudos de flags CLI antes de parsear, pero ni el Ítem 9 ni el Ítem 22 especifican cómo se serializan o escanean esos flags en `ScanTier1Raw` para emitir `field` y `line` en `REJECTED_G4`.
  4. En `ParseJSONIntake` (línea `557`), *"decoding string or array formats"* es ambiguo (no aclara si `affected_paths` / `impact_flags` aceptan tanto `[]string` como CSV `string` en JSON, ni si usa `DisallowUnknownFields()`).
  5. En `ParseMarkdownIntake` (línea `558`), falta definir: (a) qué pasa si `description` viene tanto en frontmatter como en el cuerpo Markdown, (b) qué pasa si el archivo `.md` carece de frontmatter `--- ... ---`, y (c) si `title` puede inferirse de `# Heading` cuando falta en frontmatter.
* **Corrección concreta:**
  * Documentar la firma `ReadRaw() (raw []byte, format string, err error)` con detección determinista (extensión `.json` vs `.md`/`.markdown`; para `stdin`, tras recortar UTF-8 BOM y espacios iniciales, primer byte `'{'` -> `"json"`, `"---"` -> `"md"`).
  * Definir el escaneo Stage 1 para flags CLI (escanear tanto el archivo/`stdin` crudo como cada valor de flag CLI pasando el nombre del flag como `field`).
  * Especificar en `ParseJSONIntake` el uso de `json.Decoder` con `DisallowUnknownFields()` y los tipos exactos permitidos por campo; y en `ParseMarkdownIntake`, la precedencia frontmatter vs cuerpo.

#### IR-B04 — `BrainClient` (`brain_client.go`) no especifica cómo combinar hasta 5 tokens en una sola llamada `query` sin reactivar el bug FTS5 `AND`, ni el mapeo completo de `BrainDocMatch`
* **Alcance afectado:** Ítem 12 (`1_Intake/engine/brain_client.go`, [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L583-L588)), Ítem 8 (`model.go`, línea `550`) y `FeatureRange` (`28–29`, `234–236`, `328`, `334–349`, `477`).
* **Hallazgo:**
  1. El Ítem 12 (líneas `586–587`) **no especifica cómo se unen los hasta 5 tokens** en el único argumento `--q` de `brain.exe query`. Si el implementador los une con espacios, reactiva el bug FTS5 `AND` de la línea `29`; si lanza 5 llamadas, viola el presupuesto de 2 subprocesos de la línea `477`.
  2. La línea `584` solo mapea a `ErrBrainUnavailable` (`exit 4`) la ausencia de `brain.db` o fallos de `brain.exe verify`. No especifica qué ocurre si `brain.exe` no existe en `--brain-bin`, si `Query`/`Context` agotan el timeout de `5s` o retornan código distinto de `0`.
  3. En el ejemplo JSON (líneas `334–349`), `brain_evidence.matched_docs` solo muestra `{id, rel_path, type, verified_head, transversal}`, pero Stage 4 y Stage 5 necesitan `covers_paths`, `matched_paths`, `status` y `project` del `SearchResult` de `0_Brain/engine/query.go:22-37`.
* **Corrección concreta:**
  * Especificar en el Ítem 12 (líneas `586–587`) que los tokens extraídos (máx. 5) se entrecomillan individualmente y se unen explícitamente con `" OR "` para una única invocación `brain.exe query --vault <vault> --project <p> --q "\"<t1>\" OR \"<t2>\"" --json`.
  * Especificar que cualquier error de ejecución, ausencia de binario o timeout en `Preflight`/`Query`/`Context` se mapea a `ErrBrainUnavailable` (exit code `4`).
  * Definir en el Ítem 8 todos los campos de `BrainDocMatch` (incluyendo `Status`, `Project`, `CoversPaths`, `MatchedPaths`).

#### IR-B05 — Reglas de triaje y promoción incompletas en `triage.go` (`NO_BRAIN_COVERAGE`, `INFERRED_PATHS_FAILSAFE`, `--clear-heuristic-escalation` y precedencia de plantillas)
* **Alcance afectado:** Ítem 14 (`1_Intake/engine/triage.go`, [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L603-L605)) vs. `FeatureRange` (`15`, `169–175`, `366–385`, `417–418`).
* **Hallazgo:**
  1. **Predicado de `NO_BRAIN_COVERAGE` (líneas `15, 376, 605`):** No especifica si exige documentos de `Context` con `len(MatchedPaths) > 0`, `Type` en `{"wiki", "adr"}` y `strings.EqualFold(Status, "active")` (o si verifica que cada ruta explícita en `affected_paths` esté cubierta por al menos un `wiki`/`adr` activo).
  2. **Agregación en `INFERRED_PATHS_FAILSAFE` (líneas `169–175, 374, 603–605`):** Cuando `affected_paths` se omite y `Query` devuelve documentos con `covers_paths` no vacío, falta especificar cómo se extraen/deduplican esas rutas y cómo se re-invoca `Context` + `EvaluateRiskZones` sobre ellas.
  3. **Contradicción de `--clear-heuristic-escalation` (líneas `417–418, 605`):** `intake.exe resolve` solo carga desde `runtime/quarantine/<WI-ID>.json` (`L418`), pero `HEURISTIC_LEXICAL_ESCALATION` guarda ítems `ready` en `runtime/workitems/`, haciendo el flag inalcanzable; además falta la regla exacta de validación de `--note`.
  4. **Precedencia de `suggested_template` (líneas `380–384`):** Cuando un `bug`/`audit_fix`/`refactor` dispara simultáneamente `NO_BRAIN_COVERAGE` y otra regla `ASDD` (`RISK_ZONE_CRITICO` o `EXPLICIT_IMPACT_FLAGS`), falta indicar si prevalece `"mapeo_estatico"` o `"correccion_hallazgos"`.
  5. **Rutas mixtas en `affected_paths` (líneas `368, 575, 603`):** Falta definir si ante una mezcla de ruta válida + ruta personal en `affected_paths` se descartan todas las rutas o solo la infractora al enviar a `quarantine/`.
* **Corrección concreta:** Especificar explícitamente en el Ítem 14 los 5 contratos anteriores.

---

### 🟡 Majors (`egm-implementation-readiness`)

* **IR-M01 (`model.go` y `workitem.schema.json`, líneas `520, 550`):** Omiten la lista exacta de campos JSON de `RawIntake`, `QuarantineInfo`, `IntakeEvent` (para eventos distintos de `REJECTED_G4`) y `VerifyReport`, así como la fórmula canónica exacta de `ComputeSanitizedSHA256(wi *WorkItem)`.
* **IR-M02 (`store.go` y `main.go`, líneas `611–615, 666–669`):** Dejan sin especificar la transición de archivo `WIQ-<old>.json` -> `WI-<new>.json` (y su colisión con un `WI-<new>.json` ya existente), la semántica positiva de `--update`, `--dry-run` y `--emit-md`, y la serialización `[]` no nula en `list --json`.
* **IR-M03 (`paths.go` y `main.go`, líneas `574–577, 668`):** `SanitizeAffectedPaths(rawPaths []string, repoRoot string)` recibe `repoRoot`, pero `ingest` y `resolve` no definen cómo descubren `repoRoot` ni exponen `--repo-root`, y falta especificar cómo clasifica `CountRootModules` los archivos en la raíz del repositorio (p. ej. `README.md`).
* **IR-M04 (`g4.go` y `parity_test.go`, líneas `565–568, 639`):** Si se modifica la regex de `unix_user_path` con grupos de captura para preservar delimitadores, se rompe `TestG4RulesParityWith0Brain` contra `0_Brain/engine/validator.go:67`; debe especificarse que el registro conserva el string literal exacto de `validator.go` y que `RedactTier2Text` reemplaza el núcleo en bucle de punto fijo sin consumir el delimitador compartido entre dos rutas adyacentes (`/home/a /home/b`).
* **IR-M05 (`brain_client.go`, línea `585`):** `IsProjectRegistered` no especifica filtrar por `status == "active"` en `query` ni por la columna 3 (`Estado == active`) al parsear `## Proyectos Registrados` en `0_Brain/vault/index.md` (`0_Brain/schemas/index.template.md:33-35`).
* **IR-M06 (`store.go`, línea `614` y `Verification Plan`, línea `711`):** `VerifyRuntime` no especifica inicializar `<dataDir>` si aún no existe antes de adquirir `.intake.lock`, cómo proceder cuando se ejecuta en un `t.TempDir()` que no es un repositorio Git (`git check-ignore`), ni cómo valida IDs `WIQ-*` cuyo `dedupe_key` está vacío.
* **IR-M07 (`intake_test.go`, línea `654`):** No aclara cómo se ejecuta el benchmark `< 500 ms` p95 sobre 60 documentos en `go test -count=1 ./...` sin depender de que `../../0_Brain/brain.exe` esté precompilado (o compilándolo / usando mock en tests unitarios).

### 🟢 Nits (`egm-implementation-readiness`)

* **IR-N01 (Líneas `711–712`):** `git status --ignored -s 1_Intake/` solo mostrará `!! 1_Intake/runtime/` si `intake.exe verify` crea el directorio `1_Intake/runtime/` al adquirir `.intake.lock`.
* **IR-N02 (Línea `681`):** En `C:\Black Factory\README.md`, `System Verification` abarca las líneas `39–50` (citado como `39–51`).

---

## Parte II — Auditoría Pedante de Detalles del Código (`egm-codebase-details`)

* **Alcance auditado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L501-L720) (`DetailedRange`: líneas `501..720`) contrastado contra `0_Brain/engine/*`, `0_Brain/schemas/*` y `GengiSWAP/*`.
* **Resultado:** **2 `🔴 Blocker`**, **5 `🟡 Major`**, **6 `🟢 Nit`**

### 🔴 Blockers (`egm-codebase-details`)

#### CD-B01 — API inexistente en Go stdlib `syscall` y contradicción directa con `0_Brain/engine/lock_windows.go`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L626) (Sección 17) y línea `538` (`go.mod`) vs [0_Brain/engine/lock_windows.go](file:///C:/Black%20Factory/0_Brain/engine/lock_windows.go#L11) (`L11, 30-37, 53-54`).
* **Descripción:** Convergence con `IR-B01` e `IC-1`. `0_Brain/engine/lock_windows.go` no usa `syscall.CreateFile` ni `syscall.LockFileEx`, sino `os.OpenFile` + `windows.LockFileEx` del módulo externo `golang.org/x/sys/windows`.

#### CD-B02 — Encabezados de tabla incompatibles con `0_Brain/schemas/risk_zones.template.md:35` en `ParseRiskZonesMarkdown`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L595) (Sección 13) vs [0_Brain/schemas/risk_zones.template.md](file:///C:/Black%20Factory/0_Brain/schemas/risk_zones.template.md#L33-L38) (y `1_intake.md:240-241`).
* **Descripción y justificación:** La línea `595` indica que `ParseRiskZonesMarkdown` *"validates the 6-column Markdown table header (`ID`, `Proyecto`, `Rutas cubiertas`, `Nivel`, `Compuerta obligatoria`, `Evidencia`)"* y divide patrones separados por coma (`comma-separated`). Sin embargo, `0_Brain/schemas/risk_zones.template.md:35` define los encabezados reales:
  `| ID | Proyecto | Rutas cubiertas (\`covers_paths\`) | Nivel | Compuerta obligatoria | Evidencia \`G5\` |`
  Si el parser exige coincidencia exacta con `Rutas cubiertas` y `Evidencia`, fallará contra la bóveda real (`0_Brain/vault/wiki/risk_zones.md`) y disparará permanentemente `RISK_ZONES_UNPARSEABLE_FAILSAFE`, forzando `ASDD` + `requires_autor_hitl: true` en cada ingesta. Además, la línea `595` omite el separador `;` documentado en la línea `241` y la remoción de backticks *por elemento* tras dividir la celda.
* **Corrección concreta:** Alinear la línea `595` con `0_Brain/schemas/risk_zones.template.md:35` y `1_intake.md:240-241`: validar los 6 encabezados tras normalizar backticks (aceptando `Rutas cubiertas (covers_paths)` / prefijo `Rutas cubiertas` y `Evidencia G5` / prefijo `Evidencia`), dividir la columna 3 por `,` y `;`, y recortar backticks individuales de cada patrón resultante.

### 🟡 Majors (`egm-codebase-details`)

* **CD-M01 (Líneas `594, 641` vs `0_Brain/engine/index.go:14, 32-93`):** Contradicción entre `MatchCoverPath` case-insensitive en `risk_zones.go` (`594`) y `TestMatchCoverPathParityWith0Brain` (`641`) contra `0_Brain/engine/index.go:54-93` (estrictamente case-sensitive).
* **CD-M02 (Línea `567` vs `0_Brain/engine/validator.go:67`):** En `validator.go:67`, `unix_user_path` es `(?:^|[\s"'(=:])(?:/home|/Users)/[A-Za-z0-9._-]+(?:/|$|\s|["'])`. La línea `567` omite el ancla de inicio `^` en el delimitador izquierdo (`([\s"'(=:])`), lo que fallaría cuando el string empieza en la columna 0 con `/home/<user>`.
* **CD-M03 (Líneas `583–588` vs `218–224, 314`):** `LoadRiskZones(vaultRoot string, contextDocs []BrainDocMatch) ([]RiskZoneRow, string, error)` está declarado en la interfaz `BrainClient` (`223`), pero fue omitido en la Sección 12 (`brain_client.go`, `583–588`), y `Preflight` (`584`) omite su tipo de retorno `(BrainPreflight, error)`.
* **CD-M04 (Línea `520` — `suggested_template` en `workitem.schema.json`):** Mientras `lane` permite `""` (`ASDD` | `FTD` | `""`) para ítems en cuarentena (`status: "needs_clarification"`), `suggested_template` en la línea `520` define un enum cerrado de solo 4 valores (`modulo_nuevo` | `correccion_hallazgos` | `auditoria_e2e` | `mapeo_estatico`) sin `""`. Todo ítem en `quarantine/` fallaría la validación JSON Schema y `intake.exe verify`.
* **CD-M05 (Líneas `668–669` vs `0_Brain/engine/cmd/brain/main.go:316, 479`):** La línea `669` no asigna código de salida a `ErrIntakeLocked` (contención de `1_Intake/runtime/.intake.lock`), y la línea `668` no incluye `--repo-root` ni descubrimiento de la raíz del repositorio para `SanitizeAffectedPaths` en `ingest`/`resolve`.

### 🟢 Nits (`egm-codebase-details`)

* **CD-N01 (Líneas `556, 559`):** Mover `MaxInputBytes = 1 << 20` y el rechazo de reparse points en `--input` del bullet de `MergeCLIOverrides` (que opera en memoria) al bullet de `ReadRaw()`.
* **CD-N02 (Línea `612` vs `0_Brain/engine/index.go:392-405`):** Declarar explícitamente `replaceFileAtomic(tmpPath, dstPath string) error` como helper interno de `store.go`.
* **CD-N03 (Línea `632` vs `0_Brain/engine/reparse_windows.go:16`):** Precisar que `IsReparsePointOrSymlink` siempre usa `os.Lstat(path)` y nunca un `os.FileInfo` proveniente de `os.Stat`.
* **CD-N04 (Líneas `654, 708`):** Separar tests unitarios (mock `BrainClient`) del benchmark de integración con `0_Brain/brain.exe`.
* **CD-N05 (Líneas `544, 657–660` vs `0_Brain/engine/README.md:15`):** Enlazar `./cmd/intake/README.md` desde `1_Intake/engine/README.md` (o incluir `1_Intake/engine/cmd/README.md`).
* **CD-N06 (Líneas `614, 711–714`):** Especificar `os.MkdirAll(dataDir, 0o755)` en `VerifyRuntime` antes de adquirir `.intake.lock`.

---

## Parte III — Auditoría de Estilo y Edición Técnica (`egm-style`)

* **Alcance auditado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L501-L720) (`DetailedRange`: líneas `501..720`)
* **Resultado:** **3 `🔴 Blocker`**, **9 `🟡 Major`**, **5 `🟢 Nit`**

### 🔴 Blockers (`egm-style`)

* **ST9-B01 (Líneas `583–588` vs `218–224` — Omisión de `LoadRiskZones` en `brain_client.go`):** Convergente con `CD-M03`. Añadir el bullet de `LoadRiskZones(vaultRoot string, contextDocs []BrainDocMatch) ([]RiskZoneRow, string, error)` en la Sección 12 (`brain_client.go`).
* **ST9-B02 (Líneas `575, 584, 595–596, 612, 650` — Contradicciones de nombres de símbolos y códigos entre `FeatureRange` y `DetailedRange`):**
  1. `PATH_TRAVERSAL_REJECTED` (`575`) no existe en `FeatureRange` (`147, 368`) ni se menciona en `650`; mapear explícitamente a `INVALID_AFFECTED_PATH_G4`.
  2. `ErrBrainIndexMissing` (`228`) vs `ErrBrainUnavailable` (`584`).
  3. `MEDIO`/`BAJO` en `ParseRiskZonesMarkdown` (`595`) no tienen mapeo definido en `EvaluateRiskZones` (`596`: `CRITICO > ALTO > NONE`) ni en `workitem.schema.json` (`520`: `NONE | ALTO | CRITICO`).
  4. `store.go` (`612`) omite `WIQ-<hash12>.json` en el lookup de `quarantine/`.
* **ST9-B03 (Líneas `700, 710–714` — Comando `git status` roto en `Verification Plan` por directorio de trabajo previo):** Los bloques previos (`688, 694, 700`) ejecutan `Set-Location "C:\Black Factory\1_Intake\engine"`, y el bloque final (`710–713`) ejecuta `git status --ignored -s 1_Intake/` sin volver a `Set-Location "C:\Black Factory"`. Ejecutado en secuencia, Git busca `1_Intake/engine/1_Intake/` y devuelve salida vacía. Añadir `Set-Location "C:\Black Factory"` al inicio del bloque de la línea `710`.

### 🟡 Majors (`egm-style`)

* **ST9-M01 (Líneas `505–678` — Enlaces Markdown inhabilitados por backticks en los 24 encabezados `####`):** Los 24 encabezados escriben ``#### N. `[ruta](../../ruta)` (Create)``, lo que desactiva el enlace Markdown y muestra la URL literal. Cambiar los 24 encabezados a ``#### N. [`ruta`](../../ruta) (Create)``.
* **ST9-M02 (Líneas `509, 606, 615, 642` — Calibración de hipérboles en `Rationale`):** Sustituir `"Mathematically eliminates"` (`642`), `"100% explainable"` (`606`) y `"Guarantees ... never tracked"` (`509`) por descripciones técnicas precisas de lo que verifican el código y los tests.
* **ST9-M03 (Líneas `705–707` — Calibración de criterios cuantitativos en `Verification Plan`):** Especificar cobertura `85%+` por paquete (`go test -v -count=1 -cover ./...`) y sustituir `"0% false negatives"` universal por `"0 missed detections across all test vectors in intake_test.go and main_test.go"`.
* **ST9-M04 (Líneas `520, 550, 575, 612, 626, 669` — Viñetas únicas sobrecargadas de 450–630 caracteres):** Desglosar en sub-viñetas legibles por símbolo/función.
* **ST9-M05 (Línea `567` vs `207` — Divergencia en clase de caracteres de delimitadores de `unix_user_path`):** Alinear `207` y `567` con la regex exacta de `0_Brain/engine/validator.go:67`.
* **ST9-M06 (Líneas `585, 605, 666`):** Alinear flags `--db <dbPath>` en `brain.exe query` entre `231` y `585`, y especificar la validación de `--note` cuando se pasa `--clear-heuristic-escalation`.
* **ST9-M07 (Líneas `566, 681`):** Enlazar `0_Brain/engine/validator.go:159-167` en la línea `566` y desglosar en tres sub-viñetas las modificaciones a `README.md` en la línea `681`.
* **ST9-M08 (Línea `586` vs `234, 236`):** Alinear la lista de patrones de `ExtractQueryTokens` (`*.json`, `*.sql`, `G[1-6]`, `maxTokens = 5`) con la línea `234`.
* **ST9-M09 (Líneas `538–539`):** Limpiar redacción redundante en `Rationale` de `go.mod`.

### 🟢 Nits (`egm-style`)

* **ST9-N01 (Líneas `515, 533, 560, 578, 587`):** Sustituir adjetivos/adverbios subjetivos (`clean port`, `clean quarantine`, `cleanly`, `guessing flags`) por lenguaje técnico directo.
* **ST9-N02 (Líneas `612, 615`):** Reemplazar `$O(1)$` por `` `O(1)` ``.
* **ST9-N03 (Línea `521`):** Incluir `3_Builder` en la lista de equipos consumidores de `workitem.schema.json`.
* **ST9-N04 (Líneas `702, 708`):** Añadir `-cover` al comando `go test` de la línea `702`.
* **ST9-N05 (Línea `514`):** Alinear los nombres de secciones citados de `0_Brain/README.md`.
