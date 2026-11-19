# Plantilla: mapeo_estatico v1

# Mapa de Tarea: <ID_DE_LA_TAREA> — Mapa Estático de <APLICACION_O_MODULO>
Plantilla: mapeo_estatico v1

**Objetivo genérico:** Construir de forma 100% estática y de solo lectura el mapa integral de `<APLICACION_O_MODULO>` (estructura, inventario total de rutas, matriz de autenticación/roles/CSRF, modelo de datos, cobertura de pruebas y matriz priorizada de huecos) con citas exactas `archivo:línea`, generando los entregables consolidados en `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\<CARPETA_SALIDA>\`.

**Reglas globales aplicables:** Acatamiento obligatorio de las reglas globales **G1–G6** de `gengis_swap_common.md` (no copiar su texto aquí).

## Alcance
- Puede modificar: `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\<CARPETA_SALIDA>\**`
- Mutación de datos (G3): ninguna (análisis 100% estático y de solo lectura)
- Solo lectura: `<REPO_PRINCIPAL>\**`, `<REPO_SECUNDARIO_O_PRUEBAS>\**`, archivos de configuración, plantillas y scripts de seed (sin transcribir credenciales reales), `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\verificacion\` (si aplica) y entregables en `[x] hecho`
- Prohibido: Modificar código fuente, plantillas, tests o configuración en `<REPO_PRINCIPAL>` o `<REPO_SECUNDARIO_O_PRUEBAS>`, ejecutar comandos Git de escritura (`commit`, `add`, `checkout`, `switch`, `stash`, `reset`), abrir o mutar `<BD_OBJETIVO>`, transcribir secretos/tokens/cadenas de conexión reales, enviar peticiones HTTP mutantes, modificar entregables en `[x] hecho`, `workspace\qa\` o `workspace\verificacion\`
- Si un chunk exige salirse del alcance, el Chef marca [!] y NO lo ejecuta.

## Directorio de Destino Real
- Repositorio principal analizado (solo lectura): `<REPO_PRINCIPAL>` (módulo `<RUTA_MODULO>`, rama `<RAMA_REFERENCIA>`)
- Repositorio / entorno complementario si aplica (solo lectura): `<REPO_SECUNDARIO_O_PRUEBAS>`
- Entregables consolidados: `<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\<CARPETA_SALIDA>\`

## Supuestos y Stack Tecnológico
- **Modalidad:** Análisis estático 100% de solo lectura; todo dato afirmado debe citar `archivo:línea` y distinguir hechos observados en código de hipótesis inferidas.
- **Stack Backend:** `<STACK_BACKEND>` (ej. Python/Flask, Blueprint `<NOMBRE_BLUEPRINT>`, SQLAlchemy/ORM).
- **Stack Frontend / UI:** `<STACK_FRONTEND>` (ej. Jinja2, Bootstrap).
- **Seguridad y Testing analizados:** `<MECANISMOS_AUTH_CSRF>` y suites `<RUTA_SUITES_PRUEBAS>` (Playwright, Axe, Lighthouse).
- **Máx. intentos por chunk:** 3

## Árbol de Carpetas Esperado
```text
<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\
├── index.md
├── status.md
├── chunks\
│   ├── 01_m1_ubicacion_estructura.md
│   ├── 02_m2_rutas_inventario.md
│   ├── 03_m3_seguridad_permisos.md
│   ├── 04_m4_modelo_datos.md
│   ├── 05_m5_cobertura_verificacion.md
│   ├── 06_m6_matriz_huecos.md
│   └── 07_m7_entregables_integracion.md
└── workspace\
    ├── m1_ubicacion_estructura.md
    ├── m2_rutas_inventario.md
    ├── m3_seguridad_permisos.md
    ├── m4_modelo_datos.md
    ├── m5_cobertura_verificacion.md
    ├── m6_matriz_huecos.md
    └── <CARPETA_SALIDA>\
        ├── UBICACION.md
        ├── <MAPA_GENERAL>.md
        ├── <MATRIZ_COBERTURA>.csv
        ├── <HUECOS_PRIORIZADOS>.md
        └── <DIAGRAMA>.d2 | <DIAGRAMA>.drawio.svg (si se generan diagramas independientes en workspace\)
