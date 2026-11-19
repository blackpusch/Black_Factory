package ring

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	reMaxRetries = regexp.MustCompile(`(?i)m[aá]x\.?\s*intentos(?:\s*por\s*chunk)?[\s:*]*(\d+)`)
	reWinPath    = regexp.MustCompile(`([A-Za-z]:[\\/][^\r\n<>|"]+)`)
	reBackticked = regexp.MustCompile("`([^`]+)`")
	reListPrefix = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+`)
	reHeading    = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)
)

type parsedIndex struct {
	Task       Task
	Scope      Scope
	MaxRetries int
	TargetRepo string
}

func parseIndexFromDir(taskDir string) (parsedIndex, error) {
	absDir, err := filepath.Abs(taskDir)
	if err != nil {
		return parsedIndex{}, fmt.Errorf("resolviendo ruta %s: %w", taskDir, err)
	}
	return parseIndexFile(absDir)
}

func parseTaskFromDir(taskDir string) (parsedIndex, []Chunk, error) {
	absDir, err := filepath.Abs(taskDir)
	if err != nil {
		return parsedIndex{}, nil, fmt.Errorf("resolviendo ruta %s: %w", taskDir, err)
	}
	idx, err := parseIndexFile(absDir)
	if err != nil {
		return parsedIndex{}, nil, err
	}
	chunks, err := parseChunksDir(absDir, idx.TargetRepo)
	if err != nil {
		return parsedIndex{}, nil, err
	}
	return idx, chunks, nil
}

// ParseTaskFromDir lee index.md y los archivos ordenados de chunks\*.md dentro de
// taskDir para construir Task, Scope, []Chunk y Máx. intentos por chunk.
func ParseTaskFromDir(taskDir string) (Task, Scope, []Chunk, int, error) {
	idx, chunks, err := parseTaskFromDir(taskDir)
	if err != nil {
		return Task{}, Scope{}, nil, 0, err
	}
	return idx.Task, idx.Scope, chunks, idx.MaxRetries, nil
}

// ParseTargetRepoFromDir extrae de index.md la ruta del repositorio destino real
// (vacía si la tarea usa solo marcadores de plantilla o no declara ruta física).
func ParseTargetRepoFromDir(taskDir string) (string, error) {
	idx, err := parseIndexFromDir(taskDir)
	if err != nil {
		return "", err
	}
	return idx.TargetRepo, nil
}

func readMarkdownLines(filePath string) ([]string, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n"), nil
}

func parseIndexFile(taskDir string) (parsedIndex, error) {
	indexPath := filepath.Join(taskDir, "index.md")
	lines, err := readMarkdownLines(indexPath)
	if err != nil {
		return parsedIndex{}, fmt.Errorf("leyendo %s: %w", indexPath, err)
	}

	taskID := filepath.Base(filepath.Clean(taskDir))
	title := ""
	objective := ""
	maxRetries := 3

	var scopeLines []string
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

		if m := reMaxRetries.FindStringSubmatch(trimmed); m != nil {
			if n, convErr := strconv.Atoi(m[1]); convErr == nil && n >= 1 {
				maxRetries = n
			}
		}

		if lvl, heading, ok := parseHeading(trimmed); ok {
			if lvl == 1 && title == "" {
				title = heading
			}
			normH := normalizeKey(stripHeadingTags(heading))
			switch {
			case lvl == 2 && strings.HasPrefix(normH, "alcance"):
				section = "alcance"
			case lvl == 2 && (strings.Contains(normH, "directorio de destino") || strings.HasPrefix(normH, "repositorio")):
				section = "destino"
			case lvl == 2 && strings.HasPrefix(normH, "objetivo"):
				section = "objetivo"
			default:
				section = ""
			}
			continue
		}

		if title != "" && objective == "" && section == "" {
			if key, val, ok := parseFieldLabel(trimmed); ok {
				nk := normalizeKey(key)
				if nk == "objetivo" || nk == "objetivo generico" {
					objective = strings.TrimSpace(val)
				}
			}
		}

		switch section {
		case "alcance":
			scopeLines = append(scopeLines, line)
		case "destino":
			repoLines = append(repoLines, line)
		case "objetivo":
			if trimmed != "" {
				objective = joinNonEmpty(objective, trimmed, " ")
			}
		}
	}

	if len(scopeLines) == 0 {
		return parsedIndex{}, fmt.Errorf("%s no contiene sección ## Alcance", indexPath)
	}

	targetRepo := extractTargetRepo(repoLines)
	if targetRepo == "" {
		targetRepo = extractTargetRepo(lines)
	}

	scope := parseScopeLines(scopeLines, targetRepo)
	prompt := objective
	if prompt == "" {
		prompt = title
	}
	if prompt == "" {
		prompt = taskID
	}

	return parsedIndex{
		Task:       Task{ID: taskID, Prompt: prompt},
		Scope:      scope,
		MaxRetries: maxRetries,
		TargetRepo: targetRepo,
	}, nil
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

	consider := func(priority int, path string) {
		if path == "" {
			return
		}
		if best == nil || priority < best.priority {
			best = &candidate{priority: priority, path: path}
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
		prio := 10
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

func parseChunksDir(taskDir, targetRepo string) ([]Chunk, error) {
	chunksDir := filepath.Join(taskDir, "chunks")
	entries, err := os.ReadDir(chunksDir)
	if err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", chunksDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no se encontraron archivos .md en %s", chunksDir)
	}

	chunks := make([]Chunk, 0, len(names))
	for _, name := range names {
		c, err := parseChunkFile(filepath.Join(chunksDir, name), targetRepo)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

func parseChunkFile(chunkPath, targetRepo string) (Chunk, error) {
	lines, err := readMarkdownLines(chunkPath)
	if err != nil {
		return Chunk{}, fmt.Errorf("leyendo %s: %w", chunkPath, err)
	}

	base := filepath.Base(chunkPath)
	chunkID := strings.TrimSuffix(base, filepath.Ext(base))

	var title string
	var specParts []string
	var acceptance []string
	var verifyRaw []string
	var failureParts []string
	var scope Scope

	field := ""
	inCoreH2 := false
	inFence := false
	var fenceLines []string

	flushFence := func() {
		if len(fenceLines) == 0 {
			return
		}
		if field == "verify" {
			verifyRaw = append(verifyRaw, extractFenceCommands(fenceLines)...)
		} else if field == "acceptance" {
			for _, fl := range fenceLines {
				if t := strings.TrimSpace(fl); t != "" {
					acceptance = append(acceptance, t)
				}
			}
		} else if field == "failure" {
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
				nextField := classifyChunkField(key, false)
				if nextField != "" {
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
			item := strings.TrimSpace(reListPrefix.ReplaceAllString(trimmed, ""))
			addToField(item, true)
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

	return Chunk{
		ID:             chunkID,
		Spec:           strings.TrimSpace(spec),
		Acceptance:     nonEmptyStrings(acceptance),
		VerifyCommands: nonEmptyStrings(verifyRaw),
		FailureProof:   strings.TrimSpace(strings.Join(failureParts, "; ")),
		Scope:          scope,
		Status:         StatusPending,
	}, nil
}

func classifyChunkField(label string, fromHeading bool) string {
	nk := normalizeKey(stripHeadingTags(label))
	switch {
	case nk == "descripcion" || nk == "spec" || nk == "objetivo":
		return "spec"
	case nk == "criterio de aceptacion" || nk == "criterios de aceptacion" || nk == "acceptance":
		return "acceptance"
	case nk == "verificacion" || nk == "verificacion cli" || nk == "comando de verificacion" || nk == "comandos de verificacion":
		return "verify"
	case nk == "prueba de que puede fallar" || nk == "prueba que puede fallar" || nk == "prueba de fallo" || nk == "condicion de fallo":
		return "failure"
	case nk == "fuera de alcance" || nk == "no tocar" || nk == "prohibido":
		return "forbidden"
	case nk == "puede modificar":
		return "can_modify"
	case nk == "solo lectura":
		return "read_only"
	case nk == "reglas" || nk == "entradas" || nk == "salidas" || nk == "entregables" || nk == "notas" || nk == "contexto":
		return "other"
	}
	if fromHeading {
		switch {
		case strings.HasPrefix(nk, "criterio de aceptacion") || strings.HasPrefix(nk, "criterios de aceptacion"):
			return "acceptance"
		case strings.HasPrefix(nk, "comando de verificacion") || strings.HasPrefix(nk, "comandos de verificacion"):
			return "verify"
		case strings.HasPrefix(nk, "prueba de que puede fallar") || strings.HasPrefix(nk, "prueba que puede fallar") || strings.HasPrefix(nk, "prueba de fallo"):
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
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner != "" {
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

	s = norm(s)
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

	// Formato Markdown con énfasis: **Clave:** valor  o  *Clave:* valor
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

	// Formato plano: Clave: valor (evitando unidades Windows C:\...)
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
	r := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	)
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

func joinNonEmpty(a, b, sep string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + sep + b
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

// ------------------------------------------------------------ SyncFiles (C1 + status.md + feedback)

// SyncFiles sincroniza el estado en disco sin romper la inmutabilidad del log:
//  1. Regenera <taskDir>\status.md con RenderStatusMD(st).
//  2. Añade exclusivamente st.Events[prevEventCount:] al final de
//     <gengisRoot>\gengis_global_log.md abriéndolo con os.O_APPEND|os.O_CREATE|os.O_WRONLY.
//  3. Anexa "## Historial de feedback" al final de chunks\<chunk>.md para cada
//     intento con decisión RETRY_CHUNK aún no registrado en dicho archivo.
func SyncFiles(gengisRoot, taskDir string, st *State, prevEventCount int) error {
	if st == nil {
		return errors.New("estado nil en SyncFiles")
	}

	statusPath := filepath.Join(taskDir, "status.md")
	if err := os.WriteFile(statusPath, []byte(RenderStatusMD(st)), 0o644); err != nil {
		return fmt.Errorf("escribiendo %s: %w", statusPath, err)
	}

	if err := appendNewLogLines(gengisRoot, st, prevEventCount); err != nil {
		return err
	}

	for _, c := range st.Chunks {
		if err := syncChunkFeedback(taskDir, c); err != nil {
			return err
		}
	}
	for _, c := range st.Replaced {
		if err := syncChunkFeedback(taskDir, c); err != nil {
			return err
		}
	}
	return nil
}

func appendNewLogLines(gengisRoot string, st *State, prevEventCount int) error {
	if strings.TrimSpace(gengisRoot) == "" {
		return nil
	}
	lines := st.LogLines()
	if prevEventCount < 0 {
		prevEventCount = 0
	}
	if prevEventCount >= len(lines) {
		return nil
	}
	newLines := lines[prevEventCount:]

	logPath := filepath.Join(gengisRoot, "gengis_global_log.md")
	needsNewline := fileNeedsTrailingNewline(logPath)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("abriendo log global %s: %w", logPath, err)
	}
	defer f.Close()

	var payload strings.Builder
	if needsNewline {
		payload.WriteByte('\n')
	}
	payload.WriteString(strings.Join(newLines, "\n"))
	payload.WriteByte('\n')

	if _, err := f.WriteString(payload.String()); err != nil {
		return fmt.Errorf("escribiendo en log global %s: %w", logPath, err)
	}
	return nil
}

func fileNeedsTrailingNewline(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}
	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, info.Size()-1); err != nil {
		return false
	}
	return buf[0] != '\n'
}

func syncChunkFeedback(taskDir string, c Chunk) error {
	var retries []Attempt
	for _, att := range c.Attempts {
		if att.Verdict != nil && att.Verdict.Decision == RetryChunk {
			retries = append(retries, att)
		}
	}
	if len(retries) == 0 {
		return nil
	}

	chunkPath := findChunkFilePath(taskDir, c.ID)
	if chunkPath == "" {
		return nil
	}
	raw, err := os.ReadFile(chunkPath)
	if err != nil {
		return fmt.Errorf("leyendo %s: %w", chunkPath, err)
	}
	content := string(raw)
	hasSection := strings.Contains(content, "## Historial de feedback")

	var additions strings.Builder
	for _, att := range retries {
		marker := fmt.Sprintf("### Intento %d ", att.N)
		if strings.Contains(content, marker) {
			continue
		}
		if !hasSection {
			if len(content) > 0 && !strings.HasSuffix(content, "\n") {
				additions.WriteByte('\n')
			}
			if len(content) > 0 && !strings.HasSuffix(content, "\n\n") {
				additions.WriteByte('\n')
			}
			additions.WriteString("## Historial de feedback\n")
			hasSection = true
		} else if additions.Len() == 0 && len(content) > 0 && !strings.HasSuffix(content, "\n") {
			additions.WriteByte('\n')
		}
		additions.WriteString(formatRetryFeedbackEntry(att))
	}

	if additions.Len() == 0 {
		return nil
	}
	f, err := os.OpenFile(chunkPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("abriendo %s para feedback: %w", chunkPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(additions.String()); err != nil {
		return fmt.Errorf("anexando feedback en %s: %w", chunkPath, err)
	}
	return nil
}

func findChunkFilePath(taskDir, chunkID string) string {
	candidates := []string{
		filepath.Join(taskDir, "chunks", safeName(chunkID)+".md"),
		filepath.Join(taskDir, "chunks", chunkID+".md"),
		filepath.Join(taskDir, "chunks", chunkID),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

func formatRetryFeedbackEntry(att Attempt) string {
	ts := "sin fecha"
	if att.Verdict != nil && !att.Verdict.At.IsZero() {
		ts = att.Verdict.At.Format("2006-01-02 15:04:05")
	}
	comment := ""
	if att.Verdict != nil {
		comment = att.Verdict.Comment
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n### Intento %d (%s) — RETRY_CHUNK\n", att.N, ts)
	fmt.Fprintf(&b, "- **QA:** %s\n", orDash(att.QANotes))
	fmt.Fprintf(&b, "- **Motivo del Autor:** %s\n", orDash(comment))
	for _, ev := range att.Evidence {
		if ev.ExitCode == 0 {
			continue
		}
		fence := "```"
		outTail := strings.TrimSpace(tail(ev.Output, 1500))
		if strings.Contains(outTail, "```") {
			fence = "````"
		}
		fmt.Fprintf(&b, "- **Salida literal de `%s` (código %d):**\n%stext\n%s\n%s\n",
			oneLine(ev.Command), ev.ExitCode, fence, orDash(outTail), fence)
	}
	return b.String()
}

