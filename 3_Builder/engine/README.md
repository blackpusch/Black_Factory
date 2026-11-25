# Motor Go de `3_Builder` (`blackfactory/builder`)

Implementación pure-Go (`CGO_ENABLED=0`, cero SQLite, cero llamadas LLM desde Go) del orquestador de construcción TDD y puente `git worktree` sobre `GengiSWAP`.

## Módulos y Archivos
- [model.go](./model.go): códigos de salida `0..6`, límites operativos (`MaxBounceCycles=3`, `MaxVerifyTimeoutSec=300`, `MaxCommandOutputBytes=8000`, `MaxDiffBytes=2 MiB`, `MaxOrderBytes=256 KiB`), catálogo `Reason*`, `IsRetryableReason`, structs canónicos (`SessionState`, `DeliveryBundle`, `BounceOrder`, `CLIResult`, `ContextResult`, `VerifyReport`) y `CanonicalBundleSHA256`.
- [state.go](./state.go): orden estricto de cerrojos `.session.lock -> .repo-<sha16>.lock -> .events.lock` sobre `bfcore.AcquireFileLock`, `ScanRawAndDecodedG4`, escrituras atómicas `WriteJSONAtomicG4`/`WriteFileAtomicBytes`, `LoadSessionStrict` (`DisallowUnknownFields`) y bitácora JSONL `AppendEvent`.
- [task_parser.go](./task_parser.go): parseo estricto de `index.md`, `chunks/*.md` y `.ring/{state.json,base_ref}`, `Scope.Allows`/`Narrows` (paridad `GengiSWAP/engine/ring.go`), validación de compatibilidad `VerifyCommands` vía `git ls-tree` y binding/lectura atómica de `Repositorio destino` en `index.md`.
- [git_worktree.go](./git_worktree.go): cliente Git (`ExecGitClient`), snapshot determinista sobre bytes crudos y modo octal (`worktree_tree_sha256`), rechazo de symlinks `120000`, submódulos `160000` y reparse points, manejo NUL de `R`/`C`, diff unificado `-U3` + archivos `??` y `ScanWorktreeChangesG4` (UTF-16LE/BE + 11 reglas + `bfcore.ScanTier1` / `bfcore.RedactTier2Text` solo contexto/`-`).
- [runner.go](./runner.go) (+ `runner_windows.go` / `runner_unix.go`): ejecución `VerifyCommands` en worktree y `SanitizeCommandOutputG4` dentro de `8000` bytes UTF-8 seguros.
- [verify.go](./verify.go): verificación 100% solo lectura (`VerifyRuntime`) con prioridad fija y validación SHA-256 normalizada de los 5 archivos `GengiSWAP/engine` (`RZ-001`).
- [init_cmd.go](./init_cmd.go), [context_cmd.go](./context_cmd.go), [cleanup_cmd.go](./cleanup_cmd.go): ciclo de vida del worktree, sincronización `.ring/state.json` y consultas `0_Brain`.
- [tdd_cmd.go](./tdd_cmd.go), [package_cmd.go](./package_cmd.go), [bounce_cmd.go](./bounce_cmd.go), [commit_cmd.go](./commit_cmd.go): bucle Red -> Green/Package -> ApplyOrder -> Commit anti-bypass sobre `gengis.exe check`.
- [cmd/builder/](./cmd/builder/README.md): binario CLI `builder.exe`.

## Verificación y Compilación
```powershell
Set-Location "C:\Black Factory\3_Builder\engine"
$env:CGO_ENABLED = "0"
$env:GENGIS_CONTRACT_STRICT = "1"
$env:GENGIS_BIN = "C:\Black Factory\GengiSWAP\gengis.exe"
go vet ./...
go test -count=1 -cover ./...
go test -count=1 -v -run TestLatencyBudgetP95 ./...
go build -o ..\builder.exe .\cmd\builder
```
