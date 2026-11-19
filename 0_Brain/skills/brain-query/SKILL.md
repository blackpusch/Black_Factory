---
name: brain-query
description: Consulta progresiva Read-Before-Act de la bóveda OKF usando brain.exe (index, query FTS5 y context por covers_paths) respetando G1-G6 y Opción i.
version: "1.0"
owner: 0_Brain
---

# Skill: `brain-query` (Lectura Progresiva y Búsqueda Rápida)

## Cuándo Cargar Esta Skill

Cargar **siempre antes de planificar, redactar especificaciones, modificar código o auditar una entrega** (protocolo *Read-Before-Act*). Evita lecturas ciegas del repositorio completo sustituyéndolas por carga progresiva y búsqueda determinista en SQLite FTS5.

## Secuencia Obligatoria *Read-Before-Act*

Seguir este orden estricto dentro de la bóveda (`<RUTA_VAULT>`, por defecto `0_Brain/vault`):

1. Leer `AGENTS.md` para confirmar el enrutamiento del equipo activo.
2. Leer `index.md` para ubicar proyectos, ADRs y páginas de wiki existentes.
3. Leer `constitution.md` para cargar principios e invariantes obligatorias.
4. Leer `wiki/risk_zones.md` para detectar zonas críticas sujetas a compuerta `[AUTOR]`.
5. Ejecutar consultas focalizadas con `brain.exe query` y `brain.exe context`.

## Comandos Operativos (`brain.exe`)

Si la base local `.index/brain.db` no existe o fue actualizada manualmente tras aprobar cambios, reconstruir primero el índice atómico:

```text
0_Brain\brain.exe index --vault <RUTA_VAULT>
```

### Búsqueda Full-Text (SQLite FTS5 + BM25)

```text
0_Brain\brain.exe query --vault <RUTA_VAULT> "<consulta>" [--project <PROYECTO>] [--type <TIPO>] [--tag <TAG>] [--limit <N>] [--json]
```

- `--type`: filtrar por `constitution`, `risk_zones`, `adr`, `wiki`, `index` o `agents`.
- `--project`: acotar al proyecto objetivo (ejemplo: `black-factory`).
- `--limit`: limitar cantidad de resultados ordenados por relevancia BM25.
- `--json`: salida estructurada para consumo por pipelines automatizados.

### Contexto por Archivos Modificados (`covers_paths`)

Antes de tocar o revisar rutas concretas del repositorio, cruzar los archivos contra `covers_paths` (incluye automáticamente documentos transversales `constitution` y `risk_zones`):

```text
0_Brain\brain.exe context --vault <RUTA_VAULT> --project <PROYECTO> --files <ruta1,ruta2>
```

## Cumplimiento Obligatorio (`G1–G6`) y Privacidad (`Opción i`)

- **G1:** al citar resultados de `query` o `context` en una especificación o revisión, incluir el comando ejecutado, el código de salida y los fragmentos literales relevantes.
- **G2:** si el resultado de la consulta alimenta artefactos de `workspace\`, incluir el encabezado de ubicación vigente.
- **G3:** `query` y `context` son de solo lectura sobre la bóveda e índice derivado; no alteran datos reales del proyecto objetivo.
- **G4:** si una consulta devolviera accidentalmente una credencial, token HTTP, API key, clave privada PEM/SSH, correo sensible o ruta personal de usuario, no transcribir el valor; reportar inmediatamente el hallazgo y activar `brain-verify`.
- **G5:** contrastar siempre la documentación recuperada de `0_Brain` con el código real (`archivo:línea`), separando `HECHOS` verificados de `HIPÓTESIS`.
- **G6:** si los resultados revelan contradicción con `constitution.md` o `wiki/risk_zones.md`, detener la tarea y escalar al `[AUTOR]`.
- **Privacidad `Opción i`:** nunca copiar archivos de `0_Brain/vault/` ni `.index/brain.db` hacia rutas trackeadas por Git.
