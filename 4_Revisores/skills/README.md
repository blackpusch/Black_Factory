# Skills de `4_Revisores` (Revisión Especializada en Contexto Limpio)

Catálogo de skills modulares y perfiles de orquestación para los **cuatro subagentes revisores independientes** de **Black Factory**, diseñados para operar sin sesgo de autoevaluación tras `3_Builder` (`builder.exe package`).

## Principios Operativos

1. **Contexto Totalmente Limpio (Aislamiento Goldfish):** cada revisor inicia una sesión nueva recibiendo únicamente su `SKILL.md`, su `review_packet.json` (`packets/<axis>.packet.json`), la copia aislada del diff en `4_Revisores/runtime/<task_id>/<bundle_id>/bundle.diff`, acceso de **solo lectura** al `worktree` y documentos de `0_Brain` (`brain.exe context`). **Nunca** accede a `3_Builder/runtime/`, `session.json` ni al historial conversacional del Builder.
2. **Diversidad Obligatoria de Modelos (Anti-Sesgo):** declarada en [profiles.example.json](./profiles.example.json) y verificada deterministamente tanto en `reviewer.exe validate` como en `reviewer.exe seal` mediante `model_id` / `model_family` sin llamadas HTTP desde Go (`Opción 3C` / `NG1`).
3. **Aplicabilidad Determinista y Staging Seguro:** `security` y `code_architecture` son siempre obligatorios; `ux` y `ui` solo ejecutan LLM cuando `applicability.applicable == true` en el `ReviewPacket` (cuando es `false`, `reviewer.exe prepare` pre-emite su veredicto `NA` determinista). Todo dictamen emitido por un subagente se escribe en ruta staging externa a `4_Revisores/runtime/` y pasa por `reviewer.exe validate` antes de promoverse a `verdicts/<axis>.verdict.json`.

## Catálogo de Lentes y Modelos de Referencia

| Skill | Eje (`axis`) | Modelo Referencia | Familia | Foco Principal |
| :--- | :--- | :--- | :--- | :--- |
| [`reviewer-security/SKILL.md`](./reviewer-security/SKILL.md) | `security` | **Sol** (OpenAI / Codex) | `openai-sol` | Vulnerabilidades, inyecciones, permisos, `G4`, reparse/symlinks y `wiki/risk_zones.md` |
| [`reviewer-ux/SKILL.md`](./reviewer-ux/SKILL.md) | `ux` | **Sol** (OpenAI / Codex) | `openai-sol` | Flujo de usuario, cantidad de pasos, claridad y adherencia a directrices de `0_Brain` |
| [`reviewer-ui/SKILL.md`](./reviewer-ui/SKILL.md) | `ui` | **Astra** (RL *computer use*) | `astra-cu` | Validación visual real de interfaz, estados, diseño responsive y accesibilidad (`a11y`) |
| [`reviewer-code/SKILL.md`](./reviewer-code/SKILL.md) | `code_architecture` | **Sonnet** (modelo ligero) | `anthropic-sonnet` | Contraste de código, comentarios y evidencia TDD frente a la arquitectura en `0_Brain` + rigor `G5` |
