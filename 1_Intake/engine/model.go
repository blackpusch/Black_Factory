package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

const (
	SchemaVersion           = "1.0"
	TriageDetRulesetVersion = "triage-det-v1.0"
	MaxInputBytes           = 1 << 20 // 1 MiB
	DefaultMaxFTDFiles      = 5
)

var (
	reWorkItemID = regexp.MustCompile(`^WI-[0-9a-f]{12}$`)
	reHex64      = regexp.MustCompile(`^[0-9a-f]{64}$`)

	validKinds = map[string]bool{
		"bug":       true,
		"feature":   true,
		"refactor":  true,
		"audit_fix": true,
		"ops":       true,
	}

	validImpactFlags = map[string]bool{
		"contract":   true,
		"schema":     true,
		"db":         true,
		"auth":       true,
		"security":   true,
		"governance": true,
	}

	validTemplates = map[string]bool{
		"modulo_nuevo":         true,
		"correccion_hallazgos": true,
		"auditoria_e2e":        true,
		"mapeo_estatico":       true,
	}

	riskLevelRank = map[string]int{
		"NONE":    0,
		"BAJO":    1,
		"MEDIO":   2,
		"ALTO":    3,
		"CRITICO": 4,
	}
)

// RawIntake representa los campos de entrada crudos decodificados desde JSON y/o banderas CLI.
type RawIntake struct {
	Project       string   `json:"project"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Note          string   `json:"note"`
	Kind          string   `json:"kind"`
	Source        string   `json:"source"`
	SourceRef     string   `json:"source_ref"`
	AffectedPaths []string `json:"affected_paths"`
	ImpactFlags   []string `json:"impact_flags"`
}

// G4Redaction reexporta bfcore.G4Redaction para el paquete intake.
type G4Redaction = bfcore.G4Redaction

// SanitizationReport documenta las redacciones G4 Tier 2 y el hash SHA-256 canonico del payload sanitizado.
type SanitizationReport struct {
	StrictG4               bool          `json:"strict_g4"`
	SanitizedPayloadSHA256 string        `json:"sanitized_payload_sha256"`
	G4Redactions           []G4Redaction `json:"g4_redactions"`
}

// RiskHit representa una zona de riesgo de wiki/risk_zones.md que coincidio con affected_paths.
type RiskHit struct {
	ID            string   `json:"id"`
	Project       string   `json:"project"`
	Level         string   `json:"level"`
	MatchedPaths  []string `json:"matched_paths"`
	MandatoryGate string   `json:"mandatory_gate"`
}

// TriageReason describe una regla determinista disparada durante la asignacion de carril.
type TriageReason struct {
	Code           string `json:"code"`
	RuleKind       string `json:"rule_kind"`
	RulesetVersion string `json:"ruleset_version"`
	Detail         string `json:"detail"`
}

// BrainInvocation registra la evidencia literal G1 de una consulta a 0_Brain/brain.exe.
type BrainInvocation struct {
	Command     string `json:"command"`
	ExitCode    int    `json:"exit_code"`
	DurationMs  int64  `json:"duration_ms"`
	ResultCount int    `json:"result_count"`
}

// BrainDocMatch resume un documento OKF coincidente devuelto por 0_Brain.
type BrainDocMatch struct {
	ID                  string    `json:"id"`
	RelPath             string    `json:"rel_path"`
	Type                string    `json:"type"`
	Status              string    `json:"status"`
	Project             string    `json:"project,omitempty"`
	VerifiedHead        string    `json:"verified_head"`
	Transversal         bool      `json:"transversal"`
	CoversPaths         []string  `json:"covers_paths,omitempty"`
	MatchedPaths        []string  `json:"matched_paths,omitempty"`
	RiskHits            []RiskHit `json:"risk_hits,omitempty"`
	RiskZonesSHA256     string    `json:"risk_zones_sha256,omitempty"`
	RiskZonesParseError string    `json:"risk_zones_parse_error,omitempty"`
}

// BrainEvidence agrupa toda la evidencia de consulta Read-Before-Act contra 0_Brain.
type BrainEvidence struct {
	VaultPath            string            `json:"vault_path"`
	BrainDBMTime         string            `json:"brain_db_mtime"`
	RiskZonesSHA256      string            `json:"risk_zones_sha256"`
	ExtractedQueryTokens []string          `json:"extracted_query_tokens"`
	Invocations          []BrainInvocation `json:"invocations"`
	MatchedDocs          []BrainDocMatch   `json:"matched_docs"`
}

// QuarantineInfo detalla las causas de retencion G6 cuando status == "needs_clarification".
type QuarantineInfo struct {
	ReasonCodes       []string `json:"reason_codes"`
	MissingFields     []string `json:"missing_fields"`
	InvalidPathReason string   `json:"invalid_path_reason"`
	Detail            string   `json:"detail"`
	ResolveHint       string   `json:"resolve_hint"`
}

// WorkItem es el artefacto canonico versionado (schema_version: "1.0") persistido en workitems/ o quarantine/.
type WorkItem struct {
	SchemaVersion     string             `json:"schema_version"`
	ID                string             `json:"id"`
	DedupeKey         string             `json:"dedupe_key"`
	TraceID           string             `json:"trace_id"`
	Revision          int                `json:"revision"`
	Status            string             `json:"status"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
	Project           string             `json:"project"`
	Title             string             `json:"title"`
	Description       string             `json:"description"`
	Note              string             `json:"note"`
	Kind              string             `json:"kind"`
	Source            string             `json:"source"`
	SourceRef         string             `json:"source_ref"`
	AffectedPaths     []string           `json:"affected_paths"`
	PathsConfidence   string             `json:"paths_confidence"`
	ImpactFlags       []string           `json:"impact_flags"`
	Lane              string             `json:"lane"`
	SuggestedTemplate string             `json:"suggested_template"`
	RiskLevel         string             `json:"risk_level"`
	RequiresAutorHITL bool               `json:"requires_autor_hitl"`
	RiskHits          []RiskHit          `json:"risk_hits"`
	TriageReasons     []TriageReason     `json:"triage_reasons"`
	Sanitization      SanitizationReport `json:"sanitization"`
	BrainEvidence     BrainEvidence      `json:"brain_evidence"`
	QuarantineInfo    *QuarantineInfo    `json:"quarantine_info"`
}

