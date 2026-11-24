package builder

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"blackfactory/pkg/bfcore"
)

func registerWritableCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			_ = os.Chmod(p, 0o700)
			return nil
		})
	})
}

func initTestGitRepo(t *testing.T, repo string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	registerWritableCleanup(t, repo)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "builder-test@localhost")
	runGit("config", "user.name", "Builder Test")
	runGit("config", "core.autocrlf", "false")
}

func TestSchemaJSONTagParity(t *testing.T) {
	t.Run("delivery_bundle_schema", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "schemas", "delivery_bundle.schema.json"))
		if err != nil {
			t.Fatalf("reading delivery_bundle.schema.json: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal delivery_bundle.schema.json: %v", err)
		}
		assertSchemaObjectMatchesType(t, doc, reflect.TypeOf(DeliveryBundle{}), "DeliveryBundle")

		defs := doc["$defs"].(map[string]any)
		assertSchemaObjectMatchesType(t, defs["git_bundle_info"].(map[string]any), reflect.TypeOf(GitBundleInfo{}), "GitBundleInfo")
		assertSchemaObjectMatchesType(t, defs["changed_file_entry"].(map[string]any), reflect.TypeOf(ChangedFileEntry{}), "ChangedFileEntry")
		assertSchemaObjectMatchesType(t, defs["scope_info"].(map[string]any), reflect.TypeOf(ScopeInfo{}), "ScopeInfo")
		assertSchemaObjectMatchesType(t, defs["bundle_brain_info"].(map[string]any), reflect.TypeOf(BundleBrainInfo{}), "BundleBrainInfo")
		assertSchemaObjectMatchesType(t, defs["brain_doc_summary"].(map[string]any), reflect.TypeOf(BrainDocSummary{}), "BrainDocSummary")
		assertSchemaObjectMatchesType(t, defs["risk_hit"].(map[string]any), reflect.TypeOf(bfcore.RiskHit{}), "RiskHit")
		assertSchemaObjectMatchesType(t, defs["red_evidence"].(map[string]any), reflect.TypeOf(RedEvidence{}), "RedEvidence")
		assertSchemaObjectMatchesType(t, defs["command_evidence"].(map[string]any), reflect.TypeOf(CommandEvidence{}), "CommandEvidence")
		assertSchemaObjectMatchesType(t, defs["tdd_evidence"].(map[string]any), reflect.TypeOf(TDDEvidence{}), "TDDEvidence")
		assertSchemaObjectMatchesType(t, defs["axis_applicability"].(map[string]any), reflect.TypeOf(AxisApplicability{}), "AxisApplicability")
		assertSchemaObjectMatchesType(t, defs["reviewer_applicability"].(map[string]any), reflect.TypeOf(ReviewerApplicability{}), "ReviewerApplicability")
	})

	t.Run("bounce_order_schema", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "schemas", "bounce_order.schema.json"))
		if err != nil {
			t.Fatalf("reading bounce_order.schema.json: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal bounce_order.schema.json: %v", err)
		}
		assertSchemaObjectMatchesType(t, doc, reflect.TypeOf(BounceOrder{}), "BounceOrder")

		defs := doc["$defs"].(map[string]any)
		assertSchemaObjectMatchesType(t, defs["axis_verdict"].(map[string]any), reflect.TypeOf(AxisVerdict{}), "AxisVerdict")
		assertSchemaObjectMatchesType(t, defs["reviewer_verdicts"].(map[string]any), reflect.TypeOf(ReviewerVerdicts{}), "ReviewerVerdicts")
		assertSchemaObjectMatchesType(t, defs["remediation_item"].(map[string]any), reflect.TypeOf(RemediationItem{}), "RemediationItem")
	})
}

func assertSchemaObjectMatchesType(t *testing.T, schemaObj map[string]any, typ reflect.Type, label string) {
	t.Helper()
	if addProps, ok := schemaObj["additionalProperties"].(bool); !ok || addProps {
		t.Fatalf("%s: additionalProperties must be false", label)
	}
	props, ok := schemaObj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s: missing properties map", label)
	}
	structTags := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			structTags[name] = true
		}
	}
	for k := range props {
		if !structTags[k] {
			t.Errorf("%s: schema property %q not found on Go struct", label, k)
		}
	}
	for k := range structTags {
		if _, exists := props[k]; !exists {
			t.Errorf("%s: Go struct JSON tag %q missing in schema", label, k)
		}
	}
}