// ------------------------------------------------------------ Flujo interactivo CLI

// CLIConfig agrupa los parámetros y dependencias para ejecutar subcomandos sobre el disco.
type CLIConfig struct {
	GengisRoot string
	TaskDir    string
	TargetRepo string
	Runner     CommandRunner
	WS         Workspace
	Now        func() time.Time
	In         io.Reader
	Out        io.Writer
}

// DecideOptions representa las banderas de confirmación o corrección QA del Preparador.
type DecideOptions struct {
	HasQAPass  bool
	QAPass     bool
	HasQANotes bool
	QANotes    string
}

func (c *CLIConfig) normalize() {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.In == nil {
		c.In = os.Stdin
	}
	if c.Out == nil {
		c.Out = os.Stdout
	}
}

func (c CLIConfig) store() safeFileStore {
	return safeFileStore{FileStore: FileStore{Path: filepath.Join(c.TaskDir, ".ring", "state.json")}}
}

type safeFileStore struct {
	FileStore
}

// Save reintenta brevemente ante bloqueos transitorios de indexadores/antivirus
// de Windows durante os.Rename sobre el mismo archivo en rápida sucesión.
func (s safeFileStore) Save(st *State) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = s.FileStore.Save(st)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrPermission) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 15 * time.Millisecond)
	}
	return err
}

