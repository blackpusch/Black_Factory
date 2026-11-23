package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"blackfactory/intake"
	"blackfactory/pkg/bfcore"
)

type mockBrainClient struct {
	preflightErr error
	contextErr   error
	queryErr     error
	contextResp  intake.BrainContextResponse
	queryDocs    []intake.BrainDocMatch
}

func (m *mockBrainClient) Preflight(vaultRoot, dbPath string) (intake.BrainPreflight, error) {
	if m.preflightErr != nil {
		return intake.BrainPreflight{}, m.preflightErr
	}
	return intake.BrainPreflight{
		VaultRoot:    vaultRoot,
		DBPath:       dbPath,
		BrainDBMTime: "2026-11-22T12:00:00Z",
	}, nil
}

func (m *mockBrainClient) Context(vaultRoot, dbPath, project string, files []string) (intake.BrainContextResponse, intake.BrainInvocation, error) {
	if m.contextErr != nil {
		return intake.BrainContextResponse{}, intake.BrainInvocation{Command: "brain context", ExitCode: 1}, m.contextErr
	}
	return m.contextResp, intake.BrainInvocation{
		Command:     "0_Brain/brain.exe context --json",
		ExitCode:    0,
		DurationMs:  12,
		ResultCount: len(m.contextResp.MatchedDocs),
	}, nil
}

func (m *mockBrainClient) Query(vaultRoot, dbPath string, opts intake.BrainQueryOpts) ([]intake.BrainDocMatch, intake.BrainInvocation, error) {
	if m.queryErr != nil {
		return nil, intake.BrainInvocation{Command: "brain query", ExitCode: 1}, m.queryErr
	}
	return m.queryDocs, intake.BrainInvocation{
		Command:     "0_Brain/brain.exe query --json",
		ExitCode:    0,
		DurationMs:  9,
		ResultCount: len(m.queryDocs),
	}, nil
}

