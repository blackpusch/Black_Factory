package bfcore

import (
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
