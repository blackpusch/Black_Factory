package brain

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestValidateSafeTargetPath_CanonicalAllowlist(t *testing.T) {
	valid := []string{
		"AGENTS.md",
		"index.md",
		"constitution.md",
		"wiki/risk_zones.md",
		"wiki/seguridad.md",
		"wiki/modulo_v1-final.md",
		"adr/ADR-0001-inicio.md",
		"adr/ADR-9999-decision-arquitectura.md",
		"projects/black-factory.md",
		"projects/mi_proyecto-2.md",
	}

	for _, tc := range valid {
		t.Run("valid_"+tc, func(t *testing.T) {
			got, err := validateSafeTargetPath(tc)
			if err != nil {
				t.Fatalf("se esperaba ruta valida para %q, fallo con: %v", tc, err)
			}
			if got != tc {
				t.Fatalf("se esperaba ruta %q, obtuvo %q", tc, got)
			}
		})
	}
}

func TestValidateSafeTargetPath_NonCanonicalCasingRejected(t *testing.T) {
	nonCanonical := []string{
		"agents.md",
		"Agents.md",
		"INDEX.MD",
		"Index.md",
		"Constitution.md",
		"CONSTITUTION.MD",
		"wiki/Risk_zones.md",
		"wiki/RISK_ZONES.MD",
		"Wiki/seguridad.md",
		"WIKI/seguridad.md",
		"wiki/Seguridad.md",
		"adr/adr-0001-inicio.md",
		"adr/Adr-0001-inicio.md",
		"adr/ADR-0001-Inicio.md",
		"Projects/black-factory.md",
		"projects/Black-Factory.md",
	}

	for _, tc := range nonCanonical {
		t.Run("casing_"+tc, func(t *testing.T) {
			_, err := validateSafeTargetPath(tc)
			if err == nil {
				t.Fatalf("se esperaba rechazo por capitalizacion no canonica para %q", tc)
			}
		})
	}
}

func TestValidateSafeTargetPath_DOSDeviceNamesRejected(t *testing.T) {
	dosDevices := []string{
		"wiki/con.md",
		"wiki/CON.md",
		"wiki/prn.md",
		"wiki/aux.md",
		"projects/nul.md",
		"projects/com1.md",
		"projects/lpt1.md",
		"adr/aux.md",
		"adr/con.md",
	}

	for _, tc := range dosDevices {
		t.Run("dos_"+tc, func(t *testing.T) {
			_, err := validateSafeTargetPath(tc)
			if err == nil {
				t.Fatalf("se esperaba rechazo por nombre de dispositivo DOS reservado para %q", tc)
			}
		})
	}
}

func TestValidateSafeTargetPath_ProhibitedCharactersAndStreams(t *testing.T) {
	prohibited := []string{
		"wiki/log.md:stream",
		"wiki/test:1.md",
		"wiki/doc~1.md",
		"wiki/doc?.md",
		"wiki/doc*.md",
		"wiki/doc<1>.md",
		"wiki/doc|1.md",
		"wiki/doc\"1.md",
		"wiki/ doc.md",
		"wiki/doc .md",
		"wiki/.doc.md",
		"wiki/doc..md",
		"wiki/seguridad.txt",
		"wiki/seguridad.exe",
		"wiki/sub/doc.md", // subdirectorio anidado no permitido
		"projects/sub/app.md",
		"adr/sub/ADR-0001-app.md",
		"log.md",
		"README_PRIVADO.md",
		"README.md",
		"_proposals/patch-01.md",
		".index/brain.db",
		".obsidian/workspace.md",
		"foo/bar.md",
	}

	for _, tc := range prohibited {
		t.Run("prohibited_"+tc, func(t *testing.T) {
			_, err := validateSafeTargetPath(tc)
			if err == nil {
				t.Fatalf("se esperaba rechazo para %q", tc)
			}
		})
	}
}

func TestValidateSafeTargetPath_AgainstRealVaultFiles_ExactCounts(t *testing.T) {
	realVault := `C:\Black Factory\0_Brain\vault`
	files, err := ListVaultMarkdownFiles(realVault)
	if err != nil {
		t.Skipf("no se pudo acceder a boveda real %s: %v", realVault, err)
	}

	expectedTotal := 6
	if len(files) < expectedTotal {
		t.Fatalf("se esperaban al menos %d archivos .md en la boveda real, encontrados %d", expectedTotal, len(files))
	}

	var accepted []string
	var rejected []string

	for _, absFile := range files {
		rel, err := filepath.Rel(realVault, absFile)
		if err != nil {
			t.Fatalf("filepath.Rel error: %v", err)
		}
		normRel := NormalizeRelPath(rel)
		got, err := validateSafeTargetPath(normRel)
		if err != nil {
			rejected = append(rejected, normRel)
		} else {
			if got != normRel {
				t.Fatalf("se esperaba ruta canónica %q, obtuvo %q", normRel, got)
			}
			accepted = append(accepted, normRel)
		}
	}

	if len(accepted) < 4 {
		t.Fatalf("se esperaban al menos 4 archivos aceptados, obtenidos %d: %v", len(accepted), accepted)
	}
	if len(rejected) != 2 {
		t.Fatalf("se esperaban exactamente 2 archivos rechazados, obtenidos %d: %v", len(rejected), rejected)
	}

	acceptedSet := make(map[string]bool, len(accepted))
	for _, acc := range accepted {
		acceptedSet[acc] = true
	}
	for _, reqAcc := range []string{"AGENTS.md", "index.md", "constitution.md", "wiki/risk_zones.md"} {
		if !acceptedSet[reqAcc] {
			t.Errorf("archivo nucleo %s debia ser aceptado", reqAcc)
		}
	}

	expectedRejected := map[string]bool{
		"log.md":            true,
		"README_PRIVADO.md": true,
	}
	for _, rej := range rejected {
		if !expectedRejected[rej] {
			t.Errorf("archivo rechazado no esperado: %s", rej)
		}
	}
}

