package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"blackfactory/pkg/bfcore"
)

var (
	ErrDiffTooLarge        = errors.New("el diff excede MaxDiffBytes")
	ErrForbiddenGitEntry   = errors.New("entrada Git prohibida (symlink 120000 o submodulo 160000)")
	ErrObservedFileTooBig  = errors.New("archivo excede MaxObservedFileBytes")
	ErrRegularFileRequired = errors.New("ruta cambiada no es archivo regular")
)

// GitClient abstrae las operaciones Git necesarias para worktrees, snapshots e inspeccion.
type GitClient interface {
	CheckBranchFormat(ctx context.Context, repoDir, branch string) error
	BranchExists(ctx context.Context, repoDir, branch string) (bool, error)
	ListWorktreeBranches(ctx context.Context, repoDir string) ([]string, error)
	RevParse(ctx context.Context, repoDir, ref string) (string, error)
	LsTreePaths(ctx context.Context, repoDir, treeRef string) (map[string]bool, error)
	WorktreeAdd(ctx context.Context, repoDir, worktreeDir, branch, baseRef string) error
	WorktreeRemove(ctx context.Context, repoDir, worktreeDir string, force bool) error
	WorktreePrune(ctx context.Context, repoDir string) error
	DeleteBranch(ctx context.Context, repoDir, branch string) error
	StatusPorcelainZ(ctx context.Context, dir string) ([]byte, error)
	DiffRawZ(ctx context.Context, dir, baseRef string) ([]byte, error)
	DiffUnified(ctx context.Context, dir, baseRef string) (string, error)
	StageModes(ctx context.Context, dir string) (map[string]string, error)
}

// ExecGitClient implementa GitClient invocando el binario git del sistema.
type ExecGitClient struct{}

func (ExecGitClient) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, bfcore.RedactAllG4(strings.TrimSpace(stderr.String())))
	}
	return out, nil
}

func (g ExecGitClient) CheckBranchFormat(ctx context.Context, repoDir, branch string) error {
	_, err := g.run(ctx, repoDir, "check-ref-format", "--branch", branch)
	return err
}

func (g ExecGitClient) BranchExists(ctx context.Context, repoDir, branch string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func (g ExecGitClient) ListWorktreeBranches(ctx context.Context, repoDir string) ([]string, error) {
	out, err := g.run(ctx, repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "branch refs/heads/") {
			branches = append(branches, strings.TrimPrefix(line, "branch refs/heads/"))
		}
	}
	return branches, nil
}

func (g ExecGitClient) RevParse(ctx context.Context, repoDir, ref string) (string, error) {
	out, err := g.run(ctx, repoDir, "rev-parse", "--verify", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g ExecGitClient) LsTreePaths(ctx context.Context, repoDir, treeRef string) (map[string]bool, error) {
	out, err := g.run(ctx, repoDir, "ls-tree", "-r", "--name-only", "-z", treeRef)
	if err != nil {
		return nil, err
	}
	res := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			res[p] = true
		}
	}
	return res, nil
}

func (g ExecGitClient) WorktreeAdd(ctx context.Context, repoDir, worktreeDir, branch, baseRef string) error {
	_, err := g.run(ctx, repoDir, "worktree", "add", "-b", branch, worktreeDir, baseRef)
	return err
}

func (g ExecGitClient) WorktreeRemove(ctx context.Context, repoDir, worktreeDir string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreeDir)
	_, err := g.run(ctx, repoDir, args...)
	return err
}

func (g ExecGitClient) WorktreePrune(ctx context.Context, repoDir string) error {
	_, err := g.run(ctx, repoDir, "worktree", "prune")
	return err
}

func (g ExecGitClient) DeleteBranch(ctx context.Context, repoDir, branch string) error {
	_, err := g.run(ctx, repoDir, "branch", "-D", branch)
	return err
}

