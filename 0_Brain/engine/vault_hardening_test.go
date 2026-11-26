package brain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 1. Pruebas de Anclaje de Allowlist y Extensiones (F2)
func TestAllowlistAnchoringAndEdgeCases(t *testing.T) {
	cases := []struct {
		path    string
		wantErr bool
	}{
		{"foo/wiki/x.md", true},
		{"xwiki/x.md", true},
		{"wiki/sub/x.md", true},
		{"wiki/x.mdx", true},
		{"wiki/.x.md", true},
		{"wiki/x..md", true},
		{"adr/sub/ADR-0001-test.md", true},
		{"projects/sub/app.md", true},
		{"wiki/x.md", false},
		{"adr/ADR-0001-inicio.md", false},
		{"projects/black-factory.md", false},
	}

	for _, tc := range cases {
		t.Run("check_"+tc.path, func(t *testing.T) {
			_, err := validateSafeTargetPath(tc.path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateSafeTargetPath(%q): err=%v, wantErr=%v", tc.path, err, tc.wantErr)
			}
		})
	}
}

// 2. Pruebas de Nombre de Propuesta y Bloqueo de Streams ADS (F4)
func TestProposalFilename_StreamsAndFormats(t *testing.T) {
	vault := t.TempDir()
	proposalsDir := filepath.Join(vault, "_proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	invalidNames := []string{
		"patch-01.md:oculto",
		"patch-01.md::$DATA",
		"patch~1.md",
		"patch 01.md",
		"PATCH-01.MD",
		"wiki_patch.md",
		"patch-bad.txt",
		"../outside.md",
	}

	for _, inv := range invalidNames {
		t.Run("invalid_"+inv, func(t *testing.T) {
			_, err := resolveProposalFile(vault, inv)
			if err == nil {
				t.Fatalf("se esperaba rechazo para propuesta con nombre o stream no permitido %q", inv)
			}
		})
	}

	validNames := []string{
		"patch-01.md",
		"patch-20261120-150405-wiki-seguridad.md",
		"patch-20261120-test-slug.md",
	}

	for _, val := range validNames {
		t.Run("valid_"+val, func(t *testing.T) {
			target := filepath.Join(proposalsDir, val)
			_ = os.WriteFile(target, []byte("# Test"), 0o644)
			got, err := resolveProposalFile(vault, val)
			if err != nil {
				t.Fatalf("se esperaba aceptacion para nombre valido %q: %v", val, err)
			}
			if filepath.Clean(got) != filepath.Clean(target) {
				t.Fatalf("esperado %s, obtenido %s", target, got)
			}
		})
	}
}

// 3. Contención F3: Redirección interna dentro de la bóveda
func TestF3_InternalRedirectionContainment(t *testing.T) {
	vault := t.TempDir()
	wikiDir := filepath.Join(vault, "wiki")
	_ = os.MkdirAll(wikiDir, 0o755)

	// Simulación de _proposals apuntando a vault/wiki
	// Si os.Symlink falla por permisos, lo verificamos conceptualmente
	proposalsDir := filepath.Join(vault, "_proposals")
	err := os.Symlink(wikiDir, proposalsDir)
	if err != nil {
		t.Logf("SKIP live symlink por restricciones del SO: %v", err)
		return
	}

	_, rErr := resolveProposalFile(vault, "patch-01.md")
	if rErr == nil {
		t.Fatalf("resolveProposalFile debio rechazar _proposals redirigido a vault/wiki")
	}
}

// 4. Bloqueo de Kernel a Nivel de SO (J1)
func TestLockConcurrency_OSLevel(t *testing.T) {
	vault := t.TempDir()

	lock1, err := AcquireVaultLock(vault)
	if err != nil {
		t.Fatalf("fallo adquiriendo lock inicial: %v", err)
	}

	// Segundo intento concurrente debe fallar inmediatamente con ErrVaultLocked
	lock2, err2 := AcquireVaultLock(vault)
	if err2 == nil {
		_ = lock2.Unlock()
		t.Fatalf("se esperaba ErrVaultLocked en intento concurrente")
	}
	if !errors.Is(err2, ErrVaultLocked) {
		t.Fatalf("se esperaba ErrVaultLocked, obtenido: %v", err2)
	}

	// Al liberar lock1, lock2 debe poder adquirirse
	if err := lock1.Unlock(); err != nil {
		t.Fatalf("error liberando lock1: %v", err)
	}

	lock3, err3 := AcquireVaultLock(vault)
	if err3 != nil {
		t.Fatalf("fallo adquiriendo lock tras liberacion: %v", err3)
	}
	_ = lock3.Unlock()

	// J1 Enmienda 1: .brain.lock persiste en disco y no debe ser eliminado
	lockPath := filepath.Join(vault, ".brain.lock")
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Fatalf(".brain.lock debia persistir en disco tras unlock: %v", statErr)
	}
}

