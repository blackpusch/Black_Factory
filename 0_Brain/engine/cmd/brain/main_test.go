package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"blackfactory/brain"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, LocalService{})
	return code, out.String(), errOut.String()
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s en %s fallo: %v\n%s", strings.Join(args, " "), dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeRepoFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func newTempGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	// En Windows Git crea archivos de solo lectura bajo .git/objects; en orden LIFO
	// este Cleanup corre antes del RemoveAll de t.TempDir().
	t.Cleanup(func() {
		_ = filepath.WalkDir(repoDir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				_ = os.Chmod(p, 0o755)
			} else {
				_ = os.Chmod(p, 0o644)
			}
			return nil
		})
	})
	return repoDir
}

func TestCLI_E2E_FullLifecycle(t *testing.T) {
	repoDir := newTempGitRepo(t)
	runGitCmd(t, repoDir, "init")
	runGitCmd(t, repoDir, "config", "user.name", "Tester")
	runGitCmd(t, repoDir, "config", "user.email", "tester@example.local")

	writeRepoFile(t, repoDir, "src/core/engine.go", "package core\n\nfunc Run() int { return 1 }\n")
	runGitCmd(t, repoDir, "add", "src/core/engine.go")
	runGitCmd(t, repoDir, "commit", "-m", "initial core engine")
	baseHEAD := runGitCmd(t, repoDir, "rev-parse", "HEAD")

	vaultDir := filepath.Join(t.TempDir(), "vault")
	schemasDir := filepath.Join("..", "..", "..", "schemas")

	// 1. init-vault + idempotencia
	code, out, errOut := runCLI(t, "init-vault", "--vault", vaultDir, "--schemas", schemasDir)
	if code != 0 {
		t.Fatalf("init-vault fallo (code=%d): %s %s", code, out, errOut)
	}
	for _, rel := range []string{"AGENTS.md", "index.md", "constitution.md", "log.md", "wiki/risk_zones.md"} {
		if _, err := os.Stat(filepath.Join(vaultDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("faltaba archivo bootstrap %s: %v", rel, err)
		}
	}
	for _, dir := range []string{"adr", "wiki", "projects", "_proposals", ".index"} {
		if fi, err := os.Stat(filepath.Join(vaultDir, dir)); err != nil || !fi.IsDir() {
			t.Fatalf("faltaba directorio %s", dir)
		}
	}

	logPath := filepath.Join(vaultDir, "log.md")
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
		t.Fatalf("abriendo log.md: %v", err)
	} else {
		_, _ = f.WriteString("- [custom] Marca previa que init-vault no debe sobrescribir.\n")
		_ = f.Close()
	}
	if code, out, errOut = runCLI(t, "init-vault", "--vault", vaultDir, "--schemas", schemasDir); code != 0 {
		t.Fatalf("segundo init-vault fallo (code=%d): %s %s", code, out, errOut)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(logBytes), "[custom]") {
		t.Fatalf("init-vault sobrescribio log.md existente: err=%v", err)
	}

	// 2. propose para crear wiki/core_engine.md (sin mutar destino antes de apply)
	code, out, errOut = runCLI(t, "propose",
		"--vault", vaultDir,
		"--target", "wiki/core_engine.md",
		"--title", "Motor Central Core",
		"--project", "demo",
		"--tags", "core,engine",
		"--covers", "src/core/",
		"--verified-head", baseHEAD,
		"--body", "# Motor Central Core\n\nDocumentacion verificada del motor engine principal.\n",
		"--json",
	)
	if code != 0 {
		t.Fatalf("propose wiki/core_engine.md fallo (code=%d): %s %s", code, out, errOut)
	}
	var propCore brain.ProposeResult
	if err := json.Unmarshal([]byte(out), &propCore); err != nil {
		t.Fatalf("json propose: %v\n%s", err, out)
	}
	if propCore.RequiresAutorApproval {
		t.Fatalf("wiki/core_engine.md no debia requerir aprobacion AUTOR")
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "wiki", "core_engine.md")); !os.IsNotExist(err) {
		t.Fatalf("propose no debia crear wiki/core_engine.md antes de apply")
	}

	// 3. apply de documento estandar (sin --approve-autor)
	code, out, errOut = runCLI(t, "apply", "--vault", vaultDir, "--proposal", propCore.ProposalPath, "--json")
	if code != 0 {
		t.Fatalf("apply wiki/core_engine.md fallo (code=%d): %s %s", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "wiki", "core_engine.md")); err != nil {
		t.Fatalf("wiki/core_engine.md no existe tras apply: %v", err)
	}

	// 4. index
	code, out, errOut = runCLI(t, "index", "--vault", vaultDir, "--json")
	if code != 0 {
		t.Fatalf("index fallo (code=%d): %s %s", code, out, errOut)
	}
	var idxStats brain.IndexStats
	if err := json.Unmarshal([]byte(out), &idxStats); err != nil {
		t.Fatalf("json index: %v", err)
	}
	if idxStats.DocsIndexed < 5 {
		t.Fatalf("esperaba >= 5 docs indexados, obtuvo %d", idxStats.DocsIndexed)
	}

	// 5. query
	code, out, errOut = runCLI(t, "query", "--vault", vaultDir, "engine", "--project", "demo", "--json")
	if code != 0 {
		t.Fatalf("query fallo (code=%d): %s %s", code, out, errOut)
	}
	var qRes []brain.SearchResult
	if err := json.Unmarshal([]byte(out), &qRes); err != nil {
		t.Fatalf("json query: %v", err)
	}
	if len(qRes) == 0 || qRes[0].ID != "wiki-core-engine" {
		t.Fatalf("query inesperada: %+v", qRes)
	}

	// 6. context
	code, out, errOut = runCLI(t, "context", "--vault", vaultDir, "--project", "demo", "--files", `src\core\engine.go`, "--json")
	if code != 0 {
		t.Fatalf("context fallo (code=%d): %s %s", code, out, errOut)
	}
	var cRes []brain.SearchResult
	if err := json.Unmarshal([]byte(out), &cRes); err != nil {
		t.Fatalf("json context: %v", err)
	}
	seenIDs := map[string]bool{}
	for _, r := range cRes {
		seenIDs[r.ID] = true
	}
	for _, want := range []string{"brain-constitution", "brain-risk-zones", "wiki-core-engine"} {
		if !seenIDs[want] {
			t.Fatalf("context no incluyo %s: %+v", want, cRes)
		}
	}

	// 7. verify limpio antes de commit nuevo
	code, out, errOut = runCLI(t, "verify", "--vault", vaultDir, "--repo", "demo="+repoDir, "--json")
	if code != 0 {
		t.Fatalf("verify inicial fallo (code=%d): %s %s", code, out, errOut)
	}
	var vOut VerifyOutput
	if err := json.Unmarshal([]byte(out), &vOut); err != nil {
		t.Fatalf("json verify: %v", err)
	}
	if !vOut.Validation.OK || len(vOut.Drift) != 0 {
		t.Fatalf("esperaba OK sin drift antes del commit: %+v", vOut)
	}

	// 8. Commit en repo temporal sobre ruta cubierta -> deteccion de drift
	writeRepoFile(t, repoDir, "src/core/engine.go", "package core\n\nfunc Run() int { return 2 }\n")
	runGitCmd(t, repoDir, "add", "src/core/engine.go")
	runGitCmd(t, repoDir, "commit", "-m", "update engine logic")
	newHEAD := runGitCmd(t, repoDir, "rev-parse", "HEAD")

	code, out, errOut = runCLI(t, "verify", "--vault", vaultDir, "--repo", "demo="+repoDir, "--json")
	if code != 0 {
		t.Fatalf("verify con drift (sin --fail-on-drift) fallo (code=%d): %s %s", code, out, errOut)
	}
	if err := json.Unmarshal([]byte(out), &vOut); err != nil {
		t.Fatalf("json verify drift: %v", err)
	}
	if len(vOut.Drift) != 1 {
		t.Fatalf("esperaba 1 item de drift, obtuvo %d: %+v", len(vOut.Drift), vOut.Drift)
	}
	d := vOut.Drift[0]
	if d.RelPath != "wiki/core_engine.md" || d.VerifiedHead != baseHEAD || d.CurrentHead != newHEAD {
		t.Fatalf("drift inesperado: %+v", d)
	}
	if len(d.ChangedFiles) != 1 || d.ChangedFiles[0] != "src/core/engine.go" {
		t.Fatalf("ChangedFiles inesperado: %v", d.ChangedFiles)
	}

	if code, _, _ = runCLI(t, "verify", "--vault", vaultDir, "--repo", "demo="+repoDir, "--fail-on-drift"); code == 0 {
		t.Fatalf("verify --fail-on-drift debia retornar codigo != 0 cuando hay drift")
	}

	// 9. propose sobre constitution.md + rechazo sin --approve-autor + exito con --approve-autor
	constAbs := filepath.Join(vaultDir, "constitution.md")
	constBefore, err := os.ReadFile(constAbs)
	if err != nil {
		t.Fatalf("leyendo constitution.md: %v", err)
	}

	code, out, errOut = runCLI(t, "propose",
		"--vault", vaultDir,
		"--target", "constitution.md",
		"--title", "Constitucion Actualizada",
		"--project", "black-factory",
		"--body", "# Constitucion Actualizada\n\nRegla constitucional reforzada: clausula omega.\n\nVer [AGENTS.md](AGENTS.md).\n",
		"--json",
	)
	if code != 0 {
		t.Fatalf("propose constitution.md fallo (code=%d): %s %s", code, out, errOut)
	}
	var propConst brain.ProposeResult
	if err := json.Unmarshal([]byte(out), &propConst); err != nil {
		t.Fatalf("json propose const: %v", err)
	}
	if !propConst.RequiresAutorApproval {
		t.Fatalf("constitution.md debe requerir aprobacion AUTOR")
	}

	// Rechazo sin --approve-autor
	if code, _, errOut = runCLI(t, "apply", "--vault", vaultDir, "--proposal", propConst.ProposalPath); code == 0 {
		t.Fatalf("apply sobre constitution.md sin --approve-autor debia fallar")
	} else if !strings.Contains(errOut, "AUTOR") {
		t.Fatalf("mensaje de rechazo inesperado: %s", errOut)
	}
	constStill, _ := os.ReadFile(constAbs)
	if string(constStill) != string(constBefore) {
		t.Fatalf("constitution.md fue mutado pese al rechazo")
	}

	// Aprobacion con --approve-autor
	code, out, errOut = runCLI(t, "apply", "--vault", vaultDir, "--proposal", propConst.ProposalPath, "--approve-autor", "--json")
	if code != 0 {
		t.Fatalf("apply --approve-autor fallo (code=%d): %s %s", code, out, errOut)
	}
	constAfter, _ := os.ReadFile(constAbs)
	if !strings.Contains(string(constAfter), "clausula omega") {
		t.Fatalf("constitution.md no se actualizo tras apply --approve-autor")
	}

	// Verificar append-only en log.md y reindexacion
	logAfter, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logAfter), "[custom]") || !strings.Contains(string(logAfter), "target=constitution.md") {
		t.Fatalf("log.md no preservo entradas previas o no registro apply: %s", string(logAfter))
	}

	code, out, errOut = runCLI(t, "query", "--vault", vaultDir, "omega", "--json")
	if code != 0 {
		t.Fatalf("query tras apply fallo: %s %s", out, errOut)
	}
	if err := json.Unmarshal([]byte(out), &qRes); err != nil || len(qRes) != 1 || qRes[0].RelPath != "constitution.md" {
		t.Fatalf("reindexacion tras apply no encontro 'omega' en constitution.md: %+v", qRes)
	}

	// 10. Resolver drift actualizando verified_head de wiki/core_engine.md a newHEAD
	code, out, errOut = runCLI(t, "propose",
		"--vault", vaultDir,
		"--target", "wiki/core_engine.md",
		"--project", "demo",
		"--covers", "src/core/",
		"--verified-head", newHEAD,
		"--body", "# Motor Central Core\n\nActualizado a la version 2 de engine.\n",
		"--json",
	)
	if code != 0 {
		t.Fatalf("propose resolucion drift fallo: %s %s", out, errOut)
	}
	var propSync brain.ProposeResult
	_ = json.Unmarshal([]byte(out), &propSync)
	if code, out, errOut = runCLI(t, "apply", "--vault", vaultDir, "--proposal", propSync.ProposalPath); code != 0 {
		t.Fatalf("apply resolucion drift fallo: %s %s", out, errOut)
	}

	code, out, errOut = runCLI(t, "verify", "--vault", vaultDir, "--repo", "demo="+repoDir, "--fail-on-drift", "--json")
	if code != 0 {
		t.Fatalf("verify final tras resolver drift fallo (code=%d): %s %s", code, out, errOut)
	}
	if err := json.Unmarshal([]byte(out), &vOut); err != nil || !vOut.OK || len(vOut.Drift) != 0 {
		t.Fatalf("esperaba verify final limpio sin drift: %+v", vOut)
	}
}

