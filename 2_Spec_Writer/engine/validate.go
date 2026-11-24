package specwriter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"blackfactory/pkg/bfcore"
)

var (
	reUnitID           = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{1,47}$`)
	reGoTestRunPattern = regexp.MustCompile(`^[A-Za-z0-9_$./-]+$`)
	reGoTestTimeout    = regexp.MustCompile(`^([1-9][0-9]{0,2})s$`)
	reMarkdownHeading  = regexp.MustCompile(`^#{1,6}\s+`)
	reIndexMaxRetries  = regexp.MustCompile(`(?i)m[aá]x\.?\s*intentos(?:\s*por\s*chunk)?[\s:*]*(\d+)`)
	reListPrefixCheck  = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	reMermaidFence     = regexp.MustCompile("(?im)^\\s*(?:`{3,}|~{3,})\\s*mermaid\\b")

	trivialG5Anchors = map[string]bool{
		"err":    true,
		"func":   true,
		"return": true,
		"if":     true,
		"for":    true,
		"nil":    true,
		"true":   true,
		"false":  true,
	}

	allowedEnvValues = map[string]map[string]bool{
		"CGO_ENABLED":            {"0": true},
		"GENGIS_CONTRACT_STRICT": {"0": true, "1": true},
		"GOOS":                   {"windows": true, "linux": true, "darwin": true},
		"GOARCH":                 {"amd64": true, "arm64": true},
		"GOAMD64":                {"v1": true, "v2": true, "v3": true, "v4": true},
		"PYTHONUTF8":             {"0": true, "1": true},
		"PYTHONIOENCODING":       {"utf-8": true},
	}

	allowedReviewExecutors = map[string]bool{
		"4_Revisores": true,
		"Modo_QA":     true,
		"AUTOR":       true,
	}
)

// Validate ejecuta la compuerta estatica fail-closed con precedencia fija 2 -> 5 -> 3 -> 6 -> 4 -> 1
// bajo .wi.lock, sin ejecutar nunca comandos de la especificacion.
func Validate(opts ValidateOptions) (CLIResult, int) {
	start := time.Now()
	stage := strings.ToLower(strings.TrimSpace(opts.Stage))
	if strings.TrimSpace(opts.SpecPath) == "" || (stage != "draft" && stage != "final") {
		return CLIResult{
			OK:        false,
			ExitCode:  ExitUsage,
			Reason:    ReasonInvalidUsage,
			Retryable: false,
			Issues: []ValidationIssue{{
				Pointer: "/cli",
				Code:    ReasonInvalidUsage,
				Message: "--spec y --stage <draft|final> son obligatorios",
			}},
		}, ExitUsage
	}

	factoryRoot, repoRoot, vaultRoot, dataDir, brainBin := ResolveCommonRoots(
		opts.FactoryRoot, opts.RepoRoot, opts.VaultRoot, opts.DataDir, opts.BrainBin,
	)
	wiDir := filepath.Dir(opts.SpecPath)

	// [2] Verificar ausencia de reparse points previo a .wi.lock e inspeccionar --spec
	if err := CheckReparseSafety(dataDir, wiDir, opts.SpecPath); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", []ValidationIssue{{
				Pointer: "/spec",
				Code:    ReasonReparsePointDetected,
				Message: bfcore.RedactAllG4(err.Error()),
			}})
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", []ValidationIssue{{
			Pointer: "/spec",
			Code:    ReasonIOError,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}

	specFI, err := os.Lstat(opts.SpecPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return failResult(ExitUsage, ReasonInvalidUsage, "", "", 0, "", []ValidationIssue{{
				Pointer: "/spec",
				Code:    ReasonInvalidUsage,
				Message: "archivo --spec inexistente",
			}})
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", []ValidationIssue{{
			Pointer: "/spec",
			Code:    ReasonIOError,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}
	if bfcore.IsReparseOrSymlink(specFI) {
		return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", []ValidationIssue{{
			Pointer: "/spec",
			Code:    ReasonReparsePointDetected,
			Message: "archivo --spec es un reparse point o symlink",
		}})
	}
	if specFI.IsDir() || !specFI.Mode().IsRegular() {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", 0, "", []ValidationIssue{{
			Pointer: "/spec",
			Code:    ReasonInvalidUsage,
			Message: "--spec no es un archivo regular",
		}})
	}

	// [5] Adquirir .wi.lock (mantenido hasta [0]) y leer attempts.json fail-closed
	lock, err := AcquireWorkItemLock(wiDir)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return failResult(ExitInfraOrRefusal, ReasonLockBusy, "", "", 0, "", nil)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}
	defer func() { _ = lock.Unlock() }()

	attempts, err := LoadAttemptsStrict(wiDir)
	if err != nil {
		if errors.Is(err, ErrAttemptsMissing) {
			return failResult(ExitInfraOrRefusal, ReasonAttemptsStateMissing, "", "", 0, "", nil)
		}
		if errors.Is(err, ErrAttemptsCorrupt) {
			return failResult(ExitInfraOrRefusal, ReasonAttemptsStateCorrupt, "", "", 0, "", nil)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}

	stageAttempts := attempts.DraftAttempts
	if stage == "final" {
		stageAttempts = attempts.FinalAttempts
	}
	if stageAttempts >= MaxValidateAttemptsPerStage {
		return failResult(ExitAttemptsBlocked, ReasonMaxValidateAttempts, "", "", attempts.LastExit, attempts.LastReason, []ValidationIssue{{
			Pointer: "/attempts",
			Code:    ReasonMaxValidateAttempts,
			Message: fmt.Sprintf("tope de %d intentos alcanzado en etapa %s (use prepare --reset-attempts --reason)", MaxValidateAttemptsPerStage, stage),
		}})
	}

	consumeStageAttempt := func(exitCode int, reason, traceID, specSHA, detail string) {
		if stage == "final" {
			attempts.FinalAttempts++
		} else {
			attempts.DraftAttempts++
		}
		attempts.LastExit = exitCode
		attempts.LastReason = reason
		attempts.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = WriteJSONAtomicG4(filepath.Join(wiDir, "attempts.json"), attempts)
		evName := "VALIDATE_FAIL"
		if exitCode == ExitG4 {
			evName = "REJECTED_G4"
		}
		_ = AppendEvent(dataDir, SpecWriterEvent{
			Event:      evName,
			TraceID:    traceID,
			Project:    attempts.Project,
			WorkItemID: attempts.WIID,
			Stage:      stage,
			ExitCode:   exitCode,
			Reason:     reason,
			Retryable:  IsRetryableReason(exitCode, reason),
			SpecSHA256: specSHA,
			DurationMS: time.Since(start).Milliseconds(),
			Detail:     detail,
		})
	}

	// [3] Cuotas de tamano + G4 Fail-Closed (11 reglas) + parseo JSON
	if specFI.Size() > MaxSpecBytes {
		consumeStageAttempt(ExitFixable, ReasonSpecTooLarge, "", "", "spec.json excede MaxSpecBytes")
		return failResult(ExitFixable, ReasonSpecTooLarge, "", "", 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonSpecTooLarge,
			Message: fmt.Sprintf("spec.json excede el limite de %d bytes", MaxSpecBytes),
		}})
	}

	goldfishPath := filepath.Join(wiDir, "goldfish_review.json")
	var goldfishRaw []byte
	if stage == "final" {
		gfFI, gfErr := os.Lstat(goldfishPath)
		if gfErr == nil {
			if bfcore.IsReparseOrSymlink(gfFI) {
				return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
			}
			if gfFI.IsDir() || !gfFI.Mode().IsRegular() {
				consumeStageAttempt(ExitFixable, ReasonInvalidJSON, "", "", "goldfish_review.json no es archivo regular")
				return failResult(ExitFixable, ReasonInvalidJSON, "", "", 0, "", []ValidationIssue{{
					Pointer: "/goldfish_review",
					Code:    ReasonInvalidJSON,
					Message: "goldfish_review.json no es un archivo regular",
				}})
			}
			if gfFI.Size() > MaxGoldfishBytes {
				consumeStageAttempt(ExitFixable, ReasonSpecTooLarge, "", "", "goldfish_review.json excede MaxGoldfishBytes")
				return failResult(ExitFixable, ReasonSpecTooLarge, "", "", 0, "", []ValidationIssue{{
					Pointer: "/goldfish_review",
					Code:    ReasonSpecTooLarge,
					Message: fmt.Sprintf("goldfish_review.json excede el limite de %d bytes", MaxGoldfishBytes),
				}})
			}
			goldfishRaw, err = os.ReadFile(goldfishPath)
			if err != nil {
				return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
			}
		} else if !errors.Is(gfErr, os.ErrNotExist) {
			return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
		}
	}

	specBytes, err := os.ReadFile(opts.SpecPath)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}
	if g4Hits, jsonErr := ScanRawAndDecodedG4(specBytes); len(g4Hits) > 0 {
		detail := fmt.Sprintf("regla %s en %s", g4Hits[0].Rule, g4Hits[0].Field)
		consumeStageAttempt(ExitG4, ReasonG4Violation, "", "", detail)
		return failResult(ExitG4, ReasonG4Violation, "", "", 0, "", []ValidationIssue{{
			Pointer: pointerOrRoot(g4Hits[0].Field),
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("violacion G4 detectada (regla %s, linea %d)", g4Hits[0].Rule, g4Hits[0].Line),
		}})
	} else if jsonErr != nil {
		consumeStageAttempt(ExitFixable, ReasonInvalidJSON, "", "", "JSON invalido en spec.json")
		return failResult(ExitFixable, ReasonInvalidJSON, "", "", 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonInvalidJSON,
			Message: fmt.Sprintf("JSON invalido en spec.json: %v", jsonErr),
		}})
	}

	if len(goldfishRaw) > 0 {
		if g4Hits, jsonErr := ScanRawAndDecodedG4(goldfishRaw); len(g4Hits) > 0 {
			detail := fmt.Sprintf("regla %s en goldfish_review.json", g4Hits[0].Rule)
			consumeStageAttempt(ExitG4, ReasonG4Violation, "", "", detail)
			return failResult(ExitG4, ReasonG4Violation, "", "", 0, "", []ValidationIssue{{
				Pointer: "/goldfish_review",
				Code:    ReasonG4Violation,
				Message: fmt.Sprintf("violacion G4 en goldfish_review.json (regla %s)", g4Hits[0].Rule),
			}})
		} else if jsonErr != nil {
			consumeStageAttempt(ExitFixable, ReasonInvalidJSON, "", "", "JSON invalido en goldfish_review.json")
			return failResult(ExitFixable, ReasonInvalidJSON, "", "", 0, "", []ValidationIssue{{
				Pointer: "/goldfish_review",
				Code:    ReasonInvalidJSON,
				Message: fmt.Sprintf("JSON invalido en goldfish_review.json: %v", jsonErr),
			}})
		}
	}

	// [6] Preflight compartido (CheckReparseSafety + CheckBrainPreflight) y carga de spec_context.json
	if err := CheckReparseSafety(factoryRoot, repoRoot, vaultRoot, dataDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}

	ctxObj, ctxCode, ctxReason := loadSpecContextStrict(wiDir)
	if ctxCode != ExitOK {
		return failResult(ctxCode, ctxReason, "", "", 0, "", []ValidationIssue{{
			Pointer: "/spec_context",
			Code:    ctxReason,
			Message: "spec_context.json ausente, corrupto o con violacion G4",
		}})
	}

	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(brainBin, 0)
	}
	pf, pfReason, pfErr := CheckBrainPreflight(client, vaultRoot, "")
	if pfErr != nil {
		return failResult(ExitInfraOrRefusal, pfReason, ctxObj.TraceID, "", 0, "", []ValidationIssue{{
			Pointer: "/brain",
			Code:    pfReason,
			Message: bfcore.RedactAllG4(pfErr.Error()),
		}})
	}

	var spec Spec
	_ = json.Unmarshal(specBytes, &spec)
	specSHA, _ := CanonicalSpecSHA256(spec)

	// [4] Decision Externa (STALE_CONTEXT -> ESCALATION_REQUIRED -> LANE_UPGRADE_REQUIRED)
	extRes, extErrCode, extErrReason := evaluateExternalDecisions(spec, ctxObj, repoRoot, vaultRoot, pf.DBPath, client)
	if extErrCode != ExitOK {
		return failResult(extErrCode, extErrReason, ctxObj.TraceID, specSHA, 0, "", extRes.Issues)
	}
	if extRes.ExternalReason != "" {
		if attempts.ExternalCycles >= MaxExternalCycles {
			attempts.LastExit = ExitExternalDecision
			attempts.LastReason = extRes.ExternalReason
			attempts.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = WriteJSONAtomicG4(filepath.Join(wiDir, "attempts.json"), attempts)
			_ = AppendEvent(dataDir, SpecWriterEvent{
				Event:      "MAX_EXTERNAL_CYCLES",
				TraceID:    ctxObj.TraceID,
				Project:    attempts.Project,
				WorkItemID: attempts.WIID,
				Stage:      stage,
				ExitCode:   ExitAttemptsBlocked,
				Reason:     ReasonMaxExternalCycles,
				SpecSHA256: specSHA,
				DurationMS: time.Since(start).Milliseconds(),
				Detail:     extRes.ExternalReason,
			})
			return failResult(ExitAttemptsBlocked, ReasonMaxExternalCycles, ctxObj.TraceID, specSHA, attempts.LastExit, attempts.LastReason, extRes.Issues)
		}
		attempts.ExternalCycles++
		attempts.LastExit = ExitExternalDecision
		attempts.LastReason = extRes.ExternalReason
		attempts.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = WriteJSONAtomicG4(filepath.Join(wiDir, "attempts.json"), attempts)
		_ = AppendEvent(dataDir, SpecWriterEvent{
			Event:      extRes.ExternalReason,
			TraceID:    ctxObj.TraceID,
			Project:    attempts.Project,
			WorkItemID: attempts.WIID,
			Stage:      stage,
			ExitCode:   ExitExternalDecision,
			Reason:     extRes.ExternalReason,
			SpecSHA256: specSHA,
			DurationMS: time.Since(start).Milliseconds(),
			Detail:     fmt.Sprintf("external_cycles=%d/%d", attempts.ExternalCycles, MaxExternalCycles),
		})
		return failResult(ExitExternalDecision, extRes.ExternalReason, ctxObj.TraceID, specSHA, 0, "", extRes.Issues)
	}

	// [1] Validador compartido ValidateSpecAndRenderCheck
	issues, _, goldfishSHA, valErr := ValidateSpecAndRenderCheck(
		specBytes, spec, ctxObj, wiDir, repoRoot, factoryRoot, stage, "preview_task", extRes.ReQueriedRiskHits, extRes.EscalationOccurred,
	)
	if valErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonIOError,
			Message: bfcore.RedactAllG4(valErr.Error()),
		}})
	}
	if len(issues) > 0 {
		consumeStageAttempt(ExitFixable, ReasonValidationFailed, ctxObj.TraceID, specSHA, fmt.Sprintf("%d issues", len(issues)))
		return failResult(ExitFixable, ReasonValidationFailed, ctxObj.TraceID, specSHA, 0, "", issues)
	}

	// [0] OK: persistir last_passed_* sin resetear final_attempts al pasar draft
	attempts.LastPassedSHA256 = specSHA
	attempts.LastPassedStage = stage
	if stage == "final" && spec.Lane == "ASDD" {
		attempts.LastPassedGoldfishSHA256 = goldfishSHA
	} else {
		attempts.LastPassedGoldfishSHA256 = ""
	}
	attempts.LastExit = ExitOK
	attempts.LastReason = ReasonOK
	attempts.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := WriteJSONAtomicG4(filepath.Join(wiDir, "attempts.json"), attempts); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}

	_ = AppendEvent(dataDir, SpecWriterEvent{
		Event:      "VALIDATE_PASS",
		TraceID:    ctxObj.TraceID,
		Project:    attempts.Project,
		WorkItemID: attempts.WIID,
		Stage:      stage,
		ExitCode:   ExitOK,
		Reason:     ReasonOK,
		SpecSHA256: specSHA,
		DurationMS: time.Since(start).Milliseconds(),
		Detail:     fmt.Sprintf("lane=%s units=%d", spec.Lane, len(spec.WorkUnits)),
	})

	return CLIResult{
		OK:         true,
		ExitCode:   ExitOK,
		Reason:     ReasonOK,
		Retryable:  false,
		TraceID:    ctxObj.TraceID,
		SpecSHA256: specSHA,
	}, ExitOK
}

