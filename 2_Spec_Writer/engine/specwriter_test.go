package specwriter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"blackfactory/pkg/bfcore"
)

type mockBrainClient struct {
	pfErr   error
	ctxErr  error
	ctxResp bfcore.BrainContextResponse
	calls   int
}

func (m *mockBrainClient) Preflight(vaultRoot, dbPath string) (bfcore.BrainPreflight, error) {
	if m.pfErr != nil {
		return bfcore.BrainPreflight{}, m.pfErr
	}
	return bfcore.BrainPreflight{
		VaultRoot:    vaultRoot,
		DBPath:       filepath.Join(vaultRoot, ".index", "brain.db"),
		BrainDBMTime: "2026-11-24T00:00:00Z",
	}, nil
}

func (m *mockBrainClient) Context(vaultRoot, dbPath, project string, files []string) (bfcore.BrainContextResponse, bfcore.BrainInvocation, error) {
	m.calls++
	if m.ctxErr != nil {
		return bfcore.BrainContextResponse{}, bfcore.BrainInvocation{Command: "brain context", ExitCode: 1}, m.ctxErr
	}
	return m.ctxResp, bfcore.BrainInvocation{
		Command:     "0_Brain/brain.exe context --json",
		ExitCode:    0,
		DurationMs:  5,
		ResultCount: len(m.ctxResp.MatchedDocs),
	}, nil
}

type testFixtureEnv struct {
	root        string
	factoryRoot string
	repoRoot    string
	vaultRoot   string
	dataDir     string
	taskRoot    string
	wiPath      string
	wiID        string
	wiSHA       string
	traceID     string
	client      *mockBrainClient
}

