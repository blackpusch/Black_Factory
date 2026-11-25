---
name: reviewer-ui
description: Subagente independiente de UI Visual (modelo de referencia Astra con computer use) para validar visualmente interfaces gráficas, estados, diseño responsive y accesibilidad en contexto limpio.
version: "1.0"
owner: 4_Revisores
---

# Skill: `reviewer-ui` (Auditoría Visual de UI con *Computer Use* en Contexto Limpio)

## Rol y Modelo Objetivo

* **Eje (`axis`):** `ui` (`profile_role: "reviewer_ui"`).
* **Modelo de referencia en harness:** **Astra** (entrenado con aprendizaje por refuerzo y enfocado en *computer use*).
* **Activación:** condicional según `applicability.applicable` en `packets/ui.packet.json` (archivos/rutas GUI o bandera aditiva `--include-ui`).
  * Si `applicable == false`, **no invocar modelo**; usar el dictamen determinista `status: "NA"` pre-emitido por `reviewer.exe prepare`.

## Entradas Permitidas (Prohibido Leer `3_Builder/runtime/` o Historial del Builder)

1. `packets/ui.packet.json` + copia aislada del `.diff` dentro de `4_Revisores/runtime/` (`git.diff_rel_path`) + archivos de vistas/componentes en el `worktree`.
2. Evidencia visual de ejecución real:
   * Capturas raster (`.png`, `.jpg`, `.jpeg`, `.webp`) listadas en `ui_evidence_refs[]` y copiadas por `prepare` dentro de `4_Revisores/runtime/<task_id>/<bundle_id>/ui_evidence/`.
   * Y/o sesión interactiva *computer use* sobre la superficie visual efímera (`G3`) cuando `ui_live_surface == true`.

## Checklist de Auditoría Obligatoria

1. **Validación Visual Real (*Computer Use*):** inspeccionar la interfaz renderizada verificando alineación, solapamientos, recortes de texto, jerarquía visual y ausencia de errores en consola/render.
2. **Estados Completos:** verificar estado inicial/vacío, carga, éxito, error legible y recuperación sin bloquear la pantalla.
3. **Diseño Responsive y Layouts:** comprobar comportamiento en escritorio y pantallas estrechas sin scroll horizontal roto ni controles inacclesibles.
4. **Accesibilidad (`a11y`):** contraste suficiente, etiquetas asociadas a controles, foco visible y navegación por teclado.
5. **Adherencia a Directrices de `0_Brain`:** verificar reglas visuales o de diseño registradas en `brain_context.matched_docs[]`.

## Contrato de Salida (`review_verdict.schema.json`)

* Si `applicable == true`, **prohibido emitir `NA`** (solo `PASS` o `FAIL`, y `model_family` distinto de `deterministic` / `anthropic-opus`), y debe poblar `ui_evidence_used[]` (mínimo `1` elemento, máximo `17`):
  * Si `ui_evidence_refs[]` no está vacío: incluir al menos las referencias usadas de `ui_evidence_refs[]` (rutas relativas `ui_evidence/<relPath>`, sin rutas de host `G4`).
  * Si `ui_live_surface == true` y `ui_evidence_refs[]` está vacío: incluir `"live_computer_use_session"` (junto a cualquier referencia relativa adicional si aplica).
* Cada hallazgo en `findings[]` debe citar archivo/línea de componente o vista verificada (`G5`), `evidence_quote` literal no vacío y acción concreta de corrección.
* Validar el archivo staging siempre con `reviewer.exe validate`.
