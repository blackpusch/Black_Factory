package brain

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"blackfactory/pkg/bfcore"
)

// QueryOptions parametriza una busqueda FTS5/BM25 o filtrada por metadatos sobre brain.db.
type QueryOptions struct {
	Query   string `json:"query"`
	Project string `json:"project,omitempty"`
	Type    string `json:"type,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// RiskZoneEntry representa una fila parseada de la tabla "## Catálogo de Zonas de Riesgo" en wiki/risk_zones.md.
type RiskZoneEntry struct {
	ID            string   `json:"id"`
	Project       string   `json:"project"`
	CoversPaths   []string `json:"covers_paths"`
	Level         string   `json:"level"`
	MandatoryGate string   `json:"mandatory_gate"`
	EvidenceG5    string   `json:"evidence_g5,omitempty"`
}

// RiskZoneHit representa una coincidencia estructurada de una zona de riesgo contra los archivos consultados.
type RiskZoneHit struct {
	ID            string   `json:"id"`
	Project       string   `json:"project"`
	Level         string   `json:"level"`
	MatchedPaths  []string `json:"matched_paths"`
	MandatoryGate string   `json:"mandatory_gate"`
}

// SearchResult representa un documento devuelto por Search o ContextForFiles.
type SearchResult struct {
	ID                  string        `json:"id"`
	RelPath             string        `json:"rel_path"`
	Title               string        `json:"title"`
	Type                string        `json:"type"`
	Project             string        `json:"project"`
	Status              string        `json:"status"`
	Tags                []string      `json:"tags,omitempty"`
	CoversPaths         []string      `json:"covers_paths,omitempty"`
	VerifiedHead        string        `json:"verified_head,omitempty"`
	UpdatedAt           string        `json:"updated_at,omitempty"`
	Score               float64       `json:"score"`
	Snippet             string        `json:"snippet,omitempty"`
	MatchedPaths        []string      `json:"matched_paths,omitempty"`
	Transversal         bool          `json:"transversal,omitempty"`
	RiskHits            []RiskZoneHit `json:"risk_hits,omitempty"`
	RiskZonesSHA256     string        `json:"risk_zones_sha256,omitempty"`
	RiskZonesParseError string        `json:"risk_zones_parse_error,omitempty"`
	rawBody             string
}

// Search consulta el indice SQLite FTS5 aplicando ranking BM25 y filtros opcionales.
func Search(dbPath string, opts QueryOptions) ([]SearchResult, error) {
	db, err := openReadOnlyDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	rawQuery := strings.TrimSpace(opts.Query)
	results := make([]SearchResult, 0)
	if rawQuery == "" {
		results, err = searchByMetadata(db, opts, limit)
	} else {
		ftsExpr := buildFTS5Query(rawQuery, false)
		results, err = searchWithFTS5(db, ftsExpr, opts, limit)
		if err != nil {
			fallbackExpr := buildFTS5Query(rawQuery, true)
			if fallbackExpr != "" && fallbackExpr != ftsExpr {
				if retryRes, retryErr := searchWithFTS5(db, fallbackExpr, opts, limit); retryErr == nil {
					results = retryRes
					err = nil
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = make([]SearchResult, 0)
	}

	if err := hydrateResultRelations(db, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ContextForFiles cruza archivos modificados contra covers_paths (insensible a mayusculas via bfcore.MatchCoverPathFold),
// incluye automaticamente documentos transversales (constitution y risk_zones), y evalua las zonas de riesgo de wiki/risk_zones.md.
func ContextForFiles(dbPath string, project string, files []string) ([]SearchResult, error) {
	db, err := openReadOnlyDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	normFiles := dedupeCoverPaths(files)
	allDocs, err := loadIndexedDocs(db)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0)
	for _, item := range allDocs {
		isTransversal := isTransversalType(item.Type)
		if isTransversal {
			if !transversalAppliesToProject(item.Project, project) {
				continue
			}
			item.Transversal = true
			item.MatchedPaths = matchFilesAgainstCovers(item.CoversPaths, normFiles)
			if strings.EqualFold(item.Type, "risk_zones") {
				item.RiskZonesSHA256 = sha256Hex(normalizeMarkdownContent(item.rawBody))
				entries, parseErr := ParseRiskZonesCatalog(item.rawBody)
				if parseErr != nil {
					item.RiskZonesParseError = parseErr.Error()
				} else {
					hits, hitPaths := evaluateRiskZoneEntries(entries, project, normFiles)
					item.RiskHits = hits
					if len(hitPaths) > 0 {
						item.MatchedPaths = mergeSortedUniqueStrings(item.MatchedPaths, hitPaths)
					}
				}
			}
			item.rawBody = ""
			results = append(results, item)
			continue
		}

		if strings.TrimSpace(project) != "" && !strings.EqualFold(item.Project, strings.TrimSpace(project)) {
			continue
		}
		matched := matchFilesAgainstCovers(item.CoversPaths, normFiles)
		if len(matched) == 0 {
			continue
		}
		item.MatchedPaths = matched
		item.rawBody = ""
		results = append(results, item)
	}

	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Transversal != b.Transversal {
			return a.Transversal
		}
		if a.Transversal && b.Transversal {
			pa, pb := transversalRank(a.Type), transversalRank(b.Type)
			if pa != pb {
				return pa < pb
			}
		}
		if len(a.MatchedPaths) != len(b.MatchedPaths) {
			return len(a.MatchedPaths) > len(b.MatchedPaths)
		}
		return a.RelPath < b.RelPath
	})

	return results, nil
}

// ParseRiskZonesCatalog parsea la tabla de 6 columnas bajo "## Catálogo de Zonas de Riesgo"
// en wiki/risk_zones.md, tolerando backticks en los encabezados y en cada celda,
// separando multiples patrones por ',' y ';', y normalizando CRÍTICO -> CRITICO.
func ParseRiskZonesCatalog(markdown string) ([]RiskZoneEntry, error) {
	lines := strings.Split(normalizeMarkdownContent(markdown), "\n")
	inSection := false
	headerFound := false
	var entries []RiskZoneEntry

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if strings.HasPrefix(trimmed, "## ") {
			headingLower := strings.ToLower(trimmed)
			if strings.Contains(headingLower, "zonas de riesgo") ||
				strings.Contains(headingLower, "catalogo de zonas") ||
				strings.Contains(headingLower, "catálogo de zonas") {
				inSection = true
				continue
			}
			if inSection && headerFound {
				break
			}
		}

		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
			continue
		}

		cells := splitMarkdownTableRow(trimmed)
		if len(cells) < 5 {
			continue
		}

		if isMarkdownSeparatorRow(cells) {
			continue
		}

		c0 := strings.ToLower(stripCellBackticks(cells[0]))
		c1 := strings.ToLower(stripCellBackticks(cells[1]))
		if c0 == "id" && strings.Contains(c1, "proyecto") {
			headerFound = true
			inSection = true
			continue
		}
		if !headerFound {
			continue
		}

		id := stripCellBackticks(cells[0])
		proj := stripCellBackticks(cells[1])
		rawPaths := cells[2]
		rawLevel := stripCellBackticks(cells[3])
		gate := strings.TrimSpace(cells[4])
		evidence := ""
		if len(cells) >= 6 {
			evidence = stripCellBackticks(cells[5])
		}

		if id == "" || proj == "" {
			return nil, fmt.Errorf("fila de zona de riesgo con ID o Proyecto vacio: %q", trimmed)
		}

		normLevel, err := normalizeRiskLevel(rawLevel)
		if err != nil {
			return nil, fmt.Errorf("fila %s: %w", id, err)
		}

		covers := parseRiskCoversCell(rawPaths)
		if len(covers) == 0 {
			return nil, fmt.Errorf("fila %s carece de rutas cubiertas (covers_paths)", id)
		}

		entries = append(entries, RiskZoneEntry{
			ID:            id,
			Project:       proj,
			CoversPaths:   covers,
			Level:         normLevel,
			MandatoryGate: gate,
			EvidenceG5:    evidence,
		})
	}

	if !headerFound || len(entries) == 0 {
		return nil, errors.New("tabla '## Catálogo de Zonas de Riesgo' ausente o sin entradas validas en risk_zones.md")
	}
	return entries, nil
}

func splitMarkdownTableRow(trimmed string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|")
	parts := strings.Split(inner, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func isMarkdownSeparatorRow(cells []string) bool {
	for _, c := range cells {
		clean := strings.Trim(c, " :-")
		if clean != "" {
			return false
		}
	}
	return true
}

func stripCellBackticks(s string) string {
	t := strings.TrimSpace(s)
	t = strings.Trim(t, "`")
	return strings.TrimSpace(t)
}

