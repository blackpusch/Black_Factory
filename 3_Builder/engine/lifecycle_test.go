package builder

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"blackfactory/pkg/bfcore"
)

type fakeBrainClient struct {
	preflight bfcore.BrainPreflight
	prefErr   error
	resp      bfcore.BrainContextResponse
	inv       bfcore.BrainInvocation
	ctxErr    error
	calls     int
}

func (f *fakeBrainClient) Preflight(vaultRoot, dbPath string) (bfcore.BrainPreflight, error) {
	if f.prefErr != nil {
		return bfcore.BrainPreflight{}, f.prefErr
	}
	pf := f.preflight
	if pf.VaultRoot == "" {
		pf.VaultRoot = vaultRoot
	}
	if pf.BrainDBMTime == "" {
		pf.BrainDBMTime = "2026-11-24T00:00:00Z"
	}
	return pf, nil
}

func (f *fakeBrainClient) Context(vaultRoot, dbPath, project string, files []string) (bfcore.BrainContextResponse, bfcore.BrainInvocation, error) {
	f.calls++
	if f.ctxErr != nil {
		return bfcore.BrainContextResponse{}, f.inv, f.ctxErr
	}
	inv := f.inv
	if inv.Command == "" {
		inv.Command = "0_Brain/brain.exe context --json"
	}
	return f.resp, inv, nil
}

func TestVerifyRuntimeAllPrioritiesAndCleanCheckout(t *testing.T) {
	tmp := t.TempDir()
	factory := filepath.Join(tmp, "factory")
	initTestGitRepo(t, factory)
	vault := filepath.Join(factory, "0_Brain", "vault")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factory, ".gitignore"), []byte("3_Builder/runtime/\n3_Builder/builder.exe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"GengiSWAP/engine/bridge.go",
		"GengiSWAP/engine/ring.go",
		"GengiSWAP/engine/runner.go",
		"GengiSWAP/engine/runner_windows.go",
		"GengiSWAP/engine/cmd/gengis/main.go",
	} {
		// Copy actual files from C:\Black Factory so hashes match pinned constants
		src, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading real %s: %v", rel, err)
		}
		dst := filepath.Join(factory, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dataDir := filepath.Join(factory, "3_Builder", "runtime")
	fb := &fakeBrainClient{}

	// 1. Clean checkout without runtime/ passes with 0 artifacts created
	rep, code := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    factory,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if code != ExitOK || !rep.OK || !rep.GitIgnoreOK || !rep.GengisHashesOK || !rep.BrainOK || rep.ArtifactsChecked != 0 {
		t.Fatalf("expected clean VerifyRuntime OK, got code=%d rep=%+v", code, rep)
	}
	if _, err := os.Lstat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verify must be 100%% read-only and never create dataDir")
	}

	// 2. Missing literal 3_Builder/builder.exe in .gitignore (only *.exe present) -> GITIGNORE_DRIFT
	if err := os.WriteFile(filepath.Join(factory, ".gitignore"), []byte("*.exe\n3_Builder/runtime/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repDrift, codeDrift := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    factory,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeDrift != ExitInfraOrRefusal || repDrift.Reason != ReasonGitIgnoreDrift || repDrift.GitIgnoreOK {
		t.Fatalf("expected GITIGNORE_DRIFT when literal 3_Builder/builder.exe is missing, got code=%d rep=%+v", codeDrift, repDrift)
	}
	_ = os.WriteFile(filepath.Join(factory, ".gitignore"), []byte("3_Builder/runtime/\n3_Builder/builder.exe\n"), 0o644)

	// 3. Modified GengiSWAP/engine/runner.go -> GENGIS_ENGINE_DRIFT
	runnerPath := filepath.Join(factory, "GengiSWAP", "engine", "runner.go")
	origRunner, _ := os.ReadFile(runnerPath)
	_ = os.WriteFile(runnerPath, []byte("package ring\n// tampered\n"), 0o644)
	repEngine, codeEngine := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    factory,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeEngine != ExitInfraOrRefusal || repEngine.Reason != ReasonGengisEngineDrift || repEngine.GengisHashesOK {
		t.Fatalf("expected GENGIS_ENGINE_DRIFT, got code=%d rep=%+v", codeEngine, repEngine)
	}
	_ = os.WriteFile(runnerPath, origRunner, 0o644)

	// 4. Corrupt session.json -> SESSION_STATE_CORRUPT
	taskRuntime := filepath.Join(dataDir, "t_bad")
	_ = os.MkdirAll(taskRuntime, 0o755)
	_ = os.WriteFile(filepath.Join(taskRuntime, "session.json"), []byte(`{"schema_version":"1.0"}`), 0o644)
	repSess, codeSess := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    factory,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeSess != ExitInfraOrRefusal || repSess.Reason != ReasonSessionStateCorrupt {
		t.Fatalf("expected SESSION_STATE_CORRUPT, got code=%d rep=%+v", codeSess, repSess)
	}

	// 5. G4 secret in .diff takes precedence over SESSION_STATE_CORRUPT
	_ = os.MkdirAll(filepath.Join(taskRuntime, "bundles"), 0o755)
	_ = os.WriteFile(filepath.Join(taskRuntime, "bundles", "01_a1_r0_c0.diff"), []byte("+tok=ghp_"+"1234567890abcdefghij123456\n"), 0o644)
	repG4, codeG4 := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    factory,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeG4 != ExitG4 || repG4.Reason != ReasonG4Violation || repG4.G4Issues == 0 {
		t.Fatalf("expected G4_VIOLATION priority over corrupt session, got code=%d rep=%+v", codeG4, repG4)
	}

	// 6. Brain journal active -> BRAIN_JOURNAL_ACTIVE
	_ = os.RemoveAll(dataDir)
	fbJournal := &fakeBrainClient{prefErr: bfcore.ErrBrainJournalActive}
	repJ, codeJ := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    factory,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fbJournal,
	})
	if codeJ != ExitInfraOrRefusal || repJ.Reason != ReasonBrainJournalActive {
		t.Fatalf("expected BRAIN_JOURNAL_ACTIVE, got code=%d rep=%+v", codeJ, repJ)
	}
}