type externalEvalResult struct {
	ExternalReason     string
	Issues             []ValidationIssue
	ReQueriedRiskHits  []bfcore.RiskHit
	EscalationOccurred bool
}

func evaluateExternalDecisions(spec Spec, ctx SpecContext, repoRoot, vaultRoot, dbPath string, client bfcore.BrainClient) (externalEvalResult, int, string) {
	_, repoSHA, absRepo, err := ComputeRepoRootIdentity(repoRoot)
	if err != nil {
		return externalEvalResult{}, ExitInfraOrRefusal, ReasonIOError
	}

	// Pre-saneamiento en memoria (sin I/O) sobre observed_files[] y can_modify[]
	var rawCanModify []string
	for _, u := range spec.WorkUnits {
		rawCanModify = append(rawCanModify, u.CanModify...)
	}
	cleanCanModify, canModReject := bfcore.SanitizeRelativePaths(rawCanModify, absRepo, bfcore.PathSanitizeOpts{
		AllowDir:       false,
		SplitCommas:    false,
		FoldCaseDedupe: true,
	})
	canModifyValid := len(rawCanModify) > 0 && canModReject == "" && len(cleanCanModify) > 0

	queryFiles := cleanCanModify
	if !canModifyValid {
		queryFiles = ctx.WorkItem.AffectedPaths
	}

	liveCtxResp, _, bErr := client.Context(vaultRoot, dbPath, ctx.WorkItem.Project, queryFiles)
	if bErr != nil {
		if errors.Is(bErr, bfcore.ErrBrainJournalActive) {
			return externalEvalResult{}, ExitInfraOrRefusal, ReasonBrainJournalActive
		}
		return externalEvalResult{}, ExitInfraOrRefusal, ReasonBrainUnavailable
	}

	// (1) STALE_CONTEXT
	var staleIssues []ValidationIssue
	if repoSHA != ctx.RepoRootSHA256 {
		staleIssues = append(staleIssues, ValidationIssue{
			Pointer: "/spec_context/repo_root_sha256",
			Code:    ReasonStaleContext,
			Message: "el --repo-root actual no coincide con repo_root_sha256 congelado en spec_context.json",
		})
	}
	if ctx.BrainSnapshot.RiskZonesSHA256 != "" && liveCtxResp.RiskZonesSHA256 != ctx.BrainSnapshot.RiskZonesSHA256 {
		staleIssues = append(staleIssues, ValidationIssue{
			Pointer: "/spec_context/brain_snapshot/risk_zones_sha256",
			Code:    ReasonStaleContext,
			Message: "wiki/risk_zones.md cambio desde prepare; ejecute prepare --refresh-context",
		})
	}
	for i, dh := range ctx.BrainSnapshot.MatchedDocHashes {
		st, sum, hErr := bfcore.ComputeNormalizedFileSHA256(vaultRoot, dh.RelPath)
		if hErr != nil {
			if errors.Is(hErr, bfcore.ErrReparsePoint) {
				return externalEvalResult{}, ExitInfraOrRefusal, ReasonReparsePointDetected
			}
			return externalEvalResult{}, ExitInfraOrRefusal, ReasonIOError
		}
		if st != "present" || sum != dh.SHA256 {
			staleIssues = append(staleIssues, ValidationIssue{
				Pointer: fmt.Sprintf("/spec_context/brain_snapshot/matched_doc_hashes/%d", i),
				Code:    ReasonStaleContext,
				Message: fmt.Sprintf("documento activo %q de 0_Brain cambio o desaparecio desde prepare", dh.RelPath),
			})
		}
	}
	for i, bh := range ctx.BaselineHashes {
		st, sum, hErr := bfcore.ComputeNormalizedFileSHA256(absRepo, bh.Path)
		if hErr != nil {
			if errors.Is(hErr, bfcore.ErrReparsePoint) {
				return externalEvalResult{}, ExitInfraOrRefusal, ReasonReparsePointDetected
			}
			return externalEvalResult{}, ExitInfraOrRefusal, ReasonIOError
		}
		if st != bh.State || (bh.State == "present" && sum != bh.SHA256) {
			staleIssues = append(staleIssues, ValidationIssue{
				Pointer: fmt.Sprintf("/spec_context/baseline_hashes/%d", i),
				Code:    ReasonStaleContext,
				Message: fmt.Sprintf("archivo baseline %q cambio en disco desde prepare", bh.Path),
			})
		}
	}
	for i, of := range spec.ObservedFiles {
		if of.State != "present" && of.State != "absent" {
			continue // diferido a [1]
		}
		cleanObs, rej := bfcore.SanitizeRelativePaths([]string{of.Path}, absRepo, bfcore.PathSanitizeOpts{
			AllowDir:       false,
			SplitCommas:    false,
			FoldCaseDedupe: true,
		})
		if rej != "" || len(cleanObs) != 1 {
			continue // entrada invalida diferida a [1] sin I/O
		}
		st, sum, hErr := bfcore.ComputeNormalizedFileSHA256(absRepo, cleanObs[0])
		if hErr != nil {
			if errors.Is(hErr, bfcore.ErrReparsePoint) {
				return externalEvalResult{}, ExitInfraOrRefusal, ReasonReparsePointDetected
			}
			// Si la ruta en disco es un directorio, diferir el error de esquema/ruta a [1] en vez de 6 si no es reparse
			continue
		}
		if of.State == "present" && (st != "present" || sum != of.SHA256) {
			staleIssues = append(staleIssues, ValidationIssue{
				Pointer: fmt.Sprintf("/observed_files/%d", i),
				Code:    ReasonStaleContext,
				Message: fmt.Sprintf("archivo observado %q cambio o fue eliminado respecto a observed_files", cleanObs[0]),
			})
		} else if of.State == "absent" && st != "absent" {
			staleIssues = append(staleIssues, ValidationIssue{
				Pointer: fmt.Sprintf("/observed_files/%d", i),
				Code:    ReasonStaleContext,
				Message: fmt.Sprintf("archivo marcado absent %q ahora existe en disco", cleanObs[0]),
			})
		}
	}

	if len(staleIssues) > 0 {
		return externalEvalResult{
			ExternalReason: ReasonStaleContext,
			Issues:         staleIssues,
		}, ExitOK, ReasonOK
	}

	// Filtrar risk_hits aplicables al proyecto del WorkItem
	reQueriedHits := filterProjectRiskHits(liveCtxResp.RiskHits, ctx.WorkItem.Project)

	// Si can_modify esta vacio o invalido (ej. esqueleto inicial work_units: []), omitir (2)/(3) y delegar a [1]
	if !canModifyValid {
		return externalEvalResult{ReQueriedRiskHits: reQueriedHits}, ExitOK, ReasonOK
	}

	// (2) ESCALATION_REQUIRED
	baselineMaxRank := bfcore.RiskLevelRank(ctx.WorkItem.RiskLevel)
	baselineCritIDs := make(map[string]bool)
	for _, rh := range ctx.WorkItem.RiskHits {
		if strings.EqualFold(strings.TrimSpace(rh.Level), "CRITICO") {
			baselineCritIDs[strings.ToUpper(strings.TrimSpace(rh.ID))] = true
		}
	}
	liveMaxRank := 0
	hasNewCritico := false
	for _, rh := range reQueriedHits {
		r := bfcore.RiskLevelRank(rh.Level)
		if r > liveMaxRank {
			liveMaxRank = r
		}
		if strings.EqualFold(strings.TrimSpace(rh.Level), "CRITICO") {
			if !baselineCritIDs[strings.ToUpper(strings.TrimSpace(rh.ID))] {
				hasNewCritico = true
			}
		}
	}

	canModifyMods := bfcore.CountRootModules(cleanCanModify)
	affectedMods := bfcore.CountRootModules(ctx.WorkItem.AffectedPaths)
	affectedModSet := make(map[string]bool, len(affectedMods))
	for _, m := range affectedMods {
		affectedModSet[strings.ToLower(m)] = true
	}
	addedNewRootMod := false
	for _, m := range canModifyMods {
		if !affectedModSet[strings.ToLower(m)] {
			addedNewRootMod = true
			break
		}
	}

	escalationTriggered := liveMaxRank > baselineMaxRank ||
		hasNewCritico ||
		(len(canModifyMods) > 1 && (ctx.WorkItem.Lane == "FTD" || addedNewRootMod))

	if escalationTriggered {
		currentScopeSHA := ComputeScopeSHA256(cleanCanModify)
		if !hasValidActiveDecisionForScope(spec.Decisions, absRepo, currentScopeSHA) {
			return externalEvalResult{
				ExternalReason:     ReasonEscalationRequired,
				ReQueriedRiskHits:  reQueriedHits,
				EscalationOccurred: true,
				Issues: []ValidationIssue{{
					Pointer: "/decisions",
					Code:    ReasonEscalationRequired,
					Message: fmt.Sprintf("escalado requerido (risk_rank=%d vs baseline=%d, new_critico=%v, root_modules=%v): registre decision aprobada para scope_sha256=%s",
						liveMaxRank, baselineMaxRank, hasNewCritico, canModifyMods, currentScopeSHA),
				}},
			}, ExitOK, ReasonOK
		}
	}

	// (3) LANE_UPGRADE_REQUIRED
	if spec.Lane == "FTD" {
		var upgradeReasons []string
		kindLow := strings.ToLower(strings.TrimSpace(ctx.WorkItem.Kind))
		if kindLow == "feature" || kindLow == "ops" {
			upgradeReasons = append(upgradeReasons, fmt.Sprintf("kind=%q exige ASDD", kindLow))
		}
		if len(ctx.WorkItem.ImpactFlags) > 0 {
			upgradeReasons = append(upgradeReasons, "impact_flags declarados exigen ASDD")
		}
		if liveMaxRank >= bfcore.RiskLevelRank("CRITICO") || baselineMaxRank >= bfcore.RiskLevelRank("CRITICO") {
			upgradeReasons = append(upgradeReasons, "zona de riesgo CRITICO exige ASDD")
		}
		if liveCtxResp.RiskZonesParseError != "" || !liveCtxResp.HasActiveRiskZones || hasTriageReasonCode(ctx.WorkItem.TriageReasons, "RISK_ZONES_UNPARSEABLE_FAILSAFE") {
			upgradeReasons = append(upgradeReasons, "risk_zones ausente o inparseable exige ASDD fail-safe")
		}
		ftdEval := bfcore.EvaluateFTDBounds(cleanCanModify, bfcore.DefaultMaxFTDFiles)
		if !ftdEval.WithinBounds {
			upgradeReasons = append(upgradeReasons, fmt.Sprintf("alcance fuera de FTD Opcion 5B (modulos=%d, efectivos=%d, fisicos=%d, exentos=%d)",
				len(ftdEval.RootModules), ftdEval.EffectiveCount, ftdEval.PhysicalCount, ftdEval.ExemptTestCount))
		}
		if uncov := bfcore.FindUncoveredPaths(cleanCanModify, liveCtxResp.MatchedDocs); len(uncov) > 0 {
			upgradeReasons = append(upgradeReasons, fmt.Sprintf("rutas sin cobertura activa wiki/adr en 0_Brain: %s", strings.Join(uncov, ", ")))
		}
		if len(upgradeReasons) > 0 {
			return externalEvalResult{
				ExternalReason:     ReasonLaneUpgradeRequired,
				ReQueriedRiskHits:  reQueriedHits,
				EscalationOccurred: escalationTriggered,
				Issues: []ValidationIssue{{
					Pointer: "/lane",
					Code:    ReasonLaneUpgradeRequired,
					Message: strings.Join(upgradeReasons, "; "),
				}},
			}, ExitOK, ReasonOK
		}
	}

	return externalEvalResult{
		ReQueriedRiskHits:  reQueriedHits,
		EscalationOccurred: escalationTriggered,
	}, ExitOK, ReasonOK
}