func (c CLIConfig) qaDir() string {
	return filepath.Join(c.TaskDir, "workspace", "qa")
}

func (c CLIConfig) resolveTargetRepo() string {
	if repo := strings.TrimSpace(c.TargetRepo); repo != "" {
		return repo
	}
	if idx, err := parseIndexFromDir(c.TaskDir); err == nil {
		return strings.TrimSpace(idx.TargetRepo)
	}
	return ""
}

func (c CLIConfig) runtimeDeps(targetRepo string) (Workspace, CommandRunner) {
	ws, runner := c.WS, c.Runner
	repo := strings.TrimSpace(c.TargetRepo)
	if repo == "" {
		repo = strings.TrimSpace(targetRepo)
	}
	if ws == nil {
		if repo != "" && isGitRepo(repo) {
			ws = GitWorkspace{Dir: repo}
		} else {
			ws = NopWorkspace{}
		}
	}
	if runner == nil {
		dir := c.TaskDir
		if repo != "" {
			if info, err := os.Stat(repo); err == nil && info.IsDir() {
				dir = repo
			}
		}
		runner = ShellRunner{Dir: dir, Timeout: 5 * time.Minute}
	}
	return ws, runner
}

func isGitRepo(dir string) bool {
	_, err := GitWorkspace{Dir: dir}.git(context.Background(), "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func warnIfNoGit(out io.Writer, ws Workspace, repo string) {
	switch ws.(type) {
	case NopWorkspace, *NopWorkspace:
	default:
		return
	}
	if r := strings.TrimSpace(repo); r != "" {
		fmt.Fprintf(out, "ADVERTENCIA: destino sin Git (%s) → alcance NO verificado, sin commit ni revert.\n", r)
		return
	}
	fmt.Fprintln(out, "ADVERTENCIA: destino sin Git (repositorio no configurado o no resuelto) → alcance NO verificado, sin commit ni revert.")
}

func saveBaseRef(taskDir, ref string) {
	if strings.TrimSpace(ref) == "" {
		return
	}
	ringDir := filepath.Join(taskDir, ".ring")
	_ = os.MkdirAll(ringDir, 0o755)
	_ = os.WriteFile(filepath.Join(ringDir, "base_ref"), []byte(ref+"\n"), 0o644)
}

func loadBaseRef(ctx context.Context, taskDir string, ws Workspace) (string, error) {
	if b, err := os.ReadFile(filepath.Join(taskDir, ".ring", "base_ref")); err == nil {
		if ref := strings.TrimSpace(string(b)); ref != "" {
			return ref, nil
		}
	}
	return ws.Head(ctx)
}

// PlanTask valida index.md y chunks\, pide aprobación del Autor [s/N], guarda
// .ring\state.json y sincroniza status.md y gengis_global_log.md.
func PlanTask(ctx context.Context, cfg CLIConfig) error {
	cfg.normalize()
	idx, chunks, err := parseTaskFromDir(cfg.TaskDir)
	if err != nil {
		return err
	}
	if err := validateChunks(chunks, nil); err != nil {
		return err
	}

	task, scope, maxRetries := idx.Task, idx.Scope, idx.MaxRetries
	repo := idx.TargetRepo
	if tr := strings.TrimSpace(cfg.TargetRepo); tr != "" {
		repo = tr
	}
	ws, runner := cfg.runtimeDeps(repo)
	store := cfg.store()
	scanner := bufio.NewScanner(cfg.In)

	eng := &Engine{
		Aud:    &cliAuditor{in: scanner, out: cfg.Out},
		Runner: runner,
		WS:     ws,
		Store:  store,
		QADir:  cfg.qaDir(),
		Now:    cfg.Now,
	}

	st, err := store.Load()
	if err != nil {
		return fmt.Errorf("cargando estado: %w", err)
	}
	prevEvents := 0
	if st != nil {
		if st.TaskID != task.ID {
			return fmt.Errorf("el estado guardado es de la tarea %q, no de %q", st.TaskID, task.ID)
		}
		prevEvents = len(st.Events)
		if st.PlanApproved {
			fmt.Fprintf(cfg.Out, "El plan de %s ya está aprobado (estado: %s).\n", st.TaskID, st.Status)
			return SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, prevEvents)
		}
		st.Prompt = task.Prompt
		st.Scope = scope
		st.MaxRetries = maxRetries
		st.Chunks = chunks
	} else {
		st = &State{
			TaskID:     task.ID,
			Prompt:     task.Prompt,
			Scope:      scope,
			MaxRetries: maxRetries,
			Status:     "PLAN_PENDING",
			Chunks:     chunks,
		}
		eng.event(st, "PREPARADOR", "PLAN_PENDIENTE", "", fmt.Sprintf("%d chunks", len(chunks)))
		if err := eng.save(st); err != nil {
			return err
		}
	}

	printPlanSummary(cfg.Out, st, repo)
	warnIfNoGit(cfg.Out, ws, repo)
	planErr := eng.planStep(ctx, st)
	if st.PlanApproved {
		if head, headErr := ws.Head(ctx); headErr == nil {
			saveBaseRef(cfg.TaskDir, head)
		}
	}
	if syncErr := SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, prevEvents); syncErr != nil {
		return syncErr
	}
	return planErr
}

