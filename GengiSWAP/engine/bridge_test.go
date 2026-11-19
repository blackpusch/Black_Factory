package ring

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedClock(start time.Time) func() time.Time {
	cur := start
	return func() time.Time {
		cur = cur.Add(time.Second)
		return cur
	}
}

func TestParseTaskFromDir_InlineAndSectionFormats(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "gengis_task", "tarea_parser")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexMD := `# Mapa de Tarea: tarea_parser — Validación completa

**Objetivo genérico:** Integrar el motor de GengiSWAP con el sistema de archivos.

## Alcance
- Puede modificar: ` + "`" + `C:\Repos\Demo\src\**` + "`" + `, ` + "`" + `docs/guia.md` + "`" + ` (solo actualización)
- Mutación de datos (G3): ninguna
- Solo lectura: ` + "`" + `go.mod` + "`" + `, ` + "`" + `README.md` + "`" + `
- Prohibido: ` + "`" + `.env` + "`" + `, ` + "`" + `secrets/**` + "`" + `

## Directorio de Destino Real
- Repositorio destino: ` + "`" + `C:\Repos\Demo` + "`" + ` (rama main)

## Supuestos y Stack Tecnológico
- **Máx. intentos por chunk:** 4
`
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}

	chunk01 := `# Chunk 01: Configuración inicial
Prepara los archivos base del servicio.

- *Criterio de aceptación:* Todos los tests pasan; cero cambios en secretos.
- *Verificación:* ` + "`" + `go test ./...` + "`" + `
- *Prueba de que puede fallar:* Eliminar go.mod rompe go test con salida distinta de 0.
`
	chunk02 := `# Tarea: Validación de seguridad

## Criterios de Aceptación (R1)
- Rechaza rutas prohibidas
- Verifica archivos en UTF-8

## Comando de Verificación
` + "```powershell\n" + `# comentario ignorado
go vet ./...
go test -run TestScope ./...
` + "```\n" + `

## Prueba de que puede fallar
Aceptar rutas .. hace fallar TestScope con código 1.

## Fuera de Alcance (R3)
- No tocar archivos en ` + "`" + `cmd/**` + "`" + `

## Historial de feedback

### Intento 1 (2026-11-14 10:00:00) — RETRY_CHUNK
- Esto no debe alterar Spec ni VerifyCommands al reparsear.
`
	if err := os.WriteFile(filepath.Join(chunksDir, "02_seguridad.md"), []byte(chunk02), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunksDir, "01_config.md"), []byte(chunk01), 0o644); err != nil {
		t.Fatal(err)
	}

	task, scope, chunks, maxRetries, err := ParseTaskFromDir(taskDir)
	if err != nil {
		t.Fatalf("ParseTaskFromDir falló: %v", err)
	}
	if task.ID != "tarea_parser" {
		t.Fatalf("Task.ID = %q, esperaba tarea_parser", task.ID)
	}
	if maxRetries != 4 {
		t.Fatalf("MaxRetries = %d, esperaba 4", maxRetries)
	}
	repo, err := ParseTargetRepoFromDir(taskDir)
	if err != nil || repo != filepath.FromSlash("C:/Repos/Demo") {
		t.Fatalf("TargetRepo = %q, err=%v", repo, err)
	}
	if !scope.Allows("src/main.go") || !scope.Allows("docs/guia.md") {
		t.Fatalf("alcance debería permitir src/main.go y docs/guia.md: %+v", scope)
	}
	if scope.Allows("go.mod") || scope.Allows(".env") || scope.Allows("secrets/key.txt") {
		t.Fatalf("alcance debería denegar Solo lectura / Prohibido: %+v", scope)
	}

	if err := validateChunks(chunks, nil); err != nil {
		t.Fatalf("validateChunks rechazó chunks válidos: %v", err)
	}
	if len(chunks) != 2 || chunks[0].ID != "01_config" || chunks[1].ID != "02_seguridad" {
		t.Fatalf("orden o IDs de chunks inesperados: %+v", chunks)
	}
	if chunks[0].VerifyCommands[0] != "go test ./..." {
		t.Fatalf("VerifyCommands[0] de chunk 01 = %q", chunks[0].VerifyCommands[0])
	}
	if len(chunks[1].Acceptance) != 2 {
		t.Fatalf("Acceptance de chunk 02 = %v, esperaba 2 ítems", chunks[1].Acceptance)
	}
	if len(chunks[1].VerifyCommands) != 2 || chunks[1].VerifyCommands[0] != "go vet ./..." {
		t.Fatalf("VerifyCommands de chunk 02 = %v", chunks[1].VerifyCommands)
	}
	if chunks[1].Scope.Allows("cmd/app/main.go") {
		t.Fatalf("Fuera de Alcance del chunk 02 debería bloquear cmd/**")
	}
	if strings.Contains(chunks[1].FailureProof, "Historial de feedback") {
		t.Fatalf("el parser no debe mezclar Historial de feedback en FailureProof: %q", chunks[1].FailureProof)
	}
}

func TestParseTaskFromDir_MissingScopeAndMissingFailureProof(t *testing.T) {
	taskDir := t.TempDir()
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte("# Sin alcance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := ParseTaskFromDir(taskDir); err == nil {
		t.Fatal("esperaba error por ausencia de ## Alcance")
	}

	indexValid := "# Mapa de Tarea: demo\n\n## Alcance\n- Puede modificar: `src/**`\n"
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexValid), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkNoProof := "# Chunk 01: Incompleto\n- *Criterio de aceptación:* Ok\n- *Verificación:* `echo ok`\n"
	if err := os.WriteFile(filepath.Join(chunksDir, "01_incompleto.md"), []byte(chunkNoProof), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, chunks, _, err := ParseTaskFromDir(taskDir)
	if err != nil {
		t.Fatalf("ParseTaskFromDir falló inesperadamente: %v", err)
	}
	if err := validateChunks(chunks, nil); !errors.Is(err, ErrInvalidChunk) {
		t.Fatalf("esperaba ErrInvalidChunk por falta de FailureProof, obtuve: %v", err)
	}
}

