# Equipo 4: `4_Revisores` (Revisión Especializada Multi-Lente en Contexto Limpio)

Responsable de evaluar cada entrega verificada de [`3_Builder`](../3_Builder/README.md) mediante **cuatro subagentes independientes con contexto totalmente limpio** (acceso únicamente a `0_Brain`, especificación, `git diff` y evidencia literal `G1`, sin el historial del constructor) y **diversidad de modelos** para eliminar el sesgo de autoevaluación:

1. **Revisor de Seguridad:** modelo de familia independiente al del constructor (referencia conceptual: *Sol* / Codex) enfocado en vulnerabilidades, inyecciones, permisos, `G4`, fronteras de confianza y zonas críticas (`wiki/risk_zones.md`).
2. **Revisor de UX:** modelo independiente (referencia conceptual: *Sol*) enfocado en flujo de usuario, cantidad de pasos, claridad para el usuario final y adherencia a directrices de `0_Brain`.
3. **Revisor de UI:** modelo multimodal / *computer use* (referencia conceptual: *Astra*) para validación visual real de interfaz, diseño responsive y accesibilidad *(aplicabilidad condicional cuando exista interfaz gráfica)*.
4. **Revisor de Código y Arquitectura:** modelo más ligero (referencia conceptual: *Sonnet*) para contrastar código, comentarios y citas verificables (`G5`) frente a la arquitectura registrada en `0_Brain`.

Los cuatro dictámenes estructurados se entregan a [`5_Cycle_Approver`](../5_Cycle_Approver/README.md).

**Estado:** Esqueleto alineado con [`Docs/designs/3_builder.md`](../Docs/designs/3_builder.md); diseño técnico e implementación pendientes tras `3_Builder`.
