# Black Factory — Software Factory sobre GengiSWAP & RSC

Repositorio oficial: `https://github.com/blackpusch/Black_Factory`

**Black Factory** es una factoría de software repetible, auditable y paralelizable que combina la metodología de arnés de **`ericrisco/rsc-harness`** (*Open Knowledge Format*, `constitution.md`, `AGENTS.md`, `SKILL.md` progresivos y carriles `ASDD`/`SDD`, `FTD`, `TDD`) con el motor determinista de **`GengiSWAP` (`gengis`)** (*las reglas las hace cumplir el motor, no la buena voluntad de ningún modelo*).

---

## Arquitectura en 7 Etapas (Equipos)

1. [`0_Brain/`](0_Brain/README.md) — Memoria persistente (*The Brain*): estándar OKF/RSC + motor Go `brain.exe` con índice SQLite FTS5, validación `G4`/enlaces, detección de *drift* Git y futura exposición MCP. Bajo **Opción i**, `0_Brain/vault/` permanece *untracked* en `.gitignore`.
2. [`1_Intake/`](1_Intake/README.md) — Recepción y normalización de incidencias/peticiones (`WorkItem`) y triaje de carril (`ASDD` / `FTD`).
3. [`2_Spec_Writer/`](2_Spec_Writer/README.md) — Generación de especificaciones técnicas consultando `0_Brain` antes de construir.
4. [`3_Builder/`](3_Builder/README.md) — Construcción y remediación bajo TDD en ramas/worktrees aislados, integrando el anillo determinista de **`GengiSWAP/` (`gengis`)**.
5. [`4_Revisores/`](4_Revisores/README.md) — Subagentes independientes con contexto limpio (Seguridad, Calidad/Arquitectura vs Brain + G5, UX/Flujo, UI).
6. [`5_Cycle_Approver/`](5_Cycle_Approver/README.md) — Consolidación de reportes de revisores y rebote estructurado hacia `3_Builder` hasta un límite de ciclos.
7. [`6_Filtro_de_Riesgo/`](6_Filtro_de_Riesgo/README.md) — Compuerta híbrida (reglas deterministas en Go + clasificador) entre vía rápida de bajo riesgo y PR con aprobación humana `[AUTOR]` para cambios críticos (`G3`, BD, auth, core).

---

## Núcleo `GengiSWAP/` (`gengis`) y Separación Operativa

- `GengiSWAP/`: Protocolos v2 (`gengis_swap_common.md`, `gengis_swap_prep_protocol.md`, `gengis_swap_chef_protocol.md`), plantillas `v1` (`templates/`) y motor determinista en **Go 1.24** (`GengiSWAP/engine/`, módulo `gengiswap/ring`).
- [`Docs/PROYECTO_TUNGSTENO.md`](Docs/PROYECTO_TUNGSTENO.md): Referencia técnica del motor `gengis`.
- **Artefactos privados / locales (ignorados en `.gitignore`):**
  - `0_Brain/vault/`, `0_Brain/brain.exe` y archivos `*.db` (`Opción i`).
  - `GengiSWAP/gengis.exe` y `GengiSWAP/gengis_global_log.md`.
  - `C:\Desarrollo\GengiSWAP_data\` para tareas activas/archivadas (`gengis_task\`) y respaldos.

### Verificación y Compilación de `GengiSWAP` (`gengis.exe`)

```powershell
Set-Location "C:\Black Factory\GengiSWAP\engine"
go vet ./...
go test -count=1 ./...
go build -o ..\gengis.exe .\cmd\gengis
```

*(Nota: en entornos Windows sin pila de red activa, `TestShellRunnerWaitDelay` puede omitirse con `-skip TestShellRunnerWaitDelay`.)*

### Uso rápido de `gengis.exe` desde `Black Factory`

```powershell
& "C:\Black Factory\GengiSWAP\gengis.exe" status "C:\Desarrollo\GengiSWAP_data\gengis_task\<ID>" --root "C:\Black Factory\GengiSWAP"
```