func TestSyncFiles_AppendOnlyLogAndIdempotentRetryFeedback(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "gengis_task", "t_sync")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialLog := "[2026-11-13 15:56:07] [SISTEMA] LINEA PREVIA INMUTABLE\n"
	logPath := filepath.Join(root, "gengis_global_log.md")
	if err := os.WriteFile(logPath, []byte(initialLog), 0o644); err != nil {
		t.Fatal(err)
	}

	chunkOrig := "# Chunk 01: Base\n- *Criterio de aceptación:* Pasa\n- *Verificación:* `echo ok`\n- *Prueba de que puede fallar:* exit 1\n"
	chunkPath := filepath.Join(chunksDir, "01_base.md")
	if err := os.WriteFile(chunkPath, []byte(chunkOrig), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	st := &State{
		TaskID: "t_sync",
		Chunks: []Chunk{
			{
				ID:     "01_base",
				Spec:   "Base — Implementar base",
				Status: StatusPartial,
				Attempts: []Attempt{
					{
						N:      1,
						QADone: true,
						Evidence: []Evidence{
							{Command: "go test ./...", ExitCode: 1, Output: "FAIL pkg/foo"},
						},
						QANotes: "`go test ./...` salió con código 1",
						Verdict: &Verdict{
							Decision: RetryChunk,
							Comment:  "Corregir caso borde",
							At:       ts.Add(2 * time.Second),
						},
					},
				},
			},
		},
		Events: []Event{
			{At: ts, Role: "PREPARADOR", Kind: "PLAN_PENDIENTE", Detail: "1 chunks"},
			{At: ts.Add(time.Second), Role: "PREPARADOR", Kind: "QA", ChunkID: "01_base", Detail: "Resultado: FAIL | Notas: exit 1"},
			{At: ts.Add(2 * time.Second), Role: "AUTOR", Kind: "DECISION", ChunkID: "01_base", Detail: "Decisión: RETRY_CHUNK | Motivo: Corregir caso borde"},
		},
	}

	if err := SyncFiles(root, taskDir, st, 1); err != nil {
		t.Fatalf("SyncFiles falló: %v", err)
	}

	statusBytes, err := os.ReadFile(filepath.Join(taskDir, "status.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(statusBytes) != RenderStatusMD(st) {
		t.Fatalf("status.md difiere de RenderStatusMD:\n%s", statusBytes)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(logBytes), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("esperaba 3 líneas en log (1 histórica + 2 nuevas), obtuve %d:\n%s", len(lines), logBytes)
	}
	if lines[0] != strings.TrimSuffix(initialLog, "\n") {
		t.Fatalf("se alteró la línea histórica del log: %q", lines[0])
	}
	if !strings.Contains(lines[2], "Decisión: RETRY_CHUNK") {
		t.Fatalf("falta línea RETRY_CHUNK en el log: %q", lines[2])
	}

	chunkAfter1, err := os.ReadFile(chunkPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(chunkAfter1), chunkOrig) {
		t.Fatalf("SyncFiles modificó el contenido previo de chunks\\01_base.md")
	}
	if !strings.Contains(string(chunkAfter1), "## Historial de feedback") ||
		!strings.Contains(string(chunkAfter1), "### Intento 1 (2026-11-14 15:00:02) — RETRY_CHUNK") ||
		!strings.Contains(string(chunkAfter1), "FAIL pkg/foo") {
		t.Fatalf("falta feedback esperado en chunks\\01_base.md:\n%s", chunkAfter1)
	}

	// Segunda sincronización: no debe duplicar líneas de log ni el bloque del Intento 1.
	if err := SyncFiles(root, taskDir, st, len(st.Events)); err != nil {
		t.Fatalf("segunda llamada a SyncFiles falló: %v", err)
	}
	chunkAfter2, _ := os.ReadFile(chunkPath)
	if strings.Count(string(chunkAfter2), "### Intento 1 ") != 1 {
		t.Fatalf("SyncFiles duplicó el feedback del Intento 1:\n%s", chunkAfter2)
	}
	logAfter2, _ := os.ReadFile(logPath)
	if string(logAfter2) != string(logBytes) {
		t.Fatalf("SyncFiles duplicó entradas en gengis_global_log.md")
	}
}

func TestBridgeLifecycle_RealGitRepoPlanCheckDecideStatus(t *testing.T) {
	repo := newGitRepo(t)
	repo.write("src/app.txt", "pendiente\n")
	repo.write("README.md", "docs\n")
	repo.git("add", ".")
	repo.git("commit", "-q", "-m", "base")

	gengisRoot := t.TempDir()
	taskDir := filepath.Join(gengisRoot, "gengis_task", "tarea_e2e")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexMD := "# Mapa de Tarea: tarea_e2e — Prueba E2E\n\n" +
		"## Alcance\n" +
		"- Puede modificar: `src/**`\n" +
		"- Mutación de datos (G3): ninguna\n" +
		"- Solo lectura: `README.md`\n" +
		"- Prohibido: `.env`\n\n" +
		"## Directorio de Destino Real\n" +
		"- Repositorio destino: `" + repo.dir + "`\n\n" +
		"- **Máx. intentos por chunk:** 3\n"
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}

	chunkMD := "# Chunk 01: Actualizar src/app.txt\n\n" +
		"- *Criterio de aceptación:* `src/app.txt` contiene la marca LISTO.\n" +
		"- *Verificación:* `findstr LISTO src\\app.txt`\n" +
		"- *Prueba de que puede fallar:* Sin LISTO en src/app.txt findstr retorna salida 1.\n"
	if err := os.WriteFile(filepath.Join(chunksDir, "01_app.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	clock := fixedClock(time.Date(2026, 11, 14, 16, 0, 0, 0, time.UTC))
	ctx := context.Background()

	// 1. Plan rechazado inicialmente por el Autor.
	var out bytes.Buffer
	err := PlanTask(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		In:         strings.NewReader("n\nfalta detalle\n"),
		Out:        &out,
	})
	if !errors.Is(err, ErrPlanNotApproved) {
		t.Fatalf("esperaba ErrPlanNotApproved, obtuve: %v", err)
	}

	// 2. Plan aprobado por el Autor.
	out.Reset()
	err = PlanTask(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		In:         strings.NewReader("s\nadmirable\n"),
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("PlanTask aprobado falló: %v", err)
	}

	// 3. Intento 1: el archivo no contiene LISTO todavía -> QA FAIL, estado [~] entregado.
	repo.write("src/app.txt", "borrador\n")
	out.Reset()
	if err := CheckChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		Out:        &out,
	}); err != nil {
		t.Fatalf("CheckChunk intento 1 falló: %v", err)
	}

	statusAfterCheck1, _ := os.ReadFile(filepath.Join(taskDir, "status.md"))
	if !strings.Contains(string(statusAfterCheck1), "- [~] **entregado**: `01_app`") {
		t.Fatalf("status.md tras check 1 debería mostrar [~] entregado:\n%s", statusAfterCheck1)
	}
	qaReport1, _ := os.ReadFile(filepath.Join(taskDir, "workspace", "qa", "01_app.md"))
	if !strings.Contains(string(qaReport1), "## Intento 1 — FAIL") {
		t.Fatalf("QA report intento 1 debería registrar FAIL:\n%s", qaReport1)
	}

	// 4. Autor rechaza GO_NEXT inválido y elige RETRY_CHUNK con git revert = sí.
	out.Reset()
	err = DecideChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		In:         strings.NewReader("GO_NEXT\nRETRY_CHUNK\ns\nfalta LISTO\n"),
		Out:        &out,
	}, DecideOptions{})
	if err != nil {
		t.Fatalf("DecideChunk RETRY_CHUNK falló: %v", err)
	}
	if !strings.Contains(out.String(), "no permitida") {
		t.Fatalf("debió avisar que GO_NEXT no está permitido cuando QA=FAIL:\n%s", out.String())
	}
	if got := repo.read("src/app.txt"); strings.TrimSpace(got) != "pendiente" {
		t.Fatalf("git revert de RETRY_CHUNK debió restaurar 'pendiente', obtuve %q", got)
	}
	chunkAfterRetry, _ := os.ReadFile(filepath.Join(chunksDir, "01_app.md"))
	if !strings.Contains(string(chunkAfterRetry), "## Historial de feedback") {
		t.Fatalf("chunk .md debió recibir Historial de feedback:\n%s", chunkAfterRetry)
	}

	// 5. Intento 2: Chef entrega valor correcto -> QA PASS, Autor primero aplica --qa-pass=false? No: prueba PASS + GO_NEXT y verifica --qa-pass en rechazo previo!
	repo.write("src/app.txt", "LISTO\n")
	out.Reset()
	if err := CheckChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		Out:        &out,
	}); err != nil {
		t.Fatalf("CheckChunk intento 2 falló: %v", err)
	}
	qaReport2, _ := os.ReadFile(filepath.Join(taskDir, "workspace", "qa", "01_app.md"))
	if !strings.Contains(string(qaReport2), "## Intento 2 — PASS") {
		t.Fatalf("QA report intento 2 debería registrar PASS:\n%s\nout:\n%s", qaReport2, out.String())
	}

	// Probar override del Preparador (--qa-pass=false) seguido por Retry o confirmación y GO_NEXT:
	// Primero aplicamos --qa-pass=false y RETRY_CHUNK sin revertir, luego --qa-pass=true y GO_NEXT?
	// Mejor: aplicamos --qa-pass=true --qa-notes="G5 ok" + GO_NEXT para cerrar la tarea,
	// y en el test CLI probamos --qa-pass=false bloqueando GO_NEXT.
	out.Reset()
	err = DecideChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		In:         strings.NewReader("GO_NEXT\nverificado completo\n"),
		Out:        &out,
	}, DecideOptions{HasQAPass: true, QAPass: true, HasQANotes: true, QANotes: "G5 confirmado"})
	if err != nil {
		t.Fatalf("DecideChunk GO_NEXT falló: %v", err)
	}

	out.Reset()
	if err := ShowStatus(CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Out:        &out,
	}); err != nil {
		t.Fatalf("ShowStatus falló: %v", err)
	}
	if !strings.Contains(out.String(), "Estado: DONE") {
		t.Fatalf("ShowStatus esperaba Estado: DONE, obtuve:\n%s", out.String())
	}

	finalStatus, _ := os.ReadFile(filepath.Join(taskDir, "status.md"))
	if !strings.Contains(string(finalStatus), "- [x] **hecho**: `01_app`") {
		t.Fatalf("status.md final esperaba [x] hecho:\n%s", finalStatus)
	}

	logContent, _ := os.ReadFile(filepath.Join(gengisRoot, "gengis_global_log.md"))
	for _, needle := range []string{"PLAN_PENDIENTE", "PLAN_RECHAZADO", "PLAN_APROBADO", "ENTREGADO", "RETRY_CHUNK", "GO_NEXT", "MOTOR] FIN"} {
		if !strings.Contains(string(logContent), needle) {
			t.Fatalf("gengis_global_log.md no contiene %q:\n%s", needle, logContent)
		}
	}
}

