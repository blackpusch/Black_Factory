# Proyecto Tungsteno — Documentación Técnica de `GengiSWAP`

> **Última actualización verificada:** `2026-11-19` (inicialización Git en `main` apuntando a `https://github.com/blackpusch/Proyecto_Tungsteno`, incorporación de `README.md`, mejoras en `bridge.go` —`warnIfNoGit`, `printPlanSummary` / `collectAbsoluteScopePaths`, `autoVerifyQANote` y recordatorio G5 en `DecideChunk`—, recompilación local de `gengis.exe` y verificación de métricas/SHA256 contra disco)  
> **Alcance del documento:** Documentación técnica del repositorio **Proyecto Tungsteno** (`https://github.com/blackpusch/Proyecto_Tungsteno`, raíz local `C:\Desarrollo\Proyecto Tungsteno`) y de su marco de orquestación **GengiSWAP** (`GengiSWAP\engine\` en Go 1.24, protocolos v2 unificados en `gengis_swap_common.md` y separación de tareas/respaldos en `C:\Desarrollo\GengiSWAP_data\`). No incluye inventarios ni código de repositorios externos gestionados como destino de tareas.

---

## 1. Visión General y Principio de Diseño

**Proyecto Tungsteno** aloja el núcleo versionable de **GengiSWAP** (`GengiSWAP\`), un sistema de ejecución controlada por anillos para planificar, ejecutar y auditar tareas sobre proyectos externos configurables por tarea (mediante `## Directorio de Destino Real` en cada `index.md`).

El principio rector definido en [ring.go](../GengiSWAP/engine/ring.go#L4-L8) establece que **las reglas las hace cumplir el motor y no la buena voluntad de ningún modelo**:
- El binario CLI `gengis.exe` (compilable desde `GengiSWAP\engine\`) ejecuta directamente los comandos de verificación y captura su salida literal.
- Evalúa programáticamente el alcance de archivos tocados contra `## Alcance`.
- En repositorios Git destino, realiza un commit selectivo por entrega y revierte sin destruir historial (`git revert --no-edit` y `git stash`, jamás `git reset --hard` ni `git clean`).
- Impone el límite de reintentos por chunk (`Máx. intentos por chunk`) y persiste el estado atómicamente en `.ring\state.json` tras cada transición para permitir pausas y reanudaciones.

### 1.1 Flujo del Anillo entre Actores (`gengis.exe` + Protocolos v2)

```text
  [PREPARADOR]                      [AUTOR]                         [CHEF]                         [MOTOR]
       │                               │                               │                               │
  Prepara index.md,                    │                               │                               │
  chunks\ y status.md                  │                               │                               │
       │                               │                               │                               │
       ├─── gengis plan <DIR|ID> ─────►│ Aprueba [s/N] (PLAN_APROBADO) │                               │
       │                               ├─── guarda base_ref + state ──►│                               │
       │                               │                               │                               │
       │                               │                       Ejecuta 1 chunk                         │
       │                               │                       y entrega cambios                       │
       │                               │                               ├─── gengis check <DIR|ID> ────►│
       │                               │                               │     Commit selectivo +         │
       │                               │                               │     ShellRunner (evidencia G1) │
       │◄── Modo QA (revisión G5) ─────┼───────────────────────────────┴───────────────────────────────┤
       │                               │     Genera workspace\qa\<chunk>.md + [MOTOR] VERIFICACION     │
       │                               │                               │                               │
       ├─── gengis decide <DIR|ID> ───►│ Emite decisión:               │                               │
       │    [--qa-pass] [--qa-notes]   │ GO_NEXT | RETRY_CHUNK |       │                               │
       │                               │ RESPLIT_CHUNK | ABORT         ├──── SyncFiles() ─────────────►│
       │                               │                               │     status.md, log global y    │
       │                               │                               │     ## Historial de feedback  │
```

### 1.2 Roles Oficiales

| Actor / Prefijo en Log | Responsabilidad principal | Dónde opera |
| :--- | :--- | :--- |
| **`[PREPARADOR]`** | Explora el destino en solo lectura, redacta `index.md` y segmenta en `chunks\`. Ejecuta **Modo QA** (muestreo **G5** y overrides `--qa-pass`/`--qa-notes`) y replanifica (`REPLAN`) si hay `RESPLIT_CHUNK`. Nunca edita código ni entregables del Chef durante QA. | `gengis_swap_common.md`, `gengis_swap_prep_protocol.md`, `workspace\qa\`, `workspace\verificacion\` |
| **`[CHEF]`** | Ejecuta un chunk activo respetando `## Alcance` y **G1–G6**. Realiza entrega sin autocierre (`[~] entregado`), o detiene la cadena en `[-] parcial` / `[!] bloqueado`. Nunca marca `[x] hecho`. | `gengis_swap_common.md`, `gengis_swap_chef_protocol.md`, destino de la tarea |
| **`[MOTOR]`** | Binario `gengis.exe`: verifica `Scope`, crea commits por intento en Git, corre `CommandRunner`, registra evidencia **G1** literal, persiste `.ring\state.json`, regenera `status.md` y escribe líneas en `gengis_global_log.md`. | `GengiSWAP\engine\` |
| **`[AUTOR]`** | Humano que gobierna la consola del anillo: aprueba planes (`PLAN_APROBADO`) y decide tras cada entrega (`GO_NEXT`, `RETRY_CHUNK`, `RESPLIT_CHUNK`, `ABORT`), además de autorizar ajustes al propio motor. | Consola CLI (`gengis`) y `gengis_global_log.md` |

---

## 2. Estructura del Repositorio vs. Datos Externos

### 2.1 Contenido versionado en `C:\Desarrollo\Proyecto Tungsteno`

> **Estado observado (2026-11-19):** repositorio Git inicializado en la rama `main` con remoto `origin` apuntando a `https://github.com/blackpusch/Proyecto_Tungsteno.git`.

| Ruta relativa | Tamaño / Líneas | Propósito observado |
| :--- | :---: | :--- |
| [`.gitignore`](../.gitignore) | `20` L / `464` B | Excluye binario `GengiSWAP/gengis.exe`, bitácora `gengis_global_log.md`, cachés Python y cualquier copia accidental de `gengis_task/`, `Bodega de datos/` o `.ring/`. |
| [`README.md`](../README.md) | `57` L / `3,169` B | Introducción del repositorio oficial, estructura general y comandos de compilación/uso CLI. |
| [`Docs\PROYECTO_TUNGSTENO.md`](PROYECTO_TUNGSTENO.md) | Este archivo | Documentación técnica del repositorio y arquitectura de `GengiSWAP`. |
| [`GengiSWAP\gengis_swap_common.md`](../GengiSWAP/gengis_swap_common.md) | `96` L / `16,465` B | **Fuente única compartida:** `<RAIZ_GENGISWAP>`, repositorio oficial, `[AUTOR]`, reglas **G1–G6**, diagrama offline, leyenda `status.md` y sección `Motor (gengis.exe)`. |
| [`GengiSWAP\gengis_swap_prep_protocol.md`](../GengiSWAP/gengis_swap_prep_protocol.md) | `137` L / `16,920` B | Protocolo v2 del Preparador y Modo QA (referencia a `gengis_swap_common.md`). |
| [`GengiSWAP\gengis_swap_chef_protocol.md`](../GengiSWAP/gengis_swap_chef_protocol.md) | `78` L / `12,079` B | Protocolo v2 del Chef Ejecutor (referencia a `gengis_swap_common.md`). |
| `GengiSWAP\engine\` | `10` archivos | Módulo Go `gengiswap/ring` (`go 1.24`) y suites de pruebas unitarias/CLI. |
| [`GengiSWAP\templates\`](../GengiSWAP/templates/README.md) | `README.md` (`33` L) + `4` plantillas `v1` | Plantillas canónicas de tareas adaptadas a `gengis_swap_common.md`. |
| `GengiSWAP\.vscode\extensions.json` | `5` L / `70` B | Extensión recomendada (`Gruntfuggly.todo-tree`), unificada dentro de `GengiSWAP\`. |
| `GengiSWAP\.vscode\settings.json` | `46` L / `920` B | Configuración visual **Todo Tree** para `[ ]`, `[~]`, `[x]`, `[-]`, `[!]`. |

### 2.2 Artefactos locales ignorados por Git y carpeta externa de datos (`C:\Desarrollo\GengiSWAP_data\`)

Para mantener el repositorio liviano y desacoplado de los `.md` operativos y capturas históricas (pensados para migrar a un sistema dedicado fuera del motor):

- **Ignorados en local mediante `.gitignore` (permanecen en `GengiSWAP\` sin subirse al remoto):**
  - `GengiSWAP\gengis.exe` (`5,305,344` B, SHA256 `8F3743E50BEB795067C5CC82F9400CB9A03B49E76207D727C983C9B52F9D8721`): compilable en `GengiSWAP\engine\` con `go build -o ../gengis.exe ./cmd/gengis`.
  - `GengiSWAP\gengis_global_log.md` (`104` L / `28,360` B, registros de `2026-10-09 14:22:57` a `2026-11-18 09:18:37`): bitácora operativa local *append-only*.
- **Trasladados fuera del repositorio a `C:\Desarrollo\GengiSWAP_data\`:**
  - `C:\Desarrollo\GengiSWAP_data\gengis_task\` (`93` archivos): contiene la tarea `fix_ri_hallazgos` (`27` archivos) y `_archivo\` con `6` tareas históricas (`app_map_ri_v1`, `audit_admin_e2e`, `auditoria_funcional_ri`, `fix_admin_audit_2`, `fix_admin_audit_3` —respaldada únicamente en `workspace.zip` de `49` archivos tras retirar la carpeta suelta— y `fix_admin_audit_recepcion`).
  - `C:\Desarrollo\GengiSWAP_data\Bodega de datos\` (`14` archivos): respaldos previos (`engine-pre-paso4-20261118\`, `gengis.exe.pre-paso4c.sha256` y `3` `*.bak-20261114`; `go.mod.bak-20261114` y los `24` `.bak` de `GengiSWAP\` ya fueron depurados).

**Operación de `gengis.exe` con tareas en `GengiSWAP_data\`:**  
Dado que `resolvePaths` ([main.go:160-195](../GengiSWAP/engine/cmd/gengis/main.go#L160-L195)) acepta una ruta directa a la carpeta de la tarea y la bandera `--root`, para seguir registrando eventos en `GengiSWAP\gengis_global_log.md` manteniendo las tareas en la carpeta hermana se invoca pasando la ruta directa de tarea más `--root` (o `GENGIS_ROOT`):

```powershell
.\gengis.exe status "C:\Desarrollo\GengiSWAP_data\gengis_task\fix_ri_hallazgos" --root "C:\Desarrollo\Proyecto Tungsteno\GengiSWAP"
```

*(Nota: si se omitiera `--root` y `GENGIS_ROOT`, `detectGengisRoot`/`looksLikeDirectTaskDir` detectaría `C:\Desarrollo\GengiSWAP_data\` como raíz porque contiene `gengis_task\` según [main.go:184-189, 237-245](../GengiSWAP/engine/cmd/gengis/main.go#L184-L189).)*

---

## 3. Protocolos v2, Reglas Globales (`G1`–`G6`) y Anatomía de Tareas

### 3.1 Reglas Globales Obligatorias
Centralizadas como fuente única en [`gengis_swap_common.md`](../GengiSWAP/gengis_swap_common.md#L19-L29) y referenciadas obligatoriamente por Preparador, Chef y plantillas:
- **G1 — Evidencia literal:** ningún chunk pasa a `[~] entregado` ni `[x] hecho` sin comando ejecutado, código de salida y últimas líneas literales en `status.md` (más commit si aplica Git; el cierre a `[x] hecho` requiere además `QA PASS` y `DECISION GO_NEXT`).
- **G2 — Encabezado de ubicación:** en repositorios Git, todo `.md` y `.txt` en `workspace\` inicia con `Repo:`, `Rama:` y `HEAD:`. Para `.csv`, `.json`, `.d2` y `.drawio.svg` no se altera el contenido; se acompaña con `UBICACION.md` en la misma carpeta.
- **G3 — No mutar datos reales:** toda mutación queda prohibida salvo autorización expresa en `- Mutación de datos (G3):` de `## Alcance`.
- **G4 — No secretos:** prohibido leer o transcribir contraseñas, tokens o cadenas de conexión.
- **G5 — Rigor de evidencia:** citar `archivo:línea`, separar `HECHOS` de `HIPÓTESIS` y verificar símbolos existentes en código real antes de usarlos (si faltan, marcar `[!]`).
- **G6 — Detención ante ambigüedad:** ante riesgo o ambigüedad, detenerse y marcar `[!]`.
- **Estándar offline de diagramas:** prohibido generar bloques `` ```mermaid `` en tareas GengiSWAP. Se permiten bloques ASCII/Unicode Box-Drawing embebidos en `.md`, o archivos `.d2` / `.drawio.svg` en `workspace\` cubiertos por `UBICACION.md`.
- **Inmutabilidad del log:** `gengis_global_log.md` nunca se edita retroactivamente (*append-only*).

### 3.2 Artefactos de una Tarea (`gengis_task\<ID>\`, hoy en `C:\Desarrollo\GengiSWAP_data\`)
- **`index.md`:** encabezado `# Mapa de Tarea: <ID>`, línea `Plantilla: <nombre> vX`, sección obligatoria `## Alcance` (`Puede modificar`, `Mutación de datos (G3)`, `Solo lectura` —incluyendo `workspace\verificacion\` y chunks `[x] hecho`— y `Prohibido`), `## Directorio de Destino Real`, `## Supuestos y Stack Tecnológico` (`Máx. intentos por chunk: 3`), `## Árbol de Carpetas Esperado`, `## Contratos entre Pasos` y `## Definición de Terminado (DoD)`. No contiene estado de chunks.
- **`chunks\NN_*.md`:** campos obligatorios `Entradas`, `Salidas`, `Criterios de Aceptación`, `Comando de Verificación`, **`Prueba de que puede fallar`** y `Fuera de Alcance`. Tras un `RETRY_CHUNK`, el motor anexa al final la sección `## Historial de feedback`.
- **`status.md` (Protocolo v2 semántico):** regenerado automáticamente por `RenderStatusMD` ([ring.go:939–1004](../GengiSWAP/engine/ring.go#L939-L1004)) con los 5 estados:
  1. `[ ] **pendiente**` (`StatusPending`)
  2. `[~] **entregado**` (`StatusDelivered`)
  3. `[x] **hecho**` (`StatusDone`)
  4. `[-] **parcial**` (`StatusPartial`, añade bloque de detalle `## Chunk <ID> — [-] Parcial`)
  5. `[!] **bloqueado**` (`StatusBlocked`, añade bloque de detalle `## Chunk <ID> — [!] Bloqueado`)
- **`workspace\`:** artefactos de trabajo, `workspace\verificacion\` (solo lectura para el Chef cuando el Preparador define pruebas) y `workspace\qa\<safeName(chunk)>.md` generado por `RenderQAReport` ([ring.go:886–906](../GengiSWAP/engine/ring.go#L886-L906)) con secciones `(a) Verificación independiente`, `(b) Alcance` y `(c) Revisión del Preparador (G5, muestreo)`.
- **`.ring\`:** directorio de control del motor con `.ring\state.json` (fuente de verdad serializada) y `.ring\base_ref` (referencia `HEAD` escrita por `plan` y `decide`).

---

## 4. Catálogo de Plantillas `v1` (`GengiSWAP\templates\`)

Documentado en [`templates\README.md`](../GengiSWAP/templates/README.md) (`33` L / `4,071` B). Cada plantilla lleva `# Plantilla: <nombre> v<N>` en su primera línea y apunta sus reglas globales a `gengis_swap_common.md`:

| Plantilla | Archivo | Líneas / Bytes | Procedencia histórica | Reglas / Uso destacado |
| :--- | :--- | :---: | :--- | :--- |
| `auditoria_e2e` | [`templates\auditoria_e2e\index.md`](../GengiSWAP/templates/auditoria_e2e/index.md) | `70` L / `6,424` B | `audit_admin_e2e` | Auditoría E2E de caja negra en directorio externo aislado interceptando mutaciones; aplica **regla R2** (regeneración completa del reporte desde cero en el último chunk). |
| `mapeo_estatico` | [`templates\mapeo_estatico\index.md`](../GengiSWAP/templates/mapeo_estatico/index.md) | `91` L / `8,825` B | `app_map_ri_v1` | Mapeo estático en solo lectura con diagramas offline ASCII/Unicode, `.d2` o `.drawio.svg`. |
| `correccion_hallazgos` | [`templates\correccion_hallazgos\index.md`](../GengiSWAP/templates/correccion_hallazgos/index.md) | `76` L / `7,818` B | `fix_admin_audit_recepcion`, `fix_admin_audit_2`, `fix_admin_audit_3` | Remediación incremental con entorno efímero aislado y **regla R2** en el chunk de integración final. |
| `modulo_nuevo` | [`templates\modulo_nuevo\index.md`](../GengiSWAP/templates/modulo_nuevo/index.md) | `76` L / `6,604` B | Derivada de `prep_protocol` | Creación de un módulo nuevo por capas dentro de un repositorio existente. |

---

## 5. Arquitectura del Motor Go 1.24 (`GengiSWAP\engine\`)

Módulo `gengiswap/ring` (`go.mod`, `go 1.24`).

### 5.1 Inventario de Archivos Fuentes y Pruebas

| Archivo | Líneas | Bytes | Responsabilidad |
| :--- | :---: | :---: | :--- |
| [`go.mod`](../GengiSWAP/engine/go.mod) | `3` | `31` | Módulo `gengiswap/ring` (Go 1.24). |
| [`cmd\gengis\main.go`](../GengiSWAP/engine/cmd/gengis/main.go) | `258` | `6,301` | CLI: parsing de subcomandos/banderas, detección de raíz y despacho a `ring`. |
| [`cmd\gengis\main_test.go`](../GengiSWAP/engine/cmd/gengis/main_test.go) | `115` | `4,042` | Una única prueba de integración `TestCLI_PlanCheckDecideQAFailAndAbort` (requiere `git`): `plan` → `check` (PASS) → `decide --qa-pass=false` (GO_NEXT rechazado, ABORT) → `status`. Cubre `reorderFlags` y `--root` solo de forma indirecta; no hay pruebas dedicadas de `resolvePaths`/`detectGengisRoot`/`looksLikeDirectTaskDir`. |
| [`ring.go`](../GengiSWAP/engine/ring.go) | `1,004` | `29,871` | Máquina de estados del anillo, `Scope`, `Attempt`, `Event`, `FileStore`, `applyQAResult`, `checkTerminalState`, `newAttempt`, `RenderQAReport`, `RenderStatusMD`. |
| [`ring_test.go`](../GengiSWAP/engine/ring_test.go) | `1,080` | `34,363` | Suite del núcleo del anillo, reintentos, reverts, UTF-8 pipes y quoting Windows. |
| [`runner.go`](../GengiSWAP/engine/runner.go) | `258` | `7,592` | `ShellRunner`, `withUTF8PythonEnv`, `Workspace`, `NopWorkspace` y `GitWorkspace`. |
| [`runner_windows.go`](../GengiSWAP/engine/runner_windows.go) | `26` | `619` | Construcción directa de `SysProcAttr.CmdLine` con `cmd.exe /S /C`. |
| [`runner_unix.go`](../GengiSWAP/engine/runner_unix.go) | `12` | `192` | Ejecución Unix con `sh -c`. |
| [`bridge.go`](../GengiSWAP/engine/bridge.go) | `1,720` | `46,579` | Parser Markdown (parseo único `parseIndexFromDir`), `SyncFiles`, feedback en chunks, advertencias (`warnIfNoGit`, `printPlanSummary`, `collectAbsoluteScopePaths`, recordatorio G5 en `DecideChunk` con `autoVerifyQANote`) y flujos CLI (`PlanTask`, `CheckChunk`, `DecideChunk`, `ShowStatus`). |
| [`bridge_test.go`](../GengiSWAP/engine/bridge_test.go) | `1,166` | `43,054` | Pruebas del parser Markdown, consistencia post-commit/fallo en `CheckChunk`, guards de plan no aprobado, `target-repo`, advertencias sin Git/plan y recordatorio G5. |

### 5.2 Interfaz CLI (`cmd\gengis\main.go`)

- **Subcomandos disponibles:**
  - `gengis plan <TAREA_ID>`: invoca [`ring.PlanTask`](../GengiSWAP/engine/bridge.go#L1208); parsea `index.md` y `chunks\` una sola vez, valida los chunks (`validateChunks`), solicita aprobación `[s/N]` del Autor (`cliAuditor.ReviewPlan`), guarda `.ring\base_ref` y `.ring\state.json`, y sincroniza `status.md` + `gengis_global_log.md`.
  - `gengis check <TAREA_ID>`: invoca [`ring.CheckChunk`](../GengiSWAP/engine/bridge.go#L1285); registra la entrega del chunk activo, detecta archivos cambiados contra `base_ref` (sin modificar `base_ref`), ejecuta `qaStep()` (commit selectivo + ejecución real de `VerifyCommands` vía `ShellRunner` + verificación de alcance), emite `[MOTOR] VERIFICACION`, escribe `workspace\qa\<safeName>.md`, guarda `state.json` y sincroniza `status.md` + log. Ante fallo en `qaStep`, retorna el error **sin llamar a `SyncFiles`** ([bridge.go:1341-1343](../GengiSWAP/engine/bridge.go#L1341-L1343)), evitando reflejar estado no persistido y convergiendo al reintentar.
  - `gengis decide <TAREA_ID> [--qa-pass] [--qa-notes]`: invoca [`ring.DecideChunk`](../GengiSWAP/engine/bridge.go#L1354); permite aplicar confirmación/notas QA del Preparador (`applyManualQAOverride` → `applyQAResult`), solicita la decisión del `[AUTOR]`, ejecuta `auditStep()`, marca `DONE` + `[MOTOR] FIN` al aprobar el último chunk o actualiza `.ring\base_ref` si `!DONE`, y sincroniza `status.md`, `chunks\` y el log.
  - `gengis status <TAREA_ID>`: invoca [`ring.ShowStatus`](../GengiSWAP/engine/bridge.go#L1436); imprime el resumen de estado y regenera `status.md` sin duplicar líneas en el log.
- **Banderas y resolución de rutas:**
  - `--root <DIR>` (o `GENGIS_ROOT`, o `detectGengisRoot()` subiendo directorios hasta hallar `gengis_global_log.md` o `gengis_task\`).
  - `reorderFlags()` permite ubicar `--root`, `--qa-pass` y `--qa-notes` antes o después de `<TAREA_ID>`.
  - `looksLikeDirectTaskDir()` acepta tanto un `<TAREA_ID>` bajo `<root>\gengis_task\` como una ruta directa a cualquier directorio que contenga `index.md` o `.ring\`.

### 5.3 Máquina de Estados, Control de Alcance y Reanudación (`ring.go`)

- **Estados globales (`State.Status`):** `UNINITIALIZED` (solo lectura en `status`), `PLAN_PENDING`, `RUNNING`, `DONE`, `ABORTED`. Ante `RESPLIT_CHUNK`, los chunks sustituidos pasan a `State.Replaced` preservando su historial.
- **Helpers compartidos entre `Engine.Run` y CLI:** `checkTerminalState` ([ring.go:428–437](../GengiSWAP/engine/ring.go#L428-L437)), `newAttempt` ([ring.go:439–445](../GengiSWAP/engine/ring.go#L439-L445)) y `applyQAResult` ([ring.go:699–730](../GengiSWAP/engine/ring.go#L699-L730)).
- **Evaluación de `Scope` ([ring.go:88–140](../GengiSWAP/engine/ring.go#L88-L140)):**
  - `norm()` convierte `\` a `/` para evaluar igual en Windows y Linux/Git.
  - `Scope.Allows(p)` rechaza traversal (`..`, `../`), rutas bajo `Forbidden` o `ReadOnly`, y exige coincidencia con algún prefijo de `CanModify`.
  - `Scope.narrows(p)` permite al chunk restringir aún más `CanModify`/`ReadOnly`/`Forbidden`, nunca ampliar el alcance de la tarea.
- **Commit selectivo y limpieza de violaciones (`qaStep` / `auditStep`):**
  - En `qaStep` ([ring.go:648–690](../GengiSWAP/engine/ring.go#L648-L690)), el motor solo incluye en el commit `"gengis: <TaskID>/<ChunkID> intento <N>"` los archivos dentro de alcance (`without(changed, att.ScopeViolations)`). Los archivos fuera de alcance quedan sin commitear y hacen fallar el QA (`QAPassed = false`).
  - En `auditStep` ([ring.go:798–814](../GengiSWAP/engine/ring.go#L798-L814)), si el Autor decide `RETRY_CHUNK` sin revertir (`Revert = false`) habiendo `ScopeViolations`, el motor fuerza `WS.Revert` desde `CommitRef`/`BaseRef` para resguardar y limpiar esos archivos fuera de alcance mediante `git stash`.
- **Reglas de decisión (`allowedDecisions`, [ring.go:732–741](../GengiSWAP/engine/ring.go#L732-L741)):**
  - `GO_NEXT` solo está habilitada cuando `att.QAPassed == true`.
  - `RETRY_CHUNK` solo está habilitada mientras `len(c.Attempts) < maxRetries`.
  - `RESPLIT_CHUNK` y `ABORT` están siempre habilitadas en revisión de entrega.
- **Persistencia atómica y concurrencia en Windows:**
  - `FileStore.Save` ([ring.go:340–367](../GengiSWAP/engine/ring.go#L340-L367)) escribe en `.ring-state-*`, llama `Sync()`, cierra y ejecuta `os.Rename()`.
  - `safeFileStore.Save` ([bridge.go:1123–1142](../GengiSWAP/engine/bridge.go#L1123-L1142)) reintenta hasta `5` veces con espera creciente (`15ms * (attempt+1)`) ante errores transitorios `os.ErrPermission`.
- **Reanudación sucia (`ResumePolicy`, [ring.go:372–378, 578–604](../GengiSWAP/engine/ring.go#L578-L604)):** distingue `ResumeAsk` (`ErrDirtyResume`), `ResumeContinue` (`[MOTOR] RETOMA`) y `ResumeReset` (revierte con `git revert` desde `BaseRef`).

### 5.4 Ejecución de Comandos, Control Git y Parches de Windows (`runner.go`, `runner_windows.go`)

- **`ShellRunner` ([runner.go:31–91](../GengiSWAP/engine/runner.go#L31-L91)):**
  - Timeout por defecto en CLI de `5 * time.Minute` ([bridge.go:1178](../GengiSWAP/engine/bridge.go#L1178)) y `WaitDelay = 3s`.
  - Sanitización y truncado UTF-8 (`MaxOutput = 8000` B por defecto: conserva `4000` B iniciales + `\n…[truncado]…\n` + `4000` B finales vía `strings.ToValidUTF8`).
  - **Quoting Windows ([runner_windows.go:13–26](../GengiSWAP/engine/runner_windows.go#L13-L26)):** `shellCommand` arma explícitamente `CmdLine: "<comspec>" /S /C "<trimmed>"` para evitar que `syscall.EscapeArg` transforme comillas internas en `\"` y rompa rutas con espacios.
  - **Codificación Python UTF-8 en pipes (`runner.go:81–91`):** `withUTF8PythonEnv()` sustituye/añade `PYTHONUTF8=1` y `PYTHONIOENCODING=utf-8` en `cmd.Env`, evitando excepciones `UnicodeEncodeError` en consola `cp1252`.
- **`GitWorkspace` y `NopWorkspace` ([runner.go:110–258](../GengiSWAP/engine/runner.go#L110-L258)):**
  - `GitWorkspace` detecta cambios vía `git status --porcelain=v1 -z --untracked-files=all` y `git diff --name-only -z base HEAD`, hace stage selectivo con `git add -A -- <todo>` y revierte únicamente con `git stash push --include-untracked` + `git revert --no-edit`.
  - `NopWorkspace` opera en tareas sin repositorio Git o de solo lectura.

### 5.5 Puente Markdown <-> Estado (`bridge.go`) y Pendientes Conocidos (Paso 6)

- **Parser de tarea (`parseIndexFromDir` / `ParseTaskFromDir`, [bridge.go:33–75](../GengiSWAP/engine/bridge.go#L33-L75)):**
  - `parseIndexFile` ignora bloques fenced, lee `## Alcance` (`Puede modificar`, `Solo lectura`, `Prohibido`/`Fuera de alcance`/`No tocar`), prioriza el repositorio destino en `## Directorio de Destino Real` y extrae `Máx. intentos` (`reMaxRetries`).
  - `parseChunksDir` / `parseChunkFile` parsea `chunks\*.md` extrayendo `Spec`, `Acceptance`, `VerifyCommands` (inline o en bloques fenced) y `FailureProof`.
- **Sincronización automática (`SyncFiles`, [bridge.go:904–1083](../GengiSWAP/engine/bridge.go#L904-L1083)):**
  - Reescribe `status.md` con `RenderStatusMD(st)`.
  - Anexa eventos nuevos a `gengis_global_log.md` (`appendNewLogLines` + `State.LogLines()`).
  - Anexa `## Historial de feedback` dentro de `chunks\NN_*.md` ante cada `RetryChunk`.
- **Modo `RESPLIT_CHUNK` (`cliPreparador.Resplit`, [bridge.go:1573–1634](../GengiSWAP/engine/bridge.go#L1573-L1634)):** absorbe nuevos archivos `.md` en `chunks\` (opción `dir`) o captura subchunks por consola.
- **Pendientes documentados para el Paso 6 / evolución futura:**
  1. Distinguir el override manual en `DecideChunk` como `QA_OVERRIDE` (actualmente emite un segundo `[PREPARADOR] QA`).
  2. Alinear la salida breve de `RenderStatusMD` ([ring.go:966–1002](../GengiSWAP/engine/ring.go#L966-L1002)) con la leyenda/plantilla de `gengis_swap_common.md`.
  3. Añadir cursor de eventos sincronizados en `.ring\state.json` para evitar tanto la pérdida de líneas en `gengis_global_log.md` ante un corte entre `e.save(st)` y `SyncFiles` en `CheckChunk` como el volcado de `[AUTOR] DECISION` si fallan `Revert`/`Resplit` en `DecideChunk`.
  4. Desacoplar la lectura/almacenamiento de tareas (`ParseTaskFromDir` / `.md` de chunks) tras una interfaz `TaskSource` cuando el gestor de chunks migre fuera del disco local.

---

### 5.6 Revisión profunda del código (`2026-11-18`) — Comportamientos implícitos `[HECHO]`

Comportamientos verificados en el código que no estaban explícitos en las secciones anteriores. Solo documentan; **no implican cambios autorizados**.

1. **El QA semántico del Preparador en `check` es automático.** `cliPreparador.Verify` siempre retorna `true, autoVerifyQANote` (`"verificación automática OK (G5 pendiente: usar decide --qa-pass)"`, [bridge.go:1618-1622](../GengiSWAP/engine/bridge.go#L1618-L1622)). En CLI, `QA PASS` tras `check` solo depende de: ≥1 evidencia, todos los códigos de salida `0` y ninguna violación de alcance ([ring.go:722](../GengiSWAP/engine/ring.go#L722)). La revisión real **G5** del Preparador entra únicamente vía `decide --qa-pass/--qa-notes`; si se invoca `DecideChunk` sin `--qa-*`, el motor emite una advertencia recordando la revisión G5 manual ([bridge.go:1430-1432](../GengiSWAP/engine/bridge.go#L1430-L1432)).
2. **`check` = declaración de entrega del Chef.** El CLI no invoca `Chef.Execute`; `CheckChunk` marca `ChefDone=true` + `CHEF ENTREGADO` directamente ([bridge.go:1352-1356](../GengiSWAP/engine/bridge.go#L1352-L1356)). El Chef trabaja fuera del motor.
3. **Sin Git no hay control de alcance efectivo.** `NopWorkspace.ChangedSince` devuelve `nil` ([runner.go:114](../GengiSWAP/engine/runner.go#L114)), por lo que `ScopeViolations` siempre queda vacío y no hay commit ni revert. `runtimeDeps` cae en `NopWorkspace` si el destino no existe, no es Git o no se pudo resolver ([bridge.go:1158-1181](../GengiSWAP/engine/bridge.go#L1158-L1181)); `warnIfNoGit` ([bridge.go:1188-1199](../GengiSWAP/engine/bridge.go#L1188-L1199)) emite ahora una advertencia explícita en `plan`, `check` y `decide` cuando opera sobre `NopWorkspace`.
4. **Errores de parseo silenciados en `check`/`decide`.** `resolveTargetRepo` descarta el error de `parseIndexFromDir` y devuelve `""` ([bridge.go:1148-1156](../GengiSWAP/engine/bridge.go#L1148-L1156)) → `NopWorkspace` + `ShellRunner.Dir = TaskDir` sin advertencia.
5. **`ResumePolicy` / `checkDirtyResume` solo existen en `Engine.Run`** ([ring.go:526-530, 578-604](../GengiSWAP/engine/ring.go#L578-L604)). Los subcomandos CLI no los usan (consistente con `gengis_swap_common.md` §5).
6. **`base_ref` se mueve entre intentos.** Tras `RETRY_CHUNK` **sin revertir**, `decide` guarda el `HEAD` actual (que incluye el commit del intento fallido) en `.ring\base_ref` ([bridge.go:1424-1426](../GengiSWAP/engine/bridge.go#L1424-L1426)); el intento siguiente usa esa base ([bridge.go:1319-1325](../GengiSWAP/engine/bridge.go#L1319-L1325)). Consecuencias: los archivos del intento anterior no aparecen en `ChangedFiles` del nuevo intento y un revert posterior solo deshace hasta la base del intento vigente. La frase "cada reintento compara cambios contra la misma base" (`gengis_swap_common.md:70`) aplica a re-ejecuciones de `check` dentro del **mismo** intento.
7. **Commits propios del Chef.** `GitWorkspace.Commit` solo stagea archivos *dirty* ([runner.go:199-229](../GengiSWAP/engine/runner.go#L199-L229)). Si el Chef commitea por su cuenta, esos archivos se evalúan en `ScopeViolations` (vía `diff base..HEAD`) y hacen fallar el QA, pero el motor no puede "des-commitearlos"; solo un `Revert` posterior los deshace.
8. **`Revert` sobre rangos.** Usa `git revert --no-edit <base>..HEAD` (un commit de reversión por commit; fallaría ante merges) y, si falla, `git revert --abort`; el `git stash` previo **no se restaura** automáticamente ([runner.go:231-258](../GengiSWAP/engine/runner.go#L231-L258)).
9. **Alcance sensible a mayúsculas.** `under()` usa `strings.HasPrefix` sin normalizar caja ([ring.go:101-113](../GengiSWAP/engine/ring.go#L101-L113)); en Windows, una diferencia de mayúsculas entre `## Alcance` y la ruta real reportada por Git produce una violación de alcance.
10. **`Máx. intentos`: gana la última coincidencia.** `reMaxRetries` se evalúa sobre todas las líneas no-fenced de `index.md` ([bridge.go:115-119](../GengiSWAP/engine/bridge.go#L115-L119)); valor por defecto `3`.
11. **Re-`plan` tras rechazo re-parsea todo.** Si existe `state.json` sin `PlanApproved`, `PlanTask` reemplaza `Prompt`, `Scope`, `MaxRetries` y `Chunks` desde disco ([bridge.go:1250-1253](../GengiSWAP/engine/bridge.go#L1250-L1253)); si ya está aprobado, solo sincroniza.
12. **Subchunks de `RESPLIT` interactivo.** Se crean solo en `state.json`, sin archivo `.md` ni `Scope` propio ([bridge.go:1602-1633](../GengiSWAP/engine/bridge.go#L1602-L1633)); `syncChunkFeedback` los omite silenciosamente al no hallar archivo ([bridge.go:995-998](../GengiSWAP/engine/bridge.go#L995-L998)).
13. **`status` escribe disco aunque esté `UNINITIALIZED`.** `ShowStatus` siempre llama a `SyncFiles`, que crea/sobrescribe `status.md` ([bridge.go:1443-1468](../GengiSWAP/engine/bridge.go#L1443-L1468)); no escribe log porque no hay eventos.
14. **Banderas `--qa-*` solo en `decide`.** Se registran únicamente si `sub == "decide"` ([main.go:74-77](../GengiSWAP/engine/cmd/gengis/main.go#L74-L77)); en otros subcomandos `flag` falla con "flag provided but not defined".
15. **Raíz auto-detectada con tareas en `GengiSWAP_data\`.** Sin `--root`/`GENGIS_ROOT`, `resolvePaths` toma `GengiSWAP_data\` como raíz ([main.go:184-189](../GengiSWAP/engine/cmd/gengis/main.go#L184-L189)) y `appendNewLogLines` **crearía** un `gengis_global_log.md` nuevo allí (`O_CREATE`, [bridge.go:947](../GengiSWAP/engine/bridge.go#L947)), bifurcando la bitácora.
16. **`saveBaseRef` ignora errores** de `MkdirAll`/`WriteFile` ([bridge.go:1188-1195](../GengiSWAP/engine/bridge.go#L1188-L1195)).

### 5.7 Divergencias Protocolo ↔ Motor detectadas `[HECHO]`

| # | Protocolo dice | El código hace | Referencias |
| :-: | :--- | :--- | :--- |
| D1 | `[AUTOR] DECISION` registra `RETRY_CHUNK (con revertir \| sin revertir)` | El evento solo emite `Decisión: RETRY_CHUNK \| Motivo: …`; el flag `Revert` queda en `state.json` pero no en el log | `gengis_swap_common.md:17` vs [ring.go:790](../GengiSWAP/engine/ring.go#L790) |
| D2 | Revert con `git revert --no-edit <hash>` | Revert de rango `<base>..HEAD` | `gengis_swap_common.md:13` vs [runner.go:251](../GengiSWAP/engine/runner.go#L251) |
| D3 | `[!] bloqueado` incluye "límite de reintentos" y "falta de `PLAN_APROBADO`" | `StatusBlocked` solo se asigna ante error del Chef en `Engine.chefStep`; el CLI nunca lo asigna. Al agotar reintentos solo se retira `RETRY_CHUNK` de las decisiones permitidas | `gengis_swap_common.md:41` vs [ring.go:633](../GengiSWAP/engine/ring.go#L633), [ring.go:732-741](../GengiSWAP/engine/ring.go#L732-L741) |
| D4 | Fuente de verdad en `<RAIZ_GENGISWAP>gengis_task\<ID>\.ring\state.json` | Las tareas viven hoy en `C:\Desarrollo\GengiSWAP_data\gengis_task\` (ver §2.2) | `gengis_swap_common.md:65` |
| D5 | Plantilla `[-] Parcial` con `Archivos tocados pero sin verificar` y `Motivo de la parada` | `RenderStatusMD` no emite esos campos (ya registrado como pendiente 2 de §5.5) | `gengis_swap_common.md:44-52` vs [ring.go:973-1002](../GengiSWAP/engine/ring.go#L973-L1002) |

### 5.8 Entorno de esta sesión de revisión

- **Toolchain:** Go `go1.27.4 windows/amd64` en `%USERPROFILE%\sdk\go-msi\Go\bin\go.exe` (en el `PATH` de usuario; las consolas abiertas antes de instalarlo no lo ven). `go.mod` declara `go 1.24`. `git` en `C:\Program Files\Git\cmd\git.exe`.
- **Línea base de pruebas (`2026-11-19`, `go vet ./...` + `go test -count=1 -skip TestShellRunnerWaitDelay ./...`):** `go vet` limpio; `gengiswap/ring` y `gengiswap/ring/cmd/gengis` **PASS** (omitiendo `TestShellRunnerWaitDelay`, [ring_test.go:822-840](../GengiSWAP/engine/ring_test.go#L822-L840), que en Windows sin pila de red termina al instante en `ping -n 11 127.0.0.1`; en verificación previa con red `2026-11-18` pasó en `3.15s`).
- El binario `gengis.exe` recompilado localmente el `2026-11-19` (`5,305,344` B) coincide con el SHA256 `8F3743E50BEB795067C5CC82F9400CB9A03B49E76207D727C983C9B52F9D8721` documentado en §2.2.
