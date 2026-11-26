# Skills Modulares de `1_Intake` (Carga Progresiva RSC)

Catálogo oficial de *skills* de recepción, sanitización `G4` y triaje determinista para **Black Factory**, acoplado al motor `1_Intake/intake.exe` y al contrato de solo lectura con `0_Brain/brain.exe`.

## Índice de Skills

| Skill | Propósito principal | Archivo |
| :--- | :--- | :--- |
| `intake-triage` | Ingesta normalizada, frontera `G4` (Tier 1 fail-closed / Tier 2 punto fijo), resolución de cuarentena `G6` y triaje `ASDD` vs `FTD` | [`intake-triage/SKILL.md`](intake-triage/SKILL.md) |

## Ciclo Operativo de Recepción y Triaje

```text
┌──────────────────────┐     ┌─────────────────────────┐     ┌────────────────────────┐
│ 1. intake.exe ingest │ ──► │   ¿Frontera G4 / G6?    │ ──► │ status: ready          │
│ (--input / banderas) │     │ • Tier 1 ──► Exit 3     │     │ workitems/WI-<id>.json │
└──────────────────────┘     │ • Incompleto ──► Exit 2 │     │ lane: ASDD | FTD (Ex 0)│
                             └────────────┬────────────┘     └───────────▲────────────┘
                                          │ (Exit 2: quarantine/)        │
                                          ▼                              │
                             ┌─────────────────────────┐                 │
                             │ 2. intake.exe resolve   │ ────────────────┘
                             │ --id WI-<id> --path ... │ (Promoción atómica: PROMOTED)
                             └─────────────────────────┘
```

## Matriz de Activación por Equipo

| Equipo | Uso de `intake-triage` | Artefacto consumido / producido |
| :--- | :--- | :--- |
| `1_Intake` | Carga obligatoria al recibir cualquier ticket, bug o solicitud | Produce `1_Intake/runtime/workitems/WI-<hash12>.json` o `quarantine/WI-<hash12>.json` |
| `2_Spec_Writer` | Consulta `intake.exe show --id <WI-ID>` para solicitudes en carril `ASDD` | Consume `WorkItem` con `lane: "ASDD"`, `risk_hits` y `brain_evidence` |
| `3_Builder` | Consulta `intake.exe show --id <WI-ID>` para mantenimiento acotado en carril `FTD` | Consume `WorkItem` con `lane: "FTD"` y `suggested_template: "correccion_hallazgos"` |
| `6_Filtro_de_Riesgo` | Contrasta `risk_level`, `risk_hits` y `requires_autor_hitl` pre-build contra el diff real | Verifica `intake.exe verify --json` |

## Reglas Globales (`G1–G6`) y Privacidad (`Opción i`)

- **G1:** toda ejecución de `intake.exe` registra comandos literales, códigos de salida (`0..5`) y evidencia de `0_Brain` en `brain_evidence.invocations`.
- **G2:** las vistas Markdown (`intake.exe show --format md`) se generan bajo demanda con tablas ASCII/Unicode Box-Drawing (prohibido Mermaid).
- **G3 / NG2:** `1_Intake` es consumidor estrictamente de solo lectura de `0_Brain` y nunca muta bases de datos productivas ni `0_Brain/vault/`.
- **G4:** ningún secreto Tier 1 ni ruta personal Tier 2 sin redactar llega a disco; toda escritura pasa `bfcore.ScanG4All` en memoria.
- **G5:** las zonas de riesgo (`RZ-001`, `RZ-002`) y cobertura documental provienen del catálogo verificado de `0_Brain`.
- **G6:** ante alcance ausente (`affected_paths` vacío), rutas inválidas o proyecto no registrado, `intake.exe` retiene el ítem en `quarantine/` (`Exit 2`).
- **Privacidad `Opción i`:** `1_Intake/runtime/` y `1_Intake/intake.exe` permanecen excluidos de Git en `.gitignore` y se validan con `intake.exe verify --json`.
