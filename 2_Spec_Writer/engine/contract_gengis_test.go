package specwriter

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"blackfactory/pkg/bfcore"
)

func resolveGengisBinForContract(t *testing.T) string {
	t.Helper()
	strict := strings.TrimSpace(os.Getenv("GENGIS_CONTRACT_STRICT")) == "1"
	bin := strings.TrimSpace(os.Getenv("GENGIS_BIN"))
	if bin == "" {
		bin = filepath.Clean(filepath.Join("..", "..", "GengiSWAP", "gengis.exe"))
	}
	if _, err := os.Stat(bin); err != nil {
		if strict {
			t.Fatalf("GENGIS_CONTRACT_STRICT=1 exige binario gengis.exe en %q: %v", bin, err)
		}
		t.Skipf("gengis.exe no encontrado en %q (defina GENGIS_BIN o GENGIS_CONTRACT_STRICT=1): %v", bin, err)
	}
	return bin
}

func initGitTempRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git requerido para contract_gengis_test: %v", err)
	}
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s fallo: %v (%s)", dir, err, string(out))
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
	})
}

func TestContract_GengisPlan_PositiveFTD_And_ASDDMultiChunk(t *testing.T) {
	gengisBin := resolveGengisBinForContract(t)
	tmpGengisRoot := t.TempDir()

	// 1. Fixture positivo FTD (5 productivos + 2 *_test.go hermanos bajo Opcion 5B, failure_proof.expect_exit: 1)
	affected5 := []string{
		"0_Brain/engine/a.go",
		"0_Brain/engine/b.go",
		"0_Brain/engine/c.go",
		"0_Brain/engine/d.go",
		"0_Brain/engine/e.go",
	}
	fFTD := newTestFixtureEnv(t, "FTD", "bug", "ALTO", affected5, nil, []bfcore.RiskHit{{
		ID: "RZ-002", Project: "black-factory", Level: "ALTO", MatchedPaths: []string{"0_Brain/engine/a.go"}, MandatoryGate: "go test",
	}})
	initGitTempRepo(t, fFTD.repoRoot)
	prepFTD := fFTD.prepare(t)

	// Anadir 2 tests Go hermanos
	for _, tf := range []string{"0_Brain/engine/a_test.go", "0_Brain/engine/b_test.go"} {
		_ = os.WriteFile(filepath.Join(fFTD.repoRoot, filepath.FromSlash(tf)), []byte("package demo\n"), 0o644)
	}
	canMod7 := append(append([]string{}, affected5...), "0_Brain/engine/a_test.go", "0_Brain/engine/b_test.go")
	specFTD := buildValidSpecFromSkeleton(t, fFTD, prepFTD.SpecPath, canMod7, false)
	writeSpecFile(t, prepFTD.SpecPath, specFTD)

	if res, code := Validate(ValidateOptions{
		SpecPath: prepFTD.SpecPath, Stage: "draft", RepoRoot: fFTD.repoRoot, FactoryRoot: fFTD.factoryRoot,
		VaultRoot: fFTD.vaultRoot, DataDir: fFTD.dataDir, BrainClient: fFTD.client,
	}); code != ExitOK {
		t.Fatalf("FTD Option 5B validate draft fallo: %d %+v", code, res)
	}

	compFTD, codeFTD := Compile(CompileOptions{
		SpecPath: prepFTD.SpecPath, TaskRoot: fFTD.taskRoot, TaskID: "bf_contract_ftd",
		RepoRoot: fFTD.repoRoot, FactoryRoot: fFTD.factoryRoot, VaultRoot: fFTD.vaultRoot, DataDir: fFTD.dataDir, BrainClient: fFTD.client,
	})
	if codeFTD != ExitOK {
		t.Fatalf("FTD compile fallo: %d %+v", codeFTD, compFTD)
	}

	assertGengisPlanContract(t, gengisBin, tmpGengisRoot, compFTD.TaskDir, []string{"01_core_fix"}, []string{"[EXPANSIONS:2]"})

	// 2. Fixture positivo ASDD multi-chunk (2 chunks con depends_on, CRITICO HITL, [ESCALATED])
	fASDD := newTestFixtureEnv(t, "ASDD", "feature", "CRITICO", []string{"0_Brain/engine/a.go"}, []string{"contract"}, []bfcore.RiskHit{{
		ID: "RZ-001", Project: "black-factory", Level: "CRITICO", MatchedPaths: []string{"0_Brain/engine/a.go"}, MandatoryGate: "PR + AUTOR",
	}})
	initGitTempRepo(t, fASDD.repoRoot)
	prepASDD := fASDD.prepare(t)
	wiDirASDD := filepath.Dir(prepASDD.SpecPath)

	file2 := "0_Brain/engine/second.go"
	_ = os.WriteFile(filepath.Join(fASDD.repoRoot, filepath.FromSlash(file2)), []byte("package demo\n// SecondSymbol\nfunc SecondSymbol() {}\n"), 0o644)

	specASDD := buildValidSpecFromSkeleton(t, fASDD, prepASDD.SpecPath, []string{"0_Brain/engine/a.go", file2}, true)
	specASDD.RequiresAutorHITL = true
	specASDD.Decisions = []DecisionRecord{{
		Reason: "ESCALATION_REQUIRED", DecidedBy: "AUTOR", Rationale: "Multi-chunk ASDD CRITICO aprobado",
		Timestamp: time.Now().UTC().Format(time.RFC3339), LaneFrom: "ASDD", LaneTo: "ASDD",
		ApprovedPaths: []string{"0_Brain/engine/a.go", file2},
		ScopeSHA256:   ComputeScopeSHA256([]string{"0_Brain/engine/a.go", file2}),
	}}
	// Dividir en 2 chunks con depends_on
	u1 := specASDD.WorkUnits[0]
	u1.ID = "phase_one"
	u1.CanModify = []string{"0_Brain/engine/a.go"}
	u1.ScopeExpansions = []ScopeExpansion{}
	u2 := WorkUnit{
		ID:              "phase_two",
		Title:           "Integracion del segundo archivo",
		Objective:       "Completar integracion tras phase_one",
		Inputs:          []string{"Entregables de phase_one"},
		CanModify:       []string{file2},
		ScopeExpansions: []ScopeExpansion{{Path: file2, Rationale: "Expansion ASDD controlada"}},
		Forbidden:       []string{"0_Brain/vault/"},
		DependsOn:       []string{"phase_one"},
		AcceptanceCriteria: []AcceptanceCriterion{
			{ID: "ac_u2_test", Kind: "command", Description: "go vet en verde", VerifyStepID: "vs_u2_vet"},
		},
		VerifySteps: []VerifyStep{{
			ID: "vs_u2_vet", Cwd: "0_Brain/engine", Env: map[string]string{"CGO_ENABLED": "0"},
			Argv: []string{"go", "vet", "./..."}, ExpectExit: 0, TimeoutSec: 60,
		}},
		FailureProof: FailureProof{Description: "go vet falla si se introduce simbolo no usado", ExpectExit: 1},
	}
	specASDD.WorkUnits = []WorkUnit{u1, u2}
	writeSpecFile(t, prepASDD.SpecPath, specASDD)

	if res, code := Validate(ValidateOptions{
		SpecPath: prepASDD.SpecPath, Stage: "draft", RepoRoot: fASDD.repoRoot, FactoryRoot: fASDD.factoryRoot,
		VaultRoot: fASDD.vaultRoot, DataDir: fASDD.dataDir, BrainClient: fASDD.client,
	}); code != ExitOK {
		t.Fatalf("ASDD draft fallo: %d %+v", code, res)
	}
	writeGoldfishReview(t, wiDirASDD, specASDD, []string{"egm-design", "egm-codebase-details", "egm-implementation-readiness", "egm-structural"})
	if res, code := Validate(ValidateOptions{
		SpecPath: prepASDD.SpecPath, Stage: "final", RepoRoot: fASDD.repoRoot, FactoryRoot: fASDD.factoryRoot,
		VaultRoot: fASDD.vaultRoot, DataDir: fASDD.dataDir, BrainClient: fASDD.client,
	}); code != ExitOK {
		t.Fatalf("ASDD final fallo: %d %+v", code, res)
	}

	compASDD, codeASDD := Compile(CompileOptions{
		SpecPath: prepASDD.SpecPath, TaskRoot: fASDD.taskRoot, TaskID: "bf_contract_asdd",
		RepoRoot: fASDD.repoRoot, FactoryRoot: fASDD.factoryRoot, VaultRoot: fASDD.vaultRoot, DataDir: fASDD.dataDir, BrainClient: fASDD.client,
	})
	if codeASDD != ExitOK {
		t.Fatalf("ASDD compile fallo: %d %+v", codeASDD, compASDD)
	}

	assertGengisPlanContract(t, gengisBin, tmpGengisRoot, compASDD.TaskDir,
		[]string{"01_phase_one", "02_phase_two"},
		[]string{"[HITL]", "[CRITICO]", "[ESCALATED]", "[EXPANSIONS:1]"},
	)
}

