package brain

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// IndexStats resume el resultado de una construccion o reconstruccion del indice SQLite FTS5.
type IndexStats struct {
	DBPath       string `json:"db_path"`
	DocsIndexed  int    `json:"docs_indexed"`
	PathsIndexed int    `json:"paths_indexed"`
	DurationMs   int64  `json:"duration_ms"`
}

// DefaultIndexPath devuelve la ruta canonica del indice derivado dentro de la boveda.
func DefaultIndexPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".index", "brain.db")
}

// NormalizeCoverPath unifica separadores a "/" y elimina segmentos o barras redundantes
// para comparar rutas Windows y POSIX de forma determinista.
func NormalizeCoverPath(p string) string {
	s := strings.TrimSpace(p)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, `\`, "/")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	s = path.Clean(s)
	for strings.HasPrefix(s, "./") {
		s = strings.TrimPrefix(s, "./")
	}
	s = strings.Trim(s, "/")
	if s == "." {
		return ""
	}
	return s
}

// MatchCoverPath evalua si filePath coincide con un patron de covers_paths
// (ruta exacta, prefijo de directorio o patron glob con * / ** / ?).
func MatchCoverPath(pattern, filePath string) bool {
	pat := NormalizeCoverPath(pattern)
	file := NormalizeCoverPath(filePath)
	if pat == "" || file == "" {
		return false
	}
	if pat == "*" || pat == "**" {
		return true
	}
	if !strings.ContainsAny(pat, "*?[") {
		return file == pat || strings.HasPrefix(file, pat+"/")
	}
	return matchPathSegments(strings.Split(pat, "/"), strings.Split(file, "/"))
}

func matchPathSegments(patSegs, fileSegs []string) bool {
	m, n := len(patSegs), len(fileSegs)
	dp := make([][]bool, m+1)
	for i := range dp {
		dp[i] = make([]bool, n+1)
	}
	dp[m][n] = true

	for i := m - 1; i >= 0; i-- {
		if patSegs[i] == "**" {
			dp[i][n] = dp[i+1][n]
		}
		for j := n - 1; j >= 0; j-- {
			if patSegs[i] == "**" {
				dp[i][j] = dp[i+1][j] || dp[i][j+1]
				continue
			}
			ok, err := path.Match(patSegs[i], fileSegs[j])
			if err == nil && ok {
				dp[i][j] = dp[i+1][j+1]
			}
		}
	}
	return dp[0][0]
}

// BuildIndex construye o reconstruye de forma atomica el indice .index/brain.db de vaultRoot.
func BuildIndex(vaultRoot string) (IndexStats, error) {
	return BuildIndexAt(vaultRoot, DefaultIndexPath(vaultRoot))
}

// BuildIndexAt construye o reconstruye de forma atomica el indice SQLite FTS5 en dbPath.
func BuildIndexAt(vaultRoot, dbPath string) (IndexStats, error) {
	start := time.Now()

	absVault, err := filepath.Abs(vaultRoot)
	if err != nil {
		return IndexStats{}, err
	}
	info, err := os.Stat(absVault)
	if err != nil {
		return IndexStats{}, fmt.Errorf("accediendo a boveda %s: %w", absVault, err)
	}
	if !info.IsDir() {
		return IndexStats{}, fmt.Errorf("%s no es un directorio", absVault)
	}

	docs, err := collectIndexableDocs(absVault)
	if err != nil {
		return IndexStats{}, err
	}

	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		return IndexStats{}, err
	}
	dbDir := filepath.Dir(absDB)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return IndexStats{}, fmt.Errorf("creando directorio de indice %s: %w", dbDir, err)
	}

	tmpFile, err := os.CreateTemp(dbDir, ".brain-index-*.tmp")
	if err != nil {
		return IndexStats{}, fmt.Errorf("creando base temporal: %w", err)
	}
	tmpPath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return IndexStats{}, err
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
			removeSQLiteSidecars(tmpPath)
		}
	}()

	pathsIndexed, err := writeSQLiteIndex(tmpPath, docs)
	if err != nil {
		return IndexStats{}, err
	}

	removeSQLiteSidecars(tmpPath)
	removeSQLiteSidecars(absDB)
	if err := replaceFileAtomic(tmpPath, absDB); err != nil {
		return IndexStats{}, fmt.Errorf("promoviendo indice %s: %w", absDB, err)
	}
	cleanupTmp = false

	return IndexStats{
		DBPath:       absDB,
		DocsIndexed:  len(docs),
		PathsIndexed: pathsIndexed,
		DurationMs:   time.Since(start).Milliseconds(),
	}, nil
}

func collectIndexableDocs(absVault string) ([]Document, error) {
	files, err := ListVaultMarkdownFiles(absVault)
	if err != nil {
		return nil, err
	}

	docs := make([]Document, 0, len(files))
	seenIDs := make(map[string]string)

	for _, absFile := range files {
		doc, err := ParseFile(absVault, absFile)
		if err != nil {
			return nil, fmt.Errorf("parseando %s: %w", absFile, err)
		}
		if !doc.HasFrontmatter || strings.HasPrefix(doc.RelPath, "_proposals/") {
			continue
		}
		docID := strings.TrimSpace(doc.Meta.ID)
		if docID == "" {
			docID = doc.RelPath
		}
		idKey := strings.ToLower(docID)
		if prev, dup := seenIDs[idKey]; dup {
			return nil, fmt.Errorf("id duplicado %q al indexar (%s y %s)", docID, prev, doc.RelPath)
		}
		seenIDs[idKey] = doc.RelPath
		doc.Meta.ID = docID
		docs = append(docs, doc)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].RelPath < docs[j].RelPath
	})
	return docs, nil
}

func writeSQLiteIndex(dbPath string, docs []Document) (int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("abriendo sqlite temporal: %w", err)
	}
	db.SetMaxOpenConns(1)

	closed := false
	defer func() {
		if !closed {
			_ = db.Close()
		}
	}()

	for _, pragma := range []string{
		`PRAGMA journal_mode = DELETE;`,
		`PRAGMA synchronous = OFF;`,
		`PRAGMA temp_store = MEMORY;`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			return 0, fmt.Errorf("configurando %s: %w", pragma, err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("iniciando transaccion sqlite: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	schemaDDL := []string{
		`CREATE TABLE docs (
			id TEXT PRIMARY KEY,
			rel_path TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			doc_type TEXT NOT NULL,
			project TEXT NOT NULL,
			status TEXT NOT NULL,
			tags TEXT NOT NULL,
			verified_head TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			body TEXT NOT NULL
		);`,
		`CREATE INDEX idx_docs_project ON docs(project);`,
		`CREATE INDEX idx_docs_type ON docs(doc_type);`,
		`CREATE TABLE doc_tags (
			doc_id TEXT NOT NULL,
			tag TEXT NOT NULL,
			PRIMARY KEY(doc_id, tag)
		);`,
		`CREATE INDEX idx_doc_tags_tag ON doc_tags(tag);`,
		`CREATE TABLE doc_paths (
			doc_id TEXT NOT NULL,
			cover_path TEXT NOT NULL,
			PRIMARY KEY(doc_id, cover_path)
		);`,
		`CREATE INDEX idx_doc_paths_path ON doc_paths(cover_path);`,
		`CREATE VIRTUAL TABLE docs_fts USING fts5(
			doc_id UNINDEXED,
			rel_path UNINDEXED,
			title,
			tags,
			body,
			tokenize = 'unicode61 remove_diacritics 2'
		);`,
	}
	for _, stmt := range schemaDDL {
		if _, err := tx.Exec(stmt); err != nil {
			return 0, fmt.Errorf("creando esquema sqlite: %w", err)
		}
	}

	insDoc, err := tx.Prepare(`INSERT INTO docs(id, rel_path, title, doc_type, project, status, tags, verified_head, updated_at, body) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insDoc.Close()

	insTag, err := tx.Prepare(`INSERT INTO doc_tags(doc_id, tag) VALUES(?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insTag.Close()

	insPath, err := tx.Prepare(`INSERT INTO doc_paths(doc_id, cover_path) VALUES(?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insPath.Close()

	insFTS, err := tx.Prepare(`INSERT INTO docs_fts(doc_id, rel_path, title, tags, body) VALUES(?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer insFTS.Close()

	pathsIndexed := 0
	for _, doc := range docs {
		tags := dedupeTrimmed(doc.Meta.Tags)
		tagsJoined := strings.Join(tags, " ")

		if _, err := insDoc.Exec(
			doc.Meta.ID,
			doc.RelPath,
			doc.Meta.Title,
			doc.Meta.Type,
			doc.Meta.Project,
			doc.Meta.Status,
			strings.Join(tags, ","),
			doc.Meta.VerifiedHead,
			doc.Meta.UpdatedAt,
			doc.Body,
		); err != nil {
			return 0, fmt.Errorf("insertando doc %s: %w", doc.RelPath, err)
		}

		for _, tg := range tags {
			if _, err := insTag.Exec(doc.Meta.ID, tg); err != nil {
				return 0, fmt.Errorf("insertando tag %q en %s: %w", tg, doc.RelPath, err)
			}
		}

		covers := dedupeCoverPaths(doc.Meta.CoversPaths)
		for _, cp := range covers {
			if _, err := insPath.Exec(doc.Meta.ID, cp); err != nil {
				return 0, fmt.Errorf("insertando cover_path %q en %s: %w", cp, doc.RelPath, err)
			}
			pathsIndexed++
		}

		if _, err := insFTS.Exec(doc.Meta.ID, doc.RelPath, doc.Meta.Title, tagsJoined, doc.Body); err != nil {
			return 0, fmt.Errorf("insertando fts %s: %w", doc.RelPath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("confirmando transaccion sqlite: %w", err)
	}
	tx = nil

	if err := db.Close(); err != nil {
		return 0, fmt.Errorf("cerrando sqlite temporal: %w", err)
	}
	closed = true
	return pathsIndexed, nil
}

func dedupeTrimmed(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, item := range items {
		t := strings.TrimSpace(item)
		if t == "" {
			continue
		}
		k := strings.ToLower(t)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}

func dedupeCoverPaths(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, item := range items {
		norm := NormalizeCoverPath(item)
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, norm)
	}
	return out
}

func removeSQLiteSidecars(dbPath string) {
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	_ = os.Remove(dbPath + "-journal")
}

func replaceFileAtomic(tmpPath, dstPath string) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lastErr = os.Rename(tmpPath, dstPath)
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, os.ErrPermission) {
			return lastErr
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return lastErr
}
