# Registro de Deuda Técnica — Black Factory

Registro cronológico y verificable (`G5`) de deuda técnica diferida, hallazgos no bloqueantes aceptados por `[AUTOR]` y seguimientos fuera de alcance (`NG`) detectados durante la construcción de los equipos de **Black Factory**.

---

## Tabla Resumen

| ID | Fecha | Módulo | Severidad | Estado | Origen | Resumen |
| :--- | :---: | :--- | :---: | :---: | :--- | :--- |
| `TD-001` | `2026-11-24` | `2_Spec_Writer` | `Major` + `3 Nits` | Abierta (diferida) | `bf_02_spec_writer_v1` (Intento 3, `GO_NEXT`) | Restricción estricta de `WorkItem.TraceID` (`reTraceID`) + paridad `verifyRenderedGengisParity` (`\r`, `~~~`, `#`, `reIndexMaxRetries`) + `TargetBranch` + `review_gate.verify_step_id` |
| `TD-002` | `2026-11-24` | `0_Brain` | `Major` | Abierta (preexistente) | `bf_02_spec_writer_v1` (Chunk 01 Intento 1) | `renderFrontmatter` omite `expires_at`, causando flake de borde de segundo en `TestSignProposalAndVerifyRoundTrip` |
| `TD-003` | `2026-11-23` | `0_Brain` | `Major` | Abierta (preexistente) | `Docs/designs/2_spec_writer.md` (`NG5`) | `ProposeUpdate` no adquiere `.brain.lock` ni ejecuta `ScanG4` antes de persistir propuestas en `_proposals/` |
| `TD-004` | `2026-11-19` | `GengiSWAP` | `Media` | Abierta (Paso 6) | `Docs/PROYECTO_TUNGSTENO.md` (§5.5–5.7) + auditoría `2026-11-25` | Evolución pendiente del motor `gengis`: `QA_OVERRIDE`, cursor de log sincronizado, `TaskSource`, `saveBaseRef`, divergencias `D1–D5` y limpieza estática (`runner.go:54`, `bridge.go:267`) |
| `TD-005` | `2026-11-24` | `0_Brain/vault` | `Baja` | Pendiente post-merge | Cierre `bf_02_spec_writer_v1` / `bf_03_builder_v1` | Registrar `2_Spec_Writer`, `3_Builder` y `pkg/bfcore` en la bóveda local `0_Brain/vault` tras integrar en `main` |
| `TD-006` | `2026-11-25` | `pkg/bfcore` / `0_Brain` / `2_Spec_Writer` / `3_Builder` | `Media` | Abierta (pre-`4_Revisores/engine`) | Auditoría SonarQube + `golangci-lint` (`934e885`) | Extraer `ScanRawAndDecodedG4`/`WriteJSONAtomicG4` de `2_Spec_Writer`/`3_Builder` a `pkg/bfcore` y retirar copia `lock_*.go` en `0_Brain/engine` |
| `TD-007` | `2026-11-25` | `0_Brain` / `1_Intake` / `2_Spec_Writer` | `Baja`–`Media` | Abierta (módulos cerrados `NG5`) | Auditoría SonarQube + `golangci-lint` (`934e885`) | Hacer `verify` 100% libre de cerrojos/escrituras en `0_Brain` y `1_Intake`, más limpieza `errcheck`/`ineffassign`/`SA9003` y reducción `S3776` |

---

## Detalle de Ítems

### `TD-001` — Hardening de `WorkItem.TraceID` y paridad fina en `2_Spec_Writer`