func setupLifecycleTaskFixture(t *testing.T, withPlan bool) (repo, factory, taskDir, worktreeRoot, dataDir, headSHA string) {
	t.Helper()
	tmp := t.TempDir()
	factory = filepath.Join(tmp, "factory")
	repo = factory
	initTestGitRepo(t, repo)

	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pkg", "sample.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	runGit("add", ".")
	runGit("commit", "-m", "initial commit")
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	headSHA = strings.TrimSpace(string(out))

	taskDir = filepath.Join(tmp, "gengis_data", "gengis_task", "bf_life_01")
	worktreeRoot = filepath.Join(tmp, "gengis_data", "worktrees")
	dataDir = filepath.Join(tmp, "builder_runtime")
	registerWritableCleanup(t, worktreeRoot)

	if err := os.MkdirAll(filepath.Join(taskDir, "chunks"), 0o755); err != nil {
		t.Fatal(err)
	}
	idxMD := `# Mapa de Tarea: bf_life_01 — Lifecycle Test
Plantilla: modulo_nuevo v1

## Directorio de Destino Real
- **Repositorio destino:** ` + repo + `
- **Rama destino:** master

## Alcance
- Puede modificar: ` + "`pkg/`" + `
- Solo lectura: ` + "`Docs/`" + `
- Prohibido: ` + "`GengiSWAP/`" + `

## Notas Post-Merge y Trazabilidad
- **WorkItem:** ` + "`WI-112233445566`" + ` (rev 1)
- **Trace ID:** ` + "`trc-20261124-11223344`" + `
- **Carril:** ` + "`FTD`" + `
- **Spec SHA-256:** ` + "`0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`" + `
`
	if err := os.WriteFile(filepath.Join(taskDir, "index.md"), []byte(idxMD), 0o644); err != nil {
		t.Fatal(err)
	}
	chunkMD := `# Chunk 01: Sample
## Objetivo
Implementar sample
## Puede modificar
- ` + "`pkg/sample.go`" + `
## Fuera de Alcance
- ` + "`GengiSWAP/`" + `
## Criterios de Aceptación
- Tests verdes
## Comando de Verificación
` + "```cmd\ngo test ./...\n```" + `
## Prueba de que puede fallar
Falla si rompe pkg/sample.go
`
	if err := os.WriteFile(filepath.Join(taskDir, "chunks", "01_sample.md"), []byte(chunkMD), 0o644); err != nil {
		t.Fatal(err)
	}

	if withPlan {
		_ = os.MkdirAll(filepath.Join(taskDir, ".ring"), 0o755)
		_ = os.WriteFile(filepath.Join(taskDir, ".ring", "base_ref"), []byte(headSHA+"\n"), 0o644)
		rs := RingState{
			TaskID:       "bf_life_01",
			PlanApproved: true,
			Index:        0,
			Status:       "RUNNING",
			Scope:        Scope{CanModify: []string{"pkg"}},
			Chunks: []RingChunk{{
				ID:             "01_sample",
				Spec:           "Sample",
				Acceptance:     []string{"Tests verdes"},
				VerifyCommands: []string{"go test ./..."},
				FailureProof:   "Falla si rompe",
				Scope:          Scope{CanModify: []string{"pkg/sample.go"}},
				Status:         "PENDING",
			}},
		}
		b, _ := json.MarshalIndent(rs, "", "  ")
		_ = os.WriteFile(filepath.Join(taskDir, ".ring", "state.json"), b, 0o644)
	}
	return repo, factory, taskDir, worktreeRoot, dataDir, headSHA
}