func TestCLI_E2E_FullLifecycle_AllExitCodes(t *testing.T) {
	tmpRoot := t.TempDir()
	dataDir := filepath.Join(tmpRoot, "runtime")
	vaultDir := filepath.Join(tmpRoot, "vault")
	_ = os.MkdirAll(filepath.Join(vaultDir, "projects"), 0o755)
	_ = os.WriteFile(filepath.Join(vaultDir, "projects", "black-factory.md"), []byte("---\nid: p-bf\ntitle: BF\ntype: project\nproject: black-factory\nstatus: active\n---\n# BF\n"), 0o644)

	client := &mockBrainClient{
		contextResp: intake.BrainContextResponse{
			HasActiveRiskZones: true,
			RiskZonesSHA256:    strings.Repeat("e", 64),
			RiskHits: []intake.RiskHit{{
				ID:            "RZ-002",
				Project:       "black-factory",
				Level:         "ALTO",
				MatchedPaths:  []string{"0_Brain/engine/validator.go"},
				MandatoryGate: "go vet + go test",
			}},
			MatchedDocs: []intake.BrainDocMatch{
				{ID: "brain-constitution", RelPath: "constitution.md", Type: "constitution", Status: "active", Transversal: true},
				{ID: "brain-risk-zones", RelPath: "wiki/risk_zones.md", Type: "risk_zones", Status: "active", Transversal: true},
				{ID: "brain-engine-g4", RelPath: "wiki/brain_engine_g4.md", Type: "wiki", Status: "active", CoversPaths: []string{"0_Brain/engine/"}, MatchedPaths: []string{"0_Brain/engine/validator.go"}},
			},
		},
		queryDocs: []intake.BrainDocMatch{
			{ID: "p-bf", RelPath: "projects/black-factory.md", Type: "project", Project: "black-factory", Status: "active"},
		},
	}

	execCLI := func(stdin string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		var fullArgs []string
		if len(args) > 0 {
			switch args[0] {
			case "list", "show":
				fullArgs = append(args, "--data-dir", dataDir, "--repo-root", tmpRoot)
			case "ingest", "resolve", "verify":
				fullArgs = append(args, "--data-dir", dataDir, "--vault", vaultDir, "--repo-root", tmpRoot)
			default:
				fullArgs = args
			}
		}
		code := run(fullArgs, strings.NewReader(stdin), &out, &errOut, client)
		return code, out.String(), errOut.String()
	}

	// 0. Help & empty args
	if c, _, _ := execCLI(""); c != ExitUsageOrError {
		t.Fatalf("sin argumentos debia retornar ExitUsageOrError (1), obtuvo %d", c)
	}
	if c, _, _ := execCLI("", "--help"); c != ExitOK {
		t.Fatalf("--help debia retornar ExitOK (0), obtuvo %d", c)
	}
	// List on empty store (text and JSON)
	if c, out, _ := execCLI("", "list"); c != ExitOK || !strings.Contains(out, "Sin WorkItems") {
		t.Fatalf("list texto vacio fallo: %d %s", c, out)
	}
	if c, out, _ := execCLI("", "list", "--json"); c != ExitOK || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list --json vacio debia retornar [], obtuvo: %q", out)
	}

	// 1. Exit Code 3 (ExitRejectedG4): JSON-escaped Tier 1 token -> 0 WorkItem files, REJECTED_G4 event
	secretStdin := `{"project":"black-factory","kind":"bug","source":"cli","source_ref":"SEC-1","title":"Leak","description":" token \u0067hp_1234567890abcdefghij123456","affected_paths":["0_Brain/engine/validator.go"]}`
	code, _, errOut := execCLI(secretStdin, "ingest", "--input", "-", "--json")
	if code != ExitRejectedG4 {
		t.Fatalf("esperaba ExitRejectedG4 (3), obtuvo %d (errOut=%s)", code, errOut)
	}
	// Text mode Tier 1 rejection via --strict-g4 on Tier 2 email
	code, _, errOut = execCLI("", "ingest", "--project", "black-factory", "--title", "contact admin"+"@example.com", "--strict-g4")
	if code != ExitRejectedG4 || !strings.Contains(errOut, "REJECTED_G4") {
		t.Fatalf("esperaba ExitRejectedG4 (3) en modo texto con --strict-g4, obtuvo %d (%s)", code, errOut)
	}
	if entries, _ := os.ReadDir(filepath.Join(dataDir, "workitems")); len(entries) != 0 {
		t.Fatalf("Exit 3 no debia escribir archivos en workitems/")
	}
	if entries, _ := os.ReadDir(filepath.Join(dataDir, "quarantine")); len(entries) != 0 {
		t.Fatalf("Exit 3 no debia escribir archivos en quarantine/")
	}

	// --dry-run on ready item writes 0 files
	code, out, errOut := execCLI("", "ingest",
		"--project", "black-factory",
		"--kind", "bug",
		"--source", "cli",
		"--source-ref", "DRY-1",
		"--title", "Dry run check",
		"--description", "Should not write files",
		"--paths", "0_Brain/engine/validator.go",
		"--dry-run",
	)
	if code != ExitOK || !strings.Contains(out, "triado") {
		t.Fatalf("ingest --dry-run fallo: code=%d out=%s err=%s", code, out, errOut)
	}
	if entries, _ := os.ReadDir(filepath.Join(dataDir, "workitems")); len(entries) != 0 {
		t.Fatalf("--dry-run no debia escribir archivos en workitems/")
	}

	// 2. Exit Code 2 (ExitQuarantined): Solicitud sin affected_paths ni source_ref -> quarantine/WI-<payloadSHA[:12]>.json
	code, out, errOut = execCLI("", "ingest",
		"--project", "black-factory",
		"--kind", "bug",
		"--source", "cli",
		"--title", "Vague bug report",
		"--description", `Crash in C:\`+`Users\jdoe\local.go`,
		"--json",
	)
	if code != ExitQuarantined {
		t.Fatalf("esperaba ExitQuarantined (2), obtuvo %d (out=%s err=%s)", code, out, errOut)
	}
	var quarWI intake.WorkItem
	if err := json.Unmarshal([]byte(out), &quarWI); err != nil {
		t.Fatalf("Unmarshal quarantine WI: %v\n%s", err, out)
	}
	if quarWI.Status != "needs_clarification" || quarWI.DedupeKey != "" || !strings.HasPrefix(quarWI.ID, "WI-") || quarWI.QuarantineInfo == nil {
		t.Fatalf("quarantine WorkItem invalido: %+v", quarWI)
	}

	// Quarantine por proyecto no registrado (Rule 6: UNREGISTERED_PROJECT)
	code, outUnreg, _ := execCLI("", "ingest",
		"--project", "proyecto-fantasma",
		"--kind", "bug",
		"--source", "cli",
		"--source-ref", "GHOST-1",
		"--title", "Unregistered project test",
		"--description", "Valid description",
		"--path", "0_Brain/engine/validator.go",
	)
	if code != ExitQuarantined || !strings.Contains(outUnreg, "UNREGISTERED_PROJECT") {
		t.Fatalf("esperaba ExitQuarantined (2) por UNREGISTERED_PROJECT, obtuvo %d (%s)", code, outUnreg)
	}

	// 3. Exit Code 0 (ExitOK) via resolve: promueve desde quarantine/ a workitems/ y elimina el archivo de quarantine/
	code, out, errOut = execCLI("", "resolve",
		quarWI.ID, // positional ID
		"--source-ref", "BUG-104",
		"--paths", "0_Brain/engine/validator.go",
		"--note", "Scope confirmado por AUTOR",
		"--json",
	)
	if code != ExitOK {
		t.Fatalf("resolve esperaba ExitOK (0), obtuvo %d (out=%s err=%s)", code, out, errOut)
	}
	var promotedWI intake.WorkItem
	if err := json.Unmarshal([]byte(out), &promotedWI); err != nil {
		t.Fatalf("Unmarshal promoted WI: %v", err)
	}
	if promotedWI.Status != "ready" || promotedWI.Lane != "FTD" || promotedWI.Revision != 2 {
		t.Fatalf("promoted WI inesperado: %+v", promotedWI)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "quarantine", quarWI.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("archivo antiguo en quarantine/%s.json debia eliminarse tras promocion", quarWI.ID)
	}

	// Idempotent hit on re-ingesting identical ready payload (Exit 0, revision stays 2)
	code, out, _ = execCLI("", "ingest",
		"--project", "black-factory",
		"--kind", "bug",
		"--source", "cli",
		"--source-ref", "BUG-104",
		"--title", "Vague bug report",
		"--description", `Crash in C:\`+`Users\jdoe\local.go`,
		"--note", "Scope confirmado por AUTOR",
		"--path", "0_Brain/engine/validator.go",
		"--json",
	)
	if code != ExitOK {
		t.Fatalf("re-ingest idempotente esperaba ExitOK (0), obtuvo %d", code)
	}

	// 4. Exit Code 5 (ExitDedupeConflict): mismo project+source+source_ref con distinto title sin --update
	code, _, errOut = execCLI("", "ingest",
		"--project", "black-factory",
		"--kind", "bug",
		"--source", "cli",
		"--source-ref", "BUG-104",
		"--title", "Different title causing dedupe conflict",
		"--description", "Updated description",
		"--path", "0_Brain/engine/validator.go",
		"--json",
	)
	if code != ExitDedupeConflict {
		t.Fatalf("esperaba ExitDedupeConflict (5), obtuvo %d (err=%s)", code, errOut)
	}

	// Con --update debe tener exito (Exit 0) e incrementar revision a 3
	code, out, errOut = execCLI("", "ingest",
		"--project", "black-factory",
		"--kind", "bug",
		"--source", "cli",
		"--source-ref", "BUG-104",
		"--title", "Different title causing dedupe conflict",
		"--description", "Updated description",
		"--path", "0_Brain/engine/validator.go",
		"--impact-flag", "security",
		"--update",
		"--json",
	)
	if code != ExitOK {
		t.Fatalf("ingest --update esperaba ExitOK (0), obtuvo %d (%s)", code, errOut)
	}
	var updatedWI intake.WorkItem
	_ = json.Unmarshal([]byte(out), &updatedWI)
	if updatedWI.Revision != 3 || updatedWI.Lane != "ASDD" {
		t.Fatalf("esperaba Revision=3 y Lane=ASDD tras --update con impact-flag, obtuvo %+v", updatedWI)
	}

	// 5. list, show (--format md, --format json y texto) y verify
	code, out, _ = execCLI("", "list")
	if code != ExitOK || !strings.Contains(out, promotedWI.ID) {
		t.Fatalf("list texto fallo: code=%d out=%s", code, out)
	}
	code, out, _ = execCLI("", "show", promotedWI.ID, "--format", "md")
	if code != ExitOK || !strings.Contains(out, "AUTO-GENERATED DERIVED VIEW") || strings.Contains(out, "```mermaid") {
		t.Fatalf("show --format md inesperado: code=%d out=%s", code, out)
	}
	code, out, _ = execCLI("", "show", "--id", promotedWI.ID, "--json")
	if code != ExitOK || !strings.Contains(out, `"schema_version": "1.0"`) {
		t.Fatalf("show --json inesperado: code=%d out=%s", code, out)
	}

	code, out, errOut = execCLI("", "verify")
	if code != ExitOK || !strings.Contains(out, "Verify OK=true") {
		t.Fatalf("verify texto esperaba ExitOK (0), obtuvo %d (out=%s err=%s)", code, out, errOut)
	}

	// 6. Exit Code 4 (ExitBrainOrLockUnavailable): cuando .intake.lock esta ocupado o cuando 0_Brain falla en Preflight/Context/Query
	// Probar resolve que aun permanece en cuarentena (Exit 2) y resolve con secreto Tier 1 (Exit 3)
	code, outStillQuar, _ := execCLI("", "ingest", "--project", "black-factory", "--kind", "bug", "--source", "cli", "--title", "Still quar", "--description", "No path", "--json")
	if code != ExitQuarantined {
		t.Fatalf("esperaba ExitQuarantined (2), obtuvo %d", code)
	}
	var stillQuarWI intake.WorkItem
	_ = json.Unmarshal([]byte(outStillQuar), &stillQuarWI)

	codeStill, outStill2, _ := execCLI("", "resolve", "--id", stillQuarWI.ID, "--note", "Added note but still missing path and source_ref", "--json")
	if codeStill != ExitQuarantined {
		t.Fatalf("resolve aun incompleto esperaba ExitQuarantined (2), obtuvo %d", codeStill)
	}
	var stillQuarWI2 intake.WorkItem
	_ = json.Unmarshal([]byte(outStill2), &stillQuarWI2)

	if c, _, _ := execCLI("", "resolve", "--id", stillQuarWI2.ID, "--note", "ghp_"+"1234567890abcdefghij123456"); c != ExitRejectedG4 {
		t.Fatalf("resolve con secreto Tier 1 esperaba ExitRejectedG4 (3), obtuvo %d", c)
	}
	// Resolve hacia un dedupe_key que ya existe en workitems/ (BUG-104) sin --update -> Exit 5
	if c, _, _ := execCLI("", "resolve", "--id", stillQuarWI2.ID, "--source-ref", "BUG-104", "--path", "0_Brain/engine/validator.go"); c != ExitDedupeConflict {
		t.Fatalf("resolve con colision hacia BUG-104 existente sin --update esperaba ExitDedupeConflict (5), obtuvo %d", c)
	}

	// Probar ingest desde un archivo JSON real (--input <file.json>)
	jsonInputPath := filepath.Join(tmpRoot, "req.json")
	_ = os.WriteFile(jsonInputPath, []byte(`{"project":"black-factory","kind":"feature","source":"cli","source_ref":"FEAT-01","title":"New module","description":"Spec needed","affected_paths":["0_Brain/engine/validator.go"],"impact_flags":["contract"]}`), 0o644)
	if c, outFeat, _ := execCLI("", "ingest", "--input", jsonInputPath); c != ExitOK || !strings.Contains(outFeat, "lane=ASDD") {
		t.Fatalf("ingest desde archivo JSON fallo: code=%d out=%s", c, outFeat)
	}
	// --input sobre archivo inexistente -> Exit 1 (JSON error envelope)
	if c, _, _ := execCLI("", "ingest", "--input", filepath.Join(tmpRoot, "missing.json"), "--json"); c != ExitUsageOrError {
		t.Fatalf("ingest con archivo inexistente esperaba ExitUsageOrError (1), obtuvo %d", c)
	}

	// Probar resolveCommonDirs con variables de entorno BF_INTAKE_DIR, BRAIN_VAULT y BRAIN_BIN
	t.Setenv("BF_INTAKE_DIR", dataDir)
	t.Setenv("BRAIN_VAULT", vaultDir)
	t.Setenv("BRAIN_BIN", "custom_brain.exe")
	pOpts := resolveCommonDirs("", "", "", "", 5, false, false, false, false)
	if pOpts.dataDir != dataDir || pOpts.vaultRoot != vaultDir || pOpts.brainBin != "custom_brain.exe" {
		t.Fatalf("resolveCommonDirs con env inesperado: %+v", pOpts)
	}
	t.Setenv("BF_INTAKE_DIR", "")
	t.Setenv("BRAIN_VAULT", "")
	t.Setenv("BRAIN_BIN", "")
	pOptsDef := resolveCommonDirs(tmpRoot, "", "", "", 5, false, false, false, false)
	if !strings.HasSuffix(filepath.ToSlash(pOptsDef.dataDir), "1_Intake/runtime") {
		t.Fatalf("resolveCommonDirs default con repoRoot inesperado: %+v", pOptsDef)
	}

	lockFile := filepath.Join(dataDir, ".intake.lock")
	heldLock, lockErr := bfcore.AcquireFileLock(lockFile, 0)
	if lockErr != nil {
		t.Fatalf("AcquireFileLock test: %v", lockErr)
	}
	code, _, _ = execCLI("", "verify", "--json")
	_ = heldLock.Unlock()
	if code != ExitBrainOrLockUnavailable {
		t.Fatalf("verify con .intake.lock ocupado esperaba ExitBrainOrLockUnavailable (4), obtuvo %d", code)
	}

	client.contextErr = intake.ErrBrainUnavailable
	if c, _, _ := execCLI("", "ingest", "--project", "black-factory", "--kind", "bug", "--source", "cli", "--source-ref", "B-CTX", "--title", "T", "--description", "D", "--path", "0_Brain/engine/validator.go"); c != ExitBrainOrLockUnavailable {
		t.Fatalf("ingest con Context error esperaba 4, obtuvo %d", c)
	}
	client.contextErr = nil

	client.queryErr = intake.ErrBrainUnavailable
	if c, _, _ := execCLI("", "ingest", "--project", "black-factory", "--kind", "bug", "--source", "cli", "--source-ref", "B-QRY", "--title", "Validator G4", "--description", "ScanG4All", "--path", "0_Brain/engine/validator.go"); c != ExitBrainOrLockUnavailable {
		t.Fatalf("ingest con Query error esperaba 4, obtuvo %d", c)
	}
	client.queryErr = nil

	client.preflightErr = intake.ErrBrainUnavailable
	if c, _, _ := execCLI("", "ingest", "--project", "black-factory", "--kind", "bug", "--source", "cli", "--source-ref", "BUG-999", "--title", "T", "--description", "D", "--path", "0_Brain/engine/validator.go", "--json"); c != ExitBrainOrLockUnavailable {
		t.Fatalf("esperaba ExitBrainOrLockUnavailable (4), obtuvo %d", c)
	}

	// 7. Exit Code 1 (ExitUsageOrError): subcomando desconocido, banderas invalidas, JSON corrupto, ID faltante/inexistente
	errorCases := [][]string{
		{"unknown-cmd"},
		{"ingest"},
		{"ingest", "--unknown-flag"},
		{"resolve"},
		{"resolve", "--id", "WI-000000000000"},
		{"show"},
		{"show", "--id", promotedWI.ID, "--format", "xml"},
		{"show", "--id", "WI-000000000000"},
		{"list", "--unknown-flag"},
		{"verify", "--unknown-flag"},
	}
	for _, tc := range errorCases {
		if c, _, _ := execCLI("", tc...); c != ExitUsageOrError {
			t.Fatalf("caso de error %v esperaba ExitUsageOrError (1), obtuvo %d", tc, c)
		}
	}
	if c, _, _ := execCLI("{broken json", "ingest", "--input", "-", "--json"); c != ExitUsageOrError {
		t.Fatalf("JSON roto en stdin esperaba ExitUsageOrError (1), obtuvo %d", c)
	}
}

func TestDiscoverRepoRootAndReorderFlags(t *testing.T) {
	t.Setenv("BF_REPO_ROOT", "")
	if got := discoverRepoRoot("C:/CustomRoot"); got != "C:/CustomRoot" {
		t.Fatalf("discoverRepoRoot flag = %q", got)
	}
	t.Setenv("BF_REPO_ROOT", "C:/EnvRoot")
	if got := discoverRepoRoot(""); got != "C:/EnvRoot" {
		t.Fatalf("discoverRepoRoot env = %q", got)
	}
	t.Setenv("BF_REPO_ROOT", "")
	autoRoot := discoverRepoRoot("")
	if autoRoot == "" {
		t.Fatalf("discoverRepoRoot auto desde CWD no encontro la raiz del repositorio")
	}

	reordered := reorderFlags(
		[]string{"pos1", "--json", "true", "--project", "bf", "--flag=val", "--", "--after"},
		map[string]bool{"json": true},
		map[string]bool{"project": true},
	)
	if len(reordered) < 5 || reordered[0] != "--json=true" || reordered[1] != "--project" || reordered[2] != "bf" {
		t.Fatalf("reorderFlags inesperado: %v", reordered)
	}
}
