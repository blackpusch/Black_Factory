package specwriter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"blackfactory/pkg/bfcore"
)

const (
	// PinnedBridgeGoSHA256 es el SHA-256 normalizado CRLF/BOM anclado para GengiSWAP/engine/bridge.go (RZ-001).
	PinnedBridgeGoSHA256 = "8c72badfb4fba74c5f81fd0e5c14687532ea7b5e95b8bc1ec0331390d54beb82"
	// PinnedRingGoSHA256 es el SHA-256 normalizado CRLF/BOM anclado para GengiSWAP/engine/ring.go (RZ-001).
	PinnedRingGoSHA256 = "8bb4c7bda1ec71536c95e2c57e771d25a0856de7ee9d06327d66dc81fa57b26f"
)

// ResolveCommonRoots aplica los valores por defecto oficiales para factoryRoot, repoRoot, vaultRoot, dataDir y brainBin.
func ResolveCommonRoots(factoryRoot, repoRoot, vaultRoot, dataDir, brainBin string) (fRoot, rRoot, vRoot, dDir, bBin string) {
	rRoot = strings.TrimSpace(repoRoot)
	if rRoot == "" {
		rRoot = "."
	}
	fRoot = strings.TrimSpace(factoryRoot)
	if fRoot == "" {
		fRoot = rRoot
	}
	vRoot = strings.TrimSpace(vaultRoot)
	if vRoot == "" {
		vRoot = filepath.Join(fRoot, "0_Brain", "vault")
	}
	dDir = strings.TrimSpace(dataDir)
	if dDir == "" {
		dDir = filepath.Join(fRoot, "2_Spec_Writer", "runtime")
	}
	bBin = strings.TrimSpace(brainBin)
	if bBin == "" {
		bBin = filepath.Join(fRoot, "0_Brain", "brain.exe")
	}
	return fRoot, rRoot, vRoot, dDir, bBin
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

// CheckBrainPreflight ejecuta el preflight estrictamente read-only contra 0_Brain y distingue
// BRAIN_JOURNAL_ACTIVE frente a BRAIN_UNAVAILABLE.
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

// CheckGitIgnore ejecuta dos invocaciones individuales de git check-ignore -q exigiendo exit 0 en cada una:
//  1. 2_Spec_Writer/runtime/
//  2. 2_Spec_Writer/specwriter.exe
func CheckGitIgnore(factoryRoot string) error {
	targets := []string{
		"2_Spec_Writer/runtime/",
		"2_Spec_Writer/specwriter.exe",
	}
	for _, target := range targets {
		cmd := exec.Command("git", "-C", factoryRoot, "check-ignore", "-q", target)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ruta %q no ignorada por .gitignore en factory-root", target)
		}
	}
	return nil
}

