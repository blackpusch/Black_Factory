# Skills Modulares de `2_Spec_Writer` (Carga Progresiva RSC)

Catálogo oficial de *skills* de autoría estructurada, validación estática y compilación determinista hacia `GengiSWAP` para **Black Factory**, acoplado al motor `2_Spec_Writer/specwriter.exe`.

## Índice de Skills

| Skill | Propósito principal | Archivo |
| :--- | :--- | :--- |
| `spec-authoring` | Bucle operativo `prepare → spec.json → validate → [ASDD Goldfish] → render → compile → gengis.exe plan` y recuperación ante códigos `0..6` | [`spec-authoring/SKILL.md`](spec-authoring/SKILL.md) |

## Ciclo Operativo

```text
┌───────────────────────────┐     ┌──────────────────────────────┐     ┌──────────────────────────────┐
│ 1. specwriter.exe prepare │ ──► │ 2. Editar spec.json +        │ ──► │ 3. specwriter.exe compile    │
│    (WorkItem -> runtime/) │     │    specwriter.exe validate   │     │    (create-only NTFS rename) │
└───────────────────────────┘     │    --stage draft [| final]   │     └──────────────┬───────────────┘
                                  └──────────────────────────────┘                    ▼
                                                                       ┌──────────────────────────────┐
                                                                       │ 4. [AUTOR] gengis.exe plan   │
                                                                       └──────────────────────────────┘
```

## Reglas Globales (`G1–G6`) y Privacidad (`Opción i`)
- **G1 / G5:** toda cita en `g5_citations[]` se verifica en disco (línea 1-based CRLF/BOM y ancla literal `±5` líneas).
- **G2:** prohibido incluir bloques Mermaid en `spec.json` o documentación; usar ASCII/Unicode Box-Drawing.
- **G3:** todas las pruebas corren sobre `t.TempDir()`, sin mutar datos productivos.
- **G4:** fail-closed en las 11 reglas (`exit 3`, sin auto-redacción Tier 2); cero rutas absolutas de host dentro de `2_Spec_Writer/runtime/`.
- **G6:** ante `exit 4` (`STALE_CONTEXT`, `ESCALATION_REQUIRED`, `LANE_UPGRADE_REQUIRED`) o `exit 5` (`MAX_*`), detenerse y coordinar resolución con `[AUTOR]`.
- **Opción i:** `2_Spec_Writer/runtime/` y `2_Spec_Writer/specwriter.exe` permanecen excluidos en `.gitignore`, verificados con `specwriter.exe verify --json`.
