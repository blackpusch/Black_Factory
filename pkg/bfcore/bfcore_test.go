package bfcore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestG4AllRulesAndTier1Classification(t *testing.T) {
	if got := G4RulesCount(); got != 11 {
		t.Fatalf("G4RulesCount() = %d, esperaba 11", got)
	}

	pemLine := "-----BEGIN RSA " + "PRIVATE KEY-----"
	findings := ScanG4All(pemLine)
	if len(findings) != 1 || findings[0].Rule != "pem_private_key" {
		t.Fatalf("pem_private_key debia suprimir literal_private_key en la misma linea, obtuvo: %+v", findings)
	}

	tier1Vectors := []struct {
		rule string
		text string
	}{
		{"pem_private_key", "-----BEGIN OPENSSH " + "PRIVATE KEY-----"},
		{"literal_private_key", "guardar PRIVATE " + "KEY secreta"},
		{"bearer_token", "Authorization: Bearer " + "eyJhbGciOiJIUzI1NiJ9_secret"},
		{"known_token", "token=" + "ghp_" + "1234567890abcdefghij123456"},
		{"jwt_token", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"conn_uri_credentials", "postgres://admin:" + "s3cr3tPass" + "@db.internal:5432/prod"},
		{"conn_password_param", "Server=db;Database=app;Password=" + "topsecret123"},
		{"secret_assignment", "api_key = " + "\"super_secret_value_12345\""},
	}

	for _, tc := range tier1Vectors {
		t.Run("tier1_"+tc.rule, func(t *testing.T) {
			hits := ScanTier1("raw_input", tc.text, false)
			if len(hits) == 0 {
				t.Fatalf("ScanTier1 no detecto %s", tc.rule)
			}
			found := false
			for _, h := range hits {
				if h.Rule == tc.rule && h.Tier == Tier1HardSecret {
					found = true
				}
				if strings.Contains(h.Message, "s3cr3tPass") || strings.Contains(h.Message, "1234567890abcdefghij123456") {
					t.Fatalf("mensaje G4 transcribio el secreto: %q", h.Message)
				}
			}
			if !found {
				t.Fatalf("esperaba regla %s en %+v", tc.rule, hits)
			}
		})
	}

	// Tier 2 vectors should NOT trigger ScanTier1 when strictG4 == false, but MUST trigger when strictG4 == true
	tier2Line := `En C:\` + `Users\jdoe\app.go o /home/` + `alice/main.go por jdoe` + `@example.com`
	if hits := ScanTier1("description", tier2Line, false); len(hits) != 0 {
		t.Fatalf("ScanTier1(strictG4=false) no debia disparar sobre reglas Tier 2: %+v", hits)
	}
	if hits := ScanTier1("description", tier2Line, true); len(hits) < 3 {
		t.Fatalf("ScanTier1(strictG4=true) debia detectar las 3 reglas Tier 2, obtuvo %d: %+v", len(hits), hits)
	}
}

func TestRedactTier2Text_FixedPointAndDelimiters(t *testing.T) {
	input := `Rutas adyacentes: /home/` + `alice /home/` + `bob/dir y "/Users/` + `charlie/code.go" ` +
		`env=/home/` + `dave:path C:\` + `Users\eve\go\src\main.go reportado por eve` + `@example.com y bob` + `@corp.org`

	redacted, counts := RedactTier2Text("description", input)
	if findings := ScanG4All(redacted); len(findings) != 0 {
		t.Fatalf("texto redactado aun presenta hallazgos G4: %+v\ntexto: %s", findings, redacted)
	}
	for _, leaked := range []string{"alice", "bob", "charlie", "dave", "eve"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("texto redactado conserva PII %q: %s", leaked, redacted)
		}
	}
	if !strings.Contains(redacted, `[REDACTED:G4:unix_user_path] [REDACTED:G4:unix_user_path]/dir`) {
		t.Fatalf("fallo redaccion en punto fijo de rutas unix adyacentes: %s", redacted)
	}
	if !strings.Contains(redacted, `"[REDACTED:G4:unix_user_path]/code.go"`) {
		t.Fatalf("delimitadores de comillas o subruta corrompidos: %s", redacted)
	}
	if !strings.Contains(redacted, `env=[REDACTED:G4:unix_user_path]:path`) {
		t.Fatalf("delimitadores '=' o ':' corrompidos: %s", redacted)
	}
	if !strings.Contains(redacted, `[REDACTED:G4:win_user_path]\go\src\main.go`) {
		t.Fatalf("subruta posterior a win_user_path no preservada: %s", redacted)
	}

	ruleTotals := map[string]int{}
	for _, c := range counts {
		ruleTotals[c.Rule] = c.Count
	}
	if ruleTotals["unix_user_path"] != 4 || ruleTotals["win_user_path"] != 1 || ruleTotals["email_address"] != 2 {
		t.Fatalf("conteos de redaccion inesperados: %+v", counts)
	}

	// RedactAllG4 sobre metadatos con secretos Tier 1 y Tier 2
	dirtyRef := "ghp_" + "1234567890abcdefghij123456-" + "admin" + "@example.com"
	cleanRef := RedactAllG4(dirtyRef)
	if findings := ScanG4All(cleanRef); len(findings) != 0 {
		t.Fatalf("RedactAllG4 dejo hallazgos G4: %+v en %q", findings, cleanRef)
	}

	if !MatchesPersonalPathG4(`C:\` + `Users\admin\file.go`) {
		t.Fatalf("MatchesPersonalPathG4 debia detectar win_user_path")
	}
	if !MatchesPersonalPathG4(`/home/` + `admin/file.go`) {
		t.Fatalf("MatchesPersonalPathG4 debia detectar unix_user_path")
	}
	if MatchesPersonalPathG4(`0_Brain/engine/validator.go`) {
		t.Fatalf("MatchesPersonalPathG4 dio falso positivo en ruta relativa del repo")
	}

	// 1. Blocker QA: 25 ocurrencias de unix_user_path (> 16 pasadas) en un mismo texto deben quedar 100% redactadas
	var manyUnix strings.Builder
	for i := 1; i <= 25; i++ {
		manyUnix.WriteString("/home/" + "dev/file.go ")
	}
	redMany, countsMany := RedactTier2Text("description", manyUnix.String())
	if findings := ScanG4All(redMany); len(findings) != 0 {
		t.Fatalf("RedactTier2Text dejo hallazgos G4 con 25 rutas unix_user_path: %+v\ntexto: %s", findings, redMany)
	}
	if strings.Contains(redMany, "/home/dev") {
		t.Fatalf("RedactTier2Text filtro rutas unix_user_path mas alla de la 16: %s", redMany)
	}
	found25 := false
	for _, c := range countsMany {
		if c.Rule == "unix_user_path" && c.Count == 25 {
			found25 = true
		}
	}
	if !found25 {
		t.Fatalf("esperaba Count=25 para unix_user_path, obtuvo: %+v", countsMany)
	}

	// 2. Blocker QA: RedactAllG4 debe aplicar reglas Tier 1 antes que Tier 2 para que "Bearer C:\Users\alice\secret_tok"
	// o "Bearer secret_tok_12345" con una ruta win_user_path previa redacten ambos sin romper ni filtrar nada.
	bearerWithWinPath := "Authorization: Bearer " + `C:\` + `Users\alice\secret_tok_12345 y Bearer secret_tok_987654321`
	cleanBearer := RedactAllG4(bearerWithWinPath)
	if findings := ScanG4All(cleanBearer); len(findings) != 0 {
		t.Fatalf("RedactAllG4 sobre Bearer con win_user_path dejo hallazgos G4: %+v en %q", findings, cleanBearer)
	}
	if strings.Contains(cleanBearer, "alice") || strings.Contains(cleanBearer, "secret_tok_987654321") {
		t.Fatalf("RedactAllG4 filtro datos en Bearer+win_user_path: %q", cleanBearer)
	}
	if !strings.Contains(cleanBearer, "[REDACTED:G4:bearer_token]") || !strings.Contains(cleanBearer, "[REDACTED:G4:win_user_path]") {
		t.Fatalf("RedactAllG4 debia aplicar bearer_token (Tier 1) y win_user_path (Tier 2), obtuvo: %q", cleanBearer)
	}
}

func TestNormalizeAndMatchCoverPaths_CaseAndFold(t *testing.T) {
	if got := NormalizeCoverPath(`.\0_Brain\\engine\validator.go/`); got != "0_Brain/engine/validator.go" {
		t.Fatalf("NormalizeCoverPath = %q", got)
	}
	if got := NormalizeCoverPath(" . "); got != "" {
		t.Fatalf("NormalizeCoverPath('.') = %q, esperaba vacio", got)
	}

	if !MatchCoverPath("GengiSWAP/engine/", "GengiSWAP/engine/ring.go") {
		t.Fatalf("MatchCoverPath debia coincidir con prefijo exacto")
	}
	if MatchCoverPath("GengiSWAP/engine/", "gengiswap/engine/ring.go") {
		t.Fatalf("MatchCoverPath (case-sensitive) no debia coincidir con diferente capitalizacion")
	}
	if !MatchCoverPathFold("GengiSWAP/engine/", `gengiswap\engine\ring.go`) {
		t.Fatalf("MatchCoverPathFold debia coincidir de forma insensible a mayusculas en Windows")
	}
	if !MatchCoverPathFold("services/**/*.go", "SERVICES/Billing/V2/Handler.GO") {
		t.Fatalf("MatchCoverPathFold debia coincidir con glob ** insensible a mayusculas")
	}
	if !MatchCoverPathFold("*", "README.md") {
		t.Fatalf("MatchCoverPathFold('*') debia coincidir")
	}
	if MatchCoverPathFold("", "README.md") || MatchCoverPathFold("0_Brain/", "") {
		t.Fatalf("MatchCoverPathFold con patron o archivo vacio debe retornar false")
	}
}

func TestAcquireFileLock_ContentionAndRetry(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".test.lock")

	lock1, err := AcquireFileLock(lockPath, 0)
	if err != nil {
		t.Fatalf("AcquireFileLock inicial fallo: %v", err)
	}

	// Intento concurrente con timeout corto debe devolver ErrLockBusy
	start := time.Now()
	lock2, err := AcquireFileLock(lockPath, 60*time.Millisecond)
	if err == nil {
		_ = lock2.Unlock()
		t.Fatalf("se esperaba ErrLockBusy cuando el lock esta retenido")
	}
	if !errors.Is(err, ErrLockBusy) {
		t.Fatalf("esperaba ErrLockBusy, obtuvo %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("AcquireFileLock no respeto el bucle de backoff (elapsed=%v)", elapsed)
	}

	// Liberar lock1 en goroutine para probar que el retry en curso adquiere el lock con exito
	go func() {
		time.Sleep(40 * time.Millisecond)
		_ = lock1.Unlock()
	}()

	lock3, err := AcquireFileLock(lockPath, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("AcquireFileLock tras liberacion concurrente fallo: %v", err)
	}
	if err := lock3.Unlock(); err != nil {
		t.Fatalf("Unlock fallo: %v", err)
	}
	// Segundo Unlock sobre nil/cerrado debe ser no-op seguro
	if err := lock3.Unlock(); err != nil {
		t.Fatalf("segundo Unlock debia ser idempotente: %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("el archivo de lock debe persistir en disco tras Unlock: %v", err)
	}

	// Major QA #3: error del SO al abrir ruta en directorio inexistente NO debe envolverse como ErrLockBusy
	badLockPath := filepath.Join(dir, "no_existe_subdir", ".test.lock")
	if _, err := AcquireFileLock(badLockPath, 100*time.Millisecond); err == nil {
		t.Fatalf("AcquireFileLock sobre directorio inexistente debia fallar")
	} else if errors.Is(err, ErrLockBusy) {
		t.Fatalf("error de directorio inexistente no debe envolverse en ErrLockBusy: %v", err)
	}
}

type fakeFileInfo struct {
	name  string
	mode  os.FileMode
	isDir bool
	sys   any
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Now() }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return f.sys }

func TestReparseAndAncestorChain(t *testing.T) {
	if IsReparseOrSymlink(nil) {
		t.Fatalf("IsReparseOrSymlink(nil) debe ser false")
	}
	if IsReparseOrSymlink(fakeFileInfo{name: "normal.go", mode: 0o644}) {
		t.Fatalf("archivo regular no es reparse point")
	}
	if !IsReparseOrSymlink(fakeFileInfo{name: "sym", mode: os.ModeSymlink}) {
		t.Fatalf("ModeSymlink debe detectarse")
	}
	if !IsReparseOrSymlink(fakeFileInfo{name: "junc", mode: os.ModeDir | os.ModeIrregular, isDir: true}) {
		t.Fatalf("ModeIrregular debe detectarse")
	}
	winAttr := &syscall.Win32FileAttributeData{FileAttributes: 0x00000400}
	if !IsReparseOrSymlink(fakeFileInfo{name: "win_reparse", mode: 0o644, sys: winAttr}) {
		t.Fatalf("FILE_ATTRIBUTE_REPARSE_POINT (0x400) debe detectarse")
	}

	root := t.TempDir()
	subDir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(subDir, "file.json")
	if err := os.WriteFile(targetFile, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CheckNoReparseChain(root, targetFile); err != nil {
		t.Fatalf("CheckNoReparseChain fallo en ruta limpia: %v", err)
	}
	// Ruta cuyo archivo hoja aun no existe pero sus ancestros son limpios debe pasar
	if err := CheckNoReparseChain(root, filepath.Join(subDir, "nonexistent", "new.json")); err != nil {
		t.Fatalf("CheckNoReparseChain fallo en ruta con hoja inexistente: %v", err)
	}
	// rootPath vacio tambien debe recorrer hasta la raiz del volumen sin fallar
	if err := CheckNoReparseChain("", targetFile); err != nil {
		t.Fatalf("CheckNoReparseChain con rootPath vacio fallo: %v", err)
	}
	// targetPath vacio debe retornar error
	if err := CheckNoReparseChain(root, "   "); err == nil {
		t.Fatalf("CheckNoReparseChain con targetPath vacio debia fallar")
	}

	// Casos vacios en RedactTier2Text, RedactAllG4 y MatchesPersonalPathG4
	if r, c := RedactTier2Text("title", ""); r != "" || c != nil {
		t.Fatalf("RedactTier2Text vacio debia retornar vacio")
	}
	if r := RedactAllG4(""); r != "" {
		t.Fatalf("RedactAllG4 vacio debia retornar vacio")
	}
	if MatchesPersonalPathG4("") {
		t.Fatalf("MatchesPersonalPathG4 vacio debia retornar false")
	}
	// MatchCoverPath con globs *, ** y ?
	if !MatchCoverPath("pkg/**/*.go", "pkg/bfcore/g4.go") {
		t.Fatalf("MatchCoverPath con ** debia coincidir")
	}
	if !MatchCoverPath("pkg/bfcore/g?.go", "pkg/bfcore/g4.go") {
		t.Fatalf("MatchCoverPath con ? debia coincidir")
	}
	if MatchCoverPath("pkg/bfcore/*.md", "pkg/bfcore/g4.go") {
		t.Fatalf("MatchCoverPath no debia coincidir con extension distinta")
	}
}

func TestEvaluateFTDBounds_Option5B(t *testing.T) {
	if DefaultMaxFTDFiles != 5 || MaxFTDExemptGoTests != 5 || MaxFTDPhysicalFiles != 10 || MaxFTDRootModules != 1 {
		t.Fatalf("constantes FTD inesperadas: max=%d exempt=%d phys=%d mods=%d",
			DefaultMaxFTDFiles, MaxFTDExemptGoTests, MaxFTDPhysicalFiles, MaxFTDRootModules)
	}

	if mods := CountRootModules(nil); mods != nil {
		t.Fatalf("CountRootModules(nil) = %v, esperaba nil", mods)
	}
	if mods := CountRootModules([]string{"README.md", "go.work"}); len(mods) != 1 || mods[0] != "." {
		t.Fatalf("CountRootModules raiz = %v, esperaba [\".\"]", mods)
	}
	if mods := CountRootModules([]string{"0_Brain/a.go", "0_brain/b.go"}); len(mods) != 1 || mods[0] != "0_Brain" {
		t.Fatalf("CountRootModules case-insensitive = %v, esperaba [\"0_Brain\"]", mods)
	}

	// Lista vacia -> WithinBounds: false
	emptyEval := EvaluateFTDBounds(nil, 0)
	if emptyEval.WithinBounds || emptyEval.PhysicalCount != 0 || emptyEval.EffectiveCount != 0 {
		t.Fatalf("EvaluateFTDBounds(nil) debia ser WithinBounds=false, obtuvo %+v", emptyEval)
	}

	// 5 productivos + 5 *_test.go hermanos en mismo directorio = 10 fisicos, 5 efectivos, 5 exentos -> PASS (maxEffective <= 0 usa default 5)
	pass10 := EvaluateFTDBounds([]string{
		"0_Brain/engine/a.go",
		"0_Brain/engine/b.go",
		"0_Brain/engine/c.go",
		"0_Brain/engine/d.go",
		"0_Brain/engine/e.go",
		"0_Brain/engine/a_test.go",
		"0_Brain/engine/b_test.go",
		"0_Brain/engine/c_test.go",
		"0_Brain/engine/d_test.go",
		"0_Brain/engine/E_TEST.GO",
		"0_brain/engine/a.go", // duplicado case-insensitive ignorado
	}, 0)
	if !pass10.WithinBounds || pass10.PhysicalCount != 10 || pass10.EffectiveCount != 5 || pass10.ExemptTestCount != 5 || len(pass10.RootModules) != 1 {
		t.Fatalf("5 productivos + 5 *_test.go hermanos debia pasar FTD: %+v", pass10)
	}

	// 5 productivos + 6 *_test.go hermanos = 11 fisicos, 6 efectivos, 5 exentos -> FAIL
	fail11 := EvaluateFTDBounds([]string{
		"0_Brain/engine/a.go",
		"0_Brain/engine/b.go",
		"0_Brain/engine/c.go",
		"0_Brain/engine/d.go",
		"0_Brain/engine/e.go",
		"0_Brain/engine/a_test.go",
		"0_Brain/engine/b_test.go",
		"0_Brain/engine/c_test.go",
		"0_Brain/engine/d_test.go",
		"0_Brain/engine/e_test.go",
		"0_Brain/engine/f_test.go",
	}, 5)
	if fail11.WithinBounds || fail11.PhysicalCount != 11 || fail11.EffectiveCount != 6 || fail11.ExemptTestCount != 5 {
		t.Fatalf("5 productivos + 6 *_test.go debia fallar FTD: %+v", fail11)
	}

	// 4 productivos + 6 *_test.go hermanos = 10 fisicos, 5 exentos + 1 test extra contado 1:1 = 5 efectivos -> PASS
	pass4plus6 := EvaluateFTDBounds([]string{
		"0_Brain/engine/a.go",
		"0_Brain/engine/b.go",
		"0_Brain/engine/c.go",
		"0_Brain/engine/d.go",
		"0_Brain/engine/a_test.go",
		"0_Brain/engine/b_test.go",
		"0_Brain/engine/c_test.go",
		"0_Brain/engine/d_test.go",
		"0_Brain/engine/e_test.go",
		"0_Brain/engine/f_test.go",
	}, 5)
	if !pass4plus6.WithinBounds || pass4plus6.PhysicalCount != 10 || pass4plus6.EffectiveCount != 5 || pass4plus6.ExemptTestCount != 5 {
		t.Fatalf("4 productivos + 6 *_test.go hermanos debia contar el 6.o 1:1 (=5 efectivos, 10 fisicos) y pasar: %+v", pass4plus6)
	}

	// Test Go huerfano sin archivo productivo hermano en el mismo directorio -> 1:1 (5 + 1 = 6 -> FAIL)
	orphanFail := EvaluateFTDBounds([]string{
		"0_Brain/engine/a.go",
		"0_Brain/engine/b.go",
		"0_Brain/engine/c.go",
		"0_Brain/engine/d.go",
		"0_Brain/engine/e.go",
		"0_Brain/other/orphan_test.go",
	}, 5)
	if orphanFail.WithinBounds || orphanFail.EffectiveCount != 6 || orphanFail.ExemptTestCount != 0 {
		t.Fatalf("test Go huerfano debia contar 1:1 y fallar: %+v", orphanFail)
	}

	// Tests no-Go (Python/TS) siempre cuentan 1:1 -> 5 + 1 = 6 -> FAIL
	nonGoFail := EvaluateFTDBounds([]string{
		"app/a.py",
		"app/b.py",
		"app/c.py",
		"app/d.py",
		"app/e.py",
		"app/test_a.py",
	}, 5)
	if nonGoFail.WithinBounds || nonGoFail.EffectiveCount != 6 || nonGoFail.ExemptTestCount != 0 {
		t.Fatalf("test_a.py debia contar 1:1 y fallar: %+v", nonGoFail)
	}

	// Test Go en un segundo modulo raiz -> 2 modulos raiz -> FAIL
	multiModFail := EvaluateFTDBounds([]string{
		"0_Brain/engine/a.go",
		"1_Intake/engine/a_test.go",
	}, 5)
	if multiModFail.WithinBounds || len(multiModFail.RootModules) != 2 {
		t.Fatalf("test Go en segundo modulo raiz debia fallar: %+v", multiModFail)
	}

	// Archivo raiz "." + subdirectorio -> 2 modulos raiz -> FAIL
	rootPlusSub := EvaluateFTDBounds([]string{
		"README.md",
		"pkg/bfcore/paths.go",
	}, 5)
	if rootPlusSub.WithinBounds || len(rootPlusSub.RootModules) != 2 {
		t.Fatalf("archivo raiz + subdirectorio debia computar 2 modulos y fallar: %+v", rootPlusSub)
	}
}

func TestSanitizeRelativePaths_OptionsAndSecurity(t *testing.T) {
	repoRoot := `C:\Black Factory`

	// nil/empty -> non-nil []string{}
	if p, r := SanitizeRelativePaths(nil, repoRoot, PathSanitizeOpts{}); p == nil || len(p) != 0 || r != "" {
		t.Fatalf("SanitizeRelativePaths(nil) = %#v, %q", p, r)
	}

	// SplitCommas: true omite entradas vacias (paridad 1_Intake)
	if p, r := SanitizeRelativePaths([]string{" , ", "0_Brain/a.go, 0_Brain/A.go"}, repoRoot, PathSanitizeOpts{SplitCommas: true}); r != "" || len(p) != 2 {
		t.Fatalf("SplitCommas:true sin FoldCaseDedupe inesperado: %#v %q", p, r)
	}

	// SplitCommas: false rechaza entradas vacias
	if p, r := SanitizeRelativePaths([]string{"   "}, repoRoot, PathSanitizeOpts{SplitCommas: false}); p == nil || len(p) != 0 || r != "PATH_TRAVERSAL_OR_INVALID_SCOPE" {
		t.Fatalf("SplitCommas:false debia rechazar entrada vacia: %#v %q", p, r)
	}

	// FoldCaseDedupe: true deduplica preservando capitalizacion original
	if p, r := SanitizeRelativePaths([]string{"0_Brain/Engine/A.go", `0_brain\engine\a.go`}, repoRoot, PathSanitizeOpts{FoldCaseDedupe: true}); r != "" || len(p) != 1 || p[0] != "0_Brain/Engine/A.go" {
		t.Fatalf("FoldCaseDedupe:true inesperado: %#v %q", p, r)
	}

	// AllowDir: true preserva '/' final y rechaza '.', './', '/', globs y rutas personales
	if p, r := SanitizeRelativePaths([]string{`0_Brain\engine\`, `C:\Black Factory\1_Intake\engine/`}, repoRoot, PathSanitizeOpts{AllowDir: true, FoldCaseDedupe: true}); r != "" || len(p) != 2 || p[0] != "0_Brain/engine/" || p[1] != "1_Intake/engine/" {
		t.Fatalf("AllowDir:true inesperado: %#v %q", p, r)
	}
	for _, badDir := range []string{".", "./", "/", `\`, "../", "0_Brain/engine//", "0_Brain/*.go", `C:\` + `Users\alice\dir/`} {
		p, r := SanitizeRelativePaths([]string{badDir}, repoRoot, PathSanitizeOpts{AllowDir: true})
		if p == nil || len(p) != 0 || r == "" {
			t.Fatalf("AllowDir:true debia rechazar %q, obtuvo %#v %q", badDir, p, r)
		}
	}

	// AllowDir: false rechaza '/' final
	if p, r := SanitizeRelativePaths([]string{"0_Brain/engine/"}, repoRoot, PathSanitizeOpts{AllowDir: false}); p == nil || len(p) != 0 || r != "PATH_TRAVERSAL_OR_INVALID_SCOPE" {
		t.Fatalf("AllowDir:false debia rechazar directorio: %#v %q", p, r)
	}

	// Rutas personales G4 rechazadas ANTES de recortar repoRoot
	if p, r := SanitizeRelativePaths([]string{`C:\` + `Users\alice\repo\a.go`}, `C:\`+`Users\alice\repo`, PathSanitizeOpts{}); p == nil || len(p) != 0 || r != "G4_personal_path_in_affected_paths" {
		t.Fatalf("G4 debia evaluarse antes de recortar repoRoot: %#v %q", p, r)
	}
}

func TestCheckNoReparseUpToNearestExisting(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "exist")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedMissing := filepath.Join(existing, "not", "created", "yet")
	if err := CheckNoReparseUpToNearestExisting(root, nestedMissing); err != nil {
		t.Fatalf("CheckNoReparseUpToNearestExisting con root fallo: %v", err)
	}
	if err := CheckNoReparseUpToNearestExisting("", nestedMissing); err != nil {
		t.Fatalf("CheckNoReparseUpToNearestExisting con root vacio fallo: %v", err)
	}
	if err := CheckNoReparseUpToNearestExisting(root, "   "); err == nil {
		t.Fatalf("CheckNoReparseUpToNearestExisting con target vacio debia fallar")
	}
}

func TestComputeNormalizedFileSHA256(t *testing.T) {
	if MaxObservedFileBytes != 10<<20 {
		t.Fatalf("MaxObservedFileBytes = %d, esperaba 10 MiB", MaxObservedFileBytes)
	}
	repoRoot := t.TempDir()
	subDir := filepath.Join(repoRoot, "pkg")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	lfBytes := []byte("line1\nline2\nline3\n")
	crlfBytes := []byte("line1\r\nline2\r\nline3\r\n")
	crBytes := []byte("line1\rline2\rline3\r")
	bomCRLFBytes := append([]byte{0xef, 0xbb, 0xbf}, crlfBytes...)

	wantSum := sha256.Sum256(lfBytes)
	wantHex := hex.EncodeToString(wantSum[:])

	files := map[string][]byte{
		"pkg/lf.txt":       lfBytes,
		"pkg/crlf.txt":     crlfBytes,
		"pkg/cr.txt":       crBytes,
		"pkg/bom_crlf.txt": bomCRLFBytes,
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(rel)), content, 0o644); err != nil {
			t.Fatal(err)
		}
		state, sum, err := ComputeNormalizedFileSHA256(repoRoot, rel)
		if err != nil || state != "present" || sum != wantHex {
			t.Fatalf("ComputeNormalizedFileSHA256(%s) = (%q, %q, %v), esperaba (present, %s, nil)", rel, state, sum, err, wantHex)
		}
	}

	// Limite de bloque de streaming (32 KiB) con \r\n atravesando el corte del buffer
	bigLF := bytes.Repeat([]byte("a"), 32767)
	bigLF = append(bigLF, '\n')
	bigLF = append(bigLF, bytes.Repeat([]byte("b"), 40000)...)
	bigLF = append(bigLF, '\n')
	bigCRLF := bytes.ReplaceAll(bigLF, []byte("\n"), []byte("\r\n"))
	bigWant := sha256.Sum256(bigLF)
	if err := os.WriteFile(filepath.Join(repoRoot, "pkg", "big.txt"), bigCRLF, 0o644); err != nil {
		t.Fatal(err)
	}
	state, sum, err := ComputeNormalizedFileSHA256(repoRoot, "pkg/big.txt")
	if err != nil || state != "present" || sum != hex.EncodeToString(bigWant[:]) {
		t.Fatalf("streaming CRLF en limite de buffer fallo: state=%q sum=%q err=%v", state, sum, err)
	}

	// Archivo inexistente -> ("absent", "", nil)
	state, sum, err = ComputeNormalizedFileSHA256(repoRoot, "pkg/missing.go")
	if err != nil || state != "absent" || sum != "" {
		t.Fatalf("archivo inexistente debia retornar (absent, \"\", nil), obtuvo (%q, %q, %v)", state, sum, err)
	}

	// Directorio -> error
	if _, _, err := ComputeNormalizedFileSHA256(repoRoot, "pkg"); err == nil {
		t.Fatalf("ComputeNormalizedFileSHA256 sobre directorio debia fallar")
	}

	// Traversal ../ fuera de repoRoot -> error antes de Lstat
	if _, _, err := ComputeNormalizedFileSHA256(repoRoot, "../outside.go"); err == nil {
		t.Fatalf("ComputeNormalizedFileSHA256 con traversal ../ debia fallar")
	}
	if _, _, err := ComputeNormalizedFileSHA256(repoRoot, "pkg/dir/"); err == nil {
		t.Fatalf("ComputeNormalizedFileSHA256 con sufijo / debia fallar")
	}
	if _, _, err := ComputeNormalizedFileSHA256("", "pkg/lf.txt"); err == nil {
		t.Fatalf("ComputeNormalizedFileSHA256 con repoRoot vacio debia fallar")
	}
}

func TestBrainClient_PreflightMultiWrap_AndSharedHelpers(t *testing.T) {
	// RiskLevelRank
	cases := map[string]int{"NONE": 0, "BAJO": 1, "medio": 2, "ALTO": 3, "CRITICO": 4, "desconocido": 0}
	for lvl, want := range cases {
		if got := RiskLevelRank(lvl); got != want {
			t.Fatalf("RiskLevelRank(%q) = %d, esperaba %d", lvl, got, want)
		}
	}

	// FindUncoveredPaths
	docs := []BrainDocMatch{
		{ID: "d1", Type: "wiki", Status: "active", MatchedPaths: []string{"0_Brain/engine/a.go"}},
		{ID: "d2", Type: "adr", Status: "active", CoversPaths: []string{"pkg/bfcore/"}},
		{ID: "d3", Type: "wiki", Status: "draft", CoversPaths: []string{"2_Spec_Writer/"}},
		{ID: "d4", Type: "project", Status: "active", CoversPaths: []string{"2_Spec_Writer/"}},
	}
	uncov := FindUncoveredPaths([]string{"0_Brain/engine/a.go", "pkg/bfcore/ftd.go", "2_Spec_Writer/main.go"}, docs)
	if len(uncov) != 1 || uncov[0] != "2_Spec_Writer/main.go" {
		t.Fatalf("FindUncoveredPaths = %v, esperaba [2_Spec_Writer/main.go]", uncov)
	}

	// SubprocessBrainClient.Preflight multi-wrap
	vaultDir := t.TempDir()
	dbPath := filepath.Join(vaultDir, ".index", "brain.db")
	client := NewSubprocessBrainClient(filepath.Join(vaultDir, "missing_brain.exe"), 0)

	if _, err := client.Preflight("", ""); !errors.Is(err, ErrBrainUnavailable) || errors.Is(err, ErrBrainIndexMissing) || errors.Is(err, ErrBrainJournalActive) {
		t.Fatalf("Preflight con vault vacio debia ser solo ErrBrainUnavailable: %v", err)
	}

	// 1. Falta brain.db -> ErrBrainIndexMissing Y ErrBrainUnavailable, NO ErrBrainJournalActive
	if _, err := client.Preflight(vaultDir, ""); err == nil {
		t.Fatalf("Preflight sin brain.db debia fallar")
	} else if !errors.Is(err, ErrBrainIndexMissing) || !errors.Is(err, ErrBrainUnavailable) || errors.Is(err, ErrBrainJournalActive) {
		t.Fatalf("Preflight sin brain.db violo contrato multi-wrap: %v", err)
	}

	// 2. dbPath es un directorio -> ErrBrainUnavailable
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Preflight(vaultDir, dbPath); !errors.Is(err, ErrBrainUnavailable) {
		t.Fatalf("Preflight con dbPath directorio debia retornar ErrBrainUnavailable: %v", err)
	}
	_ = os.RemoveAll(dbPath)

	// 3. Archivo regular brain.db limpio -> OK
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("sqlite-mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := client.Preflight(vaultDir, "")
	if err != nil || pf.DBPath != dbPath || pf.BrainDBMTime == "" {
		t.Fatalf("Preflight limpio fallo: pf=%+v err=%v", pf, err)
	}

	// 4. Presente .brain-journal.json -> ErrBrainJournalActive Y ErrBrainUnavailable, NO ErrBrainIndexMissing
	journalPath := filepath.Join(vaultDir, ".brain-journal.json")
	if err := os.WriteFile(journalPath, []byte(`{"proposal_id":"p1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Preflight(vaultDir, dbPath); err == nil {
		t.Fatalf("Preflight con .brain-journal.json debia fallar")
	} else if !errors.Is(err, ErrBrainJournalActive) || !errors.Is(err, ErrBrainUnavailable) || errors.Is(err, ErrBrainIndexMissing) {
		t.Fatalf("Preflight con .brain-journal.json violo contrato multi-wrap: %v", err)
	}

	// 5. Context y ExecBrain con binario faltante -> ErrBrainUnavailable
	if _, inv, err := client.Context(vaultDir, dbPath, "black-factory", []string{"a.go"}); !errors.Is(err, ErrBrainUnavailable) || inv.ExitCode != -1 {
		t.Fatalf("Context con binario ausente debia retornar ErrBrainUnavailable y ExitCode=-1: inv=%+v err=%v", inv, err)
	}

	// 6. Subproceso real si 0_Brain/brain.exe y 0_Brain/vault/.index/brain.db existen en el repo local
	realBrainBin := filepath.Clean(filepath.Join("..", "..", "0_Brain", "brain.exe"))
	realVault := filepath.Clean(filepath.Join("..", "..", "0_Brain", "vault"))
	if _, err := os.Stat(realBrainBin); err == nil {
		if _, errV := os.Stat(filepath.Join(realVault, ".index", "brain.db")); errV == nil {
			realClient := NewSubprocessBrainClient(realBrainBin, 5*time.Second)
			pfReal, err := realClient.Preflight(realVault, "")
			if err != nil {
				t.Fatalf("realClient.Preflight fallo: %v", err)
			}
			ctxResp, inv, err := realClient.Context(realVault, pfReal.DBPath, "black-factory", []string{"gengiswap/engine/ring.go"})
			if err != nil || inv.ExitCode != 0 || !ctxResp.HasActiveRiskZones || len(ctxResp.RiskHits) == 0 {
				t.Fatalf("realClient.Context fallo: err=%v inv=%+v resp=%+v", err, inv, ctxResp)
			}
		}
	}
}