type mockFileInfo struct {
	name    string
	mode    os.FileMode
	isDir   bool
	sysData any
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 100 }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Now() }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() any           { return m.sysData }

func TestIsReparseOrSymlink_ModesAndAttributes(t *testing.T) {
	// 1. Archivo regular normal
	regularFile := mockFileInfo{name: "doc.md", mode: 0o644, isDir: false}
	if isReparseOrSymlink(regularFile) {
		t.Errorf("archivo regular marcado incorrectamente como reparse/symlink")
	}

	// 2. Directorio normal
	regularDir := mockFileInfo{name: "wiki", mode: os.ModeDir | 0o755, isDir: true}
	if isReparseOrSymlink(regularDir) {
		t.Errorf("directorio regular marcado incorrectamente como reparse/symlink")
	}

	// 3. Enlace simbolico clasico (ModeSymlink)
	symlinkFile := mockFileInfo{name: "link.md", mode: os.ModeSymlink | 0o644, isDir: false}
	if !isReparseOrSymlink(symlinkFile) {
		t.Errorf("symlink clasico con ModeSymlink no fue detectado")
	}

	// 4. Directory junction en Go 1.23+ (marcado con ModeIrregular)
	junctionDirGo123 := mockFileInfo{name: "junction_dir", mode: os.ModeDir | os.ModeIrregular | 0o755, isDir: true}
	if !isReparseOrSymlink(junctionDirGo123) {
		t.Errorf("junction Go 1.23+ con ModeIrregular no fue detectado")
	}

	// 5. Atributo nativo Win32 FILE_ATTRIBUTE_REPARSE_POINT (0x400)
	winReparseData := &syscall.Win32FileAttributeData{
		FileAttributes: 0x00000400, // FILE_ATTRIBUTE_REPARSE_POINT
	}
	winReparseFile := mockFileInfo{name: "reparse.md", mode: 0o644, isDir: false, sysData: winReparseData}
	if !isReparseOrSymlink(winReparseFile) {
		t.Errorf("reparse point con atributo Win32 0x400 no fue detectado")
	}
}

func TestResolveProposalFile(t *testing.T) {
	vault := t.TempDir()
	proposalsDir := filepath.Join(vault, "_proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	validPatch := filepath.Join(proposalsDir, "patch-20261120-test.md")
	if err := os.WriteFile(validPatch, []byte("# Patch Test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Archivo fuera de _proposals
	outsideFile := filepath.Join(vault, "wiki_test.md")
	if err := os.WriteFile(outsideFile, []byte("# Outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Atajo con nombre simple
	got, err := resolveProposalFile(vault, "patch-20261120-test.md")
	if err != nil {
		t.Fatalf("fallo atajo con nombre simple: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(validPatch) {
		t.Fatalf("se esperaba %q, obtuvo %q", validPatch, got)
	}

	// 2. Ruta relativa con prefijo _proposals/
	got, err = resolveProposalFile(vault, "_proposals/patch-20261120-test.md")
	if err != nil {
		t.Fatalf("fallo ruta relativa con prefijo: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(validPatch) {
		t.Fatalf("se esperaba %q, obtuvo %q", validPatch, got)
	}

	// 3. Ruta absoluta valida dentro de _proposals/
	got, err = resolveProposalFile(vault, validPatch)
	if err != nil {
		t.Fatalf("fallo ruta absoluta valida: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(validPatch) {
		t.Fatalf("se esperaba %q, obtuvo %q", validPatch, got)
	}

	// 4. Intentos de escape
	escapes := []string{
		"wiki_test.md",
		"_proposals/../wiki_test.md",
		"../outside.md",
		outsideFile,
	}
	for _, esc := range escapes {
		t.Run("escape_"+esc, func(t *testing.T) {
			_, err := resolveProposalFile(vault, esc)
			if err == nil {
				t.Fatalf("se esperaba rechazo para propuesta fuera de _proposals: %s", esc)
			}
		})
	}
}

func TestSymlinkAndJunction_Protection_LiveIfSupported(t *testing.T) {
	tempDir := t.TempDir()
	vault := filepath.Join(tempDir, "vault")
	if err := os.MkdirAll(filepath.Join(vault, "_proposals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(vault, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Intento de creacion de symlink real
	realTarget := filepath.Join(tempDir, "secret_outside.txt")
	_ = os.WriteFile(realTarget, []byte("secreto"), 0o644)
	symlinkPath := filepath.Join(vault, "wiki", "symlink_doc.md")

	err := os.Symlink(realTarget, symlinkPath)
	if err != nil {
		t.Logf("NOTA ENTORNO WINDOWS: os.Symlink no esta disponible sin privilegios/modo desarrollador (%v). La proteccion se garantiza mediante verificacion estatica de flags os.ModeSymlink, os.ModeIrregular y Win32 FileAttributes 0x400 en TestIsReparseOrSymlink_ModesAndAttributes.", err)
		return
	}

	// Si el SO soporte creacion de symlink:
	t.Run("reject_proposal_symlink", func(t *testing.T) {
		propSymlink := filepath.Join(vault, "_proposals", "symlink_patch.md")
		if err := os.Symlink(realTarget, propSymlink); err == nil {
			_, err := resolveProposalFile(vault, "symlink_patch.md")
			if err == nil {
				t.Errorf("resolveProposalFile deberia haber rechazado un symlink de propuesta")
			}
		}
	})
}
