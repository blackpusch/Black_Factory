# Protocolo Común: GengiSWAP (Definiciones y Reglas Compartidas)

Fuente única de definiciones transversales para **Preparador** (`gengis_swap_prep_protocol.md`), **Chef Ejecutor** (`gengis_swap_chef_protocol.md`) y plantillas en `<RAIZ_GENGISWAP>templates\`.

## Rutas del Framework
Todas las rutas internas del sistema usan `<RAIZ_GENGISWAP>` con rutas relativas a dicha raíz.
- **Valor actual de `<RAIZ_GENGISWAP>`:** `C:\Black Factory\GengiSWAP\`
- **Repositorio oficial (GitHub):** `https://github.com/blackpusch/Black_Factory` (raíz local: `C:\Black Factory\`; aloja Black Factory y el motor/protocolos de GengiSWAP (`gengis`)).
- **Ubicación operativa vigente de tareas (`<DIR_TAREA>`):** `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\` (separada de `<RAIZ_GENGISWAP>`; el fallback por `<TAREA_ID>` sigue buscando `<RAIZ_GENGISWAP>gengis_task\<ID>\`). Al pasar una ruta directa de tarea al CLI, usar `--root "<RAIZ_GENGISWAP>"` o definir `GENGIS_ROOT` para mantener un único `gengis_global_log.md`.

## Rol `[AUTOR]` (Auditor)
- El **AUTOR (Auditor)** es el **humano** que gobierna las decisiones de control del anillo de ejecución: autoriza el inicio del plan (`[AUTOR] PLAN_APROBADO | Tarea: <ID>`) y decide sobre cada chunk entregado (`[~] **entregado**`) tras el reporte de `Modo QA` del Preparador.
- Sobre cada chunk marcado como `[~] **entregado**` (y tras el reporte de `Modo QA`), el `AUTOR` emite exactamente una de cuatro decisiones:
  - `GO_NEXT`: aprueba el chunk (habilita su paso a `[x] **hecho**` si también tiene `QA` `Resultado: PASS` del Preparador) y autoriza iniciar el siguiente chunk.
  - `RETRY_CHUNK`: solicita reintentar el chunk (`con revertir` mediante `git revert --no-edit <base>..HEAD` —previo `git stash push --include-untracked` si hay cambios sin commitear— o `sin revertir`), mientras no supere `Máx. intentos por chunk`.
  - `RESPLIT_CHUNK`: ordena dividir o replanificar el chunk mediante `REPLAN` formal del Preparador.
  - `ABORT`: detiene definitivamente la tarea.
- Cada decisión del `AUTOR` se registra obligatoriamente en `<RAIZ_GENGISWAP>gengis_global_log.md` con este formato exacto:
  `[<FECHA_HORA_SISTEMA>] [AUTOR] DECISION | Tarea: <ID> | Chunk: <N> | Decisión: <GO_NEXT | RETRY_CHUNK (con revertir | sin revertir) | RESPLIT_CHUNK | ABORT> | Motivo: <...>`

## Reglas Globales (Acatamiento obligatorio)
- **G1. EVIDENCIA LITERAL:** un chunk solo puede entregarse (`[~] entregado`) o pasar a `[x] hecho` si `status.md` incluye, bajo su viñeta, la evidencia literal: comando ejecutado, código de salida y las últimas líneas de la salida (además de commit si es git; para pasar a `[x] hecho` requiere adicionalmente `QA` `Resultado: PASS` del Preparador y `DECISION` `GO_NEXT` del `[AUTOR]` en el log). Una afirmación sin esa evidencia no cuenta.
- **G2. ENCABEZADO DE UBICACIÓN:** si el destino es un repositorio git, todo archivo `.md` y `.txt` que se deposite en `workspace\` empieza con tres líneas: `Repo: <ruta absoluta>`, `Rama: <rama activa>`, `HEAD: <hash corto>`. Para archivos `.csv`, `.json`, `.d2` y `.drawio.svg` no se inserta encabezado de 3 líneas dentro del archivo (para no romper su sintaxis o XML); en esa misma carpeta se escribe un archivo `UBICACION.md` con `Repo`, `Rama`, `HEAD` y la lista de archivos de datos o diagramas que cubre.
- **G3. NO MUTAR DATOS:** nunca se muta la BD real ni datos reales, ni se ejecutan peticiones que los modifiquen, salvo autorización expresa en `Mutación de datos (G3)` de `## Alcance`. Lo permitido se hace sobre entornos efímeros.
- **G4. NO SECRETOS:** no leer ni transcribir contraseñas, tokens ni cadenas de conexión. Si aparecen, solo indicar que existen y en qué archivo.
- **G5. RIGOR DE EVIDENCIA:** toda afirmación sobre el código cita `archivo:línea`; separar `HECHOS` (vistos en el código) de `HIPÓTESIS` (inferidos). Prohibido inventar rutas, funciones, tablas o parámetros: antes de usar un símbolo, el Chef lo localiza en el código real (`grep` o `import`). Si no lo encuentra, NO lo crea por su cuenta: marca `[!]` y lo registra.
- **G6. DETENCIÓN ANTE AMBIGÜEDAD:** si algo es ambiguo o riesgoso, el Chef lo reporta, marca `[!]` y se detiene; no elige por su cuenta.
- **Estándar oficial de diagramas (offline):** queda **PROHIBIDO** generar bloques `` ```mermaid ``. Formatos permitidos para diagramas:
  - **Diagramas embebidos en archivos `.md`:** bloques de texto plano con caracteres ASCII / Unicode Box-Drawing (`┌ ─ ┐ │ └ ┘ ►`).
  - **Diagramas en archivos independientes dentro de `workspace\`:** archivos `.d2` (D2) o `.drawio.svg` (Draw.io), registrados en el `UBICACION.md` de esa misma carpeta conforme a **G2** (sin encabezado dentro del archivo).
- **Regla de inmutabilidad del Log:** gengis_global_log.md nunca se edita; las correcciones se hacen añadiendo una línea nueva.

## Convenciones de Estado (`status.md` Protocolo v2) y Plantilla `[-] Parcial`
Bloque de pie de página canónico para `status.md` (5 estados semánticos) y plantilla reutilizable para interrupciones o entregas incompletas:

```markdown
---
*Convenciones de estado (Protocolo v2):*
- `pendiente`: Chunk aún no iniciado (`[ ] **pendiente**`).
- `entregado`: Chef terminó la ejecución y adjuntó evidencia G1 (y hash de commit si aplica); espera `Modo QA` del Preparador y decisión del `[AUTOR]` (`[~] **entregado**`). Detiene el avance hasta aprobarse o decidirse reintento/replan.
- `hecho`: Chunk cerrado exclusivamente cuando existe en `gengis_global_log.md` un `QA` con `Resultado: PASS` del `[PREPARADOR]` Y una decisión `GO_NEXT` del `[AUTOR]` (`[x] **hecho**`, respaldado por evidencia literal G1 bajo su viñeta). El Chef nunca marca `[x]`.
- `parcial`: Chunk NO cerrado por interrupción o evidencia incompleta; detiene la cadena (`[-] **parcial**`).
- `bloqueado`: Chunk detenido por impedimento técnico, ambigüedad o bloqueo que requiere intervención del Autor o del Preparador; detiene la cadena (`[!] **bloqueado**`).

