package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"blackfactory/pkg/bfcore"
)

type stubRunnerFunc func(ctx context.Context, dir, command string) (Evidence, error)

func (f stubRunnerFunc) Run(ctx context.Context, dir, command string) (Evidence, error) {
	return f(ctx, dir, command)
}

func runGitTestCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func setupWorkflowFixture(t *testing.T) (repo, factory, taskDir, wtRoot, dataDir, vault, headSHA string, fb *fakeBrainClient) {
	t.Helper()
	repo, factory, taskDir, wtRoot, dataDir, headSHA = setupLifecycleTaskFixture(t, true)
	vault = filepath.Join(factory, "0_Brain", "vault")
	if err := os.MkdirAll(filepath.Join(vault, "_meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "_meta", "risk_zones.json"), []byte(`{"schema_version":"1.0","zones":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "doc.md"), []byte("# Architecture Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fb = &fakeBrainClient{
		resp: bfcore.BrainContextResponse{
			MatchedDocs: []bfcore.BrainDocMatch{{
				ID: "DOC-1", RelPath: "doc.md", Type: "spec", Status: "active", VerifiedHead: headSHA,
			}},
		},
	}

	res, code := Init(InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repo,
		FactoryRoot:  factory,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vault,
		BrainClient:  fb,
	})
	if code != ExitOK || !res.OK {
		t.Fatalf("setupWorkflowFixture Init failed: code=%d res=%+v", code, res)
	}
	return repo, factory, taskDir, wtRoot, dataDir, vault, headSHA, fb
}

