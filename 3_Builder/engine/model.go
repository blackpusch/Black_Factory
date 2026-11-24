package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"blackfactory/pkg/bfcore"
)

const (
	SchemaVersion = "1.0"

	// Codigos de salida deterministas 0..6 de builder.exe.
	ExitOK               = 0
	ExitFixable          = 1
	ExitUsage            = 2
	ExitG4               = 3
	ExitExternalDecision = 4
	ExitAttemptsBlocked  = 5
	ExitInfraOrRefusal   = 6

	// Cuotas y limites operativos canonicos.
	MaxBounceCycles       = 3
	MaxVerifyTimeoutSec   = 300
	MaxCommandOutputBytes = 8000
	MaxDiffBytes          = 2 << 20   // 2 MiB
	MaxOrderBytes         = 256 << 10 // 256 KiB
)

// Catalogo cerrado de codigos Reason para CLIResult, ContextResult, VerifyReport y telemetria JSONL.
const (
	ReasonOK = "OK"

	ReasonScopeViolation          = "SCOPE_VIOLATION"
	ReasonTDDRedPassedUnexpected  = "TDD_RED_PASSED_UNEXPECTEDLY"
	ReasonTDDRedNoChanges         = "TDD_RED_NO_CHANGES"
	ReasonTDDRedMissingTestChange = "TDD_RED_MISSING_TEST_CHANGE"
	ReasonTDDGreenFailed          = "TDD_GREEN_FAILED"
	ReasonTDDGreenUnchanged       = "TDD_GREEN_UNCHANGED_FROM_RED"
	ReasonInvalidJSON             = "INVALID_JSON"
	ReasonOrderValidationFailed   = "ORDER_VALIDATION_FAILED"
	ReasonBundleMismatch          = "BUNDLE_MISMATCH"
	ReasonApprovalRequired        = "APPROVAL_REQUIRED"
	ReasonWorktreeDriftAfterAppr  = "WORKTREE_DRIFT_AFTER_APPROVAL"
	ReasonWorktreeDirty           = "WORKTREE_DIRTY"
	ReasonDiffTooLarge            = "DIFF_TOO_LARGE"

	ReasonInvalidUsage         = "INVALID_USAGE"
	ReasonSessionAlreadyExists = "SESSION_ALREADY_EXISTS"
	ReasonSessionNotFound      = "SESSION_NOT_FOUND"
	ReasonTaskDirInvalid       = "TASK_DIR_INVALID"
	ReasonCmdIndexOutOfRange   = "CMD_INDEX_OUT_OF_RANGE"

	ReasonG4Violation = "G4_VIOLATION"

	ReasonPlanNotApproved            = "PLAN_NOT_APPROVED"
	ReasonTaskTerminalState          = "TASK_TERMINAL_STATE"
	ReasonStaleBrainContext          = "STALE_BRAIN_CONTEXT"
	ReasonTDDRedRequired             = "TDD_RED_REQUIRED"
	ReasonUnexpectedCommitInWorktree = "UNEXPECTED_COMMIT_IN_WORKTREE"
	ReasonBranchCollision            = "BRANCH_COLLISION"
	ReasonGengisDecideRequired       = "GENGIS_DECIDE_REQUIRED"

	ReasonMaxBounceCycles = "MAX_BOUNCE_CYCLES"

	ReasonBrainUnavailable        = "BRAIN_UNAVAILABLE"
	ReasonBrainJournalActive      = "BRAIN_JOURNAL_ACTIVE"
	ReasonLockBusy                = "LOCK_BUSY"
	ReasonGitError                = "GIT_ERROR"
	ReasonGengisUnavailable       = "GENGIS_UNAVAILABLE"
	ReasonIOError                 = "IO_ERROR"
	ReasonTaskAlreadyActive       = "TASK_ALREADY_ACTIVE"
	ReasonWorktreePathMismatch    = "WORKTREE_PATH_MISMATCH"
	ReasonWorktreeMissing         = "WORKTREE_MISSING"
	ReasonSessionStateCorrupt     = "SESSION_STATE_CORRUPT"
	ReasonArtifactStateCorrupt    = "ARTIFACT_STATE_CORRUPT"
	ReasonReparsePointDetected    = "REPARSE_POINT_DETECTED"
	ReasonGitIgnoreDrift          = "GITIGNORE_DRIFT"
	ReasonGengisEngineDrift       = "GENGIS_ENGINE_DRIFT"
	ReasonIndexBindInvariantBreak = "INDEX_BIND_INVARIANT_BROKEN"
)