<!-- Plantilla reutilizable para documentar un chunk [-] Parcial (copiar encima de --- cuando aplique) -->
## Chunk N — [-] Parcial
- Tipo: interrupción | evidencia incompleta
- Último paso completado: <qué y con qué evidencia>
- Archivos modificados hasta ahora: <lista>
- Archivos tocados pero sin verificar: <lista>
- Último comando ejecutado, código de salida y resumen de su salida: <comando | código de salida | últimas líneas>
- Qué falta: <pasos concretos>
- Motivo de la parada: <límite de contexto / error / reinicio>
- Para retomar: continuar | revertir y repetir (recomendación del Chef)
```

## Motor (`gengis.exe`)
Referencia operativa del binario CLI compilado en `<RAIZ_GENGISWAP>gengis.exe` (código fuente en `<RAIZ_GENGISWAP>engine\`).

> **Advertencias operativas del motor (`[HECHO]`):**
> - **Sin repositorio Git, el control de alcance NO está activo:** si el destino no es un repositorio Git válido (o no pudo resolverse desde `index.md`), el motor usa `NopWorkspace`, por lo que **no** detecta archivos modificados, **no** valida `## Alcance` (`ScopeViolations` siempre queda vacío) y **no** hace commits ni reverts. La comprobación de alcance recae por completo en el Preparador (Modo QA).
> - **`QA PASS` tras `gengis check` NO incluye revisión G5:** `CheckChunk` usa `cliPreparador.Verify`, que aprueba automáticamente (`"verificación automática OK (G5 pendiente: usar decide --qa-pass)"`). Un `PASS` de `check` solo confirma comandos con código `0` y alcance (si hay Git); la revisión por muestreo **G5** del Preparador solo entra al motor mediante `gengis decide --qa-pass=true|false --qa-notes="..."` (si se invoca `gengis decide` sin `--qa-*`, el motor emite una advertencia recordando la revisión G5 manual).

