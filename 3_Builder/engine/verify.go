package builder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"blackfactory/pkg/bfcore"
)

const (
	// Hashes SHA-256 normalizados CRLF/BOM anclados para GengiSWAP/engine/ (RZ-001 intacto en v1).
	PinnedBridgeGoSHA256        = "8c72badfb4fba74c5f81fd0e5c14687532ea7b5e95b8bc1ec0331390d54beb82"
	PinnedRingGoSHA256          = "8bb4c7bda1ec71536c95e2c57e771d25a0856de7ee9d06327d66dc81fa57b26f"
	PinnedRunnerGoSHA256        = "776a2858c7e938b5e6ec3861539f78361cb9cae56a9682b824e8792688a20ec7"
	PinnedRunnerWindowsGoSHA256 = "d5c289942c01722dd2ab4e92ecafea15f366aaf2c803b1c49c627f9fe1c20d62"
	PinnedGengisMainGoSHA256    = "4ba109ad252503325f726f39dd36166beec3693bcd134f5fc8df051c2cb523a8"
)

// ResolvedRoots agrupa las rutas efectivas resueltas con los defaults oficiales de 3_Builder.
type ResolvedRoots struct {
	FactoryRoot  string
	RepoRoot     string
	VaultRoot    string
	DataDir      string
	BrainBin     string
	GengisRoot   string
	WorktreeRoot string
}

// ResolveCommonRoots aplica los valores por defecto oficiales para todas las raices y binarios.
func ResolveCommonRoots(factoryRoot, repoRoot, vaultRoot, dataDir, brainBin, gengisRoot, taskDir, worktreeRoot string) ResolvedRoots {
	rRoot := strings.TrimSpace(repoRoot)
	if rRoot == "" {
		rRoot = "."
	}
	fRoot := strings.TrimSpace(factoryRoot)
	if fRoot == "" {
		fRoot = rRoot
	}
	vRoot := strings.TrimSpace(vaultRoot)
	if vRoot == "" {
		vRoot = filepath.Join(fRoot, "0_Brain", "vault")
	}
	dDir := strings.TrimSpace(dataDir)
	if dDir == "" {
		dDir = filepath.Join(fRoot, "3_Builder", "runtime")
	}
	bBin := strings.TrimSpace(brainBin)
	if bBin == "" {
		binName := "brain.exe"
		if runtime.GOOS != "windows" {
			binName = "brain"
		}
		bBin = filepath.Join(fRoot, "0_Brain", binName)
	}
	gRoot := strings.TrimSpace(gengisRoot)
	if gRoot == "" {
		gRoot = filepath.Join(fRoot, "GengiSWAP")
	}
	wRoot := strings.TrimSpace(worktreeRoot)
	if wRoot == "" && strings.TrimSpace(taskDir) != "" {
		if absTask, err := filepath.Abs(taskDir); err == nil {
			wRoot = filepath.Join(filepath.Dir(filepath.Dir(filepath.Clean(absTask))), "worktrees")
		}
	}
	return ResolvedRoots{
		FactoryRoot:  fRoot,
		RepoRoot:     rRoot,
		VaultRoot:    vRoot,
		DataDir:      dDir,
		BrainBin:     bBin,
		GengisRoot:   gRoot,
		WorktreeRoot: wRoot,
	}
}

// ResolveGengisBin resuelve el ejecutable de GengiSWAP desde GENGIS_BIN, override explicito o <gengis-root>.
func ResolveGengisBin(gengisRoot, override string) string {
	if s := strings.TrimSpace(override); s != "" {
		return s
	}
	if env := strings.TrimSpace(os.Getenv("GENGIS_BIN")); env != "" {
		return env
	}
	binName := "gengis.exe"
	if runtime.GOOS != "windows" {
		binName = "gengis"
	}
	return filepath.Join(gengisRoot, binName)
}

// CheckReparseSafety verifica con CheckNoReparseUpToNearestExisting cada ruta no vacia proporcionada.
func CheckReparseSafety(paths ...string) error {
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if err := bfcore.CheckNoReparseUpToNearestExisting("", trimmed); err != nil {
			return err
		}
	}
	return nil
}