// CheckChunk ejecuta la fase de entrega/QA del chunk activo sobre Workspace y
// ShellRunner, guarda .ring\state.json, genera workspace\qa\<chunk>.md y
// actualiza status.md ([~] entregado) y gengis_global_log.md.
func CheckChunk(ctx context.Context, cfg CLIConfig) error {
	cfg.normalize()
	repo := cfg.resolveTargetRepo()
	ws, runner := cfg.runtimeDeps(repo)
	store := cfg.store()

	st, err := store.Load()
	if err != nil {
		return fmt.Errorf("cargando estado: %w", err)
	}
	if st == nil {
		return fmt.Errorf("no existe estado en %s; ejecuta primero 'gengis plan'", store.Path)
	}
	if done, err := checkTerminalState(st); err != nil {
		return err
	} else if !st.PlanApproved {
		return ErrPlanNotApproved
	} else if done {
		fmt.Fprintf(cfg.Out, "La tarea %s ya completó todos los chunks.\n", st.TaskID)
		return SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, len(st.Events))
	}

	eng := &Engine{
		Prep:   &cliPreparador{},
		Runner: runner,
		WS:     ws,
		Store:  store,
		QADir:  cfg.qaDir(),
		Now:    cfg.Now,
	}

	prevEvents := len(st.Events)
	i := st.Index
	c := &st.Chunks[i]

	if n := len(c.Attempts); n == 0 || c.Attempts[n-1].Verdict != nil {
		base, headErr := loadBaseRef(ctx, cfg.TaskDir, ws)
		if headErr != nil {
			return fmt.Errorf("leyendo HEAD base: %w", headErr)
		}
		c.Attempts = append(c.Attempts, newAttempt(n, cfg.Now(), base))
	}
	att := &c.Attempts[len(c.Attempts)-1]
	if att.QADone && att.Verdict == nil {
		fmt.Fprintf(cfg.Out, "El chunk %s (intento %d) ya tiene QA registrado (%s); ejecuta 'gengis decide %s'.\n",
			c.ID, att.N, qaResultWord(att.QAPassed), st.TaskID)
		if err := eng.writeQAReport(st.TaskID, *c); err != nil {
			return err
		}
		return SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, prevEvents)
	}

	warnIfNoGit(cfg.Out, ws, repo)
	if !att.ChefDone {
		att.ChefDone = true
		c.Status = StatusDelivered
		eng.event(st, "CHEF", "ENTREGADO", c.ID, fmt.Sprintf("intento %d entregado", att.N))
	}
	if err := eng.qaStep(ctx, st, i); err != nil {
		return err
	}

	att = &c.Attempts[len(c.Attempts)-1]
	fmt.Fprintf(cfg.Out, "QA de %s (intento %d): %s\n", c.ID, att.N, qaResultWord(att.QAPassed))
	fmt.Fprintf(cfg.Out, "Informe QA: %s\n", filepath.Join(cfg.qaDir(), safeName(c.ID)+".md"))
	return SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, prevEvents)
}

