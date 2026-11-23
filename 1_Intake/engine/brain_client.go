package intake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	// ErrBrainUnavailable indica que 0_Brain no esta disponible, tiene un journal pendiente o fallo al ejecutarse (Exit code 4).
	ErrBrainUnavailable = errors.New("0_Brain no disponible o preflight fallido")
	// ErrBrainIndexMissing indica que .index/brain.db no existe en la boveda (envuelto en ErrBrainUnavailable -> Exit code 4).
	ErrBrainIndexMissing = fmt.Errorf("%w: indice .index/brain.db ausente", ErrBrainUnavailable)
)

// BrainPreflight contiene los metadatos de verificacion read-only de la boveda 0_Brain.
type BrainPreflight struct {
	VaultRoot    string
	DBPath       string
	BrainDBMTime string
}

// BrainContextResponse agrupa la respuesta estructurada de 0_Brain/brain.exe context --json.
type BrainContextResponse struct {
	MatchedDocs         []BrainDocMatch
	RiskHits            []RiskHit
	RiskZonesSHA256     string
	RiskZonesParseError string
	HasActiveRiskZones  bool
}

// BrainQueryOpts parametriza una consulta a 0_Brain/brain.exe query --json.
type BrainQueryOpts struct {
	Project string
	Type    string
	Query   string
	Limit   int
}

// BrainClient abstrae la comunicacion de solo lectura con 0_Brain (CLI subprocess en v1; MCP en Fase 2).
type BrainClient interface {
	Preflight(vaultRoot, dbPath string) (BrainPreflight, error)
	Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error)
	Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error)
}

// SubprocessBrainClient implementa BrainClient invocando 0_Brain/brain.exe como subproceso.
type SubprocessBrainClient struct {
	BrainBin string
	Timeout  time.Duration
}

// NewSubprocessBrainClient construye un SubprocessBrainClient con timeout por defecto de 5s.
func NewSubprocessBrainClient(brainBin string, timeout time.Duration) *SubprocessBrainClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SubprocessBrainClient{
		BrainBin: brainBin,
		Timeout:  timeout,
	}
}

