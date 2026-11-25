package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

var reOrderID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,63}$`)

func validateSafeSummary(s string, maxRunes int, disallowShellChars bool) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || len([]rune(s)) > maxRunes {
		return false
	}
	if strings.ContainsAny(s, "\r\n") {
		return false
	}
	if disallowShellChars && strings.ContainsAny(s, "\"\\`$%") {
		return false
	}
	return true
}

func validateOrderRequiredNonNull(cleaned []byte) (string, string) {
	var rawObj map[string]any
	if err := json.Unmarshal(cleaned, &rawObj); err != nil || rawObj == nil {
		return "/order", "BounceOrder debe ser un objeto JSON no nulo"
	}
	topReq := []string{
		"schema_version", "order_id", "created_at", "trace_id", "task_id",
		"chunk_id", "bundle_id", "bundle_sha256", "decision", "reviewer_verdicts",
		"requires_red_test", "remediation_items", "summary",
	}
	for _, k := range topReq {
		v, ok := rawObj[k]
		if !ok || v == nil {
			return "/" + k, fmt.Sprintf("campo obligatorio %q ausente o null", k)
		}
	}
	verdicts, ok := rawObj["reviewer_verdicts"].(map[string]any)
	if !ok || verdicts == nil {
		return "/reviewer_verdicts", "reviewer_verdicts debe ser un objeto JSON"
	}
	for _, ax := range []string{"security", "code_architecture", "ux", "ui"} {
		axObj, ok := verdicts[ax].(map[string]any)
		if !ok || axObj == nil {
			return "/reviewer_verdicts/" + ax, fmt.Sprintf("eje %q ausente o null", ax)
		}
		for _, k := range []string{"status", "findings_count", "summary"} {
			v, exists := axObj[k]
			if !exists || v == nil {
				return "/reviewer_verdicts/" + ax + "/" + k, fmt.Sprintf("campo obligatorio %q ausente o null", k)
			}
		}
	}
	items, ok := rawObj["remediation_items"].([]any)
	if !ok || items == nil {
		return "/remediation_items", "remediation_items debe ser un arreglo JSON (no null)"
	}
	for i, elem := range items {
		itemObj, ok := elem.(map[string]any)
		if !ok || itemObj == nil {
			return fmt.Sprintf("/remediation_items/%d", i), "elemento debe ser objeto JSON"
		}
		for _, k := range []string{"id", "axis", "severity", "file", "line", "description", "required_action"} {
			v, exists := itemObj[k]
			if !exists || v == nil {
				return fmt.Sprintf("/remediation_items/%d/%s", i, k), fmt.Sprintf("campo obligatorio %q ausente o null", k)
			}
		}
	}
	return "", ""
}