// ValidateSpecAndRenderCheck ejecuta el validador compartido entre validate.go y compile.go:
// esquema estricto DisallowUnknownFields(), work_item_ref, observed_files[], carril, decisions[],
// DAG aciclico, alcance no sombreado, capitalizacion NTFS, G2, G5, risk_mitigations[], allowlist cerrado
// de verify_steps, renderizado en memoria de plantillas GengiSWAP + validateChunks + guardas parseIndexFile/parseChunkFile,
// y validacion de goldfish_review.json en ASDD --stage final.
func ValidateSpecAndRenderCheck(
	specBytes []byte,
	_ Spec,
	ctx SpecContext,
	wiDir, repoRoot, factoryRoot, stage, taskIDHint string,
	reQueriedRiskHits []bfcore.RiskHit,
	escalationOccurred bool,
) ([]ValidationIssue, RenderedTaskFiles, string, error) {
	var issues []ValidationIssue
	addIssue := func(ptr, code, msg string) {
		issues = append(issues, ValidationIssue{Pointer: ptr, Code: code, Message: msg})
	}

	// 1. Decodificacion estricta con DisallowUnknownFields()
	cleanedBytes := bytes.TrimPrefix(specBytes, []byte{0xef, 0xbb, 0xbf})
	var spec Spec
	dec := json.NewDecoder(bytes.NewReader(cleanedBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		addIssue("/", ReasonValidationFailed, fmt.Sprintf("esquema JSON estricto violado: %v", err))
		return issues, RenderedTaskFiles{}, "", nil
	}
	if dec.More() {
		addIssue("/", ReasonValidationFailed, "contenido JSON adicional inesperado tras el objeto Spec")
		return issues, RenderedTaskFiles{}, "", nil
	}

	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, RenderedTaskFiles{}, "", err
	}
	absRepo = filepath.Clean(absRepo)

	// 2. Campos top-level y work_item_ref
	if spec.SchemaVersion != SchemaVersion {
		addIssue("/schema_version", ReasonValidationFailed, fmt.Sprintf("schema_version debe ser %q", SchemaVersion))
	}
	if spec.WorkItemRef.ID != ctx.WorkItem.ID {
		addIssue("/work_item_ref/id", ReasonValidationFailed, "work_item_ref.id no coincide con spec_context.json")
	}
	if spec.WorkItemRef.Revision != ctx.WorkItem.Revision {
		addIssue("/work_item_ref/revision", ReasonValidationFailed, "work_item_ref.revision no coincide con spec_context.json")
	}
	if spec.WorkItemRef.SanitizedPayloadSHA256 != ctx.WorkItem.SanitizedPayloadSHA256 {
		addIssue("/work_item_ref/sanitized_payload_sha256", ReasonValidationFailed, "work_item_ref.sanitized_payload_sha256 no coincide con spec_context.json")
	}
	if spec.WorkItemRef.TraceID != ctx.TraceID {
		addIssue("/work_item_ref/trace_id", ReasonValidationFailed, "work_item_ref.trace_id no coincide con spec_context.json")
	}
	if spec.WorkItemRef.SourceWorkItemPath != ctx.SourceWorkItemPath {
		addIssue("/work_item_ref/source_workitem_path", ReasonValidationFailed, "work_item_ref.source_workitem_path no coincide con spec_context.json")
	}
	if !strings.EqualFold(strings.TrimSpace(spec.Project), ctx.WorkItem.Project) || strings.TrimSpace(spec.Project) == "" {
		addIssue("/project", ReasonValidationFailed, "project debe coincidir con spec_context.json")
	}
	if spec.Lane != "FTD" && spec.Lane != "ASDD" {
		addIssue("/lane", ReasonValidationFailed, "lane debe ser FTD o ASDD")
	}
	if ctx.WorkItem.Lane == "ASDD" && spec.Lane != "ASDD" {
		addIssue("/lane", ReasonValidationFailed, "prohibido descenso de carril ASDD -> FTD")
	}
	if !validTemplates[spec.Template] {
		addIssue("/template", ReasonValidationFailed, fmt.Sprintf("template invalido %q", spec.Template))
	}
	if strings.TrimSpace(spec.TargetBranch) == "" || hasLineBreak(spec.TargetBranch) || strings.ContainsAny(spec.TargetBranch, " \t`") {
		addIssue("/target_branch", ReasonValidationFailed, "target_branch obligatorio, una sola linea, sin espacios ni backticks")
	}
	if strings.TrimSpace(spec.Title) == "" || len(spec.Title) > 160 || hasLineBreak(spec.Title) {
		addIssue("/title", ReasonValidationFailed, "title obligatorio (1..160 chars, sin saltos de linea)")
	}
	if strings.TrimSpace(spec.G3DataMutation) == "" || hasLineBreak(spec.G3DataMutation) {
		addIssue("/g3_data_mutation", ReasonValidationFailed, "g3_data_mutation obligatorio en una sola linea sin saltos de linea")
	}
	if len(spec.StackAssumptions) == 0 {
		addIssue("/stack_assumptions", ReasonValidationFailed, "stack_assumptions requiere al menos 1 entrada")
	}
	if len(spec.DoD) == 0 {
		addIssue("/dod", ReasonValidationFailed, "dod requiere al menos 1 entrada")
	}

	// 3. Narrativa FTD vs ASDD y G2 (cero Mermaid)
	if strings.TrimSpace(spec.Narrative.Problem) == "" {
		addIssue("/narrative/problem", ReasonValidationFailed, "narrative.problem obligatorio")
	}
	if strings.TrimSpace(spec.Narrative.RootCause) == "" {
		addIssue("/narrative/root_cause", ReasonValidationFailed, "narrative.root_cause obligatorio")
	}
	if strings.TrimSpace(spec.Narrative.ProposedChange) == "" {
		addIssue("/narrative/proposed_change", ReasonValidationFailed, "narrative.proposed_change obligatorio")
	}
	if spec.Lane == "ASDD" {
		if len(spec.Narrative.AlternativesConsidered) == 0 {
			addIssue("/narrative/alternatives_considered", ReasonValidationFailed, "ASDD exige al menos 1 alternativa considerada")
		}
		if strings.TrimSpace(spec.Narrative.InterfacesAndContracts) == "" {
			addIssue("/narrative/interfaces_and_contracts", ReasonValidationFailed, "ASDD exige narrative.interfaces_and_contracts")
		}
		if len(spec.Narrative.EdgeCases) == 0 {
			addIssue("/narrative/edge_cases", ReasonValidationFailed, "ASDD exige al menos 1 caso borde")
		}
		if strings.TrimSpace(spec.Narrative.RollbackPlan) == "" {
			addIssue("/narrative/rollback_plan", ReasonValidationFailed, "ASDD exige narrative.rollback_plan")
		}
	}

	// G2: cero bloques Mermaid en cualquier string del spec
	if reMermaidFence.MatchString(string(cleanedBytes)) {
		addIssue("/", ReasonValidationFailed, "violacion G2: bloques Mermaid prohibidos (use diagramas ASCII/Unicode Box-Drawing)")
	} else {
		var rawAny any
		if json.Unmarshal(cleanedBytes, &rawAny) == nil {
			checkNoMermaidInJSON(rawAny, "", addIssue)
		}
	}

	// Guardas de prosa index.md (parseIndexFile)
	checkIndexProseSafety(spec.Narrative.Problem, "/narrative/problem", addIssue)
	for i, sa := range spec.StackAssumptions {
		if strings.TrimSpace(sa) == "" {
			addIssue(fmt.Sprintf("/stack_assumptions/%d", i), ReasonValidationFailed, "entrada vacia en stack_assumptions")
		}
		checkIndexProseSafety(sa, fmt.Sprintf("/stack_assumptions/%d", i), addIssue)
	}
	for i, d := range spec.DoD {
		if strings.TrimSpace(d) == "" {
			addIssue(fmt.Sprintf("/dod/%d", i), ReasonValidationFailed, "entrada vacia en dod")
		}
		checkIndexProseSafety(d, fmt.Sprintf("/dod/%d", i), addIssue)
	}
	for i, bf := range spec.BrainFollowups {
		if strings.TrimSpace(bf.TargetHint) == "" || strings.TrimSpace(bf.Summary) == "" || strings.TrimSpace(bf.Trigger) == "" {
			addIssue(fmt.Sprintf("/brain_followups/%d", i), ReasonValidationFailed, "brain_followup requiere target_hint, summary y trigger")
		}
		checkIndexProseSafety(bf.TargetHint+" "+bf.Summary+" "+bf.Trigger, fmt.Sprintf("/brain_followups/%d", i), addIssue)
	}

	// 4. Rutas de alcance restrictivo (read_only_paths, global_forbidden)
	cleanReadOnly := validateScopePathList(spec.ReadOnlyPaths, absRepo, true, "/read_only_paths", addIssue)
	cleanGlobalForbidden := validateScopePathList(spec.GlobalForbidden, absRepo, true, "/global_forbidden", addIssue)

	// 5. work_units[] (cantidad, IDs, DAG, can_modify, forbidden, scope_expansions, acceptance, verify_steps, failure_proof)
	maxUnits := MaxWorkUnitsASDD
	if spec.Lane == "FTD" {
		maxUnits = MaxWorkUnitsFTD
	}
	if len(spec.WorkUnits) == 0 || len(spec.WorkUnits) > maxUnits {
		addIssue("/work_units", ReasonValidationFailed, fmt.Sprintf("work_units debe tener entre 1 y %d elementos para carril %s", maxUnits, spec.Lane))
	}

	unitIdxByID := make(map[string]int, len(spec.WorkUnits))
	globalVerifyIDs := make(map[string]string)      // stepID -> unitID
	globalAcceptKind := make(map[string]string)     // acceptID -> kind
	unitCanModifyClean := make([][]string, len(spec.WorkUnits))
	unitForbiddenClean := make([][]string, len(spec.WorkUnits))
	unitSerializedCmds := make([][]string, len(spec.WorkUnits))

	affectedSet := make(map[string]bool, len(ctx.WorkItem.AffectedPaths))
	for _, ap := range ctx.WorkItem.AffectedPaths {
		affectedSet[strings.ToLower(bfcore.NormalizeCoverPath(ap))] = true
	}

	allCanModifyUnion := make([]string, 0)
	allCanModifySeen := make(map[string]bool)
	totalScopeExpansions := 0

	for i, u := range spec.WorkUnits {
		uPtr := fmt.Sprintf("/work_units/%d", i)
		if !reUnitID.MatchString(u.ID) {
			addIssue(uPtr+"/id", ReasonValidationFailed, fmt.Sprintf("work_unit id %q invalido (patron ^[a-z0-9][a-z0-9_]{1,47}$)", u.ID))
		} else if _, dup := unitIdxByID[u.ID]; dup {
			addIssue(uPtr+"/id", ReasonValidationFailed, fmt.Sprintf("work_unit id duplicado %q", u.ID))
		} else {
			unitIdxByID[u.ID] = i
		}
		if strings.TrimSpace(u.Title) == "" || len(u.Title) > 120 || hasLineBreak(u.Title) {
			addIssue(uPtr+"/title", ReasonValidationFailed, "work_unit.title obligatorio (1..120 chars, una sola linea)")
		}
		if strings.TrimSpace(u.Objective) == "" {
			addIssue(uPtr+"/objective", ReasonValidationFailed, "work_unit.objective obligatorio")
		} else {
			checkChunkProseSafety(u.Objective, uPtr+"/objective", false, addIssue)
		}
		for j, inp := range u.Inputs {
			iPtr := fmt.Sprintf("%s/inputs/%d", uPtr, j)
			if strings.TrimSpace(inp) == "" || hasLineBreak(inp) {
				addIssue(iPtr, ReasonValidationFailed, "inputs[] debe ser no vacio y de una sola linea")
			} else {
				checkChunkProseSafety(inp, iPtr, true, addIssue)
			}
		}

		// can_modify (archivos concretos en v1, AllowDir: false)
		if len(u.CanModify) == 0 {
			addIssue(uPtr+"/can_modify", ReasonValidationFailed, "work_unit.can_modify requiere al menos 1 archivo concreto")
		}
		cleanCM := validateScopePathList(u.CanModify, absRepo, false, uPtr+"/can_modify", addIssue)
		unitCanModifyClean[i] = cleanCM
		for _, p := range cleanCM {
			low := strings.ToLower(p)
			if !allCanModifySeen[low] {
				allCanModifySeen[low] = true
				allCanModifyUnion = append(allCanModifyUnion, p)
			}
		}

		// forbidden por chunk (AllowDir: true)
		cleanForb := validateScopePathList(u.Forbidden, absRepo, true, uPtr+"/forbidden", addIssue)
		unitForbiddenClean[i] = cleanForb

		// Verificar no sombreado de can_modify por read_only_paths, global_forbidden ni forbidden del chunk
		for j, cm := range cleanCM {
			if scopeUnderAny(cm, cleanReadOnly) {
				addIssue(fmt.Sprintf("%s/can_modify/%d", uPtr, j), ReasonValidationFailed, fmt.Sprintf("ruta %q sombreada por read_only_paths (ring.go bloquea antes que CanModify)", cm))
			}
			if scopeUnderAny(cm, cleanGlobalForbidden) {
				addIssue(fmt.Sprintf("%s/can_modify/%d", uPtr, j), ReasonValidationFailed, fmt.Sprintf("ruta %q sombreada por global_forbidden", cm))
			}
			if scopeUnderAny(cm, cleanForb) {
				addIssue(fmt.Sprintf("%s/can_modify/%d", uPtr, j), ReasonValidationFailed, fmt.Sprintf("ruta %q sombreada por work_unit.forbidden", cm))
			}
		}

		// scope_expansions obligatorio para toda ruta en can_modify \ affected_paths
		expMap := make(map[string]bool, len(u.ScopeExpansions))
		for j, se := range u.ScopeExpansions {
			sePtr := fmt.Sprintf("%s/scope_expansions/%d", uPtr, j)
			if strings.Contains(se.Path, "`") {
				addIssue(sePtr+"/path", ReasonValidationFailed, "backtick prohibido en scope_expansions.path")
			}
			cleanSE, rej := bfcore.SanitizeRelativePaths([]string{se.Path}, absRepo, bfcore.PathSanitizeOpts{
				AllowDir: false, SplitCommas: false, FoldCaseDedupe: true,
			})
			if rej != "" || len(cleanSE) != 1 {
				addIssue(sePtr+"/path", ReasonValidationFailed, fmt.Sprintf("ruta invalida en scope_expansions (%s)", rej))
			} else {
				expMap[strings.ToLower(cleanSE[0])] = true
			}
			if strings.TrimSpace(se.Rationale) == "" {
				addIssue(sePtr+"/rationale", ReasonValidationFailed, "scope_expansions.rationale obligatorio")
			}
		}
		for _, cm := range cleanCM {
			low := strings.ToLower(cm)
			if !affectedSet[low] {
				totalScopeExpansions++
				if !expMap[low] {
					addIssue(uPtr+"/scope_expansions", ReasonValidationFailed, fmt.Sprintf("ruta expandida %q fuera de WorkItem.affected_paths exige entrada en scope_expansions[]", cm))
				}
			}
		}

		// verify_steps[] (>= 1 por unidad, IDs unicos globalmente, allowlist cerrado)
		if len(u.VerifySteps) == 0 {
			addIssue(uPtr+"/verify_steps", ReasonValidationFailed, "cada work_unit exige al menos 1 verify_step valido")
		}
		unitVerifyIDs := make(map[string]bool, len(u.VerifySteps))
		for j, vs := range u.VerifySteps {
			vsPtr := fmt.Sprintf("%s/verify_steps/%d", uPtr, j)
			if strings.TrimSpace(vs.ID) == "" {
				addIssue(vsPtr+"/id", ReasonValidationFailed, "verify_step.id obligatorio")
			} else if prevUnit, dup := globalVerifyIDs[vs.ID]; dup {
				addIssue(vsPtr+"/id", ReasonValidationFailed, fmt.Sprintf("verify_step.id %q duplicado (ya usado en %s)", vs.ID, prevUnit))
			} else {
				globalVerifyIDs[vs.ID] = u.ID
				unitVerifyIDs[vs.ID] = true
			}
			validateVerifyStepAllowlist(vs, vsPtr, addIssue)
			unitSerializedCmds[i] = append(unitSerializedCmds[i], SerializeVerifyStepCmd(vs))
		}

		// acceptance_criteria[] (>= 1 por unidad, IDs unicos globalmente)
		if len(u.AcceptanceCriteria) == 0 {
			addIssue(uPtr+"/acceptance_criteria", ReasonValidationFailed, "cada work_unit exige al menos 1 acceptance_criterion")
		}
		for j, ac := range u.AcceptanceCriteria {
			acPtr := fmt.Sprintf("%s/acceptance_criteria/%d", uPtr, j)
			if strings.TrimSpace(ac.ID) == "" {
				addIssue(acPtr+"/id", ReasonValidationFailed, "acceptance_criterion.id obligatorio")
			} else if _, dup := globalAcceptKind[ac.ID]; dup {
				addIssue(acPtr+"/id", ReasonValidationFailed, fmt.Sprintf("acceptance_criterion.id duplicado %q", ac.ID))
			} else {
				globalAcceptKind[ac.ID] = ac.Kind
			}
			if strings.TrimSpace(ac.Description) == "" || hasLineBreak(ac.Description) {
				addIssue(acPtr+"/description", ReasonValidationFailed, "acceptance_criterion.description obligatorio y de una sola linea sin saltos de linea")
			}
			switch ac.Kind {
			case "command":
				if strings.TrimSpace(ac.VerifyStepID) == "" || !unitVerifyIDs[ac.VerifyStepID] {
					addIssue(acPtr+"/verify_step_id", ReasonValidationFailed, fmt.Sprintf("acceptance_criterion command debe referenciar un verify_step_id de la unidad (recibido %q)", ac.VerifyStepID))
				}
			case "review_gate":
				if !allowedReviewExecutors[ac.Executor] {
					addIssue(acPtr+"/executor", ReasonValidationFailed, fmt.Sprintf("acceptance_criterion review_gate exige executor en {4_Revisores, Modo_QA, AUTOR}, recibido %q", ac.Executor))
				}
			default:
				addIssue(acPtr+"/kind", ReasonValidationFailed, fmt.Sprintf("acceptance_criterion.kind invalido %q", ac.Kind))
			}
		}

		// failure_proof
		if strings.TrimSpace(u.FailureProof.Description) == "" {
			addIssue(uPtr+"/failure_proof/description", ReasonValidationFailed, "failure_proof.description obligatorio")
		} else {
			checkChunkProseSafety(u.FailureProof.Description, uPtr+"/failure_proof/description", false, addIssue)
		}
		if u.FailureProof.ExpectExit < 1 || u.FailureProof.ExpectExit > 255 {
			addIssue(uPtr+"/failure_proof/expect_exit", ReasonValidationFailed, "failure_proof.expect_exit debe estar entre 1 y 255")
		}
	}

	// 6. DAG aciclico en depends_on y orden topologico + can_modify disjunto sin dependencia transitiva
	topoOrder, reachMap, dagOK := buildTopologicalOrderAndReachability(spec.WorkUnits, unitIdxByID, addIssue)
	if dagOK {
		for i := 0; i < len(spec.WorkUnits); i++ {
			for j := i + 1; j < len(spec.WorkUnits); j++ {
				idA, idB := spec.WorkUnits[i].ID, spec.WorkUnits[j].ID
				if reachMap[idA][idB] || reachMap[idB][idA] {
					continue
				}
				setA := make(map[string]bool, len(unitCanModifyClean[i]))
				for _, p := range unitCanModifyClean[i] {
					setA[strings.ToLower(p)] = true
				}
				for _, p := range unitCanModifyClean[j] {
					if setA[strings.ToLower(p)] {
						addIssue(fmt.Sprintf("/work_units/%d/can_modify", j), ReasonValidationFailed,
							fmt.Sprintf("ruta %q compartida entre unidades %q y %q sin dependencia transitiva en depends_on", p, idA, idB))
					}
				}
			}
		}
	}

	// 7. decisions[] obligatorio en ascenso FTD -> ASDD y verificacion de decision activa
	currentScopeSHA := ComputeScopeSHA256(allCanModifyUnion)
	if ctx.WorkItem.Lane == "FTD" && spec.Lane == "ASDD" && len(spec.Decisions) == 0 {
		addIssue("/decisions", ReasonValidationFailed, "ascenso de carril FTD -> ASDD exige al menos un registro en decisions[]")
	}
	for i, d := range spec.Decisions {
		dPtr := fmt.Sprintf("/decisions/%d", i)
		if strings.TrimSpace(d.Reason) == "" {
			addIssue(dPtr+"/reason", ReasonValidationFailed, "decision.reason obligatorio")
		}
		if strings.TrimSpace(d.DecidedBy) == "" {
			addIssue(dPtr+"/decided_by", ReasonValidationFailed, "decision.decided_by obligatorio")
		}
		if strings.TrimSpace(d.Rationale) == "" {
			addIssue(dPtr+"/rationale", ReasonValidationFailed, "decision.rationale obligatorio")
		}
		if _, tErr := time.Parse(time.RFC3339, d.Timestamp); tErr != nil {
			addIssue(dPtr+"/timestamp", ReasonValidationFailed, "decision.timestamp debe ser RFC3339 valido")
		}
		if (d.LaneFrom != "FTD" && d.LaneFrom != "ASDD") || (d.LaneTo != "FTD" && d.LaneTo != "ASDD") {
			addIssue(dPtr+"/lane_to", ReasonValidationFailed, "decision.lane_from y lane_to deben ser FTD o ASDD")
		}
		cleanAppr, rej := bfcore.SanitizeRelativePaths(d.ApprovedPaths, absRepo, bfcore.PathSanitizeOpts{
			AllowDir: false, SplitCommas: false, FoldCaseDedupe: true,
		})
		if rej != "" || len(cleanAppr) == 0 {
			addIssue(dPtr+"/approved_paths", ReasonValidationFailed, fmt.Sprintf("decision.approved_paths invalido (%s)", rej))
		} else if !reHex64.MatchString(d.ScopeSHA256) || ComputeScopeSHA256(cleanAppr) != d.ScopeSHA256 {
			addIssue(dPtr+"/scope_sha256", ReasonValidationFailed, "decision.scope_sha256 no coincide con ComputeScopeSHA256(approved_paths)")
		}
	}
	if len(spec.Decisions) > 0 {
		lastDec := spec.Decisions[len(spec.Decisions)-1]
		if lastDec.ScopeSHA256 != currentScopeSHA {
			addIssue("/decisions", ReasonValidationFailed, fmt.Sprintf("la decision activa decisions[%d].scope_sha256 (%s) no coincide con el alcance actual can_modify (%s)",
				len(spec.Decisions)-1, lastDec.ScopeSHA256, currentScopeSHA))
		}
	}

	// 8. G5 citations (existencia, linea 1-based CRLF/BOM, ancla literal +-5 lineas)
	if len(spec.G5Citations) == 0 {
		addIssue("/g5_citations", ReasonValidationFailed, "se requiere al menos 1 cita verificable en g5_citations[]")
	}
	g5CleanPaths := make([]string, 0, len(spec.G5Citations))
	for i, cit := range spec.G5Citations {
		cPtr := fmt.Sprintf("/g5_citations/%d", i)
		if strings.Contains(cit.Path, "`") {
			addIssue(cPtr+"/path", ReasonValidationFailed, "backtick prohibido en g5_citations.path")
			continue
		}
		cleanP, rej := bfcore.SanitizeRelativePaths([]string{cit.Path}, absRepo, bfcore.PathSanitizeOpts{
			AllowDir: false, SplitCommas: false, FoldCaseDedupe: true,
		})
		if rej != "" || len(cleanP) != 1 {
			addIssue(cPtr+"/path", ReasonValidationFailed, fmt.Sprintf("ruta invalida en g5_citations (%s)", rej))
			continue
		}
		g5CleanPaths = append(g5CleanPaths, cleanP[0])
		if cit.Line < 1 {
			addIssue(cPtr+"/line", ReasonValidationFailed, "g5_citations.line debe ser >= 1")
		}
		anchor := strings.TrimSpace(cit.SymbolAnchor)
		if len(anchor) < 4 || trivialG5Anchors[strings.ToLower(anchor)] {
			addIssue(cPtr+"/symbol_anchor", ReasonValidationFailed, fmt.Sprintf("symbol_anchor %q demasiado corto (<4) o reservado trivial", cit.SymbolAnchor))
		}
		if strings.TrimSpace(cit.Claim) == "" {
			addIssue(cPtr+"/claim", ReasonValidationFailed, "g5_citations.claim obligatorio")
		}
		validateG5CitationOnDisk(absRepo, cleanP[0], cit.Line, anchor, cPtr, addIssue)
	}

	// 9. observed_files[] completitud exacta (affected_paths U g5_citations U can_modify) + capitalizacion NTFS
	expectedObs := make(map[string]string)
	for _, p := range ctx.WorkItem.AffectedPaths {
		norm := bfcore.NormalizeCoverPath(p)
		if norm != "" {
			expectedObs[strings.ToLower(norm)] = norm
		}
	}
	for _, p := range g5CleanPaths {
		expectedObs[strings.ToLower(p)] = p
	}
	for _, p := range allCanModifyUnion {
		expectedObs[strings.ToLower(p)] = p
	}

	seenObs := make(map[string]ObservedFile, len(spec.ObservedFiles))
	for i, of := range spec.ObservedFiles {
		oPtr := fmt.Sprintf("/observed_files/%d", i)
		if strings.Contains(of.Path, "`") {
			addIssue(oPtr+"/path", ReasonValidationFailed, "backtick prohibido en observed_files.path")
			continue
		}
		cleanP, rej := bfcore.SanitizeRelativePaths([]string{of.Path}, absRepo, bfcore.PathSanitizeOpts{
			AllowDir: false, SplitCommas: false, FoldCaseDedupe: true,
		})
		if rej != "" || len(cleanP) != 1 {
			addIssue(oPtr+"/path", ReasonValidationFailed, fmt.Sprintf("ruta invalida en observed_files (%s)", rej))
			continue
		}
		key := strings.ToLower(cleanP[0])
		if _, dup := seenObs[key]; dup {
			addIssue(oPtr+"/path", ReasonValidationFailed, fmt.Sprintf("ruta duplicada en observed_files: %q", cleanP[0]))
		}
		seenObs[key] = ObservedFile{Path: cleanP[0], State: of.State, SHA256: of.SHA256}
		switch of.State {
		case "present":
			if !reHex64.MatchString(of.SHA256) {
				addIssue(oPtr+"/sha256", ReasonValidationFailed, "observed_file state=present exige sha256 de 64 hex")
			}
			if caseErr := verifyExactNTFSSegmentCasing(absRepo, cleanP[0]); caseErr != nil {
				addIssue(oPtr+"/path", ReasonValidationFailed, caseErr.Error())
			}
		case "absent":
			if of.SHA256 != "" {
				addIssue(oPtr+"/sha256", ReasonValidationFailed, "observed_file state=absent exige sha256 vacio")
			}
		default:
			addIssue(oPtr+"/state", ReasonValidationFailed, fmt.Sprintf("observed_file.state invalido %q (use present o absent)", of.State))
		}
	}
	for k, p := range expectedObs {
		if _, ok := seenObs[k]; !ok {
			addIssue("/observed_files", ReasonValidationFailed, fmt.Sprintf("observed_files omite ruta requerida %q", p))
		}
	}
	for k, of := range seenObs {
		if _, ok := expectedObs[k]; !ok {
			addIssue("/observed_files", ReasonValidationFailed, fmt.Sprintf("observed_files contiene ruta no referenciada %q", of.Path))
		}
	}
	// Verificar capitalizacion exacta en disco para rutas can_modify que existan ("present")
	for i, cmList := range unitCanModifyClean {
		for j, cm := range cmList {
			if st, _, sErr := bfcore.ComputeNormalizedFileSHA256(absRepo, cm); sErr == nil && st == "present" {
				if caseErr := verifyExactNTFSSegmentCasing(absRepo, cm); caseErr != nil {
					addIssue(fmt.Sprintf("/work_units/%d/can_modify/%d", i, j), ReasonValidationFailed, caseErr.Error())
				}
			}
		}
	}

	// 10. risk_mitigations[] cubriendo 100% de WorkItem.RiskHits U reQueriedRiskHits + HITL unidireccional
	mergedRiskHits := mergeRiskHits(ctx.WorkItem.RiskHits, reQueriedRiskHits)
	hasAnyCritico := false
	for _, rh := range mergedRiskHits {
		if strings.EqualFold(rh.Level, "CRITICO") {
			hasAnyCritico = true
		}
	}
	mustRequireHITL := ctx.WorkItem.RequiresAutorHITL || hasAnyCritico || escalationOccurred
	if mustRequireHITL && !spec.RequiresAutorHITL {
		addIssue("/requires_autor_hitl", ReasonValidationFailed, "requires_autor_hitl es unidireccional y debe ser true (WorkItem HITL, CRITICO o escalado)")
	}

	mitByID := make(map[string]RiskMitigation, len(spec.RiskMitigations))
	for i, rm := range spec.RiskMitigations {
		rmPtr := fmt.Sprintf("/risk_mitigations/%d", i)
		idKey := strings.ToUpper(strings.TrimSpace(rm.RiskID))
		if idKey == "" {
			addIssue(rmPtr+"/risk_id", ReasonValidationFailed, "risk_mitigation.risk_id obligatorio")
		} else {
			mitByID[idKey] = rm
		}
		if bfcore.RiskLevelRank(rm.Level) == 0 {
			addIssue(rmPtr+"/level", ReasonValidationFailed, fmt.Sprintf("risk_mitigation.level invalido %q", rm.Level))
		}
		for j, vsID := range rm.VerifyStepIDs {
			if _, ok := globalVerifyIDs[vsID]; !ok {
				addIssue(fmt.Sprintf("%s/verify_step_ids/%d", rmPtr, j), ReasonValidationFailed, fmt.Sprintf("verify_step_id %q inexistente", vsID))
			}
		}
		for j, rgID := range rm.ReviewGates {
			kind, ok := globalAcceptKind[rgID]
			if !ok || kind != "review_gate" {
				addIssue(fmt.Sprintf("%s/review_gates/%d", rmPtr, j), ReasonValidationFailed, fmt.Sprintf("review_gate %q debe referenciar un acceptance_criterion con kind=review_gate", rgID))
			}
		}
		if strings.EqualFold(rm.Level, "CRITICO") {
			if !rm.RequiresAutorHITL {
				addIssue(rmPtr+"/requires_autor_hitl", ReasonValidationFailed, "zona CRITICO exige requires_autor_hitl: true")
			}
			if len(rm.ReviewGates) == 0 {
				addIssue(rmPtr+"/review_gates", ReasonValidationFailed, "zona CRITICO exige al menos 1 review_gate")
			}
		}
	}
	for _, rh := range mergedRiskHits {
		idKey := strings.ToUpper(strings.TrimSpace(rh.ID))
		rm, ok := mitByID[idKey]
		if !ok {
			addIssue("/risk_mitigations", ReasonValidationFailed, fmt.Sprintf("falta mitigacion para zona de riesgo %s (%s)", rh.ID, rh.Level))
			continue
		}
		if bfcore.RiskLevelRank(rm.Level) < bfcore.RiskLevelRank(rh.Level) {
			addIssue("/risk_mitigations", ReasonValidationFailed, fmt.Sprintf("mitigacion para %s declara nivel %s inferior al real %s", rh.ID, rm.Level, rh.Level))
		}
		if len(rm.VerifyStepIDs) == 0 && len(rm.ReviewGates) == 0 {
			addIssue("/risk_mitigations", ReasonValidationFailed, fmt.Sprintf("mitigacion para %s debe enlazar al menos un verify_step_id o review_gate", rh.ID))
		}
	}

	// 11. ASDD --stage final: validar goldfish_review.json
	goldfishSHA := ""
	if stage == "final" && spec.Lane == "ASDD" {
		canonicalSHA, _ := CanonicalSpecSHA256(spec)
		requiredLenses := computeRequiredGoldfishLenses(ctx.WorkItem.ImpactFlags, hasAnyCritico || strings.EqualFold(ctx.WorkItem.RiskLevel, "CRITICO"), len(spec.WorkUnits))
		gfSHA, gfIssues := validateGoldfishReviewFile(wiDir, canonicalSHA, requiredLenses)
		issues = append(issues, gfIssues...)
		goldfishSHA = gfSHA
	}

	// 12. Renderizado en memoria de plantillas GengiSWAP + superconjunto validateChunks
	var rendered RenderedTaskFiles
	if len(issues) == 0 {
		var rErr error
		rendered, rErr = renderGengisTaskInMemory(
			spec, ctx, taskIDHint, topoOrder,
			allCanModifyUnion, cleanReadOnly, cleanGlobalForbidden,
			unitCanModifyClean, unitForbiddenClean, unitSerializedCmds,
			hasAnyCritico, escalationOccurred || len(spec.Decisions) > 0, totalScopeExpansions,
		)
		if rErr != nil {
			addIssue("/", ReasonValidationFailed, fmt.Sprintf("error renderizando plantillas GengiSWAP: %v", rErr))
		} else if vcIssues := verifyRenderedGengisParity(rendered); len(vcIssues) > 0 {
			issues = append(issues, vcIssues...)
		}
	}

	return issues, rendered, goldfishSHA, nil
}