func writeFakeGengisBinary(t *testing.T) string {
	t.Helper()
	srcDir := t.TempDir()
	binName := "gengis"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(srcDir, binName)
	src := `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RingAttempt struct {
	N         int    ` + "`json:\"n\"`" + `
	CommitRef string ` + "`json:\"commit_ref,omitempty\"`" + `
	QADone    bool   ` + "`json:\"qa_done\"`" + `
	QAPassed  bool   ` + "`json:\"qa_passed\"`" + `
	QANotes   string ` + "`json:\"qa_notes,omitempty\"`" + `
}

type RingChunk struct {
	ID       string        ` + "`json:\"id\"`" + `
	Status   string        ` + "`json:\"status\"`" + `
	Attempts []RingAttempt ` + "`json:\"attempts\"`" + `
}

type RingState struct {
	Index  int         ` + "`json:\"index\"`" + `
	Status string      ` + "`json:\"status\"`" + `
	Chunks []RingChunk ` + "`json:\"chunks\"`" + `
}

func main() {
	mode := os.Getenv("FAKE_GENGIS_MODE")
	if mode == "fail_no_update" {
		fmt.Fprintln(os.Stderr, "simulated gengis check crash")
		os.Exit(2)
	}
	if len(os.Args) < 3 || os.Args[1] != "check" {
		fmt.Fprintln(os.Stderr, "unexpected args")
		os.Exit(2)
	}
	taskDir := os.Args[2]
	wtDir := os.Getenv("FAKE_GENGIS_WT")
	repoDir := os.Getenv("FAKE_GENGIS_REPO")

	if mode == "mutate_main_repo" && repoDir != "" {
		_ = exec.Command("git", "-C", repoDir, "commit", "--allow-empty", "-m", "unexpected main commit").Run()
	}

	commitRef := ""
	if wtDir != "" && mode != "qa_fail" {
		_ = exec.Command("git", "-C", wtDir, "add", "-A").Run()
		_ = exec.Command("git", "-C", wtDir, "commit", "-m", "gengis check commit").Run()
		out, _ := exec.Command("git", "-C", wtDir, "rev-parse", "HEAD").Output()
		commitRef = strings.TrimSpace(string(out))
	}

	stPath := filepath.Join(taskDir, ".ring", "state.json")
	raw, err := os.ReadFile(stPath)
	if err != nil {
		os.Exit(2)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		os.Exit(2)
	}
	chunks := obj["chunks"].([]any)
	idx := int(obj["index"].(float64))
	ch := chunks[idx].(map[string]any)
	qaPassed := mode != "qa_fail"
	notes := "QA PASS"
	if !qaPassed {
		notes = "QA FAIL simulated"
	}
	if mode == "wrong_commit" {
		commitRef = strings.Repeat("0", 40)
	}
	attempts, _ := ch["attempts"].([]any)
	att := map[string]any{
		"n":          len(attempts) + 1,
		"commit_ref": commitRef,
		"qa_done":    true,
		"qa_passed":  qaPassed,
		"qa_notes":   notes,
	}
	attempts = append(attempts, att)
	ch["attempts"] = attempts
	outBytes, _ := json.MarshalIndent(obj, "", "  ")
	if err := os.WriteFile(stPath, outBytes, 0o644); err != nil {
		os.Exit(2)
	}
	if !qaPassed {
		os.Exit(1)
	}
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module fakegengis\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building fake gengis binary failed: %v\n%s", err, string(out))
	}
	return binPath
}

func TestTDDRedPrecedenceScopeMissingTestPassAndReExec(t *testing.T) {
	repo, factory, taskDir, wtRoot, dataDir, _, _, _ := setupWorkflowFixture(t)
	wtDir := filepath.Join(wtRoot, "bf_life_01")

	// [2] Invalid usage before lock: empty taskDir, bad timeout, non-existent session
	if res, code := TDDRed(TDDRedOptions{}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE on empty taskDir, got %d %+v", code, res)
	}
	if res, code := TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, TimeoutSec: 999}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE on timeout>300, got %d %+v", code, res)
	}

	// [6a] Lock busy
	lk, _ := AcquireSessionLock(filepath.Join(dataDir, "bf_life_01"))
	if res, code := TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot}); code != ExitInfraOrRefusal || res.Reason != ReasonLockBusy {
		_ = lk.Unlock()
		t.Fatalf("expected LOCK_BUSY, got %d %+v", code, res)
	}
	_ = lk.Unlock()

	// [1] TDD_RED_NO_CHANGES when worktree clean
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			t.Fatalf("runner must not run before diff/G4 checks")
			return Evidence{}, nil
		}),
	}); code != ExitFixable || res.Reason != ReasonTDDRedNoChanges {
		t.Fatalf("expected TDD_RED_NO_CHANGES, got %d %+v", code, res)
	}

	// [3] G4 fail-closed BEFORE running VerifyCommands
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar tok = \"ghp_"+"1234567890abcdefghij123456\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\n"), 0o644)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			t.Fatalf("runner must NOT run when G4 fails")
			return Evidence{}, nil
		}),
	}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("expected G4_VIOLATION before runner, got %d %+v", code, res)
	}

	// [1] SCOPE_VIOLATION when file outside chunk scope is created (e.g. forbidden/unscoped file)
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nfunc OK() int { return 1 }\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wtDir, "outside.txt"), []byte("nope\n"), 0o644)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			return Evidence{ExitCode: 1}, nil
		}),
	}); code != ExitFixable || res.Reason != ReasonScopeViolation {
		t.Fatalf("expected SCOPE_VIOLATION, got %d %+v", code, res)
	}
	_ = os.Remove(filepath.Join(wtDir, "outside.txt"))

	// Expand chunk scope to allow pkg/ (both sample.go and sample_test.go)
	stPath := filepath.Join(taskDir, ".ring", "state.json")
	st, _ := LoadRingState(taskDir)
	st.Chunks[0].Scope.CanModify = []string{"pkg/"}
	sb, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sb, 0o644)

	// Missing test file change -> TDD_RED_MISSING_TEST_CHANGE
	_ = os.Remove(filepath.Join(wtDir, "pkg", "sample_test.go"))
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			return Evidence{ExitCode: 1}, nil
		}),
	}); code != ExitFixable || res.Reason != ReasonTDDRedMissingTestChange {
		t.Fatalf("expected TDD_RED_MISSING_TEST_CHANGE, got %d %+v", code, res)
	}

	// Out-of-range --cmd-index -> CMD_INDEX_OUT_OF_RANGE (2)
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestFail(t *testing.T){ t.Fatal(\"red\") }\n"), 0o644)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, CmdIndex: 2,
	}); code != ExitUsage || res.Reason != ReasonCmdIndexOutOfRange {
		t.Fatalf("expected CMD_INDEX_OUT_OF_RANGE, got %d %+v", code, res)
	}

	// Passing VerifyCommand unexpectedly -> TDD_RED_PASSED_UNEXPECTEDLY (1)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			return Evidence{Command: "go test ./...", ExitCode: 0, Output: "PASS\n"}, nil
		}),
	}); code != ExitFixable || res.Reason != ReasonTDDRedPassedUnexpected {
		t.Fatalf("expected TDD_RED_PASSED_UNEXPECTEDLY, got %d %+v", code, res)
	}

	// Tier 1 G4 leak in VerifyCommand stdout -> G4_VIOLATION (3)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			return Evidence{Command: "go test ./...", ExitCode: 1, Output: "FAIL ghp_" + "1234567890abcdefghij123456\n"}, nil
		}),
	}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("expected G4_VIOLATION on stdout Tier 1, got %d %+v", code, res)
	}

	// Failing VerifyCommand -> OK and persisted session.TDD.Red; re-execution allowed
	for i := 0; i < 2; i++ {
		res, code := TDDRed(TDDRedOptions{
			TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
			Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
				return Evidence{Command: "go test ./...", ExitCode: 1, Output: "--- FAIL: TestFail\n", DurationMs: 12}, nil
			}),
		})
		if code != ExitOK || !res.OK {
			t.Fatalf("expected TDDRed OK on run %d, got %d %+v", i, code, res)
		}
	}
	sess, _ := LoadSessionStrict(filepath.Join(dataDir, "bf_life_01"))
	if sess.TDD.Red == nil || sess.TDD.Red.ExitCode == 0 || !sess.TDD.Red.HasTestFile || sess.TDD.Red.RedWorktreeTreeSHA256 == "" {
		t.Fatalf("expected populated session.TDD.Red, got %+v", sess.TDD.Red)
	}

	// [5] BounceBrakeActive blocks subsequent tdd-red with ExitAttemptsBlocked (5)
	sess.BounceBrakeActive = true
	_ = WriteJSONAtomicG4(filepath.Join(dataDir, "bf_life_01", "session.json"), sess)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
	}); code != ExitAttemptsBlocked || res.Reason != ReasonMaxBounceCycles {
		t.Fatalf("expected MAX_BOUNCE_CYCLES (5), got %d %+v", code, res)
	}
}

func TestPackageStaleBrainPostCommandDirtySnapshotAndApplicability(t *testing.T) {
	// Applicability matrix check across segments & overrides
	appLib := EvaluateReviewerApplicability([]ChangedFileEntry{{Path: "pkg/core.go", Status: "modified"}}, false, false)
	if !appLib.Security.Applicable || !appLib.CodeArchitecture.Applicable || appLib.UX.Applicable || appLib.UI.Applicable {
		t.Fatalf("expected only security+code for internal pkg/core.go, got %+v", appLib)
	}
	appLibIncUX := EvaluateReviewerApplicability([]ChangedFileEntry{{Path: "pkg/core.go", Status: "modified"}}, false, true)
	if !appLibIncUX.UX.Applicable || appLibIncUX.UX.Mode != "included" || appLibIncUX.UI.Applicable {
		t.Fatalf("expected included UX without UI for --include-ux, got %+v", appLibIncUX)
	}
	appLibIncUI := EvaluateReviewerApplicability([]ChangedFileEntry{{Path: "pkg/core.go", Status: "modified"}}, true, false)
	if !appLibIncUI.UI.Applicable || appLibIncUI.UI.Mode != "included" || !appLibIncUI.UX.Applicable || appLibIncUI.UX.Mode != "auto" {
		t.Fatalf("expected included UI + auto transitive UX for --include-ui, got %+v", appLibIncUI)
	}
	appUX := EvaluateReviewerApplicability([]ChangedFileEntry{{Path: "3_Builder/skills/SKILL.md", Status: "added"}}, false, false)
	if !appUX.UX.Applicable || appUX.UX.Mode != "auto" || appUX.UI.Applicable {
		t.Fatalf("expected auto UX without UI for skills/SKILL.md, got %+v", appUX)
	}
	appUI := EvaluateReviewerApplicability([]ChangedFileEntry{{Path: "web/Button.tsx", Status: "added"}}, false, false)
	if !appUI.UI.Applicable || appUI.UI.Mode != "auto" || !appUI.UX.Applicable || appUI.UX.Reason != "user_surface_detected" {
		t.Fatalf("expected auto UI + transitive auto UX for web/Button.tsx, got %+v", appUI)
	}

	repo, factory, taskDir, wtRoot, dataDir, vault, _, fb := setupWorkflowFixture(t)
	wtDir := filepath.Join(wtRoot, "bf_life_01")
	stPath := filepath.Join(taskDir, ".ring", "state.json")
	st, _ := LoadRingState(taskDir)
	st.Chunks[0].Scope.CanModify = []string{"pkg/"}
	sb, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sb, 0o644)

	// Package before TDDRed -> TDD_RED_REQUIRED (4)
	if res, code := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
	}); code != ExitExternalDecision || res.Reason != ReasonTDDRedRequired {
		t.Fatalf("expected TDD_RED_REQUIRED, got %d %+v", code, res)
	}

	// Perform TDDRed
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestX(t *testing.T){ t.Fatal(\"red\") }\n"), 0o644)
	if res, code := TDDRed(TDDRedOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) {
			return Evidence{Command: "go test ./...", ExitCode: 1, Output: "FAIL\n"}, nil
		}),
	}); code != ExitOK || !res.OK {
		t.Fatalf("TDDRed failed: %d %+v", code, res)
	}

	// Modify matched vault doc -> STALE_BRAIN_CONTEXT (4)
	_ = os.WriteFile(filepath.Join(vault, "doc.md"), []byte("# Modified Vault Doc\n"), 0o644)
	if res, code := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
	}); code != ExitExternalDecision || res.Reason != ReasonStaleBrainContext {
		t.Fatalf("expected STALE_BRAIN_CONTEXT when vault doc changes, got %d %+v", code, res)
	}
	// Restore doc.md; check JournalActive -> BRAIN_JOURNAL_ACTIVE (6)
	_ = os.WriteFile(filepath.Join(vault, "doc.md"), []byte("# Architecture Doc\n"), 0o644)
	fb.prefErr = bfcore.ErrBrainJournalActive
	if res, code := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
	}); code != ExitInfraOrRefusal || res.Reason != ReasonBrainJournalActive {
		t.Fatalf("expected BRAIN_JOURNAL_ACTIVE, got %d %+v", code, res)
	}
	fb.prefErr = nil

	// Package unchanged from Red -> TDD_GREEN_UNCHANGED_FROM_RED (1)
	if res, code := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
	}); code != ExitFixable || res.Reason != ReasonTDDGreenUnchanged {
		t.Fatalf("expected TDD_GREEN_UNCHANGED_FROM_RED, got %d %+v", code, res)
	}

	// Edit production code -> VerifyCommand creates a dirty out-of-scope file during execution -> RE-SNAPSHOT catches SCOPE_VIOLATION
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nfunc Value() int { return 42 }\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestX(t *testing.T){ if Value() != 42 { t.Fatal(\"bad\") } }\n"), 0o644)
	if res, code := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
		Runner: stubRunnerFunc(func(ctx context.Context, dir, command string) (Evidence, error) {
			_ = os.WriteFile(filepath.Join(dir, "dirty_by_test.txt"), []byte("leaked artifact\n"), 0o644)
			return Evidence{Command: command, ExitCode: 0, Output: "PASS\n"}, nil
		}),
	}); code != ExitFixable || res.Reason != ReasonScopeViolation {
		t.Fatalf("expected post-command re-snapshot to catch dirty SCOPE_VIOLATION, got %d %+v", code, res)
	}
	_ = os.Remove(filepath.Join(wtDir, "dirty_by_test.txt"))

	// Failing Green VerifyCommand -> TDD_GREEN_FAILED
	if res, code := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
		Runner: stubRunnerFunc(func(ctx context.Context, dir, command string) (Evidence, error) {
			return Evidence{Command: command, ExitCode: 1, Output: "FAIL\n"}, nil
		}),
	}); code != ExitFixable || res.Reason != ReasonTDDGreenFailed {
		t.Fatalf("expected TDD_GREEN_FAILED, got %d %+v", code, res)
	}

	// Clean Green VerifyCommand -> OK + bundle & diff persisted with CanonicalBundleSHA256
	resPkg, codePkg := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, IncludeUI: true,
		Runner: stubRunnerFunc(func(ctx context.Context, dir, command string) (Evidence, error) {
			return Evidence{Command: command, ExitCode: 0, Output: "ok pkg 0.01s\n"}, nil
		}),
	})
	if codePkg != ExitOK || !resPkg.OK || resPkg.BundleID == "" || resPkg.BundleSHA256 == "" {
		t.Fatalf("Package OK failed: %d %+v", codePkg, resPkg)
	}
	rawBundle, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(resPkg.BundlePath)))
	if err != nil {
		t.Fatalf("reading bundle: %v", err)
	}
	var bundle DeliveryBundle
	if err := json.Unmarshal(rawBundle, &bundle); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	gotHash, _ := CanonicalBundleSHA256(bundle)
	if gotHash != resPkg.BundleSHA256 || !bundle.ReviewerApplicability.UI.Applicable || bundle.ReviewerApplicability.UI.Mode != "included" {
		t.Fatalf("bundle hash/applicability mismatch: hash=%s want=%s app=%+v", gotHash, resPkg.BundleSHA256, bundle.ReviewerApplicability)
	}
}

func makeTestOrder(bundle DeliveryBundle, bundleSHA, orderID, decision string, requiresRed bool) BounceOrder {
	ord := BounceOrder{
		SchemaVersion: schemaVerConst(),
		OrderID:       orderID,
		CreatedAt:     "2026-11-24T12:00:00Z",
		TraceID:       bundle.TraceID,
		TaskID:        bundle.TaskID,
		ChunkID:       bundle.ChunkID,
		BundleID:      bundle.BundleID,
		BundleSHA256:  bundleSHA,
		Decision:      decision,
		ReviewerVerdicts: ReviewerVerdicts{
			Security:         AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"},
			CodeArchitecture: AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"},
			UX:               AxisVerdict{Status: "NA", FindingsCount: 0, Summary: "na", NAReason: "internal"},
			UI:               AxisVerdict{Status: "NA", FindingsCount: 0, Summary: "na", NAReason: "internal"},
		},
		RequiresRedTest:  requiresRed,
		RemediationItems: []RemediationItem{},
		Summary:          "clean review",
	}
	if !bundle.ReviewerApplicability.UX.Applicable {
		ord.ReviewerVerdicts.UX.NAReason = "internal library"
	} else {
		ord.ReviewerVerdicts.UX = AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"}
	}
	if bundle.ReviewerApplicability.UI.Applicable {
		ord.ReviewerVerdicts.UI = AxisVerdict{Status: "PASS", FindingsCount: 0, Summary: "ok"}
	}
	if decision == "BOUNCE" {
		ord.ReviewerVerdicts.Security = AxisVerdict{Status: "FAIL", FindingsCount: 1, Summary: "fix bug"}
		ord.RemediationItems = []RemediationItem{{
			ID: "REM-1", Axis: "security", Severity: "MAJOR", File: "pkg/sample.go", Line: 2,
			Description: "needs edge check", RequiredAction: "add guard",
		}}
		ord.Summary = "remediate 1 item"
	}
	return ord
}

func schemaVerConst() string { return SchemaVersion }

func writeOrderTemp(t *testing.T, dir, name string, v any) string {
	t.Helper()
	b, _ := json.MarshalIndent(v, "", "  ")
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyOrderApproveBounceCycleBrakeResetAndSecurity(t *testing.T) {
	repo, factory, taskDir, wtRoot, dataDir, vault, _, fb := setupWorkflowFixture(t)
	wtDir := filepath.Join(wtRoot, "bf_life_01")
	stPath := filepath.Join(taskDir, ".ring", "state.json")
	st, _ := LoadRingState(taskDir)
	st.Chunks[0].Scope.CanModify = []string{"pkg/"}
	sb, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sb, 0o644)

	orderDir := t.TempDir()
	// Mutual exclusion: both --order and --reset-bounces
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, OrderPath: "a.json", ResetBounces: true, Reason: "valid reason"}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE on conflicting flags, got %d %+v", code, res)
	}
	// Missing --order file
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: filepath.Join(orderDir, "missing.json")}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE on missing order file, got %d %+v", code, res)
	}

	// Produce Red + Bundle c0
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestS(t *testing.T){}\n"), 0o644)
	_, _ = TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 1}, nil })})
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 1\n"), 0o644)
	pkg0, code0 := Package(PackageOptions{
		TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
		Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 0}, nil }),
	})
	if code0 != ExitOK {
		t.Fatalf("pkg0 failed: %d %+v", code0, pkg0)
	}
	var b0 DeliveryBundle
	raw0, _ := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(pkg0.BundlePath)))
	_ = json.Unmarshal(raw0, &b0)

	// Invalid orders check: G4, unknown field, wrong hash, invalid summary char, invalid NA on security
	g4Path := filepath.Join(orderDir, "g4.json")
	_ = os.WriteFile(g4Path, []byte(`{"summary":"ghp_`+"1234567890abcdefghij123456\"}"), 0o644)
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: g4Path}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("expected G4_VIOLATION on order, got %d %+v", code, res)
	}
	unkPath := filepath.Join(orderDir, "unk.json")
	_ = os.WriteFile(unkPath, []byte(`{"schema_version":"1.0","extra":true}`), 0o644)
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: unkPath}); code != ExitFixable || res.Reason != ReasonOrderValidationFailed {
		t.Fatalf("expected ORDER_VALIDATION_FAILED on unknown field, got %d %+v", code, res)
	}

	badHash := makeTestOrder(b0, strings.Repeat("f", 64), "ORD-BADHASH", "APPROVE", false)
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "bh.json", badHash)}); code != ExitFixable || res.Reason != ReasonBundleMismatch {
		t.Fatalf("expected BUNDLE_MISMATCH, got %d %+v", code, res)
	}
	badNA := makeTestOrder(b0, pkg0.BundleSHA256, "ORD-BADNA", "APPROVE", false)
	badNA.ReviewerVerdicts.Security.Status = "NA"
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "bna.json", badNA)}); code != ExitFixable || res.Reason != ReasonOrderValidationFailed {
		t.Fatalf("expected ORDER_VALIDATION_FAILED on NA security, got %d %+v", code, res)
	}
	badSummary := makeTestOrder(b0, pkg0.BundleSHA256, "ORD-BADSUM", "APPROVE", false)
	badSummary.Summary = "bad\nquote\"slash\\"
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "bsum.json", badSummary)}); code != ExitFixable || res.Reason != ReasonOrderValidationFailed {
		t.Fatalf("expected ORDER_VALIDATION_FAILED on unsafe summary, got %d %+v", code, res)
	}

	// Cycle 1 BOUNCE (requires_red_test: true -> invalidates TDD.Red)
	ord1 := makeTestOrder(b0, pkg0.BundleSHA256, "ORD-B1", "BOUNCE", true)
	p1 := writeOrderTemp(t, orderDir, "b1.json", ord1)
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: p1}); code != ExitOK || res.BounceCycle != 1 || res.Decision != "BOUNCE" {
		t.Fatalf("BOUNCE 1 failed: %d %+v", code, res)
	}
	// Replay on same bundle fails with ORDER_VALIDATION_FAILED
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "b1_dup.json", makeTestOrder(b0, pkg0.BundleSHA256, "ORD-B1-OTHER", "BOUNCE", false))}); code != ExitFixable || res.Reason != ReasonOrderValidationFailed {
		t.Fatalf("expected replay rejection on already bounced bundle, got %d %+v", code, res)
	}
	// Package without new Red fails with TDD_RED_REQUIRED
	if res, code := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb}); code != ExitExternalDecision || res.Reason != ReasonTDDRedRequired {
		t.Fatalf("expected TDD_RED_REQUIRED after requires_red_test bounce, got %d %+v", code, res)
	}

	// Cycle 2 via new TDDRed + edit + Package + BOUNCE (requires_red_test: false -> preserves Red)
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestS(t *testing.T){ /* c1 */ }\n"), 0o644)
	_, _ = TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 1}, nil })})
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 2\n"), 0o644)
	pkg1, _ := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 0}, nil })})
	var b1 DeliveryBundle
	raw1, _ := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(pkg1.BundlePath)))
	_ = json.Unmarshal(raw1, &b1)

	// Duplicate order_id ("ORD-B1") on newer bundle -> ORDER_VALIDATION_FAILED
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "dup_id.json", makeTestOrder(b1, pkg1.BundleSHA256, "ORD-B1", "BOUNCE", false))}); code != ExitFixable || res.Reason != ReasonOrderValidationFailed {
		t.Fatalf("expected duplicate order_id rejection, got %d %+v", code, res)
	}

	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "b2.json", makeTestOrder(b1, pkg1.BundleSHA256, "ORD-B2", "BOUNCE", false))}); code != ExitOK || res.BounceCycle != 2 {
		t.Fatalf("BOUNCE 2 failed: %d %+v", code, res)
	}
	sess2, _ := LoadSessionStrict(filepath.Join(dataDir, "bf_life_01"))
	if sess2.TDD.Red == nil || sess2.TDD.Red.BounceCycle != 2 || sess2.TDD.Red.InheritedFromCycle != 1 {
		t.Fatalf("expected preserved Red with BounceCycle=2 InheritedFromCycle=1, got %+v", sess2.TDD.Red)
	}

	// Unchanged Package after requires_red_test:false BOUNCE -> TDD_GREEN_UNCHANGED_FROM_RED
	if res, code := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb}); code != ExitFixable || res.Reason != ReasonTDDGreenUnchanged {
		t.Fatalf("expected TDD_GREEN_UNCHANGED_FROM_RED against LastBouncedWorktreeTreeSHA256, got %d %+v", code, res)
	}

	// Cycle 3 & 4th BOUNCE brake
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 3\n"), 0o644)
	pkg2, _ := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 0}, nil })})
	var b2 DeliveryBundle
	raw2, _ := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(pkg2.BundlePath)))
	_ = json.Unmarshal(raw2, &b2)
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "b3.json", makeTestOrder(b2, pkg2.BundleSHA256, "ORD-B3", "BOUNCE", false))}); code != ExitOK || res.BounceCycle != 3 {
		t.Fatalf("BOUNCE 3 failed: %d %+v", code, res)
	}

	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 4\n"), 0o644)
	pkg3, _ := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 0}, nil })})
	var b3 DeliveryBundle
	raw3, _ := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(pkg3.BundlePath)))
	_ = json.Unmarshal(raw3, &b3)
	resB4, codeB4 := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "b4.json", makeTestOrder(b3, pkg3.BundleSHA256, "ORD-B4", "BOUNCE", false))})
	if codeB4 != ExitAttemptsBlocked || resB4.Reason != ReasonMaxBounceCycles {
		t.Fatalf("expected MAX_BOUNCE_CYCLES (5) on 4th bounce, got %d %+v", codeB4, resB4)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "bf_life_01", "bounces", "01_sample_a1_r0_b4_brake.order.json")); err != nil {
		t.Fatalf("expected b4_brake order archived: %v", err)
	}

	// --reset-bounces validation & unlock (ResetEpoch=1)
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, ResetBounces: true, Reason: "short"}); code != ExitUsage || res.Reason != ReasonInvalidUsage {
		t.Fatalf("expected INVALID_USAGE for short reset reason, got %d %+v", code, res)
	}
	if res, code := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, ResetBounces: true, Reason: "reset ghp_" + "1234567890abcdefghij123456"}); code != ExitG4 || res.Reason != ReasonG4Violation {
		t.Fatalf("expected G4_VIOLATION for G4 reset reason, got %d %+v", code, res)
	}
	resRst, codeRst := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, ResetBounces: true, Reason: "AUTOR approved scope clarification reset"})
	if codeRst != ExitOK || !resRst.Reset {
		t.Fatalf("reset bounces failed: %d %+v", codeRst, resRst)
	}

	// Re-run Red + Package at ResetEpoch 1 + APPROVE
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestS(t *testing.T){ /* r1 */ }\n"), 0o644)
	_, _ = TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 1}, nil })})
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 5\n"), 0o644)
	pkgR1, codeR1 := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 0}, nil })})
	if codeR1 != ExitOK {
		t.Fatalf("pkgR1 failed: %d %+v", codeR1, pkgR1)
	}
	var br1 DeliveryBundle
	rawR1, _ := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(pkgR1.BundlePath)))
	_ = json.Unmarshal(rawR1, &br1)
	resApp, codeApp := ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, orderDir, "app.json", makeTestOrder(br1, pkgR1.BundleSHA256, "ORD-APP", "APPROVE", false))})
	if codeApp != ExitOK || !resApp.OK || resApp.Decision != "APPROVE" {
		t.Fatalf("APPROVE failed: %d %+v", codeApp, resApp)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "bf_life_01", "bounces", "01_sample_a1_r1_c0_approve.order.json")); err != nil {
		t.Fatalf("expected approve order archived under r1_c0: %v", err)
	}
}

