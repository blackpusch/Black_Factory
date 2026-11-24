package specwriter

import (
	"encoding/json"
	"fmt"
	"strings"

	"blackfactory/pkg/bfcore"
)

// RenderSpecMarkdown proyecta spec.json en una vista Markdown determinista (ASCII/Box-Drawing, cero Mermaid)
// sin crear ni modificar archivos en disco. Aplica cuota MaxSpecBytes, G4 fail-closed (raw+decodificado),
// validacion de schema_version == "1.0" y re-escaneo G4 de la salida generada.
func RenderSpecMarkdown(specBytes []byte) (string, Spec, int, string, error) {
	if len(specBytes) > MaxSpecBytes {
		return "", Spec{}, ExitFixable, ReasonSpecTooLarge, fmt.Errorf("spec.json excede MaxSpecBytes (%d bytes)", MaxSpecBytes)
	}
	g4Hits, jsonErr := ScanRawAndDecodedG4(specBytes)
	if len(g4Hits) > 0 {
		return "", Spec{}, ExitG4, ReasonG4Violation, fmt.Errorf("%w: regla %s (linea %d)", ErrG4Violation, g4Hits[0].Rule, g4Hits[0].Line)
	}
	if jsonErr != nil {
		return "", Spec{}, ExitFixable, ReasonInvalidJSON, fmt.Errorf("JSON invalido: %w", jsonErr)
	}

	var spec Spec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return "", Spec{}, ExitFixable, ReasonInvalidJSON, fmt.Errorf("decodificando spec.json: %w", err)
	}
	if spec.SchemaVersion != SchemaVersion {
		return "", Spec{}, ExitFixable, ReasonValidationFailed, fmt.Errorf("schema_version invalido %q (esperado %q)", spec.SchemaVersion, SchemaVersion)
	}

	specSHA, err := CanonicalSpecSHA256(spec)
	if err != nil {
		return "", Spec{}, ExitFixable, ReasonInvalidJSON, err
	}

	var b strings.Builder
	b.WriteString("<!-- AUTO-GENERATED DERIVED VIEW: DO NOT EDIT -->\n")
	b.WriteString(fmt.Sprintf("<!-- spec_sha256: %s | trace_id: %s -->\n\n", specSHA, spec.WorkItemRef.TraceID))
	b.WriteString(fmt.Sprintf("# Especificación Técnica: %s\n\n", spec.Title))

	b.WriteString("## Metadatos\n\n")
	b.WriteString("| Campo | Valor |\n")
	b.WriteString("| :--- | :--- |\n")
	b.WriteString(fmt.Sprintf("| WorkItem | `%s` (rev %d) |\n", spec.WorkItemRef.ID, spec.WorkItemRef.Revision))
	b.WriteString(fmt.Sprintf("| Trace ID | `%s` |\n", spec.WorkItemRef.TraceID))
	b.WriteString(fmt.Sprintf("| Proyecto | `%s` |\n", spec.Project))
	b.WriteString(fmt.Sprintf("| Carril | `%s` |\n", spec.Lane))
	b.WriteString(fmt.Sprintf("| Plantilla | `%s` |\n", spec.Template))
	b.WriteString(fmt.Sprintf("| Rama destino | `%s` |\n", spec.TargetBranch))
	b.WriteString(fmt.Sprintf("| Requiere `[AUTOR]` HITL | `%v` |\n", spec.RequiresAutorHITL))
	b.WriteString(fmt.Sprintf("| Mutación de datos (G3) | %s |\n", spec.G3DataMutation))
	b.WriteString(fmt.Sprintf("| Spec SHA-256 | `%s` |\n\n", specSHA))

	b.WriteString("## Problema\n\n")
	b.WriteString(spec.Narrative.Problem + "\n\n")
	b.WriteString("## Causa Raíz\n\n")
	b.WriteString(spec.Narrative.RootCause + "\n\n")
	b.WriteString("## Cambio Propuesto\n\n")
	b.WriteString(spec.Narrative.ProposedChange + "\n\n")

	if spec.Lane == "ASDD" || len(spec.Narrative.AlternativesConsidered) > 0 || spec.Narrative.InterfacesAndContracts != "" || len(spec.Narrative.EdgeCases) > 0 || spec.Narrative.RollbackPlan != "" {
		b.WriteString("## Alternativas Consideradas\n\n")
		for _, alt := range spec.Narrative.AlternativesConsidered {
			b.WriteString("- " + alt + "\n")
		}
		b.WriteString("\n## Interfaces y Contratos\n\n")
		b.WriteString(spec.Narrative.InterfacesAndContracts + "\n\n")
		b.WriteString("## Casos Borde\n\n")
		for _, ec := range spec.Narrative.EdgeCases {
			b.WriteString("- " + ec + "\n")
		}
		b.WriteString("\n## Plan de Rollback\n\n")
		b.WriteString(spec.Narrative.RollbackPlan + "\n\n")
	}

	b.WriteString("## Unidades de Trabajo (`work_units`)\n\n")
	b.WriteString("| # | ID | Título | Depende de | Archivos `can_modify` |\n")
	b.WriteString("| :---: | :--- | :--- | :--- | :---: |\n")
	for i, u := range spec.WorkUnits {
		deps := "-"
		if len(u.DependsOn) > 0 {
			deps = strings.Join(u.DependsOn, ", ")
		}
		b.WriteString(fmt.Sprintf("| %02d | `%s` | %s | %s | %d |\n", i+1, u.ID, u.Title, deps, len(u.CanModify)))
	}
	b.WriteString("\n### Grafo de Dependencias (ASCII)\n\n```text\n")
	for i, u := range spec.WorkUnits {
		if len(u.DependsOn) == 0 {
			b.WriteString(fmt.Sprintf("[%02d] %s (raíz)\n", i+1, u.ID))
		} else {
			b.WriteString(fmt.Sprintf("[%02d] %s ◄── %s\n", i+1, u.ID, strings.Join(u.DependsOn, ", ")))
		}
	}
	b.WriteString("```\n\n")

	if len(spec.RiskMitigations) > 0 {
		b.WriteString("## Mitigación de Zonas de Riesgo\n\n")
		b.WriteString("| Risk ID | Nivel | `verify_step_ids` | `review_gates` | HITL |\n")
		b.WriteString("| :--- | :---: | :--- | :--- | :---: |\n")
		for _, rm := range spec.RiskMitigations {
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s | `%v` |\n",
				rm.RiskID, rm.Level, strings.Join(rm.VerifyStepIDs, ", "), strings.Join(rm.ReviewGates, ", "), rm.RequiresAutorHITL))
		}
		b.WriteString("\n")
	}

	if len(spec.Decisions) > 0 {
		b.WriteString("## Decisiones Registradas\n\n")
		for _, d := range spec.Decisions {
			b.WriteString(fmt.Sprintf("- **%s** (`%s` → `%s`, por `%s`, `%s`): %s (`scope_sha256: %s`)\n",
				d.Reason, d.LaneFrom, d.LaneTo, d.DecidedBy, d.Timestamp, d.Rationale, d.ScopeSHA256))
		}
		b.WriteString("\n")
	}

	if len(spec.G5Citations) > 0 {
		b.WriteString("## Citas Técnicas Verificables (`G5`)\n\n")
		for _, c := range spec.G5Citations {
			b.WriteString(fmt.Sprintf("- `%s:%d` (ancla `%s`): %s\n", c.Path, c.Line, c.SymbolAnchor, c.Claim))
		}
		b.WriteString("\n")
	}

	if len(spec.BrainFollowups) > 0 {
		b.WriteString("## Seguimientos Propuestos para `0_Brain`\n\n")
		for _, bf := range spec.BrainFollowups {
			b.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", bf.TargetHint, bf.Trigger, bf.Summary))
		}
		b.WriteString("\n")
	}

	out := b.String()
	if hits := bfcore.ScanG4All(out); len(hits) > 0 {
		return "", Spec{}, ExitG4, ReasonG4Violation, fmt.Errorf("%w: fuga en salida renderizada (regla %s)", ErrG4Violation, hits[0].Rule)
	}
	return out, spec, ExitOK, ReasonOK, nil
}