func newTestFixtureEnv(t *testing.T, lane, kind, riskLevel string, affected []string, impactFlags []string, riskHits []bfcore.RiskHit) *testFixtureEnv {
	t.Helper()
	root := t.TempDir()
	factoryRoot := root
	repoRoot := root
	vaultRoot := filepath.Join(factoryRoot, "0_Brain", "vault")
	dataDir := filepath.Join(factoryRoot, "2_Spec_Writer", "runtime")
	taskRoot := filepath.Join(root, "gengis_task")

	mustMkdir := func(p string) {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustMkdir(filepath.Join(vaultRoot, "wiki"))
	mustMkdir(filepath.Join(vaultRoot, ".index"))
	mustMkdir(filepath.Join(factoryRoot, "1_Intake", "runtime", "workitems"))

	if err := os.WriteFile(filepath.Join(vaultRoot, ".index", "brain.db"), []byte("mock-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	wikiDoc := "---\nid: wiki-core\ntitle: Core\ntype: wiki\nstatus: active\n---\n# Core\n"
	if err := os.WriteFile(filepath.Join(vaultRoot, "wiki", "core.md"), []byte(wikiDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	rzDoc := "---\nid: brain-risk-zones\ntitle: Risk Zones\ntype: risk_zones\nstatus: active\n---\n# Risk Zones\n"
	if err := os.WriteFile(filepath.Join(vaultRoot, "wiki", "risk_zones.md"), []byte(rzDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, rel := range affected {
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		mustMkdir(filepath.Dir(abs))
		content := "package demo\n\n// SampleFunction anchor for G5 testing.\nfunc SampleFunction() int {\n\treturn 42\n}\n"
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	wiID := "WI-a1b2c3d4e5f6"
	wiSHA := strings.Repeat("a", 64)
	traceID := "trc-20261124T000000Z-a1b2c3d4"
	if impactFlags == nil {
		impactFlags = []string{}
	}
	if riskHits == nil {
		riskHits = []bfcore.RiskHit{}
	}

	wi := WorkItemInput{
		SchemaVersion:     SchemaVersion,
		ID:                wiID,
		DedupeKey:         strings.Repeat("b", 64),
		TraceID:           traceID,
		Revision:          1,
		Status:            "ready",
		Project:           "black-factory",
		Title:             "Fix bounded component",
		Description:       "Detailed problem description",
		Kind:              kind,
		AffectedPaths:     affected,
		ImpactFlags:       impactFlags,
		Lane:              lane,
		SuggestedTemplate: "correccion_hallazgos",
		RiskLevel:         riskLevel,
		RequiresAutorHITL: strings.EqualFold(riskLevel, "CRITICO"),
		RiskHits:          riskHits,
		TriageReasons: []TriageReasonInput{{
			Code:           "FTD_ELIGIBLE_BOUNDED_MAINTENANCE",
			RuleKind:       "deterministic",
			RulesetVersion: "triage-det-v1.0",
			Detail:         "ok",
		}},
		Sanitization: SanitizationInput{
			SanitizedPayloadSHA256: wiSHA,
			G4Redactions:           []bfcore.G4Redaction{},
		},
	}

	wiPath := filepath.Join(factoryRoot, "1_Intake", "runtime", "workitems", wiID+".json")
	rawWI, _ := json.MarshalIndent(wi, "", "  ")
	if err := os.WriteFile(wiPath, rawWI, 0o644); err != nil {
		t.Fatal(err)
	}

	rzSHA := strings.Repeat("9", 64)
	client := &mockBrainClient{
		ctxResp: bfcore.BrainContextResponse{
			HasActiveRiskZones: true,
			RiskZonesSHA256:    rzSHA,
			RiskHits:           riskHits,
			MatchedDocs: []bfcore.BrainDocMatch{
				{ID: "brain-risk-zones", RelPath: "wiki/risk_zones.md", Type: "risk_zones", Status: "active"},
				{ID: "wiki-core", RelPath: "wiki/core.md", Type: "wiki", Status: "active", CoversPaths: []string{"0_Brain/", "1_Intake/", "pkg/"}},
			},
		},
	}

	return &testFixtureEnv{
		root:        root,
		factoryRoot: factoryRoot,
		repoRoot:    repoRoot,
		vaultRoot:   vaultRoot,
		dataDir:     dataDir,
		taskRoot:    taskRoot,
		wiPath:      wiPath,
		wiID:        wiID,
		wiSHA:       wiSHA,
		traceID:     traceID,
		client:      client,
	}
}

func (f *testFixtureEnv) prepare(t *testing.T) PrepareResult {
	t.Helper()
	res, code := Prepare(PrepareOptions{
		WorkItemPath: f.wiPath,
		RepoRoot:     f.repoRoot,
		FactoryRoot:  f.factoryRoot,
		VaultRoot:    f.vaultRoot,
		DataDir:      f.dataDir,
		BrainClient:  f.client,
	})
	if code != ExitOK {
		t.Fatalf("Prepare fallo: code=%d res=%+v", code, res)
	}
	return res
}

func buildValidSpecFromSkeleton(t *testing.T, f *testFixtureEnv, specPath string, canModify []string, asdd bool) Spec {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var spec Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	spec.TargetBranch = "feat/test-branch"
	spec.G3DataMutation = "ninguna (pruebas en t.TempDir())"
	spec.StackAssumptions = []string{"Go 1.26 pure-Go CGO_ENABLED=0"}
	spec.ReadOnlyPaths = []string{"Docs/"}
	spec.GlobalForbidden = []string{"GengiSWAP/"}
	spec.DoD = []string{"go vet y go test en verde sin regresiones"}
	spec.Narrative = Narrative{
		Problem:        "Corregir comportamiento en modulo objetivo",
		RootCause:      "Validacion incompleta detectada en SampleFunction",
		ProposedChange: "Aplicar ajuste determinista y cobertura TDD",
	}
	if asdd {
		spec.Lane = "ASDD"
		spec.Narrative.AlternativesConsidered = []string{"Alternativa A rechazada por acoplamiento"}
		spec.Narrative.InterfacesAndContracts = "Contrato estable sin cambios de esquema"
		spec.Narrative.EdgeCases = []string{"Entrada vacia y rutas Windows"}
		spec.Narrative.RollbackPlan = "Revertir commit de la rama dedicada"
	}

	// Leer spec_context.json para construir observed_files exactamente sobre affected_paths U canModify
	ctxRaw, _ := os.ReadFile(filepath.Join(filepath.Dir(specPath), "spec_context.json"))
	var sc SpecContext
	_ = json.Unmarshal(ctxRaw, &sc)

	obsMap := make(map[string]ObservedFile)
	for _, bh := range sc.BaselineHashes {
		obsMap[strings.ToLower(bh.Path)] = bh
	}
	for _, p := range canModify {
		key := strings.ToLower(p)
		if _, ok := obsMap[key]; !ok {
			st, sum, err := bfcore.ComputeNormalizedFileSHA256(f.repoRoot, p)
			if err != nil {
				t.Fatalf("hash %s: %v", p, err)
			}
			obsMap[key] = ObservedFile{Path: p, State: st, SHA256: sum}
		}
	}
	spec.ObservedFiles = make([]ObservedFile, 0, len(obsMap))
	for _, of := range obsMap {
		spec.ObservedFiles = append(spec.ObservedFiles, of)
	}
	sort.Slice(spec.ObservedFiles, func(i, j int) bool { return spec.ObservedFiles[i].Path < spec.ObservedFiles[j].Path })

	// G5 citation sobre primer archivo existente
	firstExisting := ""
	for _, of := range spec.ObservedFiles {
		if of.State == "present" {
			firstExisting = of.Path
			break
		}
	}
	spec.G5Citations = []G5Citation{{
		Path:         firstExisting,
		Line:         4,
		SymbolAnchor: "SampleFunction",
		Claim:        "SampleFunction retorna 42",
	}}

	affectedSet := make(map[string]bool)
	for _, ap := range sc.WorkItem.AffectedPaths {
		affectedSet[strings.ToLower(ap)] = true
	}
	var expansions []ScopeExpansion
	for _, p := range canModify {
		if !affectedSet[strings.ToLower(p)] {
			expansions = append(expansions, ScopeExpansion{
				Path:      p,
				Rationale: "Archivo auxiliar requerido por TDD",
			})
		}
	}

	spec.WorkUnits = []WorkUnit{{
		ID:              "core_fix",
		Title:           "Corregir logica principal y pruebas",
		Objective:       "Implementar ajuste acotado con pruebas unitarias",
		Inputs:          []string{"Contrato del WorkItem y archivo base"},
		CanModify:       canModify,
		ScopeExpansions: expansions,
		Forbidden:       []string{"0_Brain/vault/"},
		DependsOn:       []string{},
		AcceptanceCriteria: []AcceptanceCriterion{
			{ID: "ac_test", Kind: "command", Description: "Ejecutar go test en verde", VerifyStepID: "vs_test"},
			{ID: "ac_qa", Kind: "review_gate", Description: "Revision Modo QA aprobada", Executor: "Modo_QA"},
		},
		VerifySteps: []VerifyStep{{
			ID:         "vs_test",
			Cwd:        "0_Brain/engine",
			Env:        map[string]string{"CGO_ENABLED": "0"},
			Argv:       []string{"go", "test", "-count=1", "./..."},
			ExpectExit: 0,
			TimeoutSec: 120,
		}},
		FailureProof: FailureProof{
			Description:          "Si SampleFunction retorna 0, go test falla con salida 1",
			ExpectExit:           1,
			ExpectOutputContains: "FAIL",
		},
	}}

	if len(sc.WorkItem.RiskHits) > 0 {
		for _, rh := range sc.WorkItem.RiskHits {
			rm := RiskMitigation{
				RiskID:            rh.ID,
				Level:             rh.Level,
				VerifyStepIDs:     []string{"vs_test"},
				ReviewGates:       []string{"ac_qa"},
				RequiresAutorHITL: strings.EqualFold(rh.Level, "CRITICO"),
			}
			spec.RiskMitigations = append(spec.RiskMitigations, rm)
		}
	}

	return spec
}

func writeSpecFile(t *testing.T, specPath string, spec Spec) {
	t.Helper()
	b, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGoldfishReview(t *testing.T, wiDir string, spec Spec, lenses []string) {
	t.Helper()
	sha, err := CanonicalSpecSHA256(spec)
	if err != nil {
		t.Fatal(err)
	}
	gf := GoldfishReview{
		SchemaVersion:      SchemaVersion,
		SpecSHA256:         sha,
		LensesRun:          lenses,
		UnresolvedBlockers: 0,
		ReportPath:         "Docs/reviews/goldfish_report.md",
		ReviewedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.MarshalIndent(gf, "", "  ")
	if err := os.WriteFile(filepath.Join(wiDir, "goldfish_review.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaJSONTagParity(t *testing.T) {
	schemaPath := filepath.Join("..", "schemas", "spec.schema.json")
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("leyendo spec.schema.json: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("JSON invalido en spec.schema.json: %v", err)
	}

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("spec.schema.json carece de $defs")
	}

	checkObjParity := func(name string, typ reflect.Type, schemaObj map[string]any) {
		t.Helper()
		if ap, exists := schemaObj["additionalProperties"]; !exists || ap != false {
			t.Fatalf("objeto %s en spec.schema.json debe fijar additionalProperties: false", name)
		}
		props, ok := schemaObj["properties"].(map[string]any)
		if !ok {
			t.Fatalf("objeto %s carece de properties", name)
		}
		goTags := make(map[string]bool)
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			goTags[tag] = true
			if _, exists := props[tag]; !exists {
				t.Errorf("%s: campo Go json:%q ausente en spec.schema.json", name, tag)
			}
		}
		for prop := range props {
			if !goTags[prop] {
				t.Errorf("%s: propiedad %q de spec.schema.json ausente en struct Go", name, prop)
			}
		}
	}

	checkObjParity("Spec", reflect.TypeOf(Spec{}), schema)
	defChecks := map[string]reflect.Type{
		"work_item_ref":        reflect.TypeOf(WorkItemRef{}),
		"narrative":            reflect.TypeOf(Narrative{}),
		"observed_file":        reflect.TypeOf(ObservedFile{}),
		"g5_citation":          reflect.TypeOf(G5Citation{}),
		"decision_record":      reflect.TypeOf(DecisionRecord{}),
		"risk_mitigation":      reflect.TypeOf(RiskMitigation{}),
		"brain_followup":       reflect.TypeOf(BrainFollowup{}),
		"work_unit":            reflect.TypeOf(WorkUnit{}),
		"scope_expansion":      reflect.TypeOf(ScopeExpansion{}),
		"acceptance_criterion": reflect.TypeOf(AcceptanceCriterion{}),
		"verify_step":          reflect.TypeOf(VerifyStep{}),
		"failure_proof":        reflect.TypeOf(FailureProof{}),
	}
	for defName, typ := range defChecks {
		obj, ok := defs[defName].(map[string]any)
		if !ok {
			t.Fatalf("$defs.%s ausente en spec.schema.json", defName)
		}
		checkObjParity(defName, typ, obj)
	}
}

func TestCanonicalHashingAndRetryableCatalog(t *testing.T) {
	s1 := Spec{
		SchemaVersion: "1.0",
		WorkUnits:     []WorkUnit{{ID: "u1", VerifySteps: []VerifyStep{{ID: "v1"}}}},
	}
	s2 := Spec{
		SchemaVersion:    "1.0",
		StackAssumptions: []string{},
		ReadOnlyPaths:    []string{},
		GlobalForbidden:  []string{},
		DoD:              []string{},
		Narrative:        Narrative{AlternativesConsidered: []string{}, EdgeCases: []string{}},
		ObservedFiles:    []ObservedFile{},
		G5Citations:      []G5Citation{},
		Decisions:        []DecisionRecord{},
		RiskMitigations:  []RiskMitigation{},
		BrainFollowups:   []BrainFollowup{},
		WorkUnits: []WorkUnit{{
			ID:                 "u1",
			Inputs:             []string{},
			CanModify:          []string{},
			ScopeExpansions:    []ScopeExpansion{},
			Forbidden:          []string{},
			DependsOn:          []string{},
			AcceptanceCriteria: []AcceptanceCriterion{},
			VerifySteps:        []VerifyStep{{ID: "v1", Env: map[string]string{}, Argv: []string{}}},
		}},
	}
	h1, err1 := CanonicalSpecSHA256(s1)
	h2, err2 := CanonicalSpecSHA256(s2)
	if err1 != nil || err2 != nil || h1 != h2 || len(h1) != 64 {
		t.Fatalf("CanonicalSpecSHA256 debia normalizar slices nil y Env nil de forma identica: h1=%s h2=%s err=%v/%v", h1, h2, err1, err2)
	}

	sc1 := ComputeScopeSHA256([]string{"0_Brain/B.go", `0_brain\a.go`, "0_Brain/b.go"})
	sc2 := ComputeScopeSHA256([]string{"0_Brain/a.go", "0_Brain/b.go"})
	if sc1 != sc2 {
		t.Fatalf("ComputeScopeSHA256 debia ser invariante a orden/casing/duplicados: %s != %s", sc1, sc2)
	}

	if !IsRetryableReason(ExitFixable, ReasonValidationFailed) || IsRetryableReason(ExitFixable, ReasonValidatorBypassDefect) {
		t.Fatalf("politica retryable en ExitFixable incorrecta")
	}
	if !IsRetryableReason(ExitInfraOrRefusal, ReasonBrainJournalActive) || IsRetryableReason(ExitInfraOrRefusal, ReasonLiveTaskState) || IsRetryableReason(ExitInfraOrRefusal, ReasonTaskDirExists) {
		t.Fatalf("politica retryable en ExitInfraOrRefusal incorrecta")
	}
	if IsRetryableReason(ExitG4, ReasonG4Violation) || IsRetryableReason(ExitExternalDecision, ReasonStaleContext) || IsRetryableReason(ExitAttemptsBlocked, ReasonMaxValidateAttempts) {
		t.Fatalf("exits 3/4/5 nunca son retryable")
	}
}

func TestPrepare_Lifecycle_Refresh_Reset_And_G4CleanRuntime(t *testing.T) {
	f := newTestFixtureEnv(t, "FTD", "bug", "ALTO", []string{"0_Brain/engine/a.go"}, nil, []bfcore.RiskHit{{
		ID: "RZ-002", Project: "black-factory", Level: "ALTO", MatchedPaths: []string{"0_Brain/engine/a.go"}, MandatoryGate: "go test",
	}})

	// Errores de uso y WorkItem invalido
	if _, code := Prepare(PrepareOptions{}); code != ExitUsage {
		t.Fatalf("Prepare sin --workitem debia retornar ExitUsage (2), obtuvo %d", code)
	}
	if _, code := Prepare(PrepareOptions{WorkItemPath: filepath.Join(f.root, "missing.json")}); code != ExitUsage {
		t.Fatalf("Prepare con --workitem inexistente debia retornar 2, obtuvo %d", code)
	}
	if _, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, ResetAttempts: true, Reason: "corto"}); code != ExitUsage {
		t.Fatalf("--reset-attempts con --reason < 8 chars debia retornar 2, obtuvo %d", code)
	}
	if _, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, ResetAttempts: true, Reason: "token ghp_" + "1234567890abcdefghij123456"}); code != ExitG4 {
		t.Fatalf("--reset-attempts con secreto G4 en --reason debia retornar 3, obtuvo %d", code)
	}

	// Journal activo -> BRAIN_JOURNAL_ACTIVE (exit 6)
	f.client.pfErr = bfcore.ErrBrainJournalActive
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainJournalActive {
		t.Fatalf("esperaba 6 BRAIN_JOURNAL_ACTIVE, obtuvo %d %s", code, res.Reason)
	}
	f.client.pfErr = nil

	// Prepare inicial exitoso
	prep := f.prepare(t)
	if !prep.OK || prep.WorkItemID != f.wiID {
		t.Fatalf("Prepare inicial inesperado: %+v", prep)
	}

	// Verificar que ningun archivo bajo runtime/ contiene rutas absolutas de host (G4 bajo t.TempDir Windows)
	for _, p := range []string{prep.SpecPath, prep.ContextPath, prep.AttemptsPath, filepath.Join(f.dataDir, "specwriter_events.jsonl")} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("leyendo %s: %v", p, err)
		}
		if hits := bfcore.ScanG4All(string(b)); len(hits) != 0 {
			t.Fatalf("artefacto runtime %s disparo G4 bajo t.TempDir(): %+v\n%s", filepath.Base(p), hits, string(b))
		}
	}

	// Segundo prepare sin flags -> SPEC_ALREADY_EXISTS (exit 2)
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonSpecAlreadyExists {
		t.Fatalf("prepare duplicado debia retornar 2 SPEC_ALREADY_EXISTS, obtuvo %d %s", code, res.Reason)
	}

	// Marcar spec.json y attempts.json, probar --refresh-context no destructivo
	customMarker := "// sentinel-keep"
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go"}, false)
	spec.Narrative.Problem = "Problema preservado " + customMarker
	writeSpecFile(t, prep.SpecPath, spec)

	att, _ := LoadAttemptsStrict(filepath.Dir(prep.SpecPath))
	att.DraftAttempts = 2
	att.LastPassedSHA256 = strings.Repeat("c", 64)
	att.LastPassedStage = "draft"
	_ = WriteJSONAtomicG4(prep.AttemptsPath, att)

	refRes, code := Prepare(PrepareOptions{
		WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir,
		RefreshContext: true, BrainClient: f.client,
	})
	if code != ExitOK || !refRes.Refreshed || refRes.Reset {
		t.Fatalf("Prepare --refresh-context fallo: %d %+v", code, refRes)
	}
	specAfter, _ := os.ReadFile(prep.SpecPath)
	if !strings.Contains(string(specAfter), customMarker) {
		t.Fatalf("--refresh-context sobrescribio spec.json")
	}
	attAfter, _ := LoadAttemptsStrict(filepath.Dir(prep.SpecPath))
	if attAfter.DraftAttempts != 2 || attAfter.LastPassedSHA256 != "" || attAfter.LastPassedStage != "" {
		t.Fatalf("--refresh-context debia conservar draft_attempts=2 e invalidar last_passed_*: %+v", attAfter)
	}

	// --reset-attempts --reason reinicia contadores y conserva spec.json
	rstRes, code := Prepare(PrepareOptions{
		WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir,
		ResetAttempts: true, Reason: "Revision manual aprobada por AUTOR", BrainClient: f.client,
	})
	if code != ExitOK || !rstRes.Reset {
		t.Fatalf("Prepare --reset-attempts fallo: %d %+v", code, rstRes)
	}
	attReset, _ := LoadAttemptsStrict(filepath.Dir(prep.SpecPath))
	if attReset.DraftAttempts != 0 || attReset.FinalAttempts != 0 || attReset.ExternalCycles != 0 {
		t.Fatalf("--reset-attempts no reinicio contadores: %+v", attReset)
	}
	specAfterReset, _ := os.ReadFile(prep.SpecPath)
	if !strings.Contains(string(specAfterReset), customMarker) {
		t.Fatalf("--reset-attempts sobrescribio spec.json")
	}
}

func TestValidate_Precedence_Budgets_ExternalCycles_And_Security(t *testing.T) {
	f := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)
	skeletonRaw, err := os.ReadFile(prep.SpecPath)
	if err != nil {
		t.Fatal(err)
	}

	valOpts := func(stage string) ValidateOptions {
		return ValidateOptions{
			SpecPath: prep.SpecPath, Stage: stage, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
			VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
		}
	}

	// 1. Esqueleto inicial (work_units: []) -> Exit 1 VALIDATION_FAILED sin consumir external_cycles
	if res, code := Validate(valOpts("draft")); code != ExitFixable || res.Reason != ReasonValidationFailed {
		t.Fatalf("esqueleto vacio debia retornar 1 VALIDATION_FAILED, obtuvo %d %s (%+v)", code, res.Reason, res.Issues)
	}
	att, _ := LoadAttemptsStrict(wiDir)
	if att.DraftAttempts != 1 || att.ExternalCycles != 0 {
		t.Fatalf("esqueleto vacio debia incrementar draft_attempts=1 y external_cycles=0: %+v", att)
	}

	// 2. JSON roto tras pasar Fase-A -> Exit 1 INVALID_JSON sin entrar a [4]
	_ = os.WriteFile(prep.SpecPath, []byte(`{"schema_version": "1.0", broken`), 0o644)
	if res, code := Validate(valOpts("draft")); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("JSON roto debia retornar 1 INVALID_JSON, obtuvo %d %s", code, res.Reason)
	}

	// 3. G4 Unicode escape (\u0067hp_...) en Fase-B -> Exit 3 G4_VIOLATION (3.er intento)
	_ = os.WriteFile(prep.SpecPath, []byte(`{"schema_version":"1.0","title":"\u0067hp_1234567890abcdefghij123456"}`), 0o644)
	if res, code := Validate(valOpts("draft")); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("secreto JSON-escapado debia retornar 3 G4_VIOLATION, obtuvo %d %s", code, res.Reason)
	}

	// 4. 4.o intento en draft -> corta en paso [5] con Exit 5 MAX_VALIDATE_ATTEMPTS preservando last_exit=3/last_reason=G4_VIOLATION
	if res, code := Validate(valOpts("draft")); code != ExitAttemptsBlocked || res.Reason != ReasonMaxValidateAttempts || res.LastExit != ExitG4 || res.LastReason != ReasonG4Violation {
		t.Fatalf("4.o intento debia retornar 5 MAX_VALIDATE_ATTEMPTS con last_exit=3, obtuvo %d %+v", code, res)
	}

	// Desbloquear con --reset-attempts
	_, _ = Prepare(PrepareOptions{
		WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir,
		ResetAttempts: true, Reason: "Reset de prueba para continuar", BrainClient: f.client,
	})

	// 5. SPEC_TOO_LARGE (> 1 MiB) consume intento y devuelve Exit 1
	bigSpec := make([]byte, MaxSpecBytes+10)
	for i := range bigSpec {
		bigSpec[i] = ' '
	}
	_ = os.WriteFile(prep.SpecPath, bigSpec, 0o644)
	if res, code := Validate(valOpts("draft")); code != ExitFixable || res.Reason != ReasonSpecTooLarge || !res.Retryable {
		t.Fatalf("spec > 1 MiB debia retornar 1 SPEC_TOO_LARGE retryable=true, obtuvo %d %+v", code, res)
	}

	// 6. Spec valida FTD pasa draft (Exit 0) sin tocar final_attempts
	_ = os.WriteFile(prep.SpecPath, skeletonRaw, 0o644)
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go"}, false)
	writeSpecFile(t, prep.SpecPath, spec)
	if res, code := Validate(valOpts("draft")); code != ExitOK {
		t.Fatalf("spec FTD valida debia pasar draft, obtuvo %d %s (%+v)", code, res.Reason, res.Issues)
	}

	// 7. Archivo marcado "absent" que aparece en disco -> Exit 4 STALE_CONTEXT (ciclo 1/3)
	newRel := "0_Brain/engine/new_file.go"
	specWithAbsent := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go", newRel}, false)
	writeSpecFile(t, prep.SpecPath, specWithAbsent)
	// Pasa draft cuando aun esta absent
	if res, code := Validate(valOpts("draft")); code != ExitOK {
		t.Fatalf("spec FTD con archivo absent debia pasar draft, obtuvo %d %+v", code, res)
	}
	// Ahora el archivo aparece en disco -> STALE_CONTEXT (1/3)
	_ = os.WriteFile(filepath.Join(f.repoRoot, filepath.FromSlash(newRel)), []byte("package demo\n"), 0o644)
	if res, code := Validate(valOpts("draft")); code != ExitExternalDecision || res.Reason != ReasonStaleContext {
		t.Fatalf("archivo absent aparecido en disco debia retornar 4 STALE_CONTEXT, obtuvo %d %+v", code, res)
	}
	// Simular ciclos externos 2 y 3 (ej. ESCALATION_REQUIRED al tocar 2.o modulo raiz)
	_ = os.Remove(filepath.Join(f.repoRoot, filepath.FromSlash(newRel)))
	mod2Rel := "1_Intake/engine/mod2.go"
	_ = os.MkdirAll(filepath.Dir(filepath.Join(f.repoRoot, filepath.FromSlash(mod2Rel))), 0o755)
	_ = os.WriteFile(filepath.Join(f.repoRoot, filepath.FromSlash(mod2Rel)), []byte("package intake\n"), 0o644)
	specEsc := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go", mod2Rel}, false)
	writeSpecFile(t, prep.SpecPath, specEsc)
	// Ciclo 2/3: ESCALATION_REQUIRED
	if res, code := Validate(valOpts("draft")); code != ExitExternalDecision || res.Reason != ReasonEscalationRequired {
		t.Fatalf("2.o modulo raiz en FTD debia retornar 4 ESCALATION_REQUIRED, obtuvo %d %+v", code, res)
	}
	// Ciclo 3/3: con decision aprobada pero manteniendo lane=FTD -> LANE_UPGRADE_REQUIRED
	scopeSHA := ComputeScopeSHA256([]string{"0_Brain/engine/a.go", mod2Rel})
	specEsc.Decisions = []DecisionRecord{{
		Reason: "ESCALATION_REQUIRED", DecidedBy: "AUTOR", Rationale: "Necesita modulo 2",
		Timestamp: time.Now().UTC().Format(time.RFC3339), LaneFrom: "FTD", LaneTo: "ASDD",
		ApprovedPaths: []string{"0_Brain/engine/a.go", mod2Rel}, ScopeSHA256: scopeSHA,
	}}
	specEsc.RequiresAutorHITL = true
	writeSpecFile(t, prep.SpecPath, specEsc)
	if res, code := Validate(valOpts("draft")); code != ExitExternalDecision || res.Reason != ReasonLaneUpgradeRequired {
		t.Fatalf("FTD con 2 modulos y decision debia retornar 4 LANE_UPGRADE_REQUIRED (ciclo 3/3), obtuvo %d %+v", code, res)
	}

	// Ahora resolver el 3.er ciclo subiendo a ASDD + pasando draft y final -> debe ser COMPILABLE sin bloqueo permanente!
	specASDD := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go", mod2Rel}, true)
	specASDD.Decisions = specEsc.Decisions
	specASDD.RequiresAutorHITL = true
	writeSpecFile(t, prep.SpecPath, specASDD)
	if res, code := Validate(valOpts("draft")); code != ExitOK {
		t.Fatalf("resolucion del 3.er ciclo externo en ASDD draft debia pasar (0), obtuvo %d %+v", code, res)
	}
	// En --stage final sin goldfish_review.json -> Exit 1 VALIDATION_FAILED (NO exit 6!)
	if res, code := Validate(valOpts("final")); code != ExitFixable || res.Reason != ReasonValidationFailed {
		t.Fatalf("ASDD final sin goldfish_review.json debia retornar 1 VALIDATION_FAILED, obtuvo %d %+v", code, res)
	}
	// Escribir goldfish_review.json valido -> Exit 0
	writeGoldfishReview(t, wiDir, specASDD, []string{"egm-design", "egm-codebase-details", "egm-implementation-readiness"})
	if res, code := Validate(valOpts("final")); code != ExitOK {
		t.Fatalf("ASDD final con goldfish valido debia pasar (0), obtuvo %d %+v", code, res)
	}
	// Compilar tras resolver el 3.er ciclo externo DEBE tener exito!
	compRes, compCode := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_resolved_cycle3",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	})
	if compCode != ExitOK || !compRes.OK {
		t.Fatalf("compile tras resolver 3.er ciclo externo debia pasar (0), obtuvo %d %+v", compCode, compRes)
	}

	// Un 4.o ciclo externo si dispara Exit 5 MAX_EXTERNAL_CYCLES
	_ = os.WriteFile(filepath.Join(f.repoRoot, "0_Brain", "engine", "a.go"), []byte("package demo\n// modified\n"), 0o644)
	if res, code := Validate(valOpts("final")); code != ExitAttemptsBlocked || res.Reason != ReasonMaxExternalCycles {
		t.Fatalf("4.o ciclo externo debia retornar 5 MAX_EXTERNAL_CYCLES, obtuvo %d %+v", code, res)
	}
}

func TestValidate_ASDD_NoFalseEscalation_UnknownFields_G2_G5_Casing_And_DAG(t *testing.T) {
	// ASDD que ya inicia en 2 modulos raiz NO debe disparar falso ESCALATION_REQUIRED
	f := newTestFixtureEnv(t, "ASDD", "feature", "NONE", []string{"0_Brain/engine/a.go", "1_Intake/engine/b.go"}, []string{"contract"}, nil)
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go", "1_Intake/engine/b.go"}, true)
	writeSpecFile(t, prep.SpecPath, spec)

	res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "draft", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	})
	if code != ExitOK {
		t.Fatalf("ASDD multi-modulo inicial no debia disparar ESCALATION_REQUIRED: %d %+v", code, res)
	}

	// En ASDD con impact_flag=contract, goldfish requiere egm-structural!
	writeGoldfishReview(t, wiDir, spec, []string{"egm-design", "egm-codebase-details", "egm-implementation-readiness"})
	if res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "final", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitFixable || res.Reason != ReasonValidationFailed {
		t.Fatalf("ASDD con impact_flag=contract debia exigir egm-structural en goldfish_review.json: %d %+v", code, res)
	}

	// Pruebas directas sobre ValidateSpecAndRenderCheck:
	ctxObj, _, _ := loadSpecContextStrict(wiDir)
	mustFail := func(name string, mutate func(s *Spec), rawPatch func([]byte) []byte) {
		t.Helper()
		cp := NormalizeSpecForCanonicalCopy(spec)
		if mutate != nil {
			mutate(&cp)
		}
		b, _ := json.Marshal(cp)
		if rawPatch != nil {
			b = rawPatch(b)
		}
		issues, _, _, err := ValidateSpecAndRenderCheck(b, cp, ctxObj, wiDir, f.repoRoot, f.factoryRoot, "draft", "t_case", nil, false)
		if err != nil {
			t.Fatalf("%s error inesperado: %v", name, err)
		}
		if len(issues) == 0 {
			t.Fatalf("caso negativo %q debia producir issues en ValidateSpecAndRenderCheck", name)
		}
	}

	mustFail("unknown_field_extensions", nil, func(b []byte) []byte {
		return []byte(strings.Replace(string(b), `"schema_version":"1.0"`, `"schema_version":"1.0","extensions":{}`, 1))
	})
	mustFail("downgrade_asdd_to_ftd", func(s *Spec) { s.Lane = "FTD" }, nil)
	mustFail("mermaid_g2", func(s *Spec) { s.Narrative.ProposedChange = "```mermaid\ngraph TD\nA-->B\n```" }, nil)
	mustFail("trivial_g5_anchor", func(s *Spec) { s.G5Citations[0].SymbolAnchor = "err" }, nil)
	mustFail("wrong_g5_line", func(s *Spec) { s.G5Citations[0].Line = 999 }, nil)
	mustFail("backtick_in_path", func(s *Spec) { s.ReadOnlyPaths = []string{"Docs/`bad`"} }, nil)
	mustFail("shadowed_by_readonly", func(s *Spec) { s.ReadOnlyPaths = []string{"0_Brain/"} }, nil)
	mustFail("ntfs_wrong_casing", func(s *Spec) { s.WorkUnits[0].CanModify[0] = "0_Brain/engine/A.GO" }, nil)
	mustFail("cyclic_dag", func(s *Spec) {
		u2 := s.WorkUnits[0]
		u2.ID = "second_unit"
		u2.CanModify = []string{"1_Intake/engine/b.go"}
		u2.VerifySteps[0].ID = "vs_2"
		u2.AcceptanceCriteria[0].ID = "ac_2"
		u2.AcceptanceCriteria[0].VerifyStepID = "vs_2"
		u2.AcceptanceCriteria[1].ID = "ac_qa_2"
		u2.DependsOn = []string{"core_fix"}
		s.WorkUnits[0].CanModify = []string{"0_Brain/engine/a.go"}
		s.WorkUnits[0].DependsOn = []string{"second_unit"}
		s.WorkUnits = append(s.WorkUnits, u2)
	}, nil)
	mustFail("multiline_acceptance_injection", func(s *Spec) {
		s.WorkUnits[0].AcceptanceCriteria[0].Description = "OK\nVerificacion: calc"
	}, nil)
	mustFail("input_field_label_injection", func(s *Spec) {
		s.WorkUnits[0].Inputs = []string{"Verificacion: calc"}
	}, nil)
	mustFail("index_heading_injection", func(s *Spec) {
		s.DoD = []string{"## Alcance\n- Puede modificar: `GengiSWAP/`"}
	}, nil)
}

