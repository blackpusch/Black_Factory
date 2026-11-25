# Informe Consolidado de Re-Auditoría Goldfish — Ronda 3 (`Docs/designs/2_spec_writer.md`)

* **Documento auditado:** `C:\Black Factory\Docs\designs\2_spec_writer.md` (`Status: Proposed`, `796` líneas, tras aplicar Ronda 2)
* **División semántica:** `FeatureRange = 1..552` | `DetailedRange = 553..796`
* **Estado de revisores:** **8/8 `completed`** (0 `not run`)
* **Regresiones sobre bloqueantes de Ronda 1 y Ronda 2 (`B-01..B-08`, `R2-B1..R2-B3`):** **`0`**
* **Veredicto Ronda 3:** **FAIL — NO PROMOCIONAR A `Approved`**
* **`unresolved_blockers`: `4`** (`5/8` lentes en `0` bloqueantes; más `3` 🟡 Majors y `10` 🟢 Nits)

---

## Cobertura de Revisores (Ronda 3)

| Revisor (`TypeName`) | Alcance | Estado | 🔴 Blocker | 🟡 Major | 🟢 Nit |
| :--- | :--- | :---: | :---: | :---: | :---: |
| `egm-zoom-out` (`23b7942c`) | `1..796` | `completed` | 0 | 0 | 2 |
| `egm-comprehension` (`db281798`) | `1..552`, `553..796` (sin contexto) | `completed` | 0 | 0 | 2 |
| `egm-structural` (`7442b55a`) | `1..552` | `completed` | 0 | 1 | 2 |
| `egm-style` (`bd7cc964`) | `1..796` | `completed` | 0 | 2 | 4 |
| `egm-design` (`06607087`) | `1..552` | `completed` | 0 | 1 | 2 |
| `egm-implementation-readiness` (`613e2e1e`) | `553..796` (+ refs `1..552`) | `completed` | 1 | 0 | 3 |
| `egm-codebase-details` (`d05f4249`) | `553..796` (+ citas `1..796` en disco) | `completed` | 2 | 2 | 2 |
| `egm-inventive-catalyst` (`a70881f5`) | `1..796` | `completed` | 1 | 2 | 1 |

---

## 🔴 Bloqueantes Consolidados de Ronda 3 (`unresolved_blockers: 4`)

### R3-B1 — Enum `WorkItem.kind` incorrecto en la definición de `FTD` en el glosario (`L17`)
* **Fuentes:** `egm-codebase-details` (Blocker), `egm-design` (Major), `egm-structural` (Major), `egm-style` (Major), `egm-zoom-out` (Nit), `egm-inventive-catalyst` (Nit)
* **Líneas:** `L17` vs. `1_Intake/engine/model.go:27-33`, `1_Intake/schemas/workitem.schema.json:87-94`, `Docs/designs/1_intake.md:19`
* **Defecto:** `L17` afirma `` `WorkItem.kind` acotado a `bug|chore|doc|refactor` ``, pero en `1_Intake` el enum cerrado es `{bug, feature, refactor, audit_fix, ops}` (`chore` y `doc` no existen, y `audit_fix` es elegible para `FTD`).
* **Corrección propuesta:** Reemplazar `` `bug|chore|doc|refactor` `` por `` `bug|audit_fix|refactor` `` en `L17`.

### R3-B2 — Regla Tier 2 inexistente `"e IPs privadas"` atribuida a `pkg/bfcore/g4.go:48-115` (`L24`)
* **Fuentes:** `egm-codebase-details` (Blocker)
* **Líneas:** `L24` vs. `pkg/bfcore/g4.go:48-115`
* **Defecto:** `L24` afirma que Tier 2 incluye `"correos e IPs privadas"`, pero las 3 reglas Tier 2 de `pkg/bfcore/g4.go:48-115` son únicamente `win_user_path` (`L62-66`), `unix_user_path` (`L68-72`) y `email_address` (`L110-114`).
* **Corrección propuesta:** Eliminar `e IPs privadas` en `L24` para describir exactamente `(rutas personales C:\Users\<user>\... / /home/<user>/... / /Users/<user>/... y correos)`.

### R3-B3 — Conflicto de compilación Go: alias `SubprocessBrainClient` en `1_Intake` con método `Query` local (`L231, L594, L609`)
* **Fuentes:** `egm-implementation-readiness` (Blocker), `egm-codebase-details` (Major)
* **Líneas:** `L231, 594, 609` vs. `1_Intake/engine/brain_client.go:53-75, 184-222` y `1_Intake/engine/intake_test.go:403, 449`
* **Defecto:** En Go no puede definirse `func (c *SubprocessBrainClient) Query(...)` en `package intake` sobre un alias `= bfcore.SubprocessBrainClient` (tipo no local), y `Query` invoca `c.execBrain(args)` (no exportado).
* **Corrección propuesta:** Exportar `ExecBrain(args []string) ([]byte, BrainInvocation, error)` en `bfcore.SubprocessBrainClient` (`L594`), definir `intake.BrainClient` embebiendo `bfcore.BrainClient` + `Query(...)`, y usar wrapper struct local `type SubprocessBrainClient struct { *bfcore.SubprocessBrainClient }` en `1_Intake/engine/brain_client.go` (`L609`).