func validateScopePathList(raw []string, absRepo string, allowDir bool, ptr string, addIssue func(string, string, string)) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for i, p := range raw {
		iPtr := fmt.Sprintf("%s/%d", ptr, i)
		if strings.Contains(p, "`") {
			addIssue(iPtr, ReasonValidationFailed, "caracter backtick (`) prohibido en rutas de alcance")
			continue
		}
		clean, rej := bfcore.SanitizeRelativePaths([]string{p}, absRepo, bfcore.PathSanitizeOpts{
			AllowDir:       allowDir,
			SplitCommas:    false,
			FoldCaseDedupe: true,
		})
		if rej != "" || len(clean) != 1 {
			addIssue(iPtr, ReasonValidationFailed, fmt.Sprintf("ruta de alcance invalida %q (%s)", p, rej))
			continue
		}
		key := strings.ToLower(clean[0])
		if !seen[key] {
			seen[key] = true
			out = append(out, clean[0])
		}
	}
	return out
}

func scopeUnderAny(filePath string, prefixes []string) bool {
	fNorm := strings.TrimSuffix(path.Clean(strings.ReplaceAll(filePath, `\`, "/")), "/")
	fLow := strings.ToLower(fNorm)
	for _, pre := range prefixes {
		pNorm := strings.TrimSuffix(path.Clean(strings.ReplaceAll(pre, `\`, "/")), "/")
		pLow := strings.ToLower(pNorm)
		if pLow == "." || fLow == pLow || strings.HasPrefix(fLow, pLow+"/") {
			return true
		}
	}
	return false
}

func verifyExactNTFSSegmentCasing(absRepo, relPath string) error {
	segs := strings.Split(relPath, "/")
	curDir := absRepo
	for _, seg := range segs {
		entries, err := os.ReadDir(curDir)
		if err != nil {
			return nil // si falla lectura no emitir falso positivo de casing
		}
		matchedExact := false
		matchedFold := ""
		for _, e := range entries {
			name := e.Name()
			if name == seg {
				matchedExact = true
				break
			}
			if strings.EqualFold(name, seg) {
				matchedFold = name
			}
		}
		if !matchedExact && matchedFold != "" {
			return fmt.Errorf("capitalizacion de ruta %q no coincide con disco (%q vs %q); ring.go compara alcance sensible a mayusculas", relPath, seg, matchedFold)
		}
		curDir = filepath.Join(curDir, seg)
	}
	return nil
}

func buildTopologicalOrderAndReachability(units []WorkUnit, idxByID map[string]int, addIssue func(string, string, string)) ([]int, map[string]map[string]bool, bool) {
	n := len(units)
	inDeg := make([]int, n)
	adj := make([][]int, n)
	validEdges := true

	for i, u := range units {
		for j, dep := range u.DependsOn {
			depIdx, ok := idxByID[dep]
			if !ok {
				addIssue(fmt.Sprintf("/work_units/%d/depends_on/%d", i, j), ReasonValidationFailed, fmt.Sprintf("dependencia desconocida %q", dep))
				validEdges = false
				continue
			}
			if depIdx == i {
				addIssue(fmt.Sprintf("/work_units/%d/depends_on/%d", i, j), ReasonValidationFailed, fmt.Sprintf("auto-dependencia ciclica en %q", u.ID))
				validEdges = false
				continue
			}
			adj[depIdx] = append(adj[depIdx], i)
			inDeg[i]++
		}
	}
	if !validEdges {
		return nil, nil, false
	}

	// Kahn determinista preservando orden por indice original cuando inDeg == 0
	var queue []int
	for i := 0; i < n; i++ {
		if inDeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	order := make([]int, 0, n)
	for len(queue) > 0 {
		sort.Ints(queue)
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, nxt := range adj[cur] {
			inDeg[nxt]--
			if inDeg[nxt] == 0 {
				queue = append(queue, nxt)
			}
		}
	}
	if len(order) != n {
		addIssue("/work_units", ReasonValidationFailed, "ciclo detectado en depends_on de work_units[]")
		return nil, nil, false
	}

	// Alcanzabilidad transitiva: reach[A][B] = true si A depende transitivamente de B
	reach := make(map[string]map[string]bool, n)
	for _, idx := range order {
		id := units[idx].ID
		m := make(map[string]bool)
		for _, dep := range units[idx].DependsOn {
			m[dep] = true
			for anc := range reach[dep] {
				m[anc] = true
			}
		}
		reach[id] = m
	}
	return order, reach, true
}

func validateG5CitationOnDisk(absRepo, relPath string, line int, anchor, ptr string, addIssue func(string, string, string)) {
	absFile := filepath.Join(absRepo, filepath.FromSlash(relPath))
	if err := bfcore.CheckNoReparseChain(absRepo, absFile); err != nil {
		addIssue(ptr+"/path", ReasonValidationFailed, fmt.Sprintf("reparse point en archivo citado %q", relPath))
		return
	}
	fi, err := os.Lstat(absFile)
	if err != nil {
		addIssue(ptr+"/path", ReasonValidationFailed, fmt.Sprintf("archivo citado G5 %q no existe en disco", relPath))
		return
	}
	if fi.IsDir() || bfcore.IsReparseOrSymlink(fi) || !fi.Mode().IsRegular() || fi.Size() > MaxObservedFileBytes {
		addIssue(ptr+"/path", ReasonValidationFailed, fmt.Sprintf("archivo citado G5 %q no es un archivo regular legible", relPath))
		return
	}
	raw, err := os.ReadFile(absFile)
	if err != nil {
		addIssue(ptr+"/path", ReasonValidationFailed, fmt.Sprintf("error leyendo archivo citado %q", relPath))
		return
	}
	text := string(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if line < 1 || line > len(lines) {
		addIssue(ptr+"/line", ReasonValidationFailed, fmt.Sprintf("linea %d fuera de rango en %q (1..%d)", line, relPath, len(lines)))
		return
	}
	if len(anchor) < 4 || trivialG5Anchors[strings.ToLower(anchor)] {
		return
	}
	lo := line - 5
	if lo < 1 {
		lo = 1
	}
	hi := line + 5
	if hi > len(lines) {
		hi = len(lines)
	}
	found := false
	for ln := lo; ln <= hi; ln++ {
		if strings.Contains(lines[ln-1], anchor) {
			found = true
			break
		}
	}
	if !found {
		addIssue(ptr+"/symbol_anchor", ReasonValidationFailed, fmt.Sprintf("ancla literal %q no encontrada en %s:%d..%d", anchor, relPath, lo, hi))
	}
}

func validateVerifyStepAllowlist(vs VerifyStep, ptr string, addIssue func(string, string, string)) {
	if vs.ExpectExit != 0 {
		addIssue(ptr+"/expect_exit", ReasonValidationFailed, "verify_step.expect_exit debe ser exactamente 0")
	}
	if vs.TimeoutSec < 1 || vs.TimeoutSec > MaxVerifyTimeoutSec {
		addIssue(ptr+"/timeout_sec", ReasonValidationFailed, fmt.Sprintf("verify_step.timeout_sec debe estar entre 1 y %d", MaxVerifyTimeoutSec))
	}

	// Validar cwd
	if vs.Cwd != "" {
		if hasUnsafeCmdChars(vs.Cwd) {
			addIssue(ptr+"/cwd", ReasonValidationFailed, "caracteres inseguros en verify_step.cwd")
		} else {
			_, rej := bfcore.SanitizeRelativePaths([]string{vs.Cwd}, "", bfcore.PathSanitizeOpts{
				AllowDir: true, SplitCommas: false, FoldCaseDedupe: false,
			})
			if rej != "" {
				addIssue(ptr+"/cwd", ReasonValidationFailed, fmt.Sprintf("cwd invalido %q (%s)", vs.Cwd, rej))
			}
		}
	}

	// Validar env (tabla cerrada)
	for k, v := range vs.Env {
		if hasUnsafeCmdChars(k) || hasUnsafeCmdChars(v) {
			addIssue(ptr+"/env/"+escapeJSONPointer(k), ReasonValidationFailed, "caracteres inseguros en variable de entorno")
			continue
		}
		allowedVals, ok := allowedEnvValues[k]
		if !ok {
			addIssue(ptr+"/env/"+escapeJSONPointer(k), ReasonValidationFailed, fmt.Sprintf("variable de entorno %q fuera del allowlist permitido", k))
			continue
		}
		if !allowedVals[v] {
			addIssue(ptr+"/env/"+escapeJSONPointer(k), ReasonValidationFailed, fmt.Sprintf("valor %q no permitido para env %s", v, k))
		}
	}

	if len(vs.Argv) < 2 {
		addIssue(ptr+"/argv", ReasonValidationFailed, "verify_step.argv requiere al menos ejecutable y subcomando")
		return
	}
	for k, tok := range vs.Argv {
		if tok == "" || hasUnsafeCmdChars(tok) {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, k), ReasonValidationFailed, fmt.Sprintf("token argv[%d] vacio o con caracteres prohibidos", k))
			return
		}
	}

	exe := strings.ReplaceAll(vs.Argv[0], `\`, "/")
	sub := vs.Argv[1]
	rest := vs.Argv[2:]

	switch exe {
	case "go":
		switch sub {
		case "test":
			validateGoTestArgs(rest, ptr, addIssue)
		case "vet":
			validateGoVetArgs(rest, ptr, addIssue)
		default:
			addIssue(ptr+"/argv/1", ReasonValidationFailed, fmt.Sprintf("subcomando go %q fuera del allowlist (solo test, vet)", sub))
		}
	case "git":
		switch sub {
		case "status":
			validateGitSimpleArgs(rest, map[string]bool{"-s": true, "--short": true, "--porcelain": true, "--ignored": true}, true, ptr, addIssue)
		case "diff":
			validateGitSimpleArgs(rest, map[string]bool{"--stat": true, "--name-only": true, "--check": true, "--cached": true, "--staged": true}, true, ptr, addIssue)
		case "rev-parse":
			validateGitRevParseArgs(rest, ptr, addIssue)
		case "check-ignore":
			validateGitSimpleArgs(rest, map[string]bool{"-q": true, "-v": true}, true, ptr, addIssue)
		default:
			addIssue(ptr+"/argv/1", ReasonValidationFailed, fmt.Sprintf("subcomando git %q fuera del allowlist", sub))
		}
	case "0_Brain/brain.exe":
		if vs.Cwd != "" {
			addIssue(ptr+"/cwd", ReasonValidationFailed, "0_Brain/brain.exe exige cwd vacio (\"\")")
		}
		switch sub {
		case "query":
			validateBlackFactoryToolArgs(rest, map[string]string{
				"--vault": "relpath", "--db": "relpath", "--project": "text",
				"--type": "text", "--tag": "text", "--q": "text", "--limit": "int",
			}, map[string]bool{"--json": true}, ptr, addIssue)
		case "context":
			validateBlackFactoryToolArgs(rest, map[string]string{
				"--vault": "relpath", "--db": "relpath", "--project": "text",
				"--file": "relpath", "--files": "relpath_csv",
			}, map[string]bool{"--json": true}, ptr, addIssue)
		default:
			addIssue(ptr+"/argv/1", ReasonValidationFailed, fmt.Sprintf("subcomando 0_Brain/brain.exe %q fuera del allowlist (solo query, context; verify excluido)", sub))
		}
	case "1_Intake/intake.exe":
		if vs.Cwd != "" {
			addIssue(ptr+"/cwd", ReasonValidationFailed, "1_Intake/intake.exe exige cwd vacio (\"\")")
		}
		switch sub {
		case "list":
			validateBlackFactoryToolArgs(rest, map[string]string{
				"--data-dir": "relpath", "--project": "text", "--status": "text",
				"--lane": "text", "--risk": "text",
			}, map[string]bool{"--json": true}, ptr, addIssue)
		case "show":
			validateBlackFactoryToolArgs(rest, map[string]string{
				"--data-dir": "relpath", "--id": "text", "--format": "text",
			}, map[string]bool{"--json": true}, ptr, addIssue)
		default:
			addIssue(ptr+"/argv/1", ReasonValidationFailed, fmt.Sprintf("subcomando 1_Intake/intake.exe %q fuera del allowlist (solo list, show; verify excluido)", sub))
		}
	case "2_Spec_Writer/specwriter.exe":
		if vs.Cwd != "" {
			addIssue(ptr+"/cwd", ReasonValidationFailed, "2_Spec_Writer/specwriter.exe exige cwd vacio (\"\")")
		}
		switch sub {
		case "verify":
			validateBlackFactoryToolArgs(rest, map[string]string{
				"--factory-root": "relpath_or_dot", "--repo-root": "relpath_or_dot",
				"--vault": "relpath", "--data-dir": "relpath", "--brain-bin": "relpath",
			}, map[string]bool{"--json": true}, ptr, addIssue)
		case "render":
			validateBlackFactoryToolArgs(rest, map[string]string{
				"--spec": "relpath",
			}, map[string]bool{"--json": true}, ptr, addIssue)
		default:
			addIssue(ptr+"/argv/1", ReasonValidationFailed, fmt.Sprintf("subcomando 2_Spec_Writer/specwriter.exe %q fuera del allowlist (solo verify, render)", sub))
		}
	default:
		addIssue(ptr+"/argv/0", ReasonValidationFailed, fmt.Sprintf("ejecutable %q fuera del allowlist cerrado de v1", vs.Argv[0]))
	}
}

func hasUnsafeCmdChars(s string) bool {
	if strings.HasSuffix(s, `\`) || strings.HasSuffix(s, "`") {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			return true
		}
		switch c {
		case '&', '|', '<', '>', '^', '%', '!', '"', '\'', '(', ')', ';':
			return true
		}
	}
	return false
}

func normalizeFlagName(tok string) (name, val string, hasEq bool) {
	s := tok
	if strings.HasPrefix(s, "--") {
		s = "-" + s[2:]
	}
	if idx := strings.IndexByte(s, '='); idx > 0 {
		return s[:idx], s[idx+1:], true
	}
	return s, "", false
}

func isSafeRelativeGoPkgArg(arg string) bool {
	if arg == "." || arg == "./..." {
		return true
	}
	if strings.HasPrefix(arg, "./") {
		rest := strings.TrimPrefix(arg, "./")
		rest = strings.TrimSuffix(rest, "/...")
		if rest == "" || rest == "..." {
			return true
		}
		_, rej := bfcore.SanitizeRelativePaths([]string{rest}, "", bfcore.PathSanitizeOpts{
			AllowDir: true, SplitCommas: false, FoldCaseDedupe: false,
		})
		return rej == ""
	}
	return false
}

func validateGoTestArgs(args []string, ptr string, addIssue func(string, string, string)) {
	for i := 0; i < len(args); i++ {
		argIdx := i + 2
		tok := args[i]
		if !strings.HasPrefix(tok, "-") {
			if !isSafeRelativeGoPkgArg(tok) {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("argumento de paquete go test invalido %q (solo paquetes relativos ./...)", tok))
			}
			continue
		}
		name, val, hasEq := normalizeFlagName(tok)
		switch name {
		case "-v", "-short", "-race", "-cover":
			if hasEq && val != "true" && val != "false" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("valor booleano invalido en %q", tok))
			}
		case "-count":
			if !hasEq {
				if i+1 >= len(args) {
					addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, "-count requiere valor 1")
					return
				}
				i++
				val = args[i]
			}
			if val != "1" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, "go test solo permite -count=1")
			}
		case "-run":
			if !hasEq {
				if i+1 >= len(args) {
					addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, "-run requiere patron valido")
					return
				}
				i++
				val = args[i]
			}
			if !reGoTestRunPattern.MatchString(val) {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("patron -run=%q fuera de ^[A-Za-z0-9_$./-]+$ (sin ^)", val))
			}
		case "-timeout":
			if !hasEq {
				if i+1 >= len(args) {
					addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, "-timeout requiere duracion 1s..300s")
					return
				}
				i++
				val = args[i]
			}
			m := reGoTestTimeout.FindStringSubmatch(val)
			if m == nil {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("duracion -timeout=%q invalida (use 1s..300s)", val))
				continue
			}
			secs, _ := strconv.Atoi(m[1])
			if secs < 1 || secs > MaxVerifyTimeoutSec {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("duracion -timeout=%q fuera de 1s..300s", val))
			}
		default:
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("bandera go test %q prohibida por allowlist cerrado", tok))
		}
	}
}

