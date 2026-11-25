# Revisión Crítica Consolidada — Etapa 7 (`FeatureRange`: líneas `1..500` de `Docs/designs/1_intake.md`)

Este documento consolida la auditoría antagonista ejecutada en la **Etapa 7 (Critic Review)** sobre `FeatureRange` (`1..500`) de [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md) aplicando `feature-spec-standard` a través de tres lentes especializados:
1. **Crítico de Diseño Hostil (`egm-design`)**: Arquitectura, seguridad (`G4`, path traversal, reparse points), integración con `0_Brain`, máquina de estados y concurrencia en Windows.
2. **Arquitecto Estructural (`egm-structural`)**: Cumplimiento de plantilla `feature-spec-standard`, jerarquía de encabezados, simetría, diagramas y separación de altitud.
3. **Editor de Estilo Técnico (`egm-style`)**: Precisión terminológica, consistencia de identificadores/banderas, calibración de afirmaciones y claridad normativa.

---

## Parte I — Crítica de Diseño y Arquitectura (`egm-design`)

* **Alcance auditado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L1-L500) (`FeatureRange`: líneas `1..500`)
* **Resultado:** **6 `🔴 Blocker`**, **6 `🟡 Major`**, **2 `🟢 Nit`**

### 🔴 Blockers (`egm-design`)

#### D-B01 — Bypass de `G4` Tier 1 por secuencias de escape JSON/YAML y fuga de secretos/PII en campos fuera de `title`/`description`/`note`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L74) (líneas `74, 130-143, 198-208, 427, 472`); [validator.go](file:///C:/Black%20Factory/0_Brain/engine/validator.go#L49-L105).
* **Hallazgo y escenario de fallo:**
  1. La Etapa 1 ejecuta las 8 reglas Tier 1 de `G4` únicamente sobre los **bytes/líneas crudas** de entrada *antes* de parsear JSON o YAML (`1_intake.md:130-132`). Si un atacante o script envía un payload JSON con escapes Unicode o de barra (`"description": "\u0067hp_AbCdEf1234567890abcd"`, `"Bearer\u0020eyJ..."`, `"postgres:\/\/user:pass@db\/prod"` o `"-----BEGIN\u0020PRIVATE\u0020KEY-----"`), las regex de `validator.go:49-105` **no coinciden** sobre el texto crudo. En la Etapa 2 (`1_intake.md:140-143`), `encoding/json` decodifica esos escapes a secretos vivos en memoria, y la Etapa 2 **solo ejecuta Tier 2**, nunca re-evalúa Tier 1 sobre los strings ya decodificados. El secreto termina persistido en texto plano en `1_Intake/runtime/workitems/WI-<hash12>.json`.
  2. La Etapa 2 aplica redacción Tier 2 (`win_user_path`, `unix_user_path`, `email_address`) **exclusivamente** sobre `title`, `description` y `note` (`1_intake.md:142-143, 206-208`). Si `source_ref`, `source`, `project`, `kind` o `impact_flags` contienen un correo (`jdoe@example.com`) o una ruta personal (`C:\Users\jdoe\...`), pasan intactos al `WorkItem` JSON. Peor aún: en `REJECTED_G4` (`1_intake.md:74, 472`), `source` y `source_ref` se graban literales en `intake_events.jsonl` — si alguien pasa un token `ghp_...` o un correo dentro de `--source-ref`, `intake_events.jsonl` filtra el secreto/PII y luego `intake.exe verify` (`1_intake.md:427`) falla permanentemente al escanear todos los `.json` y `intake_events.jsonl` contra las 11 reglas `G4`.
* **Corrección accionable:**
  * Ejecutar las reglas Tier 1 `G4` **dos veces**: sobre la entrada cruda (Etapa 1, fail-closed inmediato) **y** sobre **todos** los campos string decodificados en memoria tras el parsing JSON/YAML (Etapa 2, antes de redactar o persistir).
  * En `REJECTED_G4`, aplicar escaneo/redacción `G4` sobre `source` y `source_ref` antes de escribir en `intake_events.jsonl` (o reemplazar cualquier campo que haga match con Tier 1/Tier 2 por `[REDACTED:G4:<rule>]`).
  * Validar o rechazar con Tier 2/cuarentena cualquier PII detectada en `source_ref`, `source`, `project` o `impact_flags` antes de persistir un `WorkItem`.