// IsRetryableReason devuelve la politica determinista de reintento para cada par (exitCode, reason).
func IsRetryableReason(code int, reason string) bool {
	switch code {
	case ExitFixable:
		return true
	case ExitInfraOrRefusal:
		switch reason {
		case ReasonBrainUnavailable, ReasonBrainJournalActive, ReasonLockBusy, ReasonGitError, ReasonGengisUnavailable, ReasonIOError:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// SessionState unifica en un unico archivo atomico (3_Builder/runtime/<task_id>/session.json)
// el estado de sesion, el snapshot read-only de 0_Brain y la evidencia Red de TDD.
type SessionState struct {
	SchemaVersion                 string               `json:"schema_version"`
	TaskID                        string               `json:"task_id"`
	Project                       string               `json:"project"`
	WorkItemID                    string               `json:"work_item_id"`
	TraceID                       string               `json:"trace_id"`
	SpecSHA256                    string               `json:"spec_sha256"`
	Lane                          string               `json:"lane"`
	RepoRootBasename              string               `json:"repo_root_basename"`
	RepoRootSHA256                string               `json:"repo_root_sha256"`
	WorktreeBasename              string               `json:"worktree_basename"`
	WorktreePathSHA256            string               `json:"worktree_path_sha256"`
	TargetBranch                  string               `json:"target_branch"`
	BuildBranch                   string               `json:"build_branch"`
	InitialBaseRef                string               `json:"initial_base_ref"`
	ActiveChunkID                 string               `json:"active_chunk_id"`
	ActiveAttempt                 int                  `json:"active_attempt"`
	ActiveBaseRef                 string               `json:"active_base_ref"`
	ResetEpoch                    int                  `json:"reset_epoch"`
	BounceCycle                   int                  `json:"bounce_cycle"`
	BounceBrakeActive             bool                 `json:"bounce_brake_active"`
	PreviousBounceIDs             []string             `json:"previous_bounce_ids"`
	SeenOrderIDs                  []string             `json:"seen_order_ids"`
	ActiveBounceOrderID           string               `json:"active_bounce_order_id"`
	RequiresRedTest               bool                 `json:"requires_red_test"`
	LastBouncedWorktreeTreeSHA256 string               `json:"last_bounced_worktree_tree_sha256"`
	LatestBundleID                string               `json:"latest_bundle_id"`
	LatestBundleSHA256            string               `json:"latest_bundle_sha256"`
	LatestWorktreeTreeSHA256      string               `json:"latest_worktree_tree_sha256"`
	ApprovedBundleID              string               `json:"approved_bundle_id"`
	ApprovedBundleSHA256          string               `json:"approved_bundle_sha256"`
	ApprovedWorktreeTreeSHA256    string               `json:"approved_worktree_tree_sha256"`
	WorktreeStatus                string               `json:"worktree_status"`
	BrainContext                  BrainContextSnapshot `json:"brain_context"`
	TDD                           TDDState             `json:"tdd"`
	CreatedAt                     string               `json:"created_at"`
	UpdatedAt                     string               `json:"updated_at"`
}

// MatchedDocHash registra el SHA-256 normalizado de un documento activo de 0_Brain relativo a --vault.
type MatchedDocHash struct {
	RelPath string `json:"rel_path"`
	SHA256  string `json:"sha256"`
}

// BrainDocSummary resume un documento activo devuelto por 0_Brain context.
type BrainDocSummary struct {
	ID           string `json:"id"`
	RelPath      string `json:"rel_path"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	VerifiedHead string `json:"verified_head"`
}

// BrainContextSnapshot congela la consulta read-only a 0_Brain para la tarea/chunk activo.
type BrainContextSnapshot struct {
	ChunkID          string                   `json:"chunk_id"`
	QueriedFiles     []string                 `json:"queried_files"`
	BrainDBMTime     string                   `json:"brain_db_mtime"`
	RiskZonesSHA256  string                   `json:"risk_zones_sha256"`
	HighestRiskLevel string                   `json:"highest_risk_level"`
	MatchedDocs      []BrainDocSummary        `json:"matched_docs"`
	MatchedDocHashes []MatchedDocHash         `json:"matched_doc_hashes"`
	RiskHits         []bfcore.RiskHit         `json:"risk_hits"`
	Invocations      []bfcore.BrainInvocation `json:"invocations"`
	RefreshedAt      string                   `json:"refreshed_at"`
}

// TDDState conserva la evidencia Red vigente en session.json.
type TDDState struct {
	Red *RedEvidence `json:"red"`
}

// RedEvidence registra la prueba fallida determinista capturada por tdd-red.
type RedEvidence struct {
	ChunkID               string `json:"chunk_id"`
	GengisAttempt         int    `json:"gengis_attempt"`
	BounceCycle           int    `json:"bounce_cycle"`
	InheritedFromCycle    int    `json:"inherited_from_cycle"`
	CmdIndex              int    `json:"cmd_index"`
	Command               string `json:"command"`
	ExitCode              int    `json:"exit_code"`
	DurationMS            int64  `json:"duration_ms"`
	OutputExcerpt         string `json:"output_excerpt"`
	Truncated             bool   `json:"truncated"`
	RedWorktreeTreeSHA256 string `json:"red_worktree_tree_sha256"`
	HasTestFile           bool   `json:"has_test_file"`
	RecordedAt            string `json:"recorded_at"`
}

// CommandEvidence registra la ejecucion exitosa de un VerifyCommand en fase Green.
type CommandEvidence struct {
	CmdIndex      int    `json:"cmd_index"`
	Command       string `json:"command"`
	ExitCode      int    `json:"exit_code"`
	DurationMS    int64  `json:"duration_ms"`
	OutputExcerpt string `json:"output_excerpt"`
	Truncated     bool   `json:"truncated"`
}

// ChangedFileEntry describe un archivo modificado, añadido, borrado o renombrado con hash crudo y modo octal.
type ChangedFileEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

// GitBundleInfo agrupa el contexto Git verificable de DeliveryBundle sin rutas absolutas de host.
type GitBundleInfo struct {
	TargetBranch       string             `json:"target_branch"`
	BuildBranch        string             `json:"build_branch"`
	BaseRef            string             `json:"base_ref"`
	WorktreeTreeSHA256 string             `json:"worktree_tree_sha256"`
	ChangedFiles       []ChangedFileEntry `json:"changed_files"`
	DiffRelPath        string             `json:"diff_rel_path"`
	DiffSHA256         string             `json:"diff_sha256"`
	DiffBytes          int64              `json:"diff_bytes"`
}

// ScopeInfo refleja el alcance efectivo aplicable al chunk activo.
type ScopeInfo struct {
	CanModify []string `json:"can_modify"`
	ReadOnly  []string `json:"read_only"`
	Forbidden []string `json:"forbidden"`
}

// BundleBrainInfo proyecta la evidencia de 0_Brain dentro de DeliveryBundle.
type BundleBrainInfo struct {
	BrainDBMTime     string            `json:"brain_db_mtime"`
	RiskZonesSHA256  string            `json:"risk_zones_sha256"`
	HighestRiskLevel string            `json:"highest_risk_level"`
	MatchedDocs      []BrainDocSummary `json:"matched_docs"`
	RiskHits         []bfcore.RiskHit  `json:"risk_hits"`
}

// TDDEvidence agrupa la evidencia Red y Green dentro de DeliveryBundle.
type TDDEvidence struct {
	Red   RedEvidence       `json:"red"`
	Green []CommandEvidence `json:"green"`
}

// AxisApplicability describe la aplicabilidad determinista de un lente de 4_Revisores.
type AxisApplicability struct {
	Applicable  bool   `json:"applicable"`
	Mode        string `json:"mode"`
	Reason      string `json:"reason"`
	ProfileRole string `json:"profile_role"`
}

// ReviewerApplicability agrupa la aplicabilidad de los cuatro ejes de 4_Revisores.
type ReviewerApplicability struct {
	Security         AxisApplicability `json:"security"`
	CodeArchitecture AxisApplicability `json:"code_architecture"`
	UX               AxisApplicability `json:"ux"`
	UI               AxisApplicability `json:"ui"`
}

// DeliveryBundle es el manifiesto inmutable v1.0 emitido por package hacia 4_Revisores.
type DeliveryBundle struct {
	SchemaVersion         string                `json:"schema_version"`
	BundleID              string                `json:"bundle_id"`
	CreatedAt             string                `json:"created_at"`
	TraceID               string                `json:"trace_id"`
	WorkItemID            string                `json:"work_item_id"`
	SpecSHA256            string                `json:"spec_sha256"`
	Lane                  string                `json:"lane"`
	TaskID                string                `json:"task_id"`
	ChunkID               string                `json:"chunk_id"`
	GengisAttempt         int                   `json:"gengis_attempt"`
	ResetEpoch            int                   `json:"reset_epoch"`
	BounceCycle           int                   `json:"bounce_cycle"`
	Git                   GitBundleInfo         `json:"git"`
	Scope                 ScopeInfo             `json:"scope"`
	BrainContext          BundleBrainInfo       `json:"brain_context"`
	TDDEvidence           TDDEvidence           `json:"tdd_evidence"`
	ReviewerApplicability ReviewerApplicability `json:"reviewer_applicability"`
	PreviousBounces       []string              `json:"previous_bounces"`
}

// AxisVerdict modela el dictamen individual de un revisor de 4_Revisores dentro de BounceOrder.
type AxisVerdict struct {
	Status        string `json:"status"`
	FindingsCount int    `json:"findings_count"`
	Summary       string `json:"summary"`
	NAReason      string `json:"na_reason,omitempty"`
	ModelID       string `json:"model_id,omitempty"`
}

// ReviewerVerdicts agrupa los cuatro dictamenes de 4_Revisores.
type ReviewerVerdicts struct {
	Security         AxisVerdict `json:"security"`
	CodeArchitecture AxisVerdict `json:"code_architecture"`
	UX               AxisVerdict `json:"ux"`
	UI               AxisVerdict `json:"ui"`
}

// RemediationItem describe un hallazgo accionable emitido en una orden BOUNCE.
type RemediationItem struct {
	ID             string `json:"id"`
	Axis           string `json:"axis"`
	Severity       string `json:"severity"`
	File           string `json:"file"`
	Line           int    `json:"line"`
	Description    string `json:"description"`
	RequiredAction string `json:"required_action"`
}

// BounceOrder modela la orden consolidada v1.0 emitida por 5_Cycle_Approver.
type BounceOrder struct {
	SchemaVersion    string            `json:"schema_version"`
	OrderID          string            `json:"order_id"`
	CreatedAt        string            `json:"created_at"`
	TraceID          string            `json:"trace_id"`
	TaskID           string            `json:"task_id"`
	ChunkID          string            `json:"chunk_id"`
	BundleID         string            `json:"bundle_id"`
	BundleSHA256     string            `json:"bundle_sha256"`
	Decision         string            `json:"decision"`
	ReviewerVerdicts ReviewerVerdicts  `json:"reviewer_verdicts"`
	RequiresRedTest  bool              `json:"requires_red_test"`
	RemediationItems []RemediationItem `json:"remediation_items"`
	Summary          string            `json:"summary"`
}

// BuilderEvent modela una linea JSONL en runtime/builder_events.jsonl.
type BuilderEvent struct {
	Timestamp    string `json:"timestamp"`
	Event        string `json:"event"`
	TraceID      string `json:"trace_id"`
	TaskID       string `json:"task_id,omitempty"`
	ChunkID      string `json:"chunk_id,omitempty"`
	Attempt      int    `json:"attempt,omitempty"`
	BounceCycle  int    `json:"bounce_cycle,omitempty"`
	ExitCode     int    `json:"exit_code"`
	Reason       string `json:"reason"`
	Retryable    bool   `json:"retryable"`
	BundleSHA256 string `json:"bundle_sha256,omitempty"`
	DurationMS   int64  `json:"duration_ms"`
	Detail       string `json:"detail,omitempty"`
}

// ValidationIssue identifica un error estructurado con JSON Pointer (RFC 6901).
type ValidationIssue struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CLIResult es el sobre JSON unificado para init, tdd-red, package, apply-order, commit y cleanup.
type CLIResult struct {
	OK               bool              `json:"ok"`
	ExitCode         int               `json:"exit_code"`
	Reason           string            `json:"reason"`
	Retryable        bool              `json:"retryable"`
	TraceID          string            `json:"trace_id,omitempty"`
	TaskID           string            `json:"task_id,omitempty"`
	ChunkID          string            `json:"chunk_id,omitempty"`
	Attempt          int               `json:"attempt,omitempty"`
	BounceCycle      int               `json:"bounce_cycle,omitempty"`
	BuildBranch      string            `json:"build_branch,omitempty"`
	WorktreePath     string            `json:"worktree_path,omitempty"`
	BundleID         string            `json:"bundle_id,omitempty"`
	BundleSHA256     string            `json:"bundle_sha256,omitempty"`
	BundlePath       string            `json:"bundle_path,omitempty"`
	DiffPath         string            `json:"diff_path,omitempty"`
	Decision         string            `json:"decision,omitempty"`
	CommitRef        string            `json:"commit_ref,omitempty"`
	NextAutorCommand string            `json:"next_autor_command,omitempty"`
	Reset            bool              `json:"reset,omitempty"`
	Issues           []ValidationIssue `json:"issues,omitempty"`
}

// ContextResult es el payload estructurado emitido por builder context.
type ContextResult struct {
	OK                 bool                 `json:"ok"`
	ExitCode           int                  `json:"exit_code"`
	Reason             string               `json:"reason"`
	Retryable          bool                 `json:"retryable"`
	TraceID            string               `json:"trace_id"`
	TaskID             string               `json:"task_id"`
	WorktreePath       string               `json:"worktree_path"`
	BuildBranch        string               `json:"build_branch"`
	TargetBranch       string               `json:"target_branch"`
	PlanApproved       bool                 `json:"plan_approved"`
	ChunkID            string               `json:"chunk_id"`
	Attempt            int                  `json:"attempt"`
	BounceCycle        int                  `json:"bounce_cycle"`
	BaseRef            string               `json:"base_ref"`
	Spec               string               `json:"spec"`
	Acceptance         []string             `json:"acceptance"`
	VerifyCommands     []string             `json:"verify_commands"`
	FailureProof       string               `json:"failure_proof"`
	EffectiveCanModify []string             `json:"effective_can_modify"`
	EffectiveReadOnly  []string             `json:"effective_read_only"`
	EffectiveForbidden []string             `json:"effective_forbidden"`
	GengisFeedback     []string             `json:"gengis_feedback"`
	BrainContext       BrainContextSnapshot `json:"brain_context"`
	ActiveBounce       *BounceOrder         `json:"active_bounce,omitempty"`
}

// VerifyReport es el sobre JSON estructurado para builder verify (100% solo lectura).
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

// InitOptions parametriza builder init.
type InitOptions struct {
	TaskDir      string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	GengisRoot   string
	VaultRoot    string
	BrainBin     string
	Project      string
	Branch       string
	BaseRef      string
	BrainClient  bfcore.BrainClient
	Git          GitClient
}

// ContextOptions parametriza builder context.
type ContextOptions struct {
	TaskDir      string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	VaultRoot    string
	BrainBin     string
	RefreshBrain bool
	BrainClient  bfcore.BrainClient
	Git          GitClient
}

// TDDRedOptions parametriza builder tdd-red.
type TDDRedOptions struct {
	TaskDir      string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	CmdIndex     int
	TimeoutSec   int
	Git          GitClient
	Runner       CommandRunner
}

// PackageOptions parametriza builder package.
type PackageOptions struct {
	TaskDir      string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	VaultRoot    string
	BrainBin     string
	IncludeUI    bool
	IncludeUX    bool
	TimeoutSec   int
	BrainClient  bfcore.BrainClient
	Git          GitClient
	Runner       CommandRunner
}

// ApplyOrderOptions parametriza builder apply-order.
type ApplyOrderOptions struct {
	TaskDir      string
	OrderPath    string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	ResetBounces bool
	Reason       string
	Git          GitClient
}

// CommitOptions parametriza builder commit.
type CommitOptions struct {
	TaskDir      string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	GengisRoot   string
	GengisBin    string
	VaultRoot    string
	BrainBin     string
	TimeoutSec   int
	BrainClient  bfcore.BrainClient
	Git          GitClient
}

// CleanupOptions parametriza builder cleanup.
type CleanupOptions struct {
	TaskDir      string
	RepoRoot     string
	FactoryRoot  string
	DataDir      string
	WorktreeRoot string
	Force        bool
	DeleteBranch bool
	Git          GitClient
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

// NormalizeBundleForCanonicalCopy devuelve una copia determinista de DeliveryBundle
// donde todo slice nil se normaliza a [].
func NormalizeBundleForCanonicalCopy(b DeliveryBundle) DeliveryBundle {
	out := b
	if out.Git.ChangedFiles == nil {
		out.Git.ChangedFiles = []ChangedFileEntry{}
	}
	if out.Scope.CanModify == nil {
		out.Scope.CanModify = []string{}
	}
	if out.Scope.ReadOnly == nil {
		out.Scope.ReadOnly = []string{}
	}
	if out.Scope.Forbidden == nil {
		out.Scope.Forbidden = []string{}
	}
	if out.BrainContext.MatchedDocs == nil {
		out.BrainContext.MatchedDocs = []BrainDocSummary{}
	}
	if out.BrainContext.RiskHits == nil {
		out.BrainContext.RiskHits = []bfcore.RiskHit{}
	} else {
		hits := make([]bfcore.RiskHit, len(out.BrainContext.RiskHits))
		for i, h := range out.BrainContext.RiskHits {
			if h.MatchedPaths == nil {
				h.MatchedPaths = []string{}
			}
			hits[i] = h
		}
		out.BrainContext.RiskHits = hits
	}
	if out.TDDEvidence.Green == nil {
		out.TDDEvidence.Green = []CommandEvidence{}
	}
	if out.PreviousBounces == nil {
		out.PreviousBounces = []string{}
	}
	return out
}

// CanonicalBundleSHA256 calcula el SHA-256 hex en minusculas sobre la serializacion JSON compacta
// y canonizada de DeliveryBundle.
func CanonicalBundleSHA256(b DeliveryBundle) (string, error) {
	norm := NormalizeBundleForCanonicalCopy(b)
	raw, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func failResult(code int, reason, traceID, taskID string, issues []ValidationIssue) (CLIResult, int) {
	return CLIResult{
		OK:        false,
		ExitCode:  code,
		Reason:    reason,
		Retryable: IsRetryableReason(code, reason),
		TraceID:   traceID,
		TaskID:    taskID,
		Issues:    issues,
	}, code
}
