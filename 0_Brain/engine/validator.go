package brain

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"blackfactory/pkg/bfcore"
)

// IssueCode identifica la categoria de un hallazgo de validacion en la boveda.
type IssueCode string

const (
	IssueMissingCoreFile      IssueCode = "MISSING_CORE_FILE"
	IssueMissingFrontmatter   IssueCode = "MISSING_FRONTMATTER"
	IssueMalformedFrontmatter IssueCode = "MALFORMED_FRONTMATTER"
	IssueMissingField         IssueCode = "MISSING_FIELD"
	IssueDuplicateID          IssueCode = "DUPLICATE_ID"
	IssueBrokenLink           IssueCode = "BROKEN_LINK"
	IssueG4Violation          IssueCode = "G4_VIOLATION"
)

// ValidationIssue describe un incumplimiento puntual detectado en la boveda.
type ValidationIssue struct {
	Code    IssueCode `json:"code"`
	File    string    `json:"file"`
	Line    int       `json:"line,omitempty"`
	Message string    `json:"message"`
}

// ValidationReport resume el resultado determinista de ValidateVault.
type ValidationReport struct {
	VaultRoot    string            `json:"vault_root"`
	OK           bool              `json:"ok"`
	FilesChecked int               `json:"files_checked"`
	DocsParsed   int               `json:"docs_parsed"`
	Issues       []ValidationIssue `json:"issues"`
}

var coreVaultFiles = []string{
	"AGENTS.md",
	"index.md",
	"constitution.md",
	"log.md",
}

// ValidateDocumentMeta comprueba que un documento OKF posea frontmatter YAML
// y los cuatro campos minimos obligatorios (id, title, type, status).
func ValidateDocumentMeta(doc Document) []ValidationIssue {
	if !doc.HasFrontmatter {
		return []ValidationIssue{{
			Code:    IssueMissingFrontmatter,
			File:    doc.RelPath,
			Line:    1,
			Message: "falta bloque frontmatter YAML obligatorio ('--- ... ---')",
		}}
	}

	required := []struct {
		name string
		val  string
	}{
		{"id", doc.Meta.ID},
		{"title", doc.Meta.Title},
		{"type", doc.Meta.Type},
		{"status", doc.Meta.Status},
	}

	var issues []ValidationIssue
	for _, fld := range required {
		if strings.TrimSpace(fld.val) == "" {
			issues = append(issues, ValidationIssue{
				Code:    IssueMissingField,
				File:    doc.RelPath,
				Line:    1,
				Message: fmt.Sprintf("campo obligatorio %q ausente o vacio en frontmatter", fld.name),
			})
		}
	}
	return issues
}

// ScanG4 ejecuta un escaneo determinista linea por linea contra patrones G4
// delegando en pkg/bfcore.ScanG4All sin transcribir el valor sensible encontrado.
func ScanG4(relPath, content string) []ValidationIssue {
	normFile := NormalizeRelPath(relPath)
	findings := bfcore.ScanG4All(normalizeMarkdownContent(content))
	if len(findings) == 0 {
		return nil
	}
	issues := make([]ValidationIssue, 0, len(findings))
	for _, f := range findings {
		issues = append(issues, ValidationIssue{
			Code:    IssueG4Violation,
			File:    normFile,
			Line:    f.Line,
			Message: f.Message,
		})
	}
	return issues
}