// IntakeEvent representa una linea de telemetria estructurada en runtime/intake_events.jsonl.
type IntakeEvent struct {
	Timestamp         string   `json:"timestamp"`
	TraceID           string   `json:"trace_id"`
	Event             string   `json:"event"`
	ID                string   `json:"id,omitempty"`
	PrevID            string   `json:"prev_id,omitempty"`
	Revision          int      `json:"revision,omitempty"`
	Project           string   `json:"project,omitempty"`
	Source            string   `json:"source,omitempty"`
	SourceRef         string   `json:"source_ref,omitempty"`
	Status            string   `json:"status,omitempty"`
	Lane              string   `json:"lane,omitempty"`
	RiskLevel         string   `json:"risk_level,omitempty"`
	ReasonCodes       []string `json:"reason_codes,omitempty"`
	DurationMs        int64    `json:"duration_ms,omitempty"`
	RuleName          string   `json:"rule_name,omitempty"`
	Field             string   `json:"field,omitempty"`
	Line              int      `json:"line,omitempty"`
	RawSHA256         string   `json:"raw_sha256,omitempty"`
	OK                *bool    `json:"ok,omitempty"`
	WorkItemsChecked  int      `json:"workitems_checked,omitempty"`
	QuarantineChecked int      `json:"quarantine_checked,omitempty"`
	EventsChecked     int      `json:"events_checked,omitempty"`
}

// VerifyReport resume el resultado de intake.exe verify.
type VerifyReport struct {
	OK                bool     `json:"ok"`
	RepoRoot          string   `json:"repo_root"`
	DataDir           string   `json:"data_dir"`
	VaultPath         string   `json:"vault_path"`
	GitIgnored        bool     `json:"git_ignored"`
	BrainOK           bool     `json:"brain_ok"`
	WorkItemsChecked  int      `json:"workitems_checked"`
	QuarantineChecked int      `json:"quarantine_checked"`
	EventsChecked     int      `json:"events_checked"`
	Errors            []string `json:"errors"`
}