// DecideChunk aplica opcionalmente confirmación/notas de QA (--qa-pass/--qa-notes),
// solicita en consola la decisión del Autor, ejecuta la transición en State y
// sincroniza status.md, chunks\ y gengis_global_log.md.
func DecideChunk(ctx context.Context, cfg CLIConfig, opts DecideOptions) error {
	cfg.normalize()
	repo := cfg.resolveTargetRepo()
	ws, runner := cfg.runtimeDeps(repo)
	store := cfg.store()

	st, err := store.Load()
	if err != nil {
		return fmt.Errorf("cargando estado: %w", err)
	}
	if st == nil {
		return fmt.Errorf("no existe estado en %s; ejecuta primero 'gengis plan'", store.Path)
	}
	if done, err := checkTerminalState(st); err != nil {
		return err
	} else if !st.PlanApproved {
		return ErrPlanNotApproved
	} else if done {
		fmt.Fprintf(cfg.Out, "La tarea %s ya está cerrada (%s).\n", st.TaskID, st.Status)
		return SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, len(st.Events))
	}

	i := st.Index
	c := &st.Chunks[i]
	if len(c.Attempts) == 0 || !c.Attempts[len(c.Attempts)-1].QADone || c.Attempts[len(c.Attempts)-1].Verdict != nil {
		return fmt.Errorf("el chunk activo %s aún no tiene QA completado; ejecuta primero 'gengis check'", c.ID)
	}

	scanner := bufio.NewScanner(cfg.In)
	eng := &Engine{
		Prep: &cliPreparador{
			in:         scanner,
			out:        cfg.Out,
			taskDir:    cfg.TaskDir,
			targetRepo: repo,
			existingIDs: func() map[string]bool {
				m := map[string]bool{c.ID: true}
				for _, ch := range st.Chunks {
					m[ch.ID] = true
				}
				for _, ch := range st.Replaced {
					m[ch.ID] = true
				}
				return m
			}(),
		},
		Aud:    &cliAuditor{in: scanner, out: cfg.Out},
		Runner: runner,
		WS:     ws,
		Store:  store,
		QADir:  cfg.qaDir(),
		Now:    cfg.Now,
	}

	prevEvents := len(st.Events)
	att := &c.Attempts[len(c.Attempts)-1]
	if opts.HasQAPass || opts.HasQANotes {
		if err := applyManualQAOverride(eng, st, c, att, opts); err != nil {
			return err
		}
	} else if strings.TrimSpace(att.PrepNotes) == "" || att.PrepNotes == autoVerifyQANote || att.PrepNotes == "verificación automática OK" {
		fmt.Fprintln(cfg.Out, "ADVERTENCIA: este intento no tiene revisión G5 manual del Preparador (usa --qa-pass=true|false y --qa-notes=\"...\" para registrarla).")
	}

	warnIfNoGit(cfg.Out, ws, repo)
	auditErr := eng.auditStep(ctx, st, i)
	if auditErr == nil {
		if st.Index >= len(st.Chunks) {
			st.Status = "DONE"
			eng.event(st, "MOTOR", "FIN", "", "todos los chunks aprobados")
			if err := eng.save(st); err != nil {
				return err
			}
		} else if head, headErr := ws.Head(ctx); headErr == nil {
			saveBaseRef(cfg.TaskDir, head)
		}
	}

	if syncErr := SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, prevEvents); syncErr != nil {
		return syncErr
	}
	return auditErr
}

