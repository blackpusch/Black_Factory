package specwriter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"blackfactory/pkg/bfcore"
)

const (
	SchemaVersion = "1.0"

	// Codigos de salida deterministas 0..6 de specwriter.exe.
	ExitOK               = 0
	ExitFixable          = 1
	ExitUsage            = 2
	ExitG4               = 3
	ExitExternalDecision = 4
	ExitAttemptsBlocked  = 5
	ExitInfraOrRefusal   = 6

	// Limites operativos canonicos.
	MaxValidateAttemptsPerStage = 3
	MaxExternalCycles           = 3
	MaxVerifyTimeoutSec         = 300
	MaxWorkUnitsFTD             = 2
	MaxWorkUnitsASDD            = 99
	MaxSpecBytes                = 1 << 20   // 1 MiB
	MaxGoldfishBytes            = 256 << 10 // 256 KiB
	MaxObservedFileBytes        = bfcore.MaxObservedFileBytes
)

// Catalogo cerrado de codigos Reason para CLIResult, PrepareResult, VerifyReport y telemetria JSONL.
const (
	ReasonOK                    = "OK"
	ReasonValidationFailed      = "VALIDATION_FAILED"
	ReasonInvalidJSON           = "INVALID_JSON"
	ReasonSpecTooLarge          = "SPEC_TOO_LARGE"
	ReasonValidationRequired    = "VALIDATION_REQUIRED"
	ReasonValidatorBypassDefect = "VALIDATOR_BYPASS_DEFECT"

	ReasonInvalidUsage      = "INVALID_USAGE"
	ReasonSpecAlreadyExists = "SPEC_ALREADY_EXISTS"

	ReasonG4Violation       = "G4_VIOLATION"
	ReasonG4ViolationOnDisk = "G4_VIOLATION_ON_DISK"

	ReasonStaleContext        = "STALE_CONTEXT"
	ReasonEscalationRequired  = "ESCALATION_REQUIRED"
	ReasonLaneUpgradeRequired = "LANE_UPGRADE_REQUIRED"

	ReasonMaxValidateAttempts = "MAX_VALIDATE_ATTEMPTS"
	ReasonMaxExternalCycles   = "MAX_EXTERNAL_CYCLES"

	ReasonBrainUnavailable     = "BRAIN_UNAVAILABLE"
	ReasonBrainJournalActive   = "BRAIN_JOURNAL_ACTIVE"
	ReasonLockBusy             = "LOCK_BUSY"
	ReasonIOError              = "IO_ERROR"
	ReasonLiveTaskState        = "LIVE_TASK_STATE"
	ReasonTaskDirExists        = "TASK_DIR_EXISTS"
	ReasonAttemptsStateMissing = "ATTEMPTS_STATE_MISSING"
	ReasonAttemptsStateCorrupt = "ATTEMPTS_STATE_CORRUPT"
	ReasonArtifactStateCorrupt = "ARTIFACT_STATE_CORRUPT"
	ReasonReparsePointDetected = "REPARSE_POINT_DETECTED"
	ReasonGitIgnoreDrift       = "GITIGNORE_DRIFT"
	ReasonGengisEngineDrift    = "GENGIS_ENGINE_DRIFT"
)