- **Fecha de registro:** `2026-11-24`
- **Origen:** Auditoría Goldfish del Intento 3 de `bf_02_spec_writer_v1` (commit `00c32c7f1e3aa1f9e370a6236ef6f72f0ac6011f`), aceptada como diferida por `[AUTOR]` en `GO_NEXT` (`0` Blockers).
- **Por qué no bloquea avance:** En operación normal `1_Intake` emite `trace_id` con formato fijo `trc-<timestamp>-<hex8>` (sin espacios ni backticks). Solo es explotable si un actor modifica externamente el JSON del `WorkItem` en disco, y no inyecta comandos ni altera alcances (`CanModify`/`Forbidden`), únicamente sobrescribiría `Máx. intentos por chunk` en `index.md` (visible para `[AUTOR]` en `gengis.exe plan`).
- **Referencias `G5` y remediación propuesta:**
  1. **`Major` (`WorkItem.TraceID`):** `2_Spec_Writer/engine/prepare.go:454-456`, `2_Spec_Writer/engine/validate.go:790-818, 2335-2342` y `2_Spec_Writer/engine/templates/index.md.tmpl:28` — exigir regex estricto `^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$` en `prepare.go` y `validate.go`, pasar `checkIndexProseSafety(ctx.TraceID, ...)` y exigir en `verifyRenderedGengisParity` exactamente 1 coincidencia de `reIndexMaxRetries` con valor `"3"` en `index.md`.
  2. **`Nit 1` (`spec.TargetBranch`):** `2_Spec_Writer/engine/validate.go:740-742` — aplicar `checkIndexProseSafety(spec.TargetBranch, "/target_branch", addIssue)`.
  3. **`Nit 2` (Paridad `\r` / `~~~` / `#`):** `2_Spec_Writer/engine/validate.go:2271, 2314-2315, 2373-2408` — normalizar `\r` aislado a `\n` y validar en `verifyRenderedGengisParity` fences `~~~` y encabezados `#`/`###` en paridad exacta con `GengiSWAP/engine/bridge.go:84, 456, 843`.
  4. **`Nit 3` (`review_gate.verify_step_id`):** `2_Spec_Writer/engine/validate.go:973-980` vs `2_Spec_Writer/schemas/spec.schema.json:578-581` — rechazar `verify_step_id != ""` cuando `kind == "review_gate"`.

### `TD-002` — Pérdida de `expires_at` en `renderFrontmatter` de `0_Brain`

- **Fecha de registro:** `2026-11-24`
- **Origen:** Flake observado durante `gengis.exe check` de `bf_02_spec_writer_v1` Chunk 01 Intento 1 (fuera de alcance por `NG6`).
- **Referencias `G5`:** `0_Brain/engine/vault.go:1316` (`renderFrontmatter` solo serializa `target_path`, `operation`, `requires_autor_approval` y `author_role`, omitiendo `expires_at`), provocando fallo intermitente en `TestSignProposalAndVerifyRoundTrip` (`0_Brain/engine/vault_hardening_test.go:446`) cuando cambia el segundo de reloj entre `SignProposal` (`vault.go:973-979`) y `ApplyUpdate`.
- **Remediación propuesta:** Incluir `"expires_at"` en la lista ordenada de claves de propuesta de `renderFrontmatter` y añadir test determinista con reloj inyectado o pausa de cruce de segundo.

### `TD-003` — Lock y escaneo `G4` en `ProposeUpdate` de `0_Brain`

- **Fecha de registro:** `2026-11-23`
- **Origen:** Identificado durante el diseño aprobado [`Docs/designs/2_spec_writer.md`](designs/2_spec_writer.md) (`NG5`, L90 y L101).
- **Referencias `G5`:** `0_Brain/engine/vault.go:244-418` (`ProposeUpdate`).
- **Remediación propuesta:** Adquirir `.brain.lock` y ejecutar `ScanG4` antes de persistir propuestas en `0_Brain/vault/_proposals/` (prerrequisito antes de habilitar wrapper automático `BrainPatch` / `brain.exe propose` en `2_Spec_Writer` v2).

### `TD-004` — Pendientes Paso 6 y divergencias `D1–D5` en `GengiSWAP` (`RZ-001`)