// CheckBrainPreflight ejecuta el preflight estrictamente read-only contra 0_Brain.
func CheckBrainPreflight(client bfcore.BrainClient, vaultRoot, dbPath string) (bfcore.BrainPreflight, string, error) {
	if client == nil {
		return bfcore.BrainPreflight{}, ReasonBrainUnavailable, fmt.Errorf("%w: cliente 0_Brain nil", bfcore.ErrBrainUnavailable)
	}
	pf, err := client.Preflight(vaultRoot, dbPath)
	if err != nil {
		if errors.Is(err, bfcore.ErrBrainJournalActive) {
			return bfcore.BrainPreflight{}, ReasonBrainJournalActive, err
		}
		return bfcore.BrainPreflight{}, ReasonBrainUnavailable, err
	}
	return pf, ReasonOK, nil
}

// CheckGitIgnore ejecuta dos invocaciones individuales de git check-ignore -q (3_Builder/runtime/ y
// 3_Builder/builder.exe) y verifica que .gitignore declare literalmente 3_Builder/builder.exe.
func CheckGitIgnore(factoryRoot string) error {
	for _, target := range []string{"3_Builder/runtime/", "3_Builder/builder.exe"} {
		cmd := exec.Command("git", "-C", factoryRoot, "check-ignore", "-q", target)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ruta %q no ignorada por .gitignore en factory-root", target)
		}
	}
	giPath := filepath.Join(factoryRoot, ".gitignore")
	if err := bfcore.CheckNoReparseChain(factoryRoot, giPath); err != nil {
		return err
	}
	raw, err := ReadFileWithShortRetry(giPath)
	if err != nil {
		return fmt.Errorf("leyendo .gitignore: %w", err)
	}
	norm := strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"), "\r\n", "\n"), "\r", "\n")
	foundLiteral := false
	for _, ln := range strings.Split(norm, "\n") {
		if strings.TrimSpace(ln) == "3_Builder/builder.exe" {
			foundLiteral = true
			break
		}
	}
	if !foundLiteral {
		return errors.New(".gitignore no declara la linea literal 3_Builder/builder.exe (evitando falso positivo de *.exe)")
	}
	return nil
}

// CheckGengisEngineHashes compara los 5 archivos anclados de GengiSWAP/engine/ (RZ-001).
func CheckGengisEngineHashes(factoryRoot string) error {
	checks := []struct {
		relPath  string
		expected string
	}{
		{"GengiSWAP/engine/bridge.go", PinnedBridgeGoSHA256},
		{"GengiSWAP/engine/ring.go", PinnedRingGoSHA256},
		{"GengiSWAP/engine/runner.go", PinnedRunnerGoSHA256},
		{"GengiSWAP/engine/runner_windows.go", PinnedRunnerWindowsGoSHA256},
		{"GengiSWAP/engine/cmd/gengis/main.go", PinnedGengisMainGoSHA256},
	}
	for _, ch := range checks {
		state, sum, err := bfcore.ComputeNormalizedFileSHA256(factoryRoot, ch.relPath)
		if err != nil {
			return fmt.Errorf("calculando hash de %s: %w", ch.relPath, err)
		}
		if state != "present" {
			return fmt.Errorf("archivo %s ausente en factory-root", ch.relPath)
		}
		if sum != ch.expected {
			return fmt.Errorf("deriva detectada en %s (obtenido %s, esperado %s)", ch.relPath, sum, ch.expected)
		}
	}
	return nil
}

