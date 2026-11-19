package brain

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DriftItem describe un documento OKF cuyo contenido quedo potencialmente desactualizado
// respecto a commits posteriores a su verified_head en las rutas que cubre.
type DriftItem struct {
	DocID        string   `json:"doc_id"`
	RelPath      string   `json:"rel_path"`
	Title        string   `json:"title"`
	Project      string   `json:"project"`
	VerifiedHead string   `json:"verified_head"`
	CurrentHead  string   `json:"current_head"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	Reason       string   `json:"reason"`
}

// GitHead obtiene el hash completo de HEAD en repoDir.
func GitHead(repoDir string) (string, error) {
	out, err := runGit(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckDrift compara verified_head..HEAD mediante git diff --name-only contra
// covers_paths de cada documento OKF mapeado a un repositorio en projectRepos.
func CheckDrift(vaultRoot string, projectRepos map[string]string) ([]DriftItem, error) {
	absVault, err := filepath.Abs(vaultRoot)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absVault)
	if err != nil {
		return nil, fmt.Errorf("accediendo a boveda %s: %w", absVault, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s no es un directorio", absVault)
	}

	if len(projectRepos) == 0 {
		return []DriftItem{}, nil
	}

	reposByProject := make(map[string]string, len(projectRepos))
	for proj, dir := range projectRepos {
		key := strings.ToLower(strings.TrimSpace(proj))
		if key == "" || strings.TrimSpace(dir) == "" {
			continue
		}
		absRepo, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolviendo repo para %q: %w", proj, err)
		}
		reposByProject[key] = absRepo
	}

	files, err := ListVaultMarkdownFiles(absVault)
	if err != nil {
		return nil, err
	}

	headCache := make(map[string]string)
	diffCache := make(map[string]gitDiffResult)
	drift := make([]DriftItem, 0)

	for _, absFile := range files {
		doc, err := ParseFile(absVault, absFile)
		if err != nil {
			return nil, fmt.Errorf("parseando %s: %w", absFile, err)
		}
		if !doc.HasFrontmatter || strings.HasPrefix(doc.RelPath, "_proposals/") {
			continue
		}

		verifiedHead := strings.TrimSpace(doc.Meta.VerifiedHead)
		if verifiedHead == "" || len(doc.Meta.CoversPaths) == 0 {
			continue
		}

		projKey := strings.ToLower(strings.TrimSpace(doc.Meta.Project))
		repoDir, ok := reposByProject[projKey]
		if !ok {
			repoDir, ok = reposByProject["*"]
		}
		if !ok {
			continue
		}

		curHead, cached := headCache[repoDir]
		if !cached {
			curHead, err = GitHead(repoDir)
			if err != nil {
				return nil, fmt.Errorf("obteniendo HEAD de %s (%s): %w", repoDir, doc.Meta.Project, err)
			}
			headCache[repoDir] = curHead
		}

		if headsMatch(curHead, verifiedHead) {
			continue
		}

		cacheKey := repoDir + "|" + verifiedHead
		diffRes, ok := diffCache[cacheKey]
		if !ok {
			diffRes, err = gitDiffSince(repoDir, verifiedHead)
			if err != nil {
				return nil, fmt.Errorf("calculando drift para %s (%s): %w", doc.RelPath, repoDir, err)
			}
			diffCache[cacheKey] = diffRes
		}

		if diffRes.missingCommit {
			drift = append(drift, DriftItem{
				DocID:        doc.Meta.ID,
				RelPath:      doc.RelPath,
				Title:        doc.Meta.Title,
				Project:      doc.Meta.Project,
				VerifiedHead: verifiedHead,
				CurrentHead:  curHead,
				Reason:       "verified_head no encontrado en el repositorio",
			})
			continue
		}

		matched := matchFilesAgainstCovers(doc.Meta.CoversPaths, diffRes.changedFiles)
		if len(matched) == 0 {
			continue
		}

		drift = append(drift, DriftItem{
			DocID:        doc.Meta.ID,
			RelPath:      doc.RelPath,
			Title:        doc.Meta.Title,
			Project:      doc.Meta.Project,
			VerifiedHead: verifiedHead,
			CurrentHead:  curHead,
			ChangedFiles: matched,
			Reason:       "archivos cubiertos modificados en git desde verified_head",
		})
	}

	sort.Slice(drift, func(i, j int) bool {
		return drift[i].RelPath < drift[j].RelPath
	})
	return drift, nil
}

type gitDiffResult struct {
	missingCommit bool
	changedFiles  []string
}

func headsMatch(currentHead, verifiedHead string) bool {
	c := strings.ToLower(strings.TrimSpace(currentHead))
	v := strings.ToLower(strings.TrimSpace(verifiedHead))
	if c == "" || v == "" {
		return false
	}
	return c == v || (len(v) >= 4 && strings.HasPrefix(c, v))
}

func gitDiffSince(repoDir, verifiedHead string) (gitDiffResult, error) {
	if _, err := runGit(repoDir, "rev-parse", "--verify", verifiedHead+"^{commit}"); err != nil {
		return gitDiffResult{missingCommit: true}, nil
	}

	out, err := runGit(repoDir, "-c", "core.quotepath=false", "diff", "--name-only", "-z", verifiedHead+"..HEAD")
	if err != nil {
		return gitDiffResult{}, err
	}

	parts := strings.Split(string(out), "\x00")
	seen := make(map[string]bool)
	var changed []string
	for _, p := range parts {
		norm := NormalizeCoverPath(p)
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		changed = append(changed, norm)
	}
	sort.Strings(changed)
	return gitDiffResult{changedFiles: changed}, nil
}

func runGit(repoDir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