func TestCLI_ApplyRejectsRiskZonesWithoutAutorAndG4Violations(t *testing.T) {
	vaultDir := filepath.Join(t.TempDir(), "vault")
	schemasDir := filepath.Join("..", "..", "..", "schemas")

	if code, out, errOut := runCLI(t, "init-vault", "--vault", vaultDir, "--schemas", schemasDir); code != 0 {
		t.Fatalf("init-vault fallo (code=%d): %s %s", code, out, errOut)
	}

	// 1. risk_zones.md protegido por compuerta [AUTOR]
	rzProp, err := brain.ProposeUpdate(vaultDir, brain.ProposeRequest{
		TargetPath: "wiki/risk_zones.md",
		Title:      "Mapa de Zonas de Riesgo",
		Project:    "black-factory",
		Content:    "# Zonas de Riesgo\n\nActualizado con nuevo modulo critico.\n\nVer [../AGENTS.md](../AGENTS.md).\n",
	})
	if err != nil {
		t.Fatalf("ProposeUpdate risk_zones error: %v", err)
	}
	if !rzProp.RequiresAutorApproval {
		t.Fatalf("wiki/risk_zones.md debe activar compuerta AUTOR")
	}
	if code, _, _ := runCLI(t, "apply", "--vault", vaultDir, "--proposal", rzProp.ProposalPath); code == 0 {
		t.Fatalf("apply sobre wiki/risk_zones.md sin --approve-autor debia fallar")
	}
	if code, out, errOut := runCLI(t, "apply", "--vault", vaultDir, "--proposal", rzProp.ProposalPath, "--approve-autor"); code != 0 {
		t.Fatalf("apply --approve-autor sobre risk_zones.md fallo: %s %s", out, errOut)
	}

	// 2. Rechazo por violacion G4
	g4Payload := "No permitir " + "PRIVATE " + "KEY ni C:\\" + "Users\\demo\\secreto.txt"
	g4Prop, err := brain.ProposeUpdate(vaultDir, brain.ProposeRequest{
		TargetPath: "wiki/fuga_g4.md",
		Title:      "Prueba G4",
		Project:    "black-factory",
		Content:    g4Payload,
	})
	if err != nil {
		t.Fatalf("ProposeUpdate g4 error: %v", err)
	}
	if code, _, errOut := runCLI(t, "apply", "--vault", vaultDir, "--proposal", g4Prop.ProposalPath); code == 0 {
		t.Fatalf("apply debia rechazar propuesta con violacion G4")
	} else if !strings.Contains(errOut, "G4") {
		t.Fatalf("mensaje de error G4 inesperado: %s", errOut)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "wiki", "fuga_g4.md")); !os.IsNotExist(err) {
		t.Fatalf("wiki/fuga_g4.md no debia crearse tras rechazo G4")
	}

	// 3. Rechazo por esquema OKF incompleto (sin status)
	badSchemaProp, err := brain.ProposeUpdate(vaultDir, brain.ProposeRequest{
		TargetPath: "wiki/incompleto.md",
		Title:      "Incompleto",
		Project:    "black-factory",
		Content:    "---\nid: wiki-incompleto\ntitle: \"Sin status\"\ntype: wiki\n---\n\n# Incompleto\n",
	})
	if err != nil {
		t.Fatalf("ProposeUpdate esquema incompleto error: %v", err)
	}
	if code, _, _ := runCLI(t, "apply", "--vault", vaultDir, "--proposal", badSchemaProp.ProposalPath); code == 0 {
		t.Fatalf("apply debia rechazar propuesta sin campo obligatorio status")
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "wiki", "incompleto.md")); !os.IsNotExist(err) {
		t.Fatalf("wiki/incompleto.md no debia crearse tras rechazo de esquema")
	}
}

func TestResolveVaultPath_PriorityAndFallback(t *testing.T) {
	t.Setenv("BRAIN_VAULT", "")
	if got := resolveVaultPath("custom/vault"); got != "custom/vault" {
		t.Fatalf("resolveVaultPath con flag explicito = %q, esperaba custom/vault", got)
	}
	t.Setenv("BRAIN_VAULT", "env/vault")
	if got := resolveVaultPath(""); got != "env/vault" {
		t.Fatalf("resolveVaultPath con BRAIN_VAULT = %q, esperaba env/vault", got)
	}
	t.Setenv("BRAIN_VAULT", "")
	def := resolveVaultPath("")
	if def == "" || !strings.HasSuffix(filepath.ToSlash(def), "vault") {
		t.Fatalf("resolveVaultPath default inesperado: %q", def)
	}
}

