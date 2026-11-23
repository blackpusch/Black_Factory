package intake

import (
	"fmt"
	"strings"

	"blackfactory/pkg/bfcore"
)

// EvaluateMetadataAndScope evalua las Reglas de Prioridad 3, 4 y 5 (compuerta de metadatos y localizabilidad):
//   - Regla 3 (MISSING_REQUIRED_FIELDS): falta cualquiera de project, title, description, kind, source, source_ref;
//     kind fuera de {bug, feature, refactor, audit_fix, ops}; impact_flags fuera del enum cerrado; o PII Tier 2 en metadatos.
//   - Regla 4 (INVALID_AFFECTED_PATH_G4): SanitizeAffectedPaths detecto ruta personal G4, traversal, directorio o glob.
//   - Regla 5 (UNCERTAIN_SCOPE): len(cleanPaths) == 0.
//
// Si alguna se dispara, construye el QuarantineInfo con un comando resolve_hint accionable; si todas pasan, devuelve nil.
func EvaluateMetadataAndScope(wiID string, raw RawIntake, cleanPaths []string, invalidPathReason string, metadataHasTier2, flagsValid bool) *QuarantineInfo {
	var reasonCodes []string
	missingFields := make([]string, 0)
	var details []string

	reqCheck := []struct {
		name string
		val  string
	}{
		{"project", raw.Project},
		{"title", raw.Title},
		{"description", raw.Description},
		{"kind", raw.Kind},
		{"source", raw.Source},
		{"source_ref", raw.SourceRef},
	}
	for _, req := range reqCheck {
		if strings.TrimSpace(req.val) == "" {
			missingFields = append(missingFields, req.name)
		}
	}
	if strings.TrimSpace(raw.Kind) != "" && !IsValidKind(raw.Kind) {
		missingFields = append(missingFields, "kind")
		details = append(details, fmt.Sprintf("kind %q fuera del enum permitido {bug, feature, refactor, audit_fix, ops}", raw.Kind))
	}
	if !flagsValid {
		missingFields = append(missingFields, "impact_flags")
		details = append(details, "impact_flags contiene valores fuera del enum permitido {contract, schema, db, auth, security, governance}")
	}
	if metadataHasTier2 {
		details = append(details, "se detecto y redacto PII Tier 2 dentro de campos de metadatos")
	}

	if len(missingFields) > 0 || metadataHasTier2 {
		reasonCodes = append(reasonCodes, "MISSING_REQUIRED_FIELDS")
		if len(missingFields) > 0 {
			details = append(details, fmt.Sprintf("campos obligatorios faltantes o invalidos: %s", strings.Join(missingFields, ", ")))
		}
	}

	if invalidPathReason != "" {
		reasonCodes = append(reasonCodes, "INVALID_AFFECTED_PATH_G4")
		details = append(details, fmt.Sprintf("affected_paths contenia una ruta invalida o personal (%s) y fue descartado en memoria", invalidPathReason))
	}

	if len(cleanPaths) == 0 {
		reasonCodes = append(reasonCodes, "UNCERTAIN_SCOPE")
		details = append(details, "affected_paths esta vacio; se requiere al menos un archivo concreto relativo al repositorio")
	}

	if len(reasonCodes) == 0 {
		return nil
	}

	return &QuarantineInfo{
		ReasonCodes:       reasonCodes,
		MissingFields:     missingFields,
		InvalidPathReason: invalidPathReason,
		Detail:            strings.Join(details, "; "),
		ResolveHint:       buildResolveHint(wiID, missingFields, len(cleanPaths) == 0),
	}
}

// EvaluateProjectRegistration evalua la Regla de Prioridad 6 (UNREGISTERED_PROJECT) cuando el proyecto
// no posee un documento activo (type: "project", status: "active") en 0_Brain.
func EvaluateProjectRegistration(wiID string, raw RawIntake, projectActive bool) *QuarantineInfo {
	if projectActive {
		return nil
	}
	return &QuarantineInfo{
		ReasonCodes:       []string{"UNREGISTERED_PROJECT"},
		MissingFields:     []string{},
		InvalidPathReason: "",
		Detail:            fmt.Sprintf("el proyecto %q no figura registrado como activo (type: project, status: active) en 0_Brain", raw.Project),
		ResolveHint:       fmt.Sprintf("1_Intake/intake.exe resolve --id %s --project black-factory --json", wiID),
	}
}