func TestModelRetryabilityAndCanonicalBundleSHA256(t *testing.T) {
	if !IsRetryableReason(ExitFixable, ReasonScopeViolation) {
		t.Fatalf("expected ScopeViolation to be retryable")
	}
	if IsRetryableReason(ExitG4, ReasonG4Violation) {
		t.Fatalf("expected G4Violation not retryable")
	}
	if !IsRetryableReason(ExitInfraOrRefusal, ReasonBrainUnavailable) {
		t.Fatalf("expected BrainUnavailable to be retryable")
	}
	if IsRetryableReason(ExitInfraOrRefusal, ReasonSessionStateCorrupt) {
		t.Fatalf("expected SessionStateCorrupt not retryable")
	}
	if r, code := failResult(ExitUsage, ReasonInvalidUsage, "trc-1", "t1", nil); code != ExitUsage || r.OK || r.Retryable {
		t.Fatalf("unexpected failResult: %+v (%d)", r, code)
	}

	b1 := DeliveryBundle{
		SchemaVersion:   SchemaVersion,
		BundleID:        "BND-t1-01_core-a1-r0-c0",
		PreviousBounces: nil,
		Scope:           ScopeInfo{CanModify: nil, ReadOnly: nil, Forbidden: nil},
		BrainContext:    BundleBrainInfo{RiskHits: []bfcore.RiskHit{{ID: "RZ-001", MatchedPaths: nil}}},
	}
	b3 := DeliveryBundle{
		SchemaVersion:   SchemaVersion,
		BundleID:        "BND-t1-01_core-a1-r0-c0",
		PreviousBounces: []string{},
		Scope:           ScopeInfo{CanModify: []string{}, ReadOnly: []string{}, Forbidden: []string{}},
		BrainContext:    BundleBrainInfo{MatchedDocs: []BrainDocSummary{}, RiskHits: []bfcore.RiskHit{{ID: "RZ-001", MatchedPaths: []string{}}}},
		TDDEvidence:     TDDEvidence{Green: []CommandEvidence{}},
	}
	h1, err := CanonicalBundleSHA256(b1)
	if err != nil {
		t.Fatalf("CanonicalBundleSHA256(b1): %v", err)
	}
	h3, err := CanonicalBundleSHA256(b3)
	if err != nil {
		t.Fatalf("CanonicalBundleSHA256(b3): %v", err)
	}
	if len(h1) != 64 || h1 != h3 {
		t.Fatalf("canonical hashes should match and be 64-hex: %q vs %q", h1, h3)
	}
}

