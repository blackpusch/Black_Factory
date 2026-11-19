package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_PlanCheckDecideQAFailAndAbort(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no disponible")
	}

	repoDir := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(repoDir, func(p string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() {
				_ = os.Chmod(p, 0o666)
			}
			return nil
		})
	})

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t.t")
	runGit("config", "user.name", "t")
	runGit("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repoDir, "app.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-q", "-m", "init")

	root := t.TempDir()
	taskDir := filepath.Join(root, "gengis_task", "cli_demo")
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	indexMD := "# Mapa de Tarea: cli_demo\n\n## Alcance\n- Puede modificar: `app.txt`\n\n## Directorio de Destino Real\n- Repositorio destino: `" + repoDir + "`\n- **Máx. intentos por chunk:** 2\n"
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkMD := "# Chunk 01: CLI test\n- *Criterio de aceptación:* app.txt válido\n- *Verificación:* `findstr ok app.txt`\n- *Prueba de que puede fallar:* borrar app.txt falla findstr.\n"
	if err := os.WriteFile(filepath.Join(chunksDir, "01_cli.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer

	// plan cli_demo --root=...
	if err := run([]string{"plan", "cli_demo", "--root", root}, strings.NewReader("s\nok plan\n"), &out, &errOut); err != nil {
		t.Fatalf("plan falló: %v\n%s", err, errOut.String())
	}

	// check cli_demo
	out.Reset()
	errOut.Reset()
	if err := os.WriteFile(filepath.Join(repoDir, "app.txt"), []byte("ok v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"check", "--root=" + root, "cli_demo"}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("check falló: %v\n%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "QA de 01_cli (intento 1): PASS") {
		t.Fatalf("check debió pasar antes de --qa-pass=false:\n%s", out.String())
	}

	// decide cli_demo --qa-pass=false --qa-notes="G5 observó detalle" -> intenta GO_NEXT (rechazado) y luego ABORT
	out.Reset()
	errOut.Reset()
	err := run(
		[]string{"decide", "cli_demo", "--root=" + root, "--qa-pass=false", "--qa-notes=G5 observo detalle"},
		strings.NewReader("GO_NEXT\nABORT\ncancelado por demo\n"),
		&out,
		&errOut,
	)
	if err == nil || !strings.Contains(err.Error(), "anillo abortado por el Autor") {
		t.Fatalf("esperaba ErrAborted, obtuve err=%v, out=%s", err, out.String())
	}
	if !strings.Contains(out.String(), "no permitida") {
		t.Fatalf("debió bloquear GO_NEXT cuando --qa-pass=false:\n%s", out.String())
	}

	qaBytes, _ := os.ReadFile(filepath.Join(taskDir, "workspace", "qa", "01_cli.md"))
	if !strings.Contains(string(qaBytes), "FAIL") || !strings.Contains(string(qaBytes), "G5 observo detalle") {
		t.Fatalf("QA report debió actualizarse a FAIL con notas del flag:\n%s", qaBytes)
	}

	logBytes, _ := os.ReadFile(filepath.Join(root, "gengis_global_log.md"))
	if !strings.Contains(string(logBytes), "Decisión: ABORT") {
		t.Fatalf("gengis_global_log.md debió registrar la decisión ABORT:\n%s", logBytes)
	}

	// status cli_demo --root=...
	out.Reset()
	errOut.Reset()
	if err := run([]string{"status", "cli_demo", "--root=" + root}, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("status falló: %v", err)
	}
	if !strings.Contains(out.String(), "Estado: ABORTED") {
		t.Fatalf("status esperaba Estado: ABORTED:\n%s", out.String())
	}
}
