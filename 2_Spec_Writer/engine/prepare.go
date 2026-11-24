package specwriter

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

var (
	reWorkItemID   = regexp.MustCompile(`^WI-[0-9a-f]{12}$`)
	reHex64        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reProjectSlug  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	validTemplates = map[string]bool{
		"modulo_nuevo":         true,
		"correccion_hallazgos": true,
		"auditoria_e2e":        true,
		"mapeo_estatico":       true,
	}
	validWorkItemKinds = map[string]bool{
		"bug":       true,
		"feature":   true,
		"refactor":  true,
		"audit_fix": true,
		"ops":       true,
	}
)

// Prepare valida el WorkItem de entrada, consulta 0_Brain en modo solo lectura, congela spec_context.json,
// inicializa attempts.json en cero y genera el esqueleto inicial spec.json (o aplica --refresh-context / --reset-attempts).
func Prepare(opts PrepareOptions) (PrepareResult, int) {
	start := time.Now()
	fail := func(code int, reason, detail, traceID, wiID string) (PrepareResult, int) {
		return PrepareResult{
			OK:         false,
			ExitCode:   code,
			Reason:     reason,
			Retryable:  IsRetryableReason(code, reason),
			TraceID:    traceID,
			WorkItemID: wiID,
			Detail:     bfcore.RedactAllG4(detail),
		}, code
	}

	if strings.TrimSpace(opts.WorkItemPath) == "" {
		return fail(ExitUsage, ReasonInvalidUsage, "--workitem es obligatorio", "", "")
	}
	if opts.ResetAttempts {
		trimmedReason := strings.TrimSpace(opts.Reason)
		if len(trimmedReason) < 8 {
			return fail(ExitUsage, ReasonInvalidUsage, "--reset-attempts exige --reason de al menos 8 caracteres", "", "")
		}
		if hits := bfcore.ScanG4All(trimmedReason); len(hits) > 0 {
			return fail(ExitG4, ReasonG4Violation, fmt.Sprintf("violacion G4 en --reason (regla %s)", hits[0].Rule), "", "")
		}
	}

	factoryRoot, repoRoot, vaultRoot, dataDir, brainBin := ResolveCommonRoots(
		opts.FactoryRoot, opts.RepoRoot, opts.VaultRoot, opts.DataDir, opts.BrainBin,
	)

	// Inspeccionar archivo --workitem sin seguir symlinks/reparse points
	if err := bfcore.CheckNoReparseChain("", opts.WorkItemPath); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, err.Error(), "", "")
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), "", "")
	}
	wiFI, err := os.Lstat(opts.WorkItemPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fail(ExitUsage, ReasonInvalidUsage, "archivo --workitem inexistente", "", "")
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), "", "")
	}
	if wiFI.IsDir() || bfcore.IsReparseOrSymlink(wiFI) || !wiFI.Mode().IsRegular() {
		if bfcore.IsReparseOrSymlink(wiFI) {
			return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, "archivo --workitem es un reparse point o symlink", "", "")
		}
		return fail(ExitUsage, ReasonInvalidUsage, "--workitem no es un archivo regular", "", "")
	}
	if wiFI.Size() > MaxSpecBytes {
		return fail(ExitUsage, ReasonInvalidUsage, "archivo --workitem excede limite maximo", "", "")
	}

	wiRaw, err := os.ReadFile(opts.WorkItemPath)
	if err != nil {
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), "", "")
	}
	if g4Hits, jsonErr := ScanRawAndDecodedG4(wiRaw); len(g4Hits) > 0 {
		return fail(ExitG4, ReasonG4Violation, fmt.Sprintf("violacion G4 en WorkItem (regla %s)", g4Hits[0].Rule), "", "")
	} else if jsonErr != nil {
		return fail(ExitUsage, ReasonInvalidUsage, fmt.Sprintf("JSON invalido en WorkItem: %v", jsonErr), "", "")
	}

	var wi WorkItemInput
	if err := json.Unmarshal(wiRaw, &wi); err != nil {
		return fail(ExitUsage, ReasonInvalidUsage, fmt.Sprintf("decodificando WorkItem: %v", err), "", "")
	}

	cleanAffected, wiErr := validateWorkItemForPrepare(wi, repoRoot)
	if wiErr != "" {
		return fail(ExitUsage, ReasonInvalidUsage, wiErr, wi.TraceID, wi.ID)
	}

	// Preflight compartido: reparse safety + 0_Brain read-only
	if err := CheckReparseSafety(factoryRoot, repoRoot, vaultRoot, dataDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, err.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), wi.TraceID, wi.ID)
	}

	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(brainBin, 0)
	}
	pf, pfReason, pfErr := CheckBrainPreflight(client, vaultRoot, "")
	if pfErr != nil {
		return fail(ExitInfraOrRefusal, pfReason, pfErr.Error(), wi.TraceID, wi.ID)
	}

	repoBase, repoSHA, absRepo, err := ComputeRepoRootIdentity(repoRoot)
	if err != nil {
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), wi.TraceID, wi.ID)
	}
	sourceWIPath := relativeOrBasenamePath(factoryRoot, opts.WorkItemPath)
	if hits := bfcore.ScanG4All(sourceWIPath); len(hits) > 0 {
		sourceWIPath = filepath.Base(opts.WorkItemPath)
	}

	projectSlug := strings.ToLower(strings.TrimSpace(wi.Project))
	wiDir := filepath.Join(dataDir, projectSlug, wi.ID)
	if err := bfcore.CheckNoReparseUpToNearestExisting("", wiDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, err.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), wi.TraceID, wi.ID)
	}
	if err := os.MkdirAll(wiDir, 0o755); err != nil {
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), wi.TraceID, wi.ID)
	}

	lock, err := AcquireWorkItemLock(wiDir)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return fail(ExitInfraOrRefusal, ReasonLockBusy, err.Error(), wi.TraceID, wi.ID)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, err.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, err.Error(), wi.TraceID, wi.ID)
	}
	defer func() { _ = lock.Unlock() }()

	specPath := filepath.Join(wiDir, "spec.json")
	ctxPath := filepath.Join(wiDir, "spec_context.json")
	attemptsPath := filepath.Join(wiDir, "attempts.json")

	_, specStatErr := os.Lstat(specPath)
	_, ctxStatErr := os.Lstat(ctxPath)
	alreadyExists := specStatErr == nil || ctxStatErr == nil

	if alreadyExists && !opts.RefreshContext && !opts.ResetAttempts {
		return fail(ExitUsage, ReasonSpecAlreadyExists, "spec ya preparada; use --refresh-context o --reset-attempts --reason", wi.TraceID, wi.ID)
	}
	if !alreadyExists && (opts.RefreshContext || opts.ResetAttempts) {
		return fail(ExitUsage, ReasonInvalidUsage, "no existe spec preparada previa para --refresh-context o --reset-attempts", wi.TraceID, wi.ID)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)

	buildContext := func() (SpecContext, error) {
		ctxResp, inv, ctxErr := client.Context(vaultRoot, pf.DBPath, projectSlug, cleanAffected)
		if ctxErr != nil {
			return SpecContext{}, ctxErr
		}
		matchedHashes, hErr := computeMatchedDocHashes(vaultRoot, ctxResp.MatchedDocs)
		if hErr != nil {
			return SpecContext{}, hErr
		}
		baselineHashes := make([]ObservedFile, 0, len(cleanAffected))
		for _, rel := range cleanAffected {
			st, sum, cErr := bfcore.ComputeNormalizedFileSHA256(absRepo, rel)
			if cErr != nil {
				return SpecContext{}, cErr
			}
			baselineHashes = append(baselineHashes, ObservedFile{
				Path:   rel,
				State:  st,
				SHA256: sum,
			})
		}

		impactFlags := wi.ImpactFlags
		if impactFlags == nil {
			impactFlags = []string{}
		}
		triageReasons := wi.TriageReasons
		if triageReasons == nil {
			triageReasons = []TriageReasonInput{}
		}
		riskHits := wi.RiskHits
		if riskHits == nil {
			riskHits = []bfcore.RiskHit{}
		}

		return SpecContext{
			SchemaVersion:      SchemaVersion,
			CreatedAt:          nowStr,
			TraceID:            wi.TraceID,
			SourceWorkItemPath: sourceWIPath,
			RepoRootBasename:   repoBase,
			RepoRootSHA256:     repoSHA,
			GitHead:            detectGitShortHead(absRepo),
			WorkItem: WorkItemSnapshot{
				ID:                     wi.ID,
				Revision:               wi.Revision,
				SanitizedPayloadSHA256: wi.Sanitization.SanitizedPayloadSHA256,
				Project:                projectSlug,
				Kind:                   strings.ToLower(strings.TrimSpace(wi.Kind)),
				Lane:                   wi.Lane,
				SuggestedTemplate:      wi.SuggestedTemplate,
				RiskLevel:              wi.RiskLevel,
				RequiresAutorHITL:      wi.RequiresAutorHITL,
				AffectedPaths:          cleanAffected,
				ImpactFlags:            impactFlags,
				TriageReasons:          triageReasons,
				RiskHits:               riskHits,
			},
			BrainSnapshot: BrainSnapshot{
				BrainDBMTime:     pf.BrainDBMTime,
				RiskZonesSHA256:  ctxResp.RiskZonesSHA256,
				MatchedDocHashes: matchedHashes,
				Invocations:      []bfcore.BrainInvocation{inv},
			},
			BaselineHashes: baselineHashes,
		}, nil
	}

	// Caso A: WorkItem existente con --reset-attempts y/o --refresh-context (nunca sobrescribe spec.json)
	if alreadyExists {
		var attempts AttemptsState
		if opts.ResetAttempts {
			attempts = AttemptsState{
				SchemaVersion: SchemaVersion,
				WIID:          wi.ID,
				Project:       projectSlug,
				UpdatedAt:     nowStr,
			}
		} else {
			loaded, loadErr := LoadAttemptsStrict(wiDir)
			if loadErr != nil {
				if errors.Is(loadErr, ErrAttemptsMissing) {
					return fail(ExitInfraOrRefusal, ReasonAttemptsStateMissing, loadErr.Error(), wi.TraceID, wi.ID)
				}
				if errors.Is(loadErr, ErrAttemptsCorrupt) {
					return fail(ExitInfraOrRefusal, ReasonAttemptsStateCorrupt, loadErr.Error(), wi.TraceID, wi.ID)
				}
				if errors.Is(loadErr, bfcore.ErrReparsePoint) {
					return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, loadErr.Error(), wi.TraceID, wi.ID)
				}
				return fail(ExitInfraOrRefusal, ReasonIOError, loadErr.Error(), wi.TraceID, wi.ID)
			}
			attempts = loaded
			attempts.LastPassedSHA256 = ""
			attempts.LastPassedStage = ""
			attempts.LastPassedGoldfishSHA256 = ""
			attempts.UpdatedAt = nowStr
		}

		if opts.RefreshContext {
			sc, bErr := buildContext()
			if bErr != nil {
				if errors.Is(bErr, bfcore.ErrBrainJournalActive) {
					return fail(ExitInfraOrRefusal, ReasonBrainJournalActive, bErr.Error(), wi.TraceID, wi.ID)
				}
				if errors.Is(bErr, bfcore.ErrBrainUnavailable) {
					return fail(ExitInfraOrRefusal, ReasonBrainUnavailable, bErr.Error(), wi.TraceID, wi.ID)
				}
				if errors.Is(bErr, bfcore.ErrReparsePoint) {
					return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, bErr.Error(), wi.TraceID, wi.ID)
				}
				return fail(ExitInfraOrRefusal, ReasonIOError, bErr.Error(), wi.TraceID, wi.ID)
			}
			if wErr := WriteJSONAtomicG4(ctxPath, sc); wErr != nil {
				if errors.Is(wErr, ErrG4Violation) {
					return fail(ExitG4, ReasonG4Violation, wErr.Error(), wi.TraceID, wi.ID)
				}
				return fail(ExitInfraOrRefusal, ReasonIOError, wErr.Error(), wi.TraceID, wi.ID)
			}
		}

		if wErr := WriteJSONAtomicG4(attemptsPath, attempts); wErr != nil {
			if errors.Is(wErr, ErrG4Violation) {
				return fail(ExitG4, ReasonG4Violation, wErr.Error(), wi.TraceID, wi.ID)
			}
			return fail(ExitInfraOrRefusal, ReasonIOError, wErr.Error(), wi.TraceID, wi.ID)
		}

		dur := time.Since(start).Milliseconds()
		if opts.ResetAttempts {
			_ = AppendEvent(dataDir, SpecWriterEvent{
				Event:      "ATTEMPTS_RESET",
				TraceID:    wi.TraceID,
				Project:    projectSlug,
				WorkItemID: wi.ID,
				ExitCode:   ExitOK,
				Reason:     ReasonOK,
				DurationMS: dur,
				Detail:     strings.TrimSpace(opts.Reason),
			})
		}
		if opts.RefreshContext {
			_ = AppendEvent(dataDir, SpecWriterEvent{
				Event:      "CONTEXT_REFRESHED",
				TraceID:    wi.TraceID,
				Project:    projectSlug,
				WorkItemID: wi.ID,
				ExitCode:   ExitOK,
				Reason:     ReasonOK,
				DurationMS: dur,
				Detail:     "spec_context.json actualizado sin sobrescribir spec.json",
			})
		}

		return PrepareResult{
			OK:           true,
			ExitCode:     ExitOK,
			Reason:       ReasonOK,
			Retryable:    false,
			TraceID:      wi.TraceID,
			WorkItemID:   wi.ID,
			SpecPath:     specPath,
			ContextPath:  ctxPath,
			AttemptsPath: attemptsPath,
			Refreshed:    opts.RefreshContext,
			Reset:        opts.ResetAttempts,
		}, ExitOK
	}

	// Caso B: WorkItem nuevo
	sc, bErr := buildContext()
	if bErr != nil {
		if errors.Is(bErr, bfcore.ErrBrainJournalActive) {
			return fail(ExitInfraOrRefusal, ReasonBrainJournalActive, bErr.Error(), wi.TraceID, wi.ID)
		}
		if errors.Is(bErr, bfcore.ErrBrainUnavailable) {
			return fail(ExitInfraOrRefusal, ReasonBrainUnavailable, bErr.Error(), wi.TraceID, wi.ID)
		}
		if errors.Is(bErr, bfcore.ErrReparsePoint) {
			return fail(ExitInfraOrRefusal, ReasonReparsePointDetected, bErr.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, bErr.Error(), wi.TraceID, wi.ID)
	}

	attempts := AttemptsState{
		SchemaVersion: SchemaVersion,
		WIID:          wi.ID,
		Project:       projectSlug,
		UpdatedAt:     nowStr,
	}

	observedCopy := make([]ObservedFile, len(sc.BaselineHashes))
	copy(observedCopy, sc.BaselineHashes)

	skeleton := NormalizeSpecForCanonicalCopy(Spec{
		SchemaVersion: SchemaVersion,
		WorkItemRef: WorkItemRef{
			ID:                     wi.ID,
			Revision:               wi.Revision,
			SanitizedPayloadSHA256: wi.Sanitization.SanitizedPayloadSHA256,
			TraceID:                wi.TraceID,
			SourceWorkItemPath:     sourceWIPath,
		},
		Project:           projectSlug,
		Lane:              wi.Lane,
		Template:          wi.SuggestedTemplate,
		TargetBranch:      detectGitBranch(absRepo),
		Title:             strings.TrimSpace(wi.Title),
		G3DataMutation:    "",
		RequiresAutorHITL: wi.RequiresAutorHITL,
		ObservedFiles:     observedCopy,
	})

	if wErr := WriteJSONAtomicG4(ctxPath, sc); wErr != nil {
		if errors.Is(wErr, ErrG4Violation) {
			return fail(ExitG4, ReasonG4Violation, wErr.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, wErr.Error(), wi.TraceID, wi.ID)
	}
	if wErr := WriteJSONAtomicG4(attemptsPath, attempts); wErr != nil {
		if errors.Is(wErr, ErrG4Violation) {
			return fail(ExitG4, ReasonG4Violation, wErr.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, wErr.Error(), wi.TraceID, wi.ID)
	}
	if wErr := WriteJSONAtomicG4(specPath, skeleton); wErr != nil {
		if errors.Is(wErr, ErrG4Violation) {
			return fail(ExitG4, ReasonG4Violation, wErr.Error(), wi.TraceID, wi.ID)
		}
		return fail(ExitInfraOrRefusal, ReasonIOError, wErr.Error(), wi.TraceID, wi.ID)
	}

	_ = AppendEvent(dataDir, SpecWriterEvent{
		Event:      "PREPARED",
		TraceID:    wi.TraceID,
		Project:    projectSlug,
		WorkItemID: wi.ID,
		ExitCode:   ExitOK,
		Reason:     ReasonOK,
		DurationMS: time.Since(start).Milliseconds(),
		Detail:     fmt.Sprintf("lane=%s baseline_files=%d", wi.Lane, len(sc.BaselineHashes)),
	})

	return PrepareResult{
		OK:           true,
		ExitCode:     ExitOK,
		Reason:       ReasonOK,
		Retryable:    false,
		TraceID:      wi.TraceID,
		WorkItemID:   wi.ID,
		SpecPath:     specPath,
		ContextPath:  ctxPath,
		AttemptsPath: attemptsPath,
	}, ExitOK
}

func validateWorkItemForPrepare(wi WorkItemInput, repoRoot string) ([]string, string) {
	if wi.SchemaVersion != SchemaVersion {
		return nil, fmt.Sprintf("WorkItem schema_version invalido %q (esperado %q)", wi.SchemaVersion, SchemaVersion)
	}
	if wi.Status != "ready" {
		return nil, fmt.Sprintf("WorkItem en status %q (se requiere status=ready)", wi.Status)
	}
	if !reWorkItemID.MatchString(wi.ID) {
		return nil, fmt.Sprintf("WorkItem id invalido %q", wi.ID)
	}
	if wi.Revision < 1 {
		return nil, fmt.Sprintf("WorkItem revision invalida %d", wi.Revision)
	}
	if !reHex64.MatchString(wi.Sanitization.SanitizedPayloadSHA256) {
		return nil, "WorkItem sanitized_payload_sha256 invalido"
	}
	if strings.TrimSpace(wi.TraceID) == "" {
		return nil, "WorkItem trace_id vacio"
	}
	slug := strings.ToLower(strings.TrimSpace(wi.Project))
	if !reProjectSlug.MatchString(slug) || strings.Contains(slug, "..") {
		return nil, fmt.Sprintf("WorkItem project invalido %q", wi.Project)
	}
	if wi.Lane != "FTD" && wi.Lane != "ASDD" {
		return nil, fmt.Sprintf("WorkItem lane invalido %q", wi.Lane)
	}
	if !validTemplates[wi.SuggestedTemplate] {
		return nil, fmt.Sprintf("WorkItem suggested_template invalido %q", wi.SuggestedTemplate)
	}
	if !validWorkItemKinds[strings.ToLower(strings.TrimSpace(wi.Kind))] {
		return nil, fmt.Sprintf("WorkItem kind invalido %q", wi.Kind)
	}
	if len(wi.AffectedPaths) == 0 {
		return nil, "WorkItem affected_paths vacio"
	}
	cleanPaths, rejectReason := bfcore.SanitizeRelativePaths(wi.AffectedPaths, repoRoot, bfcore.PathSanitizeOpts{
		AllowDir:       false,
		SplitCommas:    false,
		FoldCaseDedupe: true,
	})
	if rejectReason != "" || len(cleanPaths) == 0 {
		return nil, fmt.Sprintf("WorkItem affected_paths invalido (%s)", rejectReason)
	}
	return cleanPaths, ""
}

func computeMatchedDocHashes(vaultRoot string, docs []bfcore.BrainDocMatch) ([]MatchedDocHash, error) {
	seen := make(map[string]bool, len(docs))
	out := make([]MatchedDocHash, 0, len(docs))
	for _, doc := range docs {
		if !strings.EqualFold(doc.Status, "active") {
			continue
		}
		rel := bfcore.NormalizeCoverPath(doc.RelPath)
		if rel == "" {
			continue
		}
		key := strings.ToLower(rel)
		if seen[key] {
			continue
		}
		seen[key] = true
		state, sum, err := bfcore.ComputeNormalizedFileSHA256(vaultRoot, rel)
		if err != nil {
			return nil, err
		}
		if state == "present" {
			out = append(out, MatchedDocHash{RelPath: rel, SHA256: sum})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].RelPath) < strings.ToLower(out[j].RelPath)
	})
	return out, nil
}

func relativeOrBasenamePath(factoryRoot, targetPath string) string {
	absFactory, err1 := filepath.Abs(factoryRoot)
	absTarget, err2 := filepath.Abs(targetPath)
	if err1 == nil && err2 == nil {
		if rel, err := filepath.Rel(absFactory, absTarget); err == nil {
			relSlash := filepath.ToSlash(rel)
			if relSlash != "." && !strings.HasPrefix(relSlash, "../") && !strings.Contains(relSlash, ":") {
				return relSlash
			}
		}
	}
	return filepath.Base(targetPath)
}

func detectGitShortHead(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectGitBranch(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	br := strings.TrimSpace(string(out))
	if br == "HEAD" {
		return ""
	}
	return br
}
