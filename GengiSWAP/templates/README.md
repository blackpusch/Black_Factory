# Plantillas Base de Tareas GengiSWAP

Directorio oficial de plantillas canónicas para el rol **Preparador** (`<RAIZ_GENGISWAP>gengis_swap_prep_protocol.md` y `<RAIZ_GENGISWAP>gengis_swap_common.md`).

## Catálogo y Procedencia

| Plantilla | Ruta | Versión | Procedencia histórica | Cuándo usarla |
| :--- | :--- | :---: | :--- | :--- |
| `auditoria_e2e` | `templates\auditoria_e2e\index.md` | `v1` | Derivada de **1 tarea** (`audit_admin_e2e`) | Auditorías E2E de caja negra (funcional, responsive, A11Y con Axe y rendimiento con Lighthouse) ejecutadas desde un directorio externo aislado sin tocar el repositorio evaluado. |
| `mapeo_estatico` | `templates\mapeo_estatico\index.md` | `v1` | Derivada de **1 tarea** (`app_map_ri_v1`) | Relevamientos 100% estáticos y de solo lectura para mapear rutas, permisos/CSRF, modelo de datos, cobertura automatizada y matriz de huecos sin ejecutar mutaciones. |
| `correccion_hallazgos` | `templates\correccion_hallazgos\index.md` | `v1` | Derivada de **3 tareas** (`fix_admin_audit_recepcion`, `fix_admin_audit_2`, `fix_admin_audit_3`) | Remediación incremental de hallazgos de auditoría sobre código/plantillas en rama dedicada y entorno efímero aislado (puerto y BD volátil), verificando integridad de BD real y generando reporte final. |
| `modulo_nuevo` | `templates\modulo_nuevo\index.md` | `v1` | **Genérica v1 experimental, sin precedente histórico, derivada de `prep_protocol`** | Desarrollo de un módulo o funcionalidad nueva desde cero dentro de un repositorio existente, estructurado por capas hasta pruebas e integración. |

## Cómo usar una plantilla (Preparador)

1. Elige el subdirectorio `<tipo>` que mejor encaje con la solicitud en `<RAIZ_GENGISWAP>templates\<tipo>\index.md`.
2. Lee la línea 1 (`# Plantilla: <nombre> vX`) para conocer la versión vigente de la plantilla.
3. Al generar `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\index.md`:
   - Retira la línea 1 `# Plantilla: <nombre> vX` de cabecera de archivo de plantilla y deja `# Mapa de Tarea: <ID_DE_LA_TAREA>` como H1 principal.
   - Conserva inmediatamente debajo del H1 la línea de trazabilidad `Plantilla: <nombre> vX`.
   - Reemplaza todos los marcadores `<PLACEHOLDER>` por rutas, comandos y criterios concretos inspeccionados previamente en modo lectura.
   - Define explícitamente `## Alcance` completo (`Puede modificar`, `Mutación de datos (G3)`, `Solo lectura` —incluyendo `<RAIZ_GENGISWAP>gengis_task\<ID>\workspace\verificacion\` si aplica y entregables `[x] hecho`— y `Prohibido`), conforme a `gengis_swap_prep_protocol.md`.
   - Conserva `- **Máx. intentos por chunk:** 3` en `## Supuestos y Stack Tecnológico` (ajustable por tarea antes de aprobar el plan).
   - Referencia las reglas globales **G1–G6** de `gengis_swap_common.md` **sin copiar su texto** dentro del `index.md` de la tarea.
   - En tareas basadas en `auditoria_e2e` o `correccion_hallazgos`, aplica la **regla R2**: el último chunk regenera el reporte completo desde cero con el estado real de todos los chunks, sin anexar ni contradicciones.
   - Para diagramas, aplica el estándar offline y regla **G2** de `gengis_swap_common.md` (ASCII/Unicode en `.md`, o `.d2` / `.drawio.svg` con `UBICACION.md`).
   - Materializa los chunks en `chunks\0N_*.md` usando `## Estructura Típica de Chunks` como guía (cada chunk exige **`Prueba de que puede fallar`**; retira esa sección auxiliar en el `index.md` final y nunca pongas estado de chunks en `index.md`).
4. Inicializa `status.md` y registra `[PREPARADOR] PLAN_PENDIENTE | Tarea: <ID>` en `gengis_global_log.md`; **el Chef no inicia hasta que exista `[AUTOR] PLAN_APROBADO | Tarea: <ID>` en el log**, y cada chunk avanza mediante entrega `[~]` → `Modo QA` → `DECISION` del `[AUTOR]` → cierre `[x]` según `gengis_swap_prep_protocol.md` y `gengis_swap_common.md`.

## Versionado

- Toda plantilla lleva `# Plantilla: <nombre> v<N>` en su **primera línea**.
- Cambios incompatibles en secciones obligatorias o flujo de chunks incrementan el número de versión (`v1` -> `v2`).