func validateGoVetArgs(args []string, ptr string, addIssue func(string, string, string)) {
	for i, tok := range args {
		argIdx := i + 2
		if strings.HasPrefix(tok, "-") {
			name, _, hasEq := normalizeFlagName(tok)
			if name != "-v" || hasEq {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("bandera go vet %q fuera del allowlist (solo -v)", tok))
			}
			continue
		}
		if !isSafeRelativeGoPkgArg(tok) {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("argumento de paquete go vet invalido %q", tok))
		}
	}
}

func validateGitSimpleArgs(args []string, allowedFlags map[string]bool, allowRelPaths bool, ptr string, addIssue func(string, string, string)) {
	for i, tok := range args {
		argIdx := i + 2
		if strings.HasPrefix(tok, "-") {
			if !allowedFlags[tok] {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("bandera git %q fuera del allowlist", tok))
			}
			continue
		}
		if !allowRelPaths {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("argumento posicional %q no permitido", tok))
			continue
		}
		_, rej := bfcore.SanitizeRelativePaths([]string{tok}, "", bfcore.PathSanitizeOpts{
			AllowDir: true, SplitCommas: false, FoldCaseDedupe: false,
		})
		if rej != "" {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("ruta git invalida %q (%s)", tok, rej))
		}
	}
}