// 5. Bordes del Journal y Resiliencia (J2)
func TestJournalEdgeCases_RecoveryAndFailClosed(t *testing.T) {
	vault := t.TempDir()
	_ = InitVault(vault, "C:\\Black Factory\\0_Brain\\schemas")

	// 5.1 Journal truncado -> ErrJournalCorrupt
	journalPath := filepath.Join(vault, ".brain-journal.json")
	_ = os.WriteFile(journalPath, []byte("{ truncated json"), 0o644)
	err := RecoverJournal(vault)
	if err == nil || !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("se esperaba ErrJournalCorrupt ante journal truncado, obtenido: %v", err)
	}
	_ = os.Remove(journalPath)

	// 5.2 No-op update -> ErrNoOpUpdate
	constDoc := filepath.Join(vault, "constitution.md")
	rawConst, _ := os.ReadFile(constDoc)

	propRes, pErr := ProposeUpdate(vault, ProposeRequest{
		TargetPath: "constitution.md",
		Operation:  "update",
		Content:    string(rawConst),
	})
	if pErr != nil {
		t.Fatalf("ProposeUpdate fallo: %v", pErr)
	}

	_, applyErr := ApplyUpdate(vault, ApplyOptions{
		ProposalPath: propRes.ProposalPath,
		ApproveAutor: true,
	})
	if applyErr == nil || !errors.Is(applyErr, ErrNoOpUpdate) {
		t.Fatalf("se esperaba ErrNoOpUpdate ante update con contenido identico, obtenido: %v", applyErr)
	}
}

