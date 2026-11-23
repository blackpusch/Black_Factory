package intake

import (
	"path"
	"strings"

	"blackfactory/pkg/bfcore"
)

// SanitizeAffectedPaths valida cada ruta en rawPaths ANTES de llamar a path.Clean:
//  1. Si coincide con rutas personales Tier 2 (win_user_path o unix_user_path), descarta todas las rutas
//     en memoria y devuelve nil, "G4_personal_path_in_affected_paths".
//  2. Si repoRoot != "" y la ruta es una ruta absoluta dentro de repoRoot (comparada sin distincion de mayusculas
//     tras normalizar '\' -> '/'), recorta el prefijo repoRoot.
//  3. Rechaza cualquier ruta vacia, que inicie con '/' o '\', UNC ('\\') o dispositivo NT ('\\?\'), que contenga
//     ':' (letra de unidad o drive relativo C:file.go), bytes de control/nulos, segmento '..', que termine en '/' o '\'
//     (directorio), que contenga metacaracteres glob ('*', '?', '[') o cuyo segmento sea un nombre de dispositivo
//     reservado de Windows (CON, PRN, AUX, NUL, COM1..9, LPT1..9), devolviendo nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE".
//  4. Normaliza separadores a '/', recorta './' inicial, aplica path.Clean y deduplica preservando el orden.
func SanitizeAffectedPaths(rawPaths []string, repoRoot string) ([]string, string) {
	if len(rawPaths) == 0 {
		return []string{}, ""
	}

	normRepoPrefix := ""
	if strings.TrimSpace(repoRoot) != "" {
		r := strings.ReplaceAll(strings.TrimSpace(repoRoot), `\`, "/")
		r = strings.TrimRight(r, "/")
		if r != "" {
			normRepoPrefix = r + "/"
		}
	}

	var expanded []string
	for _, item := range rawPaths {
		for _, part := range strings.Split(item, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				expanded = append(expanded, trimmed)
			}
		}
	}
	if len(expanded) == 0 {
		return []string{}, ""
	}

	seen := make(map[string]bool, len(expanded))
	cleanPaths := make([]string, 0, len(expanded))

	for _, raw := range expanded {
		// 1. Verificar rutas personales Tier 2 antes de cualquier recorte
		if bfcore.MatchesPersonalPathG4(raw) {
			return nil, "G4_personal_path_in_affected_paths"
		}

		candidate := raw
		slashCand := strings.ReplaceAll(candidate, `\`, "/")

		// 2. Si es ruta absoluta dentro de repoRoot, recortar el prefijo repoRoot
		if normRepoPrefix != "" && len(slashCand) > len(normRepoPrefix) &&
			strings.EqualFold(slashCand[:len(normRepoPrefix)], normRepoPrefix) {
			candidate = slashCand[len(normRepoPrefix):]
			slashCand = candidate
		}

		// 3. Validaciones de seguridad previas a path.Clean
		if candidate == "" {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		for i := 0; i < len(candidate); i++ {
			if candidate[i] < 0x20 || candidate[i] == 0x7f {
				return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
		}
		if strings.HasPrefix(candidate, `\\`) || strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, `\`) {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		if strings.Contains(candidate, ":") {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		if strings.HasSuffix(slashCand, "/") {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		if strings.ContainsAny(slashCand, "*?[") {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}

		// Recortar prefijos "./" repetidos antes de inspeccionar segmentos
		for strings.HasPrefix(slashCand, "./") {
			slashCand = strings.TrimPrefix(slashCand, "./")
		}
		if slashCand == "" || strings.HasPrefix(slashCand, "/") || strings.Contains(slashCand, "//") {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}

		segs := strings.Split(slashCand, "/")
		for _, seg := range segs {
			if seg == "" || seg == "." || seg == ".." {
				return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
			if isWindowsReservedDeviceSegment(seg) {
				return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
		}

		cleaned := path.Clean(slashCand)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
			return nil, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}

		if !seen[cleaned] {
			seen[cleaned] = true
			cleanPaths = append(cleanPaths, cleaned)
		}
	}

	return cleanPaths, ""
}

func isWindowsReservedDeviceSegment(seg string) bool {
	base := seg
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	switch strings.ToLower(strings.TrimSpace(base)) {
	case "con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return true
	default:
		return false
	}
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
		norm = strings.TrimPrefix(norm, "./")
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
