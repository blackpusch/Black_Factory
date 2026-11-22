package bfcore

import (
	"path"
	"strings"
)

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