func TestInitSequenceABValidationsRollbackAndCleanup(t *testing.T) {
	// 1. Sequence A (pre-plan), context fallback pre-plan, and cleanup
	repo, factory, taskDir, wtRoot, dataDir, headSHA := setupLifecycleTaskFixture(t, false)
	vault := filepath.Join(factory, "0_Brain", "vault")
	_ = os.MkdirAll(vault, 0o755)
	_ = os.WriteFile(filepath.Join(vault, "doc.md"), []byte("# Doc\n"), 0o644)

	fb := &fakeBrainClient{
		resp: bfcore.BrainContextResponse{
			RiskZonesSHA256: "rzsha",
			MatchedDocs: []bfcore.BrainDocMatch{{
				ID: "d1", RelPath: "doc.md", Type: "wiki", Status: "active", VerifiedHead: headSHA,
			}},
			RiskHits: []bfcore.RiskHit{{ID: "RZ-002", Project: "black-factory", Level: "ALTO", MatchedPaths: []string{"pkg/"}, MandatoryGate: "vet"}},
		},
	}

	// Reject overlapping --worktree-root
	resOverlap, codeOverlap := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: filepath.Join(repo, "wt_inside"),
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeOverlap != ExitUsage || resOverlap.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE on overlapping worktree-root, got %d %+v", codeOverlap, resOverlap)
	}

	// Reject untracked Option i binary in VerifyCommands before creating worktree
	badChunk := filepath.Join(taskDir, "chunks", "01_sample.md")
	origChunk, _ := os.ReadFile(badChunk)
	_ = os.WriteFile(badChunk, []byte(strings.ReplaceAll(string(origChunk), "go test ./...", `./0_Brain/brain.exe query --q test`)), 0o644)
	resOptI, codeOptI := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeOptI != ExitUsage || resOptI.Reason != ReasonTaskDirInvalid {
		t.Fatalf("expected TASK_DIR_INVALID for Option i VerifyCommand, got %d %+v", codeOptI, resOptI)
	}
	_ = os.WriteFile(badChunk, origChunk, 0o644)

	// Valid Sequence A Init (note: index.md target_branch "master" might differ if git default is "main", resolve current branch)
	curBrOut, _ := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
	curBr := strings.TrimSpace(string(curBrOut))
	resInit, codeInit := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		Project:      "black-factory",
		BaseRef:      curBr,
		BrainClient:  fb,
	})
	if codeInit != ExitOK || !resInit.OK || resInit.BuildBranch != "build/bf_life_01" {
		t.Fatalf("Init Sequence A failed: %d %+v", codeInit, resInit)
	}
	wtPath := filepath.Join(wtRoot, "bf_life_01")
	boundRepo, err := ExtractIndexTargetRepo(filepath.Join(taskDir, "index.md"))
	if err != nil || !strings.EqualFold(filepath.Clean(boundRepo), filepath.Clean(wtPath)) {
		t.Fatalf("expected index.md bound to %q, got %q (%v)", wtPath, boundRepo, err)
	}

	// Duplicate init -> SESSION_ALREADY_EXISTS
	resDup, codeDup := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeDup != ExitUsage || resDup.Reason != ReasonSessionAlreadyExists {
		t.Fatalf("expected SESSION_ALREADY_EXISTS, got %d %+v", codeDup, resDup)
	}

	// Context pre-plan (PlanApproved == false)
	ctxRes, ctxCode := Context(ContextOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
		RefreshBrain: true,
	})
	if ctxCode != ExitOK || !ctxRes.OK || ctxRes.PlanApproved || ctxRes.ChunkID != "01_sample" || ctxRes.BrainContext.HighestRiskLevel != "ALTO" {
		t.Fatalf("Context pre-plan failed: %d %+v", ctxCode, ctxRes)
	}

	// Cleanup refuses non-terminal state without --force
	resClFail, codeClFail := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
	})
	if codeClFail != ExitExternalDecision || resClFail.Reason != ReasonTaskTerminalState {
		t.Fatalf("expected TASK_TERMINAL_STATE on non-terminal cleanup, got %d %+v", codeClFail, resClFail)
	}

	// Write terminal DONE .ring/state.json and test --delete-branch guard vs safe cleanup
	_ = os.MkdirAll(filepath.Join(taskDir, ".ring"), 0o755)
	_ = os.WriteFile(filepath.Join(taskDir, ".ring", "state.json"), []byte(`{"task_id":"bf_life_01","plan_approved":true,"status":"DONE","index":1,"chunks":[{"id":"01_sample"}]}`), 0o644)
	resDelRefuse, codeDelRefuse := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		DeleteBranch: true,
	})
	if codeDelRefuse != ExitExternalDecision || resDelRefuse.Reason != ReasonTaskTerminalState {
		t.Fatalf("expected refusal for --delete-branch on DONE without --force, got %d %+v", codeDelRefuse, resDelRefuse)
	}

	// Dirty worktree rejects cleanup without --force
	_ = os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("x"), 0o644)
	resDirty, codeDirty := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
	})
	if codeDirty != ExitFixable || resDirty.Reason != ReasonWorktreeDirty {
		t.Fatalf("expected WORKTREE_DIRTY, got %d %+v", codeDirty, resDirty)
	}

	// Cleanup with --force removes worktree and preserves build/bf_life_01 branch
	resCl, codeCl := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		Force:        true,
	})
	if codeCl != ExitOK || !resCl.OK {
		t.Fatalf("Cleanup failed: %d %+v", codeCl, resCl)
	}
	if exists, _ := (ExecGitClient{}).BranchExists(context.Background(), repo, "build/bf_life_01"); !exists {
		t.Fatalf("expected build/bf_life_01 branch preserved by default cleanup")
	}
}

