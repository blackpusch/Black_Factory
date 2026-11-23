package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/intake"
	"blackfactory/pkg/bfcore"
)

const (
	ExitOK                     = 0
	ExitUsageOrError           = 1
	ExitQuarantined            = 2
	ExitRejectedG4             = 3
	ExitBrainOrLockUnavailable = 4
	ExitDedupeConflict         = 5
)

type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }
func (s *stringListFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil))
}

func run(args []string, in io.Reader, out, errOut io.Writer, client intake.BrainClient) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(out)
		if len(args) == 0 {
			return ExitUsageOrError
		}
		return ExitOK
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "ingest":
		return cmdIngest(rest, in, out, errOut, client)
	case "resolve":
		return cmdResolve(rest, out, errOut, client)
	case "list":
		return cmdList(rest, out, errOut)
	case "show":
		return cmdShow(rest, out, errOut)
	case "verify":
		return cmdVerify(rest, out, errOut, client)
	default:
		printUsage(errOut)
		fmt.Fprintf(errOut, "error: subcomando desconocido %q\n", sub)
		return ExitUsageOrError
	}
}

type pipelineOptions struct {
	repoRoot        string
	dataDir         string
	vaultRoot       string
	brainBin        string
	maxFTDFiles     int
	strictG4        bool
	update          bool
	dryRun          bool
	asJSON          bool
	prevQuarID      string
	existingCreated string
	existingRev     int
}