func TestCommitAntiBypassCRLFDriftAndGengisCheckReconciliation(t *testing.T) {
	repo, factory, taskDir, wtRoot, dataDir, vault, _, fb := setupWorkflowFixture(t)
	wtDir := filepath.Join(wtRoot, "bf_life_01")
	gengisRoot := filepath.Join(factory, "GengiSWAP")
	_ = os.MkdirAll(gengisRoot, 0o755)
	fakeGengis := writeFakeGengisBinary(t)

	stPath := filepath.Join(taskDir, ".ring", "state.json")
	st, _ := LoadRingState(taskDir)
	st.Chunks[0].Scope.CanModify = []string{"pkg/"}
	sb, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sb, 0o644)

	// Commit without approval -> APPROVAL_REQUIRED (1)
	if res, code := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb}); code != ExitFixable || res.Reason != ReasonApprovalRequired {
		t.Fatalf("expected APPROVAL_REQUIRED, got %d %+v", code, res)
	}

	// Produce Red + Package (LF file) + APPROVE
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample_test.go"), []byte("package pkg\nimport \"testing\"\nfunc TestS(t *testing.T){}\n"), 0o644)
	_, _ = TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 1}, nil })})
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 10\n"), 0o644)
	pkg, _ := Package(PackageOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb, Runner: stubRunnerFunc(func(context.Context, string, string) (Evidence, error) { return Evidence{ExitCode: 0}, nil })})
	var b DeliveryBundle
	rawB, _ := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(pkg.BundlePath)))
	_ = json.Unmarshal(rawB, &b)
	_, _ = ApplyOrder(ApplyOrderOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, OrderPath: writeOrderTemp(t, t.TempDir(), "app.json", makeTestOrder(b, pkg.BundleSHA256, "ORD-C-APP", "APPROVE", false))})

	// Subtle LF -> CRLF drift after APPROVE must trigger WORKTREE_DRIFT_AFTER_APPROVAL (1)
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\r\nvar V = 10\r\n"), 0o644)
	if res, code := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb}); code != ExitFixable || res.Reason != ReasonWorktreeDriftAfterAppr {
		t.Fatalf("expected WORKTREE_DRIFT_AFTER_APPROVAL on CRLF edit, got %d %+v", code, res)
	}
	// Restore LF bytes
	_ = os.WriteFile(filepath.Join(wtDir, "pkg", "sample.go"), []byte("package pkg\nvar V = 10\n"), 0o644)

	// Missing gengis binary -> GENGIS_UNAVAILABLE (6)
	if res, code := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: filepath.Join(t.TempDir(), "missing.exe"), VaultRoot: vault, BrainClient: fb}); code != ExitInfraOrRefusal || res.Reason != ReasonGengisUnavailable {
		t.Fatalf("expected GENGIS_UNAVAILABLE on missing exe, got %d %+v", code, res)
	}

	// Fake gengis crash without updating state.json -> GENGIS_UNAVAILABLE (6)
	t.Setenv("FAKE_GENGIS_MODE", "fail_no_update")
	t.Setenv("FAKE_GENGIS_WT", wtDir)
	t.Setenv("FAKE_GENGIS_REPO", repo)
	if res, code := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb}); code != ExitInfraOrRefusal || res.Reason != ReasonGengisUnavailable {
		t.Fatalf("expected GENGIS_UNAVAILABLE on crash, got %d %+v", code, res)
	}

	// Fake gengis check reports QA FAIL in state.json -> GENGIS_DECIDE_REQUIRED (4) with NextAutorCommand
	t.Setenv("FAKE_GENGIS_MODE", "qa_fail")
	resQAFail, codeQAFail := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb})
	if codeQAFail != ExitExternalDecision || resQAFail.Reason != ReasonGengisDecideRequired || !strings.Contains(resQAFail.NextAutorCommand, "--qa-pass=false") {
		t.Fatalf("expected GENGIS_DECIDE_REQUIRED on qa_fail, got %d %+v", codeQAFail, resQAFail)
	}

	// Test wrong CommitRef returned by gengis.exe check -> GIT_ERROR (6)
	headBefore, _ := os.ReadFile(filepath.Join(taskDir, ".ring", "base_ref"))
	baseSHA := strings.TrimSpace(string(headBefore))
	st.Chunks[0].Attempts = nil
	sbWrong, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sbWrong, 0o644)
	t.Setenv("FAKE_GENGIS_MODE", "wrong_commit")
	if resW, codeW := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb}); codeW != ExitInfraOrRefusal || resW.Reason != ReasonGitError {
		t.Fatalf("expected GIT_ERROR on wrong CommitRef, got %d %+v", codeW, resW)
	}
	runGitTestCmd(t, wtDir, "reset", "--mixed", baseSHA)

	// Test UNEXPECTED_COMMIT_IN_WORKTREE in TDDRed if worktree HEAD moved outside builder commit
	st.Chunks[0].Attempts = nil
	sbU, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sbU, 0o644)
	runGitTestCmd(t, wtDir, "commit", "--allow-empty", "-m", "bypass commit")
	if resU, codeU := TDDRed(TDDRedOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot}); codeU != ExitExternalDecision || resU.Reason != ReasonUnexpectedCommitInWorktree {
		t.Fatalf("expected UNEXPECTED_COMMIT_IN_WORKTREE, got %d %+v", codeU, resU)
	}
	runGitTestCmd(t, wtDir, "reset", "--mixed", baseSHA)

	// Reset chunk attempt back to 0 so we test happy PASS path
	st.Chunks[0].Attempts = nil
	sb2, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sb2, 0o644)

	t.Setenv("FAKE_GENGIS_MODE", "pass")
	resPass, codePass := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb})
	if codePass != ExitOK || !resPass.OK || resPass.CommitRef == "" || !strings.Contains(resPass.NextAutorCommand, "--qa-pass=true") || !strings.Contains(resPass.NextAutorCommand, gengisRoot) {
		t.Fatalf("expected Commit OK with CommitRef and quoted decide cmd, got %d %+v", codePass, resPass)
	}

	// Test mutate_main_repo guard -> GIT_ERROR (6)
	runGitTestCmd(t, wtDir, "reset", "--mixed", baseSHA)
	st.Chunks[0].Attempts = nil
	sbMut, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(stPath, sbMut, 0o644)
	t.Setenv("FAKE_GENGIS_MODE", "mutate_main_repo")
	if resM, codeM := Commit(CommitOptions{TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, GengisRoot: gengisRoot, GengisBin: fakeGengis, VaultRoot: vault, BrainClient: fb}); codeM != ExitInfraOrRefusal || resM.Reason != ReasonGitError {
		t.Fatalf("expected GIT_ERROR when main repo HEAD mutates during check, got %d %+v", codeM, resM)
	}
}

