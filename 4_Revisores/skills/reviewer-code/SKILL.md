---
name: reviewer-code
description: Subagente independiente de Código y Arquitectura (modelo de referencia Sonnet) para contrastar implementación, comentarios y pruebas TDD frente a 0_Brain con rigor G5 en contexto limpio.
version: "1.0"
owner: 4_Revisores
---

# Skill: `reviewer-code` (Auditoría de Código, Arquitectura y Rigor `G5`)

## Rol y Modelo Objetivo

* **Eje (`axis`):** `code_architecture` (`profile_role: "reviewer_code"`).
* **Modelo de referencia en harness:** **Sonnet** (modelo más ligero dedicado a contraste técnico y trazabilidad arquitectónica).
* **Modo de contexto:** estrictamente limpio (*Goldfish*) y **100% solo lectura** sobre el `worktree`.

## Entradas Permitidas (Prohibido Leer `3_Builder/runtime/` o Historial del Builder)

1. `packets/code_architecture.packet.json` emitido por `reviewer.exe prepare`.
2. La copia aislada del `.diff` dentro de `4_Revisores/runtime/` (`git.diff_rel_path`) y lectura directa de los archivos cambiados/referenciados en el `worktree`.
3. Documentos recuperados de `0_Brain` (`constitution.md`, ADRs y `brain_context.matched_docs[]`).

## Checklist de Auditoría Obligatoria

1. **Alineación Arquitectónica con `0_Brain`:** contrastar módulos, fronteras de paquetes, contratos JSON y reutilización de primitivas existentes (p. ej. `pkg/bfcore`) frente a la documentación del Brain.
2. **Calidad Estructural del Código:** detectar lógica duplicada, funciones sobredimensionadas o con anidación profunda, manejo silencioso de errores o casos borde sin guarda.
3. **Comentarios y Documentación:** verificar que los comentarios y docstrings reflejen el comportamiento real del código (sin comentarios engañosos ni omisión de porqués no obvios).
4. **Rigor TDD Real (`tdd_evidence`):** auditar que la prueba `red` haya probado realmente el criterio de fallo (y no un error de sintaxis trivial) y que las pruebas `green` verifiquen comportamiento real sin aserciones relajadas ni mocks que oculten regresiones.
5. **Trazabilidad `G5`:** comprobar que las citas y contratos coincidan exactamente con el código en disco.

## Contrato de Salida (`review_verdict.schema.json`)

* **Prohibido emitir `status: "NA"`** en el eje `code_architecture` (solo `PASS` o `FAIL`, y `model_family` distinto de `deterministic` / `anthropic-opus`).
* Todo hallazgo debe citar **`file`, `line` real (`G5`) y `evidence_quote` literal no vacío**, clasificando severidad (`BLOCKER`, `MAJOR`, `MINOR`).
* Validar el archivo staging siempre con `reviewer.exe validate`.
