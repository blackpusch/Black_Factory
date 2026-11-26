package intake

import (
	"fmt"
	"strings"
)

// RenderWorkItemMarkdown genera bajo demanda una vista Markdown legible con tablas
// ASCII/Unicode Box-Drawing (estrictamente sin bloques Mermaid, cumpliendo G2 y NG5).
func RenderWorkItemMarkdown(wi *WorkItem) string {
	if wi == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<!-- AUTO-GENERATED DERIVED VIEW: DO NOT EDIT (Source: %s.json) -->\n\n", wi.ID))
	b.WriteString(fmt.Sprintf("# WorkItem `%s`: %s\n\n", wi.ID, wi.Title))

	laneDisplay := wi.Lane
	if laneDisplay == "" {
		laneDisplay = "(quarantine)"
	}
	tplDisplay := wi.SuggestedTemplate
	if tplDisplay == "" {
		tplDisplay = "(none)"
	}

	b.WriteString("```text\n")
	b.WriteString("┌─────────────────────────────────────────────────────────────────────────────┐\n")
	b.WriteString(fmt.Sprintf("│ ID: %-20s  Status: %-20s  Revision: %-9d │\n", wi.ID, wi.Status, wi.Revision))
	b.WriteString(fmt.Sprintf("│ Project: %-15s  Kind: %-22s  Lane: %-13s │\n", wi.Project, wi.Kind, laneDisplay))
	b.WriteString(fmt.Sprintf("│ Risk Level: %-9s  AUTOR HITL: %-6v  Template: %-22s │\n", wi.RiskLevel, wi.RequiresAutorHITL, tplDisplay))
	b.WriteString("└─────────────────────────────────────────────────────────────────────────────┘\n")
	b.WriteString("```\n\n")

	b.WriteString("## Metadatos e Identidad\n\n")
	b.WriteString("| Campo | Valor |\n")
	b.WriteString("| :--- | :--- |\n")
	b.WriteString(fmt.Sprintf("| `schema_version` | `%s` |\n", wi.SchemaVersion))
	b.WriteString(fmt.Sprintf("| `dedupe_key` | `%s` |\n", wi.DedupeKey))
	b.WriteString(fmt.Sprintf("| `trace_id` | `%s` |\n", wi.TraceID))
	b.WriteString(fmt.Sprintf("| `source` / `source_ref` | `%s` / `%s` |\n", wi.Source, wi.SourceRef))
	b.WriteString(fmt.Sprintf("| `created_at` / `updated_at` | `%s` / `%s` |\n", wi.CreatedAt, wi.UpdatedAt))
	b.WriteString(fmt.Sprintf("| `sanitized_payload_sha256` | `%s` |\n\n", wi.Sanitization.SanitizedPayloadSHA256))

	b.WriteString("## Descripción\n\n")
	b.WriteString(wi.Description + "\n\n")
	if strings.TrimSpace(wi.Note) != "" {
		b.WriteString("### Nota Operativa\n\n")
		b.WriteString(wi.Note + "\n\n")
	}

	b.WriteString("## Alcance Declarado (`affected_paths`)\n\n")
	if len(wi.AffectedPaths) == 0 {
		b.WriteString("- *(Sin rutas declaradas)*\n\n")
	} else {
		for _, p := range wi.AffectedPaths {
			b.WriteString(fmt.Sprintf("- `%s`\n", p))
		}
		b.WriteString("\n")
	}

	if wi.QuarantineInfo != nil {
		b.WriteString("## Retención en Cuarentena (`G6`)\n\n")
		b.WriteString(fmt.Sprintf("- **Códigos de razón:** `%s`\n", strings.Join(wi.QuarantineInfo.ReasonCodes, ", ")))
		if len(wi.QuarantineInfo.MissingFields) > 0 {
			b.WriteString(fmt.Sprintf("- **Campos faltantes:** `%s`\n", strings.Join(wi.QuarantineInfo.MissingFields, ", ")))
		}
		if wi.QuarantineInfo.InvalidPathReason != "" {
			b.WriteString(fmt.Sprintf("- **Motivo de ruta inválida:** `%s`\n", wi.QuarantineInfo.InvalidPathReason))
		}
		b.WriteString(fmt.Sprintf("- **Detalle:** %s\n", wi.QuarantineInfo.Detail))
		b.WriteString(fmt.Sprintf("- **Comando de resolución (`resolve_hint`):**\n  ```text\n  %s\n  ```\n\n", wi.QuarantineInfo.ResolveHint))
	}

	if len(wi.TriageReasons) > 0 {
		b.WriteString("## Razones de Triaje Determinista\n\n")
		b.WriteString("| Código | Tipo | Ruleset | Detalle |\n")
		b.WriteString("| :--- | :--- | :--- | :--- |\n")
		for _, r := range wi.TriageReasons {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n", r.Code, r.RuleKind, r.RulesetVersion, r.Detail))
		}
		b.WriteString("\n")
	}

	if len(wi.RiskHits) > 0 {
		b.WriteString("## Zonas de Riesgo Detectadas (`0_Brain`)\n\n")
		b.WriteString("| ID | Proyecto | Nivel | Rutas Coincidentes | Compuerta Obligatoria |\n")
		b.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, h := range wi.RiskHits {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | %s |\n", h.ID, h.Project, h.Level, strings.Join(h.MatchedPaths, ", "), h.MandatoryGate))
		}
		b.WriteString("\n")
	}

	if len(wi.Sanitization.G4Redactions) > 0 {
		b.WriteString("## Redacciones G4 Tier 2 Aplicadas\n\n")
		b.WriteString("| Regla | Campo | Conteo |\n")
		b.WriteString("| :--- | :--- | :---: |\n")
		for _, red := range wi.Sanitization.G4Redactions {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %d |\n", red.Rule, red.Field, red.Count))
		}
		b.WriteString("\n")
	}

	if len(wi.BrainEvidence.Invocations) > 0 {
		b.WriteString("## Evidencia Literal `0_Brain` (`G1`)\n\n")
		b.WriteString("| Comando | Exit Code | Duración (ms) | Resultados |\n")
		b.WriteString("| :--- | :---: | :---: | :---: |\n")
		for _, inv := range wi.BrainEvidence.Invocations {
			b.WriteString(fmt.Sprintf("| `%s` | `%d` | %d | %d |\n", inv.Command, inv.ExitCode, inv.DurationMs, inv.ResultCount))
		}
		b.WriteString("\n")
	}

	return b.String()
}