type canonicalPayload struct {
	Project       string   `json:"project"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Note          string   `json:"note"`
	Kind          string   `json:"kind"`
	Source        string   `json:"source"`
	SourceRef     string   `json:"source_ref"`
	AffectedPaths []string `json:"affected_paths"`
	ImpactFlags   []string `json:"impact_flags"`
}

// ComputeSanitizedSHA256 calcula el digest SHA-256 hex en minusculas de la serializacion JSON compacta
// del payload sanitizado con orden de campos fijo y slices inicializados a [] no nulos.
func ComputeSanitizedSHA256(in RawIntake, cleanPaths, sortedFlags []string) string {
	pathsCopy := make([]string, 0, len(cleanPaths))
	pathsCopy = append(pathsCopy, cleanPaths...)

	flagsCopy := make([]string, 0, len(sortedFlags))
	flagsCopy = append(flagsCopy, sortedFlags...)
	sort.Strings(flagsCopy)

	cp := canonicalPayload{
		Project:       strings.ToLower(strings.TrimSpace(in.Project)),
		Title:         strings.TrimSpace(in.Title),
		Description:   strings.TrimSpace(in.Description),
		Note:          strings.TrimSpace(in.Note),
		Kind:          strings.ToLower(strings.TrimSpace(in.Kind)),
		Source:        strings.ToLower(strings.TrimSpace(in.Source)),
		SourceRef:     strings.TrimSpace(in.SourceRef),
		AffectedPaths: pathsCopy,
		ImpactFlags:   flagsCopy,
	}
	b, _ := json.Marshal(cp)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ComputeDedupeKey calcula la clave determinista de deduplicacion por prefijo de longitud
// sha256_hex(len(p)+":"+p+"|"+len(s)+":"+s+"|"+len(r)+":"+r) cuando project, source y source_ref
// son no vacios; si falta alguno, devuelve "".
func ComputeDedupeKey(project, source, sourceRef string) string {
	p := strings.ToLower(strings.TrimSpace(project))
	s := strings.ToLower(strings.TrimSpace(source))
	r := strings.ToLower(strings.TrimSpace(sourceRef))
	if p == "" || s == "" || r == "" {
		return ""
	}
	raw := fmt.Sprintf("%d:%s|%d:%s|%d:%s", len(p), p, len(s), s, len(r), r)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// FormatWorkItemID construye el identificador unificado WI-<hash12>:
// usa dedupeKey[:12] cuando dedupeKey != "", o payloadSHA256[:12] cuando falta identidad en cuarentena.
func FormatWorkItemID(dedupeKey, payloadSHA256 string) string {
	if len(dedupeKey) >= 12 {
		return "WI-" + dedupeKey[:12]
	}
	if len(payloadSHA256) >= 12 {
		return "WI-" + payloadSHA256[:12]
	}
	sum := sha256.Sum256([]byte(dedupeKey + "|" + payloadSHA256))
	return "WI-" + hex.EncodeToString(sum[:])[:12]
}

// FormatTraceID genera un identificador de traza determinista en formato trc-<UTC>-<hash8>.
func FormatTraceID(now time.Time, hashHex string) string {
	prefix := hashHex
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	if prefix == "" {
		prefix = "00000000"
	}
	return fmt.Sprintf("trc-%s-%s", now.UTC().Format("20060102T150405Z"), prefix)
}

// IsValidKind comprueba si kind pertenece al conjunto cerrado {bug, feature, refactor, audit_fix, ops}.
func IsValidKind(k string) bool {
	return validKinds[strings.ToLower(strings.TrimSpace(k))]
}

// NormalizeAndValidateImpactFlags normaliza, deduplica y ordena impact_flags validando el enum cerrado.
func NormalizeAndValidateImpactFlags(flags []string) ([]string, bool) {
	if len(flags) == 0 {
		return []string{}, true
	}
	seen := make(map[string]bool, len(flags))
	out := make([]string, 0, len(flags))
	for _, raw := range flags {
		for _, part := range strings.Split(raw, ",") {
			f := strings.ToLower(strings.TrimSpace(part))
			if f == "" {
				continue
			}
			if !validImpactFlags[f] {
				return nil, false
			}
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	sort.Strings(out)
	return out, true
}

// RiskLevelRank devuelve la severidad numerica (0..4) de un nivel de riesgo.
func RiskLevelRank(level string) int {
	if r, ok := riskLevelRank[strings.ToUpper(strings.TrimSpace(level))]; ok {
		return r
	}
	return 0
}

// ValidateWorkItem verifica todas las invariantes del esquema WorkItem v1.0 y ejecuta bfcore.ScanG4All
// sobre el JSON serializado. Devuelve la lista de errores encontrados (vacia si es valido).
func ValidateWorkItem(wi *WorkItem) []string {
	if wi == nil {
		return []string{"WorkItem es nil"}
	}
	var errs []string

	if wi.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Sprintf("schema_version invalido %q (esperaba %q)", wi.SchemaVersion, SchemaVersion))
	}
	if !reWorkItemID.MatchString(wi.ID) {
		errs = append(errs, fmt.Sprintf("id invalido %q (debe cumplir ^WI-[0-9a-f]{12}$)", wi.ID))
	}
	if wi.Revision < 1 {
		errs = append(errs, fmt.Sprintf("revision invalida %d (debe ser >= 1)", wi.Revision))
	}
	if strings.TrimSpace(wi.TraceID) == "" {
		errs = append(errs, "trace_id vacio")
	}
	if _, err := time.Parse(time.RFC3339, wi.CreatedAt); err != nil {
		errs = append(errs, fmt.Sprintf("created_at no es RFC3339 valido: %q", wi.CreatedAt))
	}
	if _, err := time.Parse(time.RFC3339, wi.UpdatedAt); err != nil {
		errs = append(errs, fmt.Sprintf("updated_at no es RFC3339 valido: %q", wi.UpdatedAt))
	}
	if !reHex64.MatchString(wi.Sanitization.SanitizedPayloadSHA256) {
		errs = append(errs, "sanitization.sanitized_payload_sha256 debe ser hex de 64 caracteres")
	}
	if _, ok := riskLevelRank[wi.RiskLevel]; !ok {
		errs = append(errs, fmt.Sprintf("risk_level invalido %q", wi.RiskLevel))
	}
	if wi.PathsConfidence != "explicit" && wi.PathsConfidence != "none" {
		errs = append(errs, fmt.Sprintf("paths_confidence invalido %q", wi.PathsConfidence))
	}
	for _, fl := range wi.ImpactFlags {
		if !validImpactFlags[fl] {
			errs = append(errs, fmt.Sprintf("impact_flag invalido %q", fl))
		}
	}

	switch wi.Status {
	case "ready":
		if !reHex64.MatchString(wi.DedupeKey) {
			errs = append(errs, "status=ready exige dedupe_key de 64 hex")
		} else if expectedID := "WI-" + wi.DedupeKey[:12]; wi.ID != expectedID {
			errs = append(errs, fmt.Sprintf("id %q no coincide con dedupe_key[:12] (%q)", wi.ID, expectedID))
		}
		if !IsValidKind(wi.Kind) {
			errs = append(errs, fmt.Sprintf("status=ready exige kind valido, obtuvo %q", wi.Kind))
		}
		if wi.Lane != "ASDD" && wi.Lane != "FTD" {
			errs = append(errs, fmt.Sprintf("status=ready exige lane ASDD o FTD, obtuvo %q", wi.Lane))
		}
		if !validTemplates[wi.SuggestedTemplate] {
			errs = append(errs, fmt.Sprintf("status=ready exige suggested_template valido, obtuvo %q", wi.SuggestedTemplate))
		}
		if wi.PathsConfidence != "explicit" || len(wi.AffectedPaths) == 0 {
			errs = append(errs, "status=ready exige paths_confidence=explicit y affected_paths no vacio")
		}
		if wi.QuarantineInfo != nil {
			errs = append(errs, "status=ready exige quarantine_info == null")
		}
		if len(wi.TriageReasons) == 0 {
			errs = append(errs, "status=ready exige al menos un triage_reason")
		}
	case "needs_clarification":
		if wi.DedupeKey != "" {
			if !reHex64.MatchString(wi.DedupeKey) {
				errs = append(errs, "dedupe_key en cuarentena debe ser vacio o 64 hex")
			} else if expectedID := "WI-" + wi.DedupeKey[:12]; wi.ID != expectedID {
				errs = append(errs, fmt.Sprintf("id %q no coincide con dedupe_key[:12] (%q)", wi.ID, expectedID))
			}
		} else if len(wi.Sanitization.SanitizedPayloadSHA256) >= 12 {
			if expectedID := "WI-" + wi.Sanitization.SanitizedPayloadSHA256[:12]; wi.ID != expectedID {
				errs = append(errs, fmt.Sprintf("id %q no coincide con sanitized_payload_sha256[:12] (%q)", wi.ID, expectedID))
			}
		}
		if wi.Lane != "" {
			errs = append(errs, fmt.Sprintf("status=needs_clarification exige lane vacio, obtuvo %q", wi.Lane))
		}
		if wi.SuggestedTemplate != "" {
			errs = append(errs, fmt.Sprintf("status=needs_clarification exige suggested_template vacio, obtuvo %q", wi.SuggestedTemplate))
		}
		if wi.QuarantineInfo == nil || len(wi.QuarantineInfo.ReasonCodes) == 0 || strings.TrimSpace(wi.QuarantineInfo.ResolveHint) == "" {
			errs = append(errs, "status=needs_clarification exige quarantine_info con reason_codes y resolve_hint")
		}
	default:
		errs = append(errs, fmt.Sprintf("status desconocido %q", wi.Status))
	}

	rawJSON, err := json.Marshal(wi)
	if err != nil {
		errs = append(errs, fmt.Sprintf("error serializando WorkItem: %v", err))
		return errs
	}
	if findings := bfcore.ScanG4All(string(rawJSON)); len(findings) > 0 {
		errs = append(errs, fmt.Sprintf("violacion G4 detectada en WorkItem serializado: regla %s (linea %d)", findings[0].Rule, findings[0].Line))
	}
	return errs
}
