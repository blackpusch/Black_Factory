package intake

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"blackfactory/pkg/bfcore"
)

// ErrInputTooLarge indica que el payload supera MaxInputBytes (1 MiB).
var ErrInputTooLarge = errors.New("payload de entrada supera el limite MaxInputBytes (1 MiB)")

// ReadRawInput lee hasta MaxInputBytes (1 MiB) desde un archivo JSON local (verificando
// bfcore.CheckNoReparseChain sobre la cadena de ancestros) o desde stdin cuando inputPath == "-",
// y recorta cualquier UTF-8 BOM inicial (\xEF\xBB\xBF). Si inputPath == "", devuelve nil, nil.
func ReadRawInput(inputPath, repoRoot string, stdin io.Reader) ([]byte, error) {
	trimmed := strings.TrimSpace(inputPath)
	if trimmed == "" {
		return nil, nil
	}

	var raw []byte
	if trimmed == "-" {
		if stdin == nil {
			return nil, errors.New("stdin es nil para --input -")
		}
		lr := io.LimitReader(stdin, MaxInputBytes+1)
		data, err := io.ReadAll(lr)
		if err != nil {
			return nil, fmt.Errorf("leyendo stdin: %w", err)
		}
		if len(data) > MaxInputBytes {
			return nil, ErrInputTooLarge
		}
		raw = data
	} else {
		if err := bfcore.CheckNoReparseChain(repoRoot, trimmed); err != nil {
			return nil, fmt.Errorf("verificando cadena de ancestros de %s: %w", trimmed, err)
		}
		fi, err := os.Lstat(trimmed)
		if err != nil {
			return nil, fmt.Errorf("inspeccionando archivo de entrada %s: %w", trimmed, err)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("la ruta de entrada %s es un directorio", trimmed)
		}
		if bfcore.IsReparseOrSymlink(fi) {
			return nil, fmt.Errorf("el archivo de entrada %s es un symlink o reparse point: %w", trimmed, bfcore.ErrReparsePoint)
		}
		if fi.Size() > MaxInputBytes {
			return nil, ErrInputTooLarge
		}
		data, err := os.ReadFile(trimmed)
		if err != nil {
			return nil, fmt.Errorf("leyendo archivo de entrada %s: %w", trimmed, err)
		}
		if len(data) > MaxInputBytes {
			return nil, ErrInputTooLarge
		}
		raw = data
	}

	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	return raw, nil
}

// ScanRawAndFlagsTier1 ejecuta el escaneo G4 Fase-A (Tier 1 o todas las 11 reglas si strictG4 == true)
// sobre los bytes crudos de entrada y sobre cada bandera CLI provista.
func ScanRawAndFlagsTier1(raw []byte, cliOverrides RawIntake, strictG4 bool) []bfcore.G4Finding {
	var findings []bfcore.G4Finding
	if len(raw) > 0 {
		findings = append(findings, bfcore.ScanTier1("raw_input", string(raw), strictG4)...)
	}

	flagFields := []struct {
		name string
		val  string
	}{
		{"--project", cliOverrides.Project},
		{"--title", cliOverrides.Title},
		{"--description", cliOverrides.Description},
		{"--note", cliOverrides.Note},
		{"--kind", cliOverrides.Kind},
		{"--source", cliOverrides.Source},
		{"--source-ref", cliOverrides.SourceRef},
	}
	for _, ff := range flagFields {
		if ff.val != "" {
			findings = append(findings, bfcore.ScanTier1(ff.name, ff.val, strictG4)...)
		}
	}
	for _, p := range cliOverrides.AffectedPaths {
		if p != "" {
			findings = append(findings, bfcore.ScanTier1("--path", p, strictG4)...)
		}
	}
	for _, f := range cliOverrides.ImpactFlags {
		if f != "" {
			findings = append(findings, bfcore.ScanTier1("--impact-flag", f, strictG4)...)
		}
	}
	return findings
}

type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*s = nil
		return nil
	}
	if trimmed[0] == '[' {
		var arr []string
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var str string
	if err := json.Unmarshal(trimmed, &str); err != nil {
		return err
	}
	var out []string
	for _, part := range strings.Split(str, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			out = append(out, t)
		}
	}
	*s = out
	return nil
}

