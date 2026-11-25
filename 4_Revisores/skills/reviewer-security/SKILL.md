---
name: reviewer-security
description: Subagente independiente de Seguridad (modelo de referencia Sol / Codex) para auditar entregas de 3_Builder en contexto totalmente limpio frente a G4, fronteras de confianza y wiki/risk_zones.md.
version: "1.0"
owner: 4_Revisores
---

# Skill: `reviewer-security` (Auditoría de Seguridad en Contexto Limpio)

## Rol y Modelo Objetivo

* **Eje (`axis`):** `security` (`profile_role: "reviewer_security"`).
* **Modelo de referencia en harness:** **Sol** (OpenAI, vía licencia Codex) — familia independiente al Builder (**Opus**) para eliminar el sesgo de autoevaluación.
* **Modo de contexto:** estrictamente limpio (*Goldfish*) y **100% solo lectura** sobre el `worktree`.

## Entradas Permitidas (Prohibido Leer `3_Builder/runtime/` o Historial del Builder)

1. `packets/security.packet.json` emitido por `reviewer.exe prepare`.
2. La copia aislada del `.diff` dentro de `4_Revisores/runtime/` (`git.diff_rel_path`, p. ej. `<task_id>/<bundle_id>/bundle.diff`) y lectura directa de los archivos cambiados en el `worktree` para verificar contexto completo (`G5`).
3. Documentos recuperados de `0_Brain` (`constitution.md`, `wiki/risk_zones.md` y `brain_context.matched_docs[]`).

## Checklist de Auditoría Obligatoria

1. **Inyecciones y Validación de Entradas:** inyección de comandos shell (`exec` / subprocesos), SQL injection, path traversal (`..`, UNC/NT, `\uXXXX`), deserialización permisiva y desbordamiento de cuotas/memoria.
2. **Regla `G4` y Fugas:** ausencia total de secretos, tokens, llaves PEM/SSH, rutas personales de host o correos en código, pruebas, JSONs y salidas sanitizadas.
3. **Sistema de Archivos y Concurrencia (`Windows` + POSIX):** protección contra symlinks, junctions/reparse points (`CheckNoReparseChain`), TOCTOU entre validación y uso, orden consistente de cerrojos y escrituras atómicas.
4. **Fronteras de Confianza y `risk_zones.md` (`G3` / HITL):** verificar que todo toque a `risk_hits[]`, autenticación, permisos o mutación de datos cumpla las mitigaciones exigidas por `0_Brain`.
5. **Alcance y Anti-Bypass:** comprobar que ningún cambio en el diff viole `scope.read_only` o `scope.forbidden`.

## Contrato de Salida (`review_verdict.schema.json`)

* **Prohibido emitir `status: "NA"`** en el eje `security` (solo `PASS` o `FAIL`).
* Todo hallazgo en `findings[]` debe citar **`file` relativo saneado, `line` real verificada en disco (`G5`) y `evidence_quote` literal no vacío** coincidente con el código o el diff.
* Si falta una prueba de regresión de seguridad para corregir el hallazgo, marcar `requires_red_test: true`.
* Registrar `model_id` y `model_family` reales del subagente (distinto de `anthropic-opus` / `deterministic`) y validar el archivo staging con `reviewer.exe validate`.