func TestInitSequenceBRollbackAndContextSyncTransitions(t *testing.T) {
	// Sequence B (post-plan with 0 attempts)
	repo, factory, taskDir, wtRoot, dataDir, _ := setupLifecycleTaskFixture(t, true)
	vault := filepath.Join(factory, "0_Brain", "vault")
	_ = os.MkdirAll(vault, 0o755)
	fb := &fakeBrainClient{}

	resB, codeB := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeB != ExitOK || !resB.OK {
		t.Fatalf("Init Sequence B failed: %d %+v", codeB, resB)
	}

	// Context sync detects GENGIS_DECIDE_REQUIRED when QADone && Verdict == nil
	stPath := filepath.Join(taskDir, ".ring", "state.json")
	st, _ := LoadRingState(taskDir)
	st.Chunks[0].Attempts = []RingAttempt{{N: 1, QADone: true, QAPassed: true, Verdict: nil}}
	b, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, b, 0o644)

	ctxWait, codeWait := Context(ContextOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeWait != ExitExternalDecision || ctxWait.Reason != ReasonGengisDecideRequired {
		t.Fatalf("expected GENGIS_DECIDE_REQUIRED, got %d %+v", codeWait, ctxWait)
	}

	// Simulate RETRY_CHUNK verdict -> Context syncs Attempt 2 cleanly
	st.Chunks[0].Attempts[0].Verdict = &RingVerdict{Decision: "RETRY_CHUNK"}
	b2, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, b2, 0o644)

	ctxRetry, codeRetry := Context(ContextOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeRetry != ExitOK || !ctxRetry.OK || ctxRetry.Attempt != 2 {
		t.Fatalf("expected Attempt 2 after RETRY_CHUNK, got %d %+v", codeRetry, ctxRetry)
	}

	// Test Rollback when session.json write fails (e.g., G4 violation in project name written to session.json)
	repo2, factory2, taskDir2, wtRoot2, dataDir2, _ := setupLifecycleTaskFixture(t, true)
	origIdx, _ := os.ReadFile(filepath.Join(taskDir2, "index.md"))
	resRB, codeRB := Init(InitOptions{
		TaskDir:      taskDir2,
		RepoRoot:     repo2,
		FactoryRoot:  factory2,
		DataDir:      dataDir2,
		WorktreeRoot: wtRoot2,
		VaultRoot:    filepath.Join(factory2, "0_Brain", "vault"),
		Project:      "ghp_" + "1234567890abcdefghij123456", // triggers ErrG4Violation on WriteJSONAtomicG4(session.json)
		BrainClient:  fb,
	})
	if codeRB == ExitOK || resRB.OK {
		t.Fatalf("expected failure and rollback when session.json fails G4")
	}
	afterIdx, _ := os.ReadFile(filepath.Join(taskDir2, "index.md"))
	if string(afterIdx) != string(origIdx) {
		t.Fatalf("expected index.md restored on rollback")
	}
	if _, err := os.Lstat(filepath.Join(wtRoot2, "bf_life_01")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected worktree directory removed on rollback")
	}
	if exists, _ := (ExecGitClient{}).BranchExists(context.Background(), repo2, "build/bf_life_01"); exists {
		t.Fatalf("expected build/bf_life_01 branch deleted on rollback")
	}

	// Sequence B rejects TASK_ALREADY_ACTIVE if attempts already exist
	st.Chunks[0].Attempts = []RingAttempt{{N: 1}}
	b3, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, b3, 0o644)
	_ = os.RemoveAll(dataDir) // clear prior session to test Sequence B guard directly
	_ = (ExecGitClient{}).WorktreeRemove(context.Background(), repo, filepath.Join(wtRoot, "bf_life_01"), true)
	_ = (ExecGitClient{}).DeleteBranch(context.Background(), repo, "build/bf_life_01")
	resAct, codeAct := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeAct != ExitInfraOrRefusal || resAct.Reason != ReasonTaskAlreadyActive {
		t.Fatalf("expected TASK_ALREADY_ACTIVE, got %d %+v", codeAct, resAct)
	}
}