// 6. Vector Criptográfico N1 Corregido con Reloj Inyectado (M1)
func TestCorrectedVectorN1_WithInjectedClock(t *testing.T) {
	vault := t.TempDir()
	_ = InitVault(vault, "C:\\Black Factory\\0_Brain\\schemas")

	// 1. Configurar clave publica en vault/.autor/autor.pub
	autorDir := filepath.Join(vault, ".autor")
	_ = os.MkdirAll(autorDir, 0o755)
	pubKeyB64 := "yTEBvopz7UoWksSeGiyC5zSGnutZRB7sxrfLdFHfx0w="
	_ = os.WriteFile(filepath.Join(autorDir, "autor.pub"), []byte(pubKeyB64), 0o644)

	// 2. Bloque embebido (exactamente 324 bytes)
	block := "---\n" +
		"id: wiki-seguridad\n" +
		"title: \"Wiki de Seguridad\"\n" +
		"type: wiki\n" +
		"project: black-factory\n" +
		"status: active\n" +
		"tags:\n" +
		"  - wiki\n" +
		"  - security\n" +
		"covers_paths:\n" +
		"  - 0_Brain/engine/\n" +
		"verified_head: \"8316011\"\n" +
		"updated_at: \"2026-11-20\"\n" +
		"---\n\n" +
		"# Wiki de Seguridad\n\n" +
		"Este documento centraliza las politicas de validacion G4 implementadas en el motor Go.\n"

	blockBytes := []byte(block)
	if len(blockBytes) != 324 {
		t.Fatalf("el bloque mide %d bytes, esperado 324", len(blockBytes))
	}
	contentHash := sha256Hex(block)
	expectedHash := "0ba7c8436b651767a92ef669a6de8d574c1ffe5a2cf98e0466cb8b0cefd23a7d"
	if contentHash != expectedHash {
		t.Fatalf("content_sha256=%s, esperado=%s", contentHash, expectedHash)
	}

	// 3. Crear propuesta en _proposals
	proposalsDir := filepath.Join(vault, "_proposals")
	propPath := filepath.Join(proposalsDir, "patch-20261120-150405-wiki-seguridad.md")
	propContent := fmt.Sprintf("---\n"+
		"id: patch-20261120-150405-wiki-seguridad\n"+
		"title: \"BrainPatch: Wiki de Seguridad\"\n"+
		"type: brain_patch\n"+
		"project: black-factory\n"+
		"status: proposed\n"+
		"target_path: wiki/seguridad.md\n"+
		"operation: create\n"+
		"requires_autor_approval: true\n"+
		"expires_at: \"2026-11-20T18:00:00Z\"\n"+
		"---\n\n"+
		"## Contenido Propuesto\n\n"+
		"````markdown\n%s````\n", block)
	_ = os.WriteFile(propPath, []byte(propContent), 0o644)

	// 4. Firma del vector
	sigB64 := "OXYFjZ6k1c2liq+aZZEbuurl+3qJFcMNfReB5CW+lG+5GlsClibGLOLoG5Kg/Lxq1LLC/ZdvytXNZ4o9JMrQCg=="

	// 5. Aplicar con reloj inyectado antes de expiracion (2026-11-20T12:00:00Z)
	validClock := func() time.Time {
		tVal, _ := time.Parse(time.RFC3339, "2026-11-20T12:00:00Z")
		return tVal
	}
	res, err := ApplyUpdate(vault, ApplyOptions{
		ProposalPath: "patch-20261120-150405-wiki-seguridad.md",
		AutorSig:     sigB64,
		NowFunc:      validClock,
	})
	if err != nil {
		t.Fatalf("ApplyUpdate con vector N1 fallo: %v", err)
	}
	if res.TargetPath != "wiki/seguridad.md" {
		t.Fatalf("target inesperado: %s", res.TargetPath)
	}

	// 6. Verificar que la propuesta quedo marcada como status: applied
	propDoc, err := ParseFile(vault, propPath)
	if err != nil {
		t.Fatalf("leyendo propuesta aplicada: %v", err)
	}
	if propDoc.Meta.Status != "applied" {
		t.Fatalf("estado de propuesta esperado 'applied', obtenido: %s", propDoc.Meta.Status)
	}

	// 7. Re-aplicacion de la misma propuesta debe fallar por idempotencia (ErrProposalAlreadyApplied)
	_, reApplyErr := ApplyUpdate(vault, ApplyOptions{
		ProposalPath: "patch-20261120-150405-wiki-seguridad.md",
		AutorSig:     sigB64,
		NowFunc:      validClock,
	})
	if reApplyErr == nil || !errors.Is(reApplyErr, ErrProposalAlreadyApplied) {
		t.Fatalf("se esperaba ErrProposalAlreadyApplied en re-aplicacion, obtenido: %v", reApplyErr)
	}

	// 8. Aplicar con reloj inyectado POST-expiracion (2026-11-20T19:00:00Z) debe fallar con error de firma expirada
	_ = os.Remove(filepath.Join(vault, "wiki", "seguridad.md"))
	expiredClock := func() time.Time {
		tVal, _ := time.Parse(time.RFC3339, "2026-11-20T19:00:00Z")
		return tVal
	}
	// Creamos otra propuesta identica
	propPathExp := filepath.Join(proposalsDir, "patch-20261120-150405-exp.md")
	propContentExp := strings.Replace(propContent, "patch-20261120-150405-wiki-seguridad", "patch-20261120-150405-exp", -1)
	_ = os.WriteFile(propPathExp, []byte(propContentExp), 0o644)

	_, expErr := ApplyUpdate(vault, ApplyOptions{
		ProposalPath: "patch-20261120-150405-exp.md",
		AutorSig:     sigB64,
		NowFunc:      expiredClock,
	})
	if expErr == nil || !strings.Contains(expErr.Error(), "firma Ed25519 expirada") {
		t.Fatalf("se esperaba error de firma expirada, obtenido: %v", expErr)
	}
}

