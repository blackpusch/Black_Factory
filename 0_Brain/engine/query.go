package brain

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
)

// QueryOptions parametriza una busqueda FTS5/BM25 o filtrada por metadatos sobre brain.db.
type QueryOptions struct {
	Query   string `json:"query"`
	Project string `json:"project,omitempty"`
	Type    string `json:"type,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// SearchResult representa un documento devuelto por Search o ContextForFiles.
type SearchResult struct {
	ID           string   `json:"id"`
	RelPath      string   `json:"rel_path"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Project      string   `json:"project"`
	Status       string   `json:"status"`
	Tags         []string `json:"tags,omitempty"`
	CoversPaths  []string `json:"covers_paths,omitempty"`
	VerifiedHead string   `json:"verified_head,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
	Score        float64  `json:"score"`
	Snippet      string   `json:"snippet,omitempty"`
	MatchedPaths []string `json:"matched_paths,omitempty"`
	Transversal  bool     `json:"transversal,omitempty"`
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
	var results []SearchResult
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

	if err := hydrateResultRelations(db, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ContextForFiles cruza archivos modificados contra covers_paths e incluye
// automaticamente documentos transversales (constitution y risk_zones).
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

	var results []SearchResult
	for _, item := range allDocs {
		isTransversal := isTransversalType(item.Type)
		if isTransversal {
			if !transversalAppliesToProject(item.Project, project) {
				continue
			}
			item.Transversal = true
			item.MatchedPaths = matchFilesAgainstCovers(item.CoversPaths, normFiles)
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

	var results []SearchResult
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

	var results []SearchResult
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

	var docs []SearchResult
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
			if MatchCoverPath(pat, f) {
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

func isTransversalType(docType string) bool {
	return strings.EqualFold(docType, "constitution") || strings.EqualFold(docType, "risk_zones")
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
	if len(compact) > 160 {
		return compact[:157] + "..."
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

		if !quoteAll && isFTSBooleanOp(tok) && idx > 0 && idx < len(tokens)-1 && !isFTSBooleanOp(tokens[idx-1]) {
			parts = append(parts, tok)
			continue
		}

		if !quoteAll && strings.HasSuffix(tok, "*") && len(tok) > 1 {
			prefix := tok[:len(tok)-1]
			if isSimpleFTSToken(prefix) {
				parts = append(parts, prefix+"*")
				continue
			}
		}

		if isSimpleFTSToken(tok) {
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