func TestBridge_ResplitChunkInteractive(t *testing.T) {
	repo := newGitRepo(t)
	repo.write("mod.txt", "base\n")
	repo.git("add", ".")
	repo.git("commit", "-q", "-m", "base")

	gengisRoot := t.TempDir()
	taskDir := filepath.Join(gengisRoot, "gengis_task", "tarea_resplit")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexMD := "# Mapa de Tarea: tarea_resplit\n\n## Alcance\n- Puede modificar: `mod.txt`\n\n## Directorio de Destino Real\n- Repositorio destino: `" + repo.dir + "`\n- **Máx. intentos por chunk:** 2\n"
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkMD := "# Chunk 01: Grande\n- *Criterio de aceptación:* contiene listo.\n- *Verificación:* `findstr LISTO mod.txt`\n- *Prueba de que puede fallar:* sin LISTO retorna 1.\n"
	if err := os.WriteFile(filepath.Join(chunksDir, "01_grande.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	clock := fixedClock(time.Date(2026, 11, 14, 17, 0, 0, 0, time.UTC))
	ctx := context.Background()
	var out bytes.Buffer

	if err := PlanTask(ctx, CLIConfig{GengisRoot: gengisRoot, TaskDir: taskDir, Now: clock, In: strings.NewReader("s\nok\n"), Out: &out}); err != nil {
		t.Fatalf("PlanTask falló: %v", err)
	}

	repo.write("mod.txt", "incompleto\n")
	out.Reset()
	if err := CheckChunk(ctx, CLIConfig{GengisRoot: gengisRoot, TaskDir: taskDir, Now: clock, Out: &out}); err != nil {
		t.Fatalf("CheckChunk falló: %v", err)
	}

	resplitInput := strings.Join([]string{
		"RESPLIT_CHUNK",
		"dividir en dos partes",
		"2",
		"01a_parte", "Primera parte", "archivo listo", "findstr BASE mod.txt", "sin BASE falla",
		"01b_parte", "Segunda parte", "ok", "echo ok", "echo no falla",
	}, "\n") + "\n"

	out.Reset()
	err := DecideChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		In:         strings.NewReader(resplitInput),
		Out:        &out,
	}, DecideOptions{})
	if err != nil {
		t.Fatalf("DecideChunk RESPLIT_CHUNK falló: %v\n%s", err, out.String())
	}

	if got := repo.read("mod.txt"); strings.TrimSpace(got) != "base" {
		t.Fatalf("RESPLIT_CHUNK debió revertir mod.txt a 'base', obtuve %q", got)
	}
	statusBytes, _ := os.ReadFile(filepath.Join(taskDir, "status.md"))
	if !strings.Contains(string(statusBytes), "`01a_parte`") || !strings.Contains(string(statusBytes), "`01b_parte`") {
		t.Fatalf("status.md no refleja los nuevos subchunks:\n%s", statusBytes)
	}
	logBytes, _ := os.ReadFile(filepath.Join(gengisRoot, "gengis_global_log.md"))
	if !strings.Contains(string(logBytes), "[PREPARADOR] REPLAN") || !strings.Contains(string(logBytes), "01_grande → 01a_parte,01b_parte") {
		t.Fatalf("gengis_global_log.md no registró REPLAN:\n%s", logBytes)
	}
}

