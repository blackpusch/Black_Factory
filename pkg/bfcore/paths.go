package bfcore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MaxObservedFileBytes fija el tamano maximo (10 MiB) admitido por ComputeNormalizedFileSHA256.
const MaxObservedFileBytes = 10 << 20

// PathSanitizeOpts parametriza SanitizeRelativePaths manteniendo el rechazo incondicional
// de globs (*?[), traversal (..), rutas personales G4 y nombres reservados de Windows.
type PathSanitizeOpts struct {
	AllowDir       bool
	SplitCommas    bool
	FoldCaseDedupe bool
}

// NormalizeCoverPath unifica separadores a "/" y elimina segmentos o barras redundantes
// para comparar rutas Windows y POSIX de forma determinista.
func NormalizeCoverPath(p string) string {
	s := strings.TrimSpace(p)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, `\`, "/")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	s = path.Clean(s)
	for strings.HasPrefix(s, "./") {
		s = strings.TrimPrefix(s, "./")
	}
	s = strings.Trim(s, "/")
	if s == "." {
		return ""
	}
	return s
}

// MatchCoverPath evalua si filePath coincide con un patron de covers_paths
// (ruta exacta, prefijo de directorio o patron glob con * / ** / ?).
func MatchCoverPath(pattern, filePath string) bool {
	pat := NormalizeCoverPath(pattern)
	file := NormalizeCoverPath(filePath)
	return matchNormalizedPaths(pat, file)
}

// MatchCoverPathFold evalua si filePath coincide con un patron de covers_paths
// aplicando strings.ToLower a ambos argumentos para insensibilidad a mayusculas en Windows.
func MatchCoverPathFold(pattern, filePath string) bool {
	pat := strings.ToLower(NormalizeCoverPath(pattern))
	file := strings.ToLower(NormalizeCoverPath(filePath))
	return matchNormalizedPaths(pat, file)
}

func matchNormalizedPaths(pat, file string) bool {
	if pat == "" || file == "" {
		return false
	}
	if pat == "*" || pat == "**" {
		return true
	}
	if !strings.ContainsAny(pat, "*?[") {
		return file == pat || strings.HasPrefix(file, pat+"/")
	}
	return matchPathSegments(strings.Split(pat, "/"), strings.Split(file, "/"))
}

func matchPathSegments(patSegs, fileSegs []string) bool {
	m, n := len(patSegs), len(fileSegs)
	dp := make([][]bool, m+1)
	for i := range dp {
		dp[i] = make([]bool, n+1)
	}
	dp[m][n] = true

	for i := m - 1; i >= 0; i-- {
		if patSegs[i] == "**" {
			dp[i][n] = dp[i+1][n]
		}
		for j := n - 1; j >= 0; j-- {
			if patSegs[i] == "**" {
				dp[i][j] = dp[i+1][j] || dp[i][j+1]
				continue
			}
			ok, err := path.Match(patSegs[i], fileSegs[j])
			if err == nil && ok {
				dp[i][j] = dp[i+1][j+1]
			}
		}
	}
	return dp[0][0]
}

// SanitizeRelativePaths valida cada ruta en rawPaths ANTES de llamar a path.Clean:
//   - Si coincide con rutas personales Tier 2 (win_user_path o unix_user_path), descarta todas las rutas
//     y devuelve []string{}, "G4_personal_path_in_affected_paths".
//   - Si repoRoot != "" y la ruta es una ruta absoluta dentro de repoRoot, recorta el prefijo repoRoot.
//   - Rechaza siempre rutas vacias (cuando SplitCommas == false), control bytes/NUL, prefijos '/', '\',
//     UNC/NT, ':', globs (*?[), segmentos '.' / '..' / "." / "./", nombres reservados Windows (CON..LPT9)
//     y sufijos '/' o '\' salvo que AllowDir == true (en cuyo caso preserva '/' final).
//   - Devuelve siempre cleanPaths no-nil ([]string{}) y rejectReason == "" en exito.
func SanitizeRelativePaths(rawPaths []string, repoRoot string, opts PathSanitizeOpts) ([]string, string) {
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

	expanded := make([]string, 0, len(rawPaths))
	if opts.SplitCommas {
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
	} else {
		for _, item := range rawPaths {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
			expanded = append(expanded, trimmed)
		}
	}

	seen := make(map[string]bool, len(expanded))
	cleanPaths := make([]string, 0, len(expanded))

	for _, raw := range expanded {
		// 1. Verificar rutas personales Tier 2 antes de cualquier recorte
		if MatchesPersonalPathG4(raw) {
			return []string{}, "G4_personal_path_in_affected_paths"
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
			return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		for i := 0; i < len(candidate); i++ {
			if candidate[i] < 0x20 || candidate[i] == 0x7f {
				return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
		}
		if strings.HasPrefix(candidate, `\\`) || strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, `\`) {
			return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		if strings.Contains(candidate, ":") {
			return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		if strings.ContainsAny(slashCand, "*?[") {
			return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}

		hasTrailingSlash := strings.HasSuffix(slashCand, "/")
		if hasTrailingSlash {
			if !opts.AllowDir {
				return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
			slashCand = strings.TrimSuffix(slashCand, "/")
		}

		// Recortar prefijos "./" repetidos antes de inspeccionar segmentos
		for strings.HasPrefix(slashCand, "./") {
			slashCand = strings.TrimPrefix(slashCand, "./")
		}
		if slashCand == "" || slashCand == "." || strings.HasPrefix(slashCand, "/") || strings.HasSuffix(slashCand, "/") || strings.Contains(slashCand, "//") {
			return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}

		segs := strings.Split(slashCand, "/")
		for _, seg := range segs {
			if seg == "" || seg == "." || seg == ".." {
				return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
			if isWindowsReservedDeviceSegment(seg) {
				return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
			}
		}

		cleaned := path.Clean(slashCand)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
			return []string{}, "PATH_TRAVERSAL_OR_INVALID_SCOPE"
		}
		if hasTrailingSlash {
			cleaned += "/"
		}

		key := cleaned
		if opts.FoldCaseDedupe {
			key = strings.ToLower(cleaned)
		}
		if !seen[key] {
			seen[key] = true
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

// CheckNoReparseUpToNearestExisting asciende desde targetPath hasta el ancestro existente
// mas cercano dentro de rootPath (o hasta la raiz del volumen si rootPath == "") y valida
// toda la cadena con CheckNoReparseChain.
func CheckNoReparseUpToNearestExisting(rootPath, targetPath string) error {
	if strings.TrimSpace(targetPath) == "" {
		return errors.New("ruta objetivo vacia")
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	var absRoot string
	if strings.TrimSpace(rootPath) != "" {
		absRoot, err = filepath.Abs(rootPath)
		if err != nil {
			return err
		}
		absRoot = filepath.Clean(absRoot)
	}

	cur := filepath.Clean(absTarget)
	for {
		vol := filepath.VolumeName(cur)
		if vol != "" && (strings.EqualFold(cur, vol) || strings.EqualFold(cur, vol+`\`)) {
			break
		}
		fi, lstatErr := os.Lstat(cur)
		if lstatErr == nil {
			if IsReparseOrSymlink(fi) {
				return fmt.Errorf("%w: %s", ErrReparsePoint, cur)
			}
			return CheckNoReparseChain(rootPath, cur)
		}
		if !errors.Is(lstatErr, os.ErrNotExist) {
			return lstatErr
		}
		if absRoot != "" && strings.EqualFold(cur, absRoot) {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return CheckNoReparseChain(rootPath, cur)
}

// ComputeNormalizedFileSHA256 valida primero la contencion relativa de relPath dentro de repoRoot
// mediante SanitizeRelativePaths, comprueba ausencia de reparse points via CheckNoReparseChain y:
//   - devuelve ("absent", "", nil) si el archivo no existe;
//   - devuelve error si la ruta es invalida, es un directorio, symlink/reparse o supera MaxObservedFileBytes;
//   - devuelve ("present", hexSHA256, nil) tras leer en streaming eliminando UTF-8 BOM y normalizando CRLF/CR a LF.
func ComputeNormalizedFileSHA256(repoRoot, relPath string) (string, string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return "", "", errors.New("repoRoot vacio")
	}
	clean, rejectReason := SanitizeRelativePaths([]string{relPath}, repoRoot, PathSanitizeOpts{
		AllowDir:       false,
		SplitCommas:    false,
		FoldCaseDedupe: false,
	})
	if rejectReason != "" || len(clean) != 1 {
		return "", "", fmt.Errorf("ruta relativa invalida %q (%s)", relPath, rejectReason)
	}

	absPath := filepath.Join(repoRoot, filepath.FromSlash(clean[0]))
	if err := CheckNoReparseChain(repoRoot, absPath); err != nil {
		return "", "", err
	}

	fi, err := os.Lstat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "absent", "", nil
		}
		return "", "", err
	}
	if IsReparseOrSymlink(fi) {
		return "", "", fmt.Errorf("%w: %s", ErrReparsePoint, absPath)
	}
	if fi.IsDir() || !fi.Mode().IsRegular() {
		return "", "", fmt.Errorf("la ruta %q no es un archivo regular", clean[0])
	}
	if fi.Size() > MaxObservedFileBytes {
		return "", "", fmt.Errorf("el archivo %q excede MaxObservedFileBytes (%d bytes)", clean[0], MaxObservedFileBytes)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()

	sumHex, err := hashNormalizedStream(f, MaxObservedFileBytes)
	if err != nil {
		return "", "", fmt.Errorf("calculando SHA-256 normalizado de %q: %w", clean[0], err)
	}
	return "present", sumHex, nil
}

func hashNormalizedStream(r io.Reader, maxBytes int64) (string, error) {
	h := sha256.New()
	rawBuf := make([]byte, 32*1024)
	outBuf := make([]byte, 0, 32*1024)

	var (
		totalRead  int64
		bomHandled bool
		bomPrefix  [3]byte
		bomLen     int
		pendingCR  bool
	)

	feedNormalized := func(data []byte) {
		outBuf = outBuf[:0]
		for _, b := range data {
			if pendingCR {
				pendingCR = false
				if b == '\n' {
					continue
				}
			}
			if b == '\r' {
				outBuf = append(outBuf, '\n')
				pendingCR = true
				continue
			}
			outBuf = append(outBuf, b)
		}
		if len(outBuf) > 0 {
			_, _ = h.Write(outBuf)
		}
	}

	for {
		n, readErr := r.Read(rawBuf)
		if n > 0 {
			totalRead += int64(n)
			if totalRead > maxBytes {
				return "", fmt.Errorf("tamano excede limite maximo de %d bytes", maxBytes)
			}
			chunk := rawBuf[:n]
			if !bomHandled {
				need := 3 - bomLen
				take := len(chunk)
				if take > need {
					take = need
				}
				copy(bomPrefix[bomLen:], chunk[:take])
				bomLen += take
				chunk = chunk[take:]
				if bomLen == 3 {
					bomHandled = true
					if !(bomPrefix[0] == 0xef && bomPrefix[1] == 0xbb && bomPrefix[2] == 0xbf) {
						feedNormalized(bomPrefix[:3])
					}
				}
			}
			if len(chunk) > 0 {
				feedNormalized(chunk)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return "", readErr
		}
	}

	if !bomHandled && bomLen > 0 {
		feedNormalized(bomPrefix[:bomLen])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