#### D-B02 — Bypass crítico de la zona de riesgo `RZ-001` (`CRITICO`) por comparación sensible a mayúsculas/minúsculas en Windows
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L241) (líneas `241, 370`); [index.go](file:///C:/Black%20Factory/0_Brain/engine/index.go#L54-L67); [risk_zones.template.md](file:///C:/Black%20Factory/0_Brain/schemas/risk_zones.template.md#L37-L38).
* **Hallazgo y escenario de fallo:**
  * `1_intake.md:241` exige evaluar `affected_paths` con `MatchCoverPath` usando el *«identical DP glob algorithm to `0_Brain/engine/index.go:54-93`»*.
  * En `0_Brain/engine/index.go:54-67`, `MatchCoverPath` es **estrictamente sensible a mayúsculas/minúsculas** (`file == pat || strings.HasPrefix(file, pat+"/")` y `path.Match`).
  * Sin embargo, `1_Intake` es *Windows-first* sobre NTFS (insensible a mayúsculas/minúsculas). Si una incidencia o stack trace declara `affected_paths: ["gengiswap/engine/ring.go"]` o `["GengiSWAP/Engine/ring.go"]`, `MatchCoverPath("GengiSWAP/engine/", "gengiswap/engine/ring.go")` devuelve **`false`**.
  * **Impacto:** Se evade por completo la regla `RISK_ZONE_CRITICO` (`RZ-001`, `requires_autor_hitl: true`) y la regla `RZ-002` (`ALTO`), permitiendo enrutar cambios en el núcleo crítico por `FTD` o degradando la etiqueta de riesgo.
* **Corrección accionable:**
  * Aplicar plegado de caso (`strings.ToLower`) tanto en el patrón de `wiki/risk_zones.md` como en `affected_paths` dentro de la evaluación de zonas de riesgo en Windows (y documentar la distinción entre `MatchCoverPath` exacto y el wrapper insensible a mayúsculas/minúsculas para que `parity_test.go` no contradiga la especificación).

#### D-B03 — Condición de carrera en `0_Brain` (Preflight sin `.brain.lock`, mutación oculta vía `brain.exe verify` y split-brain `brain.db` vs `wiki/risk_zones.md`)
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L47) (líneas `47, 154-156, 227-229, 238-241, 477`); [main.go](file:///C:/Black%20Factory/0_Brain/engine/cmd/brain/main.go#L199-L323); [index.go](file:///C:/Black%20Factory/0_Brain/engine/index.go#L152-L156).
* **Hallazgo y escenario de fallo:**
  1. **Violación de `NG2` (`1_intake.md:47`):** En `Preflight` (`1_intake.md:229`), si existe `.brain-journal.json`, `intake.exe` invoca `brain.exe verify --vault <vaultRoot> --json`. En `0_Brain/engine/cmd/brain/main.go:313-323`, `cmdVerify` **muta la bóveda** ejecutando `brain.RecoverJournal(vaultRoot)` (roll-forward o rollback de documentos y borrado del journal). Esto contradice directamente `NG2` (*«strictly a read-only consumer of `0_Brain`»*). Además, si cualquier documento no relacionado de la bóveda tiene un `BROKEN_LINK` o drift, `verify` devuelve `ok: false` y `intake.exe` aborta con `Exit 4` (`BRAIN_UNAVAILABLE`), aunque la bóveda y el índice sean funcionales.
  2. **TOCTOU entre `Preflight` y `query`/`context`/`LoadRiskZones`:** Ni `cmdQuery` ni `cmdContext` en `0_Brain/engine/cmd/brain/main.go:199-288` adquieren `.brain.lock`, y `intake.exe` lee `wiki/risk_zones.md` directamente desde disco (`1_intake.md:238-241`) sin lock alguno. Si `brain.exe apply` inicia justo después del `os.Stat(".brain-journal.json")` de `Preflight`, `intake.exe` puede leer `wiki/risk_zones.md` previo a la actualización mientras `brain.db` ya se reindexó (o viceversa), produciendo evidencia inconsistente (`brain_db_mtime` vs `risk_zones_sha256`).
* **Corrección accionable:**
  * Eliminar la invocación automática de `brain.exe verify` (mutante) desde `Preflight` de `ingest`/`resolve`: si `.brain-journal.json` existe o `.brain.lock` está bloqueado, **detenerse fail-closed (`Exit 4`)** e instruir al operador ejecutar `brain.exe verify` manualmente (preservando `NG2`).
  * Verificar `.brain-journal.json` y capturar `risk_zones.md` + `brain_db_mtime` bajo un snapshot consistente antes y después de consultar `brain.exe`, abortando con `Exit 4` si el `mtime`/hash cambió o el journal apareció durante la consulta.

#### D-B04 — Contradicción en `ExtractQueryTokens` (FTS5 `AND` implícito) y contradicción directa del ejemplo canónico de `WorkItem` con `NO_BRAIN_COVERAGE`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L29) (líneas `29, 61-65, 233-236, 266-350, 376, 477`); [query.go](file:///C:/Black%20Factory/0_Brain/engine/query.go#L412-L452).
* **Hallazgo y escenario de fallo:**
  1. **Regresión de FTS5 `AND` (`1_intake.md:29` vs `233-236`):** El documento diagnostica en `1_intake.md:29` que `brain.exe query` (`query.go:412-452`) une los términos con un `AND` implícito en FTS5 y devuelve `null` si falta una sola palabra. Sin embargo, `1_intake.md:236` especifica *"Selects up to 5 highest-specificity tokens and queries `brain.exe query`"* y `1_intake.md:477` restringe el presupuesto a *"at most 2 `0_Brain/brain.exe` subprocesses (`context --json` and `query --json`)"*. Con un único subproceso `query --json` y 3–5 tokens (`["validator.go", "scang4", "delimiter"]`, `1_intake.md:315-319`), si no se unen explícitamente con **`OR`**, `brain.exe query` ejecutará `AND` entre los 5 tokens y devolverá `null`, disparando falsos `UNCERTAIN_SCOPE`.
  2. **Contradicción en el ejemplo canónico de `WorkItem` (`1_intake.md:266-350` vs `376`):** La Regla 13 (`NO_BRAIN_COVERAGE`, `1_intake.md:376`) exige que `affected_paths` tenga al menos un documento coincidente de `type: "wiki"` o `type: "adr"` para evitar escalar a `ASDD`. Sin embargo, en el JSON de ejemplo (`1_intake.md:266-350`), el ítem tiene `lane: "FTD"` (`1_intake.md:271`) y `FTD_ELIGIBLE_BOUNDED_MAINTENANCE` (`1_intake.md:288`), pero `brain_evidence.matched_docs` (`1_intake.md:334-349`) **solo contiene `constitution` y `risk_zones`** (ningún `wiki` ni `adr`). Cualquier implementación fiel a la Regla 13 clasificaría ese mismo payload como `ASDD` por `NO_BRAIN_COVERAGE`, nunca como `FTD`.
* **Corrección accionable:**
  * Especificar explícitamente en `ExtractQueryTokens` que los tokens seleccionados (máximo 5, escapando/eliminando `AND`, `OR`, `NOT`, `NEAR`, `*`, `"`) se envuelven en comillas dobles válidas y se unen con el operador explícito `OR` (ej. `"validator.go" OR "scang4" OR "delimiter"`), evitando que el fallback `quoteAll=true` de `query.go:60` convierta `OR` en literal `"OR"`.
  * Corregir el ejemplo de `WorkItem` (`1_intake.md:328, 334-349`) mostrando la consulta disyuntiva real e incluyendo un documento `type: "wiki"` o `"adr"` activo que cubra `0_Brain/engine/validator.go`.

#### D-B05 — Fórmula indefinida de `sanitized_payload_sha256` e inconsistencias en máquina de estados (`quarantine/` `WI-` vs `WIQ-`, `re-ingest` y `resolve`)
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L121) (líneas `121, 148, 185-188, 297, 355-358, 403-408, 417-418`).
* **Hallazgo y escenario de fallo:**
  1. **Hash indefinido:** `sanitized_payload_sha256` determina si un re-ingreso idéntico devuelve `Exit 0` o conflicto `Exit 5` (`1_intake.md:185-186`), y también define el ID provisional `WIQ-<sanitized_payload_sha256[:12]>` (`1_intake.md:358`). Sin embargo, `FeatureRange` (`1..500`) **no define qué campos ni qué serialización canónica** alimentan ese SHA-256 (y el ejemplo en `1_intake.md:297` muestra el SHA-256 de la cadena vacía `""`). Si se calcula sobre bytes crudos, dos ingestiones equivalentes vía `--input a.json` vs flags CLI diferirán y dispararán `Exit 5`.
  2. **Transiciones rotas en `quarantine/` y `resolve`:**
     * Si un ítem está en `quarantine/WI-<hash12>.json` y se re-ingesta igual o modificado (`1_intake.md:404`), Etapa 5 (`1_intake.md:185-186`) solo describe colisiones contra `workitems/`, sin definir si la re-ingesta que vuelve a quedar en cuarentena retorna `Exit 2` actualizando el archivo en `quarantine/` (`revision++` o idempotente si el hash no cambió).
     * Si un ítem en cuarentena tiene ID provisional `WIQ-<sha[:12]>` y el operador ejecuta `intake.exe resolve --id WIQ-<sha[:12]> --source-ref BUG-1` (`1_intake.md:358, 418`), su ID promovido pasa a `WI-<dedupe[:12]>`. No se especifica qué ocurre si `workitems/WI-<dedupe[:12]>.json` ya existe.
