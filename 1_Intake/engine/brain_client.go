package intake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"blackfactory/pkg/bfcore"
)

var (
	// ErrBrainUnavailable reexporta bfcore.ErrBrainUnavailable (Exit code 4 en 1_Intake).
	ErrBrainUnavailable = bfcore.ErrBrainUnavailable
	// ErrBrainIndexMissing reexporta bfcore.ErrBrainIndexMissing (Exit code 4 en 1_Intake).
	ErrBrainIndexMissing = bfcore.ErrBrainIndexMissing
)

// BrainPreflight reexporta bfcore.BrainPreflight para el paquete intake.
type BrainPreflight = bfcore.BrainPreflight

// BrainContextResponse reexporta bfcore.BrainContextResponse para el paquete intake.
type BrainContextResponse = bfcore.BrainContextResponse

// BrainQueryOpts parametriza una consulta a 0_Brain/brain.exe query --json.
type BrainQueryOpts struct {
	Project string
	Type    string
	Query   string
	Limit   int
}

// BrainClient abstrae la comunicacion de solo lectura con 0_Brain embebiendo bfcore.BrainClient
// y anadiendo Query para busqueda FTS5 especifica de triaje.
type BrainClient interface {
	bfcore.BrainClient
	Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error)
}

// SubprocessBrainClient envuelve *bfcore.SubprocessBrainClient en un struct local para
// implementar el metodo Query exclusivo de 1_Intake reutilizando ExecBrain.
type SubprocessBrainClient struct {
	*bfcore.SubprocessBrainClient
}

// NewSubprocessBrainClient construye un SubprocessBrainClient con timeout por defecto de 5s.
func NewSubprocessBrainClient(brainBin string, timeout time.Duration) *SubprocessBrainClient {
	return &SubprocessBrainClient{
		SubprocessBrainClient: bfcore.NewSubprocessBrainClient(brainBin, timeout),
	}
}

// Query invoca 0_Brain/brain.exe query --vault <v> --db <db> [--project <p>] [--type <t>] [--limit <n>] [--q <q>] --json.
func (c *SubprocessBrainClient) Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error) {
	if c == nil || c.SubprocessBrainClient == nil {
		return nil, BrainInvocation{ExitCode: -1}, fmt.Errorf("%w: cliente brain nil", ErrBrainUnavailable)
	}
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(vaultRoot, ".index", "brain.db")
	}
	args := []string{"query", "--vault", vaultRoot, "--db", dbPath}
	if strings.TrimSpace(opts.Project) != "" {
		args = append(args, "--project", strings.TrimSpace(opts.Project))
	}
	if strings.TrimSpace(opts.Type) != "" {
		args = append(args, "--type", strings.TrimSpace(opts.Type))
	}
	if opts.Limit > 0 {
		args = append(args, "--limit", strconv.Itoa(opts.Limit))
	}
	if strings.TrimSpace(opts.Query) != "" {
		args = append(args, "--q", strings.TrimSpace(opts.Query))
	}
	args = append(args, "--json")

	outBytes, inv, err := c.ExecBrain(args)
	if err != nil {
		return nil, inv, err
	}

	var docs []BrainDocMatch
	trimmed := bytes.TrimSpace(outBytes)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &docs); err != nil {
			return nil, inv, fmt.Errorf("%w: decodificando salida de brain query: %v", ErrBrainUnavailable, err)
		}
	}
	if docs == nil {
		docs = []BrainDocMatch{}
	}
	inv.ResultCount = len(docs)
	return docs, inv, nil
}

var (
	reCatalogCode    = regexp.MustCompile(`(?i)^(?:RZ-\d+|ADR-\d+|G[1-6])$`)
	reFileNameExt    = regexp.MustCompile(`(?i)\.(?:go|md|json|sql|yaml|yml|ps1|py|sh)$`)
	reRedactedMarker = regexp.MustCompile(`\[REDACTED:G4:[A-Za-z0-9_]+\]`)

	stopWords = map[string]bool{
		"and": true, "or": true, "not": true, "near": true,
		"the": true, "for": true, "with": true, "from": true, "that": true,
		"this": true, "into": true, "when": true, "where": true, "check": true,
		"para": true, "como": true, "desde": true, "sobre": true, "entre": true,
		"este": true, "esta": true, "estos": true, "estas": true, "pero": true,
		"error": true, "issue": true, "bug": true, "fix": true,
	}
)