func assertGengisPlanContract(t *testing.T, gengisBin, tmpGengisRoot, taskDir string, wantChunks []string, wantTags []string) {
	t.Helper()
	cmd := exec.Command(gengisBin, "plan", taskDir, "--root", tmpGengisRoot)
	cmd.Stdin = strings.NewReader("n\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("gengis.exe plan con 'n\\n' debia salir con codigo 1 (no aprobado), obtuvo exit 0")
	}

	outStr := stdout.String()
	errStr := stderr.String()
	if strings.Contains(outStr, "ADVERTENCIA: destino sin Git") {
		t.Fatalf("gengis.exe plan reporto destino sin Git inesperadamente:\n%s", outStr)
	}
	if !strings.Contains(errStr, "plan no aprobado por el Autor") {
		t.Fatalf("stderr esperado 'plan no aprobado por el Autor', obtuvo:\n%s", errStr)
	}
	for _, chID := range wantChunks {
		if !strings.Contains(outStr, chID) {
			t.Fatalf("resumen del plan no listo chunk %s:\n%s", chID, outStr)
		}
	}
	for _, tag := range wantTags {
		if !strings.Contains(outStr, tag) {
			t.Fatalf("resumen del plan omitio etiqueta %s:\n%s", tag, outStr)
		}
	}

	statePath := filepath.Join(taskDir, ".ring", "state.json")
	rawState, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("gengis.exe plan debia crear .ring/state.json: %v", err)
	}
	var st struct {
		Status string `json:"status"`
		Chunks []struct {
			ID    string `json:"id"`
			Scope struct {
				CanModify []string `json:"can_modify"`
			} `json:"scope"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(rawState, &st); err != nil {
		t.Fatalf("decodificando state.json: %v", err)
	}
	if st.Status != "PLAN_PENDING" {
		t.Fatalf("state.json status = %q, esperaba PLAN_PENDING", st.Status)
	}
	if len(st.Chunks) != len(wantChunks) {
		t.Fatalf("state.json chunks = %d, esperaba %d", len(st.Chunks), len(wantChunks))
	}
	for _, ch := range st.Chunks {
		if len(ch.Scope.CanModify) == 0 {
			t.Fatalf("paridad rota: Chunk %s tiene Scope.CanModify vacio (verifique ## Puede modificar)", ch.ID)
		}
	}
}

func TestContract_GengisPlan_NegativeParity(t *testing.T) {
	gengisBin := resolveGengisBinForContract(t)
	tmpGengisRoot := t.TempDir()

	negCases := []struct {
		name  string
		chunk string
	}{
		{
			name:  "empty_failure_proof",
			chunk: "# Chunk 01: Bad\n\n## Objetivo\nTest\n\n## Puede modificar\n- `a.go`\n\n## Criterios de Aceptación\n- ok\n\n## Comando de Verificación\n```cmd\ngo test ./...\n```\n",
		},
		{
			name:  "empty_verify_commands",
			chunk: "# Chunk 01: Bad\n\n## Objetivo\nTest\n\n## Puede modificar\n- `a.go`\n\n## Criterios de Aceptación\n- ok\n\n## Prueba de que puede fallar\nFalla si rompe\n",
		},
	}

	for _, tc := range negCases {
		t.Run(tc.name, func(t *testing.T) {
			taskDir := filepath.Join(t.TempDir(), "neg_"+tc.name)
			_ = os.MkdirAll(filepath.Join(taskDir, "chunks"), 0o755)
			indexMD := "# Mapa de Tarea: neg\nPlantilla: correccion_hallazgos v1\n\n## Alcance\n- Puede modificar: `a.go`\n"
			_ = os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(indexMD), 0o644)
			_ = os.WriteFile(filepath.Join(taskDir, "chunks", "01_bad.md"), []byte(tc.chunk), 0o644)

			cmd := exec.Command(gengisBin, "plan", taskDir, "--root", tmpGengisRoot)
			cmd.Stdin = strings.NewReader("n\n")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				t.Fatalf("caso negativo %s debia fallar en gengis.exe plan", tc.name)
			}
			if !strings.Contains(stderr.String(), "chunk inválido") {
				t.Fatalf("caso negativo %s esperaba 'chunk inválido' en stderr, obtuvo: %s", tc.name, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(taskDir, ".ring", "state.json")); !os.IsNotExist(err) {
				t.Fatalf("caso negativo %s no debia crear .ring/state.json", tc.name)
			}
		})
	}
}
