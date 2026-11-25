package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"blackfactory/builder"
	"blackfactory/pkg/bfcore"
)

type stubBrain struct {
	prefErr error
}

func (s *stubBrain) Preflight(vaultRoot, _ string) (bfcore.BrainPreflight, error) {
	if s.prefErr != nil {
		return bfcore.BrainPreflight{}, s.prefErr
	}
	return bfcore.BrainPreflight{VaultRoot: vaultRoot, BrainDBMTime: "2026-11-24T00:00:00Z"}, nil
}

func (s *stubBrain) Context(_, _, _ string, _ []string) (bfcore.BrainContextResponse, bfcore.BrainInvocation, error) {
	return bfcore.BrainContextResponse{
		MatchedDocs: []bfcore.BrainDocMatch{{ID: "D1", RelPath: "doc.md", Type: "spec", Status: "active"}},
	}, bfcore.BrainInvocation{Command: "0_Brain/brain.exe context --json"}, nil
}

type stubRunner struct {
	fail bool
}

func (s *stubRunner) Run(_ context.Context, _, cmd string) (builder.Evidence, error) {
	if s.fail {
		return builder.Evidence{Command: cmd, ExitCode: 1, Output: "--- FAIL: TestCLI\n", DurationMs: 5}, nil
	}
	return builder.Evidence{Command: cmd, ExitCode: 0, Output: "ok pkg 0.01s\n", DurationMs: 5}, nil
}

func registerWritable(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
	})
}

func initGit(t *testing.T, repo string) string {
	t.Helper()
	registerWritable(t, repo)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init")
	runGit("config", "user.email", "builder@example.com")
	runGit("config", "user.name", "Builder Test")
	_ = os.MkdirAll(filepath.Join(repo, "pkg"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, "pkg", "core.go"), []byte("package pkg\nfunc Core() int { return 1 }\n"), 0o644)
	_ = os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("3_Builder/runtime/\n3_Builder/builder.exe\n"), 0o644)
	for _, rel := range []string{
		"GengiSWAP/engine/bridge.go",
		"GengiSWAP/engine/ring.go",
		"GengiSWAP/engine/runner.go",
		"GengiSWAP/engine/runner_windows.go",
		"GengiSWAP/engine/cmd/gengis/main.go",
	} {
		src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		dst := filepath.Join(repo, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, src, 0o644)
	}
	runGit("add", ".")
	runGit("commit", "-m", "init")
	return runGit("rev-parse", "HEAD")
}

func buildMockGengis(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "gengis"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	src := `package main
import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
func main() {
	taskDir := os.Args[2]
	wtDir := os.Getenv("CLI_TEST_WT")
	_ = exec.Command("git", "-C", wtDir, "add", "-A").Run()
	_ = exec.Command("git", "-C", wtDir, "commit", "-m", "chunk commit").Run()
	out, _ := exec.Command("git", "-C", wtDir, "rev-parse", "HEAD").Output()
	commitRef := strings.TrimSpace(string(out))
	stPath := filepath.Join(taskDir, ".ring", "state.json")
	raw, _ := os.ReadFile(stPath)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	chunks := obj["chunks"].([]any)
	idx := int(obj["index"].(float64))
	ch := chunks[idx].(map[string]any)
	ch["attempts"] = []any{map[string]any{
		"n": 1, "commit_ref": commitRef, "qa_done": true, "qa_passed": true, "qa_notes": "CLI PASS",
	}}
	outB, _ := json.MarshalIndent(obj, "", "  ")
	_ = os.WriteFile(stPath, outB, 0o644)
}
`
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module mockgengis\n\ngo 1.26.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mock gengis: %v\n%s", err, string(out))
	}
	return bin
}

func TestCLIUsageHelpAndValidation(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut, CLIHooks{}); code != builder.ExitUsage {
		t.Fatalf("expected ExitUsage on empty args, got %d", code)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"--help"}, &out, &errOut, CLIHooks{}); code != builder.ExitOK || !strings.Contains(out.String(), "apply-order") {
		t.Fatalf("expected help OK, got %d %s", code, out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"unknown-sub"}, &out, &errOut, CLIHooks{}); code != builder.ExitUsage {
		t.Fatalf("expected ExitUsage on unknown sub, got %d", code)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"verify", "--unknown-flag", "--json=true"}, &out, &errOut, CLIHooks{}); code != builder.ExitUsage || !strings.Contains(out.String(), builder.ReasonInvalidUsage) {
		t.Fatalf("expected JSON usage error on unknown flag, got %d %s", code, out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"verify", "extra-pos"}, &out, &errOut, CLIHooks{}); code != builder.ExitUsage {
		t.Fatalf("expected ExitUsage on positional arg, got %d", code)
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"apply-order", "--task-dir", "t", "--order", "a.json", "--reset-bounces", "--reason", "12345678"}, &out, &errOut, CLIHooks{}); code != builder.ExitUsage {
		t.Fatalf("expected ExitUsage on conflicting apply-order flags, got %d", code)
	}
}