// ValidateVault valida integralmente una boveda OKF en root: archivos nucleo,
// frontmatter y campos minimos, unicidad de id, enlaces relativos internos y G4.
func ValidateVault(root string) (ValidationReport, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ValidationReport{}, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return ValidationReport{}, fmt.Errorf("accediendo a boveda %s: %w", absRoot, err)
	}
	if !info.IsDir() {
		return ValidationReport{}, fmt.Errorf("%s no es un directorio", absRoot)
	}

	report := ValidationReport{
		VaultRoot: absRoot,
	}

	for _, req := range coreVaultFiles {
		p := filepath.Join(absRoot, filepath.FromSlash(req))
		fi, statErr := os.Stat(p)
		if statErr != nil || fi.IsDir() {
			report.Issues = append(report.Issues, ValidationIssue{
				Code:    IssueMissingCoreFile,
				File:    req,
				Message: fmt.Sprintf("archivo nucleo obligatorio %q ausente en la boveda", req),
			})
		}
	}

	files, err := ListVaultMarkdownFiles(absRoot)
	if err != nil {
		return ValidationReport{}, err
	}

	seenIDs := make(map[string]string)

	for _, absPath := range files {
		rel, err := filepath.Rel(absRoot, absPath)
		if err != nil {
			return ValidationReport{}, err
		}
		relPath := NormalizeRelPath(rel)
		report.FilesChecked++

		rawBytes, err := os.ReadFile(absPath)
		if err != nil {
			return ValidationReport{}, fmt.Errorf("leyendo %s: %w", absPath, err)
		}
		rawContent := string(rawBytes)

		report.Issues = append(report.Issues, ScanG4(relPath, rawContent)...)

		doc, parseErr := ParseDocument(relPath, rawContent)
		if parseErr != nil {
			report.Issues = append(report.Issues, ValidationIssue{
				Code:    IssueMalformedFrontmatter,
				File:    relPath,
				Line:    1,
				Message: parseErr.Error(),
			})
			continue
		}
		doc.AbsPath = absPath
		report.DocsParsed++

		if doc.HasFrontmatter || !isOptionalFrontmatterFile(relPath) {
			report.Issues = append(report.Issues, ValidateDocumentMeta(doc)...)
		}

		if idKey := strings.ToLower(strings.TrimSpace(doc.Meta.ID)); idKey != "" {
			if prevFile, dup := seenIDs[idKey]; dup {
				report.Issues = append(report.Issues, ValidationIssue{
					Code:    IssueDuplicateID,
					File:    relPath,
					Line:    1,
					Message: fmt.Sprintf("id duplicado %q (ya declarado en %s)", doc.Meta.ID, prevFile),
				})
			} else {
				seenIDs[idKey] = relPath
			}
		}

		docDir := filepath.Dir(absPath)
		for _, lnk := range doc.Links {
			if !resolveAndCheckLink(absRoot, docDir, lnk.Path) {
				report.Issues = append(report.Issues, ValidationIssue{
					Code:    IssueBrokenLink,
					File:    relPath,
					Line:    lnk.Line,
					Message: fmt.Sprintf("enlace interno roto hacia %q", lnk.Target),
				})
			}
		}
	}

	sort.Slice(report.Issues, func(i, j int) bool {
		a, b := report.Issues[i], report.Issues[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})

	report.OK = len(report.Issues) == 0
	return report, nil
}

func isOptionalFrontmatterFile(relPath string) bool {
	if strings.Contains(relPath, "/") {
		return false
	}
	switch strings.ToLower(relPath) {
	case "log.md", "readme.md", "readme_privado.md", "agents.md":
		return true
	default:
		return false
	}
}

func resolveAndCheckLink(absRoot, docDir, linkPath string) bool {
	candidates := []string{linkPath}
	if unescaped, err := url.PathUnescape(linkPath); err == nil && unescaped != linkPath {
		candidates = append(candidates, unescaped)
	}

	for _, cand := range candidates {
		cleaned := filepath.FromSlash(strings.TrimSpace(cand))
		if cleaned == "" {
			continue
		}
		var resolved string
		if filepath.IsAbs(cleaned) {
			return false
		}
		if strings.HasPrefix(cleaned, string(filepath.Separator)) {
			resolved = filepath.Clean(filepath.Join(absRoot, strings.TrimLeft(cleaned, `\/`)))
		} else {
			resolved = filepath.Clean(filepath.Join(docDir, cleaned))
		}

		rel, err := filepath.Rel(absRoot, resolved)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if _, statErr := os.Stat(resolved); statErr == nil {
			return true
		}
	}
	return false
}