func TestParseTaskFromDir_SubBulletsInsideH2AndCompoundScope(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "gengis_task", "tarea_h2")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexMD := `# Mapa de Tarea: tarea_h2

## Alcance
- **Puede modificar:**
  - ` + "`app/rutas.py`" + ` (exclusivamente sobre la rama ` + "`Black`" + `)
- **Solo lectura:**
  - ` + "`instance\\app.db` y `instance\\site.db`" + ` (se verifica SHA256)
  - ` + "`.env`" + ` y cualquier instancia externa (G3/G4)

## Directorio de Destino Real
- **Repositorio de implementación:** ` + "`C:\\Repos\\App`" + `
`
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}

	chunkMD := `# Chunk 1: Arreglar enlaces

## Criterios de Aceptación
- Sin enlaces rotos.

## Comando de Verificación
` + "```powershell\n" + `python verify_01.py
` + "```\n" + `

## Prueba de que puede fallar
- **Verificación en línea base (antes de editar):** ejecutar verify_01.py falla hoy con exit code = 1.
- Si persiste cualquier enlace roto, falla inmediatamente.

## Fuera de Alcance
- No modificar ` + "`app/models.py`" + ` ni BD real.
`
	if err := os.WriteFile(filepath.Join(chunksDir, "01_enlaces.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	_, scope, chunks, _, err := ParseTaskFromDir(taskDir)
	if err != nil {
		t.Fatalf("ParseTaskFromDir falló: %v", err)
	}
	if !scope.Allows("app/rutas.py") {
		t.Fatalf("debería permitir app/rutas.py: %+v", scope)
	}
	for _, ro := range []string{"instance/app.db", "instance/site.db", ".env"} {
		if scope.Allows(ro) {
			t.Fatalf("debería proteger %q en Solo lectura: %+v", ro, scope)
		}
	}
	if err := validateChunks(chunks, nil); err != nil {
		t.Fatalf("validateChunks falló: %v", err)
	}
	if len(chunks[0].VerifyCommands) != 1 || chunks[0].VerifyCommands[0] != "python verify_01.py" {
		t.Fatalf("VerifyCommands contaminado por sub-viñeta en FailureProof: %v", chunks[0].VerifyCommands)
	}
	if !strings.Contains(chunks[0].FailureProof, "Verificación en línea base") {
		t.Fatalf("FailureProof vacío o truncado: %q", chunks[0].FailureProof)
	}
}

func TestAbortedStateRejectedConsistentlyAcrossRunAndCLI(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "gengis_task", "t_aborted")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexMD := "# Mapa de Tarea: t_aborted\n\n## Alcance\n- Puede modificar: `src/**`\n"
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkMD := "# Chunk 01: Base\n- *Criterio de aceptación:* Ok\n- *Verificación:* `echo ok`\n- *Prueba de que puede fallar:* exit 1\n"
	if err := os.WriteFile(filepath.Join(chunksDir, "01_base.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, chunks, err := parseTaskFromDir(taskDir)
	if err != nil {
		t.Fatalf("parseTaskFromDir falló: %v", err)
	}
	store := FileStore{Path: filepath.Join(taskDir, ".ring", "state.json")}
	abortedState := &State{
		TaskID:       idx.Task.ID,
		Prompt:       idx.Task.Prompt,
		Scope:        idx.Scope,
		MaxRetries:   idx.MaxRetries,
		PlanApproved: true,
		Status:       "ABORTED",
		Chunks:       chunks,
	}
	if err := store.Save(abortedState); err != nil {
		t.Fatalf("guardando estado abortado: %v", err)
	}

	ctx := context.Background()
	eng := &Engine{
		Store: store,
		WS:    NopWorkspace{},
	}
	_, runErr := eng.Run(ctx, idx.Task, idx.Scope, idx.MaxRetries)

	var checkOut, decideOut bytes.Buffer
	checkErr := CheckChunk(ctx, CLIConfig{
		GengisRoot: root,
		TaskDir:    taskDir,
		WS:         NopWorkspace{},
		Out:        &checkOut,
	})
	decideErr := DecideChunk(ctx, CLIConfig{
		GengisRoot: root,
		TaskDir:    taskDir,
		WS:         NopWorkspace{},
		Out:        &decideOut,
	}, DecideOptions{})

	for label, gotErr := range map[string]error{
		"Engine.Run":  runErr,
		"CheckChunk":  checkErr,
		"DecideChunk": decideErr,
	} {
		if !errors.Is(gotErr, ErrAborted) {
			t.Fatalf("%s devolvió %v, esperaba errors.Is(..., ErrAborted)", label, gotErr)
		}
		if gotErr.Error() != ErrAborted.Error() {
			t.Fatalf("%s mensaje = %q, esperaba %q", label, gotErr.Error(), ErrAborted.Error())
		}
	}
}

type failingRunner struct {
	err error
}

func (r failingRunner) Run(context.Context, string) (Evidence, error) {
	return Evidence{}, r.err
}

func TestCheckChunk_FailureAfterCommitKeepsConsistentStateAndConvergesOnRetry(t *testing.T) {
	repo := newGitRepo(t)
	repo.write("src/app.txt", "base\n")
	repo.git("add", ".")
	repo.git("commit", "-q", "-m", "base")
	baseHead := strings.TrimSpace(repo.git("rev-parse", "HEAD"))

	gengisRoot := t.TempDir()
	taskDir := filepath.Join(gengisRoot, "gengis_task", "t_crash_4c")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexMD := "# Mapa de Tarea: t_crash_4c\n\n## Alcance\n- Puede modificar: `src/**`\n\n## Directorio de Destino Real\n- Repositorio destino: `" + repo.dir + "`\n"
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkMD := "# Chunk 01: Actualizar src/app.txt\n- *Criterio de aceptación:* contiene LISTO.\n- *Verificación:* `findstr LISTO src\\app.txt`\n- *Prueba de que puede fallar:* sin LISTO devuelve código 1.\n"
	if err := os.WriteFile(filepath.Join(chunksDir, "01_app.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	clock := fixedClock(time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC))
	ctx := context.Background()
	var out bytes.Buffer
	if err := PlanTask(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		In:         strings.NewReader("s\naprobado\n"),
		Out:        &out,
	}); err != nil {
		t.Fatalf("PlanTask falló: %v", err)
	}

	statusBeforeFail, _ := os.ReadFile(filepath.Join(taskDir, "status.md"))
	logBeforeFail, _ := os.ReadFile(filepath.Join(gengisRoot, "gengis_global_log.md"))

	// Chef modifica src/app.txt e inyectamos fallo de Runner después de WS.Commit y antes de e.save(st).
	repo.write("src/app.txt", "LISTO\n")
	out.Reset()
	errBoom := errors.New("fallo inyectado tras WS.Commit")
	checkErr1 := CheckChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Runner:     failingRunner{err: errBoom},
		Now:        clock,
		Out:        &out,
	})
	if !errors.Is(checkErr1, errBoom) {
		t.Fatalf("esperaba error %v tras fallo de Runner, obtuve: %v", errBoom, checkErr1)
	}

	store := FileStore{Path: filepath.Join(taskDir, ".ring", "state.json")}
	stAfterFail, err := store.Load()
	if err != nil || stAfterFail == nil {
		t.Fatalf("recargando state.json tras fallo: %v", err)
	}
	if len(stAfterFail.Chunks[0].Attempts) != 0 || stAfterFail.Chunks[0].Status != StatusPending {
		t.Fatalf("state.json no debió mutar sin persistencia completa: status=%s attempts=%d",
			stAfterFail.Chunks[0].Status, len(stAfterFail.Chunks[0].Attempts))
	}

	statusAfterFail, _ := os.ReadFile(filepath.Join(taskDir, "status.md"))
	logAfterFail, _ := os.ReadFile(filepath.Join(gengisRoot, "gengis_global_log.md"))
	if string(statusAfterFail) != RenderStatusMD(stAfterFail) || string(statusAfterFail) != string(statusBeforeFail) {
		t.Fatalf("status.md quedó desincronizado respecto a state.json:\n%s", statusAfterFail)
	}
	if string(logAfterFail) != string(logBeforeFail) || strings.Contains(string(logAfterFail), "ENTREGADO") {
		t.Fatalf("gengis_global_log.md registró eventos no persistidos:\n%s", logAfterFail)
	}

	// Reintento limpio de CheckChunk: debe converger sin duplicar commit ni línea ENTREGADO.
	out.Reset()
	if err := CheckChunk(ctx, CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		Now:        clock,
		Out:        &out,
	}); err != nil {
		t.Fatalf("segundo CheckChunk falló: %v\n%s", err, out.String())
	}

	stFinal, err := store.Load()
	if err != nil || stFinal == nil {
		t.Fatalf("recargando state.json tras reintento: %v", err)
	}
	if len(stFinal.Chunks[0].Attempts) != 1 || stFinal.Chunks[0].Status != StatusDelivered {
		t.Fatalf("estado esperado DELIVERED con 1 intento, obtuve status=%s attempts=%d",
			stFinal.Chunks[0].Status, len(stFinal.Chunks[0].Attempts))
	}
	if cnt := strings.TrimSpace(repo.git("rev-list", "--count", baseHead+"..HEAD")); cnt != "1" {
		t.Fatalf("esperaba exactamente 1 commit en base..HEAD, obtuve %s", cnt)
	}
	headNow := strings.TrimSpace(repo.git("rev-parse", "HEAD"))
	if stFinal.Chunks[0].Attempts[0].CommitRef != headNow {
		t.Fatalf("CommitRef = %q, esperaba HEAD = %q", stFinal.Chunks[0].Attempts[0].CommitRef, headNow)
	}

	statusFinal, _ := os.ReadFile(filepath.Join(taskDir, "status.md"))
	if string(statusFinal) != RenderStatusMD(stFinal) {
		t.Fatalf("status.md final no coincide con state.json")
	}
	logFinal, _ := os.ReadFile(filepath.Join(gengisRoot, "gengis_global_log.md"))
	if got := strings.Count(string(logFinal), "Estado: ENTREGADO"); got != 1 {
		t.Fatalf("esperaba exactamente 1 línea '[CHEF] ... ENTREGADO' en el log, obtuve %d:\n%s", got, logFinal)
	}
}