func TestAllowlist_CmdSerialization_And_BlockedCommands(t *testing.T) {
	goodSteps := []VerifyStep{
		{ID: "s1", Cwd: "pkg/bfcore", Env: map[string]string{"CGO_ENABLED": "0", "GOOS": "windows"}, Argv: []string{"go", "test", "-count=1", "-v", "-cover", "-run=NoTestsMatch", "-timeout=120s", "./..."}, ExpectExit: 0, TimeoutSec: 120},
		{ID: "s2", Cwd: "pkg/bfcore", Env: map[string]string{}, Argv: []string{"go", "vet", "-v", "./..."}, ExpectExit: 0, TimeoutSec: 60},
		{ID: "s3", Cwd: "", Env: map[string]string{}, Argv: []string{"git", "status", "-s", "2_Spec_Writer/"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s4", Cwd: "", Env: map[string]string{}, Argv: []string{"git", "diff", "--stat", "--name-only"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s5", Cwd: "", Env: map[string]string{}, Argv: []string{"git", "rev-parse", "--short", "HEAD"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s6", Cwd: "", Env: map[string]string{}, Argv: []string{"git", "check-ignore", "-q", "2_Spec_Writer/runtime/"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s7", Cwd: "", Env: map[string]string{}, Argv: []string{"0_Brain/brain.exe", "query", "--vault", "0_Brain/vault", "--q", "validator", "--limit", "5", "--json"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s8", Cwd: "", Env: map[string]string{}, Argv: []string{"0_Brain/brain.exe", "context", "--vault", "0_Brain/vault", "--files", "pkg/a.go,pkg/b.go", "--json"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s9", Cwd: "", Env: map[string]string{}, Argv: []string{"1_Intake/intake.exe", "list", "--data-dir", "1_Intake/runtime", "--json"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s10", Cwd: "", Env: map[string]string{}, Argv: []string{"1_Intake/intake.exe", "show", "--data-dir", "1_Intake/runtime", "--id", "WI-a1b2c3d4e5f6", "--format", "md"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s11", Cwd: "", Env: map[string]string{}, Argv: []string{"2_Spec_Writer/specwriter.exe", "verify", "--factory-root", ".", "--repo-root", ".", "--json"}, ExpectExit: 0, TimeoutSec: 30},
		{ID: "s12", Cwd: "", Env: map[string]string{}, Argv: []string{"2_Spec_Writer/specwriter.exe", "render", "--spec", "2_Spec_Writer/runtime/spec.json", "--json"}, ExpectExit: 0, TimeoutSec: 30},
	}
	for _, st := range goodSteps {
		var issues []ValidationIssue
		validateVerifyStepAllowlist(st, "/vs", func(p, c, m string) {
			issues = append(issues, ValidationIssue{Pointer: p, Code: c, Message: m})
		})
		if len(issues) != 0 {
			t.Fatalf("verify_step valido %s fallo allowlist: %+v", st.ID, issues)
		}
	}

	// Serializacion determinista Windows cmd.exe
	cmdLine := SerializeVerifyStepCmd(goodSteps[0])
	wantCmd := `set CGO_ENABLED=0&& set GOOS=windows&& cd /d "pkg\bfcore" && go test -count=1 -v -cover -run=NoTestsMatch -timeout=120s ./...`
	if cmdLine != wantCmd {
		t.Fatalf("SerializeVerifyStepCmd = %q, esperaba %q", cmdLine, wantCmd)
	}
	cmdBrain := SerializeVerifyStepCmd(goodSteps[7])
	if !strings.HasPrefix(cmdBrain, `0_Brain\brain.exe context`) || !strings.Contains(cmdBrain, `"pkg/a.go,pkg/b.go"`) {
		t.Fatalf("SerializeVerifyStepCmd brain.exe inesperado: %q", cmdBrain)
	}

	badSteps := []struct {
		name string
		step VerifyStep
	}{
		{"brain_verify_banned", VerifyStep{ID: "b1", Argv: []string{"0_Brain/brain.exe", "verify"}, ExpectExit: 0, TimeoutSec: 30}},
		{"brain_query_wrong_flag", VerifyStep{ID: "b2", Argv: []string{"0_Brain/brain.exe", "query", "--query", "x"}, ExpectExit: 0, TimeoutSec: 30}},
		{"brain_nonempty_cwd", VerifyStep{ID: "b3", Cwd: "0_Brain", Argv: []string{"0_Brain/brain.exe", "query", "--q", "x"}, ExpectExit: 0, TimeoutSec: 30}},
		{"intake_verify_banned", VerifyStep{ID: "b4", Argv: []string{"1_Intake/intake.exe", "verify"}, ExpectExit: 0, TimeoutSec: 30}},
		{"gengis_status_banned", VerifyStep{ID: "b5", Argv: []string{"GengiSWAP/gengis.exe", "status", "t"}, ExpectExit: 0, TimeoutSec: 30}},
		{"go_build_banned", VerifyStep{ID: "b6", Argv: []string{"go", "build", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"go_test_caret_run", VerifyStep{ID: "b7", Argv: []string{"go", "test", "-run=^$", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"go_test_ldflags", VerifyStep{ID: "b8", Argv: []string{"go", "test", "-ldflags=-extld=calc", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"go_test_cpuprofile", VerifyStep{ID: "b9", Argv: []string{"go", "test", "-test.cpuprofile=cpu.out", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"go_test_output_o", VerifyStep{ID: "b10", Argv: []string{"go", "test", "-o", "out.exe", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"go_test_c_flag", VerifyStep{ID: "b11", Argv: []string{"go", "test", "-c", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"env_gengis_bin_banned", VerifyStep{ID: "b12", Env: map[string]string{"GENGIS_BIN": "calc.exe"}, Argv: []string{"go", "test", "./..."}, ExpectExit: 0, TimeoutSec: 30}},
		{"trailing_backtick", VerifyStep{ID: "b13", Argv: []string{"go", "test", "./...`"}, ExpectExit: 0, TimeoutSec: 30}},
		{"git_diff_output", VerifyStep{ID: "b14", Argv: []string{"git", "diff", "--output=out.txt"}, ExpectExit: 0, TimeoutSec: 30}},
		{"shell_powershell", VerifyStep{ID: "b15", Argv: []string{"powershell", "-Command", "dir"}, ExpectExit: 0, TimeoutSec: 30}},
	}
	for _, tc := range badSteps {
		var issues []ValidationIssue
		validateVerifyStepAllowlist(tc.step, "/vs", func(p, c, m string) {
			issues = append(issues, ValidationIssue{Pointer: p, Code: c, Message: m})
		})
		if len(issues) == 0 {
			t.Fatalf("verify_step prohibido %q debia ser rechazado por allowlist", tc.name)
		}
	}
}

func TestRender_And_Compile_AntiBypass_CreateOnly_And_Locks(t *testing.T) {
	f := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go"}, false)
	writeSpecFile(t, prep.SpecPath, spec)

	// Render sobre spec valida -> stdout-only, sellado con spec_sha256, cero Mermaid
	specBytes, _ := os.ReadFile(prep.SpecPath)
	md, _, rCode, rReason, rErr := RenderSpecMarkdown(specBytes)
	if rCode != ExitOK || rReason != ReasonOK || rErr != nil || !strings.Contains(md, "AUTO-GENERATED DERIVED VIEW") || strings.Contains(md, "```mermaid") {
		t.Fatalf("RenderSpecMarkdown fallo: code=%d reason=%s err=%v", rCode, rReason, rErr)
	}
	// Render con schema_version invalido -> Exit 1 VALIDATION_FAILED
	if _, _, c, r, _ := RenderSpecMarkdown([]byte(`{"schema_version":"9.9"}`)); c != ExitFixable || r != ReasonValidationFailed {
		t.Fatalf("Render schema_version=9.9 debia retornar 1 VALIDATION_FAILED, obtuvo %d %s", c, r)
	}

	// Compile sin validate previo -> Exit 1 VALIDATION_REQUIRED retryable=true
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_test_task",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitFixable || res.Reason != ReasonValidationRequired || !res.Retryable {
		t.Fatalf("Compile sin validate debia retornar 1 VALIDATION_REQUIRED retryable=true, obtuvo %d %+v", code, res)
	}

	// TaskID reservado Windows -> Exit 2 INVALID_USAGE
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "con",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("task-id reservado 'con' debia retornar 2 INVALID_USAGE, obtuvo %d %+v", code, res)
	}

	// Pasar validate draft
	if res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "draft", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitOK {
		t.Fatalf("Validate draft fallo: %d %+v", code, res)
	}

	// LIVE_TASK_STATE (.ring/state.json existe) -> Exit 6 retryable=false
	liveDir := filepath.Join(f.taskRoot, "bf_live_task")
	_ = os.MkdirAll(filepath.Join(liveDir, ".ring"), 0o755)
	_ = os.WriteFile(filepath.Join(liveDir, ".ring", "state.json"), []byte(`{"status":"RUNNING"}`), 0o644)
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_live_task",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitInfraOrRefusal || res.Reason != ReasonLiveTaskState || res.Retryable {
		t.Fatalf("esperaba 6 LIVE_TASK_STATE retryable=false, obtuvo %d %+v", code, res)
	}

	// Compilacion valida create-only
	compRes, compCode := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_ok_task",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	})
	if compCode != ExitOK || !compRes.OK {
		t.Fatalf("Compile valido fallo: %d %+v", compCode, compRes)
	}
	idxContent, err := os.ReadFile(filepath.Join(compRes.TaskDir, "index.md"))
	if err != nil || strings.Contains(string(idxContent), "<TARGET_REPO>") {
		t.Fatalf("index.md externo debia sustituir <TARGET_REPO>: err=%v", err)
	}
	chunkContent, err := os.ReadFile(filepath.Join(compRes.TaskDir, "chunks", "01_core_fix.md"))
	if err != nil || !strings.Contains(string(chunkContent), "## Puede modificar") {
		t.Fatalf("chunk compilado debia contener ## Puede modificar: err=%v", err)
	}

	// Re-compilar al mismo task-id sin .ring/state.json -> TASK_DIR_EXISTS (Exit 6, retryable=false)
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_ok_task",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitInfraOrRefusal || res.Reason != ReasonTaskDirExists || res.Retryable {
		t.Fatalf("esperaba 6 TASK_DIR_EXISTS retryable=false, obtuvo %d %+v", code, res)
	}

	// VALIDATOR_BYPASS_DEFECT: forzar attempts.json coincidente para una spec invalida (ej. Mermaid)
	badSpec := spec
	badSpec.Narrative.ProposedChange = "```mermaid\ngraph TD\nA-->B\n```"
	writeSpecFile(t, prep.SpecPath, badSpec)
	badSHA, _ := CanonicalSpecSHA256(badSpec)
	att, _ := LoadAttemptsStrict(wiDir)
	att.LastPassedSHA256 = badSHA
	att.LastPassedStage = "draft"
	_ = WriteJSONAtomicG4(filepath.Join(wiDir, "attempts.json"), att)

	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_bypass_test",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitFixable || res.Reason != ReasonValidatorBypassDefect || res.Retryable {
		t.Fatalf("esperaba 1 VALIDATOR_BYPASS_DEFECT retryable=false, obtuvo %d %+v", code, res)
	}
}

func TestVerifyRuntime_ReadOnly_Priority_And_Drift(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no disponible en PATH")
	}

	root := t.TempDir()
	if err := exec.Command("git", "init", root).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
	})

	// Copiar GengiSWAP/engine/{bridge,ring}.go reales para hashes anclados exactos
	realGengisDir := filepath.Clean(filepath.Join("..", "..", "GengiSWAP", "engine"))
	dstGengisDir := filepath.Join(root, "GengiSWAP", "engine")
	_ = os.MkdirAll(dstGengisDir, 0o755)
	for _, fn := range []string{"bridge.go", "ring.go"} {
		b, err := os.ReadFile(filepath.Join(realGengisDir, fn))
		if err != nil {
			t.Fatalf("leyendo %s real: %v", fn, err)
		}
		if err := os.WriteFile(filepath.Join(dstGengisDir, fn), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// .gitignore completo Option i
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("2_Spec_Writer/runtime/\n2_Spec_Writer/specwriter.exe\n"), 0o644)

	vaultDir := filepath.Join(root, "0_Brain", "vault")
	dataDir := filepath.Join(root, "2_Spec_Writer", "runtime")
	client := &mockBrainClient{}

	// 1. Checkout limpio sin runtime/ -> OK=true y sigue sin crear runtime/
	rep, code := VerifyRuntime(VerifyOptions{FactoryRoot: root, RepoRoot: root, VaultRoot: vaultDir, DataDir: dataDir, BrainClient: client})
	if code != ExitOK || !rep.OK || !rep.GitIgnoreOK || !rep.GengisHashesOK || !rep.BrainOK || rep.ArtifactsChecked != 0 {
		t.Fatalf("VerifyRuntime en checkout limpio fallo: code=%d rep=%+v", code, rep)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("VerifyRuntime creo dataDir (viola solo lectura)")
	}

	// 2. .gitignore ignora *.exe pero NO 2_Spec_Writer/runtime/ -> GITIGNORE_DRIFT (Exit 6, retryable=false)
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.exe\n"), 0o644)
	rep, code = VerifyRuntime(VerifyOptions{FactoryRoot: root, RepoRoot: root, VaultRoot: vaultDir, DataDir: dataDir, BrainClient: client})
	if code != ExitInfraOrRefusal || rep.Reason != ReasonGitIgnoreDrift || rep.GitIgnoreOK || rep.Retryable {
		t.Fatalf("esperaba 6 GITIGNORE_DRIFT cuando falta runtime/, obtuvo %d %+v", code, rep)
	}
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("2_Spec_Writer/runtime/\n2_Spec_Writer/specwriter.exe\n"), 0o644)

	// 3. Deriva en GengiSWAP/engine/bridge.go -> GENGIS_ENGINE_DRIFT (Exit 6, retryable=false)
	_ = os.WriteFile(filepath.Join(dstGengisDir, "bridge.go"), []byte("package ring\n// drifted\n"), 0o644)
	rep, code = VerifyRuntime(VerifyOptions{FactoryRoot: root, RepoRoot: root, VaultRoot: vaultDir, DataDir: dataDir, BrainClient: client})
	if code != ExitInfraOrRefusal || rep.Reason != ReasonGengisEngineDrift || rep.GengisHashesOK || rep.Retryable {
		t.Fatalf("esperaba 6 GENGIS_ENGINE_DRIFT, obtuvo %d %+v", code, rep)
	}

	// 4. Prioridad determinista: G4_VIOLATION_ON_DISK (3) prevalece sobre GENGIS_ENGINE_DRIFT (6)
	_ = os.MkdirAll(filepath.Join(dataDir, "black-factory", "WI-111111111111"), 0o755)
	_ = os.WriteFile(filepath.Join(dataDir, "black-factory", "WI-111111111111", "spec.json"), []byte(`{"title":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	rep, code = VerifyRuntime(VerifyOptions{FactoryRoot: root, RepoRoot: root, VaultRoot: vaultDir, DataDir: dataDir, BrainClient: client})
	if code != ExitG4 || rep.Reason != ReasonG4ViolationOnDisk || rep.G4Issues == 0 {
		t.Fatalf("esperaba prioridad 3 G4_VIOLATION_ON_DISK sobre GENGIS_ENGINE_DRIFT, obtuvo %d %+v", code, rep)
	}

	// Restaurar bridge.go y limpiar G4 -> probar ATTEMPTS_STATE_CORRUPT y ARTIFACT_STATE_CORRUPT
	origBridge, _ := os.ReadFile(filepath.Join(realGengisDir, "bridge.go"))
	_ = os.WriteFile(filepath.Join(dstGengisDir, "bridge.go"), origBridge, 0o644)
	_ = os.WriteFile(filepath.Join(dataDir, "black-factory", "WI-111111111111", "spec.json"), []byte(`{broken json`), 0o644)
	rep, code = VerifyRuntime(VerifyOptions{FactoryRoot: root, RepoRoot: root, VaultRoot: vaultDir, DataDir: dataDir, BrainClient: client})
	if code != ExitInfraOrRefusal || rep.Reason != ReasonArtifactStateCorrupt {
		t.Fatalf("esperaba 6 ARTIFACT_STATE_CORRUPT, obtuvo %d %+v", code, rep)
	}
	_ = os.WriteFile(filepath.Join(dataDir, "black-factory", "WI-111111111111", "attempts.json"), []byte(`{"schema_version":"9.9"}`), 0o644)
	rep, code = VerifyRuntime(VerifyOptions{FactoryRoot: root, RepoRoot: root, VaultRoot: vaultDir, DataDir: dataDir, BrainClient: client})
	if code != ExitInfraOrRefusal || rep.Reason != ReasonAttemptsStateCorrupt {
		t.Fatalf("esperaba prioridad 6 ATTEMPTS_STATE_CORRUPT sobre ARTIFACT_STATE_CORRUPT, obtuvo %d %+v", code, rep)
	}
}

func TestLatencyBudgetP95(t *testing.T) {
	f := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)
	prep := f.prepare(t)
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go"}, false)
	writeSpecFile(t, prep.SpecPath, spec)

	// Poblar 50 artefactos validos en runtime/ para evaluar verify bajo carga real de SG6
	for i := 0; i < 50; i++ {
		dir := filepath.Join(f.dataDir, "black-factory", "WI-"+strings.Repeat("0", 10)+string(rune('a'+(i%26)))+string(rune('a'+((i/26)%26))))
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "spec.json"), []byte(`{"schema_version":"1.0","title":"bench"}`), 0o644)
	}

	const iters = 20
	var valDurs, renDurs, compDurs []time.Duration
	specBytes, _ := os.ReadFile(prep.SpecPath)

	for i := 0; i < iters; i++ {
		t0 := time.Now()
		if res, code := Validate(ValidateOptions{
			SpecPath: prep.SpecPath, Stage: "draft", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
			VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
		}); code != ExitOK {
			t.Fatalf("iter %d validate fallo: %d %+v", i, code, res)
		}
		valDurs = append(valDurs, time.Since(t0))

		t1 := time.Now()
		if _, _, code, _, _ := RenderSpecMarkdown(specBytes); code != ExitOK {
			t.Fatalf("iter %d render fallo: %d", i, code)
		}
		renDurs = append(renDurs, time.Since(t1))

		t2 := time.Now()
		if res, code := Compile(CompileOptions{
			SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_bench_" + string(rune('a'+(i%26))) + string(rune('0'+(i/26))),
			RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
		}); code != ExitOK {
			t.Fatalf("iter %d compile fallo: %d %+v", i, code, res)
		}
		compDurs = append(compDurs, time.Since(t2))
	}

	p95 := func(durs []time.Duration) time.Duration {
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		idx := (len(durs)*95 + 99) / 100
		if idx >= len(durs) {
			idx = len(durs) - 1
		}
		return durs[idx]
	}

	for name, d := range map[string]time.Duration{
		"validate": p95(valDurs),
		"render":   p95(renDurs),
		"compile":  p95(compDurs),
	} {
		t.Logf("p95 %s = %v (presupuesto < 500ms)", name, d)
		if d >= 500*time.Millisecond {
			t.Fatalf("presupuesto de latencia SG6 violado en %s: p95=%v >= 500ms", name, d)
		}
	}
}

func TestEdgePaths_PrepareValidateCompileRenderStateVerify(t *testing.T) {
	// 1. State helpers: WriteJSONAtomicG4 G4 block, AppendEvent redaction, LoadAttemptsStrict corrupt cases, Lock contention
	tmp := t.TempDir()
	if err := WriteJSONAtomicG4(filepath.Join(tmp, "bad.json"), map[string]string{"tok": "ghp_" + "1234567890abcdefghij123456"}); err == nil {
		t.Fatalf("WriteJSONAtomicG4 debia rechazar secreto G4")
	}
	if err := AppendEvent("", SpecWriterEvent{Event: "X"}); err == nil {
		t.Fatalf("AppendEvent con dataDir vacio debia fallar")
	}
	if err := AppendEvent(tmp, SpecWriterEvent{
		Event:  "TEST_EVENT",
		Detail: "secret ghp_" + "1234567890abcdefghij123456 path C:\\" + "Users\\alice\\x.go",
	}); err != nil {
		t.Fatalf("AppendEvent fallo: %v", err)
	}
	evBytes, _ := os.ReadFile(filepath.Join(tmp, "specwriter_events.jsonl"))
	if hits := bfcore.ScanG4All(string(evBytes)); len(hits) != 0 {
		t.Fatalf("AppendEvent dejo hallazgos G4: %+v", hits)
	}

	if _, err := LoadAttemptsStrict(tmp); err == nil {
		t.Fatalf("LoadAttemptsStrict sobre dir sin attempts.json debia fallar")
	}
	for _, badAtt := range []string{
		`{"schema_version":"1.0","wi_id":"WI-1","project":"bf","unknown":1}`,
		`{"schema_version":"1.0","wi_id":"WI-1","project":"bf"} {"extra":1}`,
		`{"schema_version":"9.9","wi_id":"WI-1","project":"bf"}`,
		`{"schema_version":"1.0","wi_id":"WI-1","project":"bf","draft_attempts":-1}`,
		`{"schema_version":"1.0","wi_id":"WI-1","project":"bf","last_passed_stage":"bad"}`,
	} {
		_ = os.WriteFile(filepath.Join(tmp, "attempts.json"), []byte(badAtt), 0o644)
		if _, err := LoadAttemptsStrict(tmp); err == nil {
			t.Fatalf("LoadAttemptsStrict debia rechazar %s", badAtt)
		}
	}

	lk1, err := AcquireWorkItemLock(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireWorkItemLock(tmp); err == nil {
		t.Fatalf("AcquireWorkItemLock concurrente debia fallar con ErrLockBusy")
	}
	_ = lk1.Unlock()

	cl1, err := AcquireCompileLock(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireCompileLock(tmp); err == nil {
		t.Fatalf("AcquireCompileLock concurrente debia fallar con ErrLockBusy")
	}
	_ = cl1.Unlock()

	// 2. Prepare invalid WorkItem permutations, directory, G4, broken JSON, brain.Context error
	f := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)
	if _, code := Prepare(PrepareOptions{WorkItemPath: f.root}); code != ExitUsage {
		t.Fatalf("Prepare con directorio debia retornar 2, obtuvo %d", code)
	}
	badJSONPath := filepath.Join(f.root, "bad_wi.json")
	_ = os.WriteFile(badJSONPath, []byte(`{broken`), 0o644)
	if _, code := Prepare(PrepareOptions{WorkItemPath: badJSONPath}); code != ExitUsage {
		t.Fatalf("Prepare con JSON roto debia retornar 2, obtuvo %d", code)
	}
	g4WIPath := filepath.Join(f.root, "g4_wi.json")
	_ = os.WriteFile(g4WIPath, []byte(`{"title":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	if _, code := Prepare(PrepareOptions{WorkItemPath: g4WIPath}); code != ExitG4 {
		t.Fatalf("Prepare con G4 en WorkItem debia retornar 3, obtuvo %d", code)
	}
	// WorkItem invalido (needs_clarification, etc.)
	invalidWIs := []WorkItemInput{
		{SchemaVersion: "9.9"},
		{SchemaVersion: "1.0", Status: "needs_clarification"},
		{SchemaVersion: "1.0", Status: "ready", ID: "bad"},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 0},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: "short"}},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: ""},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: "t1", Project: "../bad"},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: "t1", Project: "bf", Lane: "BAD"},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: "t1", Project: "bf", Lane: "FTD", SuggestedTemplate: "bad"},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: "t1", Project: "bf", Lane: "FTD", SuggestedTemplate: "correccion_hallazgos", Kind: "bad"},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: "t1", Project: "bf", Lane: "FTD", SuggestedTemplate: "correccion_hallazgos", Kind: "bug", AffectedPaths: nil},
		{SchemaVersion: "1.0", Status: "ready", ID: "WI-a1b2c3d4e5f6", Revision: 1, Sanitization: SanitizationInput{SanitizedPayloadSHA256: strings.Repeat("a", 64)}, TraceID: "t1", Project: "bf", Lane: "FTD", SuggestedTemplate: "correccion_hallazgos", Kind: "bug", AffectedPaths: []string{"../escape.go"}},
	}
	for idx, inv := range invalidWIs {
		if _, msg := validateWorkItemForPrepare(inv, f.repoRoot); msg == "" {
			t.Fatalf("invalidWIs[%d] debia rechazarse", idx)
		}
	}
	// Refresh on nonexistent -> Exit 2
	if _, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, RefreshContext: true, BrainClient: f.client}); code != ExitUsage {
		t.Fatalf("refresh sobre WI no preparado debia retornar 2, obtuvo %d", code)
	}
	// Brain.Context fallo en prepare -> Exit 6 BRAIN_UNAVAILABLE
	f.client.ctxErr = bfcore.ErrBrainUnavailable
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainUnavailable {
		t.Fatalf("brain.Context fallo debia retornar 6 BRAIN_UNAVAILABLE, obtuvo %d %s", code, res.Reason)
	}
	f.client.ctxErr = nil

	// 3. Render full ASDD spec (with Decisions, RiskMitigations, BrainFollowups, multi-unit DependsOn) + error branches
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go"}, true)
	spec.Decisions = []DecisionRecord{{
		Reason: "ESCALATION_REQUIRED", DecidedBy: "AUTOR", Rationale: "Ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339), LaneFrom: "FTD", LaneTo: "ASDD",
		ApprovedPaths: []string{"0_Brain/engine/a.go"}, ScopeSHA256: ComputeScopeSHA256([]string{"0_Brain/engine/a.go"}),
	}}
	spec.RiskMitigations = []RiskMitigation{{RiskID: "RZ-002", Level: "ALTO", VerifyStepIDs: []string{"vs_test"}, ReviewGates: []string{"ac_qa"}}}
	spec.BrainFollowups = []BrainFollowup{{TargetHint: "wiki/core.md", Summary: "Documentar cambio", Trigger: "post-merge"}}
	u2 := spec.WorkUnits[0]
	u2.ID = "second_unit"
	u2.DependsOn = []string{"core_fix"}
	u2.VerifySteps[0].ID = "vs_test_2"
	u2.AcceptanceCriteria[0].ID = "ac_test_2"
	u2.AcceptanceCriteria[0].VerifyStepID = "vs_test_2"
	u2.AcceptanceCriteria[1].ID = "ac_qa_2"
	spec.WorkUnits = append(spec.WorkUnits, u2)
	bASDD, _ := json.Marshal(spec)
	if md, _, c, _, err := RenderSpecMarkdown(bASDD); c != ExitOK || err != nil || !strings.Contains(md, "Alternativas Consideradas") || !strings.Contains(md, "Seguimientos Propuestos") {
		t.Fatalf("RenderSpecMarkdown ASDD completo fallo: c=%d err=%v", c, err)
	}
	if _, _, c, r, _ := RenderSpecMarkdown(make([]byte, MaxSpecBytes+1)); c != ExitFixable || r != ReasonSpecTooLarge {
		t.Fatalf("Render > MaxSpecBytes debia retornar 1 SPEC_TOO_LARGE, obtuvo %d %s", c, r)
	}
	if _, _, c, r, _ := RenderSpecMarkdown([]byte(`{broken`)); c != ExitFixable || r != ReasonInvalidJSON {
		t.Fatalf("Render JSON roto debia retornar 1 INVALID_JSON, obtuvo %d %s", c, r)
	}
	if _, _, c, r, _ := RenderSpecMarkdown([]byte(`{"schema_version":"1.0","title":"ghp_` + `1234567890abcdefghij123456"}`)); c != ExitG4 || r != ReasonG4Violation {
		t.Fatalf("Render G4 debia retornar 3 G4_VIOLATION, obtuvo %d %s", c, r)
	}

	// 4. Validate edge cases: missing/dir --spec, bad --stage, goldfish errors, STALE_CONTEXT on repo_root & risk_zones & matched_doc, LANE_UPGRADE_REQUIRED reasons
	if _, code := Validate(ValidateOptions{}); code != ExitUsage {
		t.Fatalf("Validate sin args debia retornar 2")
	}
	if _, code := Validate(ValidateOptions{SpecPath: filepath.Join(f.root, "nope.json"), Stage: "draft"}); code != ExitUsage {
		t.Fatalf("Validate spec inexistente debia retornar 2")
	}
	if _, code := Validate(ValidateOptions{SpecPath: f.root, Stage: "draft"}); code != ExitUsage {
		t.Fatalf("Validate spec directorio debia retornar 2")
	}

	// Goldfish edge cases in validate --stage final: >256KiB, G4, broken JSON, wrong spec_sha256, unresolved_blockers=1, bad report_path, >3 units needing egm-zoom-out
	writeSpecFile(t, prep.SpecPath, spec)
	gfPath := filepath.Join(wiDir, "goldfish_review.json")
	_ = os.WriteFile(gfPath, make([]byte, MaxGoldfishBytes+5), 0o644)
	if res, code := Validate(ValidateOptions{SpecPath: prep.SpecPath, Stage: "final", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitFixable || res.Reason != ReasonSpecTooLarge {
		t.Fatalf("goldfish > 256 KiB debia retornar 1 SPEC_TOO_LARGE, obtuvo %d %+v", code, res)
	}
	_ = os.WriteFile(gfPath, []byte(`{"report_path":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	if res, code := Validate(ValidateOptions{SpecPath: prep.SpecPath, Stage: "final", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("goldfish G4 debia retornar 3 G4_VIOLATION, obtuvo %d %+v", code, res)
	}
	_ = os.WriteFile(gfPath, []byte(`{broken`), 0o644)
	if res, code := Validate(ValidateOptions{SpecPath: prep.SpecPath, Stage: "final", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("goldfish JSON roto debia retornar 1 INVALID_JSON, obtuvo %d %+v", code, res)
	}
	_ = os.Remove(gfPath)
	// Direct validateGoldfishReviewFile cases
	_ = os.WriteFile(gfPath, []byte(`{"schema_version":"9.9","spec_sha256":"bad","lenses_run":[],"unresolved_blockers":1,"report_path":"../bad.md","reviewed_at":"not-a-date"}`), 0o644)
	if _, gfIss := validateGoldfishReviewFile(wiDir, strings.Repeat("a", 64), computeRequiredGoldfishLenses([]string{"db"}, true, 4)); len(gfIss) < 6 {
		t.Fatalf("validateGoldfishReviewFile debia reportar multiples issues, obtuvo %+v", gfIss)
	}

	// STALE_CONTEXT: repo_root distinto, risk_zones_sha256 cambiado, documento de vault modificado
	ctxObj, _, _ := loadSpecContextStrict(wiDir)
	if ext, code, _ := evaluateExternalDecisions(spec, ctxObj, filepath.Join(f.root, "other_repo"), f.vaultRoot, "", f.client); code != ExitOK || ext.ExternalReason != ReasonStaleContext {
		t.Fatalf("repoRoot distinto debia producir STALE_CONTEXT, obtuvo code=%d ext=%+v", code, ext)
	}
	origRZ := f.client.ctxResp.RiskZonesSHA256
	f.client.ctxResp.RiskZonesSHA256 = strings.Repeat("f", 64)
	if ext, code, _ := evaluateExternalDecisions(spec, ctxObj, f.repoRoot, f.vaultRoot, "", f.client); code != ExitOK || ext.ExternalReason != ReasonStaleContext {
		t.Fatalf("risk_zones_sha256 cambiado debia producir STALE_CONTEXT, obtuvo code=%d ext=%+v", code, ext)
	}
	f.client.ctxResp.RiskZonesSHA256 = origRZ

	// LANE_UPGRADE_REQUIRED con risk_zones_parse_error y ruta sin cobertura
	ftdSpec := spec
	ftdSpec.Lane = "FTD"
	f.client.ctxResp.RiskZonesParseError = "tabla rota"
	f.client.ctxResp.MatchedDocs = []bfcore.BrainDocMatch{}
	if ext, code, _ := evaluateExternalDecisions(ftdSpec, ctxObj, f.repoRoot, f.vaultRoot, "", f.client); code != ExitOK || ext.ExternalReason != ReasonLaneUpgradeRequired {
		t.Fatalf("FTD con risk_zones_parse_error/sin cobertura debia producir LANE_UPGRADE_REQUIRED, obtuvo code=%d ext=%+v", code, ext)
	}
	f.client.ctxResp.RiskZonesParseError = ""

	// 5. Compile edge cases: missing flags, missing spec, locked .compile.lock, Tier 1 in --repo-root, brain unavailable, ASDD goldfish tampering
	if _, code := Compile(CompileOptions{}); code != ExitUsage {
		t.Fatalf("Compile sin flags debia retornar 2")
	}
	if _, code := Compile(CompileOptions{SpecPath: filepath.Join(f.root, "missing.json"), TaskRoot: f.taskRoot, TaskID: "bf_t1"}); code != ExitUsage {
		t.Fatalf("Compile spec faltante debia retornar 2")
	}
	if _, code := Compile(CompileOptions{SpecPath: f.root, TaskRoot: f.taskRoot, TaskID: "bf_t1"}); code != ExitUsage {
		t.Fatalf("Compile spec dir debia retornar 2")
	}

	// Reset attempts so compile reaches [3]/[6]
	_, _ = Prepare(PrepareOptions{
		WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir,
		ResetAttempts: true, Reason: "Reset para pruebas de compile", BrainClient: f.client,
	})
	// Tier 1 secret in --repo-root -> Exit 3
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_t_sec",
		RepoRoot: "ghp_" + "1234567890abcdefghij123456", FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("Compile con secreto Tier 1 en --repo-root debia retornar 3 G4_VIOLATION, obtuvo %d %+v", code, res)
	}
	// .compile.lock ocupado -> Exit 6 LOCK_BUSY retryable=true
	_ = os.MkdirAll(f.taskRoot, 0o755)
	heldCompLock, _ := AcquireCompileLock(f.taskRoot)
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_t_lock",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitInfraOrRefusal || res.Reason != ReasonLockBusy || !res.Retryable {
		_ = heldCompLock.Unlock()
		t.Fatalf("Compile con .compile.lock ocupado debia retornar 6 LOCK_BUSY retryable=true, obtuvo %d %+v", code, res)
	}
	_ = heldCompLock.Unlock()

	// 6. VerifyRuntime extra branches: nil BrainClient helper, events.jsonl corrupt + G4, ErrBrainJournalActive, dataDir file
	if _, reason, err := CheckBrainPreflight(nil, f.vaultRoot, ""); err == nil || reason != ReasonBrainUnavailable {
		t.Fatalf("CheckBrainPreflight(nil) debia retornar BRAIN_UNAVAILABLE")
	}
	if err := CheckReparseSafety("", f.root); err != nil {
		t.Fatalf("CheckReparseSafety fallo en ruta limpia: %v", err)
	}
	fileAsDataDir := filepath.Join(f.root, "not_a_dir")
	_ = os.WriteFile(fileAsDataDir, []byte("x"), 0o644)
	f.client.pfErr = bfcore.ErrBrainJournalActive
	repFile, codeFile := VerifyRuntime(VerifyOptions{FactoryRoot: f.root, RepoRoot: f.root, VaultRoot: f.vaultRoot, DataDir: fileAsDataDir, BrainClient: f.client})
	if codeFile != ExitInfraOrRefusal || repFile.BrainOK {
		t.Fatalf("VerifyRuntime con dataDir archivo + journal activo debia fallar con 6: %d %+v", codeFile, repFile)
	}
	f.client.pfErr = nil

	// Events JSONL corrupt + G4 in VerifyRuntime
	evDir := filepath.Join(f.root, "ev_runtime")
	_ = os.MkdirAll(evDir, 0o755)
	_ = os.WriteFile(filepath.Join(evDir, "specwriter_events.jsonl"), []byte("{broken jsonl\n{\"detail\":\"\\u0067hp_1234567890abcdefghij123456\"}\n"), 0o644)
	repEv, codeEv := VerifyRuntime(VerifyOptions{FactoryRoot: f.root, RepoRoot: f.root, VaultRoot: f.vaultRoot, DataDir: evDir, BrainClient: f.client})
	if codeEv != ExitG4 || repEv.Reason != ReasonG4ViolationOnDisk || repEv.G4Issues == 0 {
		t.Fatalf("VerifyRuntime debia detectar G4 decodificado en specwriter_events.jsonl: %d %+v", codeEv, repEv)
	}

	// 7. Extra allowlist / bridge guard branches
	extraBadSteps := []VerifyStep{
		{ID: "x1", ExpectExit: 1, TimeoutSec: 0, Cwd: "../bad", Env: map[string]string{"CGO_ENABLED": "1"}, Argv: []string{"go"}},
		{ID: "x2", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"go", "test", "-count", "2", "./..."}},
		{ID: "x3", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"go", "test", "-run", "^bad", "./..."}},
		{ID: "x4", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"go", "test", "-timeout", "999s", "./..."}},
		{ID: "x5", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"go", "test", "github.com/remote/pkg"}},
		{ID: "x6", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"go", "vet", "-badflag", "./..."}},
		{ID: "x7", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"git", "rev-parse", "--bad", "NOTHEAD"}},
		{ID: "x8", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"git", "unknown"}},
		{ID: "x9", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"0_Brain/brain.exe", "query", "--limit", "abc", "--vault", "../bad"}},
		{ID: "x10", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"0_Brain/brain.exe", "context", "--files", "../bad.go", "posArg"}},
		{ID: "x11", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"2_Spec_Writer/specwriter.exe", "verify", "--factory-root", "../bad"}},
		{ID: "x12", ExpectExit: 0, TimeoutSec: 10, Argv: []string{"2_Spec_Writer/specwriter.exe", "compile"}},
	}
	for _, st := range extraBadSteps {
		var iss []ValidationIssue
		validateVerifyStepAllowlist(st, "/vs", func(p, c, m string) {
			iss = append(iss, ValidationIssue{Pointer: p, Code: c, Message: m})
		})
		if len(iss) == 0 {
			t.Fatalf("extraBadStep %s debia ser rechazado", st.ID)
		}
	}

	var proseIss []ValidationIssue
	addP := func(p, c, m string) { proseIss = append(proseIss, ValidationIssue{Pointer: p, Code: c, Message: m}) }
	checkIndexProseSafety("```go\n```", "/p", addP)
	checkIndexProseSafety("Máx. intentos por chunk: 99", "/p", addP)
	checkIndexProseSafety("**Puede modificar:** `GengiSWAP/`", "/p", addP)
	checkChunkProseSafety("## Injected", "/c", false, addP)
	checkChunkProseSafety("```cmd\n```", "/c", false, addP)
	checkChunkProseSafety("Verificacion: calc", "/c", false, addP)
	if len(proseIss) < 6 {
		t.Fatalf("guardas de prosa index/chunk esperaban >= 6 issues, obtuvo %d", len(proseIss))
	}
}

func TestCoverage_DeepValidationCompileAndHelpers(t *testing.T) {
	// 1. ValidateSpecAndRenderCheck comprehensive negative matrix
	f := newTestFixtureEnv(t, "FTD", "bug", "CRITICO", []string{"0_Brain/engine/a.go"}, nil, []bfcore.RiskHit{{
		ID: "RZ-001", Project: "black-factory", Level: "CRITICO", MatchedPaths: []string{"0_Brain/engine/a.go"}, MandatoryGate: "PR+AUTOR",
	}})
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)
	ctxObj, _, _ := loadSpecContextStrict(wiDir)

	// Spec con multiples defectos simultaneos para ejercitar todas las ramas de ValidateSpecAndRenderCheck
	fileB := "0_Brain/engine/b.go"
	_ = os.WriteFile(filepath.Join(f.repoRoot, filepath.FromSlash(fileB)), []byte("package demo\nfunc OtherFunc() {}\n"), 0o644)

	broken := Spec{
		SchemaVersion: "9.9",
		WorkItemRef: WorkItemRef{
			ID: "WI-000000000000", Revision: 99, SanitizedPayloadSHA256: "bad", TraceID: "bad", SourceWorkItemPath: "bad",
		},
		Project:           "other-proj",
		Lane:              "BAD",
		Template:          "bad_tmpl",
		TargetBranch:      "bad branch\nline2",
		Title:             "",
		G3DataMutation:    "",
		RequiresAutorHITL: false,
		StackAssumptions:  []string{},
		ReadOnlyPaths:     []string{"../bad"},
		GlobalForbidden:   []string{"0_Brain/engine/a.go"},
		DoD:               []string{},
		Narrative:         Narrative{Problem: "", RootCause: "", ProposedChange: ""},
		ObservedFiles: []ObservedFile{
			{Path: "0_Brain/engine/a.go", State: "present", SHA256: "short"},
			{Path: "0_Brain/engine/a.go", State: "absent", SHA256: "nonempty"},
			{Path: "0_Brain/engine/b.go", State: "weird", SHA256: ""},
			{Path: "0_Brain/`bad`.go", State: "present", SHA256: ""},
			{Path: "../bad.go", State: "present", SHA256: ""},
		},
		G5Citations: []G5Citation{
			{Path: "0_Brain/`bad`.go", Line: 0, SymbolAnchor: "a", Claim: ""},
			{Path: "../bad.go", Line: 1, SymbolAnchor: "SampleFunction", Claim: "c"},
			{Path: "0_Brain/engine/missing.go", Line: 1, SymbolAnchor: "SampleFunction", Claim: "c"},
			{Path: "0_Brain/engine", Line: 1, SymbolAnchor: "SampleFunction", Claim: "c"},
			{Path: "0_Brain/engine/a.go", Line: 1, SymbolAnchor: "NonExistentAnchorSymbol", Claim: "c"},
		},
		Decisions: []DecisionRecord{{
			Reason: "", DecidedBy: "", Rationale: "", Timestamp: "bad", LaneFrom: "BAD", LaneTo: "BAD",
			ApprovedPaths: []string{"../bad.go"}, ScopeSHA256: "bad",
		}},
		RiskMitigations: []RiskMitigation{
			{RiskID: "", Level: "INVALID", VerifyStepIDs: []string{"missing_vs"}, ReviewGates: []string{"missing_rg"}},
			{RiskID: "RZ-001", Level: "BAJO", VerifyStepIDs: []string{}, ReviewGates: []string{}, RequiresAutorHITL: false},
		},
		BrainFollowups: []BrainFollowup{{TargetHint: "", Summary: "", Trigger: ""}},
		WorkUnits: []WorkUnit{
			{
				ID: "BAD-ID", Title: "", Objective: "", Inputs: []string{""},
				CanModify: []string{"0_Brain/engine/a.go", fileB},
				ScopeExpansions: []ScopeExpansion{
					{Path: "0_Brain/`bad`.go", Rationale: ""},
					{Path: "../bad.go", Rationale: ""},
				},
				Forbidden:          []string{fileB},
				DependsOn:          []string{"BAD-ID", "nonexistent_dep"},
				AcceptanceCriteria: []AcceptanceCriterion{{ID: "", Kind: "bad", Description: ""}, {ID: "dup_ac", Kind: "command", Description: "ok", VerifyStepID: "nope"}, {ID: "dup_ac", Kind: "review_gate", Description: "ok", Executor: "NOBODY"}},
				VerifySteps:        []VerifyStep{{ID: "", Argv: []string{"go", "test", "./..."}, ExpectExit: 0, TimeoutSec: 10}, {ID: "dup_vs", Argv: []string{"go", "test", "./..."}, ExpectExit: 0, TimeoutSec: 10}, {ID: "dup_vs", Argv: []string{"go", "test", "./..."}, ExpectExit: 0, TimeoutSec: 10}},
				FailureProof:       FailureProof{Description: "", ExpectExit: 0},
			},
			{
				ID: "u_dup", Title: "T2", Objective: "Obj", Inputs: []string{"in"},
				CanModify:          []string{"0_Brain/engine/a.go"},
				AcceptanceCriteria: []AcceptanceCriterion{},
				VerifySteps:        []VerifyStep{},
				FailureProof:       FailureProof{Description: "fp", ExpectExit: 1},
			},
			{
				ID: "u_dup", Title: "T3", Objective: "Obj", Inputs: []string{"in"},
				CanModify:          []string{},
				AcceptanceCriteria: []AcceptanceCriterion{},
				VerifySteps:        []VerifyStep{},
				FailureProof:       FailureProof{Description: "fp", ExpectExit: 1},
			},
		},
	}
	rawBroken, _ := json.Marshal(broken)
	issues, _, _, err := ValidateSpecAndRenderCheck(rawBroken, broken, ctxObj, wiDir, f.repoRoot, f.factoryRoot, "draft", "t1", nil, false)
	if err != nil || len(issues) < 25 {
		t.Fatalf("spec rota debia generar >= 25 issues, obtuvo %d (err=%v)", len(issues), err)
	}

	// Shared can_modify between 2 independent valid units (no depends_on) + ASDD missing narrative + FTD->ASDD missing decisions + CRITICO missing review_gate
	f2 := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)
	prep2 := f2.prepare(t)
	wiDir2 := filepath.Dir(prep2.SpecPath)
	ctxObj2, _, _ := loadSpecContextStrict(wiDir2)
	s2 := buildValidSpecFromSkeleton(t, f2, prep2.SpecPath, []string{"0_Brain/engine/a.go"}, true)
	s2.Narrative.AlternativesConsidered = nil
	s2.Narrative.InterfacesAndContracts = ""
	s2.Narrative.EdgeCases = nil
	s2.Narrative.RollbackPlan = ""
	s2.Decisions = nil
	s2.RiskMitigations = []RiskMitigation{{RiskID: "RZ-001", Level: "CRITICO", VerifyStepIDs: []string{"vs_test"}, ReviewGates: []string{}, RequiresAutorHITL: false}}
	uB := s2.WorkUnits[0]
	uB.ID = "parallel_conflict"
	uB.VerifySteps[0].ID = "vs_b"
	uB.AcceptanceCriteria[0].ID = "ac_b"
	uB.AcceptanceCriteria[0].VerifyStepID = "vs_b"
	uB.AcceptanceCriteria[1].ID = "ac_qa_b"
	s2.WorkUnits = append(s2.WorkUnits, uB)
	rawS2, _ := json.Marshal(s2)
	if iss2, _, _, _ := ValidateSpecAndRenderCheck(rawS2, s2, ctxObj2, wiDir2, f2.repoRoot, f2.factoryRoot, "draft", "t2", []bfcore.RiskHit{{ID: "RZ-001", Project: "black-factory", Level: "CRITICO"}}, false); len(iss2) < 6 {
		t.Fatalf("esperaba >= 6 issues en ASDD/conflicto/CRITICO, obtuvo %d: %+v", len(iss2), iss2)
	}

	// 2. Compile full error matrix
	specClean := buildValidSpecFromSkeleton(t, f2, prep2.SpecPath, []string{"0_Brain/engine/a.go"}, false)
	writeSpecFile(t, prep2.SpecPath, specClean)

	// .wi.lock busy in Validate & Compile
	heldWI, _ := AcquireWorkItemLock(wiDir2)
	if res, code := Validate(ValidateOptions{SpecPath: prep2.SpecPath, Stage: "draft", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonLockBusy {
		_ = heldWI.Unlock()
		t.Fatalf("Validate con .wi.lock debia retornar 6 LOCK_BUSY: %d %+v", code, res)
	}
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_lock_wi", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonLockBusy {
		_ = heldWI.Unlock()
		t.Fatalf("Compile con .wi.lock debia retornar 6 LOCK_BUSY: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f2.wiPath, RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, RefreshContext: true, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonLockBusy {
		_ = heldWI.Unlock()
		t.Fatalf("Prepare con .wi.lock debia retornar 6 LOCK_BUSY: %d %+v", code, res)
	}
	_ = heldWI.Unlock()

	// Missing & corrupt attempts.json in Validate, Compile, Prepare --refresh-context
	attPath2 := filepath.Join(wiDir2, "attempts.json")
	origAtt, _ := os.ReadFile(attPath2)
	_ = os.Remove(attPath2)
	if res, code := Validate(ValidateOptions{SpecPath: prep2.SpecPath, Stage: "draft", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateMissing {
		t.Fatalf("Validate sin attempts.json debia retornar 6 ATTEMPTS_STATE_MISSING: %d %+v", code, res)
	}
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_no_att", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateMissing {
		t.Fatalf("Compile sin attempts.json debia retornar 6 ATTEMPTS_STATE_MISSING: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f2.wiPath, RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, RefreshContext: true, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateMissing {
		t.Fatalf("Prepare --refresh-context sin attempts.json debia retornar 6 ATTEMPTS_STATE_MISSING: %d %+v", code, res)
	}
	_ = os.WriteFile(attPath2, []byte("{corrupt"), 0o644)
	if res, code := Validate(ValidateOptions{SpecPath: prep2.SpecPath, Stage: "draft", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateCorrupt {
		t.Fatalf("Validate con attempts corrupto debia retornar 6 ATTEMPTS_STATE_CORRUPT: %d %+v", code, res)
	}
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_bad_att", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateCorrupt {
		t.Fatalf("Compile con attempts corrupto debia retornar 6 ATTEMPTS_STATE_CORRUPT: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f2.wiPath, RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, RefreshContext: true, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateCorrupt {
		t.Fatalf("Prepare --refresh-context con attempts corrupto debia retornar 6 ATTEMPTS_STATE_CORRUPT: %d %+v", code, res)
	}

	// Compile [5] blocked by draft_attempts>=3 and external_cycles>=3 unresolved
	_ = WriteJSONAtomicG4(attPath2, AttemptsState{SchemaVersion: "1.0", WIID: f2.wiID, Project: "black-factory", DraftAttempts: 3, LastExit: 1, LastReason: ReasonValidationFailed})
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_blk1", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitAttemptsBlocked || res.Reason != ReasonMaxValidateAttempts {
		t.Fatalf("Compile con draft_attempts=3 debia retornar 5: %d %+v", code, res)
	}
	_ = WriteJSONAtomicG4(attPath2, AttemptsState{SchemaVersion: "1.0", WIID: f2.wiID, Project: "black-factory", ExternalCycles: 3, LastExit: 4, LastReason: ReasonStaleContext})
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_blk2", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitAttemptsBlocked || res.Reason != ReasonMaxExternalCycles {
		t.Fatalf("Compile con external_cycles=3 sin pase debia retornar 5: %d %+v", code, res)
	}
	_ = os.WriteFile(attPath2, origAtt, 0o644)

	// Compile [3] SPEC_TOO_LARGE, G4_VIOLATION, INVALID_JSON
	origSpecBytes, _ := os.ReadFile(prep2.SpecPath)
	_ = os.WriteFile(prep2.SpecPath, make([]byte, MaxSpecBytes+5), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_big", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitFixable || res.Reason != ReasonSpecTooLarge {
		t.Fatalf("Compile SPEC_TOO_LARGE: %d %+v", code, res)
	}
	_ = os.WriteFile(prep2.SpecPath, []byte(`{"title":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_g4", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("Compile G4: %d %+v", code, res)
	}
	_ = os.WriteFile(prep2.SpecPath, []byte(`{bad`), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_badj", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("Compile INVALID_JSON: %d %+v", code, res)
	}
	_ = os.WriteFile(prep2.SpecPath, origSpecBytes, 0o644)

	// Compile [6] BrainPreflight journal active & unavailable + [4] STALE_CONTEXT without mutating attempts.json
	f2.client.pfErr = bfcore.ErrBrainJournalActive
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_jrn", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainJournalActive {
		t.Fatalf("Compile BRAIN_JOURNAL_ACTIVE: %d %+v", code, res)
	}
	f2.client.pfErr = nil
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_stale", RepoRoot: filepath.Join(f2.root, "other"), FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitExternalDecision || res.Reason != ReasonStaleContext {
		t.Fatalf("Compile STALE_CONTEXT: %d %+v", code, res)
	}

	// Compile ASDD goldfish > 256KiB, G4, invalid JSON, and tampered hash
	asddSpec := buildValidSpecFromSkeleton(t, f2, prep2.SpecPath, []string{"0_Brain/engine/a.go"}, true)
	writeSpecFile(t, prep2.SpecPath, asddSpec)
	gfPath2 := filepath.Join(wiDir2, "goldfish_review.json")
	_ = os.WriteFile(gfPath2, make([]byte, MaxGoldfishBytes+2), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_gf1", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitFixable || res.Reason != ReasonSpecTooLarge {
		t.Fatalf("Compile goldfish huge: %d %+v", code, res)
	}
	_ = os.WriteFile(gfPath2, []byte(`{"report_path":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_gf2", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("Compile goldfish G4: %d %+v", code, res)
	}
	_ = os.WriteFile(gfPath2, []byte(`{bad`), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_gf3", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("Compile goldfish bad JSON: %d %+v", code, res)
	}
	_ = os.Remove(gfPath2)

	// 3. Direct helper coverage: classifyBridgeChunkField, extractFieldLabelKey, verifyRenderedGengisParity, buffersPassG4, isSafeRelativeGoPkgArg, validateGoTestArgs, loadSpecContextStrict
	for _, lbl := range []string{"descripcion", "criterios de aceptacion", "verificacion", "prueba de fallo", "prohibido", "puede modificar", "solo lectura", "entradas", "desconocido"} {
		_ = classifyBridgeChunkField(lbl)
	}
	_, _ = extractFieldLabelKey("")
	_, _ = extractFieldLabelKey("*Nota:* valor")
	_, _ = extractFieldLabelKey(`C:\Windows\System32`)
	_, _ = extractFieldLabelKey("a: b")
	if !isSafeRelativeGoPkgArg(".") || !isSafeRelativeGoPkgArg("./...") || !isSafeRelativeGoPkgArg("./sub/...") || isSafeRelativeGoPkgArg("../up") {
		t.Fatalf("isSafeRelativeGoPkgArg inesperado")
	}
	for _, badGo := range [][]string{
		{"-v=maybe"}, {"-count"}, {"-run"}, {"-timeout"}, {"-timeout=abc"},
	} {
		var iss []ValidationIssue
		validateGoTestArgs(badGo, "/vs", func(p, c, m string) { iss = append(iss, ValidationIssue{Pointer: p, Code: c, Message: m}) })
		if len(iss) == 0 {
			t.Fatalf("validateGoTestArgs(%v) debia fallar", badGo)
		}
	}
	if iss := verifyRenderedGengisParity(RenderedTaskFiles{IndexMD: "empty"}); len(iss) < 2 {
		t.Fatalf("verifyRenderedGengisParity debia fallar en buffers vacios: %+v", iss)
	}
	if iss := verifyRenderedGengisParity(RenderedTaskFiles{
		IndexMD: "## Alcance\n- **Repositorio destino:** <TARGET_REPO>\n",
		Chunks:  []RenderedChunk{{Filename: "01_a.md", Content: "incomplete"}, {Filename: "01_a.md", Content: "incomplete"}},
	}); len(iss) < 2 {
		t.Fatalf("verifyRenderedGengisParity debia detectar duplicado y H2 faltantes: %+v", iss)
	}
	if buffersPassG4(RenderedTaskFiles{IndexMD: "ghp_" + "1234567890abcdefghij123456"}) {
		t.Fatalf("buffersPassG4 debia fallar en IndexMD con G4")
	}
	if buffersPassG4(RenderedTaskFiles{StatusMD: "ghp_" + "1234567890abcdefghij123456"}) {
		t.Fatalf("buffersPassG4 debia fallar en StatusMD con G4")
	}
	if buffersPassG4(RenderedTaskFiles{Chunks: []RenderedChunk{{Content: "ghp_" + "1234567890abcdefghij123456"}}}) {
		t.Fatalf("buffersPassG4 debia fallar en Chunk con G4")
	}
	if requiredStageForLane("ASDD") != "final" || requiredStageForLane("FTD") != "draft" {
		t.Fatalf("requiredStageForLane inesperado")
	}
	if _, code, _ := loadSpecContextStrict(t.TempDir()); code != ExitInfraOrRefusal {
		t.Fatalf("loadSpecContextStrict en dir vacio debia fallar con 6")
	}
	badCtxDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(badCtxDir, "spec_context.json"), []byte(`{"trace_id":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	if _, code, _ := loadSpecContextStrict(badCtxDir); code != ExitG4 {
		t.Fatalf("loadSpecContextStrict con G4 debia retornar 3, obtuvo %d", code)
	}
	_ = os.WriteFile(filepath.Join(badCtxDir, "spec_context.json"), []byte(`{broken`), 0o644)
	if _, code, _ := loadSpecContextStrict(badCtxDir); code != ExitInfraOrRefusal {
		t.Fatalf("loadSpecContextStrict corrupto debia retornar 6, obtuvo %d", code)
	}

	// 4. Cobertura adicional de rutas reales: Prepare combinado, Validate con goldfish dir / preflight / ctx corrupto,
	// Compile con 3 ciclos externos + live stale -> exit 5, VerifyRuntime con BrainUnavailable + events.jsonl valido.
	bigWI := filepath.Join(f2.root, "huge_wi.json")
	_ = os.WriteFile(bigWI, make([]byte, MaxSpecBytes+2), 0o644)
	if _, code := Prepare(PrepareOptions{WorkItemPath: bigWI}); code != ExitUsage {
		t.Fatalf("Prepare workitem > MaxSpecBytes debia retornar 2, obtuvo %d", code)
	}
	if base := relativeOrBasenamePath(filepath.Join(f2.root, "sub_factory"), bigWI); base != "huge_wi.json" {
		t.Fatalf("relativeOrBasenamePath fuera de factoryRoot debia usar basename, obtuvo %q", base)
	}
	if res, code := Prepare(PrepareOptions{
		WorkItemPath: f2.wiPath, RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir,
		ResetAttempts: true, RefreshContext: true, Reason: "Reset y refresh combinados", BrainClient: f2.client,
	}); code != ExitOK || !res.Reset || !res.Refreshed {
		t.Fatalf("Prepare --reset-attempts --refresh-context fallo: %d %+v", code, res)
	}

	_ = os.MkdirAll(gfPath2, 0o755)
	if res, code := Validate(ValidateOptions{SpecPath: prep2.SpecPath, Stage: "final", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("Validate con goldfish_review.json directorio debia retornar 1 INVALID_JSON: %d %+v", code, res)
	}
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_gf_dir", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("Compile con goldfish_review.json directorio debia retornar 1 INVALID_JSON: %d %+v", code, res)
	}
	_ = os.RemoveAll(gfPath2)

	f2.client.pfErr = bfcore.ErrBrainUnavailable
	if res, code := Validate(ValidateOptions{SpecPath: prep2.SpecPath, Stage: "draft", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainUnavailable {
		t.Fatalf("Validate con BrainUnavailable debia retornar 6: %d %+v", code, res)
	}
	repBU, codeBU := VerifyRuntime(VerifyOptions{FactoryRoot: f2.root, RepoRoot: f2.root, VaultRoot: f2.vaultRoot, DataDir: filepath.Join(f2.root, "absent_rt"), BrainClient: f2.client})
	if codeBU != ExitInfraOrRefusal || repBU.BrainOK {
		t.Fatalf("VerifyRuntime con BrainUnavailable debia fallar con 6: %d %+v", codeBU, repBU)
	}
	f2.client.pfErr = nil

	// Compile [4] con external_cycles=3 resuelto previamente pero nuevo drift -> Exit 5 MAX_EXTERNAL_CYCLES
	specClean2 := buildValidSpecFromSkeleton(t, f2, prep2.SpecPath, []string{"0_Brain/engine/a.go"}, false)
	writeSpecFile(t, prep2.SpecPath, specClean2)
	shaC2, _ := CanonicalSpecSHA256(specClean2)
	_ = WriteJSONAtomicG4(attPath2, AttemptsState{
		SchemaVersion: "1.0", WIID: f2.wiID, Project: "black-factory",
		ExternalCycles: 3, LastExit: 0, LastReason: ReasonOK, LastPassedSHA256: shaC2, LastPassedStage: "draft",
	})
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_4th_ext", RepoRoot: filepath.Join(f2.root, "other_repo"), FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitAttemptsBlocked || res.Reason != ReasonMaxExternalCycles {
		t.Fatalf("Compile con external_cycles=3 y nuevo drift debia retornar 5 MAX_EXTERNAL_CYCLES: %d %+v", code, res)
	}

	// VerifyRuntime con specwriter_events.jsonl limpio y linea en blanco
	cleanEvDir := filepath.Join(f2.root, "clean_ev_rt")
	_ = os.MkdirAll(cleanEvDir, 0o755)
	_ = os.WriteFile(filepath.Join(cleanEvDir, "specwriter_events.jsonl"), []byte("\n{\"event\":\"OK\",\"trace_id\":\"t1\"}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(cleanEvDir, ".events.lock"), []byte(""), 0o644)
	repCE, _ := VerifyRuntime(VerifyOptions{FactoryRoot: f2.root, RepoRoot: f2.root, VaultRoot: f2.vaultRoot, DataDir: cleanEvDir, BrainClient: f2.client})
	if repCE.ArtifactsChecked != 1 || repCE.G4Issues != 0 {
		t.Fatalf("VerifyRuntime sobre events.jsonl limpio debia contar 1 artefacto y 0 G4: %+v", repCE)
	}

	// Validate con spec_context.json corrupto
	origCtx2, _ := os.ReadFile(filepath.Join(wiDir2, "spec_context.json"))
	_ = os.WriteFile(filepath.Join(wiDir2, "spec_context.json"), []byte("{bad"), 0o644)
	if res, code := Validate(ValidateOptions{SpecPath: prep2.SpecPath, Stage: "draft", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonArtifactStateCorrupt {
		t.Fatalf("Validate con spec_context corrupto debia retornar 6 ARTIFACT_STATE_CORRUPT: %d %+v", code, res)
	}
	if res, code := Compile(CompileOptions{SpecPath: prep2.SpecPath, TaskRoot: f2.taskRoot, TaskID: "bf_bad_ctx", RepoRoot: f2.repoRoot, FactoryRoot: f2.factoryRoot, VaultRoot: f2.vaultRoot, DataDir: f2.dataDir, BrainClient: f2.client}); code != ExitInfraOrRefusal || res.Reason != ReasonArtifactStateCorrupt {
		t.Fatalf("Compile con spec_context corrupto debia retornar 6 ARTIFACT_STATE_CORRUPT: %d %+v", code, res)
	}
	_ = os.WriteFile(filepath.Join(wiDir2, "spec_context.json"), origCtx2, 0o644)
}

func TestGoldfishSecurityRegressions_BOM_Parens_ExpectOutput_TargetRepo_G4Key(t *testing.T) {
	f := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)
	ctxObj, _, _ := loadSpecContextStrict(wiDir)
	spec := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go"}, false)

	// Blocker 1: spec.json con prefijo UTF-8 BOM NO debe evadir [4] (STALE_CONTEXT / ESCALATION_REQUIRED) ni fallar en render ni corromper spec_sha256
	mod2 := "1_Intake/engine/bom_mod2.go"
	_ = os.MkdirAll(filepath.Dir(filepath.Join(f.repoRoot, filepath.FromSlash(mod2))), 0o755)
	_ = os.WriteFile(filepath.Join(f.repoRoot, filepath.FromSlash(mod2)), []byte("package intake\n"), 0o644)
	specEsc := buildValidSpecFromSkeleton(t, f, prep.SpecPath, []string{"0_Brain/engine/a.go", mod2}, false)
	rawEsc, _ := json.MarshalIndent(specEsc, "", "  ")
	bomEsc := append([]byte{0xef, 0xbb, 0xbf}, rawEsc...)
	_ = os.WriteFile(prep.SpecPath, bomEsc, 0o644)
	if res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "draft", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitExternalDecision || res.Reason != ReasonEscalationRequired {
		t.Fatalf("Blocker 1: BOM en spec.json evadio [4] ESCALATION_REQUIRED: code=%d res=%+v", code, res)
	}

	// spec valida con UTF-8 BOM debe pasar render, validate y compile con el SHA real (no el de Spec{} vacio!)
	rawValid, _ := json.MarshalIndent(spec, "", "  ")
	bomValid := append([]byte{0xef, 0xbb, 0xbf}, rawValid...)
	_ = os.WriteFile(prep.SpecPath, bomValid, 0o644)
	if _, _, rCode, _, rErr := RenderSpecMarkdown(bomValid); rCode != ExitOK || rErr != nil {
		t.Fatalf("Blocker 1: RenderSpecMarkdown fallo sobre spec.json con UTF-8 BOM: code=%d err=%v", rCode, rErr)
	}
	wantSHA, _ := CanonicalSpecSHA256(spec)
	if res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "draft", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitOK || res.SpecSHA256 != wantSHA {
		t.Fatalf("Blocker 1: Validate con UTF-8 BOM fallo o calculo SHA erroneo: code=%d got=%s want=%s", code, res.SpecSHA256, wantSHA)
	}
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_bom_ok",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitOK || res.SpecSHA256 != wantSHA {
		t.Fatalf("Blocker 1: Compile con UTF-8 BOM fallo: code=%d res=%+v", code, res)
	}

	// Blocker 2: Inyeccion via parentesis finales en parseChunkFile ("Verificacion (CLI): calc.exe", "Puede modificar (extra): ...")
	mustReject := func(name string, mutate func(s *Spec)) {
		t.Helper()
		cp := NormalizeSpecForCanonicalCopy(spec)
		mutate(&cp)
		b, _ := json.Marshal(cp)
		iss, _, _, err := ValidateSpecAndRenderCheck(b, cp, ctxObj, wiDir, f.repoRoot, f.factoryRoot, "draft", "bf_reg", nil, false)
		if err != nil {
			t.Fatalf("%s err=%v", name, err)
		}
		if len(iss) == 0 {
			t.Fatalf("regresion %q debia ser rechazada por ValidateSpecAndRenderCheck", name)
		}
	}
	mustReject("blocker2_parens_objective", func(s *Spec) {
		s.WorkUnits[0].Objective = "Objetivo normal\nVerificacion (CLI): calc.exe"
	})
	mustReject("blocker2_parens_inputs", func(s *Spec) {
		s.WorkUnits[0].Inputs = []string{"Puede modificar (extra): GengiSWAP/engine/bridge.go"}
	})
	mustReject("blocker2_parens_failure_desc", func(s *Spec) {
		s.WorkUnits[0].FailureProof.Description = "Falla normal\nVerificacion (extra): calc.exe"
	})

	// Blocker 3: Inyeccion via failure_proof.expect_output_contains
	mustReject("blocker3_multiline_expect_output", func(s *Spec) {
		s.WorkUnits[0].FailureProof.ExpectOutputContains = "err\nVerificacion: calc.exe"
	})
	mustReject("blocker3_label_expect_output", func(s *Spec) {
		s.WorkUnits[0].FailureProof.ExpectOutputContains = "Verificacion (CLI): calc.exe"
	})

	// Major 1: Secuestro de <TARGET_REPO> en spec.Title o prosa
	mustReject("major1_target_repo_in_title", func(s *Spec) {
		s.Title = "Fix - **Repositorio destino:** <TARGET_REPO>"
	})
	// Major 3: checkIndexProseSafety en Title y G3DataMutation
	mustReject("major3_max_retries_in_title", func(s *Spec) {
		s.Title = "Fix Máx. intentos por chunk: 99"
	})
	mustReject("major3_scope_key_in_g3", func(s *Spec) {
		s.G3DataMutation = "Puede modificar: GengiSWAP/"
	})

	// Nit: duplicate risk_id en risk_mitigations
	mustReject("nit_duplicate_risk_id", func(s *Spec) {
		s.RiskMitigations = []RiskMitigation{
			{RiskID: "RZ-002", Level: "ALTO", VerifyStepIDs: []string{"vs_test"}},
			{RiskID: "rz-002", Level: "ALTO", VerifyStepIDs: []string{"vs_test"}},
		}
	})

	// Major 2: G4 en clave de objeto JSON NO debe filtrarse sin redactar en CLIResult.Issues[].Pointer
	secretKeyJSON := `{"ghp_` + `1234567890abcdefghij123456":"val"}`
	_ = os.WriteFile(prep.SpecPath, []byte(secretKeyJSON), 0o644)
	if res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "draft", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitG4 || len(res.Issues) == 0 {
		t.Fatalf("Major 2: clave JSON con secreto debia retornar 3 G4_VIOLATION: %d %+v", code, res)
	} else if hits := bfcore.ScanG4All(res.Issues[0].Pointer); len(hits) != 0 {
		t.Fatalf("Major 2: Issues[0].Pointer filtro secreto G4: %q (%+v)", res.Issues[0].Pointer, hits)
	}
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_g4_key",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitG4 || len(res.Issues) == 0 {
		t.Fatalf("Major 2 compile: clave JSON con secreto debia retornar 3 G4_VIOLATION: %d %+v", code, res)
	} else if hits := bfcore.ScanG4All(res.Issues[0].Pointer); len(hits) != 0 {
		t.Fatalf("Major 2 compile: Issues[0].Pointer filtro secreto G4: %q (%+v)", res.Issues[0].Pointer, hits)
	}

	// Nit: 0-byte goldfish_review.json en validate --stage final debe retornar 1 INVALID_JSON en [3]
	writeSpecFile(t, prep.SpecPath, spec)
	_ = os.WriteFile(filepath.Join(wiDir, "goldfish_review.json"), []byte{}, 0o644)
	if res, code := Validate(ValidateOptions{
		SpecPath: prep.SpecPath, Stage: "final", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot,
		VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("Nit: 0-byte goldfish_review.json debia retornar 1 INVALID_JSON en [3], obtuvo %d %+v", code, res)
	}
	_ = os.Remove(filepath.Join(wiDir, "goldfish_review.json"))

	// Nit: archivo regular en <destDir> debe retornar 6 TASK_DIR_EXISTS (no IO_ERROR por ENOTDIR)
	_ = os.MkdirAll(f.taskRoot, 0o755)
	_ = os.WriteFile(filepath.Join(f.taskRoot, "bf_file_exists"), []byte("occupied"), 0o644)
	if res, code := Compile(CompileOptions{
		SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_file_exists",
		RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client,
	}); code != ExitInfraOrRefusal || res.Reason != ReasonTaskDirExists || res.Retryable {
		t.Fatalf("Nit: archivo regular en destDir debia retornar 6 TASK_DIR_EXISTS retryable=false, obtuvo %d %+v", code, res)
	}
}

func TestPrepareVerifyCompileEdgePaths(t *testing.T) {
	f := newTestFixtureEnv(t, "FTD", "bug", "NONE", []string{"0_Brain/engine/a.go"}, nil, nil)

	// 1. Prepare: validaciones de uso, G4 y entradas corruptas
	if res, code := Prepare(PrepareOptions{RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Prepare sin --workitem debia retornar 2 INVALID_USAGE: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, ResetAttempts: true, Reason: "corto", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Prepare --reset-attempts con --reason corto debia retornar 2: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, ResetAttempts: true, Reason: "motivo ghp_" + "1234567890abcdefghij123456", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("Prepare --reset-attempts con secreto G4 en --reason debia retornar 3: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: filepath.Join(f.root, "missing_wi.json"), RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Prepare con --workitem inexistente debia retornar 2: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.root, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Prepare con directorio como --workitem debia retornar 2: %d %+v", code, res)
	}
	badG4WI := filepath.Join(f.root, "bad_g4_wi.json")
	_ = os.WriteFile(badG4WI, []byte(`{"id":"WI-1","token":"ghp_`+`1234567890abcdefghij123456"}`), 0o644)
	if res, code := Prepare(PrepareOptions{WorkItemPath: badG4WI, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("Prepare con secreto G4 en WorkItem debia retornar 3: %d %+v", code, res)
	}
	badJSONWI := filepath.Join(f.root, "bad_json_wi.json")
	_ = os.WriteFile(badJSONWI, []byte(`{not-json`), 0o644)
	if res, code := Prepare(PrepareOptions{WorkItemPath: badJSONWI, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Prepare con JSON invalido en WorkItem debia retornar 2: %d %+v", code, res)
	}
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RefreshContext: true, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Prepare --refresh-context sin spec previa debia retornar 2: %d %+v", code, res)
	}

	// 2. Prepare: ErrBrainJournalActive en Context() (nuevo WorkItem y --refresh-context) debe clasificar BRAIN_JOURNAL_ACTIVE (retryable=true)
	f.client.ctxErr = fmt.Errorf("journal activo: %w (%w)", bfcore.ErrBrainJournalActive, bfcore.ErrBrainUnavailable)
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainJournalActive || !res.Retryable {
		t.Fatalf("Prepare nuevo con ErrBrainJournalActive debia retornar 6 BRAIN_JOURNAL_ACTIVE retryable=true: %d %+v", code, res)
	}
	f.client.ctxErr = nil
	prep := f.prepare(t)
	wiDir := filepath.Dir(prep.SpecPath)

	f.client.ctxErr = fmt.Errorf("journal activo: %w (%w)", bfcore.ErrBrainJournalActive, bfcore.ErrBrainUnavailable)
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RefreshContext: true, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainJournalActive || !res.Retryable {
		t.Fatalf("Prepare --refresh-context con ErrBrainJournalActive debia retornar 6 BRAIN_JOURNAL_ACTIVE retryable=true: %d %+v", code, res)
	}
	f.client.ctxErr = nil

	// Prepare --refresh-context con attempts.json ausente o corrupto
	origAtt, _ := os.ReadFile(filepath.Join(wiDir, "attempts.json"))
	_ = os.Remove(filepath.Join(wiDir, "attempts.json"))
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RefreshContext: true, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateMissing {
		t.Fatalf("Prepare --refresh-context sin attempts.json debia retornar 6 ATTEMPTS_STATE_MISSING: %d %+v", code, res)
	}
	_ = os.WriteFile(filepath.Join(wiDir, "attempts.json"), []byte("{corrupt"), 0o644)
	if res, code := Prepare(PrepareOptions{WorkItemPath: f.wiPath, RefreshContext: true, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || res.Reason != ReasonAttemptsStateCorrupt {
		t.Fatalf("Prepare --refresh-context con attempts.json corrupto debia retornar 6 ATTEMPTS_STATE_CORRUPT: %d %+v", code, res)
	}
	_ = os.WriteFile(filepath.Join(wiDir, "attempts.json"), origAtt, 0o644)

	// 3. Compile: argumentos invalidos y spec JSON invalido
	if res, code := Compile(CompileOptions{SpecPath: prep.SpecPath, TaskRoot: "", TaskID: "bf_ok", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("Compile sin --task-root debia retornar 2: %d %+v", code, res)
	}
	for _, badID := range []string{"", "../escape", "bad space", "UPPER_BAD"} {
		if res, code := Compile(CompileOptions{SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: badID, RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
			t.Fatalf("Compile con task-id invalido %q debia retornar 2: %d %+v", badID, code, res)
		}
	}
	origSpec, _ := os.ReadFile(prep.SpecPath)
	_ = os.WriteFile(prep.SpecPath, []byte("{not-json"), 0o644)
	if res, code := Compile(CompileOptions{SpecPath: prep.SpecPath, TaskRoot: f.taskRoot, TaskID: "bf_bad_json", RepoRoot: f.repoRoot, FactoryRoot: f.factoryRoot, VaultRoot: f.vaultRoot, DataDir: f.dataDir, BrainClient: f.client}); code != ExitFixable || res.Reason != ReasonInvalidJSON {
		t.Fatalf("Compile con spec JSON invalido debia retornar 1 INVALID_JSON: %d %+v", code, res)
	}
	_ = os.WriteFile(prep.SpecPath, origSpec, 0o644)

	// 4. VerifyRuntime: dataDir archivo regular, JSONL corrupto/G4, JSON corrupto, BrainJournalActive, BrainUnavailable, GengisDrift
	fileDataDir := filepath.Join(f.root, "not_a_dir")
	_ = os.WriteFile(fileDataDir, []byte("x"), 0o644)
	if rep, code := VerifyRuntime(VerifyOptions{FactoryRoot: f.factoryRoot, RepoRoot: f.repoRoot, VaultRoot: f.vaultRoot, DataDir: fileDataDir, BrainClient: f.client}); code != ExitInfraOrRefusal || len(rep.Issues) == 0 {
		t.Fatalf("VerifyRuntime con dataDir archivo debia reportar issues: %d %+v", code, rep)
	}

	evDir := filepath.Join(f.root, "verify_rt")
	_ = os.MkdirAll(evDir, 0o755)
	_ = os.WriteFile(filepath.Join(evDir, "specwriter_events.jsonl"), []byte("{corrupt-line\n{\"msg\":\"\\u0067hp_1234567890abcdefghij123456\"}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(evDir, "broken.json"), []byte("{broken"), 0o644)
	_ = os.WriteFile(filepath.Join(evDir, "attempts.json"), []byte("{broken"), 0o644)
	repG4, codeG4 := VerifyRuntime(VerifyOptions{FactoryRoot: f.factoryRoot, RepoRoot: f.repoRoot, VaultRoot: f.vaultRoot, DataDir: evDir, BrainClient: f.client})
	if codeG4 != ExitG4 || repG4.Reason != ReasonG4ViolationOnDisk || repG4.G4Issues == 0 || repG4.ArtifactsChecked < 3 {
		t.Fatalf("VerifyRuntime con G4 en JSONL debia priorizar 3 G4_VIOLATION_ON_DISK: code=%d rep=%+v", codeG4, repG4)
	}

	// Preflight BrainJournalActive y BrainUnavailable en VerifyRuntime
	cleanEmpty := filepath.Join(f.root, "verify_clean_empty")
	f.client.pfErr = fmt.Errorf("wal: %w (%w)", bfcore.ErrBrainJournalActive, bfcore.ErrBrainUnavailable)
	if rep, code := VerifyRuntime(VerifyOptions{FactoryRoot: f.factoryRoot, RepoRoot: f.repoRoot, VaultRoot: f.vaultRoot, DataDir: cleanEmpty, BrainClient: f.client}); code != ExitInfraOrRefusal || rep.BrainOK {
		t.Fatalf("VerifyRuntime con BrainJournalActive debia marcar BrainOK=false: %d %+v", code, rep)
	}
	f.client.pfErr = bfcore.ErrBrainUnavailable
	if rep, code := VerifyRuntime(VerifyOptions{FactoryRoot: f.factoryRoot, RepoRoot: f.repoRoot, VaultRoot: f.vaultRoot, DataDir: cleanEmpty, BrainClient: f.client}); code != ExitInfraOrRefusal || rep.BrainOK {
		t.Fatalf("VerifyRuntime con BrainUnavailable debia marcar BrainOK=false: %d %+v", code, rep)
	}
	f.client.pfErr = nil

	// CheckGengisEngineHashes drift sobre directorio temporal con bridge.go alterado
	_ = os.MkdirAll(filepath.Join(f.factoryRoot, "GengiSWAP", "engine"), 0o755)
	_ = os.WriteFile(filepath.Join(f.factoryRoot, "GengiSWAP", "engine", "bridge.go"), []byte("package engine // drifted\n"), 0o644)
	_ = os.WriteFile(filepath.Join(f.factoryRoot, "GengiSWAP", "engine", "ring.go"), []byte("package engine // drifted\n"), 0o644)
	if rep, code := VerifyRuntime(VerifyOptions{FactoryRoot: f.factoryRoot, RepoRoot: f.repoRoot, VaultRoot: f.vaultRoot, DataDir: cleanEmpty, BrainClient: f.client}); code != ExitInfraOrRefusal || rep.GengisHashesOK {
		t.Fatalf("VerifyRuntime con GengiSWAP alterado debia marcar GengisHashesOK=false: %d %+v", code, rep)
	}
}
