package brain

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ErrMalformedFrontmatter indica que el encabezado YAML inicial del documento OKF
// no tiene cierre valido o contiene lineas con sintaxis incompatible.
var ErrMalformedFrontmatter = errors.New("frontmatter YAML malformado")

// Frontmatter representa los metadatos estructurados OKF de un documento Markdown.
type Frontmatter struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Type         string            `json:"type"`
	Project      string            `json:"project"`
	Status       string            `json:"status"`
	Tags         []string          `json:"tags"`
	CoversPaths  []string          `json:"covers_paths"`
	VerifiedHead string            `json:"verified_head"`
	UpdatedAt    string            `json:"updated_at"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// Link representa un enlace Markdown relativo extraido fuera de bloques fenced y codigo inline.
type Link struct {
	Text   string `json:"text"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Anchor string `json:"anchor,omitempty"`
	Line   int    `json:"line"`
}

// Document encapsula un archivo Markdown OKF ya parseado.
type Document struct {
	RelPath        string      `json:"rel_path"`
	AbsPath        string      `json:"abs_path"`
	HasFrontmatter bool        `json:"has_frontmatter"`
	Meta           Frontmatter `json:"meta"`
	Body           string      `json:"body"`
	Raw            string      `json:"-"`
	BodyStartLine  int         `json:"-"`
	Links          []Link      `json:"links,omitempty"`
}

var (
	reMDLink     = regexp.MustCompile(`\[([^\[\]]+)\]\(([^()\r\n]+)\)`)
	reInlineCode = regexp.MustCompile("`+[^`]*`+")
)

// NormalizeRelPath unifica separadores de ruta a "/" y limpia segmentos redundantes.
func NormalizeRelPath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	slash := strings.ReplaceAll(trimmed, `\`, "/")
	cleaned := path.Clean(slash)
	cleaned = strings.TrimPrefix(cleaned, "./")
	return cleaned
}

// ParseFile lee un archivo Markdown del disco relativo a root y lo parsea como Document.
func ParseFile(root, absPath string) (Document, error) {
	rawBytes, err := os.ReadFile(absPath)
	if err != nil {
		return Document{}, fmt.Errorf("leyendo %s: %w", absPath, err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Document{}, err
	}
	absFile, err := filepath.Abs(absPath)
	if err != nil {
		return Document{}, err
	}
	rel, err := filepath.Rel(absRoot, absFile)
	if err != nil {
		return Document{}, err
	}
	doc, err := ParseDocument(NormalizeRelPath(rel), string(rawBytes))
	if err != nil {
		return Document{}, err
	}
	doc.AbsPath = absFile
	return doc, nil
}

// ParseDocument parsea un documento OKF extrayendo su frontmatter YAML plano,
// el cuerpo Markdown y los enlaces relativos.
func ParseDocument(relPath, content string) (Document, error) {
	normalized := normalizeMarkdownContent(content)
	doc := Document{
		RelPath: NormalizeRelPath(relPath),
		Raw:     normalized,
	}

	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		doc.HasFrontmatter = false
		doc.Body = normalized
		doc.BodyStartLine = 1
		doc.Links = ExtractRelativeLinks(doc.Body, doc.BodyStartLine)
		return doc, nil
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" || t == "..." {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return Document{}, fmt.Errorf("%w: falta linea de cierre '---' en %s", ErrMalformedFrontmatter, doc.RelPath)
	}

	meta, err := parseYAMLFrontmatter(lines[1:closeIdx])
	if err != nil {
		return Document{}, err
	}

	doc.HasFrontmatter = true
	doc.Meta = meta
	doc.BodyStartLine = closeIdx + 2
	doc.Body = strings.Join(lines[closeIdx+1:], "\n")
	doc.Links = ExtractRelativeLinks(doc.Body, doc.BodyStartLine)
	return doc, nil
}

func normalizeMarkdownContent(content string) string {
	s := strings.TrimPrefix(content, "\xef\xbb\xbf")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func parseYAMLFrontmatter(lines []string) (Frontmatter, error) {
	var fm Frontmatter
	activeListKey := ""

	for i, rawLine := range lines {
		lineNum := i + 2 // linea 1-based en el archivo original (linea 1 es '---')
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if trimmed == "-" || strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "-\t") {
			if activeListKey == "" {
				return Frontmatter{}, fmt.Errorf("%w: elemento de lista sin clave previa en linea %d", ErrMalformedFrontmatter, lineNum)
			}
			itemRaw := strings.TrimSpace(trimmed[1:])
			itemRaw = stripYAMLInlineComment(itemRaw)
			val, err := parseYAMLScalar(itemRaw)
			if err != nil {
				return Frontmatter{}, fmt.Errorf("%w en linea %d: %v", ErrMalformedFrontmatter, lineNum, err)
			}
			appendFrontmatterListItem(&fm, activeListKey, val)
			continue
		}

		colonIdx := strings.IndexByte(trimmed, ':')
		if colonIdx <= 0 {
			return Frontmatter{}, fmt.Errorf("%w: se esperaba 'clave: valor' en linea %d", ErrMalformedFrontmatter, lineNum)
		}

		key := strings.TrimSpace(trimmed[:colonIdx])
		if key == "" || strings.ContainsAny(key, " \t\"'") {
			return Frontmatter{}, fmt.Errorf("%w: clave invalida %q en linea %d", ErrMalformedFrontmatter, key, lineNum)
		}

		rest := strings.TrimSpace(trimmed[colonIdx+1:])
		rest = stripYAMLInlineComment(rest)
		if rest == "" {
			activeListKey = key
			continue
		}

		activeListKey = ""
		if strings.HasPrefix(rest, "[") {
			if !strings.HasSuffix(rest, "]") {
				return Frontmatter{}, fmt.Errorf("%w: lista inline sin cierre ']' en linea %d", ErrMalformedFrontmatter, lineNum)
			}
			items, err := parseYAMLInlineList(rest)
			if err != nil {
				return Frontmatter{}, fmt.Errorf("%w en linea %d: %v", ErrMalformedFrontmatter, lineNum, err)
			}
			setFrontmatterList(&fm, key, items)
			continue
		}

		val, err := parseYAMLScalar(rest)
		if err != nil {
			return Frontmatter{}, fmt.Errorf("%w en linea %d: %v", ErrMalformedFrontmatter, lineNum, err)
		}
		setFrontmatterScalar(&fm, key, val)
	}

	return fm, nil
}

func stripYAMLInlineComment(s string) string {
	inDouble := false
	inSingle := false
	bracketDepth := 0
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inDouble {
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		switch ch {
		case '"':
			inDouble = true
		case '\'':
			inSingle = true
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '#':
			if bracketDepth == 0 && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return strings.TrimSpace(s)
}

func parseYAMLScalar(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if strings.HasPrefix(s, `"`) {
		if len(s) < 2 || !strings.HasSuffix(s, `"`) {
			return "", errors.New("comilla doble sin cierre")
		}
		inner := s[1 : len(s)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner, nil
	}
	if strings.HasPrefix(s, `'`) {
		if len(s) < 2 || !strings.HasSuffix(s, `'`) {
			return "", errors.New("comilla simple sin cierre")
		}
		inner := s[1 : len(s)-1]
		inner = strings.ReplaceAll(inner, `''`, `'`)
		return inner, nil
	}
	return s, nil
}