type jsonIntakeWire struct {
	Project       string        `json:"project"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Note          string        `json:"note"`
	Kind          string        `json:"kind"`
	Source        string        `json:"source"`
	SourceRef     string        `json:"source_ref"`
	AffectedPaths stringOrSlice `json:"affected_paths"`
	ImpactFlags   stringOrSlice `json:"impact_flags"`
}

// ParseJSONIntake decodifica un payload JSON con DisallowUnknownFields() tras recortar UTF-8 BOM,
// aceptando tanto arreglos JSON ["a","b"] como cadenas separadas por comas "a,b" en affected_paths e impact_flags.
func ParseJSONIntake(raw []byte) (RawIntake, error) {
	clean := bytes.TrimSpace(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")))
	if len(clean) == 0 {
		return RawIntake{}, errors.New("payload JSON vacio")
	}
	if clean[0] != '{' {
		return RawIntake{}, errors.New("JSON invalido: se esperaba un objeto JSON '{...}'")
	}

	dec := json.NewDecoder(bytes.NewReader(clean))
	dec.DisallowUnknownFields()

	var wire jsonIntakeWire
	if err := dec.Decode(&wire); err != nil {
		return RawIntake{}, fmt.Errorf("JSON invalido: %w", err)
	}
	// Verificar que no existan tokens adicionales tras el objeto raiz
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return RawIntake{}, errors.New("JSON invalido: contenido adicional despues del objeto raiz")
	}

	return RawIntake{
		Project:       wire.Project,
		Title:         wire.Title,
		Description:   wire.Description,
		Note:          wire.Note,
		Kind:          wire.Kind,
		Source:        wire.Source,
		SourceRef:     wire.SourceRef,
		AffectedPaths: []string(wire.AffectedPaths),
		ImpactFlags:   []string(wire.ImpactFlags),
	}, nil
}

// MergeCLIOverrides aplica las banderas CLI explicitas sobre la estructura base decodificada desde JSON.
func MergeCLIOverrides(base RawIntake, overrides RawIntake) RawIntake {
	out := base
	if strings.TrimSpace(overrides.Project) != "" {
		out.Project = overrides.Project
	}
	if strings.TrimSpace(overrides.Title) != "" {
		out.Title = overrides.Title
	}
	if strings.TrimSpace(overrides.Description) != "" {
		out.Description = overrides.Description
	}
	if strings.TrimSpace(overrides.Note) != "" {
		out.Note = overrides.Note
	}
	if strings.TrimSpace(overrides.Kind) != "" {
		out.Kind = overrides.Kind
	}
	if strings.TrimSpace(overrides.Source) != "" {
		out.Source = overrides.Source
	}
	if strings.TrimSpace(overrides.SourceRef) != "" {
		out.SourceRef = overrides.SourceRef
	}
	if len(overrides.AffectedPaths) > 0 {
		out.AffectedPaths = append([]string(nil), overrides.AffectedPaths...)
	}
	if len(overrides.ImpactFlags) > 0 {
		out.ImpactFlags = append([]string(nil), overrides.ImpactFlags...)
	}
	return out
}

// ScanDecodedTier1 ejecuta el escaneo G4 Fase-B sobre todos los campos string ya decodificados
// (cerrando vectores de escape Unicode/JSON como \u0067hp_... o postgres:\/\/user:pass@host).
func ScanDecodedTier1(in RawIntake, strictG4 bool) []bfcore.G4Finding {
	var findings []bfcore.G4Finding
	fields := []struct {
		name string
		val  string
	}{
		{"project", in.Project},
		{"title", in.Title},
		{"description", in.Description},
		{"note", in.Note},
		{"kind", in.Kind},
		{"source", in.Source},
		{"source_ref", in.SourceRef},
	}
	for _, f := range fields {
		if f.val != "" {
			findings = append(findings, bfcore.ScanTier1(f.name, f.val, strictG4)...)
		}
	}
	for _, p := range in.AffectedPaths {
		if p != "" {
			findings = append(findings, bfcore.ScanTier1("affected_paths", p, strictG4)...)
		}
	}
	for _, fl := range in.ImpactFlags {
		if fl != "" {
			findings = append(findings, bfcore.ScanTier1("impact_flags", fl, strictG4)...)
		}
	}
	return findings
}

// SanitizeAndRedactTier2 aplica redaccion en punto fijo ([REDACTED:G4:<rule>]) sobre los campos de texto libre
// (title, description, note) y verifica si algun campo de metadatos (project, kind, source, source_ref)
// contiene PII Tier 2 (en cuyo caso los redacta en memoria para no persistir PII y devuelve metadataHasTier2 = true
// para activar cuarentena bajo MISSING_REQUIRED_FIELDS).
func SanitizeAndRedactTier2(in RawIntake) (RawIntake, []G4Redaction, bool) {
	out := in
	var allRedactions []G4Redaction

	var rTitle, rDesc, rNote []G4Redaction
	out.Title, rTitle = bfcore.RedactTier2Text("title", out.Title)
	out.Description, rDesc = bfcore.RedactTier2Text("description", out.Description)
	out.Note, rNote = bfcore.RedactTier2Text("note", out.Note)
	allRedactions = append(allRedactions, rTitle...)
	allRedactions = append(allRedactions, rDesc...)
	allRedactions = append(allRedactions, rNote...)

	metadataHasTier2 := false
	metaFields := []struct {
		name string
		ptr  *string
	}{
		{"project", &out.Project},
		{"kind", &out.Kind},
		{"source", &out.Source},
		{"source_ref", &out.SourceRef},
	}
	for _, mf := range metaFields {
		redVal, reds := bfcore.RedactTier2Text(mf.name, *mf.ptr)
		if len(reds) > 0 {
			metadataHasTier2 = true
			*mf.ptr = redVal
			allRedactions = append(allRedactions, reds...)
		}
	}

	if allRedactions == nil {
		allRedactions = []G4Redaction{}
	}
	return out, allRedactions, metadataHasTier2
}