// ShowStatus imprime el resumen del estado actual y regenera status.md.
func ShowStatus(cfg CLIConfig) error {
	cfg.normalize()
	store := cfg.store()
	st, err := store.Load()
	if err != nil {
		return fmt.Errorf("cargando estado: %w", err)
	}
	if st == nil {
		task, scope, chunks, maxRetries, parseErr := ParseTaskFromDir(cfg.TaskDir)
		if parseErr != nil {
			return parseErr
		}
		st = &State{
			TaskID:     task.ID,
			Prompt:     task.Prompt,
			Scope:      scope,
			MaxRetries: maxRetries,
			Status:     "UNINITIALIZED",
			Chunks:     chunks,
		}
		fmt.Fprintf(cfg.Out, "Tarea %s sin inicializar (.ring\\state.json ausente).\n", st.TaskID)
	}

	fmt.Fprintf(cfg.Out, "Tarea: %s | Estado: %s | Plan aprobado: %v | Chunk activo: %d/%d\n",
		st.TaskID, st.Status, st.PlanApproved, minInt(st.Index+1, len(st.Chunks)), len(st.Chunks))
	for idx, c := range st.Chunks {
		marker := " "
		if idx == st.Index && st.Status == "RUNNING" {
			marker = ">"
		}
		fmt.Fprintf(cfg.Out, "%s [%s] %s (%s) — intentos: %d\n", marker, c.Status, c.ID, firstLine(c.Spec), len(c.Attempts))
	}
	return SyncFiles(cfg.GengisRoot, cfg.TaskDir, st, len(st.Events))
}