func TestVerifyArtifactsSessionErrorsAndCleanupBranch(t *testing.T) {
	repo, factory, taskDir, wtRoot, dataDir, _ := setupLifecycleTaskFixture(t, true)
	vault := filepath.Join(factory, "0_Brain", "vault")
	_ = os.MkdirAll(vault, 0o755)
	_ = os.WriteFile(filepath.Join(factory, ".gitignore"), []byte("3_Builder/runtime/\n3_Builder/builder.exe\n"), 0o644)
	for _, rel := range []string{
		"GengiSWAP/engine/bridge.go",
		"GengiSWAP/engine/ring.go",
		"GengiSWAP/engine/runner.go",
		"GengiSWAP/engine/runner_windows.go",
		"GengiSWAP/engine/cmd/gengis/main.go",
	} {
		src, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading real %s: %v", rel, err)
		}
		dst := filepath.Join(factory, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, src, 0o644)
	}
	fb := &fakeBrainClient{}

	// VerifyRuntime checks valid + corrupt + G4 artifacts in runtime/
	goodBundle, _ := json.Marshal(DeliveryBundle{
		SchemaVersion: SchemaVersion,
		BundleID:      "bun-1",
		CreatedAt:     "2026-11-24T00:00:00Z",
		TraceID:       "tr-1",
		TaskID:        "bf_life_01",
		ChunkID:       "01_sample",
		GengisAttempt: 1,
		BounceCycle:   0,
		Git: GitBundleInfo{
			TargetBranch:       "master",
			BuildBranch:        "build/bf_life_01",
			BaseRef:            strings.Repeat("a", 40),
			WorktreeTreeSHA256: strings.Repeat("b", 64),
			ChangedFiles:       []ChangedFileEntry{},
			DiffRelPath:        "runtime/a.diff",
			DiffSHA256:         strings.Repeat("c", 64),
		},
		Scope:           ScopeInfo{CanModify: []string{}, ReadOnly: []string{}, Forbidden: []string{}},
		BrainContext:    BundleBrainInfo{MatchedDocs: []BrainDocSummary{}, RiskHits: []bfcore.RiskHit{}},
		TDDEvidence:     TDDEvidence{Green: []CommandEvidence{}},
		PreviousBounces: []string{},
	})
	_ = os.MkdirAll(filepath.Join(dataDir, "bf_life_01", "bundles"), 0o755)
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "bundles", "b1.bundle.json"), goodBundle, 0o644)
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "events.jsonl"), []byte(`{"event":"INIT"}`+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "a.diff"), []byte("+ok := true\n"), 0o644)

	repOK, codeOK := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    repo,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeOK != ExitOK || !repOK.OK || repOK.ArtifactsChecked != 3 {
		t.Fatalf("expected VerifyRuntime OK with 3 artifacts checked, got %d %+v", codeOK, repOK)
	}

	// Corrupt .order.json + corrupt JSONL -> ARTIFACT_STATE_CORRUPT (6)
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "bad.order.json"), []byte(`{"schema_version":"1.0","unknown":1}`), 0o644)
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "bad.jsonl"), []byte("{not-json\n"), 0o644)
	repBadArt, codeBadArt := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    repo,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeBadArt != ExitInfraOrRefusal || repBadArt.Reason != ReasonArtifactStateCorrupt {
		t.Fatalf("expected ARTIFACT_STATE_CORRUPT, got %d %+v", codeBadArt, repBadArt)
	}
	_ = os.Remove(filepath.Join(dataDir, "bf_life_01", "bad.order.json"))

	// Session corrupt overrides ARTIFACT_STATE_CORRUPT by priority
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "session.json"), []byte(`{"schema_version":"bad"}`), 0o644)
	repBadSess, codeBadSess := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    repo,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeBadSess != ExitInfraOrRefusal || repBadSess.Reason != ReasonSessionStateCorrupt {
		t.Fatalf("expected SESSION_STATE_CORRUPT, got %d %+v", codeBadSess, repBadSess)
	}

	// G4 in .diff / .jsonl overrides SESSION_STATE_CORRUPT and GITIGNORE_DRIFT
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "bad.diff"), []byte("+tok := \"sk-ant-"+"api03-secret\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dataDir, "bf_life_01", "bad.jsonl"), []byte(`{"note":"ghp_`+"1234567890abcdefghij123456\"}\n"), 0o644)
	repG4, codeG4 := VerifyRuntime(VerifyOptions{
		FactoryRoot: factory,
		RepoRoot:    repo,
		VaultRoot:   vault,
		DataDir:     dataDir,
		BrainClient: fb,
	})
	if codeG4 != ExitG4 || repG4.Reason != ReasonG4Violation {
		t.Fatalf("expected G4_VIOLATION priority, got %d %+v", codeG4, repG4)
	}
	_ = os.RemoveAll(filepath.Join(dataDir, "bf_life_01"))

	// NTFS junction in runtime triggers REPARSE_POINT_DETECTED (6) when mklink is available
	if filepath.Separator == '\\' {
		targetDir := t.TempDir()
		juncPath := filepath.Join(dataDir, "junc_probe")
		if err := exec.Command("cmd", "/c", "mklink", "/J", juncPath, targetDir).Run(); err == nil {
			t.Cleanup(func() { _ = os.Remove(juncPath) })
			repReparse, codeReparse := VerifyRuntime(VerifyOptions{
				FactoryRoot: factory,
				RepoRoot:    repo,
				VaultRoot:   vault,
				DataDir:     dataDir,
				BrainClient: fb,
			})
			if codeReparse != ExitInfraOrRefusal || repReparse.Reason != ReasonReparsePointDetected {
				t.Fatalf("expected REPARSE_POINT_DETECTED on junction, got %d %+v", codeReparse, repReparse)
			}
			_ = os.Remove(juncPath)
		}
	}

	// LoadAndVerifySessionAndWorktree negative paths
	if _, r, c := LoadAndVerifySessionAndWorktree("", repo, factory, dataDir, wtRoot, vault, true); c != ExitUsage || r.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE on empty taskDir, got %d %+v", c, r)
	}
	if _, r, c := LoadAndVerifySessionAndWorktree(taskDir, repo, factory, dataDir, wtRoot, vault, true); c != ExitUsage || r.Reason != ReasonSessionNotFound {
		t.Fatalf("expected SESSION_NOT_FOUND, got %d %+v", c, r)
	}

	// Valid Init -> WORKTREE_PATH_MISMATCH, LOCK_BUSY, active bounce in Context, WORKTREE_DIRTY, Cleanup --delete-branch
	resInit, codeInit := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if codeInit != ExitOK || !resInit.OK {
		t.Fatalf("Init failed: %d %+v", codeInit, resInit)
	}

	otherWtRoot := filepath.Join(t.TempDir(), "other_wt")
	if _, r, c := LoadAndVerifySessionAndWorktree(taskDir, repo, factory, dataDir, otherWtRoot, vault, true); c != ExitInfraOrRefusal || r.Reason != ReasonWorktreePathMismatch {
		t.Fatalf("expected WORKTREE_PATH_MISMATCH, got %d %+v", c, r)
	}

	lk, lkErr := AcquireSessionLock(filepath.Join(dataDir, "bf_life_01"))
	if lkErr != nil {
		t.Fatalf("AcquireSessionLock: %v", lkErr)
	}
	if _, r, c := LoadAndVerifySessionAndWorktree(taskDir, repo, factory, dataDir, wtRoot, vault, true); c != ExitInfraOrRefusal || r.Reason != ReasonLockBusy {
		_ = lk.Unlock()
		t.Fatalf("expected LOCK_BUSY, got %d %+v", c, r)
	}
	_ = lk.Unlock()

	// Seed an active bounce order and verify Context loads ActiveBounce
	bouncesDir := filepath.Join(dataDir, "bf_life_01", "bounces")
	_ = os.MkdirAll(bouncesDir, 0o755)
	boBytes, _ := json.Marshal(BounceOrder{
		SchemaVersion: SchemaVersion,
		OrderID:       "ord-1",
		CreatedAt:     "2026-11-24T00:00:00Z",
		TraceID:       resInit.TraceID,
		TaskID:        "bf_life_01",
		ChunkID:       "01_sample",
		BundleID:      "bun-1",
		BundleSHA256:  strings.Repeat("d", 64),
		Decision:      "BOUNCE",
		RemediationItems: []RemediationItem{{
			ID:             "F1",
			Axis:           "security",
			Severity:       "MAJOR",
			File:           "pkg/sample.go",
			Line:           1,
			Description:    "fix edge case",
			RequiredAction: "add guard",
		}},
		Summary: "bounce once",
	})
	_ = os.WriteFile(filepath.Join(bouncesDir, "ord-1.order.json"), boBytes, 0o644)
	sessPath := filepath.Join(dataDir, "bf_life_01", "session.json")
	sess, _ := LoadSessionStrict(filepath.Join(dataDir, "bf_life_01"))
	sess.ActiveBounceOrderID = "ord-1"
	_ = WriteJSONAtomicG4(sessPath, sess)

	ctxRes, ctxCode := Context(ContextOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if ctxCode != ExitOK || ctxRes.ActiveBounce == nil || ctxRes.ActiveBounce.OrderID != "ord-1" {
		t.Fatalf("expected Context to load ActiveBounce ord-1, got %d %+v", ctxCode, ctxRes)
	}

	// Dirty worktree triggers WORKTREE_DIRTY on non-forced Cleanup after DONE
	ringPath := filepath.Join(taskDir, ".ring", "state.json")
	stRaw, _ := os.ReadFile(ringPath)
	var st RingState
	_ = json.Unmarshal(stRaw, &st)
	st.Status = "DONE"
	st.Chunks[0].Status = "DONE"
	stDone, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(ringPath, stDone, 0o644)

	_ = os.WriteFile(filepath.Join(wtRoot, "bf_life_01", "dirty.txt"), []byte("uncommitted\n"), 0o644)
	clDirty, clDirtyCode := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
	})
	if clDirtyCode != ExitFixable || clDirty.Reason != ReasonWorktreeDirty {
		t.Fatalf("expected WORKTREE_DIRTY, got %d %+v", clDirtyCode, clDirty)
	}

	// Force cleanup preserves branch first, then second call with DeleteBranch+Force deletes branch too
	clForce, clForceCode := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		Force:        true,
	})
	if clForceCode != ExitOK || !clForce.OK {
		t.Fatalf("forced Cleanup failed: %d %+v", clForceCode, clForce)
	}
	// Worktree is removed -> LoadAndVerifySessionAndWorktree(requireActiveWorktree=true) returns WORKTREE_MISSING
	if _, r, c := LoadAndVerifySessionAndWorktree(taskDir, repo, factory, dataDir, wtRoot, vault, true); c != ExitInfraOrRefusal || r.Reason != ReasonWorktreeMissing {
		t.Fatalf("expected WORKTREE_MISSING after cleanup, got %d %+v", c, r)
	}
	clDel, clDelCode := Cleanup(CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		Force:        true,
		DeleteBranch: true,
	})
	if clDelCode != ExitOK || !clDel.OK {
		t.Fatalf("Cleanup DeleteBranch after removed failed: %d %+v", clDelCode, clDel)
	}
	if exists, _ := (ExecGitClient{}).BranchExists(context.Background(), repo, "build/bf_life_01"); exists {
		t.Fatalf("expected branch build/bf_life_01 deleted")
	}
}

func TestChunkTransitionFailClosedAndCleanupMismatchMissing(t *testing.T) {
	repo, factory, taskDir, wtRoot, dataDir, headSHA := setupLifecycleTaskFixture(t, true)
	vault := filepath.Join(factory, "0_Brain", "vault")
	_ = os.MkdirAll(vault, 0o755)
	_ = os.WriteFile(filepath.Join(vault, "doc.md"), []byte("# Doc\n"), 0o644)

	// Two-chunk ring state
	stPath := filepath.Join(taskDir, ".ring", "state.json")
	st, _ := LoadRingState(taskDir)
	st.Chunks = append(st.Chunks, RingChunk{
		ID:             "02_second",
		Spec:           "Second chunk",
		Acceptance:     []string{"OK"},
		VerifyCommands: []string{"go test ./..."},
		FailureProof:   "fails on bug",
		Scope:          Scope{CanModify: []string{"pkg/"}},
		Status:         "PENDING",
	})
	b, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, b, 0o644)

	fb := &fakeBrainClient{}
	if _, code := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	}); code != ExitOK {
		t.Fatalf("Init failed: %d", code)
	}

	// Simulate GO_NEXT to Chunk 2 while .ring/base_ref is missing -> ARTIFACT_STATE_CORRUPT (6) instead of silent fallback
	st.Chunks[0].Status = "DONE"
	st.Chunks[0].Attempts = []RingAttempt{{N: 1, QADone: true, QAPassed: true, Verdict: &RingVerdict{Decision: "GO_NEXT"}}}
	st.Index = 1
	b2, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, b2, 0o644)

	baseRefPath := filepath.Join(taskDir, ".ring", "base_ref")
	_ = os.Remove(baseRefPath)
	if ctxBadRef, codeBadRef := Context(ContextOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
	}); codeBadRef != ExitInfraOrRefusal || ctxBadRef.Reason != ReasonArtifactStateCorrupt {
		t.Fatalf("expected ARTIFACT_STATE_CORRUPT when .ring/base_ref missing on GO_NEXT, got %d %+v", codeBadRef, ctxBadRef)
	}

	// Restore .ring/base_ref, fail 0_Brain Context query -> BRAIN_UNAVAILABLE (6) fail-closed
	_ = os.WriteFile(baseRefPath, []byte(headSHA+"\n"), 0o644)
	fb.ctxErr = errors.New("brain context timed out")
	if ctxNoBrain, codeNoBrain := Context(ContextOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
	}); codeNoBrain != ExitInfraOrRefusal || ctxNoBrain.Reason != ReasonBrainUnavailable {
		t.Fatalf("expected BRAIN_UNAVAILABLE fail-closed on GO_NEXT, got %d %+v", codeNoBrain, ctxNoBrain)
	}

	// Recover 0_Brain + --refresh-brain during same chunkChanged -> single 0_Brain call, transitions to 02_second cleanly
	fb.ctxErr = nil
	callsBefore := fb.calls
	ctxNext, codeNext := Context(ContextOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, RefreshBrain: true,
	})
	if codeNext != ExitOK || !ctxNext.OK || ctxNext.ChunkID != "02_second" || ctxNext.Attempt != 1 || ctxNext.BrainContext.ChunkID != "02_second" {
		t.Fatalf("expected clean transition to 02_second attempt 1, got %d %+v", codeNext, ctxNext)
	}
	if fb.calls-callsBefore != 1 {
		t.Fatalf("expected exactly 1 0_Brain Context call when chunkChanged+RefreshBrain, got %d", fb.calls-callsBefore)
	}

	// Cleanup rejects mismatched --worktree-root / --repo-root
	otherWtRoot := filepath.Join(t.TempDir(), "other_wt")
	if clMis, codeMis := Cleanup(CleanupOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: otherWtRoot, Force: true,
	}); codeMis != ExitInfraOrRefusal || clMis.Reason != ReasonWorktreePathMismatch {
		t.Fatalf("expected WORKTREE_PATH_MISMATCH in Cleanup, got %d %+v", codeMis, clMis)
	}

	// Manually delete worktree directory -> Cleanup without --force returns WORKTREE_MISSING; with --force prunes and marks removed
	st.Status = "DONE"
	st.Chunks[1].Status = "DONE"
	b3, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, b3, 0o644)
	_ = os.RemoveAll(filepath.Join(wtRoot, "bf_life_01"))

	if clMiss, codeMiss := Cleanup(CleanupOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
	}); codeMiss != ExitInfraOrRefusal || clMiss.Reason != ReasonWorktreeMissing {
		t.Fatalf("expected WORKTREE_MISSING when dir missing without --force, got %d %+v", codeMiss, clMiss)
	}
	if clPrune, codePrune := Cleanup(CleanupOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, Force: true,
	}); codePrune != ExitOK || !clPrune.OK {
		t.Fatalf("expected forced Cleanup prune to succeed when dir already deleted, got %d %+v", codePrune, clPrune)
	}
}