func parseRiskCoversCell(raw string) []string {
	replaced := strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(replaced, ",")
	var covers []string
	seen := make(map[string]bool)
	for _, p := range parts {
		c := stripCellBackticks(p)
		norm := bfcore.NormalizeCoverPath(c)
		if norm == "" {
			continue
		}
		key := strings.ToLower(norm)
		if !seen[key] {
			seen[key] = true
			covers = append(covers, c)
		}
	}
	return covers
}

func normalizeRiskLevel(raw string) (string, error) {
	up := strings.ToUpper(strings.TrimSpace(raw))
	up = strings.ReplaceAll(up, "Í", "I")
	switch up {
	case "CRITICO", "ALTO", "MEDIO", "BAJO":
		return up, nil
	default:
		return "", fmt.Errorf("nivel de riesgo desconocido %q (permitidos: CRITICO, ALTO, MEDIO, BAJO)", raw)
	}
}

func evaluateRiskZoneEntries(entries []RiskZoneEntry, targetProject string, normFiles []string) ([]RiskZoneHit, []string) {
	if len(entries) == 0 || len(normFiles) == 0 {
		return nil, nil
	}
	tp := strings.TrimSpace(targetProject)
	var hits []RiskZoneHit
	allMatchedSet := make(map[string]bool)
	var allMatched []string

	for _, entry := range entries {
		ep := strings.TrimSpace(entry.Project)
		if tp != "" && ep != "" && ep != "*" && !strings.EqualFold(ep, tp) {
			continue
		}
		matched := matchFilesAgainstCovers(entry.CoversPaths, normFiles)
		if len(matched) == 0 {
			continue
		}
		hits = append(hits, RiskZoneHit{
			ID:            entry.ID,
			Project:       entry.Project,
			Level:         entry.Level,
			MatchedPaths:  matched,
			MandatoryGate: entry.MandatoryGate,
		})
		for _, m := range matched {
			if !allMatchedSet[m] {
				allMatchedSet[m] = true
				allMatched = append(allMatched, m)
			}
		}
	}
	sort.Strings(allMatched)
	return hits, allMatched
}

func mergeSortedUniqueStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var out []string
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func openReadOnlyDB(dbPath string) (*sql.DB, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		return nil, fmt.Errorf("abriendo indice %s: %w", dbPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s es un directorio, no un archivo SQLite", dbPath)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("abriendo sqlite %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA query_only = ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configurando solo lectura en %s: %w", dbPath, err)
	}
	return db, nil
}

func appendMetadataFilters(where *[]string, args *[]any, opts QueryOptions, alias string) {
	if p := strings.TrimSpace(opts.Project); p != "" {
		*where = append(*where, fmt.Sprintf("LOWER(%s.project) = LOWER(?)", alias))
		*args = append(*args, p)
	}
	if t := strings.TrimSpace(opts.Type); t != "" {
		*where = append(*where, fmt.Sprintf("LOWER(%s.doc_type) = LOWER(?)", alias))
		*args = append(*args, t)
	}
	if tg := strings.TrimSpace(opts.Tag); tg != "" {
		*where = append(*where, fmt.Sprintf("EXISTS (SELECT 1 FROM doc_tags dt WHERE dt.doc_id = %s.id AND LOWER(dt.tag) = LOWER(?))", alias))
		*args = append(*args, tg)
	}
}

func searchWithFTS5(db *sql.DB, ftsExpr string, opts QueryOptions, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(ftsExpr) == "" {
		return searchByMetadata(db, opts, limit)
	}

	where := []string{"docs_fts MATCH ?"}
	args := []any{ftsExpr}
	appendMetadataFilters(&where, &args, opts, "d")
	args = append(args, limit)

	querySQL := `
		SELECT d.id, d.rel_path, d.title, d.doc_type, d.project, d.status,
		       d.verified_head, d.updated_at,
		       bm25(docs_fts, 0.0, 0.0, 5.0, 2.0, 1.0) AS score,
		       snippet(docs_fts, 4, '[', ']', '...', 24) AS snip,
		       d.body
		FROM docs_fts
		JOIN docs d ON d.id = docs_fts.doc_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY score ASC, d.rel_path ASC
		LIMIT ?`

	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("consultando FTS5: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0)
	for rows.Next() {
		var r SearchResult
		var snip, body string
		if err := rows.Scan(
			&r.ID, &r.RelPath, &r.Title, &r.Type, &r.Project, &r.Status,
			&r.VerifiedHead, &r.UpdatedAt, &r.Score, &snip, &body,
		); err != nil {
			return nil, err
		}
		r.Snippet = cleanSnippet(snip, body, r.Title)
		results = append(results, r)
	}
	return results, rows.Err()
}

func searchByMetadata(db *sql.DB, opts QueryOptions, limit int) ([]SearchResult, error) {
	var where []string
	var args []any
	appendMetadataFilters(&where, &args, opts, "d")

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, limit)

	querySQL := `
		SELECT d.id, d.rel_path, d.title, d.doc_type, d.project, d.status,
		       d.verified_head, d.updated_at, d.body
		FROM docs d
		` + whereClause + `
		ORDER BY d.rel_path ASC
		LIMIT ?`

	rows, err := db.Query(querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("consultando documentos: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0)
	for rows.Next() {
		var r SearchResult
		var body string
		if err := rows.Scan(
			&r.ID, &r.RelPath, &r.Title, &r.Type, &r.Project, &r.Status,
			&r.VerifiedHead, &r.UpdatedAt, &body,
		); err != nil {
			return nil, err
		}
		r.Snippet = cleanSnippet("", body, r.Title)
		results = append(results, r)
	}
	return results, rows.Err()
}

func loadIndexedDocs(db *sql.DB) ([]SearchResult, error) {
	rows, err := db.Query(`
		SELECT id, rel_path, title, doc_type, project, status, verified_head, updated_at, body
		FROM docs
		ORDER BY rel_path ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := make([]SearchResult, 0)
	for rows.Next() {
		var r SearchResult
		var body string
		if err := rows.Scan(
			&r.ID, &r.RelPath, &r.Title, &r.Type, &r.Project, &r.Status,
			&r.VerifiedHead, &r.UpdatedAt, &body,
		); err != nil {
			return nil, err
		}
		r.rawBody = body
		r.Snippet = cleanSnippet("", body, r.Title)
		docs = append(docs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := hydrateResultRelations(db, docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func hydrateResultRelations(db *sql.DB, results []SearchResult) error {
	if len(results) == 0 {
		return nil
	}

	tagsMap, err := loadAllTags(db)
	if err != nil {
		return err
	}
	pathsMap, err := loadAllPaths(db)
	if err != nil {
		return err
	}

	for i := range results {
		id := results[i].ID
		if tgs, ok := tagsMap[id]; ok {
			results[i].Tags = tgs
		}
		if cps, ok := pathsMap[id]; ok {
			results[i].CoversPaths = cps
		}
	}
	return nil
}

func loadAllTags(db *sql.DB) (map[string][]string, error) {
	rows, err := db.Query(`SELECT doc_id, tag FROM doc_tags ORDER BY doc_id ASC, tag ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]string)
	for rows.Next() {
		var docID, tag string
		if err := rows.Scan(&docID, &tag); err != nil {
			return nil, err
		}
		out[docID] = append(out[docID], tag)
	}
	return out, rows.Err()
}

func loadAllPaths(db *sql.DB) (map[string][]string, error) {
	rows, err := db.Query(`SELECT doc_id, cover_path FROM doc_paths ORDER BY doc_id ASC, cover_path ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]string)
	for rows.Next() {
		var docID, cp string
		if err := rows.Scan(&docID, &cp); err != nil {
			return nil, err
		}
		out[docID] = append(out[docID], cp)
	}
	return out, rows.Err()
}

func matchFilesAgainstCovers(covers, normFiles []string) []string {
	if len(covers) == 0 || len(normFiles) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var matched []string
	for _, f := range normFiles {
		for _, pat := range covers {
			if bfcore.MatchCoverPathFold(pat, f) {
				if !seen[f] {
					seen[f] = true
					matched = append(matched, f)
				}
				break
			}
		}
	}
	sort.Strings(matched)
	return matched
}

func IsTransversalType(docType string) bool {
	return strings.EqualFold(docType, "constitution") || strings.EqualFold(docType, "risk_zones")
}

func isTransversalType(docType string) bool {
	return IsTransversalType(docType)
}

func transversalAppliesToProject(docProject, targetProject string) bool {
	tp := strings.TrimSpace(targetProject)
	dp := strings.TrimSpace(docProject)
	if tp == "" || dp == "" {
		return true
	}
	return strings.EqualFold(dp, tp) || strings.EqualFold(dp, "black-factory")
}

func transversalRank(docType string) int {
	if strings.EqualFold(docType, "constitution") {
		return 0
	}
	return 1
}

func cleanSnippet(snip, body, title string) string {
	s := strings.TrimSpace(snip)
	if s != "" {
		return strings.Join(strings.Fields(s), " ")
	}
	compact := strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	if compact == "" {
		return title
	}
	runes := []rune(compact)
	if len(runes) > 160 {
		return string(runes[:157]) + "..."
	}
	return compact
}

func buildFTS5Query(raw string, quoteAll bool) string {
	tokens := splitQueryTokens(raw)
	if len(tokens) == 0 {
		return ""
	}

	var parts []string
	for idx, tok := range tokens {
		if len(tok) >= 2 && strings.HasPrefix(tok, `"`) && strings.HasSuffix(tok, `"`) {
			inner := strings.ReplaceAll(tok[1:len(tok)-1], `"`, `""`)
			if inner != "" {
				parts = append(parts, `"`+inner+`"`)
			}
			continue
		}

		if quoteAll {
			escaped := strings.ReplaceAll(tok, `"`, `""`)
			if escaped != "" {
				parts = append(parts, `"`+escaped+`"`)
			}
			continue
		}

		if isFTSBooleanOp(tok) && idx > 0 && idx < len(tokens)-1 && !isFTSBooleanOp(tokens[idx-1]) {
			parts = append(parts, tok)
			continue
		}

		if strings.HasSuffix(tok, "*") && len(tok) > 1 {
			prefix := tok[:len(tok)-1]
			if isSimpleFTSToken(prefix) && !isFTSBooleanOp(prefix) {
				parts = append(parts, prefix+"*")
				continue
			}
		}

		if isSimpleFTSToken(tok) && !isFTSBooleanOp(tok) {
			parts = append(parts, tok)
			continue
		}

		escaped := strings.ReplaceAll(tok, `"`, `""`)
		if escaped != "" {
			parts = append(parts, `"`+escaped+`"`)
		}
	}
	return strings.Join(parts, " ")
}

func splitQueryTokens(raw string) []string {
	var tokens []string
	var cur strings.Builder
	inQuotes := false

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for _, r := range raw {
		if r == '"' {
			cur.WriteRune(r)
			inQuotes = !inQuotes
			if !inQuotes {
				flush()
			}
			continue
		}
		if unicode.IsSpace(r) && !inQuotes {
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		s := cur.String()
		if inQuotes {
			s = strings.TrimPrefix(s, `"`)
		}
		if s != "" {
			tokens = append(tokens, s)
		}
	}
	return tokens
}

func isFTSBooleanOp(tok string) bool {
	return tok == "AND" || tok == "OR" || tok == "NOT"
}

func isSimpleFTSToken(tok string) bool {
	if tok == "" {
		return false
	}
	for _, r := range tok {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}