// Preflight ejecuta la verificacion estrictamente de solo lectura (NG2) sobre vaultRoot y dbPath:
//  1. Verifica bfcore.CheckNoReparseChain sobre vaultRoot y dbPath.
//  2. Comprueba os.Lstat sobre dbPath (.index/brain.db). Si falta, devuelve ErrBrainIndexMissing (ErrBrainUnavailable).
//  3. Comprueba os.Lstat sobre <vaultRoot>/.brain-journal.json. Si existe, devuelve inmediatamente ErrBrainUnavailable
//     sin invocar brain.exe verify (evitando disparar RecoverJournal que mutaria la boveda).
func (c *SubprocessBrainClient) Preflight(vaultRoot, dbPath string) (BrainPreflight, error) {
	if strings.TrimSpace(vaultRoot) == "" {
		return BrainPreflight{}, fmt.Errorf("%w: ruta de boveda vacia", ErrBrainUnavailable)
	}
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(vaultRoot, ".index", "brain.db")
	}

	if err := bfcore.CheckNoReparseChain("", vaultRoot); err != nil {
		return BrainPreflight{}, fmt.Errorf("%w: reparse point en boveda: %v", ErrBrainUnavailable, err)
	}
	if err := bfcore.CheckNoReparseChain(vaultRoot, dbPath); err != nil {
		return BrainPreflight{}, fmt.Errorf("%w: reparse point en brain.db: %v", ErrBrainUnavailable, err)
	}

	fiDB, err := os.Lstat(dbPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BrainPreflight{}, fmt.Errorf("%w: %s (ejecute '0_Brain/brain.exe index')", ErrBrainIndexMissing, dbPath)
		}
		return BrainPreflight{}, fmt.Errorf("%w: inspeccionando %s: %v", ErrBrainUnavailable, dbPath, err)
	}
	if fiDB.IsDir() || bfcore.IsReparseOrSymlink(fiDB) {
		return BrainPreflight{}, fmt.Errorf("%w: %s no es un archivo regular", ErrBrainUnavailable, dbPath)
	}

	journalPath := filepath.Join(vaultRoot, ".brain-journal.json")
	if _, jErr := os.Lstat(journalPath); jErr == nil {
		return BrainPreflight{}, fmt.Errorf("%w: diario pendiente %s detectado (NG2 read-only: ejecute '0_Brain/brain.exe verify' manualmente)", ErrBrainUnavailable, journalPath)
	} else if !errors.Is(jErr, os.ErrNotExist) {
		return BrainPreflight{}, fmt.Errorf("%w: inspeccionando %s: %v", ErrBrainUnavailable, journalPath, jErr)
	}

	return BrainPreflight{
		VaultRoot:    vaultRoot,
		DBPath:       dbPath,
		BrainDBMTime: fiDB.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Context invoca 0_Brain/brain.exe context --vault <v> --db <db> --project <p> --files <csv> --json
// y extrae los documentos activos junto con los risk_hits estructurados de wiki/risk_zones.md.
func (c *SubprocessBrainClient) Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error) {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(vaultRoot, ".index", "brain.db")
	}
	csvFiles := strings.Join(files, ",")
	args := []string{"context", "--vault", vaultRoot, "--db", dbPath}
	if strings.TrimSpace(project) != "" {
		args = append(args, "--project", strings.TrimSpace(project))
	}
	if csvFiles != "" {
		args = append(args, "--files", csvFiles)
	}
	args = append(args, "--json")

	outBytes, inv, err := c.execBrain(args)
	if err != nil {
		return BrainContextResponse{}, inv, err
	}

	var docs []BrainDocMatch
	trimmed := bytes.TrimSpace(outBytes)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &docs); err != nil {
			return BrainContextResponse{}, inv, fmt.Errorf("%w: decodificando salida de brain context: %v", ErrBrainUnavailable, err)
		}
	}
	if docs == nil {
		docs = []BrainDocMatch{}
	}
	inv.ResultCount = len(docs)

	resp := BrainContextResponse{
		MatchedDocs: make([]BrainDocMatch, 0, len(docs)),
		RiskHits:    []RiskHit{},
	}
	for _, d := range docs {
		if !strings.EqualFold(d.Status, "active") {
			continue
		}
		if strings.EqualFold(d.Type, "risk_zones") {
			resp.HasActiveRiskZones = true
			if d.RiskZonesSHA256 != "" {
				resp.RiskZonesSHA256 = d.RiskZonesSHA256
			}
			if d.RiskZonesParseError != "" {
				resp.RiskZonesParseError = d.RiskZonesParseError
			}
			if len(d.RiskHits) > 0 {
				resp.RiskHits = append(resp.RiskHits, d.RiskHits...)
			}
		}
		d.RiskHits = nil
		d.RiskZonesSHA256 = ""
		d.RiskZonesParseError = ""
		resp.MatchedDocs = append(resp.MatchedDocs, d)
	}
	return resp, inv, nil
}

// Query invoca 0_Brain/brain.exe query --vault <v> --db <db> [--project <p>] [--type <t>] [--limit <n>] [--q <q>] --json.
func (c *SubprocessBrainClient) Query(vaultRoot, dbPath string, opts BrainQueryOpts) ([]BrainDocMatch, BrainInvocation, error) {
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

	outBytes, inv, err := c.execBrain(args)
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

func (c *SubprocessBrainClient) execBrain(args []string) ([]byte, BrainInvocation, error) {
	bin := strings.TrimSpace(c.BrainBin)
	if bin == "" {
		bin = filepath.Join("0_Brain", "brain.exe")
	}
	cmdStr := formatCommandDisplay(bin, args)
	cmdStr, _ = bfcore.RedactTier2Text("command", cmdStr)
	if _, err := os.Stat(bin); err != nil {
		return nil, BrainInvocation{Command: cmdStr, ExitCode: -1}, fmt.Errorf("%w: binario brain.exe no encontrado en %s: %v", ErrBrainUnavailable, bin, err)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	dur := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		inv := BrainInvocation{Command: cmdStr, ExitCode: exitCode, DurationMs: dur}
		return nil, inv, fmt.Errorf("%w: comando %q fallo (exit=%d): %v (%s)", ErrBrainUnavailable, cmdStr, exitCode, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), BrainInvocation{Command: cmdStr, ExitCode: 0, DurationMs: dur}, nil
}

func formatCommandDisplay(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, filepath.ToSlash(bin))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			parts = append(parts, fmt.Sprintf("%q", a))
		} else {
			parts = append(parts, filepath.ToSlash(a))
		}
	}
	return strings.Join(parts, " ")
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
