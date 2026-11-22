package brain

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRiskZonesCatalog_TableAndFailSafe(t *testing.T) {
	rawVaultRiskZones, err := os.ReadFile(filepath.Join("..", "schemas", "risk_zones.template.md"))
	if err != nil {
		t.Fatalf("leyendo risk_zones.template.md: %v", err)
	}

	entries, err := ParseRiskZonesCatalog(string(rawVaultRiskZones))
	if err != nil {
		t.Fatalf("ParseRiskZonesCatalog fallo sobre plantilla oficial: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperaba 2 entradas (RZ-001, RZ-002), obtuvo %d: %+v", len(entries), entries)
	}
	if entries[0].ID != "RZ-001" || entries[0].Level != "CRITICO" || len(entries[0].CoversPaths) != 1 || entries[0].CoversPaths[0] != "GengiSWAP/engine/" {
		t.Fatalf("RZ-001 inesperado: %+v", entries[0])
	}
	if entries[1].ID != "RZ-002" || entries[1].Level != "ALTO" || len(entries[1].CoversPaths) != 1 || entries[1].CoversPaths[0] != "0_Brain/engine/" {
		t.Fatalf("RZ-002 inesperado: %+v", entries[1])
	}

	// Variaciones: CRÍTICO con tilde, multiples patrones separados por , y ;, MEDIO y BAJO
	customMD := `# Mapa
## Catálogo de Zonas de Riesgo

| ID | Proyecto | Rutas cubiertas (` + "`covers_paths`" + `) | Nivel | Compuerta obligatoria | Evidencia ` + "`G5`" + ` |
| :--- | :--- | :--- | :--- | :--- | :--- |
| ` + "`RZ-010`" + ` | ` + "`black-factory`" + ` | ` + "`GengiSWAP/engine/`" + `, ` + "`1_Intake/engine/store.go`" + `; ` + "`pkg/bfcore/**`" + ` | ` + "`CRÍTICO`" + ` | PR + AUTOR | ` + "`GengiSWAP/engine/ring.go:1`" + ` |
| ` + "`RZ-011`" + ` | ` + "`*`" + ` | ` + "`Docs/designs/*.md`" + ` | ` + "`MEDIO`" + ` | Review | ` + "`Docs/README.md:1`" + ` |
| ` + "`RZ-012`" + ` | ` + "`black-factory`" + ` | ` + "`scripts/*.ps1`" + ` | ` + "`BAJO`" + ` | Lint | ` + "`README.md:1`" + ` |
`
	entries2, err := ParseRiskZonesCatalog(customMD)
	if err != nil {
		t.Fatalf("ParseRiskZonesCatalog custom fallo: %v", err)
	}
	if len(entries2) != 3 || entries2[0].Level != "CRITICO" || len(entries2[0].CoversPaths) != 3 {
		t.Fatalf("entries2 inesperado: %+v", entries2)
	}

	// Tabla ausente o nivel invalido debe retornar error para activar fail-safe
	if _, err := ParseRiskZonesCatalog("# Sin tabla de zonas\nTexto plano."); err == nil {
		t.Fatalf("esperaba error cuando falta la tabla de zonas de riesgo")
	}
	badLevelMD := strings.Replace(customMD, "`CRÍTICO`", "`DESCONOCIDO`", 1)
	if _, err := ParseRiskZonesCatalog(badLevelMD); err == nil {
		t.Fatalf("esperaba error ante nivel de riesgo desconocido")
	}

	// Major QA #2a: tabla con fila truncada (< 5 columnas o sin '|' final) dentro del catalogo DEBE retornar error explicito
	truncatedRowMD := customMD + "| `RZ-013` | `black-factory` | `GengiSWAP/engine/` |\n"
	if _, err := ParseRiskZonesCatalog(truncatedRowMD); err == nil {
		t.Fatalf("ParseRiskZonesCatalog debia fallar ante fila truncada con < 5 columnas")
	}
	missingPipeMD := customMD + "| `RZ-014` | `black-factory` | `GengiSWAP/engine/` | `CRITICO` | PR + AUTOR\n"
	if _, err := ParseRiskZonesCatalog(missingPipeMD); err == nil {
		t.Fatalf("ParseRiskZonesCatalog debia fallar ante fila de tabla sin '|' de cierre")
	}

	// Major QA #2b: tabla '| ID | Proyecto |' fuera de la seccion '## Catálogo de Zonas de Riesgo' no debe aceptarse
	outsideSectionMD := "# Otro Documento\n## Otra Seccion\n| ID | Proyecto | Rutas cubiertas | Nivel | Compuerta |\n| :--- | :--- | :--- | :--- | :--- |\n| `RZ-001` | `black-factory` | `GengiSWAP/` | `CRITICO` | PR |\n"
	if _, err := ParseRiskZonesCatalog(outsideSectionMD); err == nil {
		t.Fatalf("ParseRiskZonesCatalog no debia aceptar tabla fuera de la seccion de zonas de riesgo")
	}
}