func TestStateG4LocksSessionLoadAndEvents(t *testing.T) {
	tmp := t.TempDir()

	// Unicode escape bypass must be caught in Fase-B
	unicodeEscape := []byte(`{"summary":"tok ` + `\u0067hp_1234567890abcdefghij123456` + `"}`)
	findings, jsonErr := ScanRawAndDecodedG4(unicodeEscape)
	if jsonErr != nil {
		t.Fatalf("expected nil jsonErr, got %v", jsonErr)
	}
	if len(findings) == 0 || findings[0].Rule != "known_token" {
		t.Fatalf("expected known_token in Fase-B, got %+v", findings)
	}
	if _, jErr := ScanRawAndDecodedG4([]byte(`{bad json`)); jErr == nil {
		t.Fatalf("expected jsonErr for malformed JSON")
	}

	// WriteJSONAtomicG4 rejects G4 violations cleanly with zero temp files left
	badPath := filepath.Join(tmp, "bad.json")
	if err := WriteJSONAtomicG4(badPath, map[string]string{"k": "ghp_" + "1234567890abcdefghij123456"}); !errors.Is(err, ErrG4Violation) {
		t.Fatalf("expected ErrG4Violation, got %v", err)
	}
	if _, err := os.Lstat(badPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bad.json should not exist")
	}

	// ComputePathIdentity avoids personal user path in basename
	base, sha, _, err := ComputePathIdentity(filepath.Join(tmp, "repo"))
	if err != nil || len(sha) != 64 || base != "repo" {
		t.Fatalf("unexpected identity: base=%q sha=%q err=%v", base, sha, err)
	}
	pBase, _, _, err := ComputePathIdentity(`C:\` + `Users\alice`)
	if err != nil || pBase != "dir" {
		t.Fatalf("expected sanitized basename 'dir', got %q (err=%v)", pBase, err)
	}

	// Locks & LoadSessionStrict
	taskStateDir := filepath.Join(tmp, "runtime", "task_1")
	if _, err := LoadSessionStrict(taskStateDir); !errors.Is(err, ErrSessionMissing) {
		t.Fatalf("expected ErrSessionMissing, got %v", err)
	}
	sLock, err := AcquireSessionLock(taskStateDir)
	if err != nil {
		t.Fatalf("AcquireSessionLock: %v", err)
	}
	rLock, err := AcquireRepoLock(filepath.Join(tmp, "runtime"), sha)
	if err != nil {
		t.Fatalf("AcquireRepoLock: %v", err)
	}
	_ = rLock.Unlock()
	_ = sLock.Unlock()

	validSession := SessionState{
		SchemaVersion:      SchemaVersion,
		TaskID:             "task_1",
		BuildBranch:        "build/task_1",
		RepoRootSHA256:     sha,
		WorktreePathSHA256: sha,
		ActiveAttempt:      1,
		WorktreeStatus:     "active",
	}
	if err := WriteJSONAtomicG4(filepath.Join(taskStateDir, "session.json"), validSession); err != nil {
		t.Fatalf("WriteJSONAtomicG4(session): %v", err)
	}
	loaded, err := LoadSessionStrict(taskStateDir)
	if err != nil || loaded.TaskID != "task_1" {
		t.Fatalf("LoadSessionStrict failed: %+v (%v)", loaded, err)
	}

	// Unknown field or trailing JSON object in session.json must fail strict parsing
	_ = WriteFileAtomicBytes(filepath.Join(taskStateDir, "session.json"), []byte(`{"schema_version":"1.0","task_id":"task_1","extra":1}`))
	if _, err := LoadSessionStrict(taskStateDir); !errors.Is(err, ErrSessionCorrupt) {
		t.Fatalf("expected ErrSessionCorrupt on unknown field, got %v", err)
	}
	trailingPayload := `{"schema_version":"1.0","task_id":"task_1","build_branch":"build/task_1","repo_root_sha256":"` + sha + `","worktree_path_sha256":"` + sha + `","active_attempt":1,"worktree_status":"active"}{"extra":1}`
	_ = WriteFileAtomicBytes(filepath.Join(taskStateDir, "session.json"), []byte(trailingPayload))
	if _, err := LoadSessionStrict(taskStateDir); !errors.Is(err, ErrSessionCorrupt) {
		t.Fatalf("expected ErrSessionCorrupt on trailing JSON object, got %v", err)
	}

	// AppendEvent sanitizes Tier 2 and Tier 1
	dataDir := filepath.Join(tmp, "runtime")
	err = AppendEvent(dataDir, BuilderEvent{
		Event:   "TEST_EV",
		TraceID: "trc-1",
		TaskID:  "task_1",
		Reason:  ReasonOK,
		Detail:  `path C:\` + `Users\alice\repo tok ghp_` + `1234567890abcdefghij123456`,
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	evRaw, err := ReadFileWithShortRetry(filepath.Join(dataDir, "builder_events.jsonl"))
	if err != nil {
		t.Fatalf("ReadFileWithShortRetry: %v", err)
	}
	if strings.Contains(string(evRaw), "alice") || strings.Contains(string(evRaw), "ghp_") {
		t.Fatalf("expected G4 redaction in event log, got %s", string(evRaw))
	}
}

func TestTaskParserScopeRingStateChunksAndCompat(t *testing.T) {
	if !IsValidTaskID("bf_03_builder_v1") || IsValidTaskID("CON") || IsValidTaskID("A") {
		t.Fatalf("unexpected IsValidTaskID behavior")
	}
	if tr := DeterministicFallbackTraceID("bf_demo", "abc123"); !strings.HasPrefix(tr, "trc-builder-bf_demo-") {
		t.Fatalf("unexpected fallback trace id: %s", tr)
	}

	sc := Scope{
		CanModify: []string{"3_Builder/engine"},
		ReadOnly:  []string{"3_Builder/engine/readonly.go"},
		Forbidden: []string{"3_Builder/engine/forbidden"},
	}
	if !sc.Allows("3_Builder/engine/model.go") {
		t.Fatalf("expected model.go allowed")
	}
	if sc.Allows("3_Builder/Engine/model.go") {
		t.Fatalf("Scope.Allows must be case-sensitive like ring.go")
	}
	if sc.Allows("3_Builder/engine/readonly.go") || sc.Allows("3_Builder/engine/forbidden/x.go") {
		t.Fatalf("ReadOnly and Forbidden must override CanModify")
	}
	chunkSc := Scope{CanModify: []string{"3_Builder/engine/model.go"}}
	if viol := ScopeViolations(sc, chunkSc, []string{"3_Builder/engine/state.go"}); len(viol) != 1 {
		t.Fatalf("expected chunk narrowing violation, got %v", viol)
	}

	tmp := t.TempDir()
	idxPath := filepath.Join(tmp, "index.md")
	idxContent := `# Mapa de Tarea: bf_demo — Demo
Plantilla: modulo_nuevo v1

## Directorio de Destino Real
- **Repositorio destino:** C:\Black Factory
- **Rama destino:** feat/3-builder

## Alcance
- Puede modificar: ` + "`3_Builder/`" + `
- Solo lectura: ` + "`Docs/`" + `
- Prohibido: ` + "`GengiSWAP/`" + `

## Notas Post-Merge y Trazabilidad
- **WorkItem:** ` + "`WI-abcdef123456`" + ` (rev 2)
- **Trace ID:** ` + "`trc-20261124-11223344`" + `
- **Carril:** ` + "`FTD`" + `
- **Spec SHA-256:** ` + "`0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`" + `
`
	if err := os.WriteFile(idxPath, []byte(idxContent), 0o644); err != nil {
		t.Fatal(err)
	}
	wtTarget := filepath.Join(tmp, "worktrees", "bf_demo")
	if err := BindIndexTargetRepo(idxPath, wtTarget); err != nil {
		t.Fatalf("BindIndexTargetRepo: %v", err)
	}
	gotRepo, err := ExtractIndexTargetRepo(idxPath)
	if err != nil || !strings.EqualFold(filepath.Clean(gotRepo), filepath.Clean(wtTarget)) {
		t.Fatalf("ExtractIndexTargetRepo = %q, want %q (err=%v)", gotRepo, wtTarget, err)
	}
	meta, err := ParseIndexFile(tmp)
	if err != nil {
		t.Fatalf("ParseIndexFile: %v", err)
	}
	if meta.WorkItemID != "WI-abcdef123456" || meta.TraceID != "trc-20261124-11223344" || meta.TargetBranch != "feat/3-builder" {
		t.Fatalf("unexpected parsed metadata: %+v", meta)
	}

	// Chunks & .ring state loading
	chunksDir := filepath.Join(tmp, "chunks")
	if err := os.MkdirAll(chunksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chunkMD := `# Chunk 01: Core
## Objetivo
Construir nucleo
## Puede modificar
- ` + "`3_Builder/engine/model.go`" + `
## Fuera de Alcance
- ` + "`GengiSWAP/`" + `
## Criterios de Aceptación
- Tests verdes
## Comando de Verificación
` + "```cmd\nset CGO_ENABLED=0&& go test ./...\n```" + `
## Prueba de que puede fallar
Falla si falta model.go
`
	if err := os.WriteFile(filepath.Join(chunksDir, "01_core.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}
	parsedChunks, err := ParseChunksDir(tmp, wtTarget)
	if err != nil || len(parsedChunks) != 1 || len(parsedChunks[0].VerifyCommands) != 1 {
		t.Fatalf("ParseChunksDir failed: %+v (%v)", parsedChunks, err)
	}

	if st, err := LoadRingState(tmp); st != nil || err != nil {
		t.Fatalf("expected nil ring state before plan, got %+v (%v)", st, err)
	}
	_ = os.MkdirAll(filepath.Join(tmp, ".ring"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".ring", "base_ref"), []byte("0123456789abcdef0123456789abcdef01234567\n"), 0o644)
	_ = os.WriteFile(filepath.Join(tmp, ".ring", "state.json"), []byte(`{"task_id":"bf_demo","plan_approved":true,"status":"RUNNING","index":0}`), 0o644)
	if ref, err := LoadRingBaseRef(tmp); err != nil || ref != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("LoadRingBaseRef: %q (%v)", ref, err)
	}
	if st, err := LoadRingState(tmp); err != nil || st == nil || !st.PlanApproved {
		t.Fatalf("LoadRingState: %+v (%v)", st, err)
	}

	// VerifyCommands compatibility check against untracked Option i paths (incl. ./, .\, ../, and absolute)
	badCmds := []string{
		`0_Brain\brain.exe query --q test`,
		`.\0_Brain\brain.exe query --q test`,
		`./0_Brain/brain.exe query --q test`,
		`../0_Brain/brain.exe query --q test`,
		`C:\Black Factory\0_Brain\brain.exe query --q test`,
	}
	for _, badCmd := range badCmds {
		chunks := []RingChunk{{
			ID:             "01_step",
			VerifyCommands: []string{badCmd},
		}}
		ptr, msg := ValidateVerifyCommandsWorktreeCompat(chunks, map[string]bool{"3_Builder/engine/model.go": true})
		if ptr == "" || msg == "" {
			t.Fatalf("expected incompatibility for untracked Option i command %q", badCmd)
		}
	}
	chunksOK := []RingChunk{{
		ID:             "01_step",
		VerifyCommands: []string{`set CGO_ENABLED=0&& cd /d "3_Builder\engine" && go test ./...`},
	}}
	if ptr2, _ := ValidateVerifyCommandsWorktreeCompat(chunksOK, map[string]bool{}); ptr2 != "" {
		t.Fatalf("unexpected compat error: %s", ptr2)
	}
}

func TestGitWorktreeRawBytesModesRenamesWorktreeOpsAndDiffG4(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	initTestGitRepo(t, repo)

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	tracked := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("ctx1\nctx2\nctx3\njdoe"+"@example.com\nline5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	renameSrc := filepath.Join(repo, "old name.txt")
	if err := os.WriteFile(renameSrc, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(repo, "add", ".")
	runGit(repo, "commit", "-m", "init")

	gc := ExecGitClient{}
	ctx := context.Background()
	baseRef, err := gc.RevParse(ctx, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := gc.CheckBranchFormat(ctx, repo, "build/demo_1"); err != nil {
		t.Fatal(err)
	}
	if exists, err := gc.BranchExists(ctx, repo, "build/demo_1"); err != nil || exists {
		t.Fatalf("BranchExists expected false, got %v (%v)", exists, err)
	}
	if paths, err := gc.LsTreePaths(ctx, repo, baseRef); err != nil || !paths["tracked.txt"] {
		t.Fatalf("LsTreePaths missing tracked.txt: %+v (%v)", paths, err)
	}

	// Test real git worktree add / list / remove / prune / delete branch
	wtDir := filepath.Join(tmp, "wt_demo")
	registerWritableCleanup(t, wtDir)
	if err := gc.WorktreeAdd(ctx, repo, wtDir, "build/demo_1", baseRef); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	branches, err := gc.ListWorktreeBranches(ctx, repo)
	if err != nil || len(branches) < 2 {
		t.Fatalf("ListWorktreeBranches: %+v (%v)", branches, err)
	}

	// Raw byte sensitivity: LF vs CRLF produces DIFFERENT worktree_tree_sha256
	wtTracked := filepath.Join(wtDir, "tracked.txt")
	if err := os.WriteFile(wtTracked, []byte("ctx1\nctx2\nctx3\njdoe"+"@example.com\nline5_mod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap1, err := ComputeWorktreeSnapshot(ctx, gc, repo, wtDir, baseRef)
	if err != nil {
		t.Fatalf("snap1: %v", err)
	}
	if err := os.WriteFile(wtTracked, []byte("ctx1\r\nctx2\nctx3\njdoe"+"@example.com\nline5_mod\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap2, err := ComputeWorktreeSnapshot(ctx, gc, repo, wtDir, baseRef)
	if err != nil {
		t.Fatalf("snap2: %v", err)
	}
	if snap1.WorktreeTreeSHA256 == snap2.WorktreeTreeSHA256 {
		t.Fatalf("expected raw-byte hash difference between LF and CRLF")
	}

	// Tier 2 in context line is redacted in final diff without triggering G4 failure
	cleanDiff, finding, _, err := ScanWorktreeChangesG4(wtDir, snap1)
	if err != nil || finding != nil {
		t.Fatalf("context-only Tier 2 must be redacted cleanly, got finding=%+v err=%v", finding, err)
	}
	if strings.Contains(cleanDiff, "example.com") {
		t.Fatalf("expected Tier 2 redacted in persisted diff context")
	}

	// Added untracked UTF-16LE file with G4 secret must fail ScanWorktreeChangesG4
	utf16Path := filepath.Join(wtDir, "secret_u16.txt")
	secretStr := "tok=ghp_" + "1234567890abcdefghij123456\n"
	u16Bytes := []byte{0xFF, 0xFE}
	for _, r := range secretStr {
		u16Bytes = append(u16Bytes, byte(r), 0x00)
	}
	if err := os.WriteFile(utf16Path, u16Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	snapU16, err := ComputeWorktreeSnapshot(ctx, gc, repo, wtDir, baseRef)
	if err != nil {
		t.Fatalf("snapU16: %v", err)
	}
	_, findingU16, _, err := ScanWorktreeChangesG4(wtDir, snapU16)
	if err != nil || findingU16 == nil || findingU16.Rule != "known_token" {
		t.Fatalf("expected known_token in UTF-16LE untracked file, got finding=%+v err=%v", findingU16, err)
	}

	// Rename + Delete + Binary untracked + G4 in '+' line
	_ = os.Remove(utf16Path)
	runGit(wtDir, "mv", "old name.txt", "new_name.txt")
	if err := os.WriteFile(filepath.Join(wtDir, "bin.dat"), []byte{0x01, 0x00, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	snapRen, err := ComputeWorktreeSnapshot(ctx, gc, repo, wtDir, baseRef)
	if err != nil {
		t.Fatalf("snapRen: %v", err)
	}
	if !strings.Contains(snapRen.DiffText, "Binary files /dev/null and b/bin.dat differ") {
		t.Fatalf("expected binary marker in diff, got:\n%s", snapRen.DiffText)
	}
	hasRenamed, hasDeleted := false, false
	for _, cf := range snapRen.ChangedFiles {
		if cf.Path == "new_name.txt" && cf.Status == "renamed" {
			hasRenamed = true
		}
		if cf.Path == "old name.txt" && cf.Status == "deleted" && cf.Mode == "0000" && cf.SHA256 == "DELETED" {
			hasDeleted = true
		}
	}
	if !hasRenamed || !hasDeleted {
		t.Fatalf("expected both renamed and deleted entries, got %+v", snapRen.ChangedFiles)
	}

	// Added line '+' with Tier 2 email must fail closed in ScanWorktreeChangesG4
	if err := os.WriteFile(wtTracked, []byte("ctx1\nctx2\nctx3\njdoe"+"@example.com\nadded=bob"+"@corp.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapPlusG4, err := ComputeWorktreeSnapshot(ctx, gc, repo, wtDir, baseRef)
	if err != nil {
		t.Fatalf("snapPlusG4: %v", err)
	}
	_, findingPlus, ptrPlus, err := ScanWorktreeChangesG4(wtDir, snapPlusG4)
	if err != nil || findingPlus == nil || findingPlus.Rule != "email_address" || ptrPlus != "/diff/tracked.txt" {
		t.Fatalf("expected email_address on '+' line, got finding=%+v ptr=%q err=%v", findingPlus, ptrPlus, err)
	}

	// Cleanup worktree and branch
	if err := gc.WorktreeRemove(ctx, repo, wtDir, true); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}
	if err := gc.WorktreePrune(ctx, repo); err != nil {
		t.Fatalf("WorktreePrune: %v", err)
	}
	if err := gc.DeleteBranch(ctx, repo, "build/demo_1"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
}

func TestShellRunnerAndSanitizeCommandOutputG4(t *testing.T) {
	// Real ShellRunner execution
	sr := ShellRunner{Timeout: 5 * time.Second}
	ev, err := sr.Run(context.Background(), t.TempDir(), "echo builder_ok")
	if err != nil || ev.ExitCode != 0 || !strings.Contains(ev.Output, "builder_ok") {
		t.Fatalf("ShellRunner.Run failed: %+v (%v)", ev, err)
	}
	evFail, err := sr.Run(context.Background(), t.TempDir(), "cmd /c exit 7")
	if err != nil || evFail.ExitCode != 7 {
		t.Fatalf("expected ExitCode 7, got %+v (%v)", evFail, err)
	}

	// Tier 1 secret must fail closed
	_, _, _, f, err := SanitizeCommandOutputG4("go test ./...", "Bearer "+"secret_token_abcdef1234", MaxCommandOutputBytes)
	if err != nil || f == nil || f.Rule != "bearer_token" {
		t.Fatalf("expected Tier 1 finding, got finding=%+v err=%v", f, err)
	}

	// Output > 8000 bytes with multibyte UTF-8 + Tier 2 path must redact before truncating and stay <= 8000 bytes
	big := strings.Repeat("á", 2500) + ` C:\` + `Users\alice\temp\file.go ` + strings.Repeat("é", 2500)
	cmd, excerpt, trunc, f2, err2 := SanitizeCommandOutputG4(`cmd C:\`+`Users\alice\run`, big, MaxCommandOutputBytes)
	if err2 != nil || f2 != nil {
		t.Fatalf("unexpected error/finding: finding=%+v err=%v", f2, err2)
	}
	if !trunc || len(excerpt) > MaxCommandOutputBytes {
		t.Fatalf("expected truncated <= %d bytes, got trunc=%v len=%d", MaxCommandOutputBytes, trunc, len(excerpt))
	}
	if strings.Contains(strings.ToLower(cmd+excerpt), "alice") {
		t.Fatalf("expected Tier 2 redacted from cmd and excerpt")
	}
}

type stubGitModesClient struct {
	ExecGitClient
	rawZ       []byte
	stageModes map[string]string
	porcelainZ []byte
}

func (s stubGitModesClient) DiffRawZ(context.Context, string, string) ([]byte, error) {
	return s.rawZ, nil
}
func (s stubGitModesClient) StageModes(context.Context, string) (map[string]string, error) {
	return s.stageModes, nil
}
func (s stubGitModesClient) StatusPorcelainZ(context.Context, string) ([]byte, error) {
	return s.porcelainZ, nil
}
func (s stubGitModesClient) DiffUnified(context.Context, string, string) (string, error) {
	return "", nil
}

func TestGitWorktreeForbiddenModes120000And160000AndRawModePrecedence(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "script.sh"), []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		rawZ  []byte
		stage map[string]string
	}{
		{"symlink_raw", []byte(":100644 120000 aaaa bbbb M\x00link\x00"), map[string]string{}},
		{"submodule_stage", []byte(""), map[string]string{"sub": "160000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gc := stubGitModesClient{rawZ: tc.rawZ, stageModes: tc.stage}
			_, err := ComputeWorktreeSnapshot(ctx, gc, repo, repo, strings.Repeat("a", 40))
			if !errors.Is(err, ErrForbiddenGitEntry) {
				t.Fatalf("expected ErrForbiddenGitEntry, got %v", err)
			}
		})
	}

	// rawModes "100644" must take precedence over stageModes "100755"
	gc := stubGitModesClient{
		rawZ:       []byte(":100755 100644 aaaa bbbb M\x00script.sh\x00"),
		stageModes: map[string]string{"script.sh": "100755"},
		porcelainZ: []byte(" M script.sh\x00"),
	}
	snap, err := ComputeWorktreeSnapshot(ctx, gc, repo, repo, strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("ComputeWorktreeSnapshot: %v", err)
	}
	if len(snap.ChangedFiles) != 1 || snap.ChangedFiles[0].Mode != "0644" {
		t.Fatalf("expected rawModes 0644 precedence, got %+v", snap.ChangedFiles)
	}

	// Porcelain RD (renamed in index, deleted in worktree) marks both deleted
	gcRD := stubGitModesClient{
		porcelainZ: []byte("RD new.txt\x00old.txt\x00"),
	}
	snapRD, err := ComputeWorktreeSnapshot(ctx, gcRD, repo, repo, strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("ComputeWorktreeSnapshot(RD): %v", err)
	}
	if len(snapRD.ChangedFiles) != 2 || snapRD.ChangedFiles[0].Status != "deleted" || snapRD.ChangedFiles[1].Status != "deleted" {
		t.Fatalf("expected both RD paths marked deleted, got %+v", snapRD.ChangedFiles)
	}

	// Untracked diff helper respects mode "0755"
	uDiff, err := buildUntrackedUnifiedDiff(repo, "script.sh", "0755")
	if err != nil || !strings.Contains(uDiff, "new file mode 100755") {
		t.Fatalf("expected new file mode 100755 in untracked diff, got %q (%v)", uDiff, err)
	}
}
