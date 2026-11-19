package brain

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func indexTestDoc(id, title, docType, project, status string, tags, covers []string, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + id + "\n")
	b.WriteString(fmt.Sprintf("title: %q\n", title))
	b.WriteString("type: " + docType + "\n")
	b.WriteString("project: " + project + "\n")
	b.WriteString("status: " + status + "\n")
	if len(tags) == 0 {
		b.WriteString("tags: []\n")
	} else {
		b.WriteString("tags:\n")
		for _, tg := range tags {
			b.WriteString("  - " + tg + "\n")
		}
	}
	if len(covers) == 0 {
		b.WriteString("covers_paths: []\n")
	} else {
		b.WriteString("covers_paths:\n")
		for _, cp := range covers {
			b.WriteString(fmt.Sprintf("  - %q\n", cp))
		}
	}
	b.WriteString("verified_head: \"87a1b54\"\n")
	b.WriteString("updated_at: \"2026-11-19\"\n")
	b.WriteString("---\n\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

func TestBuildIndex_IdempotentAndNoDuplicates(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "AGENTS.md", indexTestDoc("brain-agents", "Enrutador", "agents", "black-factory", "active", []string{"agents"}, nil, "Protocolo Read-Before-Act."))
	writeTestFile(t, root, "index.md", indexTestDoc("brain-index", "Indice", "index", "black-factory", "active", []string{"index"}, nil, "Indice principal."))
	writeTestFile(t, root, "constitution.md", indexTestDoc("brain-constitution", "Constitucion", "constitution", "black-factory", "active", []string{"governance"}, nil, "Reglas innegociables."))
	writeTestFile(t, root, "wiki/risk_zones.md", indexTestDoc("brain-risk-zones", "Zonas de Riesgo", "risk_zones", "black-factory", "active", []string{"security"}, nil, "Compuertas HITL."))
	writeTestFile(t, root, "wiki/auth.md", indexTestDoc("wiki-auth", "Autenticacion", "wiki", "black-factory", "active", []string{"auth", "security"}, []string{`0_Brain\engine\`, `Docs/BLACK_FACTORY_BRAIN.md`, `0_Brain/engine`}, "Flujo de autenticacion."))
	writeTestFile(t, root, "adr/ADR-0001.md", indexTestDoc("adr-0001", "ADR Motor", "adr", "black-factory", "accepted", []string{"adr"}, []string{`0_Brain/engine/validator.go`}, "Uso de pure Go."))
	writeTestFile(t, root, "log.md", "# Bitacora\n\n- [2026-11-19] Inicio.\n")
	writeTestFile(t, root, "_proposals/patch-001.md", indexTestDoc("patch-001", "Propuesta pendiente", "brain_patch", "black-factory", "proposed", []string{"proposal"}, nil, "No debe indexarse en docs activos."))

	stats1, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex #1 error: %v", err)
	}
	if stats1.DBPath != DefaultIndexPath(root) {
		t.Fatalf("DBPath = %q, esperaba %q", stats1.DBPath, DefaultIndexPath(root))
	}
	if stats1.DocsIndexed != 6 {
		t.Fatalf("DocsIndexed #1 = %d, esperaba 6", stats1.DocsIndexed)
	}

	stats2, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex #2 error: %v", err)
	}
	if stats2.DocsIndexed != 6 {
		t.Fatalf("DocsIndexed #2 = %d, esperaba 6", stats2.DocsIndexed)
	}

	db, err := sql.Open("sqlite", stats2.DBPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	for _, tbl := range []string{"docs", "doc_paths", "docs_fts"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name = ?`, tbl).Scan(&name)
		if err != nil || name != tbl {
			t.Fatalf("tabla %s ausente: err=%v", tbl, err)
		}
	}

	var docCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM docs`).Scan(&docCount); err != nil {
		t.Fatalf("count docs: %v", err)
	}
	if docCount != 6 {
		t.Fatalf("SELECT COUNT(*) FROM docs = %d, esperaba 6", docCount)
	}

	var ftsCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM docs_fts`).Scan(&ftsCount); err != nil {
		t.Fatalf("count docs_fts: %v", err)
	}
	if ftsCount != 6 {
		t.Fatalf("SELECT COUNT(*) FROM docs_fts = %d, esperaba 6", ftsCount)
	}

	var propCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM docs WHERE rel_path LIKE '_proposals/%' OR rel_path = 'log.md'`).Scan(&propCount); err != nil {
		t.Fatalf("count excluidos: %v", err)
	}
	if propCount != 0 {
		t.Fatalf("se indexaron %d documentos que debian excluirse", propCount)
	}

	rows, err := db.Query(`SELECT cover_path FROM doc_paths`)
	if err != nil {
		t.Fatalf("query doc_paths: %v", err)
	}
	defer rows.Close()
	pathCount := 0
	for rows.Next() {
		var cp string
		if err := rows.Scan(&cp); err != nil {
			t.Fatalf("scan cover_path: %v", err)
		}
		pathCount++
		if strings.Contains(cp, `\`) {
			t.Fatalf("cover_path sin normalizar con backslash: %q", cp)
		}
	}
	if pathCount != 3 {
		t.Fatalf("doc_paths count = %d, esperaba 3 rutas unicas normalizadas", pathCount)
	}

	entries, err := os.ReadDir(filepath.Dir(stats2.DBPath))
	if err != nil {
		t.Fatalf("ReadDir .index: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "brain.db" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("archivos residuales en .index/: %v (esperaba solo brain.db)", names)
	}
}

func TestSearch_FTS5RelevanceAndFilters(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "constitution.md", indexTestDoc(
		"brain-constitution",
		"Constitución de Ingeniería",
		"constitution",
		"black-factory",
		"active",
		[]string{"governance", "go"},
		nil,
		"Invariantes de seguridad, evidence G1 y reglas obligatorias.",
	))
	writeTestFile(t, root, "wiki/sqlite_fts5.md", indexTestDoc(
		"wiki-sqlite-fts5",
		"Motor SQLite FTS5 Pure Go",
		"wiki",
		"black-factory",
		"active",
		[]string{"sqlite", "go"},
		[]string{"0_Brain/engine/index.go"},
		"El indexador SQLite FTS5 construye brain.db de forma atomica con sqlite fts5 y bm25.",
	))
	writeTestFile(t, root, "adr/ADR-0002.md", indexTestDoc(
		"adr-0002-storage",
		"Almacenamiento Local Derivado",
		"adr",
		"black-factory",
		"accepted",
		[]string{"golang", "storage"},
		[]string{"0_Brain/engine/"},
		"Se menciona sqlite de paso como almacenamiento local derivado.",
	))
	writeTestFile(t, root, "wiki/beta_risk_zones.md", indexTestDoc(
		"wiki-beta-risk",
		"Zonas de Riesgo en Beta",
		"wiki",
		"proyecto-beta",
		"active",
		[]string{"security", "risk-zones"},
		[]string{"beta/auth/"},
		"Revision de risk-zones y sqlite en proyecto beta.",
	))

	stats, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	res, err := Search(stats.DBPath, QueryOptions{Query: "sqlite fts5", Project: "black-factory"})
	if err != nil {
		t.Fatalf("Search sqlite fts5 error: %v", err)
	}
	if len(res) != 1 || res[0].ID != "wiki-sqlite-fts5" {
		t.Fatalf("esperaba wiki-sqlite-fts5 como primer resultado exacto, obtuvo %+v", res)
	}
	if strings.TrimSpace(res[0].Snippet) == "" {
		t.Fatalf("esperaba Snippet no vacio en resultado FTS5")
	}

	relRes, err := Search(stats.DBPath, QueryOptions{Query: "sqlite"})
	if err != nil {
		t.Fatalf("Search sqlite error: %v", err)
	}
	if len(relRes) < 2 || relRes[0].ID != "wiki-sqlite-fts5" {
		t.Fatalf("esperaba wiki-sqlite-fts5 primero por BM25, obtuvo %+v", relRes)
	}

	diacRes, err := Search(stats.DBPath, QueryOptions{Query: "constitucion"})
	if err != nil {
		t.Fatalf("Search diacriticos error: %v", err)
	}
	if len(diacRes) != 1 || diacRes[0].ID != "brain-constitution" {
		t.Fatalf("busqueda insensible a tildes fallo: %+v", diacRes)
	}

	specialRes, err := Search(stats.DBPath, QueryOptions{Query: "risk-zones"})
	if err != nil {
		t.Fatalf("Search con guion fallo: %v", err)
	}
	if len(specialRes) != 1 || specialRes[0].ID != "wiki-beta-risk" {
		t.Fatalf("busqueda 'risk-zones' inesperada: %+v", specialRes)
	}

	tagGo, err := Search(stats.DBPath, QueryOptions{Tag: "go"})
	if err != nil {
		t.Fatalf("Search filtro tag=go error: %v", err)
	}
	if len(tagGo) != 2 {
		t.Fatalf("esperaba 2 docs con tag exacto 'go' (sin 'golang'), obtuvo %d: %+v", len(tagGo), tagGo)
	}
	for _, r := range tagGo {
		if r.ID == "adr-0002-storage" {
			t.Fatalf("filtro tag='go' incluyo erradamente doc con tag 'golang'")
		}
	}

	filtered, err := Search(stats.DBPath, QueryOptions{
		Query:   "sqlite",
		Project: "black-factory",
		Type:    "wiki",
		Tag:     "go",
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("Search filtros combinados error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "wiki-sqlite-fts5" {
		t.Fatalf("filtros combinados inesperados: %+v", filtered)
	}
}

func TestContextForFiles_WindowsPOSIXGlobsAndTransversal(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "constitution.md", indexTestDoc("brain-constitution", "Constitucion", "constitution", "black-factory", "active", nil, nil, "Constitucion transversal."))
	writeTestFile(t, root, "wiki/risk_zones.md", indexTestDoc("brain-risk-zones", "Zonas de Riesgo", "risk_zones", "black-factory", "active", nil, nil, "Mapa transversal de riesgo."))
	writeTestFile(t, root, "wiki/engine.md", indexTestDoc("wiki-engine", "Motor Brain", "wiki", "black-factory", "active", nil, []string{`0_Brain\engine\`}, "Cubre directorio del motor."))
	writeTestFile(t, root, "wiki/auth.md", indexTestDoc("wiki-auth", "Auth", "wiki", "black-factory", "active", nil, []string{"app/auth/"}, "Cubre app/auth/."))
	writeTestFile(t, root, "wiki/docs_brain.md", indexTestDoc("wiki-docs-brain", "Doc Brain", "wiki", "black-factory", "active", nil, []string{"Docs/BLACK_FACTORY_BRAIN.md"}, "Cubre doc exacto."))
	writeTestFile(t, root, "wiki/services.md", indexTestDoc("wiki-services", "Servicios Go", "wiki", "black-factory", "active", nil, []string{"services/**/*.go"}, "Cubre glob recursivo."))
	writeTestFile(t, root, "wiki/scripts.md", indexTestDoc("wiki-scripts", "Scripts Python", "wiki", "black-factory", "active", nil, []string{"scripts/*.py"}, "Cubre glob simple."))
	writeTestFile(t, root, "wiki/otro_proyecto.md", indexTestDoc("wiki-otro", "Otro Proyecto", "wiki", "proyecto-beta", "active", nil, []string{"0_Brain/engine/"}, "No debe salir en black-factory."))

	stats, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	ctxRes, err := ContextForFiles(stats.DBPath, "black-factory", []string{
		`0_Brain\engine\query.go`,
		`app/auth/jwt.go`,
		`services/billing/v2/handler.go`,
		`scripts\check.py`,
	})
	if err != nil {
		t.Fatalf("ContextForFiles error: %v", err)
	}

	gotIDs := map[string]SearchResult{}
	for _, r := range ctxRes {
		gotIDs[r.ID] = r
	}
	for _, want := range []string{"brain-constitution", "brain-risk-zones", "wiki-engine", "wiki-auth", "wiki-services", "wiki-scripts"} {
		if _, ok := gotIDs[want]; !ok {
			t.Fatalf("faltaba %s en ContextForFiles: %+v", want, ctxRes)
		}
	}
	if _, bad := gotIDs["wiki-docs-brain"]; bad {
		t.Fatalf("no debia incluir wiki-docs-brain")
	}
	if _, bad := gotIDs["wiki-otro"]; bad {
		t.Fatalf("no debia incluir wiki-otro de proyecto-beta")
	}
	if !gotIDs["brain-constitution"].Transversal || !gotIDs["brain-risk-zones"].Transversal {
		t.Fatalf("se esperaba Transversal=true en constitution y risk_zones")
	}
	if len(ctxRes) < 2 || !ctxRes[0].Transversal || !ctxRes[1].Transversal {
		t.Fatalf("se esperaban documentos transversales primero en el orden: %+v", ctxRes)
	}

	falseRes, err := ContextForFiles(stats.DBPath, "black-factory", []string{
		"app/authentication/x.go",
		"scripts/run.sh",
		"scripts/sub/nested.py",
	})
	if err != nil {
		t.Fatalf("ContextForFiles no-match error: %v", err)
	}
	for _, r := range falseRes {
		if r.ID == "wiki-auth" || r.ID == "wiki-scripts" {
			t.Fatalf("coincidencia falsa en ContextForFiles: %+v", r)
		}
	}

	betaRes, err := ContextForFiles(stats.DBPath, "proyecto-beta", []string{"sin/coincidencia.txt"})
	if err != nil {
		t.Fatalf("ContextForFiles proyecto-beta error: %v", err)
	}
	if len(betaRes) != 2 || !betaRes[0].Transversal || !betaRes[1].Transversal {
		t.Fatalf("esperaba 2 transversales en proyecto-beta sin matches directos, obtuvo %+v", betaRes)
	}
}

func TestIndexAndSearch_Scale50DocsPerformance(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "constitution.md", indexTestDoc("brain-constitution", "Constitucion", "constitution", "black-factory", "active", nil, nil, "Constitucion."))
	writeTestFile(t, root, "wiki/risk_zones.md", indexTestDoc("brain-risk-zones", "Zonas de Riesgo", "risk_zones", "black-factory", "active", nil, nil, "Zonas."))

	for i := 1; i <= 60; i++ {
		rel := fmt.Sprintf("wiki/modulo_%02d.md", i)
		id := fmt.Sprintf("wiki-mod-%02d", i)
		title := fmt.Sprintf("Modulo %02d de Procesamiento", i)
		cover := fmt.Sprintf("modules/mod_%02d/", i)
		body := fmt.Sprintf("Documentacion tecnica del modulo %02d con indice determinista y telemetria.", i)
		if i == 42 {
			body += " Incluye palabra clave hiperescala para consulta."
		}
		writeTestFile(t, root, rel, indexTestDoc(id, title, "wiki", "black-factory", "active", []string{"scale"}, []string{cover}, body))
	}

	stats, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex 62 docs error: %v", err)
	}
	if stats.DocsIndexed != 62 {
		t.Fatalf("DocsIndexed = %d, esperaba 62", stats.DocsIndexed)
	}

	sRes, err := Search(stats.DBPath, QueryOptions{Query: "hiperescala", Limit: 10})
	if err != nil {
		t.Fatalf("Search escala error: %v", err)
	}
	if len(sRes) != 1 || sRes[0].ID != "wiki-mod-42" {
		t.Fatalf("esperaba wiki-mod-42 en busqueda sobre 62 docs, obtuvo %+v", sRes)
	}

	cRes, err := ContextForFiles(stats.DBPath, "black-factory", []string{`modules\mod_42\service.go`})
	if err != nil {
		t.Fatalf("ContextForFiles escala error: %v", err)
	}
	if len(cRes) != 3 {
		t.Fatalf("esperaba 3 resultados (2 transversales + mod 42), obtuvo %d: %+v", len(cRes), cRes)
	}
}

func TestBuildIndex_MissingVaultAndMissingDBErrors(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "no_existe")
	if _, err := BuildIndex(missingDir); err == nil {
		t.Fatalf("esperaba error en BuildIndex para directorio inexistente")
	}

	missingDB := filepath.Join(t.TempDir(), "inexistente.db")
	if _, err := Search(missingDB, QueryOptions{Query: "hola"}); err == nil {
		t.Fatalf("esperaba error en Search para db inexistente")
	}
	if _, err := ContextForFiles(missingDB, "black-factory", []string{"a.go"}); err == nil {
		t.Fatalf("esperaba error en ContextForFiles para db inexistente")
	}
	if _, err := os.Stat(missingDB); !os.IsNotExist(err) {
		t.Fatalf("no debia crearse %s al fallar consulta sobre db inexistente", missingDB)
	}
}