### 1. Subcomandos y artefactos que escribe cada uno
- **`gengis plan <TAREA_ID>`** (`PlanTask`): solo lectura sobre `index.md` y `chunks\` para validar estructura y `## Alcance`; solicita aprobación `[s/N]` del `[AUTOR]`. Escribe `.ring\state.json` (`PLAN_PENDING` o `RUNNING` con `PlanApproved=true`), escribe `.ring\base_ref` con el `HEAD` del repo destino si el plan queda aprobado, regenera `status.md` (`SyncFiles`) y añade eventos nuevos (`[PREPARADOR] PLAN_PENDIENTE`, `[AUTOR] PLAN_APROBADO` / `PLAN_RECHAZADO`) a `gengis_global_log.md`.
- **`gengis check <TAREA_ID>`** (`CheckChunk`): lee `.ring\state.json` y `.ring\base_ref` (**no escribe `base_ref`**; si falta o está vacío, usa `HEAD`). Sobre el chunk activo, marca entrega (`CHEF ENTREGADO`); si el destino es Git (`GitWorkspace`), `Commit` comitea **solo** archivos dentro de alcance (`"gengis: <TaskID>/<ChunkID> intento <N>"`); luego corre cada `Comando de Verificación`, escribe `workspace\qa\<safeName(chunk)>.md` (`/` y `\` normalizados a `_`), guarda `.ring\state.json`, regenera `status.md` y añade eventos a `gengis_global_log.md`.
- **`gengis decide <TAREA_ID> [--qa-pass=true|false] [--qa-notes="..."]`** (`DecideChunk`): si recibe `--qa-*`, ejecuta `applyManualQAOverride` → `applyQAResult`, reescribe `workspace\qa\<safeName(chunk)>.md` y guarda `.ring\state.json` antes de consultar al `[AUTOR]`. Tras la decisión (`GO_NEXT`, `RETRY_CHUNK`, `RESPLIT_CHUNK` o `ABORT`), `auditStep` puede revertir en Git (`GitWorkspace.Revert`: `git stash push --include-untracked` si hay cambios sin commitear y `git revert --no-edit <base>..HEAD` sobre los commits posteriores a la base del intento), actualiza `.ring\state.json` (`DONE` o `ABORTED` si aplica), actualiza `.ring\base_ref` en Git si no hubo error y la tarea sigue abierta (`!DONE`), regenera `status.md`, añade eventos nuevos a `gengis_global_log.md` y ante `RETRY_CHUNK` anexa `## Historial de feedback` en `chunks\<chunk>.md`.
- **`gengis status <TAREA_ID>`** (`ShowStatus`): lee `.ring\state.json` (o arma un estado temporal `UNINITIALIZED` en memoria desde `index.md` y `chunks\` sin persistirlo), imprime el resumen y **regenera `status.md`** mediante `SyncFiles` sin duplicar líneas en `gengis_global_log.md`.

### 2. Fuente de verdad y regeneración de `status.md`
- **Fuente de verdad del motor:** `<DIR_TAREA>\.ring\state.json` —en la separación operativa actual, `C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>\.ring\state.json` (o `<RAIZ_GENGISWAP>gengis_task\<ID>\.ring\state.json` al resolver por `<TAREA_ID>`)— guardado atómicamente por `FileStore.Save` y reintentos Windows en `safeFileStore.Save`. El motor no lee ni parsea `gengis_global_log.md` para reconstruir estado; solo añade líneas con `appendNewLogLines` (`O_APPEND`).
- **`status.md` es una vista derivada:** cada corrida de `plan`, `check`, `decide` o `status` llama a `SyncFiles` y sobrescribe `status.md` completo con `RenderStatusMD`. **Toda edición manual a `status.md` (incluida su leyenda al pie) se pierde en la siguiente invocación del motor.**
- **Límite de reintentos y `[!] bloqueado` en el motor:** al alcanzar `Máx. intentos por chunk`, `gengis.exe` **no** cambia el chunk a `[!] bloqueado`; únicamente retira `RETRY_CHUNK` de las decisiones permitidas en `decide` (dejando `GO_NEXT` si `QAPassed`, `RESPLIT_CHUNK` y `ABORT`). La ausencia de `PLAN_APROBADO` tampoco asigna `[!]` (el CLI retorna `ErrPlanNotApproved`). En código, `StatusBlocked` solo se asigna dentro de `Engine.Run` (`chefStep`) ante un error de `Chef.Execute`; los subcomandos CLI nunca lo asignan.
- **Nota pendiente (Paso 6):** la plantilla de leyenda y bloque `[-] Parcial` documentada en `Convenciones de Estado` de este archivo **diverge** de la salida actual de `RenderStatusMD` (viñetas abreviadas, bloques parcial/bloqueado ubicados al pie debajo de `*Convenciones de estado*` y omisión de la plantilla comentada vacía). Queda anotado sin modificar Go ni la plantilla hasta el Paso 6.

### 3. `.ring\base_ref` y recuperación ante cortes en `check`
- **`.ring\base_ref`:** solo lo escriben `PlanTask` (`plan`, al aprobar) y `DecideChunk` (`decide`, si termina sin error y la tarea no está `DONE`). `CheckChunk` únicamente lo lee (`loadBaseRef`), de modo que cada reintento compara cambios contra la misma base.
- **Corte en `check` antes de `e.save(st)` (recuperable):** `CheckChunk` ejecuta `GitWorkspace.Commit` dentro de `qaStep` antes de correr los comandos y guardar `.ring\state.json`. Si el proceso falla o se interrumpe después del commit y antes de `e.save(st)`, `CheckChunk` retorna sin invocar `SyncFiles` (ni `status.md` ni `gengis_global_log.md` registran estado no persistido) y `.ring\base_ref` queda intacto. Al reintentar `gengis check <TAREA_ID>`, `ChangedSince` detecta los cambios de `base..HEAD` y `GitWorkspace.Commit` reutiliza `HEAD` sin duplicar commit, convergiendo en **un único commit** en `base..HEAD` y **una sola línea** `Estado: ENTREGADO` en `gengis_global_log.md`.
- **Corte en `check` después de `e.save(st)` y antes de `SyncFiles`:** ver *Limitaciones conocidas* (`.ring\state.json` conserva el intento, pero el log global y `status.md` inicial pueden carecer de sus eventos).
- **Bloqueo `.git\index.lock` tras corte abrupto de Git:** ver *Limitaciones conocidas* (`GitWorkspace` no elimina locks de Git automáticamente).

### 4. Limitaciones conocidas
- **`DecideChunk` ante fallos en `RetryChunk` o `RESPLIT_CHUNK` (`[HECHO]`):** `auditStep` añade `att.Verdict` y `[AUTOR] DECISION` en memoria antes de ejecutar `GitWorkspace.Revert`, `Resplit` o `validateChunks`. Si alguno falla antes de `e.save(st)`, limpia `att.Verdict` pero **no revierte `st.Events`**, y `DecideChunk` invoca `SyncFiles` aun cuando `auditErr != nil` (necesario para reflejar `ABORT` y overrides QA previos). Ese evento `[AUTOR] DECISION` no persistido en `state.json` puede escribirse en `gengis_global_log.md` pese al fallo.
- **Corte en `check` después de `e.save(st)` y antes de concluir `SyncFiles` (`[HECHO]`):** en `CheckChunk`, `applyQAResult` llama a `e.save(st)` antes de que `CheckChunk` llame a `SyncFiles`. Si ocurre un corte entre `e.save(st)` y la finalización de `SyncFiles`, `.ring\state.json` conserva los eventos del intento (`ENTREGADO`, `VERIFICACION`, `QA`), mientras `gengis_global_log.md` (y `status.md` si falló antes de reescribirlo) puede carecer de ellos. Al reejecutar `gengis check` o `gengis status`, `SyncFiles` sí regenera `status.md` desde `state.json`, pero `gengis_global_log.md` no recibe retroactivamente esos eventos porque `CheckChunk` y `DecideChunk` inicializan `prevEvents` con `len(st.Events)` al cargar `state.json` y `appendNewLogLines` omite índices ya contados.
- **Bloqueo `.git\index.lock` tras corte abrupto de Git (`[HIPÓTESIS]`):** `GitWorkspace` no detecta ni elimina `.git\index.lock`; si una caída mata `git` en medio de `Commit` o `Revert` dejando ese archivo de bloqueo en el repo destino, los reintentos posteriores fallarán hasta retirarlo manualmente.

### 5. Doble evento `QA` actual y eventos `[MOTOR]` en el log
- **Doble evento `[PREPARADOR] QA`:** `CheckChunk` llama a `qaStep` → `applyQAResult` y registra un primer evento `[PREPARADOR] QA` con la verificación automática. Si luego se ejecuta `gengis decide --qa-pass/--qa-notes`, `applyManualQAOverride` vuelve a invocar `applyQAResult`, reescribe `workspace\qa\<safeName(chunk)>.md`, guarda `state.json` y emite un **segundo** `[PREPARADOR] QA` con el mismo prefijo. En el log se distinguen por posición: el QA automático es el primero del intento (inmediatamente después de `[MOTOR] VERIFICACION`), y el dictamen manual aparece como un segundo `[PREPARADOR] QA` posterior para ese chunk (inmediatamente antes de `[AUTOR] DECISION`).
- **Eventos `[MOTOR]`:** escritos en `gengis_global_log.md` (con `<FECHA_HORA_SISTEMA>` en formato `YYYY-MM-DD HH:MM:SS`):
  - **Verificación de comandos** (`CheckChunk` / `qaStep` y `Engine.Run`):
    `[<FECHA_HORA_SISTEMA>] [MOTOR] VERIFICACION | Tarea: <ID> | Chunk: <CHUNK_ID> | <N> comandos, todos ok: <true|false>`
  - **Cierre de tarea** (`DecideChunk` y `Engine.Run`):
    `[<FECHA_HORA_SISTEMA>] [MOTOR] FIN | Tarea: <ID> | todos los chunks aprobados`
  - **Reanudación automática** (solo `Engine.Run` vía `checkDirtyResume`; no emitido por subcomandos CLI):
    `[<FECHA_HORA_SISTEMA>] [MOTOR] RETOMA | Tarea: <ID> | Chunk: <CHUNK_ID> | <detalle>`
    Valores posibles de `<detalle>`: `continúa sobre <N> archivos modificados` (`ResumeContinue`) o `cambios revertidos (git revert) antes de repetir el chunk` (`ResumeReset`).