// 7. Casos Diferenciales en ParseDocument (J3)
func TestJ3_DifferentialParserCases(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		expectedType string
		wantErr      bool
	}{
		{
			name: "dobles_comillas",
			input: "---\n" +
				"id: test-1\n" +
				"title: Test 1\n" +
				"type: \"constitution\"\n" +
				"status: active\n" +
				"---\n# Test",
			expectedType: "constitution",
			wantErr:      false,
		},
		{
			name: "comillas_simples",
			input: "---\n" +
				"id: test-2\n" +
				"title: Test 2\n" +
				"type: 'constitution'\n" +
				"status: active\n" +
				"---\n# Test",
			expectedType: "constitution",
			wantErr:      false,
		},
		{
			name: "comentario_final_de_linea",
			input: "---\n" +
				"id: test-3\n" +
				"title: Test 3\n" +
				"type: constitution # politica inmutable\n" +
				"status: active\n" +
				"---\n# Test",
			expectedType: "constitution",
			wantErr:      false,
		},
		{
			name: "crlf_normalizacion",
			input: "---\r\n" +
				"id: test-4\r\n" +
				"title: Test 4\r\n" +
				"type: constitution\r\n" +
				"status: active\r\n" +
				"---\r\n# Test CRLF",
			expectedType: "constitution",
			wantErr:      false,
		},
		{
			name: "utf8_con_bom",
			input: "\xef\xbb\xbf---\n" +
				"id: test-5\n" +
				"title: Test 5\n" +
				"type: constitution\n" +
				"status: active\n" +
				"---\n# Test BOM",
			expectedType: "constitution",
			wantErr:      false,
		},
	}

	for _, tc := range cases {
		t.Run("parser_"+tc.name, func(t *testing.T) {
			doc, err := ParseDocument("doc.md", tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseDocument error: %v, wantErr: %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				if doc.Meta.Type != tc.expectedType {
					t.Fatalf("Meta.Type=%q, esperado=%q", doc.Meta.Type, tc.expectedType)
				}
			}
		})
	}
}

// 8. Generación y Firma con brain sign
func TestSignProposalAndVerifyRoundTrip(t *testing.T) {
	vault := t.TempDir()
	_ = InitVault(vault, "C:\\Black Factory\\0_Brain\\schemas")

	// Generar keypair determinista
	seed := sha256.Sum256([]byte("brain-test-seed-2026-11-20"))
	privKey := ed25519.NewKeyFromSeed(seed[:])
	pubKey := privKey.Public().(ed25519.PublicKey)

	autorDir := filepath.Join(vault, ".autor")
	_ = os.MkdirAll(autorDir, 0o755)
	_ = os.WriteFile(filepath.Join(autorDir, "autor.pub"), []byte(base64.StdEncoding.EncodeToString(pubKey)), 0o644)

	privKeyFile := filepath.Join(vault, ".autor", "autor.key")
	_ = os.WriteFile(privKeyFile, []byte(hex.EncodeToString(privKey)), 0o600)

	// Crear propuesta
	proposalsDir := filepath.Join(vault, "_proposals")
	propPath := filepath.Join(proposalsDir, "patch-20261120-150405-test.md")
	propBody := "---\n" +
		"id: patch-20261120-150405-test\n" +
		"title: \"Test Sign\"\n" +
		"type: brain_patch\n" +
		"project: black-factory\n" +
		"status: proposed\n" +
		"target_path: wiki/seguridad.md\n" +
		"operation: create\n" +
		"requires_autor_approval: true\n" +
		"---\n\n" +
		"## Contenido Propuesto\n\n" +
		"````markdown\n" +
		"---\n" +
		"id: wiki-seguridad\n" +
		"title: \"Wiki Seguridad\"\n" +
		"type: wiki\n" +
		"project: black-factory\n" +
		"status: active\n" +
		"tags: [security]\n" +
		"covers_paths: [\"0_Brain/\"]\n" +
		"verified_head: \"8316011\"\n" +
		"updated_at: \"2026-11-20\"\n" +
		"---\n\n" +
		"# Wiki Seguridad\n" +
		"````\n"

	_ = os.WriteFile(propPath, []byte(propBody), 0o644)

	// Firmar con SignProposal
	sigB64, err := SignProposal(vault, "patch-20261120-150405-test.md", privKeyFile, nil)
	if err != nil {
		t.Fatalf("SignProposal fallo: %v", err)
	}

	// Aplicar con la firma generada
	res, err := ApplyUpdate(vault, ApplyOptions{
		ProposalPath: "patch-20261120-150405-test.md",
		AutorSig:     sigB64,
	})
	if err != nil {
		t.Fatalf("ApplyUpdate con firma generada fallo: %v", err)
	}
	if res.TargetPath != "wiki/seguridad.md" {
		t.Fatalf("target inesperado: %s", res.TargetPath)
	}
}