func cmdIngest(args []string, in io.Reader, out, errOut io.Writer, client intake.BrainClient) int {
	fs := flag.NewFlagSet("intake ingest", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var inputPath, project, title, description, note, kind, source, sourceRef string
	var repoRoot, dataDir, vaultRoot, brainBin string
	var pathFlags, pathsCSVFlags, impactFlags stringListFlag
	var maxFTDFiles int
	var strictG4, update, dryRun, asJSON bool

	fs.StringVar(&inputPath, "input", "", "Ruta de archivo JSON de entrada o '-' para stdin")
	fs.StringVar(&project, "project", "", "Proyecto objetivo (ej. black-factory)")
	fs.StringVar(&title, "title", "", "Titulo de la solicitud")
	fs.StringVar(&description, "description", "", "Descripcion detallada")
	fs.StringVar(&note, "note", "", "Nota operativa adicional")
	fs.StringVar(&kind, "kind", "", "Tipo (bug|feature|refactor|audit_fix|ops)")
	fs.StringVar(&source, "source", "", "Fuente de origen (cli|issue|audit|ops)")
	fs.StringVar(&sourceRef, "source-ref", "", "Referencia unica en la fuente")
	fs.Var(&pathFlags, "path", "Ruta relativa afectada (repetible)")
	fs.Var(&pathsCSVFlags, "paths", "Rutas relativas afectadas separadas por coma")
	fs.Var(&impactFlags, "impact-flag", "Bandera de impacto arquitectonico (repetible)")
	fs.IntVar(&maxFTDFiles, "max-ftd-files", intake.DefaultMaxFTDFiles, "Maximo de archivos para carril FTD")
	fs.BoolVar(&strictG4, "strict-g4", false, "Promover reglas Tier 2 G4 a rechazo fail-closed (Exit 3)")
	fs.BoolVar(&update, "update", false, "Actualizar WorkItem existente incrementando revision")
	fs.BoolVar(&dryRun, "dry-run", false, "Simular pipeline en memoria sin escribir en disco")
	fs.StringVar(&repoRoot, "repo-root", "", "Raiz del repositorio Black Factory")
	fs.StringVar(&dataDir, "data-dir", "", "Directorio runtime de 1_Intake")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta del binario 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON estructurada")

	reordered := reorderFlags(args, map[string]bool{
		"strict-g4": true, "update": true, "dry-run": true, "json": true,
	}, map[string]bool{
		"input": true, "project": true, "title": true, "description": true, "note": true,
		"kind": true, "source": true, "source-ref": true, "path": true, "paths": true,
		"impact-flag": true, "max-ftd-files": true, "repo-root": true, "data-dir": true,
		"vault": true, "brain-bin": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return ExitUsageOrError
	}

	opts := resolveCommonDirs(repoRoot, dataDir, vaultRoot, brainBin, maxFTDFiles, strictG4, update, dryRun, asJSON)
	store := intake.NewStore(opts.dataDir)

	var allCLIPaths []string
	allCLIPaths = append(allCLIPaths, pathFlags...)
	for _, csv := range pathsCSVFlags {
		for _, p := range strings.Split(csv, ",") {
			if t := strings.TrimSpace(p); t != "" {
				allCLIPaths = append(allCLIPaths, t)
			}
		}
	}

	cliOverrides := intake.RawIntake{
		Project:       project,
		Title:         title,
		Description:   description,
		Note:          note,
		Kind:          kind,
		Source:        source,
		SourceRef:     sourceRef,
		AffectedPaths: allCLIPaths,
		ImpactFlags:   []string(impactFlags),
	}

	// STAGE 1: Lectura cruda (<= 1 MiB, CheckNoReparseChain, BOM strip) + Fase-A Tier 1 G4
	rawBytes, err := intake.ReadRawInput(inputPath, opts.repoRoot, in)
	if err != nil {
		writeErrorOutput(errOut, asJSON, "INPUT_ERROR", err.Error())
		return ExitUsageOrError
	}

	if hits := intake.ScanRawAndFlagsTier1(rawBytes, cliOverrides, strictG4); len(hits) > 0 {
		return handleTier1Rejection(store, hits[0], rawBytes, cliOverrides.Source, cliOverrides.SourceRef, cliOverrides.Project, dryRun, asJSON, errOut)
	}

	// STAGE 2: Decodificacion JSON + Merge CLI + Fase-B Tier 1 G4 (bloquea \u0067hp_...)
	var baseIntake intake.RawIntake
	if len(rawBytes) > 0 {
		parsed, pErr := intake.ParseJSONIntake(rawBytes)
		if pErr != nil {
			writeErrorOutput(errOut, asJSON, "JSON_PARSE_ERROR", pErr.Error())
			return ExitUsageOrError
		}
		baseIntake = parsed
	} else if !hasAnyCLIField(cliOverrides) {
		writeErrorOutput(errOut, asJSON, "EMPTY_INPUT", "se requiere --input <archivo.json|-> o banderas CLI de solicitud")
		return ExitUsageOrError
	}

	merged := intake.MergeCLIOverrides(baseIntake, cliOverrides)
	if hits := intake.ScanDecodedTier1(merged, strictG4); len(hits) > 0 {
		rawDigestInput := rawBytes
		if len(rawDigestInput) == 0 {
			rawDigestInput, _ = json.Marshal(merged)
		}
		return handleTier1Rejection(store, hits[0], rawDigestInput, merged.Source, merged.SourceRef, merged.Project, dryRun, asJSON, errOut)
	}

	return executeStages2To5(merged, opts, store, client, out, errOut)
}

func cmdResolve(args []string, out, errOut io.Writer, client intake.BrainClient) int {
	fs := flag.NewFlagSet("intake resolve", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var targetID, project, title, description, note, kind, source, sourceRef string
	var repoRoot, dataDir, vaultRoot, brainBin string
	var pathFlags, pathsCSVFlags, impactFlags stringListFlag
	var maxFTDFiles int
	var strictG4, update, dryRun, asJSON bool

	fs.StringVar(&targetID, "id", "", "ID del WorkItem en cuarentena (WI-<hash12>)")
	fs.StringVar(&project, "project", "", "Proyecto objetivo")
	fs.StringVar(&title, "title", "", "Titulo de la solicitud")
	fs.StringVar(&description, "description", "", "Descripcion detallada")
	fs.StringVar(&note, "note", "", "Nota operativa de resolucion")
	fs.StringVar(&kind, "kind", "", "Tipo (bug|feature|refactor|audit_fix|ops)")
	fs.StringVar(&source, "source", "", "Fuente de origen")
	fs.StringVar(&sourceRef, "source-ref", "", "Referencia unica en la fuente")
	fs.Var(&pathFlags, "path", "Ruta relativa afectada (reemplaza affected_paths)")
	fs.Var(&pathsCSVFlags, "paths", "Rutas relativas afectadas separadas por coma")
	fs.Var(&impactFlags, "impact-flag", "Bandera de impacto arquitectonico")
	fs.IntVar(&maxFTDFiles, "max-ftd-files", intake.DefaultMaxFTDFiles, "Maximo de archivos para carril FTD")
	fs.BoolVar(&strictG4, "strict-g4", false, "Promover reglas Tier 2 G4 a rechazo fail-closed (Exit 3)")
	fs.BoolVar(&update, "update", false, "Sobrescribir WorkItem ready en caso de colision de dedupe_key")
	fs.BoolVar(&dryRun, "dry-run", false, "Simular resolucion sin escribir en disco")
	fs.StringVar(&repoRoot, "repo-root", "", "Raiz del repositorio Black Factory")
	fs.StringVar(&dataDir, "data-dir", "", "Directorio runtime de 1_Intake")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta del binario 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON estructurada")

	reordered := reorderFlags(args, map[string]bool{
		"strict-g4": true, "update": true, "dry-run": true, "json": true,
	}, map[string]bool{
		"id": true, "project": true, "title": true, "description": true, "note": true,
		"kind": true, "source": true, "source-ref": true, "path": true, "paths": true,
		"impact-flag": true, "max-ftd-files": true, "repo-root": true, "data-dir": true,
		"vault": true, "brain-bin": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return ExitUsageOrError
	}
	if strings.TrimSpace(targetID) == "" && fs.NArg() > 0 {
		targetID = fs.Arg(0)
	}
	if strings.TrimSpace(targetID) == "" {
		writeErrorOutput(errOut, asJSON, "MISSING_ID", "se requiere --id WI-<hash12> para resolve")
		return ExitUsageOrError
	}

	opts := resolveCommonDirs(repoRoot, dataDir, vaultRoot, brainBin, maxFTDFiles, strictG4, update, dryRun, asJSON)
	store := intake.NewStore(opts.dataDir)

	quarWI, _, err := store.LoadQuarantinedByID(targetID)
	if err != nil {
		writeErrorOutput(errOut, asJSON, "NOT_FOUND", err.Error())
		return ExitUsageOrError
	}

	var allCLIPaths []string
	allCLIPaths = append(allCLIPaths, pathFlags...)
	for _, csv := range pathsCSVFlags {
		for _, p := range strings.Split(csv, ",") {
			if t := strings.TrimSpace(p); t != "" {
				allCLIPaths = append(allCLIPaths, t)
			}
		}
	}

	cliOverrides := intake.RawIntake{
		Project:       project,
		Title:         title,
		Description:   description,
		Note:          note,
		Kind:          kind,
		Source:        source,
		SourceRef:     sourceRef,
		AffectedPaths: allCLIPaths,
		ImpactFlags:   []string(impactFlags),
	}

	if hits := intake.ScanRawAndFlagsTier1(nil, cliOverrides, strictG4); len(hits) > 0 {
		rawDigest, _ := json.Marshal(cliOverrides)
		return handleTier1Rejection(store, hits[0], rawDigest, cliOverrides.Source, cliOverrides.SourceRef, cliOverrides.Project, dryRun, asJSON, errOut)
	}

	baseFromQuar := intake.RawIntake{
		Project:       quarWI.Project,
		Title:         quarWI.Title,
		Description:   quarWI.Description,
		Note:          quarWI.Note,
		Kind:          quarWI.Kind,
		Source:        quarWI.Source,
		SourceRef:     quarWI.SourceRef,
		AffectedPaths: quarWI.AffectedPaths,
		ImpactFlags:   quarWI.ImpactFlags,
	}
	merged := intake.MergeCLIOverrides(baseFromQuar, cliOverrides)
	if hits := intake.ScanDecodedTier1(merged, strictG4); len(hits) > 0 {
		rawDigest, _ := json.Marshal(merged)
		return handleTier1Rejection(store, hits[0], rawDigest, merged.Source, merged.SourceRef, merged.Project, dryRun, asJSON, errOut)
	}

	opts.prevQuarID = quarWI.ID
	opts.existingCreated = quarWI.CreatedAt
	opts.existingRev = quarWI.Revision
	return executeStages2To5(merged, opts, store, client, out, errOut)
}

func executeStages2To5(merged intake.RawIntake, opts pipelineOptions, store *intake.Store, client intake.BrainClient, out, errOut io.Writer) int {
	start := time.Now()
	nowUTC := start.UTC().Format(time.RFC3339)

	// Redaccion en punto fijo Tier 2 e higiene pre-path.Clean de affected_paths
	sanitized, redactions, metaHasTier2 := intake.SanitizeAndRedactTier2(merged)
	cleanPaths, invalidPathReason := intake.SanitizeAffectedPaths(sanitized.AffectedPaths, opts.repoRoot)
	sortedFlags, flagsValid := intake.NormalizeAndValidateImpactFlags(sanitized.ImpactFlags)
	if !flagsValid {
		sortedFlags = []string{}
	}

	payloadSHA := intake.ComputeSanitizedSHA256(sanitized, cleanPaths, sortedFlags)
	var dedupeKey string
	if !metaHasTier2 {
		dedupeKey = intake.ComputeDedupeKey(sanitized.Project, sanitized.Source, sanitized.SourceRef)
	}
	wiID := intake.FormatWorkItemID(dedupeKey, payloadSHA)
	traceID := intake.FormatTraceID(start, payloadSHA)

	createdAt := nowUTC
	revision := 1
	if opts.existingCreated != "" {
		createdAt = opts.existingCreated
	}
	if opts.existingRev > 0 {
		revision = opts.existingRev + 1
	}

	// STAGE 3: Preflight Read-Only 0_Brain (NG2) + Compuerta de Metadatos y Alcance
	if client == nil {
		client = intake.NewSubprocessBrainClient(opts.brainBin, 5*time.Second)
	}
	dbPath := filepath.Join(opts.vaultRoot, ".index", "brain.db")
	preflight, pfErr := client.Preflight(opts.vaultRoot, dbPath)
	if pfErr != nil {
		writeErrorOutput(errOut, opts.asJSON, "BRAIN_UNAVAILABLE", pfErr.Error())
		return ExitBrainOrLockUnavailable
	}

	pathsConf := "explicit"
	if len(cleanPaths) == 0 {
		pathsConf = "none"
	}

	vaultDisplay := filepath.ToSlash(opts.vaultRoot)
	if opts.repoRoot != "" {
		if relV, relErr := filepath.Rel(opts.repoRoot, opts.vaultRoot); relErr == nil && !strings.HasPrefix(relV, "..") {
			vaultDisplay = filepath.ToSlash(relV)
		}
	}
	vaultDisplay, _ = bfcore.RedactTier2Text("vault_path", vaultDisplay)

	wi := &intake.WorkItem{
		SchemaVersion:     intake.SchemaVersion,
		ID:                wiID,
		DedupeKey:         dedupeKey,
		TraceID:           traceID,
		Revision:          revision,
		Status:            "needs_clarification",
		CreatedAt:         createdAt,
		UpdatedAt:         nowUTC,
		Project:           strings.ToLower(strings.TrimSpace(sanitized.Project)),
		Title:             strings.TrimSpace(sanitized.Title),
		Description:       strings.TrimSpace(sanitized.Description),
		Note:              strings.TrimSpace(sanitized.Note),
		Kind:              strings.ToLower(strings.TrimSpace(sanitized.Kind)),
		Source:            strings.ToLower(strings.TrimSpace(sanitized.Source)),
		SourceRef:         strings.TrimSpace(sanitized.SourceRef),
		AffectedPaths:     cleanPaths,
		PathsConfidence:   pathsConf,
		ImpactFlags:       sortedFlags,
		Lane:              "",
		SuggestedTemplate: "",
		RiskLevel:         "NONE",
		RequiresAutorHITL: false,
		RiskHits:          []intake.RiskHit{},
		TriageReasons:     []intake.TriageReason{},
		Sanitization: intake.SanitizationReport{
			StrictG4:               opts.strictG4,
			SanitizedPayloadSHA256: payloadSHA,
			G4Redactions:           redactions,
		},
		BrainEvidence: intake.BrainEvidence{
			VaultPath:            vaultDisplay,
			BrainDBMTime:         preflight.BrainDBMTime,
			RiskZonesSHA256:      "",
			ExtractedQueryTokens: []string{},
			Invocations:          []intake.BrainInvocation{},
			MatchedDocs:          []intake.BrainDocMatch{},
		},
	}

	if qInfo := intake.EvaluateMetadataAndScope(wi.ID, sanitized, cleanPaths, invalidPathReason, metaHasTier2, flagsValid); qInfo != nil {
		wi.QuarantineInfo = qInfo
		return persistQuarantinedWorkItem(store, wi, opts, start, out, errOut)
	}

	// STAGE 4: Consulta 0_Brain (<= 2 subprocesos: context + query) y Verificacion de Proyecto Activo
	ctxResp, ctxInv, ctxErr := client.Context(opts.vaultRoot, dbPath, wi.Project, wi.AffectedPaths)
	if ctxErr != nil {
		writeErrorOutput(errOut, opts.asJSON, "BRAIN_CONTEXT_FAILED", ctxErr.Error())
		return ExitBrainOrLockUnavailable
	}
	wi.BrainEvidence.Invocations = append(wi.BrainEvidence.Invocations, ctxInv)
	wi.BrainEvidence.RiskZonesSHA256 = ctxResp.RiskZonesSHA256
	wi.BrainEvidence.MatchedDocs = ctxResp.MatchedDocs

	tokens := intake.ExtractQueryTokens(wi.Title, wi.Description, 5)
	if tokens == nil {
		tokens = []string{}
	}
	wi.BrainEvidence.ExtractedQueryTokens = tokens
	orQuery := intake.BuildDisjunctiveFTSQuery(tokens)

	projectActive := intake.IsProjectActiveOnDisk(opts.vaultRoot, wi.Project)
	for _, d := range ctxResp.MatchedDocs {
		if strings.EqualFold(d.Type, "project") && strings.EqualFold(d.Status, "active") && strings.EqualFold(d.Project, wi.Project) {
			projectActive = true
		}
	}

	if projectActive {
		if orQuery != "" {
			qDocs, qInv, qErr := client.Query(opts.vaultRoot, dbPath, intake.BrainQueryOpts{
				Project: wi.Project,
				Query:   orQuery,
				Limit:   5,
			})
			if qErr != nil {
				writeErrorOutput(errOut, opts.asJSON, "BRAIN_QUERY_FAILED", qErr.Error())
				return ExitBrainOrLockUnavailable
			}
			wi.BrainEvidence.Invocations = append(wi.BrainEvidence.Invocations, qInv)
			wi.BrainEvidence.MatchedDocs = mergeBrainDocs(wi.BrainEvidence.MatchedDocs, qDocs)
		}
	} else {
		qDocs, qInv, qErr := client.Query(opts.vaultRoot, dbPath, intake.BrainQueryOpts{
			Project: wi.Project,
			Type:    "project",
			Limit:   5,
		})
		if qErr != nil {
			writeErrorOutput(errOut, opts.asJSON, "BRAIN_QUERY_FAILED", qErr.Error())
			return ExitBrainOrLockUnavailable
		}
		wi.BrainEvidence.Invocations = append(wi.BrainEvidence.Invocations, qInv)
		for _, d := range qDocs {
			if strings.EqualFold(d.Type, "project") && strings.EqualFold(d.Status, "active") &&
				(strings.EqualFold(d.Project, wi.Project) || strings.HasSuffix(strings.ToLower(d.RelPath), "/"+wi.Project+".md")) {
				projectActive = true
				break
			}
		}
	}

	if qInfo := intake.EvaluateProjectRegistration(wi.ID, sanitized, projectActive); qInfo != nil {
		wi.QuarantineInfo = qInfo
		return persistQuarantinedWorkItem(store, wi, opts, start, out, errOut)
	}

	// STAGE 5: Triaje Determinista de Dos Carriles (ASDD vs FTD), Idempotencia y Commit Atomico
	intake.TriageWorkItem(wi, ctxResp, opts.maxFTDFiles)
	durMs := time.Since(start).Milliseconds()

	_, saveErr := store.SaveTriaged(wi, opts.prevQuarID, opts.update, opts.dryRun, durMs)
	if saveErr != nil {
		if errors.Is(saveErr, intake.ErrDedupeConflict) {
			writeErrorOutput(errOut, opts.asJSON, "DEDUPE_CONFLICT", saveErr.Error())
			return ExitDedupeConflict
		}
		if errors.Is(saveErr, intake.ErrIntakeLocked) {
			writeErrorOutput(errOut, opts.asJSON, "INTAKE_LOCKED", saveErr.Error())
			return ExitBrainOrLockUnavailable
		}
		writeErrorOutput(errOut, opts.asJSON, "SAVE_ERROR", saveErr.Error())
		return ExitUsageOrError
	}

	writeWorkItemOutput(out, wi, opts.asJSON)
	return ExitOK
}

func persistQuarantinedWorkItem(store *intake.Store, wi *intake.WorkItem, opts pipelineOptions, start time.Time, out, errOut io.Writer) int {
	durMs := time.Since(start).Milliseconds()
	_, saveErr := store.SaveQuarantined(wi, opts.prevQuarID, opts.update, opts.dryRun, durMs)
	if saveErr != nil {
		if errors.Is(saveErr, intake.ErrDedupeConflict) {
			writeErrorOutput(errOut, opts.asJSON, "DEDUPE_CONFLICT", saveErr.Error())
			return ExitDedupeConflict
		}
		if errors.Is(saveErr, intake.ErrIntakeLocked) {
			writeErrorOutput(errOut, opts.asJSON, "INTAKE_LOCKED", saveErr.Error())
			return ExitBrainOrLockUnavailable
		}
		writeErrorOutput(errOut, opts.asJSON, "QUARANTINE_SAVE_ERROR", saveErr.Error())
		return ExitUsageOrError
	}
	writeWorkItemOutput(out, wi, opts.asJSON)
	return ExitQuarantined
}

func handleTier1Rejection(store *intake.Store, finding bfcore.G4Finding, rawBytes []byte, source, sourceRef, project string, dryRun, asJSON bool, errOut io.Writer) int {
	sum := sha256.Sum256(rawBytes)
	rawSHA := hex.EncodeToString(sum[:])
	now := time.Now().UTC()
	traceID := intake.FormatTraceID(now, rawSHA)

	ev := intake.IntakeEvent{
		Timestamp: now.Format(time.RFC3339),
		TraceID:   traceID,
		Event:     "REJECTED_G4",
		RuleName:  finding.Rule,
		Field:     finding.Field,
		Line:      finding.Line,
		RawSHA256: rawSHA,
		Project:   project,
		Source:    source,
		SourceRef: sourceRef,
	}
	if err := store.AppendRejectedG4Event(ev, dryRun); err != nil {
		if errors.Is(err, intake.ErrIntakeLocked) {
			writeErrorOutput(errOut, asJSON, "INTAKE_LOCKED", err.Error())
			return ExitBrainOrLockUnavailable
		}
	}

	if asJSON {
		writeJSON(errOut, map[string]any{
			"error":      "REJECTED_G4",
			"trace_id":   traceID,
			"rule_name":  finding.Rule,
			"field":      finding.Field,
			"line":       finding.Line,
			"raw_sha256": rawSHA,
			"message":    finding.Message,
		})
	} else {
		fmt.Fprintf(errOut, "error [REJECTED_G4]: regla=%s campo=%s linea=%d (%s)\n", finding.Rule, finding.Field, finding.Line, finding.Message)
	}
	return ExitRejectedG4
}

func cmdList(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("intake list", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var status, lane, project, risk, repoRoot, dataDir string
	var asJSON bool
	fs.StringVar(&status, "status", "", "Filtrar por estado (ready|needs_clarification)")
	fs.StringVar(&lane, "lane", "", "Filtrar por carril (ASDD|FTD)")
	fs.StringVar(&project, "project", "", "Filtrar por proyecto")
	fs.StringVar(&risk, "risk", "", "Filtrar por nivel de riesgo (NONE|BAJO|MEDIO|ALTO|CRITICO)")
	fs.StringVar(&repoRoot, "repo-root", "", "Raiz del repositorio")
	fs.StringVar(&dataDir, "data-dir", "", "Directorio runtime de 1_Intake")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"status": true, "lane": true, "project": true, "risk": true, "repo-root": true, "data-dir": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return ExitUsageOrError
	}

	opts := resolveCommonDirs(repoRoot, dataDir, "", "", intake.DefaultMaxFTDFiles, false, false, false, asJSON)
	store := intake.NewStore(opts.dataDir)
	items, err := store.List(intake.ListFilter{
		Status:  status,
		Lane:    lane,
		Project: project,
		Risk:    risk,
	})
	if err != nil {
		writeErrorOutput(errOut, asJSON, "LIST_ERROR", err.Error())
		return ExitUsageOrError
	}
	if items == nil {
		items = []intake.WorkItem{}
	}

	if asJSON {
		writeJSON(out, items)
	} else {
		if len(items) == 0 {
			fmt.Fprintln(out, "Sin WorkItems.")
			return ExitOK
		}
		for _, it := range items {
			fmt.Fprintf(out, "%s | status=%s | lane=%s | risk=%s | rev=%d | %s\n",
				it.ID, it.Status, it.Lane, it.RiskLevel, it.Revision, it.Title)
		}
	}
	return ExitOK
}

func cmdShow(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("intake show", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var targetID, format, repoRoot, dataDir string
	var asJSON bool
	fs.StringVar(&targetID, "id", "", "ID del WorkItem (WI-<hash12>)")
	fs.StringVar(&format, "format", "", "Formato de salida (json|md)")
	fs.StringVar(&repoRoot, "repo-root", "", "Raiz del repositorio")
	fs.StringVar(&dataDir, "data-dir", "", "Directorio runtime de 1_Intake")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"id": true, "format": true, "repo-root": true, "data-dir": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return ExitUsageOrError
	}
	if strings.TrimSpace(targetID) == "" && fs.NArg() > 0 {
		targetID = fs.Arg(0)
	}
	if strings.TrimSpace(targetID) == "" {
		writeErrorOutput(errOut, asJSON, "MISSING_ID", "se requiere --id WI-<hash12> para show")
		return ExitUsageOrError
	}

	fmtMode := strings.ToLower(strings.TrimSpace(format))
	if fmtMode == "" {
		if asJSON {
			fmtMode = "json"
		} else {
			fmtMode = "md"
		}
	}
	if fmtMode != "json" && fmtMode != "md" {
		writeErrorOutput(errOut, asJSON, "INVALID_FORMAT", fmt.Sprintf("formato no soportado %q (use json o md)", format))
		return ExitUsageOrError
	}

	opts := resolveCommonDirs(repoRoot, dataDir, "", "", intake.DefaultMaxFTDFiles, false, false, false, asJSON)
	store := intake.NewStore(opts.dataDir)
	wi, _, err := store.LoadByID(targetID)
	if err != nil {
		writeErrorOutput(errOut, asJSON, "NOT_FOUND", err.Error())
		return ExitUsageOrError
	}

	if fmtMode == "json" {
		writeJSON(out, wi)
	} else {
		fmt.Fprint(out, intake.RenderWorkItemMarkdown(wi))
	}
	return ExitOK
}

func cmdVerify(args []string, out, errOut io.Writer, client intake.BrainClient) int {
	fs := flag.NewFlagSet("intake verify", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var repoRoot, dataDir, vaultRoot, brainBin string
	var asJSON bool
	fs.StringVar(&repoRoot, "repo-root", "", "Raiz del repositorio Black Factory")
	fs.StringVar(&dataDir, "data-dir", "", "Directorio runtime de 1_Intake")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta del binario 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"repo-root": true, "data-dir": true, "vault": true, "brain-bin": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return ExitUsageOrError
	}

	opts := resolveCommonDirs(repoRoot, dataDir, vaultRoot, brainBin, intake.DefaultMaxFTDFiles, false, false, false, asJSON)
	if client == nil {
		client = intake.NewSubprocessBrainClient(opts.brainBin, 5*time.Second)
	}
	store := intake.NewStore(opts.dataDir)
	report, err := store.VerifyRuntime(opts.repoRoot, opts.vaultRoot, client)
	if err != nil {
		if errors.Is(err, intake.ErrIntakeLocked) {
			writeErrorOutput(errOut, asJSON, "INTAKE_LOCKED", err.Error())
			return ExitBrainOrLockUnavailable
		}
		writeErrorOutput(errOut, asJSON, "VERIFY_ERROR", err.Error())
		return ExitUsageOrError
	}

	if asJSON {
		writeJSON(out, report)
	} else {
		fmt.Fprintf(out, "Verify OK=%v | git_ignored=%v | brain_ok=%v | workitems=%d | quarantine=%d | events=%d | errors=%d\n",
			report.OK, report.GitIgnored, report.BrainOK, report.WorkItemsChecked, report.QuarantineChecked, report.EventsChecked, len(report.Errors))
		for _, e := range report.Errors {
			fmt.Fprintf(out, "  [ERROR] %s\n", e)
		}
	}
	if !report.OK {
		return ExitUsageOrError
	}
	return ExitOK
}

func resolveCommonDirs(repoFlag, dataFlag, vaultFlag, brainFlag string, maxFTD int, strict, update, dryRun, asJSON bool) pipelineOptions {
	root := discoverRepoRoot(repoFlag)

	dataDir := strings.TrimSpace(dataFlag)
	if dataDir == "" {
		dataDir = strings.TrimSpace(os.Getenv("BF_INTAKE_DIR"))
	}
	if dataDir == "" {
		if root != "" {
			dataDir = filepath.Join(root, "1_Intake", "runtime")
		} else {
			dataDir = filepath.Join("1_Intake", "runtime")
		}
	}

	vaultRoot := strings.TrimSpace(vaultFlag)
	if vaultRoot == "" {
		vaultRoot = strings.TrimSpace(os.Getenv("BRAIN_VAULT"))
	}
	if vaultRoot == "" {
		if root != "" {
			vaultRoot = filepath.Join(root, "0_Brain", "vault")
		} else {
			vaultRoot = filepath.Join("0_Brain", "vault")
		}
	}

	brainBin := strings.TrimSpace(brainFlag)
	if brainBin == "" {
		brainBin = strings.TrimSpace(os.Getenv("BRAIN_BIN"))
	}
	if brainBin == "" {
		if root != "" {
			brainBin = filepath.Join(root, "0_Brain", "brain.exe")
		} else {
			brainBin = filepath.Join("0_Brain", "brain.exe")
		}
	}

	return pipelineOptions{
		repoRoot:    root,
		dataDir:     dataDir,
		vaultRoot:   vaultRoot,
		brainBin:    brainBin,
		maxFTDFiles: maxFTD,
		strictG4:    strict,
		update:      update,
		dryRun:      dryRun,
		asJSON:      asJSON,
	}
}

func discoverRepoRoot(flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal)
	}
	if env := strings.TrimSpace(os.Getenv("BF_REPO_ROOT")); env != "" {
		return env
	}

	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}

	for _, start := range starts {
		cur := filepath.Clean(start)
		for {
			if isRepoRootDir(cur) {
				return cur
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	return ""
}

func isRepoRootDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	if fi, err := os.Stat(filepath.Join(dir, "0_Brain")); err == nil && fi.IsDir() {
		if fi2, err2 := os.Stat(filepath.Join(dir, "1_Intake")); err2 == nil && fi2.IsDir() {
			return true
		}
	}
	return false
}

func hasAnyCLIField(in intake.RawIntake) bool {
	return strings.TrimSpace(in.Project) != "" ||
		strings.TrimSpace(in.Title) != "" ||
		strings.TrimSpace(in.Description) != "" ||
		strings.TrimSpace(in.Note) != "" ||
		strings.TrimSpace(in.Kind) != "" ||
		strings.TrimSpace(in.Source) != "" ||
		strings.TrimSpace(in.SourceRef) != "" ||
		len(in.AffectedPaths) > 0 ||
		len(in.ImpactFlags) > 0
}

func mergeBrainDocs(base, extra []intake.BrainDocMatch) []intake.BrainDocMatch {
	seen := make(map[string]bool, len(base))
	out := make([]intake.BrainDocMatch, 0, len(base)+len(extra))
	for _, d := range base {
		seen[d.ID] = true
		out = append(out, d)
	}
	for _, d := range extra {
		if !seen[d.ID] {
			seen[d.ID] = true
			out = append(out, d)
		}
	}
	return out
}

func writeWorkItemOutput(out io.Writer, wi *intake.WorkItem, asJSON bool) {
	if asJSON {
		writeJSON(out, wi)
		return
	}
	if wi.Status == "ready" {
		fmt.Fprintf(out, "WorkItem %s triado: status=%s lane=%s template=%s risk=%s hitl=%v (rev=%d)\n",
			wi.ID, wi.Status, wi.Lane, wi.SuggestedTemplate, wi.RiskLevel, wi.RequiresAutorHITL, wi.Revision)
	} else {
		codes := ""
		if wi.QuarantineInfo != nil {
			codes = strings.Join(wi.QuarantineInfo.ReasonCodes, ",")
		}
		fmt.Fprintf(out, "WorkItem %s en cuarentena: status=%s reasons=[%s] (rev=%d)\n",
			wi.ID, wi.Status, codes, wi.Revision)
	}
}

func writeErrorOutput(errOut io.Writer, asJSON bool, code, msg string) {
	if asJSON {
		writeJSON(errOut, map[string]string{
			"error":   code,
			"message": msg,
		})
		return
	}
	fmt.Fprintf(errOut, "error [%s]: %s\n", code, msg)
}

func writeJSON(out io.Writer, v any) {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func reorderFlags(args []string, boolFlags, valFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			flags = append(flags, a)
			continue
		}
		if valFlags[name] && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i++
			continue
		}
		if boolFlags[name] && i+1 < len(args) {
			low := strings.ToLower(args[i+1])
			if low == "true" || low == "false" {
				flags = append(flags, a+"="+low)
				i++
				continue
			}
		}
		flags = append(flags, a)
	}
	return append(flags, pos...)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Uso: intake <subcomando> [banderas]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcomandos:")
	fmt.Fprintln(w, "  ingest   Recibe, sanitiza (G4 Tier 1/Tier 2) y clasifica una solicitud (ASDD | FTD | Quarantine)")
	fmt.Fprintln(w, "  resolve  Completa y promueve atomicamente un WorkItem desde runtime/quarantine/")
	fmt.Fprintln(w, "  list     Lista WorkItems en workitems/ y quarantine/ con filtros opcionales")
	fmt.Fprintln(w, "  show     Muestra un WorkItem en formato JSON o Markdown ASCII/Box-Drawing bajo demanda")
	fmt.Fprintln(w, "  verify   Verifica integridad G4, esquema v1.0, aislamiento Git Opcion i y conexion 0_Brain")
}
