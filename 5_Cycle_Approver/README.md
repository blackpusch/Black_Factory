# Equipo 5: `5_Cycle_Approver` (Consolidación y Ciclo de Corrección Controlado)

Responsable de consolidar los cuatro dictámenes independientes emitidos por [`4_Revisores`](../4_Revisores/README.md) (Seguridad, UX, UI y Código/Arquitectura), deduplicar hallazgos y gobernar el ciclo de remediación (modelo de referencia conceptual: *Opus* en el harness externo + control determinista de tope de ciclos en Go):

- Si cualquiera de los cuatro revisores emite *feedback* negativo, devuelve una orden estructurada de corrección (*BounceOrder*) hacia [`3_Builder`](../3_Builder/README.md) para corregir los fallos y re-someter la entrega a validación dentro de un límite máximo de ciclos.
- Cuando los cuatro ejes quedan aprobados, promueve la entrega hacia [`6_Filtro_de_Riesgo`](../6_Filtro_de_Riesgo/README.md).

**Estado:** Esqueleto alineado con [`Docs/designs/3_builder.md`](../Docs/designs/3_builder.md); diseño técnico e implementación pendientes tras `4_Revisores`.