### R3-B4 — Bypass del allowlist `verify_steps` por multilínea en `acceptance_criteria[].description` (`L362-363, L720`)
* **Fuentes:** `egm-inventive-catalyst` (Blocker), `egm-codebase-details` (Major), `egm-implementation-readiness` (Nit), `egm-design` (Nit)
* **Líneas:** `L362-363, 720` vs. `GengiSWAP/engine/bridge.go:450-474`
* **Defecto:** Una segunda línea tras `\n` dentro de `acceptance_criteria[].description` (`\nVerificacion: <cmd>`) tiene `isBullet == false`, activando `parseFieldLabel` en `bridge.go:464-474` e inyectando comandos no validados en `Chunk.VerifyCommands`.
* **Corrección propuesta:** Prohibir `\r`/`\n` en todos los campos de línea única (`title`, `target_branch`, `g3_data_mutation`, `work_units[].title`, `work_units[].inputs[]`, `work_units[].acceptance_criteria[].description`) y extender la guarda de `ValidateSpecAndRenderCheck` a los campos de texto libre de `index.md` (`R3-M2`).

---

## 🟡 Hallazgos Mayores Consolidados de Ronda 3 (`3`)

| ID | Tema | Líneas | Fuentes |
| :--- | :--- | :--- | :--- |
| **R3-M1** | Alinear expansiones de acrónimos en glosario con `1_intake.md:15,18,19`: `FTD` (`Fast-Track Development`), `ASDD` (`Agentic / Spec-Driven Development`), `OKF` (`Open Knowledge Format`). | `17, 18, 28` | `egm-style` |
| **R3-M2** | Extender guarda `ValidateSpecAndRenderCheck` a los campos de texto libre de `index.md` (`narrative.problem`, `stack_assumptions[]`, `dod[]`, `brain_followups[]`) prohibiendo `^#{1,6}\s`, fences, `reMaxRetries` y `parseFieldLabel` de alcance (`bridge.go:115-158, 196-212`). | `246-259, 641, 720` | `egm-inventive-catalyst`, `egm-design` |
| **R3-M3** | En `[1]` (`L355, L715`), siempre que `len(decisions) > 0`, verificar que la decisión activa coincida con `current_scope_sha256` (cerrando la brecha tras ascenso `FTD → ASDD` sin escalado de riesgo/módulo). | `262, 344-347, 355, 715` | `egm-inventive-catalyst` |

---

## 🟢 Hallazgos Menores Consolidados de Ronda 3 (`10` Nits)

1. Exportar `FindUncoveredPaths` (`1_Intake/engine/triage.go:254-289`) y `RiskLevelRank` (`1_Intake/engine/model.go:324-329`) en `pkg/bfcore/brain_client.go` (Fase 0) para paridad 1:1 sin duplicación (`egm-zoom-out`).
2. Fijar orden determinista de salida en `VerifyRuntime` (`L683`): `REPARSE_POINT_DETECTED` > `G4_VIOLATION_ON_DISK` (`3`) > `GITIGNORE_DRIFT` > `GENGIS_ENGINE_DRIFT` > `ATTEMPTS_STATE_CORRUPT` > `ARTIFACT_STATE_CORRUPT` > `BRAIN_JOURNAL_ACTIVE` > `BRAIN_UNAVAILABLE` > `IO_ERROR` (`egm-implementation-readiness`).
3. Normalizar `VerifyStep.Env == nil` → `map[string]string{}` en `CanonicalSpecSHA256` (`L278, L660`) (`egm-implementation-readiness`).
4. Especificar tag JSON `GitIgnoreOK` (`json:"gitignore_ok"`) en `VerifyReport` (`L659`) (`egm-codebase-details`).
5. Incluir `render` en la fila `exit 1` de la tabla comparada (`L44`) (`egm-structural`).
6. Unificar etiquetas de `Objetivo` (`index.md`: `[HITL]/[CRITICO]/[ESCALATED]`; `Chunk.Spec`: `[HITL]/[CRITICO]/[ESCALATED]/[EXPANSIONS:N]`, `L248, 253, 641-642`) (`egm-structural`, `egm-comprehension`).
7. Explicitar `NormalizeCoverPath` y `MatchesPersonalPathG4` como símbolos preexistentes de `pkg/bfcore` (`L163-164`) (`egm-comprehension`).
8. Añadir prefijo `GengiSWAP/` en `L244, 247` y ruta completa `0_Brain/engine/cmd/brain/main.go` en `L403`; añadir `(mismos defaults que prepare)` en `L309, 311, 312` (`egm-style`).
9. Calibrar `L526` (`libre de rutas absolutas de --repo-root y de rutas personales G4`), aclarar condicional de `L740` (`si dispara una nueva decisión externa...`), y precisar `bridge.go:607` (espacio+`` ` ``) en `L412` (`egm-style`, `egm-codebase-details`).
10. Aclarar en `L405` que `--factory-root`/`--repo-root` en `verify_steps` admiten `"."` explícito o se omiten para usar el default `"."` (`egm-design`).