func TestCLIEndToEndCUJ1ThroughCUJ4(t *testing.T) {
	tmp := t.TempDir()
	factory := filepath.Join(tmp, "factory")
	headSHA := initGit(t, factory)
	vault := filepath.Join(factory, "0_Brain", "vault")
	_ = os.MkdirAll(filepath.Join(vault, "_meta"), 0o755)
	_ = os.WriteFile(filepath.Join(vault, "_meta", "risk_zones.json"), []byte(`{"zones":[]}`), 0o644)
	_ = os.WriteFile(filepath.Join(vault, "doc.md"), []byte("# Doc\n"), 0o644)

	dataDir := filepath.Join(factory, "3_Builder", "runtime")
	wtRoot := filepath.Join(tmp, "worktrees")
	registerWritable(t, wtRoot)
	taskDir := filepath.Join(tmp, "gengis_data", "gengis_task", "bf_cli_01")
	_ = os.MkdirAll(filepath.Join(taskDir, "chunks"), 0o755)
	_ = os.MkdirAll(filepath.Join(taskDir, ".ring"), 0o755)
	_ = os.WriteFile(filepath.Join(taskDir, "index.md"), []byte("# Mapa de Tarea: bf_cli_01 — CLI Test\nPlantilla: modulo_nuevo v1\n\n## Directorio de Destino Real\n- **Repositorio destino:** "+factory+"\n- **Rama destino:** master\n\n## Alcance\n- Puede modificar: `pkg/`\n\n## Notas Post-Merge y Trazabilidad\n- **WorkItem:** `WI-112233445566` (rev 1)\n- **Trace ID:** `trc-cli-01`\n- **Carril:** `FTD`\n- **Spec SHA-256:** `"+strings.Repeat("a", 64)+"`\n"), 0o644)
	_ = os.WriteFile(filepath.Join(taskDir, "chunks", "01_core.md"), []byte("# Chunk 01\n## Objetivo\nx\n## Puede modificar\n- `pkg/`\n## Criterios de Aceptación\n- ok\n## Comando de Verificación\n```cmd\ngo test ./...\n```\n## Prueba de que puede fallar\nfail\n"), 0o644)
	_ = os.WriteFile(filepath.Join(taskDir, ".ring", "base_ref"), []byte(headSHA+"\n"), 0o644)
	rs, _ := json.Marshal(builder.RingState{
		TaskID: "bf_cli_01", PlanApproved: true, Index: 0, Status: "RUNNING",
		Scope: builder.Scope{CanModify: []string{"pkg/"}},
		Chunks: []builder.RingChunk{{
			ID: "01_core", Spec: "core", Acceptance: []string{"ok"},
			VerifyCommands: []string{"go test ./..."}, FailureProof: "fail",
			Scope: builder.Scope{CanModify: []string{"pkg/"}}, Status: "PENDING",
		}},
	})
	_ = os.WriteFile(filepath.Join(taskDir, ".ring", "state.json"), rs, 0o644)

	hooks := CLIHooks{BrainClient: &stubBrain{}, Runner: &stubRunner{fail: true}}
	invoke := func(args ...string) (string, string, int) {
		t.Helper()
		var out, errOut bytes.Buffer
		c := run(args, &out, &errOut, hooks)
		return out.String(), errOut.String(), c
	}

	// verify (human + JSON)
	if out, _, code := invoke("verify", "--factory-root", factory, "--repo-root", factory, "--vault", vault, "--data-dir", dataDir); code != builder.ExitOK || !strings.Contains(out, "Verify OK=true") {
		t.Fatalf("verify human failed: %d %s", code, out)
	}
	if out, _, code := invoke("verify", "--factory-root", factory, "--repo-root", factory, "--vault", vault, "--data-dir", dataDir, "--json"); code != builder.ExitOK || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("verify json failed: %d %s", code, out)
	}

	// verify FAIL formatting check
	hooks.BrainClient = &stubBrain{prefErr: bfcore.ErrBrainUnavailable}
	if _, errStr, code := invoke("verify", "--factory-root", factory, "--repo-root", factory, "--vault", vault, "--data-dir", dataDir); code != builder.ExitInfraOrRefusal || !strings.Contains(errStr, "Verify FAIL") {
		t.Fatalf("verify fail format mismatch: %d %s", code, errStr)
	}
	hooks.BrainClient = &stubBrain{}

	// init -> context -> tdd-red
	if out, _, code := invoke("init", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--vault", vault, "--json"); code != builder.ExitOK || !strings.Contains(out, `"build_branch": "build/bf_cli_01"`) {
		t.Fatalf("init failed: %d %s", code, out)
	}
	if out, _, code := invoke("context", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--vault", vault, "--refresh-brain", "--json"); code != builder.ExitOK || !strings.Contains(out, `"chunk_id": "01_core"`) {
		t.Fatalf("context json failed: %d %s", code, out)
	}
	if out, _, code := invoke("context", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--vault", vault); code != builder.ExitOK || !strings.Contains(out, "Context OK:") {
		t.Fatalf("context human failed: %d %s", code, out)
	}

	wtDir := filepath.Join(wtRoot, "bf_cli_01")
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "core_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestCore(t *testing.T){}\n"), 0o644)
	if out, _, code := invoke("tdd-red", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--timeout", "60"); code != builder.ExitOK || !strings.Contains(out, "TDD-Red OK:") {
		t.Fatalf("tdd-red failed: %d %s", code, out)
	}

	// package -> apply-order(APPROVE)
	hooks.Runner = &stubRunner{fail: false}
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "core.go"), []byte("package pkg\nfunc Core() int { return 2 }\n"), 0o644)
	pkgOut, _, pkgCode := invoke("package", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--vault", vault, "--include-ux=true", "--json")
	if pkgCode != builder.ExitOK {
		t.Fatalf("package failed: %d %s", pkgCode, pkgOut)
	}
	var pkgRes builder.CLIResult
	_ = json.Unmarshal([]byte(pkgOut), &pkgRes)

	ord := builder.BounceOrder{
		SchemaVersion: builder.SchemaVersion, OrderID: "ORD-CLI-1", CreatedAt: "2026-11-24T12:00:00Z",
		TraceID: "trc-cli-01", TaskID: "bf_cli_01", ChunkID: "01_core",
		BundleID: pkgRes.BundleID, BundleSHA256: pkgRes.BundleSHA256, Decision: "APPROVE",
		ReviewerVerdicts: builder.ReviewerVerdicts{
			Security:         builder.AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"},
			CodeArchitecture: builder.AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"},
			UX:               builder.AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"},
			UI:               builder.AxisVerdict{Status: "NA", FindingsCount: 0, Summary: "na", NAReason: "no ui"},
		},
		RequiresRedTest: false, RemediationItems: []builder.RemediationItem{}, Summary: "approved via CLI",
	}
	ordPath := filepath.Join(tmp, "approve.json")
	ordBytes, _ := json.Marshal(ord)
	_ = os.WriteFile(ordPath, ordBytes, 0o644)
	if out, _, code := invoke("apply-order", "--task-dir", taskDir, "--order", ordPath, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot); code != builder.ExitOK || !strings.Contains(out, "decision=APPROVE") {
		t.Fatalf("apply-order failed: %d %s", code, out)
	}

	// commit -> cleanup --force
	mockGengis := buildMockGengis(t)
	t.Setenv("GENGIS_BIN", mockGengis)
	t.Setenv("CLI_TEST_WT", wtDir)
	if out, _, code := invoke("commit", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--gengis-root", filepath.Join(factory, "GengiSWAP"), "--vault", vault); code != builder.ExitOK || !strings.Contains(out, "Siguiente paso [AUTOR]:") {
		t.Fatalf("commit failed: %d %s", code, out)
	}
	if out, _, code := invoke("cleanup", "--task-dir", taskDir, "--repo-root", factory, "--factory-root", factory, "--data-dir", dataDir, "--worktree-root", wtRoot, "--force=true", "--delete-branch", "--json"); code != builder.ExitOK || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("cleanup failed: %d %s", code, out)
	}
}
