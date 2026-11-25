# Skills de `3_Builder`

Esta carpeta contiene las guías operativas progresivas (*progressive disclosure*) para los agentes y arneses externos que operan sobre [`builder.exe`](../README.md).

## Catálogo

| Skill | Archivo principal | Plantilla de perfiles | Propósito |
| :--- | :--- | :--- | :--- |
| `builder-execution` | [SKILL.md](./builder-execution/SKILL.md) | [profiles.example.json](./builder-execution/profiles.example.json) | Bucle completo TDD (`tdd-red` -> `package` -> `4_Revisores` -> `5_Cycle_Approver` -> `apply-order` -> `commit` -> `[AUTOR]` -> `cleanup`), aplicabilidad UX/UI y recuperación de códigos `1..6`. |

## Reglas invariantes
- `builder.exe` (Go pure-`CGO_ENABLED=0`) nunca lee ni interpreta archivos de esta carpeta; solo valida contratos deterministas de entrada/salida (`DeliveryBundle` v1.0 y `BounceOrder` v1.0).
- Ningún modelo ni perfil puede saltarse `[AUTOR]` (`gengis.exe plan` / `gengis.exe decide`), editar el repositorio principal fuera del `git worktree` aislado o mutar `GengiSWAP/engine/**` (`RZ-001`).