// CheckGengisEngineHashes compara el SHA-256 normalizado CRLF/BOM de GengiSWAP/engine/{bridge,ring}.go
// contra las constantes ancladas PinnedBridgeGoSHA256 y PinnedRingGoSHA256.
func CheckGengisEngineHashes(factoryRoot string) error {
	checks := []struct {
		relPath  string
		expected string
	}{
		{"GengiSWAP/engine/bridge.go", PinnedBridgeGoSHA256},
		{"GengiSWAP/engine/ring.go", PinnedRingGoSHA256},
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

// VerifyRuntime ejecuta todas las comprobaciones de solo lectura de specwriter verify (0 cerrojos exclusivos,
// 0 archivos creados) y selecciona (ExitCode, Reason) con prioridad determinista fija:
// REPARSE_POINT_DETECTED (6) > G4_VIOLATION_ON_DISK (3) > GITIGNORE_DRIFT (6) > GENGIS_ENGINE_DRIFT (6) >
// ATTEMPTS_STATE_CORRUPT (6) > ARTIFACT_STATE_CORRUPT (6) > BRAIN_JOURNAL_ACTIVE (6) > BRAIN_UNAVAILABLE (6) > IO_ERROR (6).
func VerifyRuntime(opts VerifyOptions) (VerifyReport, int) {
	factoryRoot, repoRoot, vaultRoot, dataDir, brainBin := ResolveCommonRoots(
		opts.FactoryRoot, opts.RepoRoot, opts.VaultRoot, opts.DataDir, opts.BrainBin,
	)
	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(brainBin, 0)
	}

	report := VerifyReport{
		Issues: []ValidationIssue{},
	}

	var (
		hasReparse        bool
		hasG4             bool
		hasGitIgnoreDrift bool
		hasGengisDrift    bool
		hasAttemptsBad    bool
		hasArtifactBad    bool
		hasBrainJournal   bool
		hasBrainUnavail   bool
		hasIOError        bool
	)

	// 1. Reparse safety sobre raices configuradas
	if err := CheckReparseSafety(factoryRoot, repoRoot, vaultRoot, dataDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			hasReparse = true
			report.Issues = append(report.Issues, ValidationIssue{
				Pointer: "/reparse",
				Code:    ReasonReparsePointDetected,
				Message: bfcore.RedactAllG4(err.Error()),
			})
		} else {
			hasIOError = true
			report.Issues = append(report.Issues, ValidationIssue{
				Pointer: "/reparse",
				Code:    ReasonIOError,
				Message: bfcore.RedactAllG4(err.Error()),
			})
		}
	}

	// 2. Aislamiento Option i (.gitignore individual para runtime/ y specwriter.exe)
	if err := CheckGitIgnore(factoryRoot); err != nil {
		hasGitIgnoreDrift = true
		report.GitIgnoreOK = false
		report.Issues = append(report.Issues, ValidationIssue{
			Pointer: "/gitignore",
			Code:    ReasonGitIgnoreDrift,
			Message: bfcore.RedactAllG4(err.Error()),
		})
	} else {
		report.GitIgnoreOK = true
	}

	// 3. Deriva en GengiSWAP/engine/{bridge,ring}.go (RZ-001)
	if err := CheckGengisEngineHashes(factoryRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			hasReparse = true
			report.Issues = append(report.Issues, ValidationIssue{
				Pointer: "/gengis_hashes",
				Code:    ReasonReparsePointDetected,
				Message: bfcore.RedactAllG4(err.Error()),
			})
		} else {
			hasGengisDrift = true
			report.Issues = append(report.Issues, ValidationIssue{
				Pointer: "/gengis_hashes",
				Code:    ReasonGengisEngineDrift,
				Message: bfcore.RedactAllG4(err.Error()),
			})
		}
		report.GengisHashesOK = false
	} else {
		report.GengisHashesOK = true
	}

	// 4. Preflight read-only de 0_Brain
	if _, reason, err := CheckBrainPreflight(client, vaultRoot, ""); err != nil {
		report.BrainOK = false
		if reason == ReasonBrainJournalActive {
			hasBrainJournal = true
		} else {
			hasBrainUnavail = true
		}
		report.Issues = append(report.Issues, ValidationIssue{
			Pointer: "/brain",
			Code:    reason,
			Message: bfcore.RedactAllG4(err.Error()),
		})
	} else {
		report.BrainOK = true
	}

	// 5. Escaneo solo lectura de 2_Spec_Writer/runtime/ (checkout limpio sin dataDir pasa sin crear nada)
	if fi, statErr := os.Lstat(dataDir); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			hasIOError = true
			report.Issues = append(report.Issues, ValidationIssue{
				Pointer: "/runtime",
				Code:    ReasonIOError,
				Message: bfcore.RedactAllG4(statErr.Error()),
			})
		}
	} else if bfcore.IsReparseOrSymlink(fi) {
		hasReparse = true
		report.Issues = append(report.Issues, ValidationIssue{
			Pointer: "/runtime",
			Code:    ReasonReparsePointDetected,
			Message: "dataDir es un symlink o reparse point prohibido",
		})
	} else if !fi.IsDir() {
		hasArtifactBad = true
		report.Issues = append(report.Issues, ValidationIssue{
			Pointer: "/runtime",
			Code:    ReasonArtifactStateCorrupt,
			Message: "dataDir no es un directorio",
		})
	} else {
		walkErr := filepath.WalkDir(dataDir, func(p string, d fs.DirEntry, wErr error) error {
			if wErr != nil {
				hasIOError = true
				report.Issues = append(report.Issues, ValidationIssue{
					Pointer: "/runtime",
					Code:    ReasonIOError,
					Message: bfcore.RedactAllG4(wErr.Error()),
				})
				return nil
			}
			lfi, lErr := os.Lstat(p)
			if lErr != nil {
				if errors.Is(lErr, os.ErrNotExist) {
					return nil
				}
				hasIOError = true
				report.Issues = append(report.Issues, ValidationIssue{
					Pointer: "/runtime",
					Code:    ReasonIOError,
					Message: bfcore.RedactAllG4(lErr.Error()),
				})
				return nil
			}
			if bfcore.IsReparseOrSymlink(lfi) {
				hasReparse = true
				rel, _ := filepath.Rel(dataDir, p)
				report.Issues = append(report.Issues, ValidationIssue{
					Pointer: "/runtime/" + filepath.ToSlash(rel),
					Code:    ReasonReparsePointDetected,
					Message: "reparse point o symlink detectado en runtime/",
				})
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
			rel, _ := filepath.Rel(dataDir, p)
			relSlash := filepath.ToSlash(rel)

			if base == "specwriter_events.jsonl" || strings.HasSuffix(base, ".jsonl") {
				raw, rErr := ReadFileWithShortRetry(p)
				if rErr != nil {
					hasIOError = true
					report.Issues = append(report.Issues, ValidationIssue{
						Pointer: "/runtime/" + relSlash,
						Code:    ReasonIOError,
						Message: bfcore.RedactAllG4(rErr.Error()),
					})
					return nil
				}
				report.ArtifactsChecked++
				if hits := bfcore.ScanG4All(string(raw)); len(hits) > 0 {
					hasG4 = true
					report.G4Issues += len(hits)
					report.Issues = append(report.Issues, ValidationIssue{
						Pointer: "/runtime/" + relSlash,
						Code:    ReasonG4ViolationOnDisk,
						Message: fmt.Sprintf("violacion G4 en %s: regla %s (linea %d)", relSlash, hits[0].Rule, hits[0].Line),
					})
				}
				lines := strings.Split(strings.ReplaceAll(string(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})), "\r\n", "\n"), "\n")
				for idx, ln := range lines {
					trimmed := strings.TrimSpace(ln)
					if trimmed == "" {
						continue
					}
					var obj map[string]any
					if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
						hasArtifactBad = true
						report.Issues = append(report.Issues, ValidationIssue{
							Pointer: fmt.Sprintf("/runtime/%s#%d", relSlash, idx+1),
							Code:    ReasonArtifactStateCorrupt,
							Message: fmt.Sprintf("linea JSONL corrupta en %s:%d", relSlash, idx+1),
						})
						continue
					}
					var decHits []bfcore.G4Finding
					walkJSONStringsForG4(obj, "", &decHits)
					if len(decHits) > 0 {
						hasG4 = true
						report.G4Issues += len(decHits)
						report.Issues = append(report.Issues, ValidationIssue{
							Pointer: fmt.Sprintf("/runtime/%s#%d", relSlash, idx+1),
							Code:    ReasonG4ViolationOnDisk,
							Message: fmt.Sprintf("violacion G4 decodificada en %s:%d (regla %s)", relSlash, idx+1, decHits[0].Rule),
						})
					}
				}
				return nil
			}

			if strings.HasSuffix(base, ".json") {
				raw, rErr := ReadFileWithShortRetry(p)
				if rErr != nil {
					hasIOError = true
					report.Issues = append(report.Issues, ValidationIssue{
						Pointer: "/runtime/" + relSlash,
						Code:    ReasonIOError,
						Message: bfcore.RedactAllG4(rErr.Error()),
					})
					return nil
				}
				report.ArtifactsChecked++
				hits, jsonErr := ScanRawAndDecodedG4(raw)
				if len(hits) > 0 {
					hasG4 = true
					report.G4Issues += len(hits)
					report.Issues = append(report.Issues, ValidationIssue{
						Pointer: "/runtime/" + relSlash,
						Code:    ReasonG4ViolationOnDisk,
						Message: fmt.Sprintf("violacion G4 en %s: regla %s", relSlash, hits[0].Rule),
					})
				}
				if base == "attempts.json" {
					if _, aErr := parseAttemptsBytesStrict(raw); aErr != nil {
						hasAttemptsBad = true
						report.Issues = append(report.Issues, ValidationIssue{
							Pointer: "/runtime/" + relSlash,
							Code:    ReasonAttemptsStateCorrupt,
							Message: fmt.Sprintf("attempts.json corrupto en %s", relSlash),
						})
					}
				} else if jsonErr != nil {
					hasArtifactBad = true
					report.Issues = append(report.Issues, ValidationIssue{
						Pointer: "/runtime/" + relSlash,
						Code:    ReasonArtifactStateCorrupt,
						Message: fmt.Sprintf("artefacto JSON corrupto en %s", relSlash),
					})
				}
			}
			return nil
		})
		if walkErr != nil {
			hasIOError = true
			report.Issues = append(report.Issues, ValidationIssue{
				Pointer: "/runtime",
				Code:    ReasonIOError,
				Message: bfcore.RedactAllG4(walkErr.Error()),
			})
		}
	}

	// Seleccion determinista de ExitCode y Reason segun prioridad canonica:
	switch {
	case hasReparse:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonReparsePointDetected
	case hasG4:
		report.ExitCode = ExitG4
		report.Reason = ReasonG4ViolationOnDisk
	case hasGitIgnoreDrift:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonGitIgnoreDrift
	case hasGengisDrift:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonGengisEngineDrift
	case hasAttemptsBad:
		report.ExitCode = ExitInfraOrRefusal
		report.Reason = ReasonAttemptsStateCorrupt
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