func parseYAMLInlineList(raw string) ([]string, error) {
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if inner == "" {
		return []string{}, nil
	}

	var items []string
	var cur strings.Builder
	inDouble := false
	inSingle := false
	escaped := false

	flush := func() error {
		token := strings.TrimSpace(cur.String())
		cur.Reset()
		if token == "" {
			return nil
		}
		v, err := parseYAMLScalar(token)
		if err != nil {
			return err
		}
		items = append(items, v)
		return nil
	}

	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if escaped {
			cur.WriteByte(ch)
			escaped = false
			continue
		}
		if inDouble {
			cur.WriteByte(ch)
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inSingle {
			cur.WriteByte(ch)
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		switch ch {
		case '"':
			inDouble = true
			cur.WriteByte(ch)
		case '\'':
			inSingle = true
			cur.WriteByte(ch)
		case ',':
			if err := flush(); err != nil {
				return nil, err
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if inDouble || inSingle {
		return nil, errors.New("comilla sin cierre en lista inline")
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return items, nil
}

func setFrontmatterScalar(fm *Frontmatter, key, val string) {
	switch strings.ToLower(key) {
	case "id":
		fm.ID = val
	case "title":
		fm.Title = val
	case "type":
		fm.Type = val
	case "project":
		fm.Project = val
	case "status":
		fm.Status = val
	case "verified_head":
		fm.VerifiedHead = val
	case "updated_at":
		fm.UpdatedAt = val
	default:
		if fm.Extra == nil {
			fm.Extra = make(map[string]string)
		}
		fm.Extra[key] = val
	}
}

func setFrontmatterList(fm *Frontmatter, key string, items []string) {
	switch strings.ToLower(key) {
	case "tags":
		fm.Tags = items
	case "covers_paths":
		fm.CoversPaths = items
	default:
		if fm.Extra == nil {
			fm.Extra = make(map[string]string)
		}
		fm.Extra[key] = strings.Join(items, ",")
	}
}

func appendFrontmatterListItem(fm *Frontmatter, key, val string) {
	switch strings.ToLower(key) {
	case "tags":
		fm.Tags = append(fm.Tags, val)
	case "covers_paths":
		fm.CoversPaths = append(fm.CoversPaths, val)
	default:
		if fm.Extra == nil {
			fm.Extra = make(map[string]string)
		}
		if prev := fm.Extra[key]; prev != "" {
			fm.Extra[key] = prev + "," + val
		} else {
			fm.Extra[key] = val
		}
	}
}

// ExtractRelativeLinks extrae enlaces relativos Markdown [texto](ruta#ancla) del cuerpo
// comenzando en startLine (1-based), ignorando bloques fenced y codigo inline.
func ExtractRelativeLinks(body string, startLine int) []Link {
	if startLine <= 0 {
		startLine = 1
	}
	lines := strings.Split(normalizeMarkdownContent(body), "\n")
	var links []Link

	var fenceChar byte
	fenceLen := 0

	for idx, rawLine := range lines {
		lineNum := startLine + idx
		if nextChar, nextLen, toggled := updateFenceState(rawLine, fenceChar, fenceLen); toggled {
			fenceChar, fenceLen = nextChar, nextLen
			continue
		}
		if fenceLen > 0 {
			continue
		}

		cleanLine := maskInlineCode(rawLine)
		matches := reMDLink.FindAllStringSubmatchIndex(cleanLine, -1)
		for _, m := range matches {
			startIdx := m[0]
			if startIdx > 0 && cleanLine[startIdx-1] == '!' {
				continue // imagen Markdown ![alt](src)
			}
			text := cleanLine[m[2]:m[3]]
			rawDest := cleanLine[m[4]:m[5]]
			target, linkPath, anchor, ok := parseRelativeDestination(rawDest)
			if !ok {
				continue
			}
			links = append(links, Link{
				Text:   text,
				Target: target,
				Path:   linkPath,
				Anchor: anchor,
				Line:   lineNum,
			})
		}
	}
	return links
}

func updateFenceState(line string, activeChar byte, activeLen int) (byte, int, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return activeChar, activeLen, false
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return activeChar, activeLen, false
	}
	run := 0
	for run < len(trimmed) && trimmed[run] == ch {
		run++
	}
	if run < 3 {
		return activeChar, activeLen, false
	}
	rest := strings.TrimSpace(trimmed[run:])
	if activeLen == 0 {
		if ch == '`' && strings.ContainsRune(rest, '`') {
			return 0, 0, false // codigo inline en una sola linea
		}
		return ch, run, true
	}
	if ch == activeChar && run >= activeLen && rest == "" {
		return 0, 0, true
	}
	return activeChar, activeLen, false
}

func maskInlineCode(line string) string {
	if !strings.ContainsRune(line, '`') {
		return line
	}
	return reInlineCode.ReplaceAllStringFunc(line, func(seg string) string {
		return strings.Repeat(" ", len(seg))
	})
}

func parseRelativeDestination(raw string) (target, linkPath, anchor string, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", "", false
	}
	if strings.HasPrefix(s, "<") {
		if end := strings.IndexByte(s, '>'); end > 1 {
			s = strings.TrimSpace(s[1:end])
		}
	} else if ws := strings.IndexAny(s, " \t"); ws >= 0 {
		s = strings.TrimSpace(s[:ws])
	}

	if s == "" || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "//") {
		return "", "", "", false
	}
	lower := strings.ToLower(s)
	for _, scheme := range []string{"http://", "https://", "mailto:", "tel:", "ftp://", "data:", "javascript:", "irc:"} {
		if strings.HasPrefix(lower, scheme) {
			return "", "", "", false
		}
	}

	target = s
	beforeHash := s
	if hash := strings.IndexByte(s, '#'); hash >= 0 {
		beforeHash = s[:hash]
		anchor = s[hash+1:]
	}
	if q := strings.IndexByte(beforeHash, '?'); q >= 0 {
		beforeHash = beforeHash[:q]
	}
	linkPath = strings.TrimSpace(beforeHash)
	if linkPath == "" {
		return "", "", "", false
	}
	return target, linkPath, anchor, true
}

// ListVaultMarkdownFiles lista los archivos .md de la boveda ordenados por ruta relativa,
// omitiendo directorios ocultos (prefijo "." como .index o .git).
func ListVaultMarkdownFiles(root string) ([]string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s no es un directorio", absRoot)
	}

	type entry struct {
		rel string
		abs string
	}
	var collected []entry
	err = filepath.WalkDir(absRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == absRoot {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(absRoot, p)
		if relErr != nil {
			return relErr
		}
		collected = append(collected, entry{rel: NormalizeRelPath(rel), abs: p})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(collected, func(i, j int) bool {
		return collected[i].rel < collected[j].rel
	})
	out := make([]string, len(collected))
	for i, e := range collected {
		out[i] = e.abs
	}
	return out, nil
}

// LoadVaultDocs carga y parsea todos los archivos Markdown de la boveda.
func LoadVaultDocs(root string) ([]Document, error) {
	files, err := ListVaultMarkdownFiles(root)
	if err != nil {
		return nil, err
	}
	docs := make([]Document, 0, len(files))
	for _, abs := range files {
		doc, err := ParseFile(root, abs)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
