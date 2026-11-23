package intake

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"blackfactory/pkg/bfcore"
)

func TestModelHashingIdentityAndValidation(t *testing.T) {
	raw := RawIntake{
		Project:     "Black-Factory",
		Title:       "Fix validator G4",
		Description: "Clean description",
		Note:        "",
		Kind:        "BUG",
		Source:      "CLI",
		SourceRef:   "BUG-104",
	}
	sha1 := ComputeSanitizedSHA256(raw, []string{"0_Brain/engine/validator.go"}, []string{"security", "auth"})
	if len(sha1) != 64 {
		t.Fatalf("ComputeSanitizedSHA256 len = %d, esperaba 64 hex", len(sha1))
	}

	dk := ComputeDedupeKey("Black-Factory", "CLI", "BUG-104")
	if len(dk) != 64 {
		t.Fatalf("ComputeDedupeKey len = %d, esperaba 64 hex", len(dk))
	}
	if ComputeDedupeKey("a|b", "c", "d") == ComputeDedupeKey("a", "b|c", "d") {
		t.Fatalf("ComputeDedupeKey colisiono ante '|' en campos")
	}
	if ComputeDedupeKey("", "cli", "ref") != "" {
		t.Fatalf("ComputeDedupeKey con project vacio debe retornar vacio")
	}

	idReady := FormatWorkItemID(dk, sha1)
	if idReady != "WI-"+dk[:12] {
		t.Fatalf("FormatWorkItemID con dedupeKey = %q, esperaba WI-%s", idReady, dk[:12])
	}
	idQuar := FormatWorkItemID("", sha1)
	if idQuar != "WI-"+sha1[:12] {
		t.Fatalf("FormatWorkItemID sin dedupeKey = %q, esperaba WI-%s", idQuar, sha1[:12])
	}
	idFallback := FormatWorkItemID("short", "short")
	if !strings.HasPrefix(idFallback, "WI-") || len(idFallback) != 15 {
		t.Fatalf("FormatWorkItemID fallback = %q", idFallback)
	}

	trc := FormatTraceID(time.Now().UTC(), "")
	if !strings.HasPrefix(trc, "trc-") {
		t.Fatalf("FormatTraceID = %q", trc)
	}

	flags, ok := NormalizeAndValidateImpactFlags([]string{"security,auth", "db", "auth"})
	if !ok || len(flags) != 3 || flags[0] != "auth" {
		t.Fatalf("NormalizeAndValidateImpactFlags = %v (%v)", flags, ok)
	}
	if _, ok := NormalizeAndValidateImpactFlags([]string{"invalid_flag"}); ok {
		t.Fatalf("NormalizeAndValidateImpactFlags debia rechazar bandera invalida")
	}

	if RiskLevelRank("CRITICO") <= RiskLevelRank("ALTO") || RiskLevelRank("UNKNOWN") != 0 {
		t.Fatalf("RiskLevelRank inesperado")
	}

	// ValidateWorkItem exhaustive checks
	if errs := ValidateWorkItem(nil); len(errs) == 0 {
		t.Fatalf("ValidateWorkItem(nil) debia retornar error")
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	validReady := &WorkItem{
		SchemaVersion:     SchemaVersion,
		ID:                idReady,
		DedupeKey:         dk,
		TraceID:           trc,
		Revision:          1,
		Status:            "ready",
		CreatedAt:         nowStr,
		UpdatedAt:         nowStr,
		Project:           "black-factory",
		Title:             "Title",
		Description:       "Desc",
		Kind:              "bug",
		Source:            "cli",
		SourceRef:         "BUG-104",
		AffectedPaths:     []string{"0_Brain/engine/validator.go"},
		PathsConfidence:   "explicit",
		ImpactFlags:       []string{},
		Lane:              "FTD",
		SuggestedTemplate: "correccion_hallazgos",
		RiskLevel:         "ALTO",
		RiskHits:          []RiskHit{},
		TriageReasons:     []TriageReason{{Code: "FTD_ELIGIBLE_BOUNDED_MAINTENANCE", RuleKind: "deterministic", RulesetVersion: TriageDetRulesetVersion, Detail: "ok"}},
		Sanitization:      SanitizationReport{SanitizedPayloadSHA256: sha1, G4Redactions: []G4Redaction{}},
	}
	if errs := ValidateWorkItem(validReady); len(errs) != 0 {
		t.Fatalf("ValidateWorkItem(validReady) fallo: %v", errs)
	}

	// Invalid ready mutations
	badReady := *validReady
	badReady.SchemaVersion = "2.0"
	badReady.Revision = 0
	badReady.TraceID = ""
	badReady.CreatedAt = "bad-date"
	badReady.UpdatedAt = "bad-date"
	badReady.RiskLevel = "BAD"
	badReady.PathsConfidence = "bad"
	badReady.ImpactFlags = []string{"bad_flag"}
	badReady.DedupeKey = "short"
	badReady.Kind = "bad_kind"
	badReady.Lane = ""
	badReady.SuggestedTemplate = ""
	badReady.QuarantineInfo = &QuarantineInfo{ReasonCodes: []string{"X"}, ResolveHint: "cmd"}
	badReady.TriageReasons = nil
	if errs := ValidateWorkItem(&badReady); len(errs) < 8 {
		t.Fatalf("ValidateWorkItem(badReady) esperaba multiples errores, obtuvo: %v", errs)
	}

	// Valid and invalid quarantine WorkItem
	validQuar := &WorkItem{
		SchemaVersion:   SchemaVersion,
		ID:              idQuar,
		DedupeKey:       "",
		TraceID:         trc,
		Revision:        1,
		Status:          "needs_clarification",
		CreatedAt:       nowStr,
		UpdatedAt:       nowStr,
		Project:         "black-factory",
		Title:           "Title",
		Description:     "Desc",
		Kind:            "bug",
		Source:          "cli",
		AffectedPaths:   []string{},
		PathsConfidence: "none",
		ImpactFlags:     []string{},
		RiskLevel:       "NONE",
		Sanitization:    SanitizationReport{SanitizedPayloadSHA256: sha1, G4Redactions: []G4Redaction{}},
		QuarantineInfo:  &QuarantineInfo{ReasonCodes: []string{"UNCERTAIN_SCOPE"}, MissingFields: []string{"source_ref"}, Detail: "d", ResolveHint: "intake resolve"},
	}
	if errs := ValidateWorkItem(validQuar); len(errs) != 0 {
		t.Fatalf("ValidateWorkItem(validQuar) fallo: %v", errs)
	}

	badQuar := *validQuar
	badQuar.Lane = "ASDD"
	badQuar.SuggestedTemplate = "modulo_nuevo"
	badQuar.QuarantineInfo = nil
	badQuar.DedupeKey = "invalid"
	if errs := ValidateWorkItem(&badQuar); len(errs) < 3 {
		t.Fatalf("ValidateWorkItem(badQuar) esperaba errores: %v", errs)
	}

	unknownStatus := *validReady
	unknownStatus.Status = "unknown"
	if errs := ValidateWorkItem(&unknownStatus); len(errs) == 0 {
		t.Fatalf("ValidateWorkItem con status desconocido debia fallar")
	}
}

func TestInputBoundary_Tier1DualPhase_AndTier2Redaction(t *testing.T) {
	dir := t.TempDir()

	if b, err := ReadRawInput("", dir, nil); err != nil || b != nil {
		t.Fatalf("ReadRawInput vacio debia retornar nil, nil")
	}
	if _, err := ReadRawInput("-", dir, nil); err == nil {
		t.Fatalf("ReadRawInput stdin nil debia fallar")
	}
	if _, err := ReadRawInput(dir, dir, nil); err == nil {
		t.Fatalf("ReadRawInput sobre directorio debia fallar")
	}
	if _, err := ReadRawInput(filepath.Join(dir, "no_existe.json"), dir, nil); err == nil {
		t.Fatalf("ReadRawInput sobre archivo inexistente debia fallar")
	}

	// Quota > MaxInputBytes (1 MiB) en stdin y en archivo
	bigStdin := bytes.NewReader(bytes.Repeat([]byte("a"), MaxInputBytes+10))
	if _, err := ReadRawInput("-", dir, bigStdin); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("ReadRawInput stdin > 1 MiB debia retornar ErrInputTooLarge, obtuvo %v", err)
	}
	bigFile := filepath.Join(dir, "big.json")
	_ = os.WriteFile(bigFile, bytes.Repeat([]byte("a"), MaxInputBytes+1), 0o644)
	if _, err := ReadRawInput(bigFile, dir, nil); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("ReadRawInput archivo > 1 MiB debia retornar ErrInputTooLarge, obtuvo %v", err)
	}

	// UTF-8 BOM strip + CSV affected_paths/impact_flags + DisallowUnknownFields
	bomJSON := "\xef\xbb\xbf" + `{"project":"black-factory","kind":"bug","source":"cli","source_ref":"B-1","title":"T","description":"D","affected_paths":"a.go, b.go","impact_flags":"auth,security"}`
	validFile := filepath.Join(dir, "valid.json")
	_ = os.WriteFile(validFile, []byte(bomJSON), 0o644)
	rawBytes, err := ReadRawInput(validFile, dir, nil)
	if err != nil || bytes.HasPrefix(rawBytes, []byte("\xef\xbb\xbf")) {
		t.Fatalf("ReadRawInput no elimino UTF-8 BOM: err=%v", err)
	}
	parsed, err := ParseJSONIntake(rawBytes)
	if err != nil || parsed.Project != "black-factory" || len(parsed.AffectedPaths) != 2 || len(parsed.ImpactFlags) != 2 {
		t.Fatalf("ParseJSONIntake fallo: %v (%+v)", err, parsed)
	}

	if _, err := ParseJSONIntake([]byte("   ")); err == nil {
		t.Fatalf("ParseJSONIntake vacio debia fallar")
	}
	if _, err := ParseJSONIntake([]byte(`{"project":"bf"} {"extra":1}`)); err == nil {
		t.Fatalf("ParseJSONIntake con objeto extra debia fallar")
	}
	if _, err := ParseJSONIntake([]byte(`{"project":"bf","unknown_field":123}`)); err == nil {
		t.Fatalf("ParseJSONIntake debia rechazar campos desconocidos con DisallowUnknownFields")
	}

	// MergeCLIOverrides full fields
	merged := MergeCLIOverrides(parsed, RawIntake{
		Project:       "p2",
		Title:         "t2",
		Description:   "d2",
		Note:          "n2",
		Kind:          "feature",
		Source:        "issue",
		SourceRef:     "R2",
		AffectedPaths: []string{"c.go"},
		ImpactFlags:   []string{"db"},
	})
	if merged.Project != "p2" || merged.Note != "n2" || merged.Kind != "feature" || merged.AffectedPaths[0] != "c.go" || merged.ImpactFlags[0] != "db" {
		t.Fatalf("MergeCLIOverrides fallo: %+v", merged)
	}

	// Dual-Phase Tier 1 G4: Raw secret + CLI flag secret + JSON Unicode-escaped secret (\u0067hp_...)
	rawSecretJSON := []byte(`{"title":"token ` + "ghp_" + `1234567890abcdefghij123456"}`)
	if hits := ScanRawAndFlagsTier1(rawSecretJSON, RawIntake{
		Title:         "Bearer " + "eyJhbGciOiJIUzI1NiJ9_secret",
		AffectedPaths: []string{"ghp_" + "1234567890abcdefghij123456"},
		ImpactFlags:   []string{"ghp_" + "1234567890abcdefghij123456"},
	}, false); len(hits) < 3 {
		t.Fatalf("Phase-A ScanRawAndFlagsTier1 debia detectar tokens en raw y banderas, obtuvo %+v", hits)
	}

	escapedSecretJSON := []byte(`{"project":"black-factory","kind":"bug","source":"cli","source_ref":"1","title":"escaped","description":"\u0067hp_1234567890abcdefghij123456"}`)
	decEscaped, err := ParseJSONIntake(escapedSecretJSON)
	if err != nil {
		t.Fatalf("ParseJSONIntake escaped fallo: %v", err)
	}
	if hits := ScanDecodedTier1(decEscaped, false); len(hits) == 0 || hits[0].Rule != "known_token" {
		t.Fatalf("Phase-B ScanDecodedTier1 debia detectar token JSON-escapado \\u0067hp_..., obtuvo: %+v", hits)
	}

	// Tier 2 Fixed-Point Redaction en title/description/note vs deteccion en metadata
	inTier2 := RawIntake{
		Project:     "black-factory",
		Title:       "Bug en /home/" + "alice /home/" + "bob",
		Description: `Stack en C:\` + `Users\jdoe\main.go por jdoe` + `@example.com`,
		Note:        "Nota de /Users/" + "admin/log.txt",
		Kind:        "bug",
		Source:      "cli",
		SourceRef:   "REF-1",
	}
	sanitized, redactions, metaHasTier2 := SanitizeAndRedactTier2(inTier2)
	if metaHasTier2 || len(redactions) < 3 {
		t.Fatalf("esperaba redacciones sin metaHasTier2: meta=%v red=%+v", metaHasTier2, redactions)
	}
	if len(bfcore.ScanG4All(sanitized.Title+" "+sanitized.Description+" "+sanitized.Note)) != 0 {
		t.Fatalf("SanitizeAndRedactTier2 dejo PII sin redactar: %+v", sanitized)
	}

	inMetaPII := inTier2
	inMetaPII.SourceRef = "ticket-" + "user" + "@example.com"
	_, _, metaHasTier2 = SanitizeAndRedactTier2(inMetaPII)
	if !metaHasTier2 {
		t.Fatalf("metaHasTier2 debia ser true cuando source_ref contiene un email Tier 2")
	}
}

func TestSanitizeAffectedPaths_AndCountRootModules(t *testing.T) {
	repoRoot := `C:\Black Factory`

	if clean, reason := SanitizeAffectedPaths(nil, repoRoot); len(clean) != 0 || reason != "" {
		t.Fatalf("SanitizeAffectedPaths(nil) = %v, %q", clean, reason)
	}
	if clean, reason := SanitizeAffectedPaths([]string{" , "}, repoRoot); len(clean) != 0 || reason != "" {
		t.Fatalf("SanitizeAffectedPaths vacio = %v, %q", clean, reason)
	}
	if mods := CountRootModules(nil); mods != nil {
		t.Fatalf("CountRootModules(nil) = %v", mods)
	}

	clean, reason := SanitizeAffectedPaths([]string{
		`0_Brain\engine\validator.go`,
		`./1_Intake/engine/model.go`,
		`C:\Black Factory\GengiSWAP\engine\ring.go`,
		`0_Brain/engine/validator.go`,
	}, repoRoot)
	if reason != "" || len(clean) != 3 {
		t.Fatalf("SanitizeAffectedPaths valido fallo: reason=%q clean=%v", reason, clean)
	}
	mods := CountRootModules(clean)
	if len(mods) != 3 {
		t.Fatalf("CountRootModules = %v, esperaba 3 modulos", mods)
	}
	if modsRoot := CountRootModules([]string{"README.md", "go.work"}); len(modsRoot) != 1 || modsRoot[0] != "." {
		t.Fatalf("CountRootModules archivos raiz = %v, esperaba [\".\"]", modsRoot)
	}

	if p, r := SanitizeAffectedPaths([]string{`C:\` + `Users\admin\app.go`}, repoRoot); p != nil || r != "G4_personal_path_in_affected_paths" {
		t.Fatalf("esperaba G4_personal_path_in_affected_paths, obtuvo paths=%v reason=%q", p, r)
	}
	if p, r := SanitizeAffectedPaths([]string{`/home/` + `alice/app.go`}, repoRoot); p != nil || r != "G4_personal_path_in_affected_paths" {
		t.Fatalf("esperaba G4_personal_path_in_affected_paths para unix_user_path, obtuvo %v %q", p, r)
	}

	invalidVectors := []string{
		`../outside.go`,
		`0_Brain/../../secret.go`,
		`\leading_backslash.go`,
		`/leading_slash.go`,
		`\\server\share\file.go`,
		`\\?\C:\file.go`,
		`C:relative_drive.go`,
		`D:\OtherRepo\file.go`,
		`0_Brain/engine/`,
		`0_Brain/engine/*.go`,
		`0_Brain/engine/file?.go`,
		`0_Brain/engine/[a-z].go`,
		`0_Brain/CON.go`,
		`1_Intake/nul.txt`,
		`pkg/com1/main.go`,
		"0_Brain/\x00bad.go",
		`./`,
		`0_Brain//engine/a.go`,
	}
	for _, vec := range invalidVectors {
		if p, r := SanitizeAffectedPaths([]string{vec}, repoRoot); p != nil || r != "PATH_TRAVERSAL_OR_INVALID_SCOPE" {
			t.Fatalf("vector invalido %q debia ser rechazado con PATH_TRAVERSAL_OR_INVALID_SCOPE, obtuvo paths=%v reason=%q", vec, p, r)
		}
	}
}

func TestBrainClient_DisjunctiveFTS_ReadOnlyPreflight_AndRealSubprocess(t *testing.T) {
	tokens := ExtractQueryTokens(
		"Fix AND/OR/NOT/NEAR bug in 0_Brain/engine/validator.go for RZ-002 and ScanG4All",
		"Check ADR-0001 and workitem.schema.json with camelCaseIdentifier and snake_case_tok",
		0, // default 5
	)
	if len(tokens) == 0 || len(tokens) > 5 {
		t.Fatalf("ExtractQueryTokens len inesperado: %v", tokens)
	}
	ftsQ := BuildDisjunctiveFTSQuery(append(tokens, "AND", "OR", "NOT", "NEAR", ""))
	if !strings.Contains(ftsQ, " OR ") || strings.Contains(ftsQ, `"AND"`) {
		t.Fatalf("BuildDisjunctiveFTSQuery formato inesperado: %q", ftsQ)
	}
	if BuildDisjunctiveFTSQuery(nil) != "" {
		t.Fatalf("BuildDisjunctiveFTSQuery(nil) debe ser vacio")
	}

	vaultDir := t.TempDir()
	dbPath := filepath.Join(vaultDir, ".index", "brain.db")
	client := NewSubprocessBrainClient(filepath.Join(vaultDir, "missing_brain.exe"), 0)

	if _, err := client.Preflight("", ""); !errors.Is(err, ErrBrainUnavailable) {
		t.Fatalf("Preflight con vault vacio debia fallar")
	}
	if _, err := client.Preflight(vaultDir, ""); !errors.Is(err, ErrBrainUnavailable) {
		t.Fatalf("Preflight sin brain.db debia retornar ErrBrainUnavailable, obtuvo %v", err)
	}

	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	_ = os.WriteFile(dbPath, []byte("sqlite-mock"), 0o644)
	if pf, err := client.Preflight(vaultDir, dbPath); err != nil || pf.BrainDBMTime == "" {
		t.Fatalf("Preflight limpio fallo: err=%v pf=%+v", err, pf)
	}

	// Missing binary on Context/Query must return ErrBrainUnavailable
	if _, _, err := client.Context(vaultDir, dbPath, "black-factory", []string{"a.go"}); !errors.Is(err, ErrBrainUnavailable) {
		t.Fatalf("Context con brain.exe inexistente debia retornar ErrBrainUnavailable, obtuvo %v", err)
	}
	if _, _, err := client.Query(vaultDir, dbPath, BrainQueryOpts{Project: "black-factory", Type: "project", Limit: 1, Query: "test"}); !errors.Is(err, ErrBrainUnavailable) {
		t.Fatalf("Query con brain.exe inexistente debia retornar ErrBrainUnavailable, obtuvo %v", err)
	}

	// IsProjectActiveOnDisk checks
	if IsProjectActiveOnDisk(vaultDir, "") || IsProjectActiveOnDisk(vaultDir, "../bad") || IsProjectActiveOnDisk(vaultDir, "black-factory") {
		t.Fatalf("IsProjectActiveOnDisk debia ser false cuando no existe projects/black-factory.md")
	}
	_ = os.MkdirAll(filepath.Join(vaultDir, "projects"), 0o755)
	_ = os.WriteFile(filepath.Join(vaultDir, "projects", "black-factory.md"), []byte("---\nid: p-bf\ntitle: BF\ntype: project\nproject: black-factory\nstatus: active\n---\n# BF\n"), 0o644)
	if !IsProjectActiveOnDisk(vaultDir, "black-factory") {
		t.Fatalf("IsProjectActiveOnDisk debia ser true para projects/black-factory.md activo")
	}

	journalPath := filepath.Join(vaultDir, ".brain-journal.json")
	_ = os.WriteFile(journalPath, []byte(`{"proposal_id":"p1"}`), 0o644)
	if _, err := client.Preflight(vaultDir, dbPath); !errors.Is(err, ErrBrainUnavailable) {
		t.Fatalf("Preflight con .brain-journal.json pendiente debia retornar ErrBrainUnavailable (NG2), obtuvo %v", err)
	}

	// Si 0_Brain/brain.exe y 0_Brain/vault existen en el repositorio local, probar invocacion real de subproceso y latencia < 500 ms
	realBrainBin := filepath.Clean(filepath.Join("..", "..", "0_Brain", "brain.exe"))
	realVault := filepath.Clean(filepath.Join("..", "..", "0_Brain", "vault"))
	if _, err := os.Stat(realBrainBin); err == nil {
		if _, errV := os.Stat(filepath.Join(realVault, ".index", "brain.db")); errV == nil {
			realClient := NewSubprocessBrainClient(realBrainBin, 5*time.Second)
			start := time.Now()
			pf, err := realClient.Preflight(realVault, "")
			if err != nil {
				t.Fatalf("realClient.Preflight fallo: %v", err)
			}
			ctxResp, inv1, err := realClient.Context(realVault, pf.DBPath, "black-factory", []string{"gengiswap/engine/ring.go"})
			if err != nil || inv1.ExitCode != 0 || len(ctxResp.RiskHits) == 0 || ctxResp.RiskHits[0].ID != "RZ-001" {
				t.Fatalf("realClient.Context RZ-001 fallo: err=%v hits=%+v", err, ctxResp.RiskHits)
			}
			qDocs, inv2, err := realClient.Query(realVault, pf.DBPath, BrainQueryOpts{Project: "black-factory", Query: `"validator.go" OR "g4"`, Limit: 5})
			if err != nil || inv2.ExitCode != 0 || qDocs == nil {
				t.Fatalf("realClient.Query fallo: err=%v", err)
			}
			elapsed := time.Since(start)
			if elapsed > 500*time.Millisecond {
				t.Logf("Aviso latencia primera corrida en frio: %v", elapsed)
			}
		}
	}
}

func TestDeterministicTriageMatrix_All13Rules_StoreAndRender(t *testing.T) {
	// EvaluateMetadataAndScope & EvaluateProjectRegistration
	qInfo := EvaluateMetadataAndScope("WI-111111111111", RawIntake{Kind: "invalid_kind"}, nil, "G4_personal_path_in_affected_paths", true, false)
	if qInfo == nil || len(qInfo.ReasonCodes) != 3 {
		t.Fatalf("EvaluateMetadataAndScope debia reportar 3 reason_codes (MISSING_REQUIRED_FIELDS, INVALID_AFFECTED_PATH_G4, UNCERTAIN_SCOPE): %+v", qInfo)
	}
	if EvaluateProjectRegistration("WI-111111111111", RawIntake{Project: "unregistered"}, false) == nil {
		t.Fatalf("EvaluateProjectRegistration(false) debia retornar QuarantineInfo")
	}
	if EvaluateProjectRegistration("WI-111111111111", RawIntake{Project: "black-factory"}, true) != nil {
		t.Fatalf("EvaluateProjectRegistration(true) debia retornar nil")
	}

	// Rule 7: RZ-001 CRITICO -> ASDD + requires_autor_hitl: true + correccion_hallazgos
	wiCrit := &WorkItem{
		Project:       "black-factory",
		Kind:          "bug",
		AffectedPaths: []string{"gengiswap/engine/ring.go"},
		ImpactFlags:   []string{"security"},
	}
	ctxCrit := BrainContextResponse{
		HasActiveRiskZones: true,
		RiskZonesSHA256:    strings.Repeat("a", 64),
		RiskHits: []RiskHit{
			{ID: "RZ-OTHER", Project: "other-project", Level: "BAJO", MatchedPaths: []string{"x"}},
			{ID: "RZ-001", Project: "black-factory", Level: "CRITICO", MatchedPaths: []string{"gengiswap/engine/ring.go"}, MandatoryGate: "PR + 4_Revisores + [AUTOR]"},
		},
		MatchedDocs: []BrainDocMatch{
			{ID: "wiki-gengis", RelPath: "wiki/gengis.md", Type: "wiki", Status: "active", CoversPaths: []string{"GengiSWAP/engine/"}, MatchedPaths: []string{"gengiswap/engine/ring.go"}},
		},
	}
	TriageWorkItem(wiCrit, ctxCrit, 0)
	if wiCrit.Lane != "ASDD" || !wiCrit.RequiresAutorHITL || wiCrit.RiskLevel != "CRITICO" || wiCrit.SuggestedTemplate != "correccion_hallazgos" {
		t.Fatalf("Rule 7 CRITICO triaje inesperado: %+v", wiCrit)
	}

	// Rule 8: Fail-safe cuando risk_zones es inparseable -> ASDD + requires_autor_hitl: true
	wiFailSafe := &WorkItem{
		Project:       "black-factory",
		Kind:          "bug",
		AffectedPaths: []string{"0_Brain/engine/validator.go", "1_Intake/engine/model.go"},
	}
	TriageWorkItem(wiFailSafe, BrainContextResponse{HasActiveRiskZones: false, RiskZonesParseError: "tabla corrupta"}, 5)
	if wiFailSafe.Lane != "ASDD" || !wiFailSafe.RequiresAutorHITL {
		t.Fatalf("Rule 8 RISK_ZONES_UNPARSEABLE_FAILSAFE debia forzar ASDD + HITL: %+v", wiFailSafe)
	}

	// Rule 13: RZ-002 ALTO en bug single-module con wiki activo y 0 impact_flags -> FTD
	wiFTD := &WorkItem{
		Project:       "black-factory",
		Kind:          "bug",
		AffectedPaths: []string{"0_Brain/engine/validator.go"},
		ImpactFlags:   []string{},
	}
	ctxFTD := BrainContextResponse{
		HasActiveRiskZones: true,
		RiskZonesSHA256:    strings.Repeat("b", 64),
		RiskHits: []RiskHit{{
			ID:            "RZ-002",
			Project:       "black-factory",
			Level:         "ALTO",
			MatchedPaths:  []string{"0_Brain/engine/validator.go"},
			MandatoryGate: "go vet + go test",
		}},
		MatchedDocs: []BrainDocMatch{
			{ID: "brain-engine-g4", RelPath: "wiki/brain_engine_g4.md", Type: "wiki", Status: "active", CoversPaths: []string{"0_Brain/engine/"}},
		},
	}
	TriageWorkItem(wiFTD, ctxFTD, 5)
	if wiFTD.Lane != "FTD" || wiFTD.RequiresAutorHITL || wiFTD.RiskLevel != "ALTO" || wiFTD.SuggestedTemplate != "correccion_hallazgos" {
		t.Fatalf("Rule 13 FTD con RZ-002 ALTO inesperado: %+v", wiFTD)
	}

	// Rule 12: Bug sin cobertura wiki/adr -> ASDD + mapeo_estatico
	wiNoCov := &WorkItem{Project: "black-factory", Kind: "bug", AffectedPaths: []string{"2_Spec_Writer/main.go"}}
	TriageWorkItem(wiNoCov, BrainContextResponse{HasActiveRiskZones: true}, 5)
	if wiNoCov.Lane != "ASDD" || wiNoCov.SuggestedTemplate != "mapeo_estatico" {
		t.Fatalf("Rule 12 NO_BRAIN_COVERAGE unico trigger debia asignar ASDD + mapeo_estatico, obtuvo: %+v", wiNoCov)
	}

	// Rule 9: feature -> modulo_nuevo; ops -> auditoria_e2e
	wiFeat := &WorkItem{Project: "black-factory", Kind: "feature", AffectedPaths: []string{"0_Brain/engine/validator.go"}}
	TriageWorkItem(wiFeat, ctxFTD, 5)
	if wiFeat.Lane != "ASDD" || wiFeat.SuggestedTemplate != "modulo_nuevo" {
		t.Fatalf("Rule 9 feature inesperado: %+v", wiFeat)
	}
	wiOps := &WorkItem{Project: "black-factory", Kind: "ops", AffectedPaths: []string{"0_Brain/engine/validator.go"}}
	TriageWorkItem(wiOps, ctxFTD, 5)
	if wiOps.Lane != "ASDD" || wiOps.SuggestedTemplate != "auditoria_e2e" {
		t.Fatalf("Rule 9 ops inesperado: %+v", wiOps)
	}

	// Store & Render tests
	if RenderWorkItemMarkdown(nil) != "" {
		t.Fatalf("RenderWorkItemMarkdown(nil) debe ser vacio")
	}

	dataDir := filepath.Join(t.TempDir(), "runtime")
	store := NewStore(dataDir)
	nowStr := time.Now().UTC().Format(time.RFC3339)
	dk := ComputeDedupeKey("black-factory", "cli", "S-1")
	sha := ComputeSanitizedSHA256(RawIntake{Project: "black-factory", Source: "cli", SourceRef: "S-1", Title: "T", Description: "D", Kind: "bug"}, []string{"0_Brain/engine/validator.go"}, nil)

	wiStore := &WorkItem{
		SchemaVersion:     SchemaVersion,
		ID:                FormatWorkItemID(dk, sha),
		DedupeKey:         dk,
		TraceID:           "trc-1",
		Revision:          1,
		Status:            "ready",
		CreatedAt:         nowStr,
		UpdatedAt:         nowStr,
		Project:           "black-factory",
		Title:             "Store Test",
		Description:       "Redacted [REDACTED:G4:email_address]",
		Note:              "Operational note",
		Kind:              "bug",
		Source:            "cli",
		SourceRef:         "S-1",
		AffectedPaths:     []string{"0_Brain/engine/validator.go"},
		PathsConfidence:   "explicit",
		ImpactFlags:       []string{},
		Lane:              "FTD",
		SuggestedTemplate: "correccion_hallazgos",
		RiskLevel:         "ALTO",
		RiskHits:          ctxFTD.RiskHits,
		TriageReasons:     wiFTD.TriageReasons,
		Sanitization: SanitizationReport{
			SanitizedPayloadSHA256: sha,
			G4Redactions:           []G4Redaction{{Rule: "email_address", Field: "description", Count: 1}},
		},
		BrainEvidence: BrainEvidence{
			VaultPath:            "0_Brain/vault",
			BrainDBMTime:         nowStr,
			RiskZonesSHA256:      ctxFTD.RiskZonesSHA256,
			ExtractedQueryTokens: []string{"validator.go"},
			Invocations:          []BrainInvocation{{Command: "0_Brain/brain.exe context --json", ExitCode: 0, DurationMs: 10, ResultCount: 1}},
			MatchedDocs:          ctxFTD.MatchedDocs,
		},
	}

	// Dry-run writes 0 files
	dryRes, err := store.SaveTriaged(wiStore, "", false, true, 5)
	if err != nil || !dryRes.DryRun {
		t.Fatalf("SaveTriaged dryRun fallo: %v", err)
	}
	if _, statErr := os.Stat(dryRes.Path); !os.IsNotExist(statErr) {
		t.Fatalf("SaveTriaged dryRun no debia escribir archivo en disco")
	}

	// Real SaveTriaged + Idempotent hit + Conflict
	saveRes, err := store.SaveTriaged(wiStore, "", false, false, 15)
	if err != nil || saveRes.IdempotentHit {
		t.Fatalf("SaveTriaged #1 fallo: %v (%+v)", err, saveRes)
	}
	idemRes, err := store.SaveTriaged(wiStore, "", false, false, 5)
	if err != nil || !idemRes.IdempotentHit {
		t.Fatalf("SaveTriaged #2 debia ser IdempotentHit: %v (%+v)", err, idemRes)
	}

	// 48-bit prefix collision detection (same 12-hex prefix, different 64-hex dedupe_key)
	collidingWI := *wiStore
	collidingWI.DedupeKey = dk[:12] + strings.Repeat("f", 52)
	if _, err := store.SaveTriaged(&collidingWI, "", true, false, 5); !errors.Is(err, ErrIDCollision) {
		t.Fatalf("SaveTriaged con colision de prefijo de 48 bits debia retornar ErrIDCollision, obtuvo: %v", err)
	}

	// Markdown rendering check (ready and quarantine)
	mdReady := RenderWorkItemMarkdown(wiStore)
	if !strings.Contains(mdReady, "AUTO-GENERATED DERIVED VIEW") || strings.Contains(mdReady, "```mermaid") {
		t.Fatalf("RenderWorkItemMarkdown ready invalido: %s", mdReady)
	}

	wiQuar := &WorkItem{
		SchemaVersion:   SchemaVersion,
		ID:              "WI-" + sha[:12],
		DedupeKey:       "",
		TraceID:         "trc-2",
		Revision:        1,
		Status:          "needs_clarification",
		CreatedAt:       nowStr,
		UpdatedAt:       nowStr,
		Project:         "black-factory",
		Title:           "Quarantine Test",
		Description:     "Missing path",
		Kind:            "bug",
		Source:          "cli",
		AffectedPaths:   []string{},
		PathsConfidence: "none",
		ImpactFlags:     []string{},
		RiskLevel:       "NONE",
		Sanitization:    SanitizationReport{SanitizedPayloadSHA256: sha, G4Redactions: []G4Redaction{}},
		QuarantineInfo: &QuarantineInfo{
			ReasonCodes:       []string{"INVALID_AFFECTED_PATH_G4", "UNCERTAIN_SCOPE"},
			MissingFields:     []string{"source_ref"},
			InvalidPathReason: "G4_personal_path_in_affected_paths",
			Detail:            "path discarded",
			ResolveHint:       "intake resolve --id WI-" + sha[:12],
		},
	}
	if _, err := store.SaveQuarantined(wiQuar, "", false, true, 5); err != nil {
		t.Fatalf("SaveQuarantined dryRun fallo: %v", err)
	}
	if _, err := store.SaveQuarantined(wiQuar, "", false, false, 5); err != nil {
		t.Fatalf("SaveQuarantined #1 fallo: %v", err)
	}
	// Re-save identical quarantine item preserves revision 1
	qRes2, err := store.SaveQuarantined(wiQuar, "", false, false, 5)
	if err != nil || qRes2.Revision != 1 {
		t.Fatalf("SaveQuarantined idempotente debia preservar Revision=1: %v (%+v)", err, qRes2)
	}
	mdQuar := RenderWorkItemMarkdown(wiQuar)
	if !strings.Contains(mdQuar, "Retención en Cuarentena") || !strings.Contains(mdQuar, "G4_personal_path_in_affected_paths") {
		t.Fatalf("RenderWorkItemMarkdown quarantine incompleto: %s", mdQuar)
	}

	// List filters & LoadByID errors
	listAll, err := store.List(ListFilter{Project: "black-factory"})
	if err != nil || len(listAll) != 2 {
		t.Fatalf("store.List esperaba 2 items, obtuvo %d (%v)", len(listAll), err)
	}
	listFTD, err := store.List(ListFilter{Lane: "FTD", Risk: "ALTO"})
	if err != nil || len(listFTD) != 1 {
		t.Fatalf("store.List FTD/ALTO esperaba 1 item, obtuvo %d (%v)", len(listFTD), err)
	}
	if _, _, err := store.LoadByID("INVALID-ID"); !errors.Is(err, ErrWorkItemNotFound) {
		t.Fatalf("LoadByID invalido debia retornar ErrWorkItemNotFound: %v", err)
	}
	if _, _, err := store.LoadByID("WI-000000000000"); !errors.Is(err, ErrWorkItemNotFound) {
		t.Fatalf("LoadByID inexistente debia retornar ErrWorkItemNotFound: %v", err)
	}
	if _, _, err := store.LoadQuarantinedByID("INVALID-ID"); !errors.Is(err, ErrWorkItemNotFound) {
		t.Fatalf("LoadQuarantinedByID invalido debia retornar ErrWorkItemNotFound: %v", err)
	}
	if _, _, err := store.LoadQuarantinedByID("WI-000000000000"); !errors.Is(err, ErrWorkItemNotFound) {
		t.Fatalf("LoadQuarantinedByID inexistente debia retornar ErrWorkItemNotFound: %v", err)
	}
	if gotWI, _, err := store.LoadByID(wiStore.ID); err != nil || gotWI.ID != wiStore.ID {
		t.Fatalf("LoadByID existente en workitems fallo: %v", err)
	}
	if gotQ, _, err := store.LoadByID(wiQuar.ID); err != nil || gotQ.ID != wiQuar.ID {
		t.Fatalf("LoadByID existente en quarantine fallo: %v", err)
	}

	// Re-save modified quarantine item increments revision to 2
	wiQuarMod := *wiQuar
	wiQuarMod.Note = "Updated quarantine note"
	wiQuarMod.Sanitization.SanitizedPayloadSHA256 = strings.Repeat("c", 64)
	wiQuarMod.ID = "WI-" + wiQuarMod.Sanitization.SanitizedPayloadSHA256[:12]
	qResMod, err := store.SaveQuarantined(&wiQuarMod, wiQuar.ID, false, false, 5)
	if err != nil || qResMod.Revision != 2 {
		t.Fatalf("SaveQuarantined modificado con prevQuarantineID debia incrementar Revision=2: %v (%+v)", err, qResMod)
	}

	// SaveQuarantined moving an existing ready item to quarantine when --update is true
	wiDemote := *wiStore
	wiDemote.Status = "needs_clarification"
	wiDemote.Lane = ""
	wiDemote.SuggestedTemplate = ""
	wiDemote.AffectedPaths = []string{}
	wiDemote.PathsConfidence = "none"
	wiDemote.TriageReasons = []TriageReason{}
	wiDemote.QuarantineInfo = &QuarantineInfo{ReasonCodes: []string{"UNCERTAIN_SCOPE"}, MissingFields: []string{}, ResolveHint: "intake resolve"}
	if _, err := store.SaveQuarantined(&wiDemote, "", false, false, 5); !errors.Is(err, ErrDedupeConflict) {
		t.Fatalf("SaveQuarantined sobre item ready existente sin --update debia retornar ErrDedupeConflict: %v", err)
	}
	if _, err := store.SaveQuarantined(&wiDemote, "", true, false, 5); err != nil {
		t.Fatalf("SaveQuarantined sobre item ready existente con --update fallo: %v", err)
	}

	// AppendRejectedG4Event dryRun and real
	rejEv := IntakeEvent{
		TraceID:   "trc-rej",
		RuleName:  "known_token",
		Field:     "raw_input",
		Line:      1,
		RawSHA256: strings.Repeat("d", 64),
		Project:   "black-factory",
		Source:    "cli",
		SourceRef: "ghp_" + "1234567890abcdefghij123456",
	}
	if err := store.AppendRejectedG4Event(rejEv, true); err != nil {
		t.Fatalf("AppendRejectedG4Event dryRun fallo: %v", err)
	}
	if err := store.AppendRejectedG4Event(rejEv, false); err != nil {
		t.Fatalf("AppendRejectedG4Event real fallo: %v", err)
	}
}

func TestVerifyRuntime_DetectsSplitBrainG4AndGitIgnore(t *testing.T) {
	repoDir := t.TempDir()
	dataDir := filepath.Join(repoDir, "runtime")
	vaultDir := filepath.Join(repoDir, "vault")
	store := NewStore(dataDir)

	nowStr := time.Now().UTC().Format(time.RFC3339)
	dk := ComputeDedupeKey("black-factory", "cli", "V-1")
	sha := ComputeSanitizedSHA256(RawIntake{Project: "black-factory", Source: "cli", SourceRef: "V-1", Title: "T", Description: "D", Kind: "bug"}, []string{"a.go"}, nil)
	wi := &WorkItem{
		SchemaVersion:     SchemaVersion,
		ID:                "WI-" + dk[:12],
		DedupeKey:         dk,
		TraceID:           "trc-v1",
		Revision:          1,
		Status:            "ready",
		CreatedAt:         nowStr,
		UpdatedAt:         nowStr,
		Project:           "black-factory",
		Title:             "Verify Test",
		Description:       "Clean",
		Kind:              "bug",
		Source:            "cli",
		SourceRef:         "V-1",
		AffectedPaths:     []string{"a.go"},
		PathsConfidence:   "explicit",
		ImpactFlags:       []string{},
		Lane:              "FTD",
		SuggestedTemplate: "correccion_hallazgos",
		RiskLevel:         "NONE",
		TriageReasons:     []TriageReason{{Code: "FTD_ELIGIBLE_BOUNDED_MAINTENANCE", RuleKind: "deterministic", RulesetVersion: TriageDetRulesetVersion, Detail: "ok"}},
		Sanitization:      SanitizationReport{SanitizedPayloadSHA256: sha, G4Redactions: []G4Redaction{}},
	}
	if _, err := store.SaveTriaged(wi, "", false, false, 5); err != nil {
		t.Fatalf("SaveTriaged: %v", err)
	}

	// Simular split-brain escribiendo un archivo con el mismo ID en quarantine/ y una linea corrupta en intake_events.jsonl
	badQuar := *wi
	badQuar.Status = "needs_clarification"
	badQuar.Lane = ""
	badQuar.SuggestedTemplate = ""
	badQuar.QuarantineInfo = &QuarantineInfo{ReasonCodes: []string{"UNCERTAIN_SCOPE"}, ResolveHint: "intake resolve"}
	rawBadQuar, _ := json.Marshal(badQuar)
	_ = os.WriteFile(filepath.Join(dataDir, "quarantine", wi.ID+".json"), rawBadQuar, 0o644)

	rep, err := store.VerifyRuntime(repoDir, vaultDir, nil)
	if err != nil {
		t.Fatalf("VerifyRuntime error: %v", err)
	}
	if rep.OK || len(rep.Errors) < 2 {
		t.Fatalf("VerifyRuntime debia detectar split-brain y BrainClient nil: %+v", rep)
	}

	// Probar VerifyRuntime con un mockBrainClient limpio y luego con errores en Preflight/Context/RiskZones
	cleanDir := filepath.Join(t.TempDir(), "clean_runtime")
	cleanStore := NewStore(cleanDir)
	if _, err := cleanStore.SaveTriaged(wi, "", false, false, 5); err != nil {
		t.Fatalf("SaveTriaged cleanStore: %v", err)
	}
	// Promocion desde cuarentena hacia workitems con el mismo ID o distinto ID
	quarProm := &WorkItem{
		SchemaVersion:   SchemaVersion,
		ID:              "WI-" + sha[:12],
		DedupeKey:       "",
		TraceID:         "trc-qp",
		Revision:        1,
		Status:          "needs_clarification",
		CreatedAt:       nowStr,
		UpdatedAt:       nowStr,
		Project:         "black-factory",
		Title:           "Promote Test",
		Description:     "D",
		Kind:            "bug",
		Source:          "cli",
		AffectedPaths:   []string{},
		PathsConfidence: "none",
		ImpactFlags:     []string{},
		RiskLevel:       "NONE",
		Sanitization:    SanitizationReport{SanitizedPayloadSHA256: sha, G4Redactions: []G4Redaction{}},
		QuarantineInfo:  &QuarantineInfo{ReasonCodes: []string{"UNCERTAIN_SCOPE"}, MissingFields: []string{"source_ref"}, ResolveHint: "intake resolve"},
	}
	if _, err := cleanStore.SaveQuarantined(quarProm, "", false, false, 5); err != nil {
		t.Fatalf("SaveQuarantined quarProm: %v", err)
	}
	dkProm := ComputeDedupeKey("black-factory", "cli", "PROM-1")
	readyProm := *wi
	readyProm.ID = "WI-" + dkProm[:12]
	readyProm.DedupeKey = dkProm
	readyProm.SourceRef = "PROM-1"
	pRes, err := cleanStore.SaveTriaged(&readyProm, quarProm.ID, false, false, 5)
	if err != nil || !pRes.Promoted {
		t.Fatalf("SaveTriaged promocion desde cuarentena fallo: err=%v res=%+v", err, pRes)
	}

	vClient := &verifyMockBrainClient{
		ctxResp: BrainContextResponse{
			HasActiveRiskZones: true,
			RiskZonesSHA256:    strings.Repeat("1", 64),
		},
	}
	repClean, err := cleanStore.VerifyRuntime("", vaultDir, vClient)
	if err != nil || !repClean.OK || !repClean.BrainOK {
		t.Fatalf("VerifyRuntime limpio debia retornar OK=true: err=%v rep=%+v", err, repClean)
	}

	vClient.ctxResp.HasActiveRiskZones = false
	repNoRZ, _ := cleanStore.VerifyRuntime("", vaultDir, vClient)
	if repNoRZ.OK || repNoRZ.BrainOK {
		t.Fatalf("VerifyRuntime sin risk_zones activo debia fallar: %+v", repNoRZ)
	}

	vClient.pfErr = ErrBrainUnavailable
	repPfErr, _ := cleanStore.VerifyRuntime("", vaultDir, vClient)
	if repPfErr.OK || repPfErr.BrainOK {
		t.Fatalf("VerifyRuntime con preflight error debia fallar: %+v", repPfErr)
	}

	// Si git esta disponible, probar la deteccion de git check-ignore en un repo Git temporal sin .gitignore
	if _, gitErr := exec.LookPath("git"); gitErr == nil {
		gitCmd := exec.Command("git", "init", repoDir)
		if err := gitCmd.Run(); err == nil {
			t.Cleanup(func() {
				_ = filepath.WalkDir(repoDir, func(p string, d os.DirEntry, err error) error {
					if err == nil {
						_ = os.Chmod(p, 0o755)
					}
					return nil
				})
			})
			repGit, _ := store.VerifyRuntime(repoDir, vaultDir, nil)
			if repGit.GitIgnored {
				t.Fatalf("VerifyRuntime debia detectar que runtime/ NO esta ignorado en un repo Git sin .gitignore")
			}
		}
	}
}

type verifyMockBrainClient struct {
	pfErr   error
	ctxErr  error
	ctxResp BrainContextResponse
}

func (v *verifyMockBrainClient) Preflight(vaultRoot, dbPath string) (BrainPreflight, error) {
	if v.pfErr != nil {
		return BrainPreflight{}, v.pfErr
	}
	return BrainPreflight{VaultRoot: vaultRoot, DBPath: dbPath, BrainDBMTime: "2026-11-22T12:00:00Z"}, nil
}

func (v *verifyMockBrainClient) Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error) {
	if v.ctxErr != nil {
		return BrainContextResponse{}, BrainInvocation{}, v.ctxErr
	}
	return v.ctxResp, BrainInvocation{Command: "brain context", ExitCode: 0}, nil
}

func (v *verifyMockBrainClient) Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error) {
	return []BrainDocMatch{}, BrainInvocation{Command: "brain query", ExitCode: 0}, nil
}