func validateGitRevParseArgs(args []string, ptr string, addIssue func(string, string, string)) {
	posCount := 0
	for i, tok := range args {
		argIdx := i + 2
		if strings.HasPrefix(tok, "-") {
			if tok != "--short" && tok != "--abbrev-ref" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("bandera git rev-parse %q fuera del allowlist", tok))
			}
			continue
		}
		posCount++
		if posCount > 1 || tok != "HEAD" {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("git rev-parse solo admite argumento posicional opcional HEAD, recibido %q", tok))
		}
	}
}

func validateBlackFactoryToolArgs(args []string, valueFlags map[string]string, boolFlags map[string]bool, ptr string, addIssue func(string, string, string)) {
	for i := 0; i < len(args); i++ {
		argIdx := i + 2
		tok := args[i]
		if !strings.HasPrefix(tok, "-") {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("argumento posicional inesperado %q en herramienta Black Factory", tok))
			continue
		}
		flagName := tok
		val := ""
		hasEq := false
		if eq := strings.IndexByte(tok, '='); eq > 0 {
			flagName = tok[:eq]
			val = tok[eq+1:]
			hasEq = true
		}
		// Normalizar -flag -> --flag para comprobar en tablas con prefijo --
		canonicalFlag := flagName
		if strings.HasPrefix(canonicalFlag, "-") && !strings.HasPrefix(canonicalFlag, "--") {
			canonicalFlag = "-" + canonicalFlag
		}
		if boolFlags[canonicalFlag] {
			if hasEq && val != "true" && val != "false" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("valor booleano invalido en %q", tok))
			}
			continue
		}
		kind, ok := valueFlags[canonicalFlag]
		if !ok {
			addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("bandera %q fuera del allowlist cerrado", tok))
			continue
		}
		if !hasEq {
			if i+1 >= len(args) {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("bandera %s requiere un valor", flagName))
				return
			}
			i++
			val = args[i]
		}
		switch kind {
		case "relpath":
			_, rej := bfcore.SanitizeRelativePaths([]string{val}, "", bfcore.PathSanitizeOpts{
				AllowDir: true, SplitCommas: false, FoldCaseDedupe: false,
			})
			if rej != "" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("valor de ruta %q invalido para %s (%s)", val, flagName, rej))
			}
		case "relpath_or_dot":
			if val != "." {
				_, rej := bfcore.SanitizeRelativePaths([]string{val}, "", bfcore.PathSanitizeOpts{
					AllowDir: true, SplitCommas: false, FoldCaseDedupe: false,
				})
				if rej != "" {
					addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("valor de ruta %q invalido para %s (%s)", val, flagName, rej))
				}
			}
		case "relpath_csv":
			parts := strings.Split(val, ",")
			_, rej := bfcore.SanitizeRelativePaths(parts, "", bfcore.PathSanitizeOpts{
				AllowDir: false, SplitCommas: false, FoldCaseDedupe: false,
			})
			if rej != "" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("lista CSV de rutas %q invalida para %s (%s)", val, flagName, rej))
			}
		case "int":
			n, convErr := strconv.Atoi(val)
			if convErr != nil || n < 1 || n > 1000 {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("entero invalido %q para %s", val, flagName))
			}
		case "text":
			if strings.TrimSpace(val) == "" {
				addIssue(fmt.Sprintf("%s/argv/%d", ptr, argIdx), ReasonValidationFailed, fmt.Sprintf("valor vacio para %s", flagName))
			}
		}
	}
}