func TestUnapprovedPlanRejectedInCheckAndDecide(t *testing.T) {
	root := t.TempDir()
	taskDir := filepath.Join(root, "gengis_task", "t_unapproved")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte("# Mapa de Tarea: t_unapproved\n\n## Alcance\n- Puede modificar: `src/**`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chunksDir, "01_base.md"), []byte("# Chunk 01\n- *Criterio de aceptación:* Ok\n- *Verificación:* `echo ok`\n- *Prueba de que puede fallar:* exit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var out bytes.Buffer
	planErr := PlanTask(ctx, CLIConfig{
		GengisRoot: root,
		TaskDir:    taskDir,
		In:         strings.NewReader("n\nfalta revisar\n"),
		Out:        &out,
	})
	if !errors.Is(planErr, ErrPlanNotApproved) {
		t.Fatalf("PlanTask rechazado devolvió %v, esperaba ErrPlanNotApproved", planErr)
	}

	checkErr := CheckChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, WS: NopWorkspace{}, Out: &out})
	decideErr := DecideChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, WS: NopWorkspace{}, Out: &out}, DecideOptions{})
	if !errors.Is(checkErr, ErrPlanNotApproved) {
		t.Fatalf("CheckChunk devolvió %v, esperaba ErrPlanNotApproved", checkErr)
	}
	if !errors.Is(decideErr, ErrPlanNotApproved) {
		t.Fatalf("DecideChunk devolvió %v, esperaba ErrPlanNotApproved", decideErr)
	}
}