func (g ExecGitClient) StatusPorcelainZ(ctx context.Context, dir string) ([]byte, error) {
	return g.run(ctx, dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
}

func (g ExecGitClient) DiffRawZ(ctx context.Context, dir, baseRef string) ([]byte, error) {
	return g.run(ctx, dir, "diff", "--raw", "-z", baseRef)
}

func (g ExecGitClient) DiffUnified(ctx context.Context, dir, baseRef string) (string, error) {
	out, err := g.run(ctx, dir, "diff", "--no-ext-diff", "--binary", "-U3", baseRef)
	return string(out), err
}

func (g ExecGitClient) StageModes(ctx context.Context, dir string) (map[string]string, error) {
	out, err := g.run(ctx, dir, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	modes := map[string]string{}
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, pathPart, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) >= 1 {
			modes[pathPart] = fields[0]
		}
	}
	return modes, nil
}

// PorcelainEntry representa una entrada parseada de git status --porcelain=v1 -z.
type PorcelainEntry struct {
	XY       string
	Path     string
	OrigPath string
}

// ParsePorcelainZEntries parsea la salida NUL-terminada de git status --porcelain=v1 -z.
func ParsePorcelainZEntries(b []byte) []PorcelainEntry {
	parts := strings.Split(string(b), "\x00")
	var entries []PorcelainEntry
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		xy := p[:2]
		path := p[3:]
		orig := ""
		if strings.ContainsAny(xy, "RC") && i+1 < len(parts) {
			i++
			orig = parts[i]
		}
		entries = append(entries, PorcelainEntry{XY: xy, Path: path, OrigPath: orig})
	}
	return entries
}

// WorktreeSnapshot agrupa el estado crudo completo y diff del worktree respecto a baseRef.
type WorktreeSnapshot struct {
	BaseRef            string
	WorktreeTreeSHA256 string
	ChangedFiles       []ChangedFileEntry
	DiffText           string
	DiffBytes          int64
}