// Guardas de inyeccion Markdown contra parseIndexFile y parseChunkFile de GengiSWAP/engine/bridge.go
func checkIndexProseSafety(text, ptr string, addIssue func(string, string, string)) {
	norm := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for _, ln := range strings.Split(norm, "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if reMarkdownHeading.MatchString(trimmed) {
			addIssue(ptr, ReasonValidationFailed, "prosa de index.md no puede contener encabezados Markdown (#)")
			return
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			addIssue(ptr, ReasonValidationFailed, "prosa de index.md no puede contener bloques fenced (``` o ~~~)")
			return
		}
		if reIndexMaxRetries.MatchString(trimmed) {
			addIssue(ptr, ReasonValidationFailed, "prosa de index.md no puede sobrescribir 'Max. intentos'")
			return
		}
		if key, ok := extractFieldLabelKey(trimmed); ok {
			nk := normalizeBridgeKey(key)
			if strings.HasPrefix(nk, "puede modificar") || strings.HasPrefix(nk, "solo lectura") ||
				strings.HasPrefix(nk, "prohibido") || strings.HasPrefix(nk, "fuera de alcance") ||
				strings.HasPrefix(nk, "no tocar") || nk == "objetivo" || nk == "objetivo generico" ||
				strings.Contains(nk, "repositorio destino") || strings.Contains(nk, "directorio de destino") {
				addIssue(ptr, ReasonValidationFailed, fmt.Sprintf("prosa de index.md contiene clave reservada de parseIndexFile (%q)", key))
				return
			}
		}
	}
}

func checkChunkProseSafety(text, ptr string, checkEvenIfBullet bool, addIssue func(string, string, string)) {
	norm := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for _, ln := range strings.Split(norm, "\n") {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if reMarkdownHeading.MatchString(trimmed) {
			addIssue(ptr, ReasonValidationFailed, "campo de chunk no puede contener encabezados Markdown (#)")
			return
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			addIssue(ptr, ReasonValidationFailed, "campo de chunk no puede contener bloques fenced (``` o ~~~)")
			return
		}
		isBullet := reListPrefixCheck.MatchString(ln)
		if !isBullet || checkEvenIfBullet {
			if key, ok := extractFieldLabelKey(trimmed); ok && classifyBridgeChunkField(key) != "" {
				addIssue(ptr, ReasonValidationFailed, fmt.Sprintf("campo de chunk contiene etiqueta reservada de parseChunkFile (%q)", key))
				return
			}
		}
	}
}

func extractFieldLabelKey(line string) (string, bool) {
	s := strings.TrimSpace(line)
	if s == "" {
		return "", false
	}
	if reListPrefixCheck.MatchString(s) {
		s = strings.TrimSpace(reListPrefixCheck.ReplaceAllString(s, ""))
	}
	for _, marker := range []string{"**", "*"} {
		if strings.HasPrefix(s, marker) {
			rest := s[len(marker):]
			if end := strings.Index(rest, marker); end > 0 {
				inner := strings.TrimSpace(rest[:end])
				after := strings.TrimSpace(rest[end+len(marker):])
				if strings.HasSuffix(inner, ":") {
					return strings.TrimSuffix(inner, ":"), true
				}
				if strings.HasPrefix(after, ":") {
					return inner, true
				}
			}
		}
	}
	colon := strings.IndexByte(s, ':')
	if colon <= 1 {
		return "", false
	}
	if colon+1 < len(s) && (s[colon+1] == '\\' || s[colon+1] == '/') {
		return "", false
	}
	k := strings.TrimSpace(strings.Trim(s[:colon], "*`"))
	if k == "" || len(k) > 80 {
		return "", false
	}
	return k, true
}

func normalizeBridgeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, "*:` ")
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u")
	return strings.TrimSpace(r.Replace(s))
}

func classifyBridgeChunkField(label string) string {
	nk := normalizeBridgeKey(label)
	switch nk {
	case "descripcion", "spec", "objetivo":
		return "spec"
	case "criterio de aceptacion", "criterios de aceptacion", "acceptance":
		return "acceptance"
	case "verificacion", "verificacion cli", "comando de verificacion", "comandos de verificacion":
		return "verify"
	case "prueba de que puede fallar", "prueba que puede fallar", "prueba de fallo", "condicion de fallo":
		return "failure"
	case "fuera de alcance", "no tocar", "prohibido":
		return "forbidden"
	case "puede modificar":
		return "can_modify"
	case "solo lectura":
		return "read_only"
	case "reglas", "entradas", "salidas", "entregables", "notas", "contexto":
		return "other"
	default:
		return ""
	}
}

func computeRequiredGoldfishLenses(impactFlags []string, isCritico bool, unitCount int) []string {
	lenses := []string{"egm-design", "egm-codebase-details", "egm-implementation-readiness"}
	needStructural := isCritico
	for _, fl := range impactFlags {
		switch strings.ToLower(strings.TrimSpace(fl)) {
		case "contract", "schema", "db":
			needStructural = true
		}
	}
	if needStructural {
		lenses = append(lenses, "egm-structural")
	}
	if unitCount > 3 {
		lenses = append(lenses, "egm-zoom-out")
	}
	return lenses
}

func validateGoldfishReviewFile(wiDir, expectedSpecSHA string, requiredLenses []string) (string, []ValidationIssue) {
	var issues []ValidationIssue
	gfPath := filepath.Join(wiDir, "goldfish_review.json")
	raw, err := os.ReadFile(gfPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			issues = append(issues, ValidationIssue{
				Pointer: "/goldfish_review",
				Code:    ReasonValidationFailed,
				Message: "goldfish_review.json ausente (obligatorio para ASDD --stage final)",
			})
			return "", issues
		}
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review",
			Code:    ReasonValidationFailed,
			Message: fmt.Sprintf("error leyendo goldfish_review.json: %v", err),
		})
		return "", issues
	}

	var gf GoldfishReview
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&gf); err != nil {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review",
			Code:    ReasonValidationFailed,
			Message: fmt.Sprintf("esquema invalido en goldfish_review.json: %v", err),
		})
		return "", issues
	}
	if dec.More() {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review",
			Code:    ReasonValidationFailed,
			Message: "contenido JSON adicional en goldfish_review.json",
		})
	}
	if gf.SchemaVersion != SchemaVersion {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review/schema_version",
			Code:    ReasonValidationFailed,
			Message: fmt.Sprintf("goldfish_review.schema_version debe ser %q", SchemaVersion),
		})
	}
	if gf.SpecSHA256 != expectedSpecSHA {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review/spec_sha256",
			Code:    ReasonValidationFailed,
			Message: "goldfish_review.spec_sha256 no coincide con el spec.json actual",
		})
	}
	if gf.UnresolvedBlockers != 0 {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review/unresolved_blockers",
			Code:    ReasonValidationFailed,
			Message: fmt.Sprintf("goldfish_review.unresolved_blockers=%d (debe ser 0)", gf.UnresolvedBlockers),
		})
	}
	if _, err := time.Parse(time.RFC3339, gf.ReviewedAt); err != nil {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review/reviewed_at",
			Code:    ReasonValidationFailed,
			Message: "goldfish_review.reviewed_at debe ser RFC3339 valido",
		})
	}
	cleanRep, rej := bfcore.SanitizeRelativePaths([]string{gf.ReportPath}, "", bfcore.PathSanitizeOpts{
		AllowDir: false, SplitCommas: false, FoldCaseDedupe: false,
	})
	if rej != "" || len(cleanRep) != 1 || len(bfcore.ScanG4All(gf.ReportPath)) > 0 {
		issues = append(issues, ValidationIssue{
			Pointer: "/goldfish_review/report_path",
			Code:    ReasonValidationFailed,
			Message: "goldfish_review.report_path debe ser ruta relativa limpia a --factory-root libre de G4",
		})
	}

	runSet := make(map[string]bool, len(gf.LensesRun))
	for _, l := range gf.LensesRun {
		runSet[strings.ToLower(strings.TrimSpace(l))] = true
	}
	for _, req := range requiredLenses {
		if !runSet[req] {
			issues = append(issues, ValidationIssue{
				Pointer: "/goldfish_review/lenses_run",
				Code:    ReasonValidationFailed,
				Message: fmt.Sprintf("lente Goldfish requerido ausente en lenses_run: %s", req),
			})
		}
	}

	normRaw := bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	normRaw = bytes.ReplaceAll(normRaw, []byte("\r\n"), []byte("\n"))
	normRaw = bytes.ReplaceAll(normRaw, []byte("\r"), []byte("\n"))
	sum := sha256.Sum256(normRaw)
	return hex.EncodeToString(sum[:]), issues
}

type indexTemplateData struct {
	TaskID           string
	Title            string
	Template         string
	ObjectiveLine    string
	TargetBranch     string
	CanModifyLine    string
	ReadOnlyLine     string
	ForbiddenLine    string
	G3DataMutation   string
	StackAssumptions []string
	StepContracts    []string
	DoD              []string
	WorkItemID       string
	WorkItemRevision int
	TraceID          string
	Lane             string
	SpecSHA256       string
	BrainFollowups   []BrainFollowup
}

type chunkTemplateData struct {
	Number             string
	Title              string
	Objective          string
	Inputs             []string
	CanModify          []string
	Forbidden          []string
	AcceptanceCriteria []string
	VerifyCommands     []string
	FailureProof       string
}

type statusChunkItem struct {
	ChunkID string
	Title   string
}

type statusTemplateData struct {
	TaskID     string
	SpecSHA256 string
	TraceID    string
	Chunks     []statusChunkItem
}