func TestContextForFiles_StructuredRiskHitsAndCaseInsensitiveWindowsPaths(t *testing.T) {
	root := t.TempDir()
	rawRiskZones, err := os.ReadFile(filepath.Join("..", "schemas", "risk_zones.template.md"))
	if err != nil {
		t.Fatalf("leyendo risk_zones.template.md: %v", err)
	}

	writeTestFile(t, root, "constitution.md", indexTestDoc("brain-constitution", "Constitucion", "constitution", "black-factory", "active", nil, nil, "Constitucion."))
	writeTestFile(t, root, "wiki/risk_zones.md", string(rawRiskZones))
	writeTestFile(t, root, "wiki/brain_engine_g4.md", indexTestDoc("brain-engine-g4", "Motor Brain G4", "wiki", "black-factory", "active", []string{"g4"}, []string{"0_Brain/engine/"}, "Documentacion G4."))

	stats, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	// Caso 1: ruta Windows con diferente capitalizacion hacia GengiSWAP/engine/ -> debe detectar RZ-001 (CRITICO)
	ctxRes, err := ContextForFiles(stats.DBPath, "black-factory", []string{`gengiswap\engine\ring.go`})
	if err != nil {
		t.Fatalf("ContextForFiles error: %v", err)
	}

	var rzDoc *SearchResult
	for i := range ctxRes {
		if ctxRes[i].Type == "risk_zones" {
			rzDoc = &ctxRes[i]
			break
		}
	}
	if rzDoc == nil {
		t.Fatalf("no se encontro documento risk_zones en ContextForFiles: %+v", ctxRes)
	}
	if rzDoc.RiskZonesParseError != "" {
		t.Fatalf("RiskZonesParseError inesperado: %s", rzDoc.RiskZonesParseError)
	}
	if len(rzDoc.RiskZonesSHA256) != 64 {
		t.Fatalf("RiskZonesSHA256 invalido: %q", rzDoc.RiskZonesSHA256)
	}
	if len(rzDoc.RiskHits) != 1 || rzDoc.RiskHits[0].ID != "RZ-001" || rzDoc.RiskHits[0].Level != "CRITICO" {
		t.Fatalf("esperaba RiskHit RZ-001 CRITICO para gengiswap/engine/ring.go, obtuvo: %+v", rzDoc.RiskHits)
	}
	if len(rzDoc.MatchedPaths) != 1 || rzDoc.MatchedPaths[0] != "gengiswap/engine/ring.go" {
		t.Fatalf("MatchedPaths en risk_zones debia incluir gengiswap/engine/ring.go, obtuvo: %+v", rzDoc.MatchedPaths)
	}

	// Caso 2: ruta con diferente capitalizacion hacia 0_Brain/engine/validator.go -> debe coincidir con wiki/brain_engine_g4.md y RZ-002 (ALTO)
	ctxRes2, err := ContextForFiles(stats.DBPath, "black-factory", []string{`0_brain/ENGINE/validator.go`})
	if err != nil {
		t.Fatalf("ContextForFiles #2 error: %v", err)
	}
	foundWiki := false
	foundRZ2 := false
	for _, r := range ctxRes2 {
		if r.ID == "brain-engine-g4" && len(r.MatchedPaths) == 1 {
			foundWiki = true
		}
		if r.Type == "risk_zones" && len(r.RiskHits) == 1 && r.RiskHits[0].ID == "RZ-002" && r.RiskHits[0].Level == "ALTO" {
			foundRZ2 = true
		}
	}
	if !foundWiki || !foundRZ2 {
		t.Fatalf("esperaba match case-insensitive en brain-engine-g4 (%v) y RZ-002 (%v): %+v", foundWiki, foundRZ2, ctxRes2)
	}

	// Major QA #4: ContextForFiles con --project distinto (ej. "gengiswap") debe incluir risk_hits transversales de "black-factory"
	ctxSubProj, err := ContextForFiles(stats.DBPath, "gengiswap", []string{`GengiSWAP/engine/ring.go`})
	if err != nil {
		t.Fatalf("ContextForFiles gengiswap error: %v", err)
	}
	foundTransversalRZ := false
	for _, r := range ctxSubProj {
		if r.Type == "risk_zones" && len(r.RiskHits) == 1 && r.RiskHits[0].ID == "RZ-001" {
			foundTransversalRZ = true
		}
	}
	if !foundTransversalRZ {
		t.Fatalf("ContextForFiles(--project gengiswap) debia reportar RZ-001 transversal de black-factory: %+v", ctxSubProj)
	}

	// Caso 3: risk_zones.md con tabla malformada debe poblar RiskZonesParseError sin abortar la consulta
	writeTestFile(t, root, "wiki/risk_zones.md", indexTestDoc("brain-risk-zones", "Zonas Corruptas", "risk_zones", "black-factory", "active", nil, nil, "Tabla rota sin columnas."))
	if _, err := BuildIndex(root); err != nil {
		t.Fatalf("BuildIndex #2: %v", err)
	}
	ctxCorrupt, err := ContextForFiles(stats.DBPath, "black-factory", []string{"0_Brain/engine/validator.go"})
	if err != nil {
		t.Fatalf("ContextForFiles corrupt error: %v", err)
	}
	hasParseErr := false
	for _, r := range ctxCorrupt {
		if r.Type == "risk_zones" && r.RiskZonesParseError != "" {
			hasParseErr = true
		}
	}
	if !hasParseErr {
		t.Fatalf("esperaba RiskZonesParseError no vacio cuando risk_zones.md carece de tabla valida: %+v", ctxCorrupt)
	}
}

