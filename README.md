# Proyecto Tungsteno — GengiSWAP

Repositorio oficial: `https://github.com/blackpusch/Proyecto_Tungsteno`

**Proyecto Tungsteno** aloja el núcleo versionable de **GengiSWAP**, un sistema de ejecución controlada por anillos (`Preparador → Chef → Motor → Autor`) diseñado para planificar, ejecutar y auditar tareas de desarrollo sobre repositorios de destino externos bajo reglas estrictas (`G1–G6`). Sobre esta base determinista evolucionará la arquitectura de *Software Factory*, dentro de la cual **`gengis`** actúa como equipo ejecutor especializado.

---

## Estructura del Repositorio

- [`Docs/PROYECTO_TUNGSTENO.md`](Docs/PROYECTO_TUNGSTENO.md): Documentación técnica detallada del marco, protocolos, motor Go y comportamientos verificados.
- `GengiSWAP/`:
  - `gengis_swap_common.md`: Fuente única de definiciones transversales, reglas `G1–G6`, rol `[AUTOR]`, convenciones `status.md` v2 y referencia del motor.
  - `gengis_swap_prep_protocol.md`: Protocolo del **Preparador** (exploración read-only, segmentación en chunks, Modo QA y `REPLAN`).
  - `gengis_swap_chef_protocol.md`: Protocolo del **Chef Ejecutor** (ejecución por chunk y entrega sin autocierre).
  - `templates/`: Plantillas canónicas `v1` (`auditoria_e2e`, `mapeo_estatico`, `correccion_hallazgos`, `modulo_nuevo`) y guía de uso en `templates/README.md`.
  - `engine/`: Motor determinista en **Go 1.24** (sin dependencias externas, módulo `gengiswap/ring`) y CLI (`cmd/gengis`).
  - `.vscode/`: Recomendación y configuración de resaltado para estados `[ ]`, `[~]`, `[x]`, `[-]` y `[!]`.

### Separación operativa (no versionada en Git)

Según `.gitignore`, los artefactos operativos locales permanecen fuera del control de versiones:
- `GengiSWAP/gengis.exe` (binario compilado localmente).
- `GengiSWAP/gengis_global_log.md` (bitácora operativa local *append-only*).
- `C:\Desarrollo\GengiSWAP_data\` (carpeta externa para `gengis_task\` y `Bodega de datos\`).

---

## Verificación y Compilación (`gengis.exe`)

Desde `GengiSWAP\engine`:

```powershell
go vet ./...
go test -count=1 ./...
go build -o ..\gengis.exe .\cmd\gengis
```

*(Nota: en entornos Windows sin pila de red activa, `TestShellRunnerWaitDelay` puede fallar porque usa `ping -n 11 127.0.0.1` como proceso lento; puede omitirse con `-skip TestShellRunnerWaitDelay`.)*

---

## Uso rápido del CLI

```powershell
# Aprobar el plan de una tarea externa manteniendo la raíz del log global
.\gengis.exe plan "C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>" --root "C:\Desarrollo\Proyecto Tungsteno\GengiSWAP"

# Registrar entrega y ejecutar verificación determinista + commit selectivo
.\gengis.exe check "C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>" --root "C:\Desarrollo\Proyecto Tungsteno\GengiSWAP"

# Registrar dictamen G5 del Preparador y decisión del [AUTOR]
.\gengis.exe decide "C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>" --root "C:\Desarrollo\Proyecto Tungsteno\GengiSWAP" --qa-pass=true --qa-notes="G5 verificado"

# Consultar estado y regenerar status.md
.\gengis.exe status "C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>" --root "C:\Desarrollo\Proyecto Tungsteno\GengiSWAP"
```