func TestDecideChunk_WithAndWithoutExplicitTargetRepo(t *testing.T) {
	cases := []struct {
		name           string
		omitRepoInMD   bool
		explicitTarget bool
	}{
		{name: "sin_target_repo_resuelve_desde_index", omitRepoInMD: false, explicitTarget: false},
		{name: "con_target_repo_explicito_omite_lectura_index", omitRepoInMD: true, explicitTarget: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newGitRepo(t)
			repo.write("app.txt", "original\n")
			repo.git("add", ".")
			repo.git("commit", "-q", "-m", "base")

			root := t.TempDir()
			taskDir := filepath.Join(root, "gengis_task", "t_"+tc.name)
			chunksDir := filepath.Join(taskDir, "chunks")
			if err := os.MkdirAll(chunksDir, 0o755); err != nil {
				t.Fatal(err)
			}

			indexMD := "# Mapa de Tarea: t_target\n\n## Alcance\n- Puede modificar: `app.txt`\n"
			if !tc.omitRepoInMD {
				indexMD += "\n## Directorio de Destino Real\n- Repositorio destino: `" + repo.dir + "`\n"
			}
			if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
				t.Fatal(err)
			}
			chunkMD := "# Chunk 01: App\n- *Criterio de aceptación:* contiene LISTO.\n- *Verificación:* `findstr LISTO app.txt`\n- *Prueba de que puede fallar:* sin LISTO falla.\n"
			if err := os.WriteFile(filepath.Join(chunksDir, "01_app.md"), []byte(chunkMD), 0o644); err != nil {
				t.Fatal(err)
			}

			targetArg := ""
			if tc.explicitTarget {
				targetArg = repo.dir
			}
			ctx := context.Background()
			clock := fixedClock(time.Date(2026, 11, 18, 17, 10, 0, 0, time.UTC))
			var out bytes.Buffer

			if err := PlanTask(ctx, CLIConfig{
				GengisRoot: root,
				TaskDir:    taskDir,
				TargetRepo: targetArg,
				Now:        clock,
				In:         strings.NewReader("s\nok\n"),
				Out:        &out,
			}); err != nil {
				t.Fatalf("PlanTask falló: %v", err)
			}

			repo.write("app.txt", "incompleto\n")
			if err := CheckChunk(ctx, CLIConfig{
				GengisRoot: root,
				TaskDir:    taskDir,
				TargetRepo: targetArg,
				Now:        clock,
				Out:        &out,
			}); err != nil {
				t.Fatalf("CheckChunk falló: %v", err)
			}

			if tc.explicitTarget {
				// Prueba que DecideChunk con TargetRepo explícito no depende de index.md para resolver repo.
				if err := os.Remove(filepath.Join(taskDir, "index.md")); err != nil {
					t.Fatal(err)
				}
			}

			out.Reset()
			if err := DecideChunk(ctx, CLIConfig{
				GengisRoot: root,
				TaskDir:    taskDir,
				TargetRepo: targetArg,
				Now:        clock,
				In:         strings.NewReader("RETRY_CHUNK\ns\nreintentar\n"),
				Out:        &out,
			}, DecideOptions{}); err != nil {
				t.Fatalf("DecideChunk falló: %v", err)
			}

			if got := strings.TrimSpace(repo.read("app.txt")); got != "original" {
				t.Fatalf("RETRY_CHUNK con revert debió restaurar 'original', obtuve %q", got)
			}
		})
	}
}