* **Corrección accionable:**
  * Definir la fórmula canónica exacta de `sanitized_payload_sha256` (JSON compacto con claves ordenadas de `{project, title, description, note, kind, source, source_ref, affected_paths, impact_flags}` ya normalizados y redactados por Tier 2).
  * Especificar explícitamente en el autómata: (a) re-ingesta sobre `quarantine/` con igual/distinto hash, y (b) colisión de `WIQ-` -> `WI-` en `resolve` contra un `workitems/WI-<dedupe[:12]>.json` preexistente (`Exit 5` sin borrar el archivo en cuarentena a menos que se pase `--update`).

#### D-B06 — Condición de carrera en `1_Intake/runtime/` por adquirir `.intake.lock` solo en Etapa 5 y mecanismo roto en `--clear-heuristic-escalation`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L134-L135) (líneas `134-135, 161, 171, 184, 377, 417-418`).
* **Hallazgo y escenario de fallo:**
  1. **Escrituras sin lock en Etapas 1, 3 y 4:** Según el diagrama (`1_intake.md:184`), `.intake.lock` se adquiere recién en Etapa 5. Sin embargo, Etapa 1 escribe en `intake_events.jsonl` ante `REJECTED_G4` (`1_intake.md:135`), Etapa 3 escribe en `runtime/quarantine/` (`1_intake.md:161`) y Etapa 4 escribe en `runtime/quarantine/` (`1_intake.md:171`) y en los logs append-only. Si dos procesos concurrentes fallan en Etapa 1 o entran en cuarentena simultáneamente, escribirán sin exclusión mutua.
  2. **`--clear-heuristic-escalation` inoperante (`1_intake.md:417-418`):** Los ítems escalados por `HEURISTIC_LEXICAL_ESCALATION` (Regla 14, `1_intake.md:377`) quedan en **`runtime/workitems/`** (`status: "ready"`, `lane: "ASDD"`), **no** en `runtime/quarantine/`. Pero `1_intake.md:418` especifica que `resolve` *"Loads the quarantined JSON from `runtime/quarantine/<WI-ID>.json`"*, por lo que `resolve --clear-heuristic-escalation` fallará por archivo no encontrado.