func applyManualQAOverride(eng *Engine, st *State, c *Chunk, att *Attempt, opts DecideOptions) error {
	if opts.HasQAPass {
		att.PrepPassed = opts.QAPass
	}
	if opts.HasQANotes {
		att.PrepNotes = opts.QANotes
	}
	return eng.applyQAResult(st, c, att)
}

func printPlanSummary(out io.Writer, st *State, repo string) {
	fmt.Fprintf(out, "=== Plan de Tarea: %s ===\n", st.TaskID)
	fmt.Fprintf(out, "Repositorio destino: %s\n", orDash(repo))
	fmt.Fprintf(out, "Alcance — Modificar: %v | Lectura: %v | Prohibido: %v\n",
		st.Scope.CanModify, st.Scope.ReadOnly, st.Scope.Forbidden)
	fmt.Fprintf(out, "Máx. intentos por chunk: %d | Chunks: %d\n", st.MaxRetries, len(st.Chunks))
	for i, c := range st.Chunks {
		fmt.Fprintf(out, "  %d. %s: %s (verif=%d, failureProof=%v)\n",
			i+1, c.ID, firstLine(c.Spec), len(c.VerifyCommands), c.FailureProof != "")
	}
	if strings.TrimSpace(repo) == "" {
		fmt.Fprintln(out, "ADVERTENCIA: no se detectó repositorio destino en index.md → los comandos de verificación correrán en la carpeta de la tarea.")
	}
	if len(st.Scope.CanModify) == 0 {
		fmt.Fprintln(out, "ADVERTENCIA: 'Puede modificar' quedó vacío en ## Alcance → ninguna ruta del repositorio podrá modificarse.")
	}
	if abs := collectAbsoluteScopePaths(st.Scope); len(abs) > 0 {
		fmt.Fprintf(out, "ADVERTENCIA: rutas absolutas fuera del repositorio destino en ## Alcance (no coinciden con rutas relativas de Git): %v\n", abs)
	}
}

func isAbsoluteScopePath(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}
	return len(p) >= 2 && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) && p[1] == ':'
}

