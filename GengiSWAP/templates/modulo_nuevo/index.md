# Plantilla: modulo_nuevo v1

# Mapa de Tarea: <ID_DE_LA_TAREA> — Implementación de Módulo Nuevo (<NOMBRE_MODULO>)
Plantilla: modulo_nuevo v1

**Objetivo genérico:** Diseñar, implementar, probar e integrar el módulo nuevo `<NOMBRE_MODULO>` dentro de `<REPO_DESTINO>`, respetando las convenciones arquitectónicas y de datos del proyecto existente y validando su funcionamiento sin regresiones en el resto de la aplicación.

*(Nota de procedencia: plantilla base genérica v1 experimental, sin precedente histórico en `gengis_task\`, derivada de `gengis_swap_prep_protocol.md`.)*

**Reglas globales aplicables:** Acatamiento obligatorio de las reglas globales **G1–G6** de `gengis_swap_common.md` (no copiar su texto aquí).

## Alcance
- Puede modificar: `<REPO_DESTINO>\<RUTA_MODULO_NUEVO>\**`, `<REPO_DESTINO>\<ARCHIVO_REGISTRO_MODULO>`, `<REPO_DESTINO>\<CARPETA_PRUEBAS_MODULO>\**` (únicamente en `<RAMA_TRABAJO>`)
- Mutación de datos (G3): `<ninguna | solo BD/esquema efímero en <ENTORNO_PRUEBAS>>`
- Solo lectura: `<REPO_DESTINO>\<MODULOS_EXISTENTES_DE_REFERENCIA>\**`, `<ESQUEMAS_O_MIGRACIONES_BASE>`, documentación de arquitectura en `<RUTA_DOCS>`, `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\verificacion\` (si aplica) y entregables en `[x] hecho`
- Prohibido: Refactorizar módulos ajenos a `<RUTA_MODULO_NUEVO>`, alterar bases de datos de producción o desarrollo real sin script/migración controlada en `<ENTORNO_PRUEBAS>`, modificar ramas protegidas (`<RAMA_PROTEGIDA>`), exponer secretos en código o documentación, ejecutar `git reset --hard` o `git clean`, modificar entregables en `[x] hecho`, `workspace\qa\` o `workspace\verificacion\`
- Si un chunk exige salirse del alcance, el Chef marca [!] y NO lo ejecuta.

## Directorio de Destino Real
- Repositorio destino: `<REPO_DESTINO>` (rama `<RAMA_TRABAJO>`)
- Ruta del módulo nuevo: `<REPO_DESTINO>\<RUTA_MODULO_NUEVO>`
- Suite de pruebas del módulo: `<REPO_DESTINO>\<CARPETA_PRUEBAS_MODULO>`

## Supuestos y Stack Tecnológico
- **Lenguaje / Runtime:** `<LENGUAJE_Y_VERSION>`
- **Framework Web / API:** `<FRAMEWORK_WEB>` (registro en `<ARCHIVO_REGISTRO_MODULO>`)
- **UI / Plantillas (si aplica):** `<MOTOR_UI_O_PLANTILLAS>`
- **Persistencia / ORM:** `<BASE_DE_DATOS_Y_ORM>` (sobre `<ENTORNO_PRUEBAS>`)
- **Calidad y Testing:** `<FRAMEWORK_PRUEBAS>` y `<LINTER_O_BUILD>`
- **Máx. intentos por chunk:** 3

## Árbol de Carpetas Esperado
```text
<REPO_DESTINO>\
├── <ARCHIVO_REGISTRO_MODULO>             (registro del nuevo módulo/blueprint)
├── <RUTA_MODULO_NUEVO>\
│   ├── <ARCHIVO_ENTRADA_O_RUTAS>
│   ├── <ARCHIVO_MODELOS_O_SERVICIOS>
│   └── <SUBCARPETA_VISTAS_O_COMPONENTES>\
└── <CARPETA_PRUEBAS_MODULO>\
    └── <ARCHIVO_PRUEBAS_MODULO>
```

## Contratos entre Pasos
- **Chunk 01 (Exploración y Diseño) -> Chunks 02 y 03:** Define contratos de rutas, payloads, modelos y convenciones observadas en `<REPO_DESTINO>`.
- **Chunk 02 (Datos / Modelos) -> Chunk 03:** Entrega entidades, esquemas y validaciones listos para consumirse desde controladores/endpoints.
- **Chunks 02 y 03 -> Chunk 04:** Exponen la lógica y endpoints completos para cobertura de pruebas automatizadas.
- **Chunks 01 a 04 -> Chunk 05 (Integración):** Entregan código, pruebas y registros listos para regresión global y verificación final.

## Estructura Típica de Chunks (Guía de Segmentación y Verificación)
*(Ciclo de anillo, `Modo QA` y decisiones del `[AUTOR]` según `gengis_swap_common.md`; todo chunk requiere **Prueba de que puede fallar**.)*
1. **`01_exploracion_y_contratos.md`:** Inspeccionar en solo lectura `<REPO_DESTINO>` y documentar contratos técnicos del módulo en `workspace\diseno_<NOMBRE_MODULO>.md`.
   - *Criterio de aceptación:* Contratos alineados a convenciones reales del proyecto; cero modificaciones en código.
   - *Verificación:* `git -C "<REPO_DESTINO>" status` (debe permanecer limpio) y lectura del documento en `workspace\`.
   - *Prueba de que puede fallar:* Modificar cualquier archivo dentro de `<REPO_DESTINO>` u omitir el encabezado **G2** en `workspace\diseno_<NOMBRE_MODULO>.md` haría fallar el chequeo de limpieza o formato.
2. **`02_modelo_y_capa_datos.md`:** Crear modelos, esquemas o servicios base en `<RUTA_MODULO_NUEVO>\<ARCHIVO_MODELOS_O_SERVICIOS>`.
   - *Criterio de aceptación:* Modelos consistentes con `<BASE_DE_DATOS_Y_ORM>` sin mutar BD real.
   - *Verificación:* `<COMANDO_VALIDACION_MODELOS_O_MIGRACIONES>`
   - *Prueba de que puede fallar:* Introducir un campo con tipo/clave foránea inválida o error de importación en `<ARCHIVO_MODELOS_O_SERVICIOS>` hace fallar `<COMANDO_VALIDACION_MODELOS_O_MIGRACIONES>`.
3. **`03_rutas_logica_y_vistas.md`:** Implementar endpoints/controladores, vistas y registro en `<ARCHIVO_REGISTRO_MODULO>`.
   - *Criterio de aceptación:* Rutas accesibles con validación de entradas y permisos acordes al sistema.
   - *Verificación:* `<COMANDO_LINTER_O_BUILD>`
   - *Prueba de que puede fallar:* Dejar sin registrar `<ARCHIVO_REGISTRO_MODULO>` o introducir un error de sintaxis/lint rompe `<COMANDO_LINTER_O_BUILD>` con código distinto de `0`.
4. **`04_pruebas_automatizadas.md`:** Escribir y ejecutar pruebas unitarias/integración/E2E en `<CARPETA_PRUEBAS_MODULO>`.
   - *Criterio de aceptación:* Casos principales y de error cubiertos y en verde sobre `<ENTORNO_PRUEBAS>`.
   - *Verificación:* `<COMANDO_PRUEBAS_MODULO>`
   - *Prueba de que puede fallar:* Invertir un código de estado esperado o romper la lógica de validación de entrada en un endpoint provoca el fallo de `<COMANDO_PRUEBAS_MODULO>`.
5. **`05_integracion_y_regresion.md` (Integración):** Verificar integración completa en `<RAMA_TRABAJO>`, correr suite de regresión general y revisar `git diff` contra `## Alcance`.
   - *Criterio de aceptación:* Suite completa sin fallos ni regresiones; solo archivos autorizados modificados.
   - *Verificación:* `<COMANDO_SUITE_COMPLETA>` y `git -C "<REPO_DESTINO>" status`
   - *Prueba de que puede fallar:* Romper un endpoint existente o tocar archivos fuera de `<RUTA_MODULO_NUEVO>` provoca fallo en `<COMANDO_SUITE_COMPLETA>` o en el cotejo de alcance.

## Definición de Terminado (DoD)
- Módulo `<NOMBRE_MODULO>` implementado, registrado en `<ARCHIVO_REGISTRO_MODULO>` y funcionando en `<ENTORNO_PRUEBAS>`.
- Pruebas del módulo (`<COMANDO_PRUEBAS_MODULO>`) y regresión (`<COMANDO_SUITE_COMPLETA>`) ejecutadas exitosamente con evidencia registrada.
- Todos los cambios confinados a `<RAMA_TRABAJO>` y a las rutas permitidas en `## Alcance`.
