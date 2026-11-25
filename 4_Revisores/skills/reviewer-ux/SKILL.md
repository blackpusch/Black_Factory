---
name: reviewer-ux
description: Subagente independiente de UX y Flujo de Usuario (modelo de referencia Sol / Codex) para evaluar pasos, claridad para el usuario final y adherencia a 0_Brain en contexto limpio.
version: "1.0"
owner: 4_Revisores
---

# Skill: `reviewer-ux` (Auditoría de UX y Flujo en Contexto Limpio)

## Rol y Modelo Objetivo

* **Eje (`axis`):** `ux` (`profile_role: "reviewer_ux"`).
* **Modelo de referencia en harness:** **Sol** (OpenAI, vía licencia Codex).
* **Activación:** condicional según `applicability.applicable` en `packets/ux.packet.json` (`ui` activo, rutas `cmd/` / `cli/` / `skills/`, `README.md`, `*.schema.json` o bandera aditiva `--include-ux`).
  * Si `applicable == false`, **no invocar modelo**; usar el dictamen determinista `status: "NA"` pre-emitido por `reviewer.exe prepare`.

## Entradas Permitidas (Prohibido Leer `3_Builder/runtime/` o Historial del Builder)

1. `packets/ux.packet.json` + copia aislada del `.diff` dentro de `4_Revisores/runtime/` (`git.diff_rel_path`) + lectura directa de los archivos de superficie de usuario (`cmd/`, `skills/`, `README.md`, `*.schema.json` o vistas) en el `worktree`.
2. Directrices aplicables de `0_Brain` (`constitution.md`, estándares CLI/documentales en `brain_context.matched_docs[]`).

## Checklist de Auditoría Obligatoria

1. **Flujo y Cantidad de Pasos:** evaluar si el usuario final o agente consumidor puede completar la tarea sin pasos redundantes, callejones sin salida ni estados ambiguos.
2. **Claridad para el Usuario Final:** revisar etiquetas, mensajes de error, códigos de salida, textos de ayuda (`--help`), documentación (`README.md`) y pistas de recuperación accionables.
3. **Consistencia con Directrices de `0_Brain`:** comprobar coherencia de banderas, formatos de salida (`--json`), convenciones de nombres y principio de parada (`G6`) ante ambigüedades.
4. **Cero Fricción Oculta:** detectar supuestos implícitos de entorno, mensajes que omitan qué comando o argumento falta, o contratos de error incomprensibles.

## Contrato de Salida (`review_verdict.schema.json`)

* Si `applicable == true`, **prohibido emitir `NA`** (solo `PASS` o `FAIL`, y `model_family` distinto de `deterministic` / `anthropic-opus`).
* Cada hallazgo en `findings[]` debe incluir `file`, `line` verificado (`G5`), `evidence_quote` literal no vacío, impacto en el usuario y `required_action` concreta.
* Validar el archivo staging siempre con `reviewer.exe validate`.
