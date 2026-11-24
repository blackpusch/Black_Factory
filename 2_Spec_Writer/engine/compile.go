package specwriter

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

var reTaskID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,63}$`)

// SerializeVerifyStepCmd serializa un VerifyStep ya auditado a una linea determinista compatible con
// Windows cmd.exe /S /C (GengiSWAP/engine/runner_windows.go:23):
//  1. Claves env ordenadas lexicograficamente: set K=V&&
//  2. Si cwd != "": cd /d "<filepath.FromSlash(cwd)>" && (siempre entrecomillado)
//  3. argv[0] convertido con filepath.FromSlash (evita que cmd.exe interprete / como modificador)
//  4. Cada argv[k] con espacio o coma entrecomillado con comillas dobles
func SerializeVerifyStepCmd(vs VerifyStep) string {
	var parts []string

	if len(vs.Env) > 0 {
		keys := make([]string, 0, len(vs.Env))
		for k := range vs.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var envPrefix strings.Builder
		for _, k := range keys {
			envPrefix.WriteString("set ")
			envPrefix.WriteString(k)
			envPrefix.WriteByte('=')
			envPrefix.WriteString(vs.Env[k])
			envPrefix.WriteString("&& ")
		}
		parts = append(parts, strings.TrimSpace(envPrefix.String()))
	}

	if strings.TrimSpace(vs.Cwd) != "" {
		winCwd := strings.ReplaceAll(strings.TrimSuffix(strings.ReplaceAll(vs.Cwd, `\`, "/"), "/"), "/", `\`)
		parts = append(parts, fmt.Sprintf(`cd /d "%s" &&`, winCwd))
	}

	if len(vs.Argv) > 0 {
		cmdTokens := make([]string, 0, len(vs.Argv))
		exe := strings.ReplaceAll(vs.Argv[0], "/", `\`)
		cmdTokens = append(cmdTokens, quoteCmdTokenIfNeeded(exe))
		for _, tok := range vs.Argv[1:] {
			cmdTokens = append(cmdTokens, quoteCmdTokenIfNeeded(tok))
		}
		parts = append(parts, strings.Join(cmdTokens, " "))
	}

	return strings.Join(parts, " ")
}

func quoteCmdTokenIfNeeded(tok string) string {
	if strings.ContainsAny(tok, " ,") {
		return `"` + tok + `"`
	}
	return tok
}

// Compile ejecuta la precedencia 2 -> 5 -> 3 -> 6 -> 4 -> 1 -> 0 respetando el orden estricto de cerrojos
// (.wi.lock -> .compile.lock -> .events.lock), anti-bypass sobre attempts.json, frontera G4 de dos zonas
// (<TARGET_REPO>) y promocion atomica create-only en NTFS.
func Compile(opts CompileOptions) (CLIResult, int) {
	start := time.Now()

	// [2] Uso CLI, --task-id, reparse previo y os.Lstat(--spec)
	if strings.TrimSpace(opts.SpecPath) == "" || strings.TrimSpace(opts.TaskRoot) == "" || strings.TrimSpace(opts.TaskID) == "" {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", 0, "", []ValidationIssue{{
			Pointer: "/cli",
			Code:    ReasonInvalidUsage,
			Message: "--spec, --task-root y --task-id son obligatorios",
		}})
	}
	if !isValidTaskID(opts.TaskID) {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", 0, "", []ValidationIssue{{
			Pointer: "/cli/task_id",
			Code:    ReasonInvalidUsage,
			Message: fmt.Sprintf("--task-id %q invalido (patron ^[a-z0-9][a-z0-9_-]{2,63}$, sin nombres reservados Windows)", opts.TaskID),
		}})
	}

	factoryRoot, repoRoot, vaultRoot, dataDir, brainBin := ResolveCommonRoots(
		opts.FactoryRoot, opts.RepoRoot, opts.VaultRoot, opts.DataDir, opts.BrainBin,
	)
	wiDir := filepath.Dir(opts.SpecPath)

	if err := CheckReparseSafety(dataDir, wiDir, opts.SpecPath); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
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
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}
	if bfcore.IsReparseOrSymlink(specFI) {
		return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
	}
	if specFI.IsDir() || !specFI.Mode().IsRegular() {
		return failResult(ExitUsage, ReasonInvalidUsage, "", "", 0, "", []ValidationIssue{{
			Pointer: "/spec",
			Code:    ReasonInvalidUsage,
			Message: "--spec no es un archivo regular",
		}})
	}

	// [5] Adquirir .wi.lock (mantenido hasta [0]) y comprobar freno operativo en attempts.json
	wiLock, err := AcquireWorkItemLock(wiDir)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return failResult(ExitInfraOrRefusal, ReasonLockBusy, "", "", 0, "", nil)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", "", 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}
	defer func() { _ = wiLock.Unlock() }()

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

	if attempts.DraftAttempts >= MaxValidateAttemptsPerStage || attempts.FinalAttempts >= MaxValidateAttemptsPerStage {
		return failResult(ExitAttemptsBlocked, ReasonMaxValidateAttempts, "", "", attempts.LastExit, attempts.LastReason, []ValidationIssue{{
			Pointer: "/attempts",
			Code:    ReasonMaxValidateAttempts,
			Message: "freno de intentos activo; ejecute prepare --reset-attempts --reason",
		}})
	}
	if attempts.ExternalCycles >= MaxExternalCycles && (attempts.LastPassedSHA256 == "" || attempts.LastExit == ExitExternalDecision) {
		return failResult(ExitAttemptsBlocked, ReasonMaxExternalCycles, "", "", attempts.LastExit, attempts.LastReason, []ValidationIssue{{
			Pointer: "/attempts",
			Code:    ReasonMaxExternalCycles,
			Message: "freno de ciclos externos activo sin pase validate posterior; ejecute prepare --reset-attempts --reason",
		}})
	}

	// [3] Cuotas de tamano + barrera G4 (sin consumir intentos en compile)
	if specFI.Size() > MaxSpecBytes {
		return failResult(ExitFixable, ReasonSpecTooLarge, "", "", 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonSpecTooLarge,
			Message: fmt.Sprintf("spec.json excede MaxSpecBytes (%d bytes)", MaxSpecBytes),
		}})
	}

	specBytes, err := os.ReadFile(opts.SpecPath)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", "", 0, "", nil)
	}
	if g4Hits, jsonErr := ScanRawAndDecodedG4(specBytes); len(g4Hits) > 0 {
		safePtr := sanitizeG4Pointer(g4Hits[0].Field)
		return failResult(ExitG4, ReasonG4Violation, "", "", 0, "", []ValidationIssue{{
			Pointer: safePtr,
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("violacion G4 en spec.json (regla %s, linea %d)", g4Hits[0].Rule, g4Hits[0].Line),
		}})
	} else if jsonErr != nil {
		return failResult(ExitFixable, ReasonInvalidJSON, "", "", 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonInvalidJSON,
			Message: fmt.Sprintf("JSON invalido en spec.json: %v", jsonErr),
		}})
	}

	trimmedSpecBytes := bytes.TrimPrefix(specBytes, []byte{0xef, 0xbb, 0xbf})
	var spec Spec
	if err := json.Unmarshal(trimmedSpecBytes, &spec); err != nil {
		return failResult(ExitFixable, ReasonInvalidJSON, "", "", 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonInvalidJSON,
			Message: fmt.Sprintf("decodificando spec.json: %v", err),
		}})
	}
	specSHA, err := CanonicalSpecSHA256(spec)
	if err != nil {
		return failResult(ExitFixable, ReasonInvalidJSON, "", "", 0, "", []ValidationIssue{{
			Pointer: "/",
			Code:    ReasonInvalidJSON,
			Message: fmt.Sprintf("calculando spec_sha256: %v", err),
		}})
	}

	// Si ASDD y goldfish_review.json esta presente, aplicar cuota y G4 antes de [6]
	var goldfishDiskSHA string
	goldfishPresent := false
	if spec.Lane == "ASDD" {
		gfPath := filepath.Join(wiDir, "goldfish_review.json")
		gfFI, gfErr := os.Lstat(gfPath)
		if gfErr == nil {
			if bfcore.IsReparseOrSymlink(gfFI) {
				return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", specSHA, 0, "", nil)
			}
			if gfFI.IsDir() || !gfFI.Mode().IsRegular() {
				return failResult(ExitFixable, ReasonInvalidJSON, "", specSHA, 0, "", nil)
			}
			if gfFI.Size() > MaxGoldfishBytes {
				return failResult(ExitFixable, ReasonSpecTooLarge, "", specSHA, 0, "", []ValidationIssue{{
					Pointer: "/goldfish_review",
					Code:    ReasonSpecTooLarge,
					Message: fmt.Sprintf("goldfish_review.json excede MaxGoldfishBytes (%d bytes)", MaxGoldfishBytes),
				}})
			}
			gfRaw, rErr := os.ReadFile(gfPath)
			if rErr != nil {
				return failResult(ExitInfraOrRefusal, ReasonIOError, "", specSHA, 0, "", nil)
			}
			if g4Hits, jsonErr := ScanRawAndDecodedG4(gfRaw); len(g4Hits) > 0 {
				return failResult(ExitG4, ReasonG4Violation, "", specSHA, 0, "", []ValidationIssue{{
					Pointer: "/goldfish_review",
					Code:    ReasonG4Violation,
					Message: fmt.Sprintf("violacion G4 en goldfish_review.json (regla %s)", g4Hits[0].Rule),
				}})
			} else if jsonErr != nil {
				return failResult(ExitFixable, ReasonInvalidJSON, "", specSHA, 0, "", nil)
			}
			goldfishPresent = true
			normGF := strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(string(gfRaw), "\xef\xbb\xbf"), "\r\n", "\n"), "\r", "\n")
			sum := sha256.Sum256([]byte(normGF))
			goldfishDiskSHA = hex.EncodeToString(sum[:])
		} else if !errors.Is(gfErr, os.ErrNotExist) {
			return failResult(ExitInfraOrRefusal, ReasonIOError, "", specSHA, 0, "", nil)
		}
	}

	// [6] Preflight compartido, ScanTier1 sobre --repo-root, y cerrojo TOCTOU <task-root>/.compile.lock
	if hits := bfcore.ScanTier1("repo_root", repoRoot, false); len(hits) > 0 {
		return failResult(ExitG4, ReasonG4Violation, "", specSHA, 0, "", []ValidationIssue{{
			Pointer: "/cli/repo_root",
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("secreto Tier 1 detectado en --repo-root (regla %s)", hits[0].Rule),
		}})
	}
	if err := CheckReparseSafety(factoryRoot, repoRoot, vaultRoot, dataDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, "", specSHA, 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, "", specSHA, 0, "", nil)
	}

	ctxObj, ctxCode, ctxReason := loadSpecContextStrict(wiDir)
	if ctxCode != ExitOK {
		return failResult(ctxCode, ctxReason, "", specSHA, 0, "", nil)
	}

	client := opts.BrainClient
	if client == nil {
		client = bfcore.NewSubprocessBrainClient(brainBin, 0)
	}
	pf, pfReason, pfErr := CheckBrainPreflight(client, vaultRoot, "")
	if pfErr != nil {
		return failResult(ExitInfraOrRefusal, pfReason, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/brain",
			Code:    pfReason,
			Message: bfcore.RedactAllG4(pfErr.Error()),
		}})
	}

	_, _, absRepoClean, err := ComputeRepoRootIdentity(repoRoot)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if hits := bfcore.ScanTier1("repo_root", absRepoClean, false); len(hits) > 0 {
		return failResult(ExitG4, ReasonG4Violation, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/cli/repo_root",
			Code:    ReasonG4Violation,
			Message: fmt.Sprintf("secreto Tier 1 detectado en ruta resuelta de --repo-root (regla %s)", hits[0].Rule),
		}})
	}
	absTaskRoot, err := filepath.Abs(opts.TaskRoot)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	absTaskRoot = filepath.Clean(absTaskRoot)

	if err := bfcore.CheckNoReparseUpToNearestExisting("", absTaskRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, ctxObj.TraceID, specSHA, 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if err := os.MkdirAll(absTaskRoot, 0o755); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if err := bfcore.CheckNoReparseChain("", absTaskRoot); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, ctxObj.TraceID, specSHA, 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}

	compileLock, err := AcquireCompileLock(absTaskRoot)
	if err != nil {
		if errors.Is(err, ErrLockBusy) {
			return failResult(ExitInfraOrRefusal, ReasonLockBusy, ctxObj.TraceID, specSHA, 0, "", nil)
		}
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, ctxObj.TraceID, specSHA, 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	defer func() { _ = compileLock.Unlock() }()

	// Comprobacion estricta create-only dentro de .compile.lock (inspeccionar destDir primero para distinguir archivo regular vs directorio con .ring/state.json)
	destDir := filepath.Join(absTaskRoot, opts.TaskID)
	ringStatePath := filepath.Join(destDir, ".ring", "state.json")
	if err := bfcore.CheckNoReparseChain(absTaskRoot, destDir); err != nil {
		if errors.Is(err, bfcore.ErrReparsePoint) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, ctxObj.TraceID, specSHA, 0, "", nil)
		}
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	destFI, destErr := os.Lstat(destDir)
	if destErr == nil {
		if bfcore.IsReparseOrSymlink(destFI) {
			return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, ctxObj.TraceID, specSHA, 0, "", nil)
		}
		if destFI.IsDir() {
			if err := bfcore.CheckNoReparseChain(absTaskRoot, ringStatePath); err != nil {
				if errors.Is(err, bfcore.ErrReparsePoint) {
					return failResult(ExitInfraOrRefusal, ReasonReparsePointDetected, ctxObj.TraceID, specSHA, 0, "", nil)
				}
				return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
			}
			if _, stateErr := os.Lstat(ringStatePath); stateErr == nil {
				return failResult(ExitInfraOrRefusal, ReasonLiveTaskState, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
					Pointer: "/task_dir",
					Code:    ReasonLiveTaskState,
					Message: "existe .ring/state.json activo en destino; compile no sobrescribe tareas vivas",
				}})
			} else if !errors.Is(stateErr, os.ErrNotExist) {
				return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
			}
		}
		return failResult(ExitInfraOrRefusal, ReasonTaskDirExists, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/task_dir",
			Code:    ReasonTaskDirExists,
			Message: "el directorio de tarea destino ya existe (politica v1 create-only; elija un nuevo --task-id)",
		}})
	} else if !errors.Is(destErr, os.ErrNotExist) {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}

	// [4] Revalidacion de contexto/alcance en vivo SIN mutar attempts.json
	extRes, extErrCode, extErrReason := evaluateExternalDecisions(spec, ctxObj, repoRoot, vaultRoot, pf.DBPath, client)
	if extErrCode != ExitOK {
		return failResult(extErrCode, extErrReason, ctxObj.TraceID, specSHA, 0, "", extRes.Issues)
	}
	if extRes.ExternalReason != "" {
		if attempts.ExternalCycles >= MaxExternalCycles {
			return failResult(ExitAttemptsBlocked, ReasonMaxExternalCycles, ctxObj.TraceID, specSHA, ExitExternalDecision, extRes.ExternalReason, extRes.Issues)
		}
		return failResult(ExitExternalDecision, extRes.ExternalReason, ctxObj.TraceID, specSHA, 0, "", extRes.Issues)
	}

	// [1] Anti-bypass completo contra attempts.json
	stageOK := false
	if spec.Lane == "FTD" {
		stageOK = attempts.LastPassedStage == "draft" || attempts.LastPassedStage == "final"
	} else if spec.Lane == "ASDD" {
		stageOK = attempts.LastPassedStage == "final"
	}
	goldfishOK := true
	if spec.Lane == "ASDD" {
		goldfishOK = goldfishPresent && attempts.LastPassedGoldfishSHA256 != "" && goldfishDiskSHA == attempts.LastPassedGoldfishSHA256
	}
	if attempts.LastPassedSHA256 == "" || specSHA != attempts.LastPassedSHA256 || !stageOK || !goldfishOK {
		return failResult(ExitFixable, ReasonValidationRequired, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/attempts",
			Code:    ReasonValidationRequired,
			Message: fmt.Sprintf("se requiere validate --stage %s exitoso para el spec_sha256 actual antes de compile", requiredStageForLane(spec.Lane)),
		}})
	}

	// Re-invocar el validador compartido + ScanG4All sobre buffers en memoria (con <TARGET_REPO>)
	valStage := attempts.LastPassedStage
	issues, rendered, _, valErr := ValidateSpecAndRenderCheck(
		specBytes, spec, ctxObj, wiDir, repoRoot, factoryRoot, valStage, opts.TaskID, extRes.ReQueriedRiskHits, extRes.EscalationOccurred,
	)
	if valErr != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if len(issues) > 0 || !buffersPassG4(rendered) {
		detail := "ValidateSpecAndRenderCheck o ScanG4All fallo tras pase previo coincidente"
		if len(issues) > 0 {
			detail = issues[0].Message
		}
		_ = AppendEvent(dataDir, SpecWriterEvent{
			Event:      ReasonValidatorBypassDefect,
			TraceID:    ctxObj.TraceID,
			Project:    attempts.Project,
			WorkItemID: attempts.WIID,
			Stage:      valStage,
			ExitCode:   ExitFixable,
			Reason:     ReasonValidatorBypassDefect,
			Retryable:  false,
			SpecSHA256: specSHA,
			DurationMS: time.Since(start).Milliseconds(),
			Detail:     detail,
		})
		return failResult(ExitFixable, ReasonValidatorBypassDefect, ctxObj.TraceID, specSHA, 0, "", issues)
	}

	// [0] Frontera G4 de dos zonas: enlazar la linea exacta <TARGET_REPO> solo en el index.md externo
	const targetRepoLine = "\n- **Repositorio destino:** <TARGET_REPO>\n"
	if strings.Count(rendered.IndexMD, targetRepoLine) != 1 {
		return failResult(ExitFixable, ReasonValidatorBypassDefect, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/render/index",
			Code:    ReasonValidatorBypassDefect,
			Message: "invariante rota: marcador unico <TARGET_REPO> no encontrado exactamente una vez en index.md",
		}})
	}
	finalIndexMD := strings.Replace(
		rendered.IndexMD,
		targetRepoLine,
		"\n- **Repositorio destino:** "+absRepoClean+"\n",
		1,
	)

	// Promocion atomica create-only en el mismo volumen NTFS
	stagingDir, err := createSiblingStagingDir(absTaskRoot, opts.TaskID)
	if err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	if err := os.MkdirAll(filepath.Join(stagingDir, "chunks"), 0o755); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if err := os.MkdirAll(filepath.Join(stagingDir, "workspace", "qa"), 0o755); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if err := writeFileSyncClose(filepath.Join(stagingDir, "index.md"), []byte(finalIndexMD)); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	if err := writeFileSyncClose(filepath.Join(stagingDir, "status.md"), []byte(rendered.StatusMD)); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}
	for _, ch := range rendered.Chunks {
		if err := writeFileSyncClose(filepath.Join(stagingDir, "chunks", ch.Filename), []byte(ch.Content)); err != nil {
			return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
		}
	}

	// Re-verificar create-only justo antes del rename atomico
	if _, statErr := os.Lstat(destDir); statErr == nil {
		return failResult(ExitInfraOrRefusal, ReasonTaskDirExists, ctxObj.TraceID, specSHA, 0, "", nil)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", nil)
	}

	if err := renameWithWindowsRetry(stagingDir, destDir); err != nil {
		return failResult(ExitInfraOrRefusal, ReasonIOError, ctxObj.TraceID, specSHA, 0, "", []ValidationIssue{{
			Pointer: "/task_dir",
			Code:    ReasonIOError,
			Message: bfcore.RedactAllG4(err.Error()),
		}})
	}
	cleanupStaging = false

	_ = AppendEvent(dataDir, SpecWriterEvent{
		Event:      "COMPILED",
		TraceID:    ctxObj.TraceID,
		Project:    attempts.Project,
		WorkItemID: attempts.WIID,
		Stage:      valStage,
		ExitCode:   ExitOK,
		Reason:     ReasonOK,
		SpecSHA256: specSHA,
		DurationMS: time.Since(start).Milliseconds(),
		Detail:     fmt.Sprintf("task_id=%s chunks=%d lane=%s", opts.TaskID, len(rendered.Chunks), spec.Lane),
	})

	return CLIResult{
		OK:         true,
		ExitCode:   ExitOK,
		Reason:     ReasonOK,
		Retryable:  false,
		TraceID:    ctxObj.TraceID,
		SpecSHA256: specSHA,
		TaskDir:    destDir,
	}, ExitOK
}

func isValidTaskID(id string) bool {
	if !reTaskID.MatchString(id) {
		return false
	}
	switch strings.ToLower(id) {
	case "con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return false
	}
	return true
}

func requiredStageForLane(lane string) string {
	if lane == "ASDD" {
		return "final"
	}
	return "draft"
}

func buffersPassG4(r RenderedTaskFiles) bool {
	if len(bfcore.ScanG4All(r.IndexMD)) > 0 {
		return false
	}
	if len(bfcore.ScanG4All(r.StatusMD)) > 0 {
		return false
	}
	for _, ch := range r.Chunks {
		if len(bfcore.ScanG4All(ch.Content)) > 0 {
			return false
		}
	}
	return true
}

func createSiblingStagingDir(taskRoot, taskID string) (string, error) {
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf(".%s.staging-%s", taskID, hex.EncodeToString(rnd[:]))
	dir := filepath.Join(taskRoot, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func writeFileSyncClose(dstPath string, data []byte) error {
	f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
