# Plantilla: correccion_hallazgos v1

# Mapa de Tarea: <ID_DE_LA_TAREA> — Corrección de Hallazgos de Auditoría (<LISTA_HALLAZGOS>)
Plantilla: correccion_hallazgos v1

**Objetivo genérico:** Remediar hallazgos priorizados de auditoría (`<LISTA_HALLAZGOS>`, p. ej. accesibilidad, cobertura E2E de mutación, guards de entorno o rendimiento) en una rama dedicada y entorno efímero aislado, demostrando mediante hash SHA256 que `<RUTA_BD_REAL>` permanece intacta y generando `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md`.

**Reglas globales aplicables:** Acatamiento obligatorio de las reglas globales **G1–G6** de `gengis_swap_common.md` (no copiar su texto aquí).

## Alcance
- Puede modificar: `<REPO_APLICACION>\<RUTAS_CODIGO_O_TEMPLATES>`, `<REPO_APLICACION>\<CARPETA_PRUEBAS_EFIMERAS>\**`, `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md` (exclusivamente sobre la rama `<RAMA_TRABAJO>`)
- Mutación de datos (G3): únicamente `<BD_EFIMERA>` en puerto efímero `<PUERTO_EFIMERO>`
- Solo lectura: `<REPO_UMBRELLA_O_AJENOS>\**`, `<RUTA_BD_REAL>`, ramas de otros frentes (`<RAMA_PROTEGIDA>`), `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\verificacion\` (si aplica) y entregables en `[x] hecho`
- Prohibido: Mutar `<RUTA_BD_REAL>`, ejecutar pruebas de mutación fuera de `<PUERTO_EFIMERO>` / `<BD_EFIMERA>`, mezclar cambios o commits de `<RAMA_PROTEGIDA>`, ejecutar `git add .`, `git add -A`, `git reset --hard`, `git push --force` o `git clean`, modificar entregables en `[x] hecho`, `workspace\qa\` o `workspace\verificacion\`
- Si un chunk exige salirse del alcance, el Chef marca [!] y NO lo ejecuta.

## Directorio de Destino Real
- Repositorio de implementación: `<REPO_APLICACION>` (rama `<RAMA_TRABAJO>`)
- Suite y entorno efímero: `<REPO_APLICACION>\<CARPETA_PRUEBAS_EFIMERAS>\` (puerto `<PUERTO_EFIMERO>`, BD `<BD_EFIMERA>`)
- Reporte final consolidado: `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md`

## Supuestos y Stack Tecnológico
- **Backend / Servidor local:** `<STACK_BACKEND>` (ej. Python `<VERSION_PYTHON>`, Flask, Waitress en puerto `<PUERTO_EFIMERO>`).
- **Frontend / Plantillas:** `<STACK_FRONTEND>` (ej. Jinja2, Bootstrap, CSS).
- **Testing y Auditoría:** Playwright (`@playwright/test`), `@axe-core/playwright`, Lighthouse CLI, PowerShell (`Get-FileHash SHA256`), Git.
- **Aislamiento estricto:** Las pruebas mutantes operan únicamente con `<BD_EFIMERA>` y guards de seguridad activos que abortan si la URL o BD no corresponden al entorno efímero.
- **Máx. intentos por chunk:** 3

## Árbol de Carpetas Esperado
```text
<REPO_APLICACION>\
├── <RUTAS_CODIGO_O_TEMPLATES>\          (cambios puntuales por hallazgo en <RAMA_TRABAJO>)
├── <CARPETA_PRUEBAS_EFIMERAS>\
│   ├── package.json
│   ├── playwright.config.js
│   ├── <SCRIPT_SEED_EFIMERO>
│   ├── <SCRIPT_VERIFICACION_AISLAMIENTO>
│   └── tests\
│       ├── <SPEC_MUTACIONES>.spec.js
│       └── <SPEC_A11Y>.spec.js
└── <NOMBRE_REPORTE>.md                  (si reside en la raíz además de <RUTA_REPORTE_FINAL>)
```

## Contratos entre Pasos
- **Chunk 01 (Ubicación / Aislamiento) -> Chunk 02:** Confirma rama `<RAMA_TRABAJO>`, ausencia de mezcla con `<RAMA_PROTEGIDA>` y estado limpio o pendiente controlado.
- **Chunk 02 (Entorno Efímero y Guards) -> Chunk 03 y 04:** Garantiza servidor efímero en `<PUERTO_EFIMERO>` con `<BD_EFIMERA>` y guards que abortan ante URL/BD reales.
- **Chunk 03 (Correcciones) -> Chunk 04:** Entrega ajustes puntuales en templates/código listos para re-verificación.
- **Chunks 04 y 05 -> Chunk 06 (Integración):** Producen salidas de suite `<COMANDO_SUITE_EFIMERA>`, hashes SHA256 de `<RUTA_BD_REAL>`, capturas Antes/Después y mediciones comparables de rendimiento para el reporte final.

## Estructura Típica de Chunks (Guía de Segmentación y Verificación)
*(Ciclo de anillo, `Modo QA` y decisiones del `[AUTOR]` según `gengis_swap_common.md`; todo chunk requiere **Prueba de que puede fallar**.)*
1. **`01_ubicacion_y_rama.md`:** Verificar repositorio, rama `<RAMA_TRABAJO>`, commits respecto a `<RAMA_PROTEGIDA>` e integridad inicial de `<RUTA_BD_REAL>`.
   - *Verificación:* `git -C "<REPO_APLICACION>" status && git -C "<REPO_APLICACION>" branch --show-current`
   - *Prueba de que puede fallar:* Estar posicionado en `<RAMA_PROTEGIDA>` o tener archivos modificados sin control fuera de `<RAMA_TRABAJO>` hace fallar la validación de rama/estado.
2. **`02_entorno_efimero_y_guards.md`:** Implementar o verificar `<SCRIPT_SEED_EFIMERO>`, guard de puerto `<PUERTO_EFIMERO>` y `<SCRIPT_VERIFICACION_AISLAMIENTO>` (SHA256 antes/después).
   - *Verificación:* `<COMANDO_PRUEBA_GUARD_Y_AISLAMIENTO>` (debe abortar al apuntar a puerto/BD real y mantener hash SHA256 idéntico).
   - *Prueba de que puede fallar:* Apuntar la variable de entorno al puerto o a `<RUTA_BD_REAL>` debe provocar aborto inmediato (o disparar falla si el guard no detiene la ejecución).
3. **`03_correccion_hallazgos_codigo.md`:** Aplicar fixes mínimos por hallazgo (`<LISTA_HALLAZGOS>`, ej. `<main>`/skip-link, contraste/encabezados, rutas/plantillas) dentro de `Puede modificar`.
   - *Verificación:* `git -C "<REPO_APLICACION>" diff --stat` (solo archivos autorizados) y pruebas de sintaxis/lint locales.
   - *Prueba de que puede fallar:* Introducir un error de sintaxis en la plantilla/archivo editado o tocar un archivo fuera de `<RUTAS_CODIGO_O_TEMPLATES>` hace fallar el lint o la comprobación de alcance.
4. **`04_verificacion_e2e_a11y.md`:** Ejecutar la suite unificada E2E de mutaciones y Axe en `<VIEWPORT_DESKTOP>` y `<VIEWPORT_MOBILE>`, más verificación de hashes SHA256 de `<RUTA_BD_REAL>`.
   - *Criterio de aceptación:* Salida completa sin truncar, conteo de tests pasados/fallidos y hashes SHA256 idénticos antes/después.
   - *Verificación:* `cd "<REPO_APLICACION>\<CARPETA_PRUEBAS_EFIMERAS>" && <COMANDO_SUITE_EFIMERA> && <COMANDO_HASH_BD>`
   - *Prueba de que puede fallar:* Revertir temporalmente un fix de accesibilidad o mutar un byte de `<BD_EFIMERA>`/`<SPEC_MUTACIONES>` falla la aserción E2E/Axe o cambia el conteo de fallos.
5. **`05_evidencia_visual_y_performance.md`:** Generar capturas Antes/Después (Desktop/Mobile) y correr mediciones comparables de Lighthouse (`<N_CORRIDAS>` corridas Antes vs Después sobre `<PUERTO_EFIMERO>`).
   - *Verificación:* Comprobar artefactos en `workspace\` y reportes JSON/HTML generados.
   - *Prueba de que puede fallar:* Detener el servidor de `<PUERTO_EFIMERO>` o borrar una corrida Lighthouse hace fallar la generación del reporte JSON o su conteo esperado.
6. **`06_reporte_final_integracion.md` (Integración):** Consolida `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md` con estado real por hallazgo (`resuelto` / `parcial` / `no resuelto`), métricas, hashes y comandos reproducibles (**Regla R2**: regenerar completo desde cero).
   - *Verificación:* `Test-Path "<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md"` y verificación de SHA256 final de `<RUTA_BD_REAL>`.
   - *Prueba de que puede fallar:* Omitir `<NOMBRE_REPORTE>.md` devuelve `False` en `Test-Path` y cualquier discrepancia entre el hash SHA256 inicial y final de `<RUTA_BD_REAL>` rompe el chequeo de integridad.

## Definición de Terminado (DoD)
- Todos los hallazgos en `<LISTA_HALLAZGOS>` atendidos o diagnosticados con evidencia numérica verificable (cero afirmaciones sin salida de comando o captura).
- Suite `<COMANDO_SUITE_EFIMERA>` y pruebas Axe ejecutadas contra `<PUERTO_EFIMERO>` / `<BD_EFIMERA>` con hash SHA256 de `<RUTA_BD_REAL>` idéntico antes y después.
- Trabajo aislado en `<RAMA_TRABAJO>` sin contaminación de `<RAMA_PROTEGIDA>`.
- `<RUTA_REPORTE_FINAL>\<NOMBRE_REPORTE>.md` generado aplicando **R2**, con estado real por hallazgo y comandos exactos de reproducción.