func collectAbsoluteScopePaths(sc Scope) []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range [][]string{sc.CanModify, sc.ReadOnly, sc.Forbidden} {
		for _, p := range list {
			if isAbsoluteScopePath(p) && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ------------------------------------------------------------ Implementaciones interactivas

type cliAuditor struct {
	in  *bufio.Scanner
	out io.Writer
}

func (a *cliAuditor) ReviewPlan(_ context.Context, req PlanRequest) (PlanVerdict, error) {
	fmt.Fprintf(a.out, "¿Aprobar el plan de %s? [s/N]: ", req.TaskID)
	ans, ok := readLineOptional(a.in)
	if !ok {
		return PlanVerdict{Approved: false, Comment: "sin respuesta en consola"}, nil
	}
	approved := parseYes(ans)
	fmt.Fprint(a.out, "Motivo / comentario del Autor (opcional): ")
	comment, _ := readLineOptional(a.in)
	return PlanVerdict{Approved: approved, Comment: comment}, nil
}

func (a *cliAuditor) Review(_ context.Context, req ReviewRequest) (Verdict, error) {
	fmt.Fprintf(a.out, "\nChunk %s — Intento %d | QA: %s (%s)\n",
		req.Chunk.ID, req.Attempt.N, qaResultWord(req.Attempt.QAPassed), orDash(req.Attempt.QANotes))
	fmt.Fprintln(a.out, "Decisiones permitidas para el [AUTOR]:")
	for idx, d := range req.Allowed {
		fmt.Fprintf(a.out, "  %d) %s\n", idx+1, d)
	}

	var chosen Decision
	for {
		fmt.Fprint(a.out, "Decisión [nombre o número]: ")
		line, ok := readLineOptional(a.in)
		if !ok {
			return Verdict{}, errors.New("entrada estándar cerrada antes de recibir la decisión del Autor")
		}
		if d, valid := parseDecisionInput(line, req.Allowed); valid {
			chosen = d
			break
		}
		fmt.Fprintf(a.out, "Decisión %q no permitida en este punto. Opciones: %v\n", line, req.Allowed)
	}

	revert := false
	if chosen == RetryChunk {
		if len(req.Attempt.ScopeViolations) > 0 {
			fmt.Fprintln(a.out, "Nota: hay archivos fuera de alcance; el motor los limpiará con git stash aunque no reviertas.")
		}
		fmt.Fprint(a.out, "¿Aplicar git revert antes de repetir el chunk? [s/N]: ")
		if revAns, ok := readLineOptional(a.in); ok {
			revert = parseYes(revAns)
		}
	}

	fmt.Fprint(a.out, "Motivo del Autor (opcional): ")
	comment, _ := readLineOptional(a.in)
	return Verdict{Decision: chosen, Comment: comment, Revert: revert}, nil
}

type cliPreparador struct {
	in          *bufio.Scanner
	out         io.Writer
	taskDir     string
	targetRepo  string
	existingIDs map[string]bool
}

func (p *cliPreparador) Split(context.Context, Task) ([]Chunk, error) {
	return nil, errors.New("no implementado fuera de ParseTaskFromDir")
}

const autoVerifyQANote = "verificación automática OK (G5 pendiente: usar decide --qa-pass)"

func (p *cliPreparador) Verify(_ context.Context, _ Chunk, _ Attempt) (bool, string, error) {
	return true, autoVerifyQANote, nil
}

func (p *cliPreparador) Resplit(_ context.Context, c Chunk) ([]Chunk, error) {
	fmt.Fprintf(p.out, "\n[PREPARADOR] RESPLIT_CHUNK para %s.\n", c.ID)
	fmt.Fprint(p.out, "Número de nuevos subchunks (o 'dir' para leer nuevos archivos en chunks\\): ")
	mode, ok := readLineOptional(p.in)
	if !ok {
		return nil, errors.New("entrada estándar cerrada durante RESPLIT_CHUNK")
	}

	if strings.EqualFold(mode, "dir") || mode == "" {
		all, err := parseChunksDir(p.taskDir, p.targetRepo)
		if err != nil {
			return nil, err
		}
		var added []Chunk
		for _, ch := range all {
			if !p.existingIDs[ch.ID] {
				added = append(added, ch)
			}
		}
		if len(added) == 0 {
			return nil, fmt.Errorf("no se encontraron nuevos archivos de chunk en %s\\chunks", p.taskDir)
		}
		return added, nil
	}

	n, err := strconv.Atoi(mode)
	if err != nil || n < 1 {
		return nil, fmt.Errorf("cantidad de subchunks inválida %q", mode)
	}
	subs := make([]Chunk, 0, n)
	for k := 1; k <= n; k++ {
		id, ok := promptRequired(p.in, p.out, fmt.Sprintf("Subchunk %d ID: ", k))
		if !ok {
			return nil, errors.New("entrada cerrada definiendo subchunk")
		}
		spec, ok := promptRequired(p.in, p.out, fmt.Sprintf("Subchunk %d Spec: ", k))
		if !ok {
			return nil, errors.New("entrada cerrada definiendo Spec")
		}
		acc, ok := promptRequired(p.in, p.out, fmt.Sprintf("Subchunk %d Criterio de aceptación: ", k))
		if !ok {
			return nil, errors.New("entrada cerrada definiendo criterio de aceptación")
		}
		cmd, ok := promptRequired(p.in, p.out, fmt.Sprintf("Subchunk %d Comando de verificación: ", k))
		if !ok {
			return nil, errors.New("entrada cerrada definiendo comando")
		}
		fp, ok := promptRequired(p.in, p.out, fmt.Sprintf("Subchunk %d Prueba de que puede fallar: ", k))
		if !ok {
			return nil, errors.New("entrada cerrada definiendo prueba de fallo")
		}
		subs = append(subs, Chunk{
			ID:             id,
			Spec:           spec,
			Acceptance:     []string{acc},
			VerifyCommands: []string{cmd},
			FailureProof:   fp,
			Status:         StatusPending,
		})
	}
	return subs, nil
}

func promptRequired(in *bufio.Scanner, out io.Writer, label string) (string, bool) {
	fmt.Fprint(out, label)
	return readLineOptional(in)
}

func readLineOptional(in *bufio.Scanner) (string, bool) {
	if in == nil || !in.Scan() {
		return "", false
	}
	return strings.TrimSpace(in.Text()), true
}

func parseYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "s", "si", "sí", "y", "yes":
		return true
	default:
		return false
	}
}

func parseDecisionInput(raw string, allowed []Decision) (Decision, bool) {
	s := strings.TrimSpace(raw)
	if idx, err := strconv.Atoi(s); err == nil && idx >= 1 && idx <= len(allowed) {
		return allowed[idx-1], true
	}
	norm := strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
	for _, d := range allowed {
		if d.String() == norm {
			return d, true
		}
	}
	return 0, false
}
