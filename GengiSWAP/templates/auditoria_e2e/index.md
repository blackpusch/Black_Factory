# Plantilla: auditoria_e2e v1

# Mapa de Tarea: <ID_DE_LA_TAREA> — Auditoría E2E Caja Negra (<MODULO_O_ENDPOINT>)
Plantilla: auditoria_e2e v1

**Objetivo genérico:** Ejecutar una auditoría E2E de caja negra (funcional, visual responsive, accesibilidad y rendimiento) contra `<URL_OBJETIVO>` desde un directorio externo aislado, interceptando mutaciones de red para dejar el repositorio fuente y la base de datos intactos, y consolidar los hallazgos en `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md`.

**Reglas globales aplicables:** Acatamiento obligatorio de las reglas globales **G1–G6** de `gengis_swap_common.md` (no copiar su texto aquí).

## Alcance
- Puede modificar: `<DIRECTORIO_EXTERNO_AUDITORIA>\**`, `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md`
- Mutación de datos (G3): ninguna (envíos mutantes interceptados con `page.route()`)
- Solo lectura: `<REPO_APLICACION>\**`, `<URL_OBJETIVO>` (solo navegación/GET; formularios y mutaciones POST/PUT/DELETE interceptados vía `page.route()`), `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\verificacion\` (si aplica) y entregables en `[x] hecho`
- Prohibido: Instalar paquetes Node o crear archivos de pruebas dentro de `<REPO_APLICACION>`, modificar código fuente o configuración del proyecto auditado, enviar peticiones mutantes reales contra `<BD_OBJETIVO>`, modificar entregables en `[x] hecho`, `workspace\qa\` o `workspace\verificacion\`
- Si un chunk exige salirse del alcance, el Chef marca [!] y NO lo ejecuta.

## Directorio de Destino Real
- Entorno aislado de auditoría (untracked): `<DIRECTORIO_EXTERNO_AUDITORIA>`
- Aplicación auditada (solo lectura): `<REPO_APLICACION>` en `<URL_OBJETIVO>`
- Reporte final consolidado: `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md`

## Supuestos y Stack Tecnológico
- **Runtime y Herramientas:** Node.js `<VERSION_NODE>`, `@playwright/test`, `@axe-core/playwright`, `lighthouse`, Git (solo verificación de estado limpio).
- **Tipo de prueba:** E2E caja negra en navegador con `trace: 'on'` y captura automática ante fallos en `<VIEWPORT_DESKTOP>` (ej. `1366x768`) y `<VIEWPORT_MOBILE>` (ej. `390x844`).
- **Aislamiento de red:** Toda operación mutante hacia `<ENDPOINTS_MUTANTES>` debe interceptarse y abortarse o simularse (`page.route()`).
- **Máx. intentos por chunk:** 3

## Árbol de Carpetas Esperado
```text
<DIRECTORIO_EXTERNO_AUDITORIA>\
├── package.json
├── playwright.config.js
├── tests\
│   ├── e2e.spec.js
│   ├── a11y.spec.js
│   └── visual.spec.js
├── lighthouse\
│   └── run_lighthouse.js
├── test-results\
└── node_modules\
```
*(Más el entregable final `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md`.)*

## Contratos entre Pasos
- **Chunk 01 (Configuración) -> Chunks 02 y 03:** Inicializa `<DIRECTORIO_EXTERNO_AUDITORIA>` con dependencias y `playwright.config.js` listo para ser consumido por las suites E2E, visuales, Axe y Lighthouse.
- **Chunks 02 y 03 -> Chunk 04 (Integración):** Producen trazas, capturas desktop/mobile, resultados de errores de consola/red, reportes Axe y JSON/HTML de Lighthouse necesarios para redactar el informe final.

## Estructura Típica de Chunks (Guía de Segmentación y Verificación)
*(Ciclo de anillo, `Modo QA` y decisiones del `[AUTOR]` según `gengis_swap_common.md`; todo chunk requiere **Prueba de que puede fallar**.)*
1. **`01_configuracion.md`:** Crea `package.json`, instala Playwright/Axe/Lighthouse y genera `playwright.config.js` (`trace: 'on'`, viewports `<VIEWPORT_DESKTOP>` y `<VIEWPORT_MOBILE>`).
   - *Criterio de aceptación:* Dependencias operativas exclusivamente en `<DIRECTORIO_EXTERNO_AUDITORIA>`; cero cambios en `<REPO_APLICACION>`.
   - *Verificación:* `cd "<DIRECTORIO_EXTERNO_AUDITORIA>" && npx playwright test --help`
   - *Prueba de que puede fallar:* Eliminar o corromper `playwright.config.js` / la dependencia `@playwright/test` hace fallar `npx playwright test --help` con código distinto de `0`.
2. **`02_pruebas_e2e_playwright.md`:** Implementa y corre `tests\e2e.spec.js` (y `visual.spec.js` si aplica) verificando flujos `<FLUJOS_A_PROBAR>`, interceptación `page.route()`, errores de consola/red (`4xx`/`5xx`) y responsive.
   - *Criterio de aceptación:* Flujos ejecutados sin mutar datos reales; capturas y trazas generadas.
   - *Verificación:* `cd "<DIRECTORIO_EXTERNO_AUDITORIA>" && npx playwright test tests/e2e.spec.js`
   - *Prueba de que puede fallar:* Apuntar a una ruta inexistente o romper una aserción de selector/código HTTP en `tests/e2e.spec.js` hace fallar la ejecución de Playwright.
3. **`03_accesibilidad_rendimiento.md`:** Ejecuta `tests\a11y.spec.js` con `@axe-core/playwright` y `lighthouse\run_lighthouse.js` en Desktop y Mobile guardando resultados locales.
   - *Criterio de aceptación:* Puntajes Lighthouse (Performance, Accessibility, Best Practices, SEO) y violaciones Axe extraídos localmente.
   - *Verificación:* `cd "<DIRECTORIO_EXTERNO_AUDITORIA>" && npx playwright test tests/a11y.spec.js && node lighthouse/run_lighthouse.js`
   - *Prueba de que puede fallar:* Omitir la escritura del reporte JSON de Lighthouse o simular un URL inválido hace fallar el script o el test de `a11y.spec.js` con código distinto de `0`.
4. **`04_integracion_reporte.md` (Integración):** Redacta `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md` con resumen ejecutivo, hallazgos por gravedad, tabla de flujos y limpieza del repositorio (**Regla R2**: regenerar completo desde cero).
   - *Criterio de aceptación:* Informe regenerado desde cero con evidencia verificable y `git status` limpio en `<REPO_APLICACION>`.
   - *Verificación:* `git -C "<REPO_APLICACION>" status`
   - *Prueba de que puede fallar:* Crear o modificar cualquier archivo temporal dentro de `<REPO_APLICACION>` provocaría que `git status` reporte el árbol sucio (`untracked`/`modified`).

## Definición de Terminado (DoD)
- Todas las pruebas E2E (`<FLUJOS_A_PROBAR>`), responsive (`<VIEWPORT_DESKTOP>` / `<VIEWPORT_MOBILE>`), accesibilidad (Axe) y rendimiento (Lighthouse) ejecutadas con evidencia local en `<DIRECTORIO_EXTERNO_AUDITORIA>`.
- `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md` generado aplicando **R2**, con hallazgos priorizados, tabla de flujos E2E, bloqueos/sospechas y orden sugerido de corrección.
- Cero archivos creados, modificados o eliminados en `<REPO_APLICACION>`, comprobado vía `git -C "<REPO_APLICACION>" status`.