func TestNoGitWarningEmittedInPlanCheckDecide(t *testing.T) {
	ctx := context.Background()
	chunkMD := "# Chunk 01: Base\n- *Criterio de aceptación:* Ok\n- *Verificación:* `echo ok`\n- *Prueba de que puede fallar:* cambiar comando a exit 1 falla.\n"

	setupTask := func(t *testing.T, id, indexMD string) (string, string) {
		t.Helper()
		root := t.TempDir()
		taskDir := filepath.Join(root, "gengis_task", id)
		chunksDir := filepath.Join(taskDir, "chunks")
		if err := os.MkdirAll(chunksDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(chunksDir, "01_base.md"), []byte(chunkMD), 0o644); err != nil {
			t.Fatal(err)
		}
		return root, taskDir
	}

	t.Run("destino_no_es_git_avisa_en_plan_check_decide", func(t *testing.T) {
		nonGitDir := t.TempDir()
		indexMD := "# Mapa de Tarea: t_nogit\n\n## Alcance\n- Puede modificar: `src/**`\n\n## Directorio de Destino Real\n- Repositorio destino: `" + nonGitDir + "`\n"
		root, taskDir := setupTask(t, "t_nogit", indexMD)
		want := "ADVERTENCIA: destino sin Git (" + nonGitDir + ") → alcance NO verificado, sin commit ni revert."

		var out bytes.Buffer
		if err := PlanTask(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, In: strings.NewReader("s\nok\n"), Out: &out}); err != nil {
			t.Fatalf("PlanTask falló: %v", err)
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("PlanTask debió emitir %q, obtuve:\n%s", want, out.String())
		}

		out.Reset()
		if err := CheckChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, Out: &out}); err != nil {
			t.Fatalf("CheckChunk falló: %v", err)
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("CheckChunk debió emitir %q, obtuve:\n%s", want, out.String())
		}

		out.Reset()
		if err := DecideChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, In: strings.NewReader("GO_NEXT\nok\n"), Out: &out}, DecideOptions{}); err != nil {
			t.Fatalf("DecideChunk falló: %v", err)
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("DecideChunk debió emitir %q, obtuve:\n%s", want, out.String())
		}
	})

	t.Run("sin_destino_configurado_avisa_en_plan", func(t *testing.T) {
		indexMD := "# Mapa de Tarea: t_norepo\n\n## Alcance\n- Puede modificar: `workspace/**`\n"
		root, taskDir := setupTask(t, "t_norepo", indexMD)
		want := "ADVERTENCIA: destino sin Git (repositorio no configurado o no resuelto) → alcance NO verificado, sin commit ni revert."

		var out bytes.Buffer
		if err := PlanTask(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, In: strings.NewReader("s\nok\n"), Out: &out}); err != nil {
			t.Fatalf("PlanTask falló: %v", err)
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("PlanTask debió emitir %q, obtuve:\n%s", want, out.String())
		}
	})

	t.Run("repo_git_real_no_emite_advertencia", func(t *testing.T) {
		repo := newGitRepo(t)
		repo.write("app.txt", "ok\n")
		repo.git("add", ".")
		repo.git("commit", "-q", "-m", "base")

		indexMD := "# Mapa de Tarea: t_git\n\n## Alcance\n- Puede modificar: `app.txt`\n\n## Directorio de Destino Real\n- Repositorio destino: `" + repo.dir + "`\n"
		root, taskDir := setupTask(t, "t_git", indexMD)

		var out bytes.Buffer
		if err := PlanTask(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, In: strings.NewReader("s\nok\n"), Out: &out}); err != nil {
			t.Fatalf("PlanTask falló: %v", err)
		}
		if strings.Contains(out.String(), "ADVERTENCIA: destino sin Git") {
			t.Fatalf("PlanTask con repo Git no debió emitir advertencia:\n%s", out.String())
		}

		out.Reset()
		if err := CheckChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, Out: &out}); err != nil {
			t.Fatalf("CheckChunk falló: %v", err)
		}
		if strings.Contains(out.String(), "ADVERTENCIA: destino sin Git") {
			t.Fatalf("CheckChunk con repo Git no debió emitir advertencia:\n%s", out.String())
		}

		out.Reset()
		if err := DecideChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, In: strings.NewReader("GO_NEXT\nok\n"), Out: &out}, DecideOptions{}); err != nil {
			t.Fatalf("DecideChunk falló: %v", err)
		}
		if strings.Contains(out.String(), "ADVERTENCIA: destino sin Git") {
			t.Fatalf("DecideChunk con repo Git no debió emitir advertencia:\n%s", out.String())
		}
	})
}

func TestPrintPlanSummaryWarnings(t *testing.T) {
	t.Run("plan_sano_sin_advertencias", func(t *testing.T) {
		var out bytes.Buffer
		st := &State{
			TaskID:     "t_sano",
			MaxRetries: 3,
			Scope:      Scope{CanModify: []string{"src"}, ReadOnly: []string{"README.md"}, Forbidden: []string{".env"}},
			Chunks:     []Chunk{{ID: "01_a", Spec: "Paso A", VerifyCommands: []string{"echo ok"}, FailureProof: "exit 1"}},
		}
		printPlanSummary(&out, st, `C:\Repos\Demo`)
		if strings.Contains(out.String(), "ADVERTENCIA:") {
			t.Fatalf("plan sano no debió emitir advertencias:\n%s", out.String())
		}
	})

	t.Run("sin_repo_canmodify_vacio_y_rutas_absolutas", func(t *testing.T) {
		var out bytes.Buffer
		st := &State{
			TaskID:     "t_warn",
			MaxRetries: 3,
			Scope: Scope{
				ReadOnly:  []string{"C:/Otro/a.go", "/etc/hosts"},
				Forbidden: []string{"C:/Otro/a.go"},
			},
			Chunks: []Chunk{{ID: "01_a", Spec: "Paso A", VerifyCommands: []string{"echo ok"}, FailureProof: "exit 1"}},
		}
		printPlanSummary(&out, st, "")
		got := out.String()
		for _, want := range []string{
			"ADVERTENCIA: no se detectó repositorio destino en index.md",
			"ADVERTENCIA: 'Puede modificar' quedó vacío en ## Alcance",
			"ADVERTENCIA: rutas absolutas fuera del repositorio destino en ## Alcance (no coinciden con rutas relativas de Git): [C:/Otro/a.go /etc/hosts]",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("faltó advertencia %q en:\n%s", want, got)
			}
		}
	})

	t.Run("plan_task_emite_advertencias_antes_de_aprobar", func(t *testing.T) {
		root := t.TempDir()
		repoDir := t.TempDir()
		taskDir := filepath.Join(root, "gengis_task", "t_b4")
		chunksDir := filepath.Join(taskDir, "chunks")
		if err := os.MkdirAll(chunksDir, 0o755); err != nil {
			t.Fatal(err)
		}
		indexMD := "# Mapa de Tarea: t_b4\n\n## Alcance\n- Puede modificar: ninguno\n- Solo lectura: `C:\\Externo\\secreto.txt`\n\n## Directorio de Destino Real\n- Repositorio destino: `" + repoDir + "`\n"
		if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
			t.Fatal(err)
		}
		chunkMD := "# Chunk 01: Base\n- *Criterio de aceptación:* Ok\n- *Verificación:* `echo ok`\n- *Prueba de que puede fallar:* exit 1.\n"
		if err := os.WriteFile(filepath.Join(chunksDir, "01_base.md"), []byte(chunkMD), 0o644); err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		if err := PlanTask(context.Background(), CLIConfig{
			GengisRoot: root,
			TaskDir:    taskDir,
			In:         strings.NewReader("s\nok\n"),
			Out:        &out,
		}); err != nil {
			t.Fatalf("PlanTask falló: %v", err)
		}
		got := out.String()
		promptIdx := strings.Index(got, "¿Aprobar el plan de t_b4?")
		for _, want := range []string{
			"ADVERTENCIA: 'Puede modificar' quedó vacío en ## Alcance",
			"C:/Externo/secreto.txt",
		} {
			idx := strings.Index(got, want)
			if idx < 0 || idx > promptIdx {
				t.Fatalf("se esperaba %q antes del prompt de aprobación:\n%s", want, got)
			}
		}
	})
}