func validateBounceOrderAgainstBundle(ord BounceOrder, bundle DeliveryBundle, session SessionState) (string, string, string) {
	if ord.TraceID != bundle.TraceID {
		return ReasonBundleMismatch, "/trace_id", "trace_id de la orden no coincide con DeliveryBundle activo"
	}
	if ord.TaskID != bundle.TaskID {
		return ReasonBundleMismatch, "/task_id", "task_id de la orden no coincide con DeliveryBundle activo"
	}
	if ord.ChunkID != bundle.ChunkID {
		return ReasonBundleMismatch, "/chunk_id", "chunk_id de la orden no coincide con DeliveryBundle activo"
	}
	if ord.BundleID != bundle.BundleID {
		return ReasonBundleMismatch, "/bundle_id", "bundle_id de la orden no coincide con DeliveryBundle activo"
	}
	if ord.BundleSHA256 != session.LatestBundleSHA256 {
		return ReasonBundleMismatch, "/bundle_sha256", "bundle_sha256 de la orden no coincide con LatestBundleSHA256"
	}

	if ord.SchemaVersion != SchemaVersion {
		return ReasonOrderValidationFailed, "/schema_version", "schema_version debe ser 1.0"
	}
	if !reOrderID.MatchString(ord.OrderID) {
		return ReasonOrderValidationFailed, "/order_id", "order_id invalido"
	}
	if _, err := time.Parse(time.RFC3339, ord.CreatedAt); err != nil {
		return ReasonOrderValidationFailed, "/created_at", "created_at debe ser RFC3339 valido"
	}
	if ord.Decision != "APPROVE" && ord.Decision != "BOUNCE" {
		return ReasonOrderValidationFailed, "/decision", "decision debe ser APPROVE o BOUNCE"
	}
	if !validateSafeSummary(ord.Summary, 200, true) {
		return ReasonOrderValidationFailed, "/summary", "summary invalido o contiene caracteres prohibidos"
	}

	type axisCheck struct {
		name       string
		v          AxisVerdict
		applicable bool
		allowNA    bool
	}
	axes := []axisCheck{
		{name: "security", v: ord.ReviewerVerdicts.Security, applicable: true, allowNA: false},
		{name: "code_architecture", v: ord.ReviewerVerdicts.CodeArchitecture, applicable: true, allowNA: false},
		{name: "ux", v: ord.ReviewerVerdicts.UX, applicable: bundle.ReviewerApplicability.UX.Applicable, allowNA: true},
		{name: "ui", v: ord.ReviewerVerdicts.UI, applicable: bundle.ReviewerApplicability.UI.Applicable, allowNA: true},
	}

	hasFail := false
	for _, ax := range axes {
		if ax.v.FindingsCount < 0 {
			return ReasonOrderValidationFailed, "/reviewer_verdicts/" + ax.name + "/findings_count", "findings_count debe ser >= 0"
		}
		if !validateSafeSummary(ax.v.Summary, 240, false) {
			return ReasonOrderValidationFailed, "/reviewer_verdicts/" + ax.name + "/summary", "summary de revisor invalido"
		}
		switch ax.v.Status {
		case "PASS":
		case "FAIL":
			hasFail = true
		case "NA":
			if !ax.allowNA || ax.applicable || strings.TrimSpace(ax.v.NAReason) == "" {
				return ReasonOrderValidationFailed, "/reviewer_verdicts/" + ax.name, "NA solo permitido para ux/ui no aplicables y con na_reason"
			}
		default:
			return ReasonOrderValidationFailed, "/reviewer_verdicts/" + ax.name + "/status", "status invalido"
		}
	}

	if ord.Decision == "APPROVE" {
		if ord.RequiresRedTest {
			return ReasonOrderValidationFailed, "/requires_red_test", "requires_red_test debe ser false en APPROVE"
		}
		if len(ord.RemediationItems) != 0 {
			return ReasonOrderValidationFailed, "/remediation_items", "remediation_items debe estar vacio en APPROVE"
		}
		if hasFail {
			return ReasonOrderValidationFailed, "/reviewer_verdicts", "ningun eje puede estar en FAIL cuando decision==APPROVE"
		}
		for _, ax := range axes {
			if ax.v.FindingsCount != 0 {
				return ReasonOrderValidationFailed, "/reviewer_verdicts/" + ax.name + "/findings_count", "findings_count debe ser 0 en APPROVE"
			}
		}
	} else {
		if len(ord.RemediationItems) == 0 {
			return ReasonOrderValidationFailed, "/remediation_items", "remediation_items requiere al menos 1 elemento en BOUNCE"
		}
		if !hasFail {
			return ReasonOrderValidationFailed, "/reviewer_verdicts", "al menos un eje debe estar en FAIL cuando decision==BOUNCE"
		}
		for i, rem := range ord.RemediationItems {
			if strings.TrimSpace(rem.ID) == "" {
				return ReasonOrderValidationFailed, fmt.Sprintf("/remediation_items/%d/id", i), "id requerido"
			}
			switch rem.Axis {
			case "security", "ux", "ui", "code_architecture":
			default:
				return ReasonOrderValidationFailed, fmt.Sprintf("/remediation_items/%d/axis", i), "axis invalido"
			}
			switch rem.Severity {
			case "BLOCKER", "MAJOR", "MINOR":
			default:
				return ReasonOrderValidationFailed, fmt.Sprintf("/remediation_items/%d/severity", i), "severity invalida"
			}
			if rem.Line < 0 {
				return ReasonOrderValidationFailed, fmt.Sprintf("/remediation_items/%d/line", i), "line debe ser >= 0"
			}
			if rem.File != "" {
				if _, reasonCode := bfcore.SanitizeRelativePaths([]string{rem.File}, "", bfcore.PathSanitizeOpts{}); reasonCode != "" {
					return ReasonOrderValidationFailed, fmt.Sprintf("/remediation_items/%d/file", i), "ruta relativa invalida en remediation_item: " + reasonCode
				}
			}
			if len([]rune(strings.TrimSpace(rem.Description))) == 0 || len([]rune(rem.Description)) > 500 ||
				len([]rune(strings.TrimSpace(rem.RequiredAction))) == 0 || len([]rune(rem.RequiredAction)) > 500 {
				return ReasonOrderValidationFailed, fmt.Sprintf("/remediation_items/%d", i), "description y required_action requieren 1..500 chars"
			}
		}
	}
	return "", "", ""
}