func TestSearchAndContext_NonNullJSONSlices_FTSBooleanKeywords_AndUTF8Snippet(t *testing.T) {
	root := t.TempDir()
	// Body con caracteres multibyte UTF-8 alrededor del byte 157 para verificar que cleanSnippet corta por []rune
	longUTF8Body := strings.Repeat("áéíóúñ", 40)
	writeTestFile(t, root, "constitution.md", indexTestDoc("brain-constitution", "Constitucion", "constitution", "black-factory", "active", nil, nil, longUTF8Body))

	stats, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	// 1. Consulta sin resultados debe serializarse como "[]" y nunca como "null"
	emptySearch, err := Search(stats.DBPath, QueryOptions{Query: "termino_inexistente_xyz_987"})
	if err != nil {
		t.Fatalf("Search vacio error: %v", err)
	}
	if emptySearch == nil {
		t.Fatalf("Search sin resultados devolvio slice nil (serializa a null en JSON)")
	}
	rawJSON, _ := json.Marshal(emptySearch)
	if string(rawJSON) != "[]" {
		t.Fatalf("Search vacio serializo a %s, esperaba []", string(rawJSON))
	}

	emptyMeta, err := Search(stats.DBPath, QueryOptions{Project: "proyecto-inexistente"})
	if err != nil || emptyMeta == nil {
		t.Fatalf("Search por metadata sin resultados devolvio nil o error: %v", err)
	}

	emptyCtx, err := ContextForFiles(stats.DBPath, "proyecto-otro", []string{"no/existe.go"})
	if err != nil {
		t.Fatalf("ContextForFiles error: %v", err)
	}
	if emptyCtx == nil {
		t.Fatalf("ContextForFiles sin resultados devolvio slice nil")
	}

	// 2. Consulta con operadores booleanos FTS5 aislados ("NOT", "AND", "OR", "NEAR") no debe fallar con error de sintaxis FTS5
	for _, kw := range []string{"NOT", "AND", "OR", "NEAR", "NOT AND OR NEAR"} {
		res, kwErr := Search(stats.DBPath, QueryOptions{Query: kw})
		if kwErr != nil {
			t.Fatalf("Search(%q) fallo con error FTS5: %v", kw, kwErr)
		}
		if res == nil {
			t.Fatalf("Search(%q) devolvio slice nil", kw)
		}
	}
	if q := buildFTS5Query("NEAR", false); q != `"NEAR"` {
		t.Fatalf("buildFTS5Query(\"NEAR\", false) = %q, esperaba \"\\\"NEAR\\\"\"", q)
	}

	// 3. Major QA #3: AcquireVaultLock sobre directorio inexistente NO debe envolver el error en ErrVaultLocked
	missingVaultDir := filepath.Join(t.TempDir(), "boveda_inexistente")
	if _, lockErr := AcquireVaultLock(missingVaultDir); lockErr == nil {
		t.Fatalf("AcquireVaultLock sobre directorio inexistente debia fallar")
	} else if strings.Contains(lockErr.Error(), ErrVaultLocked.Error()) {
		t.Fatalf("AcquireVaultLock sobre directorio inexistente no debe envolver en ErrVaultLocked: %v", lockErr)
	}

	// 3. Verificacion de cleanSnippet con runas UTF-8 validas
	allDocs, err := Search(stats.DBPath, QueryOptions{Project: "black-factory"})
	if err != nil || len(allDocs) != 1 {
		t.Fatalf("Search metadata error: %v (%+v)", err, allDocs)
	}
	if !bytes.Equal([]byte(allDocs[0].Snippet), bytes.ToValidUTF8([]byte(allDocs[0].Snippet), nil)) {
		t.Fatalf("cleanSnippet corrompio secuencia UTF-8 multibyte: %q", allDocs[0].Snippet)
	}
}
