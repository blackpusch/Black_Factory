# Equipo 4: `4_Revisores` (Revisión Especializada Multi-Lente en Contexto Limpio)

Responsable de evaluar cada entrega verificada de [`3_Builder`](../3_Builder/README.md) (`DeliveryBundle` v1.0) mediante **cuatro subagentes independientes con contexto totalmente limpio** (*Goldfish*: acceso únicamente a `0_Brain`, `ReviewPacket`, copia aislada `bundle.diff` en `4_Revisores/runtime/` y `worktree` en solo lectura, sin acceso a `3_Builder/runtime/` ni historial del constructor) y **diversidad estricta de modelos** para eliminar el sesgo de autoevaluación:

1. **Revisor de Seguridad (`security`):** modelo **Sol** (OpenAI / Codex) enfocado en vulnerabilidades, inyecciones, permisos, `G4`, reparse points, fronteras de confianza y zonas críticas (`wiki/risk_zones.md`).
2. **Revisor de UX (`ux`):** modelo **Sol** (OpenAI / Codex) enfocado en flujo de usuario, cantidad de pasos, claridad para el usuario final y adherencia a directrices de `0_Brain` *(condicional según `reviewer_applicability.ux`)*.
3. **Revisor de UI (`ui`):** modelo **Astra** (RL / *computer use*) para validación visual real de interfaz, estados, diseño responsive y accesibilidad *(condicional según `reviewer_applicability.ui`)*.
4. **Revisor de Código y Arquitectura (`code_architecture`):** modelo **Sonnet** (modelo más ligero) para contrastar código, comentarios y evidencia TDD frente a la arquitectura registrada en `0_Brain` con rigor `G5`.

Los cuatro dictámenes individuales validados (`ReviewVerdict` v1.0) se sellan en un `ReviewSet` v1.0 y se entregan a [`5_Cycle_Approver`](../5_Cycle_Approver/README.md).

## Subdirectories & Contracts (Fase 1)

*   [`DESIGN.md`](./DESIGN.md) — Especificación técnica completa de `4_Revisores` (matriz CLI, códigos `0..6`, structs Go, cuotas, `G5` real en disco, anti-TOCTOU, verificación `RZ-001` y plan TDD Fase 2).
*   [`schemas/`](./schemas/review_verdict.schema.json) — Contratos JSON Schema Draft 2020-12 v1.0 (`additionalProperties: false`):
    *   [`review_packet.schema.json`](./schemas/review_packet.schema.json) — Entrada aislada en contexto limpio por eje.
    *   [`review_verdict.schema.json`](./schemas/review_verdict.schema.json) — Dictamen individual por revisor, alineado 1:1 con `BounceOrder`.
    *   [`review_set.schema.json`](./schemas/review_set.schema.json) — Conjunto sellado de los 4 dictámenes para `5_Cycle_Approver`.
*   [`skills/`](./skills/README.md) — Perfiles agnósticos ([`profiles.example.json`](./skills/profiles.example.json)) y skills progresivas: [`reviewer-security`](./skills/reviewer-security/SKILL.md), [`reviewer-ux`](./skills/reviewer-ux/SKILL.md), [`reviewer-ui`](./skills/reviewer-ui/SKILL.md) y [`reviewer-code`](./skills/reviewer-code/SKILL.md).

## Flujo Operativo Previsto (`reviewer.exe`, Fase 2)

```text
3_Builder (DeliveryBundle v1.0 + .diff)
        │
        ▼
reviewer prepare  ──► Copia aislada bundle.diff + 4 ReviewPackets + NA determinista (si aplica)
        │
        ▼
4 Subagentes en Contexto Limpio (Sol Security | Sol UX | Astra UI | Sonnet Code) -> staging .json
        │
        ▼
reviewer validate ──► Staging externo + worktree hash + G4 doble fase + G5 disco + diversidad modelo
        │
        ▼
reviewer seal     ──► Re-validación completa + ReviewSet v1.0 ──► 5_Cycle_Approver
```

**Estado:** Fase 1 restaurada y endurecida con correcciones de Ronda 2 Goldfish dentro de `4_Revisores/`; lista para confirmación final y promoción a `Docs/designs/4_revisores.md`.