func loadActiveBundleStrict(sctx SessionRuntimeContext) (DeliveryBundle, error) {
	stem := fmt.Sprintf("%s_a%d_r%d_c%d", sctx.Session.ActiveChunkID, sctx.Session.ActiveAttempt, sctx.Session.ResetEpoch, sctx.Session.BounceCycle)
	bundlePath := filepath.Join(sctx.TaskStateDir, "bundles", stem+".bundle.json")
	raw, err := ReadFileWithShortRetry(bundlePath)
	if err != nil {
		return DeliveryBundle{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})))
	dec.DisallowUnknownFields()
	var b DeliveryBundle
	if err := dec.Decode(&b); err != nil {
		return DeliveryBundle{}, err
	}
	var extra any
	if !errors.Is(dec.Decode(&extra), io.EOF) {
		return DeliveryBundle{}, errors.New("JSON trailing en DeliveryBundle")
	}
	return b, nil
}

// ApplyOrder implementa builder apply-order (--order XOR --reset-bounces --reason).
func ApplyOrder(opts ApplyOrderOptions) (CLIResult, int) {
	start := time.Now()
	hasOrder := strings.TrimSpace(opts.OrderPath) != ""
	if hasOrder == opts.ResetBounces {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
			Pointer: "/cli/options",
			Code:    ReasonInvalidUsage,
			Message: "debe pasar exactamente --order O --reset-bounces --reason (mutuamente excluyentes)",
		}})
	}

	reasonTrim := strings.TrimSpace(opts.Reason)
	if opts.ResetBounces {
		if len(reasonTrim) < 8 {
			return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
				Pointer: "/cli/reason",
				Code:    ReasonInvalidUsage,
				Message: "--reset-bounces requiere --reason de al menos 8 caracteres",
			}})
		}
	} else {
		if err := bfcore.CheckNoReparseChain("", opts.OrderPath); err != nil {
			if errors.Is(err, bfcore.ErrReparsePoint) {
				return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", nil)
			}
			if !errors.Is(err, os.ErrNotExist) {
				return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", nil)
			}
		}
		fi, err := os.Lstat(opts.OrderPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
					Pointer: "/cli/order",
					Code:    ReasonInvalidUsage,
					Message: "el archivo --order especificado no existe",
				}})
			}
			return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", nil)
		}
		if bfcore.IsReparseOrSymlink(fi) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", nil)
		}
		if fi.IsDir() || !fi.Mode().IsRegular() {
			return failResult(ExitUsage, ReasonInvalidUsage, "", "", []ValidationIssue{{
				Pointer: "/cli/order",
				Code:    ReasonInvalidUsage,
				Message: "--order debe ser un archivo regular",
			}})
		}
	}

	sctx, errRes, code := LoadAndVerifySessionAndWorktree(opts.TaskDir, opts.RepoRoot, opts.FactoryRoot, opts.DataDir, opts.WorktreeRoot, "", !opts.ResetBounces)
	if errRes != nil {
		return *errRes, code
	}
	defer func() { _ = sctx.Lock.Unlock() }()

	if opts.ResetBounces {
		if hits := bfcore.ScanG4All(reasonTrim); len(hits) > 0 {
			return failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: "/cli/reason",
				Code:    ReasonG4Violation,
				Message: fmt.Sprintf("violacion G4 regla %s en --reason", hits[0].Rule),
			}})
		}
		now := time.Now().UTC().Format(time.RFC3339)
		sctx.Session.ResetEpoch++
		sctx.Session.BounceCycle = 0
		sctx.Session.BounceBrakeActive = false
		sctx.Session.ActiveBounceOrderID = ""
		sctx.Session.RequiresRedTest = true
		sctx.Session.TDD.Red = nil
		sctx.Session.LatestBundleID = ""
		sctx.Session.LatestBundleSHA256 = ""
		sctx.Session.LatestWorktreeTreeSHA256 = ""
		sctx.Session.ApprovedBundleID = ""
		sctx.Session.ApprovedBundleSHA256 = ""
		sctx.Session.ApprovedWorktreeTreeSHA256 = ""
		sctx.Session.UpdatedAt = now

		if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
		_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
			Event:       "BOUNCES_RESET",
			TraceID:     sctx.Session.TraceID,
			TaskID:      sctx.Session.TaskID,
			ChunkID:     sctx.Session.ActiveChunkID,
			Attempt:     sctx.Session.ActiveAttempt,
			BounceCycle: 0,
			ExitCode:    ExitOK,
			Reason:      ReasonOK,
			DurationMS:  time.Since(start).Milliseconds(),
			Detail:      fmt.Sprintf("reset_epoch=%d reason=%s", sctx.Session.ResetEpoch, bfcore.RedactAllG4(reasonTrim)),
		})
		return CLIResult{
			OK:          true,
			ExitCode:    ExitOK,
			Reason:      ReasonOK,
			Retryable:   false,
			TraceID:     sctx.Session.TraceID,
			TaskID:      sctx.Session.TaskID,
			ChunkID:     sctx.Session.ActiveChunkID,
			Attempt:     sctx.Session.ActiveAttempt,
			BounceCycle: 0,
			BuildBranch: sctx.Session.BuildBranch,
			Reset:       true,
		}, ExitOK
	}

	// Ruta --order
	if sctx.Session.BounceBrakeActive {
		return failResult(ExitAttemptsBlocked, ReasonMaxBounceCycles, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}

	gc := opts.Git
	if gc == nil {
		gc = ExecGitClient{}
	}
	ctx := context.Background()
	_, activeChunk, attemptNum, syncChanged, syncErr, syncCode := SyncSessionWithRingState(ctx, gc, &sctx, nil, true)
	if syncErr != nil {
		return *syncErr, syncCode
	}
	if syncChanged {
		if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
	}
	if headErr := checkWorktreeHeadMatchesBaseRef(ctx, gc, sctx); headErr != nil {
		return headErr.res, headErr.code
	}

	raw, err := ReadFileWithShortRetry(opts.OrderPath)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if len(raw) > MaxOrderBytes {
		return failResult(ExitFixable, ReasonOrderValidationFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/order",
			Code:    ReasonOrderValidationFailed,
			Message: fmt.Sprintf("la orden excede MaxOrderBytes (%d bytes)", MaxOrderBytes),
		}})
	}

	hits, jsonErr := ScanRawAndDecodedG4(raw)
	if len(hits) > 0 {
		return failResult(ExitG4, ReasonG4Violation, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: sanitizeG4Pointer(hits[0].Field),
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("violacion G4 regla %s (linea %d)", hits[0].Rule, hits[0].Line),
		}})
	}
	if jsonErr != nil {
		return failResult(ExitFixable, ReasonInvalidJSON, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/order",
			Code:    ReasonInvalidJSON,
			Message: "JSON invalido en orden de revision",
		}})
	}

	cleaned := bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	var ord BounceOrder
	if dErr := dec.Decode(&ord); dErr != nil {
		return failResult(ExitFixable, ReasonOrderValidationFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/order",
			Code:    ReasonOrderValidationFailed,
			Message: "contrato JSON BounceOrder invalido o campo desconocido",
		}})
	}
	var extra any
	if !errors.Is(dec.Decode(&extra), io.EOF) {
		return failResult(ExitFixable, ReasonOrderValidationFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/order",
			Code:    ReasonOrderValidationFailed,
			Message: "contenido JSON trailing detectado tras BounceOrder",
		}})
	}
	if reqPtr, reqMsg := validateOrderRequiredNonNull(cleaned); reqPtr != "" {
		return failResult(ExitFixable, ReasonOrderValidationFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: reqPtr,
			Code:    ReasonOrderValidationFailed,
			Message: reqMsg,
		}})
	}
	if ord.RemediationItems == nil {
		ord.RemediationItems = []RemediationItem{}
	}

	if sctx.Session.LatestBundleSHA256 == "" || sctx.Session.ActiveBounceOrderID != "" || sctx.Session.ApprovedBundleSHA256 != "" {
		return failResult(ExitFixable, ReasonOrderValidationFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/bundle_id",
			Code:    ReasonOrderValidationFailed,
			Message: "no hay DeliveryBundle pendiente o el bundle activo ya recibio veredicto",
		}})
	}
	for _, seen := range sctx.Session.SeenOrderIDs {
		if seen == ord.OrderID {
			return failResult(ExitFixable, ReasonOrderValidationFailed, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
				Pointer: "/order_id",
				Code:    ReasonOrderValidationFailed,
				Message: fmt.Sprintf("order_id %q ya fue aplicado anteriormente", ord.OrderID),
			}})
		}
	}

	bundle, bErr := loadActiveBundleStrict(sctx)
	if bErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonArtifactStateCorrupt, sctx.Session.TraceID, sctx.Session.TaskID, nil)
	}
	if vCode, vPtr, vMsg := validateBounceOrderAgainstBundle(ord, bundle, sctx.Session); vCode != "" {
		return failResult(ExitFixable, vCode, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: vPtr, Code: vCode, Message: vMsg,
		}})
	}

	now := time.Now().UTC().Format(time.RFC3339)
	bouncesDir := filepath.Join(sctx.TaskStateDir, "bounces")
	stem := fmt.Sprintf("%s_a%d_r%d", activeChunk.ID, attemptNum, sctx.Session.ResetEpoch)
	sctx.Session.SeenOrderIDs = append(sctx.Session.SeenOrderIDs, ord.OrderID)
	sctx.Session.ActiveBounceOrderID = ord.OrderID
	sctx.Session.UpdatedAt = now

	if ord.Decision == "APPROVE" {
		archivePath := filepath.Join(bouncesDir, fmt.Sprintf("%s_c%d_approve.order.json", stem, sctx.Session.BounceCycle))
		if err := WriteJSONAtomicG4(archivePath, ord); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
		sctx.Session.ApprovedBundleID = bundle.BundleID
		sctx.Session.ApprovedBundleSHA256 = sctx.Session.LatestBundleSHA256
		sctx.Session.ApprovedWorktreeTreeSHA256 = sctx.Session.LatestWorktreeTreeSHA256
		if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
		_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
			Event:        "ORDER_APPLIED",
			TraceID:      sctx.Session.TraceID,
			TaskID:       sctx.Session.TaskID,
			ChunkID:      activeChunk.ID,
			Attempt:      attemptNum,
			BounceCycle:  sctx.Session.BounceCycle,
			ExitCode:     ExitOK,
			Reason:       ReasonOK,
			BundleSHA256: sctx.Session.LatestBundleSHA256,
			DurationMS:   time.Since(start).Milliseconds(),
			Detail:       "decision=APPROVE order_id=" + ord.OrderID,
		})
		return CLIResult{
			OK:           true,
			ExitCode:     ExitOK,
			Reason:       ReasonOK,
			Retryable:    false,
			TraceID:      sctx.Session.TraceID,
			TaskID:       sctx.Session.TaskID,
			ChunkID:      activeChunk.ID,
			Attempt:      attemptNum,
			BounceCycle:  sctx.Session.BounceCycle,
			BuildBranch:  sctx.Session.BuildBranch,
			BundleID:     bundle.BundleID,
			BundleSHA256: sctx.Session.LatestBundleSHA256,
			Decision:     "APPROVE",
		}, ExitOK
	}

	// BOUNCE
	sctx.Session.PreviousBounceIDs = append(sctx.Session.PreviousBounceIDs, ord.OrderID)
	sctx.Session.LastBouncedWorktreeTreeSHA256 = sctx.Session.LatestWorktreeTreeSHA256

	if sctx.Session.BounceCycle >= MaxBounceCycles {
		brakePath := filepath.Join(bouncesDir, stem+"_b4_brake.order.json")
		if err := WriteJSONAtomicG4(brakePath, ord); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
		sctx.Session.BounceBrakeActive = true
		if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
			return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
		}
		_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
			Event:        "BOUNCE_BRAKE_ACTIVATED",
			TraceID:      sctx.Session.TraceID,
			TaskID:       sctx.Session.TaskID,
			ChunkID:      activeChunk.ID,
			Attempt:      attemptNum,
			BounceCycle:  sctx.Session.BounceCycle,
			ExitCode:     ExitAttemptsBlocked,
			Reason:       ReasonMaxBounceCycles,
			BundleSHA256: sctx.Session.LatestBundleSHA256,
			DurationMS:   time.Since(start).Milliseconds(),
			Detail:       "order_id=" + ord.OrderID,
		})
		r, c := failResult(ExitAttemptsBlocked, ReasonMaxBounceCycles, sctx.Session.TraceID, sctx.Session.TaskID, []ValidationIssue{{
			Pointer: "/bounce_cycle",
			Code:    ReasonMaxBounceCycles,
			Message: "limite de 3 ciclos BOUNCE alcanzado; ejecuta apply-order --reset-bounces --reason ...",
		}})
		r.ChunkID = activeChunk.ID
		r.Attempt = attemptNum
		r.BounceCycle = sctx.Session.BounceCycle
		r.Decision = "BOUNCE"
		return r, c
	}

	nextCycle := sctx.Session.BounceCycle + 1
	bouncePath := filepath.Join(bouncesDir, fmt.Sprintf("%s_b%d.order.json", stem, nextCycle))
	if err := WriteJSONAtomicG4(bouncePath, ord); err != nil {
		return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
	}
	sctx.Session.BounceCycle = nextCycle
	if ord.RequiresRedTest {
		sctx.Session.RequiresRedTest = true
		sctx.Session.TDD.Red = nil
	} else if sctx.Session.TDD.Red != nil {
		sctx.Session.TDD.Red.BounceCycle = nextCycle
	}
	if err := WriteJSONAtomicG4(sctx.SessionPath, sctx.Session); err != nil {
		return persistSessionOnWriteError(err, sctx.Session.TraceID, sctx.Session.TaskID)
	}

	_ = AppendEvent(sctx.AbsDataDir, BuilderEvent{
		Event:        "ORDER_APPLIED",
		TraceID:      sctx.Session.TraceID,
		TaskID:       sctx.Session.TaskID,
		ChunkID:      activeChunk.ID,
		Attempt:      attemptNum,
		BounceCycle:  nextCycle,
		ExitCode:     ExitOK,
		Reason:       ReasonOK,
		BundleSHA256: sctx.Session.LatestBundleSHA256,
		DurationMS:   time.Since(start).Milliseconds(),
		Detail:       fmt.Sprintf("decision=BOUNCE cycle=%d requires_red=%v order_id=%s", nextCycle, ord.RequiresRedTest, ord.OrderID),
	})

	return CLIResult{
		OK:           true,
		ExitCode:     ExitOK,
		Reason:       ReasonOK,
		Retryable:    false,
		TraceID:      sctx.Session.TraceID,
		TaskID:       sctx.Session.TaskID,
		ChunkID:      activeChunk.ID,
		Attempt:      attemptNum,
		BounceCycle:  nextCycle,
		BuildBranch:  sctx.Session.BuildBranch,
		BundleID:     bundle.BundleID,
		BundleSHA256: sctx.Session.LatestBundleSHA256,
		Decision:     "BOUNCE",
	}, ExitOK
}