- **Fecha de registro:** `2026-11-19` (revisado `2026-11-24` como insumo para `3_Builder`)
- **Origen:** [`Docs/PROYECTO_TUNGSTENO.md`](PROYECTO_TUNGSTENO.md) (§5.5–5.7).
- **Alcance:** `GengiSWAP/engine/{ring.go,bridge.go,runner.go}` (`RZ-001`, nivel `CRITICO`; cualquier cambio requiere autorización explícita `[AUTOR]` y actualización coordinada de `PinnedBridgeGoSHA256`/`PinnedRingGoSHA256` en `2_Spec_Writer/engine/verify.go`).
- **Ítems clave a evaluar al diseñar `3_Builder`:** distinción `QA_OVERRIDE`, cursor de eventos sincronizados en `.ring/state.json`, errores ignorados en `saveBaseRef`, `NopWorkspace` silencioso cuando falla `parseIndexFromDir` en `resolveTargetRepo`, y aislamiento real con `git worktree` + interfaz `TaskSource`.
- **Deuda estática menor detectada (`2026-11-25`, SonarQube / `golangci-lint`):** variable `max` en `GengiSWAP/engine/runner.go:54` sombrea el builtin `max` (`go:S978`) y asignación inefectiva `prio := 10` en `GengiSWAP/engine/bridge.go:267` (`ineffassign`) — no tocadas en `feat/3-builder` por `RZ-001` y hashes SHA-256 anclados en `verify.go`.

### `TD-005` — Sincronización de `0_Brain/vault` post-merge de `1_Intake`, `2_Spec_Writer` y `3_Builder`

- **Fecha de registro:** `2026-11-24`
- **Origen:** Cierre de `bf_01_intake_v1`, `bf_02_spec_writer_v1` y `bf_03_builder_v1` (ramas `feat/1-intake`, `feat/2-spec-writer` y `feat/3-builder` pendientes de integración en `main`).
- **Acción requerida post-merge:** Actualizar `wiki/` y `index.md` en `0_Brain/vault/` (bóveda privada `Opción i`) documentando `pkg/bfcore`, `1_Intake`, `2_Spec_Writer` y `3_Builder`, y reejecutar `brain.exe index` + `brain.exe verify`.

### `TD-006` — Consolidación compartida en `pkg/bfcore` antes de `4_Revisores/engine`

- **Fecha de registro:** `2026-11-25`
- **Origen:** Auditoría SonarQube + `golangci-lint` sobre `feat/3-builder` (`934e885`), confirmando [`Docs/designs/3_builder.md`](designs/3_builder.md#L653).
- **Referencias `G5`:**
  1. Duplicación JSON atómico y `G4` entre `2_Spec_Writer/engine/state.go:25-140` (`bfcore.ScanG4All(string(raw))` en `L65`) y `3_Builder/engine/state.go:29-230` (`ScanRawAndDecodedG4(raw)` en `L204`).
  2. Duplicación de wrapper de cerrojos en `0_Brain/engine/lock_windows.go:25-45` y `0_Brain/engine/lock_other.go:25-45` frente a `pkg/bfcore/lock_windows.go` y `pkg/bfcore/lock_other.go`.
- **Remediación propuesta:** Extraer `WriteJSONAtomicG4` / `ScanRawAndDecodedG4` y helpers de lock hacia `pkg/bfcore` antes de implementar `4_Revisores/engine`, y retirar las copias locales en una ventana coordinada posterior al cierre de `feat/3-builder`.

### `TD-007` — `verify` 100% libre de cerrojos en `0_Brain`/`1_Intake` e higiene estática en módulos cerrados

- **Fecha de registro:** `2026-11-25`
- **Origen:** Auditoría transversal (`2026-11-25`) sobre módulos cerrados (`0` bytes tocados en `feat/3-builder` por `NG5`).
- **Referencias `G5`:**
  1. `0_Brain/engine/cmd/brain/main.go:313-323` (`AcquireVaultLock` sobre `.brain.lock` + `RecoverJournal` durante `verify`) y `1_Intake/engine/store.go:566, 683-691` (`s.withLock` sobre `.intake.lock` + evento `"VERIFIED"` en `VerifyRuntime`, consumido en `1_Intake/engine/cmd/intake/main.go:697-705`), que fallan con `Acceso denegado` en tokens/entornos de solo lectura (a diferencia de `2_Spec_Writer` y `3_Builder`).
  2. Higiene estática menor en módulos cerrados: `0_Brain/engine/query.go:79` (`ineffassign`), `2_Spec_Writer/engine/validate.go:2275` (`SA9003` rama vacía), `errcheck` en `0_Brain/engine`, `1_Intake/engine` y `2_Spec_Writer/engine`, y refactorización gradual de funciones de alta complejidad cognitiva (`go:S3776` / `go:S107`, p. ej. `2_Spec_Writer/engine/validate.go:676`).