func TestLatencyBudgetP95(t *testing.T) {
	repo, factory, taskDir, wtRoot, dataDir, vault, _, fb := setupWorkflowFixture(t)
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
			t.Fatalf("read %s: %v", rel, err)
		}
		dst := filepath.Join(factory, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, src, 0o644)
	}

	// Seed 50 valid runtime artifacts (25 bundles + 25 diffs)
	bundlesDir := filepath.Join(dataDir, "bf_life_01", "bundles")
	_ = os.MkdirAll(bundlesDir, 0o755)
	for i := 0; i < 25; i++ {
		diffText := fmt.Sprintf("--- a/pkg/f%d.go\n+++ b/pkg/f%d.go\n@@ -1 +1 @@\n-old\n+new\n", i, i)
		sum := sha256.Sum256([]byte(diffText))
		dPath := filepath.Join(bundlesDir, fmt.Sprintf("chunk_a1_r0_c%d.diff", i))
		bPath := filepath.Join(bundlesDir, fmt.Sprintf("chunk_a1_r0_c%d.bundle.json", i))
		_ = os.WriteFile(dPath, []byte(diffText), 0o644)
		bun := DeliveryBundle{
			SchemaVersion: SchemaVersion,
			BundleID:      fmt.Sprintf("BND-bf_life_01-01_sample-a1-r0-c%d", i),
			CreatedAt:     "2026-11-24T12:00:00Z",
			TraceID:       "trc-lat",
			TaskID:        "bf_life_01",
			ChunkID:       "01_sample",
			GengisAttempt: 1,
			BounceCycle:   0,
			Git: GitBundleInfo{
				TargetBranch: "master", BuildBranch: "build/bf_life_01",
				BaseRef: strings.Repeat("a", 40), WorktreeTreeSHA256: strings.Repeat("b", 64),
				ChangedFiles: []ChangedFileEntry{}, DiffRelPath: fmt.Sprintf("bf_life_01/bundles/chunk_a1_r0_c%d.diff", i),
				DiffSHA256: hex.EncodeToString(sum[:]), DiffBytes: int64(len(diffText)),
			},
			Scope:           ScopeInfo{CanModify: []string{}, ReadOnly: []string{}, Forbidden: []string{}},
			BrainContext:    BundleBrainInfo{MatchedDocs: []BrainDocSummary{}, RiskHits: []bfcore.RiskHit{}},
			TDDEvidence:     TDDEvidence{Green: []CommandEvidence{}},
			PreviousBounces: []string{},
		}
		_ = WriteJSONAtomicG4(bPath, bun)
	}

	const iterations = 25
	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		t0 := time.Now()
		rep, code := VerifyRuntime(VerifyOptions{
			FactoryRoot: factory, RepoRoot: repo, VaultRoot: vault, DataDir: dataDir, BrainClient: fb,
		})
		if code != ExitOK || !rep.OK {
			t.Fatalf("VerifyRuntime iteration %d failed: %d %+v", i, code, rep)
		}
		ctxRes, ctxCode := Context(ContextOptions{
			TaskDir: taskDir, RepoRoot: repo, FactoryRoot: factory, DataDir: dataDir, WorktreeRoot: wtRoot, VaultRoot: vault, BrainClient: fb,
		})
		if ctxCode != ExitOK || !ctxRes.OK {
			t.Fatalf("Context iteration %d failed: %d %+v", i, ctxCode, ctxRes)
		}
		durations = append(durations, time.Since(t0))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95Idx := (95*len(durations)+99)/100 - 1
	p95 := durations[p95Idx]
	t.Logf("VerifyRuntime+Context (50 artifacts) p50=%v p95=%v max=%v", durations[len(durations)/2], p95, durations[len(durations)-1])
	if p95 >= 500*time.Millisecond {
		t.Fatalf("latency budget exceeded: p95=%v >= 500ms", p95)
	}
}