```

## Contratos entre Pasos
- **M1 -> M2:** Fija rutas físicas en disco, ramas, `HEAD`, entrypoints y blueprints para delimitar el conjunto exacto de archivos fuente.
- **M2 -> M3, M4 y M5:** Entrega el inventario maestro consolidado de rutas (URL, métodos HTTP, función controladora y `archivo:línea`) para cruzar seguridad/CSRF (M3), modelos/tablas CRUD (M4) y cobertura de tests (M5).
- **M1 a M5 -> M6:** Consolida hallazgos multidimensionales para catalogar y priorizar huecos con baremo objetivo.
- **M1 a M6 -> M7 (Integración):** Sintetiza todos los análisis en `<CARPETA_SALIDA>\` cumpliendo **G2** (`UBICACION.md` para `.csv` y diagramas) sin alterar repositorios de desarrollo.

## Estructura Típica de Chunks (Guía de Segmentación y Verificación)
*(Ciclo de anillo, `Modo QA` y decisiones del `[AUTOR]` según `gengis_swap_common.md`; todo chunk requiere **Prueba de que puede fallar**.)*
1. **`01_m1_ubicacion_estructura.md`:** Verificar rutas en disco, ramas, `HEAD`, blueprints y dependencias.
   - *Verificación:* `git -C "<REPO_PRINCIPAL>" rev-parse --show-toplevel && git -C "<REPO_PRINCIPAL>" branch --show-current && git -C "<REPO_PRINCIPAL>" rev-parse --short HEAD`
   - *Prueba de que puede fallar:* Omitir el encabezado de ubicación (Repo/Rama/HEAD) en `workspace\m1_ubicacion_estructura.md` o apuntar a una ruta fuera de `<REPO_PRINCIPAL>` hace fallar la validación contra la salida literal de Git.
2. **`02_m2_rutas_inventario.md` (subdivisible en `m2a`/`m2b` si hay >50 rutas):** Extraer 100% de rutas con URL, métodos, función y `archivo:línea`.
   - *Verificación:* `Select-String -Path "<REPO_PRINCIPAL>\<RUTA_MODULO>\*.py" -Pattern "<PATRON_DECORADOR_RUTAS>" | Measure-Object`
   - *Prueba de que puede fallar:* Omitir rutas o reportar un conteo distinto al arrojado por `Select-String` sobre los decoradores en `<RUTA_MODULO>` provoca discrepancia detectable de conteo.
3. **`03_m3_seguridad_permisos.md`:** Auditar autenticación, roles, validación de entradas y CSRF por ruta (y diff de `<COMMIT_REFERENCIA>` si existe).
   - *Verificación:* `Select-String -Path "<REPO_PRINCIPAL>\<RUTA_MODULO>\*.py" -Pattern "<PATRON_AUTH_O_ROLES>" | Measure-Object`
   - *Prueba de que puede fallar:* Inventar o omitir un decorador de rol/CSRF respecto a las líneas reales contadas por `Select-String` falla el cotejo directo `archivo:línea`.
4. **`04_m4_modelo_datos.md`:** Catalogar modelos/tablas, relaciones y matriz Tabla × Ruta sobre `<ARCHIVO_MODELOS>`.
   - *Verificación:* `Select-String -Path "<REPO_PRINCIPAL>\<ARCHIVO_MODELOS>" -Pattern "<PATRON_MODELO>" | Measure-Object`
   - *Prueba de que puede fallar:* Citar un modelo/columna inexistente o un total diferente al contado por `Select-String` en `<ARCHIVO_MODELOS>` falla la verificación G5 y de conteo.
5. **`05_m5_cobertura_verificacion.md`:** Cruzar estáticamente rutas/vistas contra `<RUTA_SUITES_PRUEBAS>` (E2E, mutación, Axe, Lighthouse).
   - *Verificación:* `Get-ChildItem -Path "<RUTA_SUITES_PRUEBAS>" -Recurse -File | Select-Object Name, Length`
   - *Prueba de que puede fallar:* Citar un archivo `.spec` inexistente en `<RUTA_SUITES_PRUEBAS>` o reportar cobertura sin cruce con `workspace\m2_rutas_inventario.md` falla la comprobación de existencia.
6. **`06_m6_matriz_huecos.md`:** Clasificar huecos (rutas sin cobertura, mutaciones expuestas, plantillas huérfanas) con criterios de riesgo, impacto y esfuerzo.
   - *Verificación:* Revisión de consistencia de conteos entre `workspace\m2..m6*.md` (script o comando de verificación sobre totales y citas `archivo:línea`).
   - *Prueba de que puede fallar:* Que la suma de hallazgos por severidad o rutas sin cobertura no cuadre con `workspace\m2..m5*.md` hace fallar la verificación de consistencia.
7. **`07_m7_entregables_integracion.md` (Integración):** Generar `<MAPA_GENERAL>.md` (ej. `APP_MAP.md` con diagramas según estándar offline de `gengis_swap_common.md`), `<MATRIZ_COBERTURA>.csv` (1 fila por ruta, acompañado de `UBICACION.md` según **G2**) y `<HUECOS_PRIORIZADOS>.md`, validando limpieza de repos.
   - *Criterio de aceptación:* Filas del CSV iguales al total de rutas inventariadas; `UBICACION.md` y diagramas válidos conforme a **G2**; cero cambios en `<REPO_PRINCIPAL>`.
   - *Verificación:* `(Get-Content "<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\<CARPETA_SALIDA>\<MATRIZ_COBERTURA>.csv").Count`, `Test-Path "<RAIZ_GENGISWAP>gengis_task\<ID_DE_LA_TAREA>\workspace\<CARPETA_SALIDA>\UBICACION.md"` y `git -C "<REPO_PRINCIPAL>" status`
   - *Prueba de que puede fallar:* Que el CSV tenga más o menos filas que el total de rutas de M2, que falte `UBICACION.md` o que `<REPO_PRINCIPAL>` muestre cambios en `git status` hace fallar la verificación.

## Definición de Terminado (DoD)
- Inventario del 100% de rutas de `<APLICACION_O_MODULO>` con `archivo:línea`, matriz de seguridad/CSRF, mapa de datos CRUD y matriz de cobertura automatizada.
- Entregables consolidados `<MAPA_GENERAL>.md`, `<MATRIZ_COBERTURA>.csv` y `<HUECOS_PRIORIZADOS>.md` completos y consistentes en `workspace\<CARPETA_SALIDA>\`.
- Cero modificaciones en archivos, configuración, base de datos o historial Git de los repositorios analizados (`git status` intacto).