* **Corrección accionable:**
  * Adquirir `.intake.lock` alrededor de **toda** escritura a `1_Intake/runtime/` (`intake_events.jsonl`, `intake_log.md`, `quarantine/`, `workitems/`), incluyendo Etapa 1 (`REJECTED_G4`) y Etapas 3–4 (`quarantine/`).
  * Permitir que `resolve` (o el flujo de reclasificación) cargue ítems desde `workitems/` cuando se invoque `--clear-heuristic-escalation` con nota de `[AUTOR]` (o retirar la bandera si se elimina la heurística léxica).

---

### 🟡 Majors (`egm-design`)

#### D-M01 — Brechas de path traversal y evasión de umbrales en `affected_paths`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L144-L147) (líneas `79-82, 144-147, 368, 375`); [index.go](file:///C:/Black%20Factory/0_Brain/engine/index.go#L32-L50).
* **Hallazgo:** `1_intake.md:144-145` no bloquea explícitamente rutas UNC (`\\servidor\recurso`), rutas relativas de unidad Windows (`C:archivo.go`), prefijos NT (`\\?\`), rutas que inician con `/` (`/etc/passwd`) o nombres reservados de Windows (`CON`, `NUL`, `COM1..9`), ni aclara qué ocurre si `affected_paths` pasa un directorio (`["0_Brain/engine/"]`) para evadir el tope de `<= 5` archivos en `FTD`, cómo se cuentan archivos en la raíz del repo (`README.md`) en `MULTI_MODULE_OR_FILE_THRESHOLD`, o si `resolve --path` reemplaza o añade rutas.
* **Corrección accionable:** Validar cada segmento **antes** de `path.Clean`; exigir rutas de archivo concretas (sin directorios ni globs) para ser elegible a `FTD`; definir cómo cuentan archivos raíz y especificar que `resolve --path`/`--paths` reemplaza `affected_paths`.

#### D-M02 — Protección incompleta contra reparse points/junctions en directorios ancestros
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L228-L229) (líneas `228-229, 474`); [reparse_windows.go](file:///C:/Black%20Factory/0_Brain/engine/reparse_windows.go#L16-L29).
* **Hallazgo:** Verificar `os.Lstat` solo en el archivo hoja (`risk_zones.md`, `index.md`, `--input`) no detecta si un directorio intermedio (`0_Brain/vault/wiki/` o `1_Intake/runtime/workitems/`) es un junction NTFS (`mklink /J`), y `Preflight` usa `os.Stat` en lugar de `os.Lstat`.
* **Corrección accionable:** Verificar ausencia de reparse points en toda la cadena de directorios ancestros desde la raíz del workspace/vault/runtime y usar `os.Lstat` en `Preflight`.

#### D-M03 — Fragilidad del parser de `wiki/risk_zones.md` y pérdida de `CRITICO` en rutas inferidas con globs (Etapa 4 Caso A)
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L169-L175) (líneas `169-175, 238-243, 374`); [risk_zones.template.md](file:///C:/Black%20Factory/0_Brain/schemas/risk_zones.template.md#L35-L38).
* **Hallazgo:** (1) En la columna 3 de `risk_zones.md`, quitar backticks antes de dividir por `,`/`;` falla si hay múltiples patrones con backticks individuales (`` `GengiSWAP/engine/`, `0_Brain/engine/` ``) o si `Nivel` lleva tilde (`CRÍTICO`). (2) En Caso A (`paths_confidence: "inferred_from_query"`), si `covers_paths` inferido es un glob (`GengiSWAP/**`), `MatchCoverPath("GengiSWAP/engine/", "GengiSWAP/**")` devuelve `false` porque trata el segundo argumento como ruta literal, perdiendo el match `CRITICO` (`RZ-001`) y `requires_autor_hitl: true`.
* **Corrección accionable:** Quitar backticks *por patrón* tras dividir por `,`/`;`, normalizar `CRÍTICO` -> `CRITICO`, y evaluar intersección de prefijos/globs cuando `paths_confidence == "inferred_from_query"` contenga comodines.

#### D-M04 — Presupuesto real de subprocesos (hasta 4 vs `<= 2`), latencia `< 500 ms` p95 y contención inmediata en `.intake.lock`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L41) (líneas `41, 229-236, 476-480`).
* **Hallazgo:** `1_intake.md:477` promete máximo 2 subprocesos `brain.exe`, pero `Preflight` (`verify`) + `IsProjectRegistered` (`query --type project`) + `Context` + `Query` suman hasta 4 spawns; además, `.intake.lock` usa `LOCKFILE_FAIL_IMMEDIATELY` sin reintento/backoff ante ráfagas concurrentes cortas.
* **Corrección accionable:** Resolver `IsProjectRegistered` leyendo `index.md` / `projects/<project>.md` (o combinando con `query`) para respetar el tope estricto de 2 subprocesos, y agregar un bucle de reintento acotado (p. ej. 250–500 ms con backoff de 20 ms) antes de abortar por `.intake.lock` ocupado.

#### D-M05 — Límites de entrada en `FeatureRange`, manejo de UTF-8 BOM en Windows, gramática YAML+Markdown y filtro `status == "active"` en `0_Brain`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L62) (líneas `62, 130-131, 141, 232, 376, 378`); [frontmatter.go](file:///C:/Black%20Factory/0_Brain/engine/frontmatter.go#L140-L210); [index.template.md](file:///C:/Black%20Factory/0_Brain/schemas/index.template.md#L33-L35).
* **Hallazgo:** `FeatureRange` no declara la cuota `MaxInputBytes` (1 MiB, que solo aparece en `DetailedRange` línea `559`), no especifica recorte de UTF-8 BOM (`\xEF\xBB\xBF`) antes de parsear JSON/Markdown en Windows, y no exige explícitamente que los proyectos en `index.md` (`Estado`) y los documentos `wiki`/`adr` en `0_Brain` tengan `status == "active"` (y no `draft` o `deprecated`) para habilitar `FTD`.
* **Corrección accionable:** Elevar a `FeatureRange` la cuota de 1 MiB, el recorte de UTF-8 BOM y el filtro obligatorio `status == "active"` para proyectos y cobertura `wiki`/`adr`.

#### D-M06 — Tabla completa de códigos de salida (`0..5`), esquemas JSON de `quarantine_info` / `intake_events.jsonl` y semántica de `--dry-run`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L115-L116) (líneas `115-116, 351, 414, 475`).
* **Hallazgo:** En `FeatureRange` falta una tabla consolidada de los 6 códigos de salida (`0..5`, incluyendo qué condiciones producen `Exit 1` y qué código devuelve un lock ocupado), el esquema JSON no nulo de `quarantine_info`, los tipos de eventos de `intake_events.jsonl` distintos de `REJECTED_G4`/`PROMOTED`, y la garantía de cero escrituras bajo `--dry-run`.
* **Corrección accionable:** Documentar en la Sección 4/6 el esquema de `quarantine_info`, el catálogo de eventos de `intake_events.jsonl`, la tabla completa de códigos de salida `0..5` y la semántica de `--dry-run`.

---

### 🟢 Nits (`egm-design`)

#### D-N01 — Limitación heredada de `secret_assignment` sobre sintaxis JSON con comillas y delimitadores de `unix_user_path`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L205-L207) (líneas `205-207, 211`); [validator.go](file:///C:/Black%20Factory/0_Brain/engine/validator.go#L67).
* **Recomendación:** Documentar que `unix_user_path` se redacta en bucle hasta punto fijo para cubrir rutas adyacentes separadas por un único espacio (`/home/a /home/b`) y que el escaneo en Etapa 2 sobre strings JSON decodificados cubre asignaciones dentro de textos embebidos.

#### D-N02 — Manejo de colisión de prefijo de 12 hex chars en `WI-<hash12>` y línea final truncada en `intake_events.jsonl`
* **Alcance afectado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L357) (líneas `357, 427, 478`).
* **Recomendación:** Al hacer lookup `O(1)` de `WI-<hash12>.json`, verificar siempre que el campo interno `dedupe_key` (64 hex chars) coincida exactamente; si difiere (colisión de 48 bits), devolver error explícito.

---

## Parte II — Revisión Estructural y de Plantilla (`egm-structural`)

* **Alcance auditado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L1-L500) (`FeatureRange`: líneas `1..500`)
* **Resultado:** **0 `🔴 Blocker`**, **3 `🟡 Major`**, **5 `🟢 Nit`**

### 🟡 Majors (`egm-structural`)

#### S-M01 — Jerarquía incorrecta (`H3` en lugar de `H2`) en `Additional Resources` y `Future Improvements`
* **Alcance afectado:** `### Additional Resources` ([1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L485)) y `### Future Improvements` ([1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L493)).
* **Discrepancia:** Ambas secciones están declaradas como `H3` (`###`) debajo de `## Non-Functional Requirements` (línea `469`). En `feature-spec-standard`, son secciones de primer nivel (`H2` / `##`).
* **Corrección accionable:** Promover las líneas `485` y `493` a `## Additional Resources` y `## Future Improvements`.

#### S-M02 — Colisión de identificadores estructurales `G1–G6` en `## Goals` con las reglas globales `G1–G6`
* **Alcance afectado:** `## Goals` ([1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L37-L42)).
* **Discrepancia:** Los objetivos locales usan `G1`–`G6`, colisionando con las invariantes globales `G1`–`G6` de Black Factory (`README.md:23-29`), mientras `## Non-Goals` usa `NG1`–`NG6`.
* **Corrección accionable:** Renombrar los objetivos de `## Goals` a `IG1`–`IG6` (o `GOAL-1`–`GOAL-6`), reservando `G1`–`G6` para las invariantes globales.

#### S-M03 — Asimetría entre el árbol de topología de `FeatureRange` (`90..124`) y el inventario de `DetailedRange` (`541`, `657`)
* **Alcance afectado:** `### 1. System Topology and Five-Stage Execution Pipeline` ([1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L99-L116)).
* **Discrepancia:** El árbol de `1_Intake\` enumera cada archivo de `engine\`, pero omite `1_Intake/engine/README.md` (ítem 7 de `DetailedRange`, línea `541`) y `1_Intake/engine/cmd/intake/README.md` (ítem 21 de `DetailedRange`, línea `657`).
* **Corrección accionable:** Añadir `README.md` bajo `engine\` y bajo `cmd\intake\` en el árbol de las líneas `99..116`.

### 🟢 Nits (`egm-structural`)

* **S-N01 (Línea `240` — Backticks anidados rotos):** `` `| ID | Proyecto | Rutas cubiertas (`covers_paths`) | Nivel | Compuerta obligatoria | Evidencia `G5` |` `` rompe el código inline por usar comillas invertidas simples anidadas. Reemplazar los delimitadores externos por dobles comillas invertidas (`` ``| ID | ... |`` ``).
* **S-N02 (Líneas `110..116, 207, 357, 411, 474, 480` — Fugas de altitud):** Expresiones literales de Go (`fmt.Sprintf`), nombres de helpers (`reorderFlags`) y constantes hexadecimales Win32 (`0x00000400`) en `FeatureRange` anticipan detalles propios de `DetailedRange`.
* **S-N03 (Líneas `110..111, 228..243, 381..384, 414..427, 490` — Consistencia de listas y rutas):** Unificar viñetas (`*` vs `-`), separar `lock_windows.go` / `lock_other.go` y `reparse_windows.go` / `reparse_other.go` en ramas propias del árbol, y simplificar `../../Docs/BLACK_FACTORY_BRAIN.md` a `../BLACK_FACTORY_BRAIN.md`.
* **S-N04 (Líneas `7, 467, 483, 499` — Separadores horizontales asimétricos):** Mantener `---` solo tras el bloque de metadatos (`7`) y en la frontera `FeatureRange` / `DetailedRange` (`499`), eliminando los de las líneas `467` y `483`.
* **S-N05 (Líneas `398, 403` — Truncamiento en caja ASCII `QUARANTINED`):** Ensanchar la caja de la línea `398` para mostrar `status: needs_clarification` sin truncar (`needs_clarif`) y mostrar `intake.exe resolve --id WI[Q]-...` en la línea `403`.

---

## Parte III — Auditoría de Estilo y Redacción Técnica (`egm-style`)

* **Alcance auditado:** [1_intake.md](file:///C:/Black%20Factory/Docs/designs/1_intake.md#L1-L500) (`FeatureRange`: líneas `1..500`)
* **Resultado:** **1 `🔴 Blocker`**, **7 `🟡 Major`**, **6 `🟢 Nit`**

### 🔴 Blocker (`egm-style`)

#### ST-B01 — Colisión terminológica entre objetivos locales (`G1–G6`) e invariantes globales (`G1–G6`)
* **Alcance afectado:** Líneas `35–42` frente a líneas `11, 16, 22, 31, 37, 38, 42, 434, 471, 487` (convergente con `S-M02` y `Comprehension #2`).
* **Reemplazo recomendado:** Renombrar los objetivos locales en `## Goals` (`37–42`) a `IG1`–`IG6` y definir brevemente en su primera mención cada invariante global (`G1` evidencia literal de ejecución, `G3` pruebas aisladas en directorios temporales sin mutar producción, `G4` cero fugas de secretos/rutas personales, `G5` rigor técnico/tests, `G6` parada segura ante incertidumbre).

### 🟡 Majors (`egm-style`)

* **ST-M01 (Líneas `40, 50, 79, 81, 121, 161, 247, 357–358, 403, 461, 464, 478` — Nomenclatura `WI-` vs `WIQ-`):** Unificar la notación a `WI-<hash12>` (o `WIQ-<hash12>` cuando faltan campos de identidad en cuarentena) y eliminar placeholders informales como `WI-xxx.md` (`461, 464`).
* **ST-M02 (Línea `147` vs `81, 368` — Literal `invalid_path_reason: "G4_personal_path_in_affected_paths"`):** El diagrama de Etapa 2 usa un nombre de campo y código que no existen en la matriz de triaje (`INVALID_AFFECTED_PATH_G4` en `quarantine_info.reason_codes`). Unificar a `INVALID_AFFECTED_PATH_G4`.
* **ST-M03 (Líneas `243, 378, 420` — Dominio de valores de `risk_level` y `--risk`):** El parser de `wiki/risk_zones.md` valida `CRITICO`, `ALTO`, `MEDIO`, `BAJO`, pero `list --risk` (`420`) y la Regla 15 (`378`) solo enumeran `NONE`, `ALTO`, `CRITICO`. Alinear el enum o especificar explícitamente cómo se mapean `MEDIO` y `BAJO`.
* **ST-M04 (Líneas `38, 39, 40, 435` — Calibración de `"100% compile-time/test-time equivalence"`):** En la línea `435`, `parity_test.go` se ejecuta durante `go test` (tiempo de prueba, no de compilación). Sustituir `"100% compile-time/test-time equivalence"` por `"test-time equivalence during go test"`.
* **ST-M05 (Líneas `11, 13, 39, 55, 88` — Jerga sin glosario en primera aparición):** Explicar brevemente `Option i`, `Option 3C`, `[AUTOR]` y `HITL` en su primera aparición y precisar `"Windows-first"` como `"Windows primary target with POSIX fallback"`.
* **ST-M06 (Líneas `11, 33, 38, 211, 427, 447` — Oraciones sobrecargadas de 70–100+ palabras):** Dividir las oraciones largas en frases cortas y desglosar la línea `427` (`intake.exe verify` incisos `(a)–(e)`) en una lista de sub-viñetas verificables.
* **ST-M07 (Líneas `157, 231, 328` vs `0_Brain/engine/cmd/brain/main.go:199` — Firma real de `brain.exe query`):** La línea `328` invoca `brain.exe query --vault 0_Brain/vault --project black-factory --q validator.go --json`, pero en `0_Brain/engine/cmd/brain/main.go:199-243` `cmdQuery` **no tiene ningún flag `--q`** (`fs.Parse` fallaría con `flag provided but not defined: -q` y código de salida `1`); `brain.exe query` recibe el texto de búsqueda como **argumentos posicionales**. Corregir todas las invocaciones de `brain.exe query` en el documento para usar argumentos posicionales en lugar de `--q`.

### 🟢 Nits (`egm-style`)

* **ST-N01 (Línea `55`):** Eliminar la meta-referencia interna `"and the seven-lens Goldfish review"` en `## Open Questions`.
* **ST-N02 (Líneas `40, 403`):** Escribir `intake.exe ingest` e `intake.exe resolve` (con `.exe`) de forma consistente.
* **ST-N03 (Líneas `20, 440, 497`):** En la línea `20`, cambiar `"five operational failure modes"` por `"five operational and integration gaps"`.
* **ST-N04 (Líneas `475, 481`):** Sustituir `"fails closed if untracked privacy is broken"` y `"fail safely"` por el comportamiento concreto (código de salida distinto de cero y cero mutaciones en disco).
* **ST-N05 (Líneas `14, 15, 61, 116, 376, 378, 478`):** Reemplazar `$O(1)$` por `` `O(1)` ``, `t.TempDir` por `` `t.TempDir()` `` y unificar el orden `adr`/`wiki`.
* **ST-N06 (Línea `366`):** Enumerar explícitamente los 5 valores válidos de `kind` (`bug`, `feature`, `refactor`, `audit_fix`, `ops`) en la Regla 3 de la matriz de decisión.
