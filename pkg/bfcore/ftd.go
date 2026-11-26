package bfcore

import (
	"path"
	"strings"
)

const (
	// DefaultMaxFTDFiles es el tope por defecto de archivos efectivos para el carril FTD.
	DefaultMaxFTDFiles = 5
	// MaxFTDExemptGoTests es el maximo de tests Go hermanos (*_test.go en mismo directorio que un archivo productivo) exentos en FTD (Opcion 5B).
	MaxFTDExemptGoTests = 5
	// MaxFTDPhysicalFiles es el tope duro de archivos fisicos totales (productivos + tests exentos) en FTD.
	MaxFTDPhysicalFiles = 10
	// MaxFTDRootModules es el maximo de modulos raiz permitidos en FTD sin excepciones.
	MaxFTDRootModules = 1
)

// FTDEvaluation resume el resultado determinista de evaluar los limites de alcance FTD bajo la Opcion 5B.
type FTDEvaluation struct {
	PhysicalCount   int
	EffectiveCount  int
	ExemptTestCount int
	RootModules     []string
	WithinBounds    bool
}

// CountRootModules extrae los segmentos de modulo raiz distintos de una lista de rutas relativas limpias
// (ej. "0_Brain", "1_Intake", "GengiSWAP"; archivos en la raiz sin '/' se mapean a ".").
func CountRootModules(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var modules []string
	for _, p := range paths {
		norm := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
		for strings.HasPrefix(norm, "./") {
			norm = strings.TrimPrefix(norm, "./")
		}
		if norm == "" {
			continue
		}
		mod := "."
		if slash := strings.IndexByte(norm, '/'); slash > 0 {
			mod = norm[:slash]
		}
		key := strings.ToLower(mod)
		if !seen[key] {
			seen[key] = true
			modules = append(modules, mod)
		}
	}
	return modules
}

// EvaluateFTDBounds evalua si un conjunto de rutas cumple los limites de FTD bajo la Opcion 5B:
//   - Si maxEffective <= 0, aplica DefaultMaxFTDFiles (5).
//   - Si len(paths) == 0 (o no quedan rutas no vacias), devuelve WithinBounds: false.
//   - Normaliza y deduplica rutas de forma insensible a mayusculas.
//   - Exime archivos *_test.go unicamente cuando su directorio padre contiene al menos un archivo
//     productivo (no *_test.go) en la lista y hasta el tope duro MaxFTDExemptGoTests (5).
//   - Todo *_test.go huerfano, todo test por encima del 5.o exento y todo archivo no *_test.go (.py, .js, .ts, etc.) cuenta 1:1.
//   - RootModules se calcula sobre todos los archivos fisicos incluidos los tests exentos (<= 1 modulo raiz sin excepciones).
func EvaluateFTDBounds(paths []string, maxEffective int) FTDEvaluation {
	if maxEffective <= 0 {
		maxEffective = DefaultMaxFTDFiles
	}
	if len(paths) == 0 {
		return FTDEvaluation{WithinBounds: false}
	}

	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, raw := range paths {
		norm := NormalizeCoverPath(raw)
		if norm == "" {
			continue
		}
		key := strings.ToLower(norm)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, norm)
		}
	}
	if len(unique) == 0 {
		return FTDEvaluation{WithinBounds: false}
	}

	productiveDirs := make(map[string]bool)
	for _, p := range unique {
		if !isGoTestFilePath(p) {
			productiveDirs[strings.ToLower(path.Dir(p))] = true
		}
	}

	effectiveCount := 0
	exemptCount := 0
	for _, p := range unique {
		if isGoTestFilePath(p) {
			dirKey := strings.ToLower(path.Dir(p))
			if productiveDirs[dirKey] && exemptCount < MaxFTDExemptGoTests {
				exemptCount++
				continue
			}
		}
		effectiveCount++
	}

	rootMods := CountRootModules(unique)
	physicalCount := len(unique)
	within := physicalCount > 0 &&
		len(rootMods) <= MaxFTDRootModules &&
		effectiveCount <= maxEffective &&
		exemptCount <= MaxFTDExemptGoTests &&
		physicalCount <= MaxFTDPhysicalFiles

	return FTDEvaluation{
		PhysicalCount:   physicalCount,
		EffectiveCount:  effectiveCount,
		ExemptTestCount: exemptCount,
		RootModules:     rootMods,
		WithinBounds:    within,
	}
}

func isGoTestFilePath(normPath string) bool {
	base := strings.ToLower(path.Base(normPath))
	return strings.HasSuffix(base, "_test.go")
}