// ComputeWorktreeSnapshot valida reparse safety, rechaza modos 120000/160000, calcula hashes crudos
// + modos octales y construye el diff unificado completo (sin mutar .git/index).
func ComputeWorktreeSnapshot(ctx context.Context, gc GitClient, repoRoot, worktreePath, baseRef string) (WorktreeSnapshot, error) {
	if err := bfcore.CheckNoReparseChain("", repoRoot); err != nil {
		return WorktreeSnapshot{}, err
	}
	if err := bfcore.CheckNoReparseChain(repoRoot, filepath.Join(repoRoot, ".git")); err != nil {
		return WorktreeSnapshot{}, err
	}
	if err := bfcore.CheckNoReparseChain("", worktreePath); err != nil {
		return WorktreeSnapshot{}, err
	}
	if err := bfcore.CheckNoReparseChain(worktreePath, filepath.Join(worktreePath, ".git")); err != nil {
		return WorktreeSnapshot{}, err
	}

	rawModes, err := checkDiffRawForbiddenModes(ctx, gc, worktreePath, baseRef)
	if err != nil {
		return WorktreeSnapshot{}, err
	}
	stageModes, err := gc.StageModes(ctx, worktreePath)
	if err != nil {
		return WorktreeSnapshot{}, err
	}
	for _, m := range stageModes {
		if m == "120000" || m == "160000" {
			return WorktreeSnapshot{}, fmt.Errorf("%w: modo %s en indice", ErrForbiddenGitEntry, m)
		}
	}

	porcelainBytes, err := gc.StatusPorcelainZ(ctx, worktreePath)
	if err != nil {
		return WorktreeSnapshot{}, err
	}
	entries := ParsePorcelainZEntries(porcelainBytes)

	type changeKey struct {
		path   string
		status string
	}
	byKey := map[changeKey]ChangedFileEntry{}
	var untracked []string

	addDeleted := func(rel string) {
		rel = strings.ReplaceAll(rel, `\`, "/")
		k := changeKey{path: rel, status: "deleted"}
		byKey[k] = ChangedFileEntry{
			Path:   rel,
			Status: "deleted",
			Mode:   "0000",
			SHA256: "DELETED",
		}
	}

	addPresent := func(rel, status string) error {
		rel = strings.ReplaceAll(rel, `\`, "/")
		abs := filepath.Join(worktreePath, filepath.FromSlash(rel))
		if err := bfcore.CheckNoReparseChain(worktreePath, abs); err != nil {
			return err
		}
		fi, err := os.Lstat(abs)
		if err != nil {
			return err
		}
		if bfcore.IsReparseOrSymlink(fi) {
			return fmt.Errorf("%w: %s", bfcore.ErrReparsePoint, abs)
		}
		if fi.IsDir() || !fi.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrRegularFileRequired, rel)
		}
		if fi.Size() > bfcore.MaxObservedFileBytes {
			return fmt.Errorf("%w: %s (%d bytes)", ErrObservedFileTooBig, rel, fi.Size())
		}
		shaHex, err := hashRawFileStreaming(abs)
		if err != nil {
			return err
		}
		mode := "0644"
		if m, ok := rawModes[rel]; ok {
			if m == "100755" {
				mode = "0755"
			}
		} else if sm, ok := stageModes[rel]; ok {
			if sm == "100755" {
				mode = "0755"
			}
		} else if fi.Mode()&0o111 != 0 {
			mode = "0755"
		}
		byKey[changeKey{path: rel, status: status}] = ChangedFileEntry{
			Path:   rel,
			Status: status,
			Mode:   mode,
			SHA256: shaHex,
		}
		return nil
	}

	untrackedModes := map[string]string{}
	for _, e := range entries {
		switch {
		case e.XY == "??":
			if err := addPresent(e.Path, "added"); err != nil {
				return WorktreeSnapshot{}, err
			}
			rel := strings.ReplaceAll(e.Path, `\`, "/")
			untracked = append(untracked, rel)
			untrackedModes[rel] = byKey[changeKey{path: rel, status: "added"}].Mode
		case e.XY[0] == 'D' || e.XY[1] == 'D':
			addDeleted(e.Path)
			if e.OrigPath != "" {
				addDeleted(e.OrigPath)
			}
		case strings.ContainsAny(e.XY, "R"):
			if err := addPresent(e.Path, "renamed"); err != nil {
				return WorktreeSnapshot{}, err
			}
			if e.OrigPath != "" {
				addDeleted(e.OrigPath)
			}
		case strings.ContainsAny(e.XY, "CA"):
			if err := addPresent(e.Path, "added"); err != nil {
				return WorktreeSnapshot{}, err
			}
		default:
			if err := addPresent(e.Path, "modified"); err != nil {
				return WorktreeSnapshot{}, err
			}
		}
	}

	changed := make([]ChangedFileEntry, 0, len(byKey))
	for _, entry := range byKey {
		changed = append(changed, entry)
	}
	sort.Slice(changed, func(i, j int) bool {
		if changed[i].Path != changed[j].Path {
			return changed[i].Path < changed[j].Path
		}
		return changed[i].Status < changed[j].Status
	})

	var treeBuf strings.Builder
	treeBuf.WriteString(baseRef)
	treeBuf.WriteByte('\n')
	for _, c := range changed {
		fmt.Fprintf(&treeBuf, "%s\t%s\t%s\t%s\n", c.Status, c.Mode, c.Path, c.SHA256)
	}
	treeSum := sha256.Sum256([]byte(treeBuf.String()))

	diffText, err := gc.DiffUnified(ctx, worktreePath, baseRef)
	if err != nil {
		return WorktreeSnapshot{}, err
	}
	sort.Strings(untracked)
	var fullDiff strings.Builder
	fullDiff.WriteString(diffText)
	for _, rel := range untracked {
		if fullDiff.Len() > 0 && !strings.HasSuffix(fullDiff.String(), "\n") {
			fullDiff.WriteByte('\n')
		}
		hunk, err := buildUntrackedUnifiedDiff(worktreePath, rel, untrackedModes[rel])
		if err != nil {
			return WorktreeSnapshot{}, err
		}
		fullDiff.WriteString(hunk)
		if fullDiff.Len() > MaxDiffBytes {
			return WorktreeSnapshot{}, fmt.Errorf("%w: excede %d bytes", ErrDiffTooLarge, MaxDiffBytes)
		}
	}
	if fullDiff.Len() > MaxDiffBytes {
		return WorktreeSnapshot{}, fmt.Errorf("%w: excede %d bytes", ErrDiffTooLarge, MaxDiffBytes)
	}

	return WorktreeSnapshot{
		BaseRef:            baseRef,
		WorktreeTreeSHA256: hex.EncodeToString(treeSum[:]),
		ChangedFiles:       changed,
		DiffText:           fullDiff.String(),
		DiffBytes:          int64(fullDiff.Len()),
	}, nil
}

func checkDiffRawForbiddenModes(ctx context.Context, gc GitClient, worktreePath, baseRef string) (map[string]string, error) {
	raw, err := gc.DiffRawZ(ctx, worktreePath, baseRef)
	if err != nil {
		return nil, err
	}
	modes := map[string]string{}
	tokens := strings.Split(string(raw), "\x00")
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if !strings.HasPrefix(tok, ":") {
			continue
		}
		fields := strings.Fields(tok[1:])
		if len(fields) < 5 {
			continue
		}
		oldMode, newMode := fields[0], fields[1]
		if oldMode == "120000" || oldMode == "160000" || newMode == "120000" || newMode == "160000" {
			return nil, fmt.Errorf("%w: modo %s/%s", ErrForbiddenGitEntry, oldMode, newMode)
		}
		if i+1 < len(tokens) {
			i++
			p := tokens[i]
			if strings.HasPrefix(fields[4], "R") || strings.HasPrefix(fields[4], "C") {
				if i+1 < len(tokens) {
					i++
					p = tokens[i]
				}
			}
			modes[p] = newMode
		}
	}
	return modes, nil
}

func hashRawFileStreaming(absPath string) (string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func buildUntrackedUnifiedDiff(worktreePath, rel, mode string) (string, error) {
	abs := filepath.Join(worktreePath, filepath.FromSlash(rel))
	raw, err := ReadFileWithShortRetry(abs)
	if err != nil {
		return "", err
	}
	gitMode := "100644"
	if mode == "0755" {
		gitMode = "100755"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", rel, rel)
	fmt.Fprintf(&b, "new file mode %s\n", gitMode)
	if decoded, isU16 := decodeUTF16BOMIfPresent(raw); isU16 {
		writeAddedLinesHunk(&b, rel, decoded)
		return b.String(), nil
	}
	if bytes.IndexByte(raw, 0x00) >= 0 {
		fmt.Fprintf(&b, "Binary files /dev/null and b/%s differ\n", rel)
		return b.String(), nil
	}
	writeAddedLinesHunk(&b, rel, string(raw))
	return b.String(), nil
}

func writeAddedLinesHunk(b *strings.Builder, rel, text string) {
	norm := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if norm == "" {
		return
	}
	lines := strings.Split(strings.TrimSuffix(norm, "\n"), "\n")
	fmt.Fprintf(b, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", rel, len(lines))
	for _, ln := range lines {
		b.WriteByte('+')
		b.WriteString(ln)
		b.WriteByte('\n')
	}
}

func decodeUTF16BOMIfPresent(raw []byte) (string, bool) {
	if len(raw) < 2 {
		return "", false
	}
	var order binary.ByteOrder
	switch {
	case raw[0] == 0xFF && raw[1] == 0xFE:
		order = binary.LittleEndian
	case raw[0] == 0xFE && raw[1] == 0xFF:
		order = binary.BigEndian
	default:
		return "", false
	}
	body := raw[2:]
	if len(body)%2 != 0 {
		body = body[:len(body)-1]
	}
	u16 := make([]uint16, len(body)/2)
	for i := range u16 {
		u16[i] = order.Uint16(body[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16)), true
}

// ScanWorktreeChangesG4 valida G4 (11 reglas fail-closed en rutas, lineas '+', archivos UTF-16
// y archivos nuevos) y redacta Tier 2 unicamente en lineas de contexto (' ') y borradas ('-')
// del diff persistido tras rechazar cualquier secreto Tier 1.
func ScanWorktreeChangesG4(worktreePath string, snap WorktreeSnapshot) (cleanDiff string, finding *bfcore.G4Finding, pointer string, err error) {
	for _, cf := range snap.ChangedFiles {
		if hits := bfcore.ScanG4All(cf.Path); len(hits) > 0 {
			f := hits[0]
			return "", &f, "/diff/path", nil
		}
		if cf.Status == "deleted" {
			continue
		}
		abs := filepath.Join(worktreePath, filepath.FromSlash(cf.Path))
		raw, rErr := ReadFileWithShortRetry(abs)
		if rErr != nil {
			return "", nil, "", rErr
		}
		if decoded, isU16 := decodeUTF16BOMIfPresent(raw); isU16 {
			if hits := bfcore.ScanG4All(decoded); len(hits) > 0 {
				f := hits[0]
				return "", &f, "/diff/" + cf.Path, nil
			}
			continue
		}
		if cf.Status == "added" && bytes.IndexByte(raw, 0x00) < 0 {
			if hits := bfcore.ScanG4All(string(raw)); len(hits) > 0 {
				f := hits[0]
				return "", &f, "/diff/" + cf.Path, nil
			}
		}
	}

	if hits := bfcore.ScanTier1("diff", snap.DiffText, false); len(hits) > 0 {
		f := hits[0]
		return "", &f, "/diff", nil
	}

	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(snap.DiffText, "\r\n", "\n"), "\r", "\n"), "\n")
	currentFile := ""
	inHunk := false
	var outLines []string

	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "diff --git "):
			inHunk = false
			currentFile = ""
			if idx := strings.Index(ln, " b/"); idx >= 0 {
				currentFile = ln[idx+3:]
			}
			outLines = append(outLines, ln)
		case strings.HasPrefix(ln, "+++ b/"):
			currentFile = strings.TrimPrefix(ln, "+++ b/")
			outLines = append(outLines, ln)
		case strings.HasPrefix(ln, "--- "):
			outLines = append(outLines, ln)
		case strings.HasPrefix(ln, "@@ "):
			inHunk = true
			outLines = append(outLines, ln)
		case inHunk && strings.HasPrefix(ln, "+"):
			if hits := bfcore.ScanG4All(ln[1:]); len(hits) > 0 {
				f := hits[0]
				ptr := "/diff"
				if currentFile != "" {
					ptr = "/diff/" + currentFile
				}
				return "", &f, ptr, nil
			}
			outLines = append(outLines, ln)
		case inHunk && (strings.HasPrefix(ln, "-") || strings.HasPrefix(ln, " ")):
			prefix := ln[:1]
			redacted, _ := bfcore.RedactTier2Text("diff_context", ln[1:])
			outLines = append(outLines, prefix+redacted)
		default:
			if strings.HasPrefix(ln, "rename from ") || strings.HasPrefix(ln, "rename to ") || strings.HasPrefix(ln, "copy from ") || strings.HasPrefix(ln, "copy to ") {
				if hits := bfcore.ScanG4All(ln); len(hits) > 0 {
					f := hits[0]
					return "", &f, "/diff/path", nil
				}
			}
			outLines = append(outLines, ln)
		}
	}

	finalDiff := strings.Join(outLines, "\n")
	if hits := bfcore.ScanG4All(finalDiff); len(hits) > 0 {
		f := hits[0]
		return "", &f, "/diff", nil
	}
	return finalDiff, nil, "", nil
}