// ExtractQueryTokens extrae hasta maxTokens (default 5) tokens tecnicos de alta especificidad
// desde title y description (rutas, nombres de archivo, codigos RZ-*/ADR-*/G1..G6, identificadores
// CamelCase/snake_case y palabras alfanumericas >= 4 caracteres), filtrando stop-words y operadores FTS5 (AND, OR, NOT, NEAR).
func ExtractQueryTokens(title, description string, maxTokens int) []string {
	if maxTokens <= 0 {
		maxTokens = 5
	}

	type scoredToken struct {
		token string
		prio  int
		order int
	}

	cleanedText := reRedactedMarker.ReplaceAllString(title+" "+description, " ")
	rawWords := strings.Fields(cleanedText)
	seen := make(map[string]bool)
	var candidates []scoredToken

	for idx, w := range rawWords {
		clean := trimNonTokenPunct(w)
		if clean == "" {
			continue
		}
		low := strings.ToLower(clean)
		if stopWords[low] {
			continue
		}

		// Si es una ruta con '/', extraer el nombre base del archivo
		if strings.ContainsAny(clean, `/\`) {
			norm := strings.ReplaceAll(clean, `\`, "/")
			base := filepath.Base(norm)
			baseClean := trimNonTokenPunct(base)
			if baseClean != "" && !stopWords[strings.ToLower(baseClean)] {
				clean = baseClean
				low = strings.ToLower(clean)
			}
		}

		prio := classifyTokenPriority(clean)
		if prio == 0 {
			continue
		}
		if !seen[low] {
			seen[low] = true
			candidates = append(candidates, scoredToken{token: low, prio: prio, order: idx})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].prio != candidates[j].prio {
			return candidates[i].prio > candidates[j].prio
		}
		return candidates[i].order < candidates[j].order
	})

	if len(candidates) > maxTokens {
		candidates = candidates[:maxTokens]
	}
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.token
	}
	return out
}

func trimNonTokenPunct(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '.' || r == '-' || r == '/' || r == '\\' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._-/\\")
}

func classifyTokenPriority(tok string) int {
	if reCatalogCode.MatchString(tok) {
		return 4
	}
	if reFileNameExt.MatchString(tok) {
		return 4
	}
	if strings.Contains(tok, "_") && len(tok) >= 4 {
		return 3
	}
	hasUpper := false
	hasLower := false
	for _, r := range tok {
		if unicode.IsUpper(r) {
			hasUpper = true
		} else if unicode.IsLower(r) {
			hasLower = true
		}
	}
	if hasUpper && hasLower && len(tok) >= 4 {
		return 3
	}
	if len(tok) >= 4 {
		return 1
	}
	return 0
}

// BuildDisjunctiveFTSQuery envuelve cada token sanitizado entre comillas dobles y los une con " OR "
// para que 0_Brain ejecute una consulta FTS5 disyuntiva con ranking BM25.
func BuildDisjunctiveFTSQuery(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		clean := strings.TrimSpace(strings.ReplaceAll(tok, `"`, ""))
		if clean == "" {
			continue
		}
		up := strings.ToUpper(clean)
		if up == "AND" || up == "OR" || up == "NOT" || up == "NEAR" {
			continue
		}
		parts = append(parts, `"`+clean+`"`)
	}
	return strings.Join(parts, " OR ")
}

// IsProjectActiveOnDisk comprueba en <1ms si <vaultRoot>/projects/<project>.md existe en disco
// (sin reparse points) y declara type: project y status: active en su frontmatter YAML.
func IsProjectActiveOnDisk(vaultRoot, project string) bool {
	slug := strings.ToLower(strings.TrimSpace(project))
	if slug == "" || strings.Contains(slug, "..") || strings.ContainsAny(slug, `/\:`) {
		return false
	}
	projFile := filepath.Join(vaultRoot, "projects", slug+".md")
	if err := bfcore.CheckNoReparseChain(vaultRoot, projFile); err != nil {
		return false
	}
	fi, err := os.Lstat(projFile)
	if err != nil || fi.IsDir() || bfcore.IsReparseOrSymlink(fi) {
		return false
	}
	raw, err := os.ReadFile(projFile)
	if err != nil {
		return false
	}
	content := strings.ReplaceAll(strings.TrimPrefix(string(raw), "\xef\xbb\xbf"), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return false
	}
	hasTypeProject := false
	hasStatusActive := false
	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" || t == "..." {
			break
		}
		k, v, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.Trim(strings.TrimSpace(v), `"'`)
		if key == "type" && strings.EqualFold(val, "project") {
			hasTypeProject = true
		}
		if key == "status" && strings.EqualFold(val, "active") {
			hasStatusActive = true
		}
	}
	return hasTypeProject && hasStatusActive
}