func renderGengisTaskInMemory(
	spec Spec,
	ctx SpecContext,
	taskID string,
	topoOrder []int,
	allCanModify, cleanReadOnly, cleanGlobalForbidden []string,
	unitCanModify, unitForbidden, unitCmds [][]string,
	hasCritico, hasEscalated bool,
	totalExpansions int,
) (RenderedTaskFiles, error) {
	tmpl, err := template.ParseFS(TemplatesFS, "templates/*.md.tmpl")
	if err != nil {
		return RenderedTaskFiles{}, err
	}

	specSHA, err := CanonicalSpecSHA256(spec)
	if err != nil {
		return RenderedTaskFiles{}, err
	}

	var indexTags []string
	if spec.RequiresAutorHITL {
		indexTags = append(indexTags, "[HITL]")
	}
	if hasCritico {
		indexTags = append(indexTags, "[CRITICO]")
	}
	if hasEscalated {
		indexTags = append(indexTags, "[ESCALATED]")
	}
	tagPrefix := ""
	if len(indexTags) > 0 {
		tagPrefix = strings.Join(indexTags, " ") + " "
	}

	firstProbLine := strings.Split(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(spec.Narrative.Problem), "\r\n", "\n"), "\r", "\n"), "\n")[0]

	// Contratos entre pasos en orden topologico
	stepContracts := make([]string, 0, len(topoOrder))
	statusItems := make([]statusChunkItem, 0, len(topoOrder))
	renderedChunks := make([]RenderedChunk, 0, len(topoOrder))

	for pos, uIdx := range topoOrder {
		u := spec.WorkUnits[uIdx]
		num := fmt.Sprintf("%02d", pos+1)
		chunkID := fmt.Sprintf("%s_%s", num, u.ID)
		statusItems = append(statusItems, statusChunkItem{ChunkID: chunkID, Title: u.Title})

		depDesc := "sin dependencias previas"
		if len(u.DependsOn) > 0 {
			depDesc = "depende de " + strings.Join(u.DependsOn, ", ")
		}
		stepContracts = append(stepContracts, fmt.Sprintf("Chunk %s (%s): %s; modifica %d archivo(s).", num, u.ID, depDesc, len(unitCanModify[uIdx])))

		// Construir Fuera de Alcance del chunk: global_forbidden + unit.forbidden + can_modify de otros chunks
		// EXCLUYENDO cualquier ruta que se solape con unitCanModify[uIdx] (evita bloqueo en depends_on)
		otherForb := make([]string, 0)
		seenForb := make(map[string]bool)
		addForb := func(p string) {
			if scopeUnderAny(p, unitCanModify[uIdx]) {
				return
			}
			// Tambien evitar que un prefijo forbidden cubra alguna ruta de unitCanModify[uIdx]
			for _, own := range unitCanModify[uIdx] {
				if scopeUnderAny(own, []string{p}) {
					return
				}
			}
			low := strings.ToLower(p)
			if !seenForb[low] {
				seenForb[low] = true
				otherForb = append(otherForb, p)
			}
		}
		for _, p := range cleanGlobalForbidden {
			addForb(p)
		}
		for _, p := range unitForbidden[uIdx] {
			addForb(p)
		}
		for otherIdx, cmList := range unitCanModify {
			if otherIdx == uIdx {
				continue
			}
			for _, p := range cmList {
				addForb(p)
			}
		}

		// Etiquetas compactas en 1.a linea de Chunk.Spec para gengis.exe plan
		chunkTags := append([]string{}, indexTags...)
		if len(u.ScopeExpansions) > 0 {
			chunkTags = append(chunkTags, fmt.Sprintf("[EXPANSIONS:%d]", len(u.ScopeExpansions)))
		}
		chunkTagPrefix := ""
		if len(chunkTags) > 0 {
			chunkTagPrefix = strings.Join(chunkTags, " ") + " "
		}

		accLines := make([]string, 0, len(u.AcceptanceCriteria))
		for _, ac := range u.AcceptanceCriteria {
			if ac.Kind == "review_gate" {
				accLines = append(accLines, fmt.Sprintf("[%s | review_gate:%s] %s", ac.ID, ac.Executor, ac.Description))
			} else {
				accLines = append(accLines, fmt.Sprintf("[%s | verify:%s] %s", ac.ID, ac.VerifyStepID, ac.Description))
			}
		}

		fpLine := fmt.Sprintf("%s (expect_exit=%d)", strings.TrimSpace(u.FailureProof.Description), u.FailureProof.ExpectExit)
		if strings.TrimSpace(u.FailureProof.ExpectOutputContains) != "" {
			fpLine += fmt.Sprintf(" [salida esperada contiene: %s]", strings.TrimSpace(u.FailureProof.ExpectOutputContains))
		}

		var cBuf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&cBuf, "chunk.md.tmpl", chunkTemplateData{
			Number:             num,
			Title:              u.Title,
			Objective:          chunkTagPrefix + strings.TrimSpace(u.Objective),
			Inputs:             u.Inputs,
			CanModify:          unitCanModify[uIdx],
			Forbidden:          otherForb,
			AcceptanceCriteria: accLines,
			VerifyCommands:     unitCmds[uIdx],
			FailureProof:       fpLine,
		}); err != nil {
			return RenderedTaskFiles{}, err
		}
		renderedChunks = append(renderedChunks, RenderedChunk{
			Filename: chunkID + ".md",
			Content:  cBuf.String(),
		})
	}

	if totalExpansions > 0 && !strings.Contains(tagPrefix, "[EXPANSIONS:") {
		// Mantener etiquetas de index.md enfocadas en HITL/CRITICO/ESCALATED
	}

	var idxBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&idxBuf, "index.md.tmpl", indexTemplateData{
		TaskID:           taskID,
		Title:            spec.Title,
		Template:         spec.Template,
		ObjectiveLine:    tagPrefix + firstProbLine,
		TargetBranch:     spec.TargetBranch,
		CanModifyLine:    formatBacktickedList(allCanModify, "ninguna"),
		ReadOnlyLine:     formatBacktickedList(cleanReadOnly, "ninguna"),
		ForbiddenLine:    formatBacktickedList(cleanGlobalForbidden, "ninguna"),
		G3DataMutation:   spec.G3DataMutation,
		StackAssumptions: spec.StackAssumptions,
		StepContracts:    stepContracts,
		DoD:              spec.DoD,
		WorkItemID:       ctx.WorkItem.ID,
		WorkItemRevision: ctx.WorkItem.Revision,
		TraceID:          ctx.TraceID,
		Lane:             spec.Lane,
		SpecSHA256:       specSHA,
		BrainFollowups:   spec.BrainFollowups,
	}); err != nil {
		return RenderedTaskFiles{}, err
	}

	var stBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&stBuf, "status.md.tmpl", statusTemplateData{
		TaskID:     taskID,
		SpecSHA256: specSHA,
		TraceID:    ctx.TraceID,
		Chunks:     statusItems,
	}); err != nil {
		return RenderedTaskFiles{}, err
	}

	return RenderedTaskFiles{
		IndexMD:  idxBuf.String(),
		StatusMD: stBuf.String(),
		Chunks:   renderedChunks,
	}, nil
}

func formatBacktickedList(items []string, emptyWord string) string {
	if len(items) == 0 {
		return emptyWord
	}
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = "`" + it + "`"
	}
	return strings.Join(quoted, ", ")
}

// verifyRenderedGengisParity ejecuta el superconjunto en memoria de validateChunks (GengiSWAP/engine/ring.go:393-426)
// verificando que el Markdown compilado preserve todas las secciones obligatorias de index.md y chunks/*.md.
func verifyRenderedGengisParity(r RenderedTaskFiles) []ValidationIssue {
	var issues []ValidationIssue
	if !strings.Contains(r.IndexMD, "## Alcance") || !strings.Contains(r.IndexMD, "- **Repositorio destino:** <TARGET_REPO>") {
		issues = append(issues, ValidationIssue{
			Pointer: "/render/index",
			Code:    ReasonValidationFailed,
			Message: "index.md renderizado carece de ## Alcance o marcador <TARGET_REPO>",
		})
	}
	if len(r.Chunks) == 0 {
		issues = append(issues, ValidationIssue{
			Pointer: "/render/chunks",
			Code:    ReasonValidationFailed,
			Message: "lista de chunks renderizados vacia (validateChunks)",
		})
		return issues
	}
	seenIDs := make(map[string]bool, len(r.Chunks))
	for i, ch := range r.Chunks {
		id := strings.TrimSuffix(ch.Filename, ".md")
		if id == "" || seenIDs[id] {
			issues = append(issues, ValidationIssue{
				Pointer: fmt.Sprintf("/render/chunks/%d", i),
				Code:    ReasonValidationFailed,
				Message: fmt.Sprintf("chunk ID vacio o duplicado %q", id),
			})
		}
		seenIDs[id] = true
		for _, requiredH2 := range []string{"## Objetivo", "## Puede modificar", "## Criterios de Aceptación", "## Comando de Verificación", "## Prueba de que puede fallar"} {
			if !strings.Contains(ch.Content, requiredH2) {
				issues = append(issues, ValidationIssue{
					Pointer: fmt.Sprintf("/render/chunks/%d", i),
					Code:    ReasonValidationFailed,
					Message: fmt.Sprintf("chunk %s carece de seccion obligatoria %s", ch.Filename, requiredH2),
				})
			}
		}
	}
	return issues
}

func checkNoMermaidInJSON(v any, ptr string, addIssue func(string, string, string)) {
	switch t := v.(type) {
	case string:
		if reMermaidFence.MatchString(t) {
			addIssue(pointerOrRoot(ptr), ReasonValidationFailed, "violacion G2: bloque Mermaid prohibido")
		}
	case []any:
		for i, elem := range t {
			checkNoMermaidInJSON(elem, fmt.Sprintf("%s/%d", ptr, i), addIssue)
		}
	case map[string]any:
		for k, val := range t {
			checkNoMermaidInJSON(val, ptr+"/"+escapeJSONPointer(k), addIssue)
		}
	}
}

func loadSpecContextStrict(wiDir string) (SpecContext, int, string) {
	ctxPath := filepath.Join(wiDir, "spec_context.json")
	if err := bfcore.CheckNoReparseChain(wiDir, ctxPath); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return SpecContext{}, ExitInfraOrRefusal, ReasonReparsePointDetected
		}
		return SpecContext{}, ExitInfraOrRefusal, ReasonIOError
	}
	raw, err := ReadFileWithShortRetry(ctxPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SpecContext{}, ExitInfraOrRefusal, ReasonArtifactStateCorrupt
		}
		return SpecContext{}, ExitInfraOrRefusal, ReasonIOError
	}
	if hits, jsonErr := ScanRawAndDecodedG4(raw); len(hits) > 0 {
		return SpecContext{}, ExitG4, ReasonG4ViolationOnDisk
	} else if jsonErr != nil {
		return SpecContext{}, ExitInfraOrRefusal, ReasonArtifactStateCorrupt
	}
	var sc SpecContext
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sc); err != nil || sc.SchemaVersion != SchemaVersion || sc.WorkItem.ID == "" {
		return SpecContext{}, ExitInfraOrRefusal, ReasonArtifactStateCorrupt
	}
	return sc, ExitOK, ReasonOK
}

func filterProjectRiskHits(hits []bfcore.RiskHit, project string) []bfcore.RiskHit {
	out := make([]bfcore.RiskHit, 0, len(hits))
	for _, h := range hits {
		hp := strings.TrimSpace(h.Project)
		if hp != "" && hp != "*" && !strings.EqualFold(hp, "black-factory") && !strings.EqualFold(hp, project) {
			continue
		}
		out = append(out, h)
	}
	return out
}

func mergeRiskHits(base, live []bfcore.RiskHit) []bfcore.RiskHit {
	byID := make(map[string]bfcore.RiskHit)
	order := make([]string, 0, len(base)+len(live))
	add := func(h bfcore.RiskHit) {
		k := strings.ToUpper(strings.TrimSpace(h.ID))
		if k == "" {
			return
		}
		prev, ok := byID[k]
		if !ok {
			byID[k] = h
			order = append(order, k)
			return
		}
		if bfcore.RiskLevelRank(h.Level) > bfcore.RiskLevelRank(prev.Level) {
			byID[k] = h
		}
	}
	for _, h := range base {
		add(h)
	}
	for _, h := range live {
		add(h)
	}
	out := make([]bfcore.RiskHit, 0, len(order))
	for _, k := range order {
		out = append(out, byID[k])
	}
	return out
}

func hasValidActiveDecisionForScope(decisions []DecisionRecord, absRepo, currentScopeSHA string) bool {
	if len(decisions) == 0 {
		return false
	}
	active := decisions[len(decisions)-1]
	cleanAppr, rej := bfcore.SanitizeRelativePaths(active.ApprovedPaths, absRepo, bfcore.PathSanitizeOpts{
		AllowDir: false, SplitCommas: false, FoldCaseDedupe: true,
	})
	if rej != "" || len(cleanAppr) == 0 {
		return false
	}
	calcSHA := ComputeScopeSHA256(cleanAppr)
	return calcSHA == active.ScopeSHA256 && active.ScopeSHA256 == currentScopeSHA
}

func hasTriageReasonCode(reasons []TriageReasonInput, code string) bool {
	for _, r := range reasons {
		if strings.EqualFold(strings.TrimSpace(r.Code), code) {
			return true
		}
	}
	return false
}

func hasLineBreak(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}

func pointerOrRoot(ptr string) string {
	if strings.TrimSpace(ptr) == "" {
		return "/"
	}
	return ptr
}

func failResult(code int, reason, traceID, specSHA string, lastExit int, lastReason string, issues []ValidationIssue) (CLIResult, int) {
	return CLIResult{
		OK:         false,
		ExitCode:   code,
		Reason:     reason,
		Retryable:  IsRetryableReason(code, reason),
		TraceID:    traceID,
		SpecSHA256: specSHA,
		LastExit:   lastExit,
		LastReason: lastReason,
		Issues:     issues,
	}, code
}