func buildResolveHint(wiID string, missingFields []string, needsPath bool) string {
	var parts []string
	parts = append(parts, "1_Intake/intake.exe", "resolve", "--id", wiID)
	for _, mf := range missingFields {
		switch mf {
		case "project":
			parts = append(parts, "--project", "<PROJECT_SLUG>")
		case "title":
			parts = append(parts, "--title", "<TITLE>")
		case "description":
			parts = append(parts, "--description", "<DESCRIPTION>")
		case "kind":
			parts = append(parts, "--kind", "<bug|feature|refactor|audit_fix|ops>")
		case "source":
			parts = append(parts, "--source", "<SOURCE>")
		case "source_ref":
			parts = append(parts, "--source-ref", "<REF>")
		case "impact_flags":
			parts = append(parts, "--impact-flag", "<contract|schema|db|auth|security|governance>")
		}
	}
	if needsPath {
		parts = append(parts, "--path", "<repo/relative/file.go>")
	}
	parts = append(parts, "--json")
	return strings.Join(parts, " ")
}

// TriageWorkItem evalua las Reglas de Prioridad 7 a 13 de forma 100% determinista sobre un WorkItem
// cuyos metadatos, proyecto y affected_paths ya superaron las compuertas 1..6.
func TriageWorkItem(wi *WorkItem, contextResp BrainContextResponse, maxFTDFiles int) {
	if maxFTDFiles <= 0 {
		maxFTDFiles = DefaultMaxFTDFiles
	}

	// 1. Agregar risk_hits aplicables al proyecto del WorkItem (o transversales "*" / "black-factory")
	filteredHits := make([]RiskHit, 0, len(contextResp.RiskHits))
	maxRank := 0
	maxLevel := "NONE"

	for _, hit := range contextResp.RiskHits {
		hp := strings.TrimSpace(hit.Project)
		if hp != "" && hp != "*" && !strings.EqualFold(hp, "black-factory") && !strings.EqualFold(hp, wi.Project) {
			continue
		}
		filteredHits = append(filteredHits, hit)
		if r := RiskLevelRank(hit.Level); r > maxRank {
			maxRank = r
			maxLevel = strings.ToUpper(strings.TrimSpace(hit.Level))
		}
	}
	wi.RiskHits = filteredHits
	wi.RiskLevel = maxLevel
	wi.RequiresAutorHITL = false

	var reasons []TriageReason
	structuralOrRiskTrigger := false
	noBrainCoverageFired := false

	// Regla 7: RISK_ZONE_CRITICO
	if maxLevel == "CRITICO" {
		wi.RequiresAutorHITL = true
		structuralOrRiskTrigger = true
		reasons = append(reasons, TriageReason{
			Code:           "RISK_ZONE_CRITICO",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         "affected_paths intersecta una zona de riesgo CRITICO en wiki/risk_zones.md; exige carril ASDD y compuerta [AUTOR] HITL",
		})
	}

	// Regla 8: RISK_ZONES_UNPARSEABLE_FAILSAFE
	if contextResp.RiskZonesParseError != "" || !contextResp.HasActiveRiskZones {
		wi.RequiresAutorHITL = true
		structuralOrRiskTrigger = true
		detail := "no se encontro documento activo type: risk_zones en 0_Brain; fail-safe a ASDD con compuerta [AUTOR]"
		if contextResp.RiskZonesParseError != "" {
			detail = fmt.Sprintf("error parseando catalogo en wiki/risk_zones.md (%s); fail-safe a ASDD con compuerta [AUTOR]", contextResp.RiskZonesParseError)
		}
		reasons = append(reasons, TriageReason{
			Code:           "RISK_ZONES_UNPARSEABLE_FAILSAFE",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         detail,
		})
	}

	// Regla 9: KIND_REQUIRES_ASDD
	kindLow := strings.ToLower(strings.TrimSpace(wi.Kind))
	if kindLow == "feature" || kindLow == "ops" {
		structuralOrRiskTrigger = true
		reasons = append(reasons, TriageReason{
			Code:           "KIND_REQUIRES_ASDD",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         fmt.Sprintf("kind=%q exige especificacion previa en carril ASDD", kindLow),
		})
	}

	// Regla 10: EXPLICIT_IMPACT_FLAGS
	if len(wi.ImpactFlags) > 0 {
		structuralOrRiskTrigger = true
		reasons = append(reasons, TriageReason{
			Code:           "EXPLICIT_IMPACT_FLAGS",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         fmt.Sprintf("impact_flags declarados (%s) exigen carril ASDD", strings.Join(wi.ImpactFlags, ", ")),
		})
	}

	// Regla 11: MULTI_MODULE_OR_FILE_THRESHOLD
	rootMods := CountRootModules(wi.AffectedPaths)
	if len(rootMods) > 1 || len(wi.AffectedPaths) > maxFTDFiles {
		structuralOrRiskTrigger = true
		reasons = append(reasons, TriageReason{
			Code:           "MULTI_MODULE_OR_FILE_THRESHOLD",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         fmt.Sprintf("alcance abarca %d modulo(s) raiz (%s) y %d archivo(s) (umbral FTD: 1 modulo y <= %d archivos)", len(rootMods), strings.Join(rootMods, ", "), len(wi.AffectedPaths), maxFTDFiles),
		})
	}

	// Regla 12: NO_BRAIN_COVERAGE
	uncovered := findUncoveredPaths(wi.AffectedPaths, contextResp.MatchedDocs)
	if len(uncovered) > 0 {
		noBrainCoverageFired = true
		reasons = append(reasons, TriageReason{
			Code:           "NO_BRAIN_COVERAGE",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         fmt.Sprintf("rutas sin cobertura activa de documento wiki/adr en 0_Brain: %s", strings.Join(uncovered, ", ")),
		})
	}

	if len(reasons) > 0 {
		wi.Lane = "ASDD"
	} else {
		// Regla 13: FTD_ELIGIBLE_BOUNDED_MAINTENANCE
		wi.Lane = "FTD"
		reasons = append(reasons, TriageReason{
			Code:           "FTD_ELIGIBLE_BOUNDED_MAINTENANCE",
			RuleKind:       "deterministic",
			RulesetVersion: TriageDetRulesetVersion,
			Detail:         fmt.Sprintf("Mantenimiento acotado (%s) en 1 modulo (%d<=%d archivos) con cobertura activa wiki/adr en 0_Brain, 0 impact_flags y riesgo %s (no CRITICO)", kindLow, len(wi.AffectedPaths), maxFTDFiles, wi.RiskLevel),
		})
	}

	wi.Status = "ready"
	wi.PathsConfidence = "explicit"
	wi.QuarantineInfo = nil
	wi.TriageReasons = reasons
	wi.SuggestedTemplate = selectSuggestedTemplate(wi.Status, wi.Lane, kindLow, noBrainCoverageFired, structuralOrRiskTrigger)
}

