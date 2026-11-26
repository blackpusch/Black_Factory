package main

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
	"blackfactory/specwriter"
)

type cliMockBrain struct {
	pfErr   error
	ctxResp bfcore.BrainContextResponse
}

func (m *cliMockBrain) Preflight(vaultRoot, dbPath string) (bfcore.BrainPreflight, error) {
	if m.pfErr != nil {
		return bfcore.BrainPreflight{}, m.pfErr
	}
	return bfcore.BrainPreflight{
		VaultRoot:    vaultRoot,
		DBPath:       filepath.Join(vaultRoot, ".index", "brain.db"),
		BrainDBMTime: "2026-11-24T00:00:00Z",
	}, nil
}

func (m *cliMockBrain) Context(vaultRoot, dbPath, project string, files []string) (bfcore.BrainContextResponse, bfcore.BrainInvocation, error) {
	return m.ctxResp, bfcore.BrainInvocation{Command: "brain context", ExitCode: 0, ResultCount: len(m.ctxResp.MatchedDocs)}, nil
}

func TestCLI_E2E_CUJ1_CUJ2_CUJ3_CUJ4(t *testing.T) {
	root := t.TempDir()
	vaultDir := filepath.Join(root, "0_Brain", "vault")
	dataDir := filepath.Join(root, "2_Spec_Writer", "runtime")
	taskRoot := filepath.Join(root, "gengis_task")
	_ = os.MkdirAll(filepath.Join(vaultDir, "wiki"), 0o755)
	_ = os.MkdirAll(filepath.Join(vaultDir, ".index"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "1_Intake", "runtime", "workitems"), 0o755)
	_ = os.WriteFile(filepath.Join(vaultDir, ".index", "brain.db"), []byte("db"), 0o644)
	_ = os.WriteFile(filepath.Join(vaultDir, "wiki", "core.md"), []byte("---\nid: w1\ntitle: Core\ntype: wiki\nstatus: active\n---\n# Core\n"), 0o644)

	affected := []string{
		"0_Brain/engine/a.go",
		"0_Brain/engine/b.go",
		"0_Brain/engine/c.go",
		"0_Brain/engine/d.go",
		"0_Brain/engine/e.go",
	}
	for _, rel := range affected {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		_ = os.WriteFile(abs, []byte("package demo\nfunc TargetAnchor() int { return 1 }\n"), 0o644)
	}
	for _, tf := range []string{"0_Brain/engine/a_test.go", "0_Brain/engine/b_test.go"} {
		abs := filepath.Join(root, filepath.FromSlash(tf))
		_ = os.WriteFile(abs, []byte("package demo\n"), 0o644)
	}

	wi := specwriter.WorkItemInput{
		SchemaVersion:     "1.0",
		ID:                "WI-1234567890ab",
		DedupeKey:         strings.Repeat("d", 64),
		TraceID:           "trc-20261124T000000Z-12345678",
		Revision:          1,
		Status:            "ready",
		Project:           "black-factory",
		Title:             "CUJ 1 FTD 5B",
		Description:       "Bounded fix",
		Kind:              "bug",
		AffectedPaths:     affected,
		ImpactFlags:       []string{},
		Lane:              "FTD",
		SuggestedTemplate: "correccion_hallazgos",
		RiskLevel:         "ALTO",
		RiskHits: []bfcore.RiskHit{{
			ID: "RZ-002", Project: "black-factory", Level: "ALTO", MatchedPaths: []string{"0_Brain/engine/a.go"}, MandatoryGate: "go test",
		}},
		TriageReasons: []specwriter.TriageReasonInput{{Code: "FTD_ELIGIBLE_BOUNDED_MAINTENANCE", RuleKind: "deterministic", RulesetVersion: "triage-det-v1.0", Detail: "ok"}},
		Sanitization:  specwriter.SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("e", 64)},
	}
	wiPath := filepath.Join(root, "1_Intake", "runtime", "workitems", wi.ID+".json")
	rawWI, _ := json.MarshalIndent(wi, "", "  ")
	_ = os.WriteFile(wiPath, rawWI, 0o644)

	client := &cliMockBrain{
		ctxResp: bfcore.BrainContextResponse{
			HasActiveRiskZones: true,
			RiskZonesSHA256:    strings.Repeat("7", 64),
			RiskHits:           wi.RiskHits,
			MatchedDocs: []bfcore.BrainDocMatch{
				{ID: "w1", RelPath: "wiki/core.md", Type: "wiki", Status: "active", CoversPaths: []string{"0_Brain/", "1_Intake/"}},
			},
		},
	}

	execCLI := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := run(args, &out, &errOut, client)
		return code, out.String(), errOut.String()
	}

	// Help, empty args, unknown subcommand, invalid flags
	if c, _, _ := execCLI(); c != specwriter.ExitUsage {
		t.Fatalf("sin args debia retornar 2, obtuvo %d", c)
	}
	if c, out, _ := execCLI("--help"); c != specwriter.ExitOK || !strings.Contains(out, "Subcomandos:") {
		t.Fatalf("--help fallo: %d %s", c, out)
	}
	if c, _, _ := execCLI("unknown"); c != specwriter.ExitUsage {
		t.Fatalf("subcomando desconocido debia retornar 2, obtuvo %d", c)
	}
	if c, _, _ := execCLI("prepare", "--bad-flag", "--json"); c != specwriter.ExitUsage {
		t.Fatalf("flag invalido debia retornar 2, obtuvo %d", c)
	}
	if c, _, _ := execCLI("validate", "--bad-flag"); c != specwriter.ExitUsage {
		t.Fatalf("validate flag invalido debia retornar 2, obtuvo %d", c)
	}
	if c, _, _ := execCLI("render"); c != specwriter.ExitUsage {
		t.Fatalf("render sin --spec debia retornar 2, obtuvo %d", c)
	}
	if c, _, _ := execCLI("render", "--spec", filepath.Join(root, "nope.json"), "--json"); c != specwriter.ExitUsage {
		t.Fatalf("render archivo inexistente debia retornar 2, obtuvo %d", c)
	}
	if c, _, _ := execCLI("compile", "--bad-flag"); c != specwriter.ExitUsage {
		t.Fatalf("compile flag invalido debia retornar 2, obtuvo %d", c)
	}
	if c, _, _ := execCLI("verify", "--bad-flag"); c != specwriter.ExitUsage {
		t.Fatalf("verify flag invalido debia retornar 2, obtuvo %d", c)
	}

	// CUJ 1: prepare -> edit spec (5 prod + 2 sibling *_test.go) -> validate draft -> render -> compile
	c, out, errOut := execCLI("prepare", "--workitem", wiPath, "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json")
	if c != specwriter.ExitOK {
		t.Fatalf("CUJ1 prepare fallo: %d out=%s err=%s", c, out, errOut)
	}
	var prepRes specwriter.PrepareResult
	_ = json.Unmarshal([]byte(out), &prepRes)

	// Completar spec.json para CUJ 1
	specRaw, _ := os.ReadFile(prepRes.SpecPath)
	var spec specwriter.Spec
	_ = json.Unmarshal(specRaw, &spec)
	spec.TargetBranch = "feat/cuj1"
	spec.G3DataMutation = "ninguna"
	spec.StackAssumptions = []string{"Go 1.26 CGO_ENABLED=0"}
	spec.DoD = []string{"go test en verde"}
	spec.Narrative = specwriter.Narrative{Problem: "Fix 5B", RootCause: "TargetAnchor", ProposedChange: "Patch"}
	spec.G5Citations = []specwriter.G5Citation{{Path: "0_Brain/engine/a.go", Line: 2, SymbolAnchor: "TargetAnchor", Claim: "retorna 1"}}
	for _, tf := range []string{"0_Brain/engine/a_test.go", "0_Brain/engine/b_test.go"} {
		st, sum, _ := bfcore.ComputeNormalizedFileSHA256(root, tf)
		spec.ObservedFiles = append(spec.ObservedFiles, specwriter.ObservedFile{Path: tf, State: st, SHA256: sum})
	}
	canMod := append(append([]string{}, affected...), "0_Brain/engine/a_test.go", "0_Brain/engine/b_test.go")
	spec.WorkUnits = []specwriter.WorkUnit{{
		ID:        "fix_5b",
		Title:     "Fix 5 prod + 2 Go tests",
		Objective: "Ejercitar Opcion 5B",
		Inputs:    []string{"WorkItem CUJ1"},
		CanModify: canMod,
		ScopeExpansions: []specwriter.ScopeExpansion{
			{Path: "0_Brain/engine/a_test.go", Rationale: "Test hermano a"},
			{Path: "0_Brain/engine/b_test.go", Rationale: "Test hermano b"},
		},
		Forbidden: []string{},
		DependsOn: []string{},
		AcceptanceCriteria: []specwriter.AcceptanceCriterion{
			{ID: "ac1", Kind: "command", Description: "Ejecutar go test", VerifyStepID: "vs1"},
			{ID: "ac_rg", Kind: "review_gate", Description: "QA Modo_QA", Executor: "Modo_QA"},
		},
		VerifySteps: []specwriter.VerifyStep{{
			ID: "vs1", Cwd: "0_Brain/engine", Env: map[string]string{"CGO_ENABLED": "0"},
			Argv: []string{"go", "test", "-count=1", "./..."}, ExpectExit: 0, TimeoutSec: 120,
		}},
		FailureProof: specwriter.FailureProof{Description: "Falla si TargetAnchor != 1", ExpectExit: 1},
	}}
	spec.RiskMitigations = []specwriter.RiskMitigation{{
		RiskID: "RZ-002", Level: "ALTO", VerifyStepIDs: []string{"vs1"}, ReviewGates: []string{"ac_rg"},
	}}
	bSpec, _ := json.MarshalIndent(spec, "", "  ")
	_ = os.WriteFile(prepRes.SpecPath, bSpec, 0o644)

	if c, out, errOut = execCLI("validate", "--spec", prepRes.SpecPath, "--stage", "draft", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitOK || !strings.Contains(out, "Validate OK") {
		t.Fatalf("CUJ1 validate texto fallo: %d out=%s err=%s", c, out, errOut)
	}
	if c, out, errOut = execCLI("render", "--spec", prepRes.SpecPath); c != specwriter.ExitOK || !strings.Contains(out, "AUTO-GENERATED DERIVED VIEW") {
		t.Fatalf("CUJ1 render texto fallo: %d out=%s err=%s", c, out, errOut)
	}
	if c, out, errOut = execCLI("render", "--spec", prepRes.SpecPath, "--json"); c != specwriter.ExitOK || !strings.Contains(out, `"markdown"`) {
		t.Fatalf("CUJ1 render json fallo: %d out=%s err=%s", c, out, errOut)
	}
	if c, out, errOut = execCLI("compile", "--spec", prepRes.SpecPath, "--task-root", taskRoot, "--task-id", "bf_cuj1_task", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitOK || !strings.Contains(out, "Compile OK") {
		t.Fatalf("CUJ1 compile texto fallo: %d out=%s err=%s", c, out, errOut)
	}

	// CUJ 3: LIVE_TASK_STATE & TASK_DIR_EXISTS (exit 6) + G4 reincident -> exit 5 -> --reset-attempts
	if c, _, errOut = execCLI("compile", "--spec", prepRes.SpecPath, "--task-root", taskRoot, "--task-id", "bf_cuj1_task", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitInfraOrRefusal || !strings.Contains(errOut, "TASK_DIR_EXISTS") {
		t.Fatalf("CUJ3 TASK_DIR_EXISTS fallo: %d %s", c, errOut)
	}
	_ = os.MkdirAll(filepath.Join(taskRoot, "bf_cuj1_task", ".ring"), 0o755)
	_ = os.WriteFile(filepath.Join(taskRoot, "bf_cuj1_task", ".ring", "state.json"), []byte(`{}`), 0o644)
	if c, out, _ = execCLI("compile", "--spec", prepRes.SpecPath, "--task-root", taskRoot, "--task-id", "bf_cuj1_task", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json"); c != specwriter.ExitInfraOrRefusal || !strings.Contains(out, "LIVE_TASK_STATE") {
		t.Fatalf("CUJ3 LIVE_TASK_STATE fallo: %d %s", c, out)
	}

	// CUJ 3 G4 x3 -> 4th hits exit 5 -> --reset-attempts
	badG4 := `{"schema_version":"1.0","title":"` + `C:\\` + `Users\\alice\\leak.go"}`
	_ = os.WriteFile(prepRes.SpecPath, []byte(badG4), 0o644)
	for i := 0; i < 3; i++ {
		if c, _, _ = execCLI("validate", "--spec", prepRes.SpecPath, "--stage", "draft", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json"); c != specwriter.ExitG4 {
			t.Fatalf("CUJ3 G4 intento %d debia retornar 3, obtuvo %d", i+1, c)
		}
	}
	if c, out, _ = execCLI("validate", "--spec", prepRes.SpecPath, "--stage", "draft", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json"); c != specwriter.ExitAttemptsBlocked || !strings.Contains(out, "MAX_VALIDATE_ATTEMPTS") {
		t.Fatalf("CUJ3 4.o intento debia retornar 5 MAX_VALIDATE_ATTEMPTS: %d %s", c, out)
	}
	if c, out, errOut = execCLI("prepare", "--workitem", wiPath, "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--reset-attempts", "--reason", "Ruta personal removida tras revision"); c != specwriter.ExitOK || !strings.Contains(out, "Prepare OK") {
		t.Fatalf("CUJ3 --reset-attempts fallo: %d out=%s err=%s", c, out, errOut)
	}

	// Render sobre G4 debe fallar con Exit 3
	if c, _, _ = execCLI("render", "--spec", prepRes.SpecPath); c != specwriter.ExitG4 {
		t.Fatalf("render sobre archivo con G4 debia retornar 3, obtuvo %d", c)
	}

	// CUJ 2: ESCALATION_REQUIRED -> decisions[] -> ASDD draft -> goldfish -> ASDD final -> compile
	mod2 := "1_Intake/engine/extra.go"
	_ = os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(mod2))), 0o755)
	_ = os.WriteFile(filepath.Join(root, filepath.FromSlash(mod2)), []byte("package intake\n"), 0o644)
	stM2, sumM2, _ := bfcore.ComputeNormalizedFileSHA256(root, mod2)
	spec.ObservedFiles = append(spec.ObservedFiles, specwriter.ObservedFile{Path: mod2, State: stM2, SHA256: sumM2})
	spec.WorkUnits[0].CanModify = append(spec.WorkUnits[0].CanModify, mod2)
	spec.WorkUnits[0].ScopeExpansions = append(spec.WorkUnits[0].ScopeExpansions, specwriter.ScopeExpansion{Path: mod2, Rationale: "2.o modulo"})
	bSpec2, _ := json.MarshalIndent(spec, "", "  ")
	_ = os.WriteFile(prepRes.SpecPath, bSpec2, 0o644)

	if c, out, _ = execCLI("validate", "--spec", prepRes.SpecPath, "--stage", "draft", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json"); c != specwriter.ExitExternalDecision || !strings.Contains(out, "ESCALATION_REQUIRED") {
		t.Fatalf("CUJ2 debia disparar 4 ESCALATION_REQUIRED: %d %s", c, out)
	}

	spec.Lane = "ASDD"
	spec.RequiresAutorHITL = true
	spec.Narrative.AlternativesConsidered = []string{"Alt 1"}
	spec.Narrative.InterfacesAndContracts = "Contract"
	spec.Narrative.EdgeCases = []string{"Edge 1"}
	spec.Narrative.RollbackPlan = "Git revert"
	spec.Decisions = []specwriter.DecisionRecord{{
		Reason: "ESCALATION_REQUIRED", DecidedBy: "AUTOR", Rationale: "Aprobado",
		Timestamp: time.Now().UTC().Format(time.RFC3339), LaneFrom: "FTD", LaneTo: "ASDD",
		ApprovedPaths: spec.WorkUnits[0].CanModify,
		ScopeSHA256:   specwriter.ComputeScopeSHA256(spec.WorkUnits[0].CanModify),
	}}
	bSpecASDD, _ := json.MarshalIndent(spec, "", "  ")
	_ = os.WriteFile(prepRes.SpecPath, bSpecASDD, 0o644)

	if c, _, errOut = execCLI("validate", "--spec", prepRes.SpecPath, "--stage", "draft", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitOK {
		t.Fatalf("CUJ2 ASDD draft fallo: %d %s", c, errOut)
	}
	shaASDD, _ := specwriter.CanonicalSpecSHA256(spec)
	gfBytes, _ := json.MarshalIndent(specwriter.GoldfishReview{
		SchemaVersion: "1.0", SpecSHA256: shaASDD,
		LensesRun: []string{"egm-design", "egm-codebase-details", "egm-implementation-readiness"},
		UnresolvedBlockers: 0, ReportPath: "Docs/reviews/cuj2.md", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	_ = os.WriteFile(filepath.Join(filepath.Dir(prepRes.SpecPath), "goldfish_review.json"), gfBytes, 0o644)
	if c, _, errOut = execCLI("validate", "--spec", prepRes.SpecPath, "--stage", "final", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitOK {
		t.Fatalf("CUJ2 ASDD final fallo: %d %s", c, errOut)
	}
	if c, out, errOut = execCLI("compile", "--spec", prepRes.SpecPath, "--task-root", taskRoot, "--task-id", "bf_cuj2_asdd", "--repo-root", root, "--factory-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json"); c != specwriter.ExitOK || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("CUJ2 ASDD compile fallo: %d out=%s err=%s", c, out, errOut)
	}

	// CUJ 4: verify texto y JSON (con GITIGNORE_DRIFT primero, luego limpio tras git init + .gitignore + bridge/ring reales)
	if c, _, errOut = execCLI("verify", "--factory-root", root, "--repo-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitInfraOrRefusal || !strings.Contains(errOut, "Verify FAIL") {
		t.Fatalf("CUJ4 verify sin git/.gitignore debia fallar con 6: %d %s", c, errOut)
	}
	if _, gitErr := exec.LookPath("git"); gitErr == nil {
		_ = exec.Command("git", "init", root).Run()
		t.Cleanup(func() {
			_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err == nil {
					_ = os.Chmod(p, 0o755)
				}
				return nil
			})
		})
		_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("2_Spec_Writer/runtime/\n2_Spec_Writer/specwriter.exe\n"), 0o644)
		realGengisDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "GengiSWAP", "engine"))
		dstGengisDir := filepath.Join(root, "GengiSWAP", "engine")
		_ = os.MkdirAll(dstGengisDir, 0o755)
		for _, fn := range []string{"bridge.go", "ring.go"} {
			b, err := os.ReadFile(filepath.Join(realGengisDir, fn))
			if err == nil {
				_ = os.WriteFile(filepath.Join(dstGengisDir, fn), b, 0o644)
			}
		}
		if c, out, errOut = execCLI("verify", "--factory-root", root, "--repo-root", root, "--vault", vaultDir, "--data-dir", dataDir); c != specwriter.ExitOK || !strings.Contains(out, "Verify OK=true") {
			t.Fatalf("CUJ4 verify limpio texto fallo: %d out=%s err=%s", c, out, errOut)
		}
		if c, out, errOut = execCLI("verify", "--factory-root", root, "--repo-root", root, "--vault", vaultDir, "--data-dir", dataDir, "--json"); c != specwriter.ExitOK || !strings.Contains(out, `"ok": true`) {
			t.Fatalf("CUJ4 verify limpio json fallo: %d out=%s err=%s", c, out, errOut)
		}
	}

	// Test reorderFlags helper & additional text/JSON error branches for >= 85% coverage
	if c, _, errOut := execCLI("prepare", "--workitem", filepath.Join(root, "nope.json")); c != specwriter.ExitUsage || !strings.Contains(errOut, "Prepare FAIL") {
		t.Fatalf("prepare fail texto inesperado: %d %s", c, errOut)
	}
	if c, _, errOut := execCLI("validate", "--spec", filepath.Join(root, "nope.json"), "--stage", "draft"); c != specwriter.ExitUsage || !strings.Contains(errOut, "Validate FAIL") {
		t.Fatalf("validate fail texto inesperado: %d %s", c, errOut)
	}
	if c, _, errOut := execCLI("render", "--spec", root); c != specwriter.ExitUsage || !strings.Contains(errOut, "INVALID_USAGE") {
		t.Fatalf("render sobre directorio debia retornar 2: %d %s", c, errOut)
	}
	if c, _, errOut := execCLI("render", "--spec", filepath.Join(root, "nope.json")); c != specwriter.ExitUsage || !strings.Contains(errOut, "Render FAIL") {
		t.Fatalf("render fail texto inesperado: %d %s", c, errOut)
	}
	bigFile := filepath.Join(root, "huge.json")
	_ = os.WriteFile(bigFile, make([]byte, specwriter.MaxSpecBytes+4), 0o644)
	if c, out, _ := execCLI("render", "--spec", bigFile, "--json"); c != specwriter.ExitFixable || !strings.Contains(out, "SPEC_TOO_LARGE") {
		t.Fatalf("render huge --json debia retornar 1 SPEC_TOO_LARGE: %d %s", c, out)
	}
	if c, _, errOut := execCLI("render", "--spec", bigFile); c != specwriter.ExitFixable || !strings.Contains(errOut, "SPEC_TOO_LARGE") {
		t.Fatalf("render huge texto debia retornar 1 SPEC_TOO_LARGE: %d %s", c, errOut)
	}
	badJSON := filepath.Join(root, "bad.json")
	_ = os.WriteFile(badJSON, []byte("{bad"), 0o644)
	if c, out, _ := execCLI("render", "--spec", badJSON, "--json=true"); c != specwriter.ExitFixable || !strings.Contains(out, "INVALID_JSON") {
		t.Fatalf("render bad json debia retornar 1 INVALID_JSON: %d %s", c, out)
	}
	if c, _, errOut := execCLI("render", "--bad-flag"); c != specwriter.ExitUsage || !strings.Contains(errOut, "INVALID_USAGE") {
		t.Fatalf("render flag invalido debia retornar 2: %d %s", c, errOut)
	}
	if c, out, _ := execCLI("verify", "--factory-root", root, "--repo-root", root, "--vault", vaultDir, "--data-dir", filepath.Join(root, "bad_dir_missing"), "-json"); c == specwriter.ExitOK && !strings.Contains(out, `"ok": true`) {
		t.Fatalf("verify -json inesperado: %d %s", c, out)
	}

	re := reorderFlags([]string{"pos", "--json", "true", "--spec", "a.json", "--", "--extra"}, map[string]bool{"json": true}, map[string]bool{"spec": true})
	if len(re) < 4 || re[0] != "--json=true" || re[1] != "--spec" || re[2] != "a.json" {
		t.Fatalf("reorderFlags inesperado: %v", re)
	}
}