// VerifyRuntime ejecuta todas las comprobaciones 100% solo lectura de builder verify (0 cerrojos, 0 escrituras)
// con prioridad determinista fija:
// REPARSE_POINT_DETECTED (6) > G4_VIOLATION (3) > GITIGNORE_DRIFT (6) > GENGIS_ENGINE_DRIFT (6) >
// SESSION_STATE_CORRUPT (6) > ARTIFACT_STATE_CORRUPT (6) > BRAIN_JOURNAL_ACTIVE (6) > BRAIN_UNAVAILABLE (6) > IO_ERROR (6).
func VerifyRuntime(opts VerifyOptions) (VerifyReport, int) {
	roots := ResolveCommonRoots(opts.FactoryRoot, opts.RepoRoot, opts.VaultRoot, opts.DataDir, opts.BrainBin, "", "", "")
	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(roots.BrainBin, 0)
	}

	report := VerifyReport{Issues: []ValidationIssue{}}
	var (
		hasReparse        bool
		hasG4             bool
		hasGitIgnoreDrift bool
		hasGengisDrift    bool
		hasSessionBad     bool
		hasArtifactBad    bool
		hasBrainJournal   bool
		hasBrainUnavail   bool
		hasIOError        bool
	)

	if err := CheckReparseSafety(roots.FactoryRoot, roots.RepoRoot, roots.VaultRoot, roots.DataDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			hasReparse = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/reparse", Code: ReasonReparsePointDetected, Message: bfcore.RedactAllG4(err.Error())})
		} else {
			hasIOError = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/reparse", Code: ReasonIOError, Message: bfcore.RedactAllG4(err.Error())})
		}
	}

	if err := CheckGitIgnore(roots.FactoryRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			hasReparse = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/gitignore", Code: ReasonReparsePointDetected, Message: bfcore.RedactAllG4(err.Error())})
		} else {
			hasGitIgnoreDrift = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/gitignore", Code: ReasonGitIgnoreDrift, Message: bfcore.RedactAllG4(err.Error())})
		}
		report.GitIgnoreOK = false
	} else {
		report.GitIgnoreOK = true
	}

	if err := CheckGengisEngineHashes(roots.FactoryRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			hasReparse = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/gengis_hashes", Code: ReasonReparsePointDetected, Message: bfcore.RedactAllG4(err.Error())})
		} else {
			hasGengisDrift = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/gengis_hashes", Code: ReasonGengisEngineDrift, Message: bfcore.RedactAllG4(err.Error())})
		}
		report.GengisHashesOK = false
	} else {
		report.GengisHashesOK = true
	}

	if _, reason, err := CheckBrainPreflight(client, roots.VaultRoot, ""); err != nil {
		report.BrainOK = false
		if reason == ReasonBrainJournalActive {
			hasBrainJournal = true
		} else {
			hasBrainUnavail = true
		}
		report.Issues = append(report.Issues, ValidationIssue{Pointer: "/brain", Code: reason, Message: bfcore.RedactAllG4(err.Error())})
	} else {
		report.BrainOK = true
	}

	if fi, statErr := os.Lstat(roots.DataDir); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			hasIOError = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime", Code: ReasonIOError, Message: bfcore.RedactAllG4(statErr.Error())})
		}
	} else if bfcore.IsReparseOrSymlink(fi) {
		hasReparse = true
		report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime", Code: ReasonReparsePointDetected, Message: "dataDir es un symlink o reparse point prohibido"})
	} else if !fi.IsDir() {
		hasArtifactBad = true
		report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime", Code: ReasonArtifactStateCorrupt, Message: "dataDir no es un directorio"})
	} else {
		walkErr := filepath.WalkDir(roots.DataDir, func(p string, d fs.DirEntry, wErr error) error {
			if wErr != nil {
				hasIOError = true
				report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime", Code: ReasonIOError, Message: bfcore.RedactAllG4(wErr.Error())})
				return nil
			}
			lfi, lErr := os.Lstat(p)
			if lErr != nil {
				if errors.Is(lErr, os.ErrNotExist) {
					return nil
				}
				hasIOError = true
				report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime", Code: ReasonIOError, Message: bfcore.RedactAllG4(lErr.Error())})
				return nil
			}
			if bfcore.IsReparseOrSymlink(lfi) {
				hasReparse = true
				rel, _ := filepath.Rel(roots.DataDir, p)
				report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + filepath.ToSlash(rel), Code: ReasonReparsePointDetected, Message: "reparse point o symlink detectado en runtime/"})
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			base := strings.ToLower(d.Name())
			if strings.HasSuffix(base, ".lock") {
				return nil
			}
			rel, _ := filepath.Rel(roots.DataDir, p)
			relSlash := filepath.ToSlash(rel)

			switch {
			case strings.HasSuffix(base, ".diff"):
				raw, rErr := ReadFileWithShortRetry(p)
				if rErr != nil {
					hasIOError = true
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonIOError, Message: bfcore.RedactAllG4(rErr.Error())})
					return nil
				}
				report.ArtifactsChecked++
				if hits := bfcore.ScanG4All(string(raw)); len(hits) > 0 {
					hasG4 = true
					report.G4Issues += len(hits)
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonG4Violation, Message: fmt.Sprintf("violacion G4 en %s: regla %s (linea %d)", relSlash, hits[0].Rule, hits[0].Line)})
				}

			case strings.HasSuffix(base, ".jsonl"):
				raw, rErr := ReadFileWithShortRetry(p)
				if rErr != nil {
					hasIOError = true
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonIOError, Message: bfcore.RedactAllG4(rErr.Error())})
					return nil
				}
				report.ArtifactsChecked++
				if hits := bfcore.ScanG4All(string(raw)); len(hits) > 0 {
					hasG4 = true
					report.G4Issues += len(hits)
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonG4Violation, Message: fmt.Sprintf("violacion G4 en %s: regla %s (linea %d)", relSlash, hits[0].Rule, hits[0].Line)})
				}
				lines := strings.Split(strings.ReplaceAll(string(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})), "\r\n", "\n"), "\n")
				for idx, ln := range lines {
					trimmed := strings.TrimSpace(ln)
					if trimmed == "" {
						continue
					}
					var obj map[string]any
					if err := json.Unmarshal([]byte(trimmed), &obj); err != nil || obj == nil {
						hasArtifactBad = true
						report.Issues = append(report.Issues, ValidationIssue{Pointer: fmt.Sprintf("/runtime/%s#%d", relSlash, idx+1), Code: ReasonArtifactStateCorrupt, Message: fmt.Sprintf("linea JSONL corrupta en %s:%d", relSlash, idx+1)})
						continue
					}
					var decHits []bfcore.G4Finding
					walkJSONStringsForG4(obj, "", &decHits)
					if len(decHits) > 0 {
						hasG4 = true
						report.G4Issues += len(decHits)
						report.Issues = append(report.Issues, ValidationIssue{Pointer: fmt.Sprintf("/runtime/%s#%d", relSlash, idx+1), Code: ReasonG4Violation, Message: fmt.Sprintf("violacion G4 decodificada en %s:%d (regla %s)", relSlash, idx+1, decHits[0].Rule)})
					}
				}

			case strings.HasSuffix(base, ".json"):
				raw, rErr := ReadFileWithShortRetry(p)
				if rErr != nil {
					hasIOError = true
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonIOError, Message: bfcore.RedactAllG4(rErr.Error())})
					return nil
				}
				report.ArtifactsChecked++
				hits, jsonErr := ScanRawAndDecodedG4(raw)
				if len(hits) > 0 {
					hasG4 = true
					report.G4Issues += len(hits)
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonG4Violation, Message: fmt.Sprintf("violacion G4 en %s: regla %s", relSlash, hits[0].Rule)})
				}
				if base == "session.json" {
					if _, sErr := parseSessionBytesStrict(raw); sErr != nil {
						hasSessionBad = true
						report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonSessionStateCorrupt, Message: fmt.Sprintf("session.json corrupto en %s", relSlash)})
					}
				} else if jsonErr != nil || !validateArtifactJSONStrict(base, raw) {
					hasArtifactBad = true
					report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime/" + relSlash, Code: ReasonArtifactStateCorrupt, Message: fmt.Sprintf("artefacto JSON corrupto en %s", relSlash)})
				}
			}
			return nil
		})
		if walkErr != nil {
			hasIOError = true
			report.Issues = append(report.Issues, ValidationIssue{Pointer: "/runtime", Code: ReasonIOError, Message: bfcore.RedactAllG4(walkErr.Error())})
		}
	}

	switch {
	case hasReparse:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonReparsePointDetected
	case hasG4:
		report.ExitCode = ExitG4
		report.Reason = ReasonG4Violation
	case hasGitIgnoreDrift:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonGitIgnoreDrift
	case hasGengisDrift:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonGengisEngineDrift
	case hasSessionBad:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonSessionStateCorrupt
	case hasArtifactBad:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonArtifactStateCorrupt
	case hasBrainJournal:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonBrainJournalActive
	case hasBrainUnavail:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonBrainUnavailable
	case hasIOError:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonIOError
	default:
		report.OK = true
		report.ExitCode = ExitOK
		report.Reason = ReasonOK
	}
	report.Retryable = IsRetryableReason(report.ExitCode, report.Reason)
	return report, report.ExitCode
}

func validateArtifactJSONStrict(base string, raw []byte) bool {
	cleaned := bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if strings.HasSuffix(base, ".bundle.json") {
		var b DeliveryBundle
		dec := json.NewDecoder(bytes.NewReader(cleaned))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&b); err != nil {
			return false
		}
		var extra any
		return errors.Is(dec.Decode(&extra), io.EOF) && b.SchemaVersion == SchemaVersion && b.BundleID != ""
	}
	if strings.HasSuffix(base, ".order.json") {
		var o BounceOrder
		dec := json.NewDecoder(bytes.NewReader(cleaned))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return false
		}
		var extra any
		return errors.Is(dec.Decode(&extra), io.EOF) && o.SchemaVersion == SchemaVersion && o.OrderID != ""
	}
	return true
}
