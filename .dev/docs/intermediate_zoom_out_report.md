# Revisión Etapa 8 (Intermediate Zoom Out Review): `Docs/designs/1_intake.md`

* **Documento auditado:** [`Docs/designs/1_intake.md`](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L1-L720) (líneas `1..720`)
* **Insumos contrastados:** [`zoom_out_report.md`](file:///C:/Black%20Factory/.dev/docs/zoom_out_report.md) (Etapa 5), [`comprehension_report.md`](file:///C:/Black%20Factory/.dev/docs/comprehension_report.md) (Etapa 6), [`critic_report.md`](file:///C:/Black%20Factory/.dev/docs/critic_report.md) (Etapa 7) y contratos base en `0_Brain/engine/`, `0_Brain/schemas/`, `README.md` y `.gitignore`.
* **Estándar aplicado:** `zoom-out-standard` (coherencia de alto nivel, causa raíz, simplicidad, YAGNI, reutilización y proporcionalidad).

---

## Veredicto Ejecutivo de Etapa 8

**No es recomendable pasar a la Etapa 9 (Implementation Readiness Test) aplicando parches locales dentro de `1_intake.md`.**

El peso acumulado de los hallazgos de Comprensión (Etapa 6) y Crítica (Etapa 7) demuestra que la arquitectura actual de `1_Intake` está sufriendo **inflamiento defensivo**: la mayoría de los `🔴 Blockers` y `🟡 Majors` de las Etapas 6 y 7 no nacen de descuidos aislados, sino de intentar sostener **tres restricciones artificiales** dentro de `1_Intake`:
1. Tratar a `0_Brain` como intocable incluso para sus propios defectos estructurales (`wiki/risk_zones.md` con `covers_paths: []`, ausencia de `projects/black-factory.md`, `MatchCoverPath` case-sensitive, `AND` implícito en FTS5) mientras se prohíbe cualquier dependencia en `1_Intake/engine/go.mod`.
2. Soportar duplicidad innecesaria en v1 (dos formatos de entrada JSON + parser YAML/Markdown custom, dos bitácoras `intake_log.md` + `intake_events.jsonl`, doble esquema de IDs `WI-` vs `WIQ-`, y exportación `--emit-md`).
3. Añadir clasificación especulativa (`HEURISTIC_LEXICAL_ESCALATION`, `INFERRED_PATHS_FAILSAFE`, `ExtractQueryTokens` NLP-lite) que obliga a construir válvulas de escape (`--clear-heuristic-escalation`, intersección glob-vs-glob).

**Parchear localmente los 20+ hallazgos de las Etapas 6 y 7 dentro de `1_Intake` aumentaría aún más la complejidad** (doble escaneo `G4`, bucles de punto fijo, double-snapshot de `mtime` en `Preflight`, walk de reparse points en ancestros, máquina de reclasificación sobre `workitems/`, índice o escaneo de `WIQ-` huérfanos, llamadas `unsafe` a `kernel32.dll` para esquivar `go.mod`). En contraste, **cinco simplificaciones estructurales** eliminan simultáneamente la mayoría de los hallazgos de las Etapas 5, 6 y 7 y reducen el diseño a un núcleo verificable mucho más pequeño.

---

## Hallazgos de Alto Nivel (Etapa 8)

### ZO8-1. 🔴 Blocker — La falsa frontera "cero cambios en `0_Brain` + cero `require` en `go.mod`" dispara una cascada de parches frágiles
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L11) (`L11, L26-L27, L37-L39, L47, L154-L175, L210-L243, L431-L453, L497, L538-L539, L580-L598, L623-L642`).
* **Convergencia Etapas 5–7:** `ZO-1`, `ZO-2`, `IC-1`, `IC-2`, `IC-5`, `D-B02`, `D-B03`, `D-M02`, `D-M03`, `D-M04`.
* **Análisis de simplicidad y deriva:**
  1. **Contradicción de compilación en `go.mod`:** `L538` prohíbe cualquier `require` externo en `1_Intake/engine/go.mod`, pero `L626` exige `syscall.CreateFile + LockFileEx`. Como se verificó en [`0_Brain/engine/lock_windows.go:11`](file:///C:/Black%20Factory/0_Brain/engine/lock_windows.go#L11) y [`0_Brain/engine/go.mod:13`](file:///C:/Black%20Factory/0_Brain/engine/go.mod#L13), la stdlib `syscall` en Windows no expone `LockFileEx`; por eso `0_Brain` importa `golang.org/x/sys/windows`. Mantener `go.mod` vacío obliga a escribir bindings `unsafe` contra `kernel32.dll`.
  2. **Duplicación de 6 componentes + deriva ya visible en `MatchCoverPath`:** En [`0_Brain/engine/validator.go`](file:///C:/Black%20Factory/0_Brain/engine/validator.go), [`frontmatter.go`](file:///C:/Black%20Factory/0_Brain/engine/frontmatter.go) y [`reparse_windows.go`](file:///C:/Black%20Factory/0_Brain/engine/reparse_windows.go) no existe ninguna importación de `modernc.org/sqlite`. La separación real es contra SQLite, no contra Go compartido. Copiar esos archivos y protegerlos con un `parity_test.go` basado en `go/parser` (`L211, L639`) ya falló en el propio documento: `L594` pide `MatchCoverPath` case-insensitive para Windows mientras [`0_Brain/engine/index.go:64`](file:///C:/Black%20Factory/0_Brain/engine/index.go#L64) es case-sensitive (`D-B02`), invalidando `TestMatchCoverPathParityWith0Brain` (`L641`).
  3. **Frontera rota con `0_Brain/vault/`:** Para esquivar defectos propios de `0_Brain` (tablas en `wiki/risk_zones.md` con `covers_paths: []`, ausencia de `projects/black-factory.md`), `1_Intake` lee archivos internos de la bóveda directo de disco (`brain.db`, `.brain-journal.json`, `index.md`, `wiki/risk_zones.md`) mientras hace llamadas a `brain.exe` sin `.brain.lock`, e incluso invoca `brain.exe verify` (`L229`) que muta la bóveda vía `RecoverJournal` ([`0_Brain/engine/cmd/brain/main.go:320`](file:///C:/Black%20Factory/0_Brain/engine/cmd/brain/main.go#L320)) violando `NG2`. Parchear esto localmente en `1_Intake` (dobles snapshots de `mtime`, walks de `os.Lstat` en ancestros, parser de celdas con tilde `CRÍTICO` y backticks individuales) multiplica el código para resolver un bug que también bloqueará a `6_Filtro_de_Riesgo`.
* **Simplificación estructural:**
  * Permitir `golang.org/x/sys` (y extraer un paquete puro Go compartido sin SQLite para `g4Rules`/`ScanG4`, `MatchCoverPath` con soporte case-insensitive configurable, `lock_*`, `reparse_*`) **o** consumirlos vía un subpaquete compartido en `go.work` sin `modernc.org/sqlite`, eliminando por completo `parity_test.go` y la duplicación de 6 módulos.
  * Corregir la causa raíz en `0_Brain` (un solo contrato limpio): poblar `projects/black-factory.md` e indexar/exponer `RZ-*` desde `0_Brain` (o un único subcomando read-only / snapshot consistente que no dispare `RecoverJournal`), eliminando los parsers de tablas Markdown en español dentro de `1_Intake`.

---

### ZO8-2. 🔴 Blocker — Parchear el bypass de `G4` y la asimetría `ingest`/`verify` sobre formatos y bitácoras duplicadas infla el perímetro
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L74) (`L74, L102, L108, L122-L123, L130-L148, L196-L208, L414, L417, L427, L461-L465, L472, L556-L569, L608-L622`).
* **Convergencia Etapas 5–7:** `ZO-5`, `ZO-6`, `IC-3`, `IC-7`, `Comp #6`, `D-B01`, `D-N01`.
* **Análisis de simplicidad y deriva:**
  1. **Bypass estructural y asimetría `G4`:** La Etapa 1 solo escanea bytes crudos (`L132`), por lo que secuencias escapadas en JSON (`\u0067hp_...`, `\/`) evaden Tier 1, se decodifican en Etapa 2 y se persisten en `workitems/` (`D-B01`). A la vez, la Etapa 2 solo redacta Tier 2 en `{title, description, note}` (`L142-L143`), dejando correos o rutas personales en `source_ref` o en eventos `REJECTED_G4` (`L74`), lo que hace fallar `intake.exe verify` (`ScanG4All`, `L427`) sobre los propios archivos válidos del runtime (`ZO-5`, `Comp #6`, `IC-3`).
  2. **Peso acumulado por superficie duplicada (YAGNI):** Si `1_Intake` mantiene en v1 un parser custom de Markdown+YAML frontmatter junto a JSON (`input.go`), una interfaz `IngestSource` para webhooks de Fase 2 fuera de alcance (`NG1`), dos bitácoras append-only (`intake_log.md` + `intake_events.jsonl`), y el flag `--emit-md`, cada endurecimiento `G4` (doble escaneo Tier 1 antes y después de decodificar, bucle de punto fijo para `unix_user_path`, sanitización en `source_ref` y `REJECTED_G4`, validación `ScanG4All` previa al `replaceFileAtomic`) debe implementarse y probarse sobre múltiples formatos y salidas redundantes.
* **Simplificación estructural:**
  * Aplicar un único invariante de memoria a disco: **ningún payload ni evento se escribe sin haber pasado `ScanG4All` sobre el buffer JSON final serializado** (y Tier 1 sobre bytes crudos + strings decodificados, con redacción/bloqueo Tier 2 uniforme sobre campos libres incluyendo `source_ref` en `REJECTED_G4`).
  * Recortar la superficie de v1 por YAGNI: entrada única en JSON + flags CLI (postergando el parser custom YAML+Markdown e `IngestSource` a Fase 2), bitácora única machine-queryable (`intake_events.jsonl`, eliminando `intake_log.md` de v1), y eliminar `--emit-md` dejando Markdown solo como salida de lectura en `intake.exe show --format md`.

---

### ZO8-3. 🔴 Blocker — Sobre-ingeniería en la máquina de estados y triaje especulativo (`HEURISTIC_LEXICAL_ESCALATION`, `WIQ-` vs `WI-`, `INFERRED_PATHS_FAILSAFE`, lock tardío)
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L121) (`L121, L134-L135, L161, L169-L175, L184-L188, L355-L358, L366-L378, L396-L408, L414-L418, L478, L603-L615`).
* **Convergencia Etapas 5–7:** `ZO-4`, `ZO-5`, `IC-6`, `Comp #1`, `Comp #3`, `Comp #4`, `Comp #5`, `D-B05`, `D-B06`, `D-M01`, `D-M03`, `ST-M01`.
* **Análisis de simplicidad y deriva:**
  1. **Heurística ruidosa + comando de escape inalcanzable:** `HEURISTIC_LEXICAL_ESCALATION` (`lexicon-v1.0`, `L377` vs `L604`) escala a `ASDD` palabras ubicuas del propio repositorio (`sqlite`, `lock`, `jwt`, `schema`, `risk_zones`), anulando `FTD`. Para compensarlo, el diseño creó `intake.exe resolve --clear-heuristic-escalation` (`L417`), que es **inoperante** porque `resolve` solo busca en `quarantine/` (`L418`) mientras la Regla 14 envía ítems a `workitems/`. Parchear esto con permisos de reclasificación en `workitems/`, validación de `[AUTOR]` en `--note` y definición cerrada de `lexicon-v1.0` añade complejidad para corregir una heurística innecesaria.
  2. **Bifurcación `WIQ-` vs `WI-`:** Crear un segundo prefijo `WIQ-<payloadSHA[:12]>` (`L358`) rompe la promesa de búsqueda `O(1)` (`L478`), deja archivos huérfanos en `quarantine/` al re-ingestar con `source_ref` completo (`ZO-5`, `IC-6`), deja indefinida la colisión al promover `WIQ-` hacia un `WI-` existente (`D-B05`), y obliga a especificar qué pasa cuando `quarantine/` recibe una re-ingesta modificada (`Comp #3`).
  3. **Inferencia frágil de rutas (`Caso A`):** Inferir `affected_paths` desde `covers_paths` de `brain.exe query` (`L169-L171`) obliga de todos modos a enrutar a `ASDD` por Regla 11 (`INFERRED_PATHS_FAILSAFE`, `L374`), pero introduce riesgo crítico si `covers_paths` es un glob (`GengiSWAP/**`) y `MatchCoverPath` no detecta intersección con `GengiSWAP/engine/` (`RZ-001` `CRITICO`, `D-M03`).
  4. **Lock adquirido demasiado tarde:** Adquirir `.intake.lock` solo en Etapa 5 (`L184`) deja desprotegidas las escrituras de Etapa 1 (`REJECTED_G4` -> `intake_events.jsonl`) y Etapas 3–4 (`quarantine/`) (`D-B06`).
* **Simplificación estructural:**
  * **Eliminar `HEURISTIC_LEXICAL_ESCALATION` y `--clear-heuristic-escalation` en v1.** El triaje queda 100% determinista usando señales estructuradas (`kind`, `impact_flags`, `risk_zones`, número de archivos/módulos y cobertura `0_Brain`).
  * **Eliminar `INFERRED_PATHS_FAILSAFE` (inferencia de rutas desde `query`):** Si `affected_paths` está vacío en entrada, pasa directo a `quarantine/` (`UNCERTAIN_SCOPE`, `Exit 2`) hasta que un operador suministre rutas concretas con `resolve` (para `FTD`, exigir rutas de archivo explícitas sin directorios ni globs, resolviendo `D-M01` de paso).
  * **Unificar identidad e idempotencia (`WI-` único):** Exigir `project + source + source_ref` para persistir cualquier `WorkItem` (si falta alguno de los tres, devolver error de entrada `Exit 1` o asignar un único ID determinista `WI-<dedupe_key[:12]>` con fallback sintético explícito sin prefijo distinto `WIQ-`), y definir una única regla de ciclo de vida en `Store` bajo `.intake.lock` adquirido antes de **cualquier** escritura en `runtime/` (Etapas 1, 3, 4 y 5), con reintento breve de lock (`D-M04`).

---

### ZO8-4. 🟡 Major — `ExtractQueryTokens` sigue fallando ante FTS5 `AND` y `CUJ 1` no es verificable contra la bóveda real
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L28-L29) (`L28-L29, L61-L65, L167-L175, L233-L236, L266-L350, L376, L477, L586`).
* **Convergencia Etapas 5–7:** `ZO-3`, `ZO-7`, `IC-4`, `Comp #1`, `D-B04` *(y aclaración sobre `ST-M07`)*.
* **Verificación en código base:**
  * Se verificó directamente en [`0_Brain/engine/cmd/brain/main.go:211`](file:///C:/Black%20Factory/0_Brain/engine/cmd/brain/main.go#L211) que `cmdQuery` **sí declara** el flag `-q` (`fs.StringVar(&qFlag, "q", "", "Consulta FTS5")`); por tanto, la observación `ST-M07` del reporte de Etapa 7 sobre inexistencia de `--q` es incorrecta y no requiere cambiar la firma CLI.
  * El defecto real confirmado en [`0_Brain/engine/query.go:412-452`](file:///C:/Black%20Factory/0_Brain/engine/query.go#L412-L452) es que `buildFTS5Query` concatena términos con `AND` implícito. `ExtractQueryTokens` (`L233-L236`) crea un extractor NLP-lite con listas bilingües de stop-words pero sigue enviando hasta 5 tokens sin `OR` explícito bajo un presupuesto de `<= 2` subprocesos (`L477`). Además, en `query.go:60`, si la expresión FTS5 tiene cualquier sintaxis inválida, el fallback `quoteAll=true` convierte `OR` en literal `"OR"`.
  * En paralelo, el ejemplo canónico de `WorkItem` (`L266-L350`) clasifica `0_Brain/engine/validator.go` como `FTD` con `matched_docs` conteniendo únicamente `constitution` y `risk_zones` (`L334-L349`), lo que viola la propia Regla 13 (`NO_BRAIN_COVERAGE`, `L376`), y `0_Brain/vault/` actualmente carece de documentos `wiki`/`adr` con `covers_paths` poblados (`ZO-7`, `Comp #1`).
* **Simplificación estructural:**
  * Si se elimina la inferencia de rutas desde `query` (`ZO8-3`), la consulta a `brain.exe query` se reduce a **complemento de cobertura** (o se resuelve en `0_Brain` mediante `--any` / unión `OR` nativa sobre `query.go`, o enviando tokens entrecomillados unidos con `OR` en 1 sola llamada cuando sea necesario).
  * Alinear `CUJ 1` y el ejemplo `WorkItem` con la Regla 13 (incluyendo un `wiki`/`adr` activo en el ejemplo y documentando su creación en `0_Brain/vault/` como prerequisito E2E).

---

### ZO8-5. 🟡 Major — Desequilibrio de altitud y contratos normativos incompletos en `FeatureRange` (`1..500`)
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L37-L42) (`L37-L42, L90-L124, L243, L351, L362-L384, L411-L428, L469-L498, L501-L683`).
* **Convergencia Etapas 5–7:** `ZO-6`, `ZO-7`, `Comp #2`, `Comp #5`, `Comp #6`, `D-M05`, `D-M06`, `S-M01..M03`, `S-N02`, `ST-B01`, `ST-M01..M06`.
* **Análisis de simplicidad y deriva:**
  * `FeatureRange` desciende a detalles de implementación (`fmt.Sprintf`, `reorderFlags`, `0x00000400`, `CountRootModules`, manipulación de delimitadores regex) mientras omite los contratos normativos esenciales para implementar y probar:
    * Fórmula canónica de `sanitized_payload_sha256` (`D-B05`; en `L297` aparece el hash de `""`).
    * Esquema de `quarantine_info` (`L351`), catálogo de `IntakeEvent` en `intake_events.jsonl`, tabla consolidada de códigos de salida `0..5` (`D-M06`, `Comp #6`).
    * Semántica positiva de `ingest --update` y `--dry-run` (`Comp #4`, `Comp #6`, `D-M06`).
    * Cuota `MaxInputBytes` (1 MiB), manejo de UTF-8 BOM en Windows y filtro obligatorio `status == "active"` en `0_Brain` (`D-M05`).
    * Mapeo de niveles `MEDIO` y `BAJO` de `risk_zones.md` en `WorkItem.risk_level` / `list --risk` (`Comp #5`, `ST-M03`).
    * Colisión de identificadores `G1–G6` (metas locales vs invariantes globales `README.md:23-29`: `Comp #2`, `S-M02`, `ST-B01`).
* **Simplificación estructural:** Elevar estos contratos mínimos a `FeatureRange`, renombrar las metas locales a `IG1–IG6`, y podar `DetailedRange` (actualmente 24 archivos) removiendo los detalles internos redundantes y los módulos descartados por YAGNI.

---

### ZO8-6. 🟢 Nit — Limpieza estructural de plantilla, jerarquía Markdown y consistencia de flags
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L55) (`L55, L147, L240, L398, L417, L467, L483, L485, L493`).
* **Convergencia Etapas 6–7:** `S-M01`, `S-M03`, `S-N01..N05`, `ST-M02`, `ST-N01..N06`, `Comp #6`.
* **Observación:** Promover `### Additional Resources` (`L485`) y `### Future Improvements` (`L493`) a `H2` (`##`), unificar `INVALID_AFFECTED_PATH_G4` (`L147`), arreglar backticks anidados (`L240`), retirar `--emit-md` (`IC-7`), alinear `--max-ftd-files` entre `ingest` y `resolve`, y definir el glosario breve de `Option i`, `Option 3C`, `OKF` y `[AUTOR]` en su primera aparición.

---

## Síntesis Arquitectónica Mínima para Etapa 9 (Implementation Readiness Test)

Antes de entrar a la Etapa 9, reestructurar [`Docs/designs/1_intake.md`](file:///C:/Black%20Factory/Docs/designs/1_intake.md) aplicando estas **5 decisiones de simplificación** resuelve de forma conjunta la mayoría de los hallazgos de las Etapas 5, 6 y 7 sin sobre-ingeniería:

1. **Compartir primitivas puras Go sin SQLite (y habilitar `golang.org/x/sys`):**
   * Usar `go.work` o un paquete compartido mínimo en el monorepo (sin `modernc.org/sqlite`) para `g4Rules`/`ScanG4`, `NormalizeCoverPath`/`MatchCoverPath` (con parámetro `caseInsensitive bool`), `lock_*` (`golang.org/x/sys/windows` en `go.mod`, igual que `0_Brain`) y `reparse_*` (validando cadena de ancestros).
   * **Impacto:** Elimina `parity_test.go`, elimina duplicación de 6 subsistemas, resuelve el fallo de compilación `LockFileEx` (`IC-1`), resuelve la deriva Windows case-insensitive (`D-B02`), y evita código `unsafe` (`ZO-2`, `IC-2`, `D-M02`).
2. **Resolver las dos brechas en `0_Brain` donde pertenecen (en lugar de parchearlas en `1_Intake`):**
   * Crear `projects/black-factory.md` en la bóveda y hacer que `0_Brain` (vía `context --json` o un endpoint/flag read-only único) exponga las zonas de riesgo de `wiki/risk_zones.md` y soporte búsqueda disyuntiva (`--any` / `OR`) con lectura atómica sin disparar `RecoverJournal`.
   * **Impacto:** Elimina el parser de tablas Markdown en español (`ZO-1`), elimina el parser de `index.md`, elimina la mutación oculta de `brain.exe verify` en `Preflight` (`D-B03`), elimina el split-brain `brain.db` vs disco (`IC-5`) y resuelve `ExtractQueryTokens` / `AND` implícito bajo presupuesto `<= 2` subprocesos (`ZO-3`, `IC-4`, `Comp #1`, `D-B04`, `D-M04`).
3. **Núcleo `G4` inmutable de memoria a disco y superficie v1 mínima (JSON + CLI, una sola bitácora):**
   * Recortar v1 a entrada **JSON + flags CLI** (retirando `IngestSource` especulativo y el parser custom YAML+Markdown hasta Fase 2), persistencia única en `intake_events.jsonl` (retirando `intake_log.md`) y Markdown únicamente bajo `show --format md` (retirando `--emit-md`).
   * Aplicar el invariante `G4` completo: Tier 1 en bytes crudos **y** strings decodificados; redacción Tier 2 en bucle de punto fijo sobre campos libres (incluido `source_ref`); y verificación `ScanG4All` sobre el JSON final serializado en memoria antes de escribir cualquier archivo o evento `REJECTED_G4`.
   * **Impacto:** Cierra todos los bypasses de escapes y asimetría `ingest`/`verify` (`D-B01`, `IC-3`, `ZO-5`, `D-N01`) con la mitad del código (`ZO-6`, `IC-7`).
4. **Triaje 100% determinista sin heurística léxica ni inferencia de rutas:**
   * Eliminar `HEURISTIC_LEXICAL_ESCALATION` (`lexicon-v1.0`), `--clear-heuristic-escalation`, e `INFERRED_PATHS_FAILSAFE` (si faltan `affected_paths` -> `quarantine/` `UNCERTAIN_SCOPE`; para `FTD`, exigir rutas de archivo explícitas sin globs ni directorios).
   * **Impacto:** Elimina falsos positivos en `FTD`, elimina la máquina de reclasificación rota (`ZO-4`, `Comp #4`, `D-B06`, `IC-6`), elimina la omisión de `CRITICO` en globs inferidos (`D-M03`, `Comp #1`) y el vacío de catálogo `lexicon-v1.0` (`Comp #5`).
5. **Unificar ID (`WI-<dedupe_key[:12]>`), ciclo de vida de `Store` y lock temprano:**
   * Adquirir `.intake.lock` (con breve backoff de reintento) alrededor de **toda** escritura en `runtime/` (Etapas 1, 3, 4 y 5), definir la fórmula canónica de `sanitized_payload_sha256`, retirar el prefijo `WIQ-` (exigiendo identidad mínima o usando un esquema único de hash sin archivos huérfanos), y consolidar en `FeatureRange` la tabla de exit codes `0..5`, `quarantine_info`, `IntakeEvent`, `MEDIO`/`BAJO` -> `risk_level`, `--update` y `--dry-run`, usando `IG1–IG6` para metas locales.
   * **Impacto:** Reduce el plan de **24 archivos** a un núcleo compacto (~12–14 archivos incluyendo docs/skills/tests) listo para superar limpiamente la Etapa 9 (Implementation Readiness Test).