func findUncoveredPaths(affectedPaths []string, matchedDocs []BrainDocMatch) []string {
	var uncovered []string
	for _, file := range affectedPaths {
		covered := false
		for _, doc := range matchedDocs {
			if !strings.EqualFold(doc.Status, "active") {
				continue
			}
			if !strings.EqualFold(doc.Type, "wiki") && !strings.EqualFold(doc.Type, "adr") {
				continue
			}
			for _, mp := range doc.MatchedPaths {
				if strings.EqualFold(bfcore.NormalizeCoverPath(mp), bfcore.NormalizeCoverPath(file)) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
			for _, cp := range doc.CoversPaths {
				if bfcore.MatchCoverPathFold(cp, file) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			uncovered = append(uncovered, file)
		}
	}
	return uncovered
}

func selectSuggestedTemplate(status, lane, kind string, noBrainCoverageFired, structuralOrRiskTrigger bool) string {
	if status == "needs_clarification" {
		return ""
	}
	if lane == "ASDD" {
		switch kind {
		case "feature":
			return "modulo_nuevo"
		case "ops":
			return "auditoria_e2e"
		default:
			if noBrainCoverageFired && !structuralOrRiskTrigger {
				return "mapeo_estatico"
			}
			return "correccion_hallazgos"
		}
	}
	return "correccion_hallazgos"
}