// IsRetryableReason devuelve la politica determinista de reintento para cada par (exitCode, reason).
func IsRetryableReason(code int, reason string) bool {
	switch code {
	case ExitFixable:
		switch reason {
		case ReasonValidationFailed, ReasonInvalidJSON, ReasonSpecTooLarge, ReasonValidationRequired:
			return true
		default:
			return false
		}
	case ExitInfraOrRefusal:
		switch reason {
		case ReasonBrainUnavailable, ReasonBrainJournalActive, ReasonLockBusy, ReasonIOError:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// WorkItemRef vincula spec.json de forma verificable con el WorkItem inmutable de 1_Intake.
type WorkItemRef struct {
	ID                     string `json:"id"`
	Revision               int    `json:"revision"`
	SanitizedPayloadSHA256 string `json:"sanitized_payload_sha256"`
	TraceID                string `json:"trace_id"`
	SourceWorkItemPath     string `json:"source_workitem_path"`
}

// Narrative agrupa las secciones narrativas base (FTD) y extendidas (ASDD) de spec.json.
type Narrative struct {
	Problem                string   `json:"problem"`
	RootCause              string   `json:"root_cause"`
	ProposedChange         string   `json:"proposed_change"`
	AlternativesConsidered []string `json:"alternatives_considered"`
	InterfacesAndContracts string   `json:"interfaces_and_contracts"`
	EdgeCases              []string `json:"edge_cases"`
	RollbackPlan           string   `json:"rollback_plan"`
}

// ObservedFile registra el estado ("present" | "absent") y SHA-256 normalizado CRLF/BOM de un archivo observado.
type ObservedFile struct {
	Path   string `json:"path"`
	State  string `json:"state"`
	SHA256 string `json:"sha256"`
}

// G5Citation registra una cita verificable en disco (linea 1-based CRLF/BOM y ancla literal +-5 lineas).
type G5Citation struct {
	Path         string `json:"path"`
	Line         int    `json:"line"`
	SymbolAnchor string `json:"symbol_anchor"`
	Claim        string `json:"claim"`
}

// DecisionRecord vincula una decision de escalado o ascenso de carril al scope_sha256 exacto.
type DecisionRecord struct {
	Reason        string   `json:"reason"`
	DecidedBy     string   `json:"decided_by"`
	Rationale     string   `json:"rationale"`
	Timestamp     string   `json:"timestamp"`
	LaneFrom      string   `json:"lane_from"`
	LaneTo        string   `json:"lane_to"`
	ApprovedPaths []string `json:"approved_paths"`
	ScopeSHA256   string   `json:"scope_sha256"`
}

// RiskMitigation enlaza cada zona de riesgo impactada con verify_steps y review_gates.
type RiskMitigation struct {
	RiskID            string   `json:"risk_id"`
	Level             string   `json:"level"`
	VerifyStepIDs     []string `json:"verify_step_ids"`
	ReviewGates       []string `json:"review_gates"`
	RequiresAutorHITL bool     `json:"requires_autor_hitl"`
}

// BrainFollowup documenta actualizaciones post-merge sugeridas para 0_Brain (sin ejecutar propose en v1).
type BrainFollowup struct {
	TargetHint string `json:"target_hint"`
	Summary    string `json:"summary"`
	Trigger    string `json:"trigger"`
}

// ScopeExpansion justifica cada archivo anadido en can_modify fuera de WorkItem.affected_paths.
type ScopeExpansion struct {
	Path      string `json:"path"`
	Rationale string `json:"rationale"`
}

// AcceptanceCriterion representa un criterio de aceptacion ejecutable (command) o compuerta de revision (review_gate).
type AcceptanceCriterion struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Description  string `json:"description"`
	VerifyStepID string `json:"verify_step_id,omitempty"`
	Executor     string `json:"executor,omitempty"`
}

// VerifyStep modela un comando estructurado auditado contra el allowlist cerrado antes de serializar a cmd.exe.
type VerifyStep struct {
	ID         string            `json:"id"`
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env"`
	Argv       []string          `json:"argv"`
	ExpectExit int               `json:"expect_exit"`
	TimeoutSec int               `json:"timeout_sec"`
}

// FailureProof documenta la prueba de que la verificacion del chunk puede fallar (expect_exit 1..255).
type FailureProof struct {
	Description          string `json:"description"`
	ExpectExit           int    `json:"expect_exit"`
	ExpectOutputContains string `json:"expect_output_contains,omitempty"`
}

// WorkUnit representa una unidad compilable 1-a-1 a chunks/NN_<id>.md de GengiSWAP.
type WorkUnit struct {
	ID                 string                `json:"id"`
	Title              string                `json:"title"`
	Objective          string                `json:"objective"`
	Inputs             []string              `json:"inputs"`
	CanModify          []string              `json:"can_modify"`
	ScopeExpansions    []ScopeExpansion      `json:"scope_expansions"`
	Forbidden          []string              `json:"forbidden"`
	DependsOn          []string              `json:"depends_on"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	VerifySteps        []VerifyStep          `json:"verify_steps"`
	FailureProof       FailureProof          `json:"failure_proof"`
}

// Spec es la unica fuente de verdad (schema_version: "1.0") editada por el agente y compilada a GengiSWAP.
type Spec struct {
	SchemaVersion     string           `json:"schema_version"`
	WorkItemRef       WorkItemRef      `json:"work_item_ref"`
	Project           string           `json:"project"`
	Lane              string           `json:"lane"`
	Template          string           `json:"template"`
	TargetBranch      string           `json:"target_branch"`
	Title             string           `json:"title"`
	G3DataMutation    string           `json:"g3_data_mutation"`
	RequiresAutorHITL bool             `json:"requires_autor_hitl"`
	StackAssumptions  []string         `json:"stack_assumptions"`
	ReadOnlyPaths     []string         `json:"read_only_paths"`
	GlobalForbidden   []string         `json:"global_forbidden"`
	DoD               []string         `json:"dod"`
	Narrative         Narrative        `json:"narrative"`
	ObservedFiles     []ObservedFile   `json:"observed_files"`
	G5Citations       []G5Citation     `json:"g5_citations"`
	Decisions         []DecisionRecord `json:"decisions"`
	RiskMitigations   []RiskMitigation `json:"risk_mitigations"`
	BrainFollowups    []BrainFollowup  `json:"brain_followups"`
	WorkUnits         []WorkUnit       `json:"work_units"`
}

// TriageReasonInput modela una entrada de triage_reasons del WorkItem v1.0 de 1_Intake.
type TriageReasonInput struct {
	Code           string `json:"code"`
	RuleKind       string `json:"rule_kind"`
	RulesetVersion string `json:"ruleset_version"`
	Detail         string `json:"detail"`
}

// SanitizationInput modela la seccion sanitization del WorkItem v1.0 de 1_Intake.
type SanitizationInput struct {
	StrictG4               bool                 `json:"strict_g4"`
	SanitizedPayloadSHA256 string               `json:"sanitized_payload_sha256"`
	G4Redactions           []bfcore.G4Redaction `json:"g4_redactions"`
}

// WorkItemInput representa los campos leidos de un WorkItem v1.0 emitido por 1_Intake.
type WorkItemInput struct {
	SchemaVersion     string              `json:"schema_version"`
	ID                string              `json:"id"`
	DedupeKey         string              `json:"dedupe_key"`
	TraceID           string              `json:"trace_id"`
	Revision          int                 `json:"revision"`
	Status            string              `json:"status"`
	Project           string              `json:"project"`
	Title             string              `json:"title"`
	Description       string              `json:"description"`
	Kind              string              `json:"kind"`
	AffectedPaths     []string            `json:"affected_paths"`
	ImpactFlags       []string            `json:"impact_flags"`
	Lane              string              `json:"lane"`
	SuggestedTemplate string              `json:"suggested_template"`
	RiskLevel         string              `json:"risk_level"`
	RequiresAutorHITL bool                `json:"requires_autor_hitl"`
	RiskHits          []bfcore.RiskHit    `json:"risk_hits"`
	TriageReasons     []TriageReasonInput `json:"triage_reasons"`
	Sanitization      SanitizationInput   `json:"sanitization"`
}

// WorkItemSnapshot conserva la fotografia inmutable del WorkItem dentro de spec_context.json.
type WorkItemSnapshot struct {
	ID                     string              `json:"id"`
	Revision               int                 `json:"revision"`
	SanitizedPayloadSHA256 string              `json:"sanitized_payload_sha256"`
	Project                string              `json:"project"`
	Kind                   string              `json:"kind"`
	Lane                   string              `json:"lane"`
	SuggestedTemplate      string              `json:"suggested_template"`
	RiskLevel              string              `json:"risk_level"`
	RequiresAutorHITL      bool                `json:"requires_autor_hitl"`
	AffectedPaths          []string            `json:"affected_paths"`
	ImpactFlags            []string            `json:"impact_flags"`
	TriageReasons          []TriageReasonInput `json:"triage_reasons"`
	RiskHits               []bfcore.RiskHit    `json:"risk_hits"`
}

// MatchedDocHash registra el SHA-256 normalizado de un documento activo de 0_Brain relativo a --vault.
type MatchedDocHash struct {
	RelPath string `json:"rel_path"`
	SHA256  string `json:"sha256"`
}

// BrainSnapshot congela la consulta read-only de prepare sobre 0_Brain.
type BrainSnapshot struct {
	BrainDBMTime     string                   `json:"brain_db_mtime"`
	RiskZonesSHA256  string                   `json:"risk_zones_sha256"`
	MatchedDocHashes []MatchedDocHash         `json:"matched_doc_hashes"`
	Invocations      []bfcore.BrainInvocation `json:"invocations"`
}

// SpecContext es el snapshot congelado por prepare (sin rutas absolutas de host para cumplir G4).
type SpecContext struct {
	SchemaVersion      string           `json:"schema_version"`
	CreatedAt          string           `json:"created_at"`
	TraceID            string           `json:"trace_id"`
	SourceWorkItemPath string           `json:"source_workitem_path"`
	RepoRootBasename   string           `json:"repo_root_basename"`
	RepoRootSHA256     string           `json:"repo_root_sha256"`
	GitHead            string           `json:"git_head"`
	WorkItem           WorkItemSnapshot `json:"work_item"`
	BrainSnapshot      BrainSnapshot    `json:"brain_snapshot"`
	BaselineHashes     []ObservedFile   `json:"baseline_hashes"`
}

// AttemptsState modela el freno operativo contra bucles en attempts.json.
type AttemptsState struct {
	SchemaVersion            string `json:"schema_version"`
	WIID                     string `json:"wi_id"`
	Project                  string `json:"project"`
	DraftAttempts            int    `json:"draft_attempts"`
	FinalAttempts            int    `json:"final_attempts"`
	ExternalCycles           int    `json:"external_cycles"`
	LastExit                 int    `json:"last_exit"`
	LastReason               string `json:"last_reason"`
	LastPassedSHA256         string `json:"last_passed_sha256"`
	LastPassedStage          string `json:"last_passed_stage"`
	LastPassedGoldfishSHA256 string `json:"last_passed_goldfish_sha256"`
	UpdatedAt                string `json:"updated_at"`
}

// GoldfishReview registra la higiene de consistencia de la revision Goldfish para ASDD --stage final.
type GoldfishReview struct {
	SchemaVersion      string   `json:"schema_version"`
	SpecSHA256         string   `json:"spec_sha256"`
	LensesRun          []string `json:"lenses_run"`
	UnresolvedBlockers int      `json:"unresolved_blockers"`
	ReportPath         string   `json:"report_path"`
	ReviewedAt         string   `json:"reviewed_at"`
}

// ValidationIssue identifica un error estructurado con JSON Pointer (RFC 6901).
type ValidationIssue struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RenderedChunk contiene el nombre de archivo y contenido Markdown de un chunk compilado.
type RenderedChunk struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// RenderedTaskFiles agrupa los buffers en memoria de index.md (<TARGET_REPO>), chunks/*.md y status.md.
type RenderedTaskFiles struct {
	IndexMD  string          `json:"index_md"`
	StatusMD string          `json:"status_md"`
	Chunks   []RenderedChunk `json:"chunks"`
}

// SpecWriterEvent modela una linea JSONL en runtime/specwriter_events.jsonl.
type SpecWriterEvent struct {
	Timestamp  string `json:"timestamp"`
	Event      string `json:"event"`
	TraceID    string `json:"trace_id"`
	Project    string `json:"project,omitempty"`
	WorkItemID string `json:"work_item_id,omitempty"`
	Stage      string `json:"stage,omitempty"`
	ExitCode   int    `json:"exit_code"`
	Reason     string `json:"reason"`
	Retryable  bool   `json:"retryable"`
	SpecSHA256 string `json:"spec_sha256,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
}

// CLIResult es el sobre JSON unificado para validate, render y compile.
type CLIResult struct {
	OK         bool              `json:"ok"`
	ExitCode   int               `json:"exit_code"`
	Reason     string            `json:"reason"`
	Retryable  bool              `json:"retryable"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpecSHA256 string            `json:"spec_sha256,omitempty"`
	LastExit   int               `json:"last_exit,omitempty"`
	LastReason string            `json:"last_reason,omitempty"`
	TaskDir    string            `json:"task_dir,omitempty"`
	Markdown   string            `json:"markdown,omitempty"`
	Issues     []ValidationIssue `json:"issues,omitempty"`
}

// PrepareResult es el sobre JSON estructurado para specwriter prepare.
type PrepareResult struct {
	OK           bool   `json:"ok"`
	ExitCode     int    `json:"exit_code"`
	Reason       string `json:"reason"`
	Retryable    bool   `json:"retryable"`
	TraceID      string `json:"trace_id,omitempty"`
	WorkItemID   string `json:"work_item_id,omitempty"`
	SpecPath     string `json:"spec_path,omitempty"`
	ContextPath  string `json:"context_path,omitempty"`
	AttemptsPath string `json:"attempts_path,omitempty"`
	Refreshed    bool   `json:"refreshed"`
	Reset        bool   `json:"reset"`
	Detail       string `json:"detail,omitempty"`
}

// VerifyReport es el sobre JSON estructurado para specwriter verify (100% solo lectura).
type VerifyReport struct {
	OK               bool              `json:"ok"`
	ExitCode         int               `json:"exit_code"`
	Reason           string            `json:"reason"`
	Retryable        bool              `json:"retryable"`
	GitIgnoreOK      bool              `json:"gitignore_ok"`
	GengisHashesOK   bool              `json:"gengis_hashes_ok"`
	BrainOK          bool              `json:"brain_ok"`
	G4Issues         int               `json:"g4_issues"`
	ArtifactsChecked int               `json:"artifacts_checked"`
	Issues           []ValidationIssue `json:"issues"`
}

// PrepareOptions parametriza Prepare.
type PrepareOptions struct {
	WorkItemPath   string
	RepoRoot       string
	FactoryRoot    string
	VaultRoot      string
	DataDir        string
	BrainBin       string
	RefreshContext bool
	ResetAttempts  bool
	Reason         string
	BrainClient    bfcore.BrainClient
}

// ValidateOptions parametriza Validate.
type ValidateOptions struct {
	SpecPath    string
	Stage       string
	RepoRoot    string
	FactoryRoot string
	VaultRoot   string
	DataDir     string
	BrainBin    string
	BrainClient bfcore.BrainClient
}

// CompileOptions parametriza Compile.
type CompileOptions struct {
	SpecPath    string
	TaskRoot    string
	TaskID      string
	RepoRoot    string
	FactoryRoot string
	VaultRoot   string
	DataDir     string
	BrainBin    string
	BrainClient bfcore.BrainClient
}

// VerifyOptions parametriza VerifyRuntime.
type VerifyOptions struct {
	FactoryRoot string
	RepoRoot    string
	VaultRoot   string
	DataDir     string
	BrainBin    string
	BrainClient bfcore.BrainClient
}

// NormalizeSpecForCanonicalCopy devuelve una copia determinista de Spec donde todo slice nil
// se normaliza a [] y todo VerifyStep.Env nil se normaliza a {}.
func NormalizeSpecForCanonicalCopy(s Spec) Spec {
	out := s
	if out.StackAssumptions == nil {
		out.StackAssumptions = []string{}
	}
	if out.ReadOnlyPaths == nil {
		out.ReadOnlyPaths = []string{}
	}
	if out.GlobalForbidden == nil {
		out.GlobalForbidden = []string{}
	}
	if out.DoD == nil {
		out.DoD = []string{}
	}
	if out.Narrative.AlternativesConsidered == nil {
		out.Narrative.AlternativesConsidered = []string{}
	}
	if out.Narrative.EdgeCases == nil {
		out.Narrative.EdgeCases = []string{}
	}
	if out.ObservedFiles == nil {
		out.ObservedFiles = []ObservedFile{}
	}
	if out.G5Citations == nil {
		out.G5Citations = []G5Citation{}
	}
	if out.Decisions == nil {
		out.Decisions = []DecisionRecord{}
	} else {
		decs := make([]DecisionRecord, len(out.Decisions))
		for i, d := range out.Decisions {
			if d.ApprovedPaths == nil {
				d.ApprovedPaths = []string{}
			}
			decs[i] = d
		}
		out.Decisions = decs
	}
	if out.RiskMitigations == nil {
		out.RiskMitigations = []RiskMitigation{}
	} else {
		rms := make([]RiskMitigation, len(out.RiskMitigations))
		for i, r := range out.RiskMitigations {
			if r.VerifyStepIDs == nil {
				r.VerifyStepIDs = []string{}
			}
			if r.ReviewGates == nil {
				r.ReviewGates = []string{}
			}
			rms[i] = r
		}
		out.RiskMitigations = rms
	}
	if out.BrainFollowups == nil {
		out.BrainFollowups = []BrainFollowup{}
	}
	if out.WorkUnits == nil {
		out.WorkUnits = []WorkUnit{}
	} else {
		units := make([]WorkUnit, len(out.WorkUnits))
		for i, u := range out.WorkUnits {
			if u.Inputs == nil {
				u.Inputs = []string{}
			}
			if u.CanModify == nil {
				u.CanModify = []string{}
			}
			if u.ScopeExpansions == nil {
				u.ScopeExpansions = []ScopeExpansion{}
			}
			if u.Forbidden == nil {
				u.Forbidden = []string{}
			}
			if u.DependsOn == nil {
				u.DependsOn = []string{}
			}
			if u.AcceptanceCriteria == nil {
				u.AcceptanceCriteria = []AcceptanceCriterion{}
			}
			if u.VerifySteps == nil {
				u.VerifySteps = []VerifyStep{}
			} else {
				steps := make([]VerifyStep, len(u.VerifySteps))
				for j, st := range u.VerifySteps {
					if st.Env == nil {
						st.Env = map[string]string{}
					}
					if st.Argv == nil {
						st.Argv = []string{}
					}
					steps[j] = st
				}
				u.VerifySteps = steps
			}
			units[i] = u
		}
		out.WorkUnits = units
	}
	return out
}

// CanonicalSpecSHA256 calcula el SHA-256 hex en minusculas sobre la serializacion JSON compacta
// y canonizada de Spec (slices nil -> [] y VerifyStep.Env nil -> {}).
func CanonicalSpecSHA256(s Spec) (string, error) {
	norm := NormalizeSpecForCanonicalCopy(s)
	b, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// ComputeScopeSHA256 calcula el SHA-256 hex sobre el conjunto unico, normalizado en minusculas
// y ordenado lexicograficamente de rutas can_modify unidas por "\n".
func ComputeScopeSHA256(paths []string) string {
	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, p := range paths {
		n := strings.ToLower(bfcore.NormalizeCoverPath(p))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		unique = append(unique, n)
	}
	sort.Strings(unique)
	sum := sha256.Sum256([]byte(strings.Join(unique, "\n")))
	return hex.EncodeToString(sum[:])
}

// ComputeRepoRootIdentity deriva repo_root_basename y repo_root_sha256 sin persistir rutas absolutas de host.
func ComputeRepoRootIdentity(repoRoot string) (basename string, sha256Hex string, absClean string, err error) {
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", "", "", err
	}
	absClean = filepath.Clean(abs)
	normSlash := strings.ToLower(strings.TrimRight(filepath.ToSlash(absClean), "/"))
	sum := sha256.Sum256([]byte(normSlash))
	base := filepath.Base(absClean)
	if len(bfcore.ScanG4All(base)) > 0 {
		base = "repo"
	}
	return base, hex.EncodeToString(sum[:]), absClean, nil
}