func TestAutoVerifyNoteAndG5ReminderInDecide(t *testing.T) {
	ctx := context.Background()
	setupCheckedTask := func(t *testing.T, id string) (string, string) {
		t.Helper()
		root := t.TempDir()
		taskDir := filepath.Join(root, "gengis_task", id)
		chunksDir := filepath.Join(taskDir, "chunks")
		if err := os.MkdirAll(chunksDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte("# Mapa de Tarea: "+id+"\n\n## Alcance\n- Puede modificar: `src/**`\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(chunksDir, "01_base.md"), []byte("# Chunk 01: Base\n- *Criterio de aceptación:* Ok\n- *Verificación:* `echo ok`\n- *Prueba de que puede fallar:* exit 1.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := PlanTask(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, WS: NopWorkspace{}, In: strings.NewReader("s\nok\n"), Out: &out}); err != nil {
			t.Fatalf("PlanTask falló: %v", err)
		}
		out.Reset()
		if err := CheckChunk(ctx, CLIConfig{GengisRoot: root, TaskDir: taskDir, WS: NopWorkspace{}, Out: &out}); err != nil {
			t.Fatalf("CheckChunk falló: %v", err)
		}
		return root, taskDir
	}

	const reminder = "ADVERTENCIA: este intento no tiene revisión G5 manual del Preparador"

	t.Run("check_registra_nota_g5_pendiente_y_decide_sin_flags_avisa", func(t *testing.T) {
		root, taskDir := setupCheckedTask(t, "t_g5_warn")
		qaReport, err := os.ReadFile(filepath.Join(taskDir, "workspace", "qa", "01_base.md"))
		if err != nil {
			t.Fatalf("leyendo informe QA: %v", err)
		}
		if !strings.Contains(string(qaReport), autoVerifyQANote) {
			t.Fatalf("informe QA debía incluir %q:\n%s", autoVerifyQANote, qaReport)
		}

		var out bytes.Buffer
		if err := DecideChunk(ctx, CLIConfig{
			GengisRoot: root,
			TaskDir:    taskDir,
			WS:         NopWorkspace{},
			In:         strings.NewReader("GO_NEXT\nok\n"),
			Out:        &out,
		}, DecideOptions{}); err != nil {
			t.Fatalf("DecideChunk falló: %v", err)
		}
		if !strings.Contains(out.String(), reminder) {
			t.Fatalf("DecideChunk sin --qa-* debía emitir recordatorio G5:\n%s", out.String())
		}
	})

	t.Run("decide_con_flags_qa_no_emite_recordatorio", func(t *testing.T) {
		root, taskDir := setupCheckedTask(t, "t_g5_ok")
		var out bytes.Buffer
		if err := DecideChunk(ctx, CLIConfig{
			GengisRoot: root,
			TaskDir:    taskDir,
			WS:         NopWorkspace{},
			In:         strings.NewReader("GO_NEXT\nok\n"),
			Out:        &out,
		}, DecideOptions{HasQAPass: true, QAPass: true, HasQANotes: true, QANotes: "G5 revisado"}); err != nil {
			t.Fatalf("DecideChunk falló: %v", err)
		}
		if strings.Contains(out.String(), reminder) {
			t.Fatalf("DecideChunk con --qa-pass/--qa-notes no debía emitir recordatorio G5:\n%s", out.String())
		}
		qaReport, _ := os.ReadFile(filepath.Join(taskDir, "workspace", "qa", "01_base.md"))
		if !strings.Contains(string(qaReport), "G5 revisado") {
			t.Fatalf("informe QA debía registrar las notas manuales:\n%s", qaReport)
		}
	})

	t.Run("state_heredado_con_nota_anterior_tambien_avisa", func(t *testing.T) {
		root, taskDir := setupCheckedTask(t, "t_g5_legacy")
		store := FileStore{Path: filepath.Join(taskDir, ".ring", "state.json")}
		st, err := store.Load()
		if err != nil || st == nil {
			t.Fatalf("cargando state.json: %v", err)
		}
		st.Chunks[0].Attempts[0].PrepNotes = "verificación automática OK"
		st.Chunks[0].Attempts[0].QANotes = "verificación automática OK"
		if err := store.Save(st); err != nil {
			t.Fatalf("guardando state.json legado: %v", err)
		}

		var out bytes.Buffer
		if err := DecideChunk(ctx, CLIConfig{
			GengisRoot: root,
			TaskDir:    taskDir,
			WS:         NopWorkspace{},
			In:         strings.NewReader("GO_NEXT\nok\n"),
			Out:        &out,
		}, DecideOptions{}); err != nil {
			t.Fatalf("DecideChunk legado falló: %v", err)
		}
		if !strings.Contains(out.String(), reminder) {
			t.Fatalf("DecideChunk con nota legada debía emitir recordatorio G5:\n%s", out.String())
		}
	})
}
