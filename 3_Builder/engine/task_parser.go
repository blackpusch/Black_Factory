package builder

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"blackfactory/pkg/bfcore"
)

var (
	reTaskID       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,63}$`)
	reWorkItemID   = regexp.MustCompile(`^WI-[0-9a-f]{12}$`)
	reHex64        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reHex40        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	reWinPath      = regexp.MustCompile(`([A-Za-z]:[\\/][^\r\n<>|"]+)`)
	reBackticked   = regexp.MustCompile("`([^`]+)`")
	reListPrefix   = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	reHeading      = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
	reRevSuffix    = regexp.MustCompile(`\s*\(rev\s+\d+\)\s*$`)
	reParensSuffix = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

	optionIUntrackedTargets = []string{
		"0_brain/brain.exe",
		"1_intake/intake.exe",
		"2_spec_writer/specwriter.exe",
		"3_builder/builder.exe",
		"gengiswap/gengis.exe",
		"0_brain/vault",
		"1_intake/runtime",
		"2_spec_writer/runtime",
		"3_builder/runtime",
	}
)

// Scope modela el alcance de GengiSWAP (paridad exacta con GengiSWAP/engine/ring.go:88-142).
type Scope struct {
	CanModify []string `json:"can_modify"`
	ReadOnly  []string `json:"read_only"`
	Forbidden []string `json:"forbidden"`
}

func normScopePath(p string) string {
	return strings.TrimSuffix(path.Clean(strings.ReplaceAll(p, `\`, "/")), "/")
}

func underScope(p string, prefixes []string) bool {
	p = normScopePath(p)
	for _, pre := range prefixes {
		if strings.TrimSpace(pre) == "" {
			continue
		}
		pre = normScopePath(pre)
		if pre == "." || p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

// Allows evalua si el archivo p puede modificarse segun el alcance de la tarea (sensible a mayusculas).
func (s Scope) Allows(p string) bool {
	if n := normScopePath(p); n == ".." || strings.HasPrefix(n, "../") {
		return false
	}
	if underScope(p, s.Forbidden) || underScope(p, s.ReadOnly) {
		return false
	}
	return underScope(p, s.CanModify)
}

// Narrows aplica el alcance del chunk, que solo puede restringir el de la tarea.
func (s Scope) Narrows(p string) bool {
	if underScope(p, s.Forbidden) || underScope(p, s.ReadOnly) {
		return false
	}
	return len(s.CanModify) == 0 || underScope(p, s.CanModify)
}

// ScopeViolations devuelve las rutas cambiadas que violan el alcance combinado task + chunk.
func ScopeViolations(task, chunk Scope, changed []string) []string {
	var out []string
	for _, f := range changed {
		if !task.Allows(f) || !chunk.Narrows(f) {
			out = append(out, f)
		}
	}
	return out
}

// RingVerdict modela el veredicto en .ring/state.json de GengiSWAP.
type RingVerdict struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
	Revert   bool   `json:"revert"`
	At       string `json:"at"`
}

// RingAttempt modela un intento en .ring/state.json.
type RingAttempt struct {
	N               int          `json:"n"`
	StartedAt       string       `json:"started_at"`
	BaseRef         string       `json:"base_ref"`
	CommitRef       string       `json:"commit_ref"`
	ChefDone        bool         `json:"chef_done"`
	ChefOutput      string       `json:"chef_output"`
	ChangedFiles    []string     `json:"changed_files"`
	ScopeViolations []string     `json:"scope_violations"`
	Evidence        []Evidence   `json:"evidence"`
	PrepPassed      bool         `json:"prep_passed"`
	PrepNotes       string       `json:"prep_notes"`
	QADone          bool         `json:"qa_done"`
	QAPassed        bool         `json:"qa_passed"`
	QANotes         string       `json:"qa_notes"`
	Verdict         *RingVerdict `json:"verdict,omitempty"`
}

// RingChunk modela un chunk en .ring/state.json o chunks/*.md.
type RingChunk struct {
	ID             string        `json:"id"`
	Spec           string        `json:"spec"`
	Acceptance     []string      `json:"acceptance"`
	VerifyCommands []string      `json:"verify_commands"`
	FailureProof   string        `json:"failure_proof"`
	Scope          Scope         `json:"scope"`
	Status         string        `json:"status"`
	Attempts       []RingAttempt `json:"attempts"`
	Feedback       []string      `json:"feedback"`
	ApprovedOutput string        `json:"approved_output"`
}

// RingState modela el archivo <taskDir>/.ring/state.json de GengiSWAP.
type RingState struct {
	TaskID       string      `json:"task_id"`
	Prompt       string      `json:"prompt"`
	Scope        Scope       `json:"scope"`
	MaxRetries   int         `json:"max_retries"`
	PlanApproved bool        `json:"plan_approved"`
	Index        int         `json:"index"`
	Status       string      `json:"status"`
	Chunks       []RingChunk `json:"chunks"`
}

// LoadRingState lee <taskDir>/.ring/state.json; retorna (nil, nil) si no existe.
func LoadRingState(taskDir string) (*RingState, error) {
	statePath := filepath.Join(taskDir, ".ring", "state.json")
	if err := bfcore.CheckNoReparseChain(taskDir, statePath); err != nil {
		return nil, err
	}
	raw, err := ReadFileWithShortRetry(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var st RingState
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &st); err != nil {
		return nil, fmt.Errorf("estado .ring/state.json corrupto: %w", err)
	}
	return &st, nil
}

// LoadRingBaseRef lee <taskDir>/.ring/base_ref.
func LoadRingBaseRef(taskDir string) (string, error) {
	basePath := filepath.Join(taskDir, ".ring", "base_ref")
	if err := bfcore.CheckNoReparseChain(taskDir, basePath); err != nil {
		return "", err
	}
	raw, err := ReadFileWithShortRetry(basePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// ParsedIndexMetadata agrupa los metadatos extraidos de <taskDir>/index.md.
type ParsedIndexMetadata struct {
	TaskID        string
	WorkItemID    string
	TraceID       string
	SpecSHA256    string
	Lane          string
	TargetBranch  string
	TargetRepo    string
	RepoLineCount int
	Scope         Scope
}

// IsValidTaskID valida task_id contra ^[a-z0-9][a-z0-9_-]{2,63}$ sin nombres reservados de Windows.
func IsValidTaskID(id string) bool {
	if !reTaskID.MatchString(id) {
		return false
	}
	switch strings.ToLower(id) {
	case "con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return false
	}
	return true
}

// DeterministicFallbackTraceID deriva un trace_id estable cuando index.md manual no declara Trace ID.
func DeterministicFallbackTraceID(taskID, baseRef string) string {
	sum := sha256.Sum256([]byte(taskID + "\n" + baseRef))
	return "trc-builder-" + taskID + "-" + hex.EncodeToString(sum[:])[:8]
}

// ParseIndexFile extrae alcance, metadatos de trazabilidad y repositorio destino de <taskDir>/index.md.
func ParseIndexFile(taskDir string) (ParsedIndexMetadata, error) {
	absDir, err := filepath.Abs(taskDir)
	if err != nil {
		return ParsedIndexMetadata{}, err
	}
	indexPath := filepath.Join(absDir, "index.md")
	if err := bfcore.CheckNoReparseChain("", indexPath); err != nil {
		return ParsedIndexMetadata{}, err
	}
	lines, err := readMarkdownLines(indexPath)
	if err != nil {
		return ParsedIndexMetadata{}, err
	}

	taskID := filepath.Base(filepath.Clean(absDir))
	var (
		scopeLines    []string
		repoLines     []string
		section       string
		inFence       bool
		repoLineCount int
		meta          = ParsedIndexMetadata{TaskID: taskID}
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFenceToggle(trimmed) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if lvl, heading, ok := parseHeading(trimmed); ok {
			normH := normalizeKey(stripHeadingTags(heading))
			switch {
			case lvl == 2 && strings.HasPrefix(normH, "alcance"):
				section = "alcance"
			case lvl == 2 && (strings.Contains(normH, "directorio de destino") || strings.HasPrefix(normH, "repositorio")):
				section = "destino"
			case lvl == 2 && strings.Contains(normH, "notas post-merge"):
				section = "notas"
			default:
				section = ""
			}
			continue
		}

		switch section {
		case "alcance":
			scopeLines = append(scopeLines, line)
		case "destino":
			repoLines = append(repoLines, line)
			if key, val, ok := parseFieldLabel(trimmed); ok {
				nk := normalizeKey(key)
				if nk == "repositorio destino" {
					repoLineCount++
					meta.TargetRepo = cleanMetadataRepoValue(val)
				} else if nk == "rama destino" && meta.TargetBranch == "" {
					meta.TargetBranch = cleanBacktickedScalar(val)
				}
			}
		case "notas":
			if key, val, ok := parseFieldLabel(trimmed); ok {
				nk := normalizeKey(key)
				switch nk {
				case "workitem":
					v := reRevSuffix.ReplaceAllString(strings.TrimSpace(val), "")
					meta.WorkItemID = cleanBacktickedScalar(v)
				case "trace id":
					meta.TraceID = cleanBacktickedScalar(val)
				case "carril":
					meta.Lane = cleanBacktickedScalar(val)
				case "spec sha-256":
					meta.SpecSHA256 = cleanBacktickedScalar(val)
				}
			}
		}
	}

	if len(scopeLines) == 0 {
		return ParsedIndexMetadata{}, fmt.Errorf("%s no contiene seccion ## Alcance", indexPath)
	}
	extracted := extractTargetRepo(repoLines)
	if extracted == "" {
		extracted = extractTargetRepo(lines)
	}
	if meta.TargetRepo == "" {
		meta.TargetRepo = extracted
	}
	meta.RepoLineCount = repoLineCount
	meta.Scope = parseScopeLines(scopeLines, meta.TargetRepo)
	return meta, nil
}

// ExtractIndexTargetRepo extrae la ruta de repositorio destino de index.md con paridad a GengiSWAP.
func ExtractIndexTargetRepo(indexPath string) (string, error) {
	if err := bfcore.CheckNoReparseChain("", indexPath); err != nil {
		return "", err
	}
	lines, err := readMarkdownLines(indexPath)
	if err != nil {
		return "", err
	}
	var repoLines []string
	var section string
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFenceToggle(trimmed) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if lvl, heading, ok := parseHeading(trimmed); ok {
			normH := normalizeKey(stripHeadingTags(heading))
			if lvl == 2 && (strings.Contains(normH, "directorio de destino") || strings.HasPrefix(normH, "repositorio")) {
				section = "destino"
			} else {
				section = ""
			}
			continue
		}
		if section == "destino" {
			repoLines = append(repoLines, line)
		}
	}
	if r := extractTargetRepo(repoLines); r != "" {
		return r, nil
	}
	return extractTargetRepo(lines), nil
}

// BindIndexTargetRepo reescribe atomicamente la unica linea "Repositorio destino:" bajo
// ## Directorio de Destino Real en indexPath apuntando a absWorktreeClean y verifica paridad round-trip.
func BindIndexTargetRepo(indexPath, absWorktreeClean string) error {
	if err := bfcore.CheckNoReparseChain("", indexPath); err != nil {
		return err
	}
	if hits := bfcore.ScanTier1("worktree_path", absWorktreeClean, false); len(hits) > 0 {
		return fmt.Errorf("%w: secreto Tier 1 en ruta worktree (%s)", ErrG4Violation, hits[0].Rule)
	}
	raw, err := ReadFileWithShortRetry(indexPath)
	if err != nil {
		return err
	}
	hadCRLF := bytes.Contains(raw, []byte("\r\n"))
	s := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")

	section := ""
	inFence := false
	matchIdx := -1
	matches := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFenceToggle(trimmed) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if lvl, heading, ok := parseHeading(trimmed); ok {
			normH := normalizeKey(stripHeadingTags(heading))
			if lvl == 2 && (strings.Contains(normH, "directorio de destino") || strings.HasPrefix(normH, "repositorio")) {
				section = "destino"
			} else {
				section = ""
			}
			continue
		}
		if section == "destino" {
			if key, _, ok := parseFieldLabel(trimmed); ok && normalizeKey(key) == "repositorio destino" {
				matches++
				matchIdx = i
			}
		}
	}
	if matches != 1 || matchIdx < 0 {
		return fmt.Errorf("invariante rota en index.md: se esperaba exactamente 1 linea 'Repositorio destino', halladas %d", matches)
	}

	lines[matchIdx] = "- **Repositorio destino:** " + absWorktreeClean
	joined := strings.Join(lines, "\n")
	if hadCRLF {
		joined = strings.ReplaceAll(joined, "\n", "\r\n")
	}
	if err := WriteFileAtomicBytes(indexPath, []byte(joined)); err != nil {
		return err
	}
	got, err := ExtractIndexTargetRepo(indexPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(absWorktreeClean)) {
		return fmt.Errorf("fallo verificacion round-trip de index.md: obtenido %q, esperado %q", got, absWorktreeClean)
	}
	return nil
}

// ParseChunksDir parsea chunks/*.md ordenados lexicograficamente (para Secuencia A pre-plan).
func ParseChunksDir(taskDir, targetRepo string) ([]RingChunk, error) {
	chunksDir := filepath.Join(taskDir, "chunks")
	if err := bfcore.CheckNoReparseChain(taskDir, chunksDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no se encontraron chunks .md en %s", chunksDir)
	}
	chunks := make([]RingChunk, 0, len(names))
	for _, name := range names {
		c, err := parseChunkFile(filepath.Join(chunksDir, name), targetRepo)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// ValidateVerifyCommandsWorktreeCompat comprueba que ningun VerifyCommand referencie
// rutas/binarios locales Option i que no esten rastreados en lsTreePaths.
func ValidateVerifyCommandsWorktreeCompat(chunks []RingChunk, lsTreePaths map[string]bool) (pointer, msg string) {
	lowerTracked := make(map[string]bool, len(lsTreePaths))
	for k := range lsTreePaths {
		lowerTracked[strings.ToLower(strings.ReplaceAll(k, `\`, "/"))] = true
	}
	for _, c := range chunks {
		for idx, cmd := range c.VerifyCommands {
			normCmd := strings.ToLower(strings.ReplaceAll(cmd, `\`, "/"))
			for _, target := range optionIUntrackedTargets {
				if containsPathToken(normCmd, target) && !lowerTracked[target] {
					return fmt.Sprintf("/chunks/%s/verify_commands/%d", c.ID, idx),
						fmt.Sprintf("comando referencia ruta local Opcion i no rastreada (%s), ausente en git worktree", target)
				}
			}
		}
	}
	return "", ""
}

func containsPathToken(normCmd, target string) bool {
	start := 0
	for {
		idx := strings.Index(normCmd[start:], target)
		if idx < 0 {
			return false
		}
		pos := start + idx
		beforeOK := pos == 0 || isPathBoundary(normCmd[pos-1])
		end := pos + len(target)
		afterOK := end == len(normCmd) || isPathBoundary(normCmd[end]) || normCmd[end] == '/'
		if beforeOK && afterOK {
			return true
		}
		start = pos + 1
	}
}

func isPathBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '"', '\'', '=', ';', '&', '|', '(', ')', '/':
		return true
	default:
		return false
	}
}

func cleanBacktickedScalar(val string) string {
	s := strings.TrimSpace(val)
	if m := reBackticked.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.Trim(s, "`")
}

func cleanMetadataRepoValue(val string) string {
	s := strings.TrimSpace(val)
	if s == "<TARGET_REPO>" {
		return s
	}
	if m := reBackticked.FindStringSubmatch(s); m != nil {
		return filepath.Clean(strings.TrimSpace(m[1]))
	}
	s = reParensSuffix.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if s == "" || s == "<TARGET_REPO>" {
		return s
	}
	return filepath.Clean(s)
}

func readMarkdownLines(filePath string) ([]string, error) {
	raw, err := ReadFileWithShortRetry(filePath)
	if err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n"), nil
}

func parseScopeLines(lines []string, targetRepo string) Scope {
	var scope Scope
	activeKey := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ">") {
			continue
		}
		if key, val, ok := parseFieldLabel(trimmed); ok {
			nk := normalizeKey(key)
			switch {
			case strings.HasPrefix(nk, "puede modificar"):
				activeKey = "can_modify"
				scope.CanModify = append(scope.CanModify, cleanScopeItems(val, targetRepo)...)
			case strings.HasPrefix(nk, "solo lectura"):
				activeKey = "read_only"
				scope.ReadOnly = append(scope.ReadOnly, cleanScopeItems(val, targetRepo)...)
			case strings.HasPrefix(nk, "prohibido"), strings.HasPrefix(nk, "fuera de alcance"), strings.HasPrefix(nk, "no tocar"):
				activeKey = "forbidden"
				scope.Forbidden = append(scope.Forbidden, cleanScopeItems(val, targetRepo)...)
			default:
				activeKey = ""
			}
			continue
		}
		if activeKey != "" && reListPrefix.MatchString(line) && leadingSpaces(line) >= 2 {
			itemText := reListPrefix.ReplaceAllString(trimmed, "")
			items := cleanScopeItems(itemText, targetRepo)
			switch activeKey {
			case "can_modify":
				scope.CanModify = append(scope.CanModify, items...)
			case "read_only":
				scope.ReadOnly = append(scope.ReadOnly, items...)
			case "forbidden":
				scope.Forbidden = append(scope.Forbidden, items...)
			}
			continue
		}
		if reListPrefix.MatchString(line) && leadingSpaces(line) < 2 {
			activeKey = ""
		}
	}
	return scope
}

func extractTargetRepo(lines []string) string {
	type candidate struct {
		priority int
		path     string
	}
	var best *candidate
	consider := func(priority int, p string) {
		if p == "" {
			return
		}
		if best == nil || priority < best.priority {
			best = &candidate{priority: priority, path: p}
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, val, ok := parseFieldLabel(trimmed)
		if !ok {
			if p := extractConcretePath(trimmed); p != "" {
				consider(5, p)
			}
			continue
		}
		nk := normalizeKey(key)
		readOnlyTag := strings.Contains(nk, "solo lectura")
		var prio int
		switch {
		case strings.Contains(nk, "repositorio destino") || strings.Contains(nk, "repositorio de implementacion"):
			prio = 1
		case strings.Contains(nk, "repositorio principal") || nk == "repositorio":
			prio = 2
		case strings.Contains(nk, "repositorio") || strings.Contains(nk, "directorio de destino"):
			prio = 3
		case strings.Contains(nk, "entorno aislado"):
			prio = 4
		default:
			continue
		}
		if readOnlyTag {
			prio += 10
		}
		consider(prio, extractConcretePath(val))
	}
	if best == nil {
		return ""
	}
	return best.path
}

func extractConcretePath(text string) string {
	for _, m := range reBackticked.FindAllStringSubmatch(text, -1) {
		if p := sanitizeRepoCandidate(m[1]); p != "" {
			return p
		}
	}
	clean := stripTrailingParens(strings.TrimSpace(text))
	if m := reWinPath.FindStringSubmatch(clean); m != nil {
		if p := sanitizeRepoCandidate(m[1]); p != "" {
			return p
		}
	}
	if p := sanitizeRepoCandidate(clean); p != "" && (strings.HasPrefix(p, "/") || strings.Contains(p, `\`)) {
		return p
	}
	return ""
}

func sanitizeRepoCandidate(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "`\"'")
	s = stripTrailingParens(s)
	s = strings.TrimRight(s, "* ")
	if s == "" || strings.ContainsAny(s, "<>") {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return ""
	}
	if len(s) > 3 {
		s = strings.TrimRight(s, `/\`)
	}
	if s == "." || s == ".." {
		return ""
	}
	return filepath.FromSlash(s)
}

func parseChunkFile(chunkPath, targetRepo string) (RingChunk, error) {
	if err := bfcore.CheckNoReparseChain("", chunkPath); err != nil {
		return RingChunk{}, err
	}
	lines, err := readMarkdownLines(chunkPath)
	if err != nil {
		return RingChunk{}, err
	}
	base := filepath.Base(chunkPath)
	chunkID := strings.TrimSuffix(base, filepath.Ext(base))

	var title string
	var specParts, acceptance, verifyRaw, failureParts []string
	var scope Scope
	field := ""
	inCoreH2 := false
	inFence := false
	var fenceLines []string

	flushFence := func() {
		if len(fenceLines) == 0 {
			return
		}
		switch field {
		case "verify":
			verifyRaw = append(verifyRaw, extractFenceCommands(fenceLines)...)
		case "acceptance":
			for _, fl := range fenceLines {
				if t := strings.TrimSpace(fl); t != "" {
					acceptance = append(acceptance, t)
				}
			}
		case "failure":
			failureParts = append(failureParts, strings.Join(fenceLines, "\n"))
		}
		fenceLines = nil
	}

	addToField := func(val string, isBullet bool) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		switch field {
		case "spec":
			specParts = append(specParts, val)
		case "acceptance":
			acceptance = append(acceptance, val)
		case "verify":
			verifyRaw = append(verifyRaw, splitVerifyCommands(val)...)
		case "failure":
			failureParts = append(failureParts, val)
		case "forbidden":
			scope.Forbidden = append(scope.Forbidden, cleanPathTokensOnly(val, targetRepo)...)
		case "can_modify":
			scope.CanModify = append(scope.CanModify, cleanScopeItems(val, targetRepo)...)
		case "read_only":
			scope.ReadOnly = append(scope.ReadOnly, cleanScopeItems(val, targetRepo)...)
		default:
			if !isBullet {
				specParts = append(specParts, val)
			}
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isFenceToggle(trimmed) {
			if inFence {
				flushFence()
				inFence = false
			} else {
				inFence = true
				fenceLines = nil
			}
			continue
		}
		if inFence {
			fenceLines = append(fenceLines, line)
			continue
		}
		if lvl, heading, ok := parseHeading(trimmed); ok {
			if normalizeKey(stripHeadingTags(heading)) == "historial de feedback" {
				break
			}
			if lvl == 1 && title == "" {
				title = cleanChunkTitle(heading)
				field = ""
				inCoreH2 = false
				continue
			}
			if nextField := classifyChunkField(heading, true); nextField != "" || lvl <= 2 {
				field = nextField
				inCoreH2 = lvl >= 2 && field != "" && field != "other"
			}
			continue
		}
		isBullet := reListPrefix.MatchString(line)
		if !isBullet || !inCoreH2 {
			if key, val, ok := parseFieldLabel(trimmed); ok {
				if nextField := classifyChunkField(key, false); nextField != "" {
					field = nextField
					inCoreH2 = false
					addToField(val, true)
					continue
				}
			}
		}
		if trimmed == "" || trimmed == "---" {
			continue
		}
		if isBullet {
			addToField(strings.TrimSpace(reListPrefix.ReplaceAllString(trimmed, "")), true)
			continue
		}
		addToField(trimmed, false)
	}
	if inFence {
		flushFence()
	}

	spec := strings.TrimSpace(strings.Join(specParts, " "))
	if spec == "" {
		spec = title
	} else if title != "" && !strings.HasPrefix(strings.ToLower(spec), strings.ToLower(title)) {
		spec = title + " — " + spec
	}
	return RingChunk{
		ID:             chunkID,
		Spec:           strings.TrimSpace(spec),
		Acceptance:     nonEmptyStrings(acceptance),
		VerifyCommands: nonEmptyStrings(verifyRaw),
		FailureProof:   strings.TrimSpace(strings.Join(failureParts, "; ")),
		Scope:          scope,
		Status:         "PENDING",
	}, nil
}

func classifyChunkField(label string, fromHeading bool) string {
	nk := normalizeKey(stripHeadingTags(label))
	switch nk {
	case "descripcion", "spec", "objetivo":
		return "spec"
	case "criterio de aceptacion", "criterios de aceptacion", "acceptance":
		return "acceptance"
	case "verificacion", "verificacion cli", "comando de verificacion", "comandos de verificacion":
		return "verify"
	case "prueba de que puede fallar", "prueba que puede fallar", "prueba de fallo", "condicion de fallo":
		return "failure"
	case "fuera de alcance", "no tocar", "prohibido":
		return "forbidden"
	case "puede modificar":
		return "can_modify"
	case "solo lectura":
		return "read_only"
	case "reglas", "entradas", "salidas", "entregables", "notas", "contexto":
		return "other"
	}
	if fromHeading {
		switch {
		case strings.HasPrefix(nk, "criterio de aceptacion"), strings.HasPrefix(nk, "criterios de aceptacion"):
			return "acceptance"
		case strings.HasPrefix(nk, "comando de verificacion"), strings.HasPrefix(nk, "comandos de verificacion"):
			return "verify"
		case strings.HasPrefix(nk, "prueba de que puede fallar"), strings.HasPrefix(nk, "prueba que puede fallar"), strings.HasPrefix(nk, "prueba de fallo"):
			return "failure"
		case strings.HasPrefix(nk, "fuera de alcance"):
			return "forbidden"
		}
	}
	return ""
}

func cleanChunkTitle(heading string) string {
	s := strings.TrimSpace(heading)
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "tarea:") {
		return strings.TrimSpace(s[len("tarea:"):])
	}
	if strings.HasPrefix(lower, "chunk ") {
		if idx := strings.IndexByte(s, ':'); idx > 0 {
			return strings.TrimSpace(s[idx+1:])
		}
	}
	return s
}

func splitVerifyCommands(val string) []string {
	s := strings.TrimSpace(val)
	if s == "" {
		return nil
	}
	if strings.Count(s, "`") == 2 && strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`") {
		if inner := strings.TrimSpace(s[1 : len(s)-1]); inner != "" {
			return []string{inner}
		}
		return nil
	}
	if strings.Count(s, "`") >= 4 && onlyBackticksAndSeparators(s) {
		var cmds []string
		for _, m := range reBackticked.FindAllStringSubmatch(s, -1) {
			if cmd := strings.TrimSpace(m[1]); cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
		return cmds
	}
	return []string{s}
}

func onlyBackticksAndSeparators(s string) bool {
	stripped := reBackticked.ReplaceAllString(s, " ")
	for _, tok := range strings.Fields(stripped) {
		switch strings.ToLower(tok) {
		case ",", ";", "y", "e", "o", "and", "or":
		default:
			return false
		}
	}
	return true
}

func extractFenceCommands(lines []string) []string {
	var cmds []string
	var buf strings.Builder
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			if buf.Len() > 0 {
				cmds = append(cmds, strings.TrimSpace(buf.String()))
				buf.Reset()
			}
			continue
		}
		if strings.HasSuffix(line, `\`) || strings.HasSuffix(line, " `") {
			cont := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(line, `\`), "`"))
			if buf.Len() > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(cont)
			continue
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
			buf.WriteString(line)
			cmds = append(cmds, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		cmds = append(cmds, line)
	}
	if buf.Len() > 0 {
		cmds = append(cmds, strings.TrimSpace(buf.String()))
	}
	return cmds
}

func cleanScopeItems(raw, targetRepo string) []string {
	parts := splitScopeComma(raw)
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		bt := reBackticked.FindAllStringSubmatch(trimmed, -1)
		if len(bt) > 1 || (len(bt) == 1 && (!strings.HasPrefix(trimmed, "`") || !strings.HasSuffix(stripTrailingParens(trimmed), "`"))) {
			added := false
			for _, m := range bt {
				if strings.HasSuffix(strings.TrimSpace(m[1]), "()") {
					continue
				}
				if normItem, ok := normalizeScopeToken(m[1], targetRepo, true); ok {
					out = append(out, normItem)
					added = true
				}
			}
			if added {
				continue
			}
		}
		if normItem, ok := normalizeScopeToken(part, targetRepo, false); ok {
			out = append(out, normItem)
		}
	}
	return out
}

func cleanPathTokensOnly(raw, targetRepo string) []string {
	var out []string
	for _, m := range reBackticked.FindAllStringSubmatch(raw, -1) {
		if item, ok := normalizeScopeToken("`"+m[1]+"`", targetRepo, true); ok {
			out = append(out, item)
		}
	}
	if len(out) > 0 {
		return out
	}
	if strings.ContainsAny(raw, `/\`) {
		for _, part := range splitScopeComma(raw) {
			if item, ok := normalizeScopeToken(part, targetRepo, true); ok {
				out = append(out, item)
			}
		}
	}
	return out
}

func splitScopeComma(raw string) []string {
	var parts []string
	var cur strings.Builder
	parenDepth := 0
	inTick := false
	for _, r := range raw {
		switch r {
		case '`':
			inTick = !inTick
			cur.WriteRune(r)
		case '(':
			if !inTick {
				parenDepth++
			}
			cur.WriteRune(r)
		case ')':
			if !inTick && parenDepth > 0 {
				parenDepth--
			}
			cur.WriteRune(r)
		case ',':
			if !inTick && parenDepth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
				continue
			}
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func normalizeScopeToken(raw, targetRepo string, requirePath bool) (string, bool) {
	s := strings.TrimSpace(raw)
	s = stripTrailingParens(s)
	quoted := strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`") && len(s) >= 2
	s = strings.TrimSpace(strings.Trim(s, "`\"'"))
	s = stripTrailingParens(s)
	if s == "" {
		return "", false
	}
	switch strings.ToLower(s) {
	case "ninguno", "ninguna", "ningún", "nada", "none", "n/a", "-", "—":
		return "", false
	}
	s = strings.ReplaceAll(s, `\`, "/")
	for _, suf := range []string{"/**", "/*"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	s = strings.TrimSuffix(s, "/")
	if s == "." || s == "./" || s == "*" || s == "**" {
		return ".", true
	}
	if targetRepo != "" {
		repoNorm := strings.TrimSuffix(strings.ReplaceAll(filepath.Clean(targetRepo), `\`, "/"), "/")
		if strings.EqualFold(s, repoNorm) {
			return ".", true
		}
		if len(s) > len(repoNorm)+1 && strings.EqualFold(s[:len(repoNorm)], repoNorm) && s[len(repoNorm)] == '/' {
			s = s[len(repoNorm)+1:]
		}
	}
	s = normScopePath(s)
	if s == "" {
		return "", false
	}
	if requirePath && !quoted && !strings.Contains(s, "/") && !strings.Contains(s, ".") {
		return "", false
	}
	if !quoted && strings.Contains(s, " ") && !strings.Contains(s, "/") {
		return "", false
	}
	return s, true
}

func stripTrailingParens(s string) string {
	out := strings.TrimSpace(s)
	for strings.HasSuffix(out, ")") {
		depth := 0
		openIdx := -1
		for i := len(out) - 1; i >= 0; i-- {
			switch out[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					openIdx = i
				}
			}
			if openIdx >= 0 {
				break
			}
		}
		if openIdx <= 0 || out[openIdx-1] != ' ' {
			break
		}
		out = strings.TrimSpace(out[:openIdx])
	}
	return out
}

func parseFieldLabel(line string) (key, val string, ok bool) {
	s := strings.TrimSpace(line)
	if s == "" {
		return "", "", false
	}
	if reListPrefix.MatchString(s) {
		s = strings.TrimSpace(reListPrefix.ReplaceAllString(s, ""))
	}
	for _, marker := range []string{"**", "*"} {
		if strings.HasPrefix(s, marker) {
			rest := s[len(marker):]
			end := strings.Index(rest, marker)
			if end > 0 {
				inner := strings.TrimSpace(rest[:end])
				after := strings.TrimSpace(rest[end+len(marker):])
				if strings.HasSuffix(inner, ":") {
					return strings.TrimSuffix(inner, ":"), after, true
				}
				if strings.HasPrefix(after, ":") {
					return inner, strings.TrimSpace(after[1:]), true
				}
			}
		}
	}
	colon := strings.IndexByte(s, ':')
	if colon <= 1 {
		return "", "", false
	}
	if colon+1 < len(s) && (s[colon+1] == '\\' || s[colon+1] == '/') {
		return "", "", false
	}
	k := strings.TrimSpace(strings.Trim(s[:colon], "*`"))
	if k == "" || len(k) > 80 {
		return "", "", false
	}
	return k, strings.TrimSpace(s[colon+1:]), true
}

func parseHeading(trimmed string) (int, string, bool) {
	m := reHeading.FindStringSubmatch(trimmed)
	if m == nil {
		return 0, "", false
	}
	return len(m[1]), strings.TrimSpace(m[2]), true
}

func isFenceToggle(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func stripHeadingTags(h string) string {
	s := strings.TrimSpace(h)
	s = strings.Trim(s, "*:`")
	s = stripTrailingParens(s)
	return strings.TrimSpace(s)
}

func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, "*:` ")
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u")
	return strings.TrimSpace(r.Replace(s))
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 2
		} else {
			break
		}
	}
	return n
}

func nonEmptyStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
