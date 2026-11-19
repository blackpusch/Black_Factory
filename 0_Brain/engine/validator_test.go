package brain

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

func makeValidVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeTestFile(t, root, "AGENTS.md", `---
id: brain-agents
title: "Enrutador Operativo"
type: agents
project: black-factory
status: active
tags:
  - agents
covers_paths: []
verified_head: "abc1234"
updated_at: "2026-11-19"
---

# Enrutador
Consulta [index.md](index.md) y [constitution.md](constitution.md).
`)

	writeTestFile(t, root, "index.md", `---
id: brain-index
title: "Indice Maestro"
type: index
project: black-factory
status: active
tags:
  - index
covers_paths: []
verified_head: ""
updated_at: "2026-11-19"
---

# Indice
- [Constitucion](constitution.md#principios)
- [Zonas](wiki/risk_zones.md)
- [ADR 0001](adr/ADR-0001.md)
`)

	writeTestFile(t, root, "constitution.md", `---
id: brain-constitution
title: "Constitucion"
type: constitution
project: black-factory
status: active
tags:
  - governance
covers_paths: []
verified_head: ""
updated_at: "2026-11-19"
---

# Constitucion
Ver [Bitacora](log.md).
`)

	writeTestFile(t, root, "log.md", "# Bitacora\n\n- [2026-11-19] Inicializacion.\n")
	writeTestFile(t, root, "README_PRIVADO.md", "# Privado\n\nSolo lectura local.\n")

	writeTestFile(t, root, "wiki/risk_zones.md", `---
id: brain-risk-zones
title: "Zonas de Riesgo"
type: risk_zones
project: black-factory
status: active
tags:
  - security
covers_paths: []
verified_head: ""
updated_at: "2026-11-19"
---

# Zonas de Riesgo
Volver a [Constitucion](../constitution.md).
`)

	writeTestFile(t, root, "adr/ADR-0001.md", `---
id: adr-0001-okf
title: "ADR-0001: OKF"
type: adr
project: black-factory
status: accepted
tags:
  - adr
covers_paths:
  - 0_Brain/engine/
verified_head: "691ba6a"
updated_at: "2026-11-19"
---

# ADR-0001
Referencia externa [Go](https://go.dev/doc/) permitida.
`)

	return root
}

func hasIssue(issues []ValidationIssue, code IssueCode, file string) bool {
	for _, iss := range issues {
		if iss.Code == code && (file == "" || iss.File == file) {
			return true
		}
	}
	return false
}

func countIssueCode(issues []ValidationIssue, code IssueCode) int {
	n := 0
	for _, iss := range issues {
		if iss.Code == code {
			n++
		}
	}
	return n
}

func TestParseDocument_FrontmatterAndLinks(t *testing.T) {
	fence := strings.Repeat("`", 3)
	raw := "\xef\xbb\xbf" +
		"---\r\n" +
		"# comentario inicial\r\n" +
		"id: patch-0001 # comentario inline\r\n" +
		"title: \"Titulo: con dos puntos y # numeral\"\r\n" +
		"type: brain_patch\r\n" +
		"project: black-factory\r\n" +
		"status: proposed\r\n" +
		"tags: [\"alpha\", 'beta', gamma]\r\n" +
		"covers_paths:\r\n" +
		"  - 0_Brain/engine/\r\n" +
		"  - Docs/BLACK_FACTORY_BRAIN.md\r\n" +
		"verified_head: '691ba6a'\r\n" +
		"updated_at: \"2026-11-19\"\r\n" +
		"target_path: wiki/ejemplo.md\r\n" +
		"operation: update\r\n" +
		"requires_autor_approval: false\r\n" +
		"---\r\n" +
		"\r\n" +
		"# Cuerpo\r\n" +
		"Enlace valido [Uno](wiki/doc.md#sec-1) y [Dos](../index.md).\r\n" +
		"Ignorar [Ext](https://example.com), [HTTP](http://localhost:8080), [Mail](mailto:test@example.com), [Ancla](#local).\r\n" +
		"Ignorar codigo inline `" + "[Inline](no_existe.md)" + "` y imagen ![img](pic.png).\r\n" +
		fence + "markdown\r\n" +
		"[Fenced](fenced_inexistente.md)\r\n" +
		fence + "\r\n" +
		"~~~\r\n" +
		"[TildeFenced](tilde_inexistente.md)\r\n" +
		"~~~\r\n" +
		"Enlace final [Tres](adr/ADR-0001.md \"titulo\").\r\n"

	doc, err := ParseDocument("wiki/test.md", raw)
	if err != nil {
		t.Fatalf("ParseDocument error inesperado: %v", err)
	}
	if !doc.HasFrontmatter {
		t.Fatalf("se esperaba HasFrontmatter=true")
	}
	if doc.Meta.ID != "patch-0001" {
		t.Fatalf("ID = %q, esperaba patch-0001", doc.Meta.ID)
	}
	if doc.Meta.Title != "Titulo: con dos puntos y # numeral" {
		t.Fatalf("Title = %q", doc.Meta.Title)
	}
	if doc.Meta.Type != "brain_patch" || doc.Meta.Project != "black-factory" || doc.Meta.Status != "proposed" {
		t.Fatalf("metadatos inesperados: %+v", doc.Meta)
	}
	if doc.Meta.VerifiedHead != "691ba6a" || doc.Meta.UpdatedAt != "2026-11-19" {
		t.Fatalf("verified_head/updated_at inesperados: %+v", doc.Meta)
	}
	if len(doc.Meta.Tags) != 3 || doc.Meta.Tags[0] != "alpha" || doc.Meta.Tags[1] != "beta" || doc.Meta.Tags[2] != "gamma" {
		t.Fatalf("Tags = %#v", doc.Meta.Tags)
	}
	if len(doc.Meta.CoversPaths) != 2 || doc.Meta.CoversPaths[0] != "0_Brain/engine/" || doc.Meta.CoversPaths[1] != "Docs/BLACK_FACTORY_BRAIN.md" {
		t.Fatalf("CoversPaths = %#v", doc.Meta.CoversPaths)
	}
	if doc.Meta.Extra["target_path"] != "wiki/ejemplo.md" || doc.Meta.Extra["operation"] != "update" || doc.Meta.Extra["requires_autor_approval"] != "false" {
		t.Fatalf("Extra = %#v", doc.Meta.Extra)
	}

	if len(doc.Links) != 3 {
		t.Fatalf("esperaba 3 enlaces relativos, obtuvo %d: %#v", len(doc.Links), doc.Links)
	}
	if doc.Links[0].Path != "wiki/doc.md" || doc.Links[0].Anchor != "sec-1" || doc.Links[0].Line != 20 {
		t.Fatalf("Links[0] = %+v (esperaba path=wiki/doc.md anchor=sec-1 line=20)", doc.Links[0])
	}
	if doc.Links[1].Path != "../index.md" || doc.Links[1].Anchor != "" || doc.Links[1].Line != 20 {
		t.Fatalf("Links[1] = %+v", doc.Links[1])
	}
	if doc.Links[2].Path != "adr/ADR-0001.md" || doc.Links[2].Line != 29 {
		t.Fatalf("Links[2] = %+v (esperaba path=adr/ADR-0001.md line=29)", doc.Links[2])
	}
}

func TestParseDocument_MalformedFrontmatter(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "sin_cierre",
			content: "---\nid: test\ntitle: sin cierre\n\n# Cuerpo\n",
		},
		{
			name:    "linea_sin_dos_puntos",
			content: "---\nid: test\nlinea invalida sin separador\n---\n# Cuerpo\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDocument("wiki/bad.md", tc.content)
			if !errors.Is(err, ErrMalformedFrontmatter) {
				t.Fatalf("esperaba ErrMalformedFrontmatter, obtuvo %v", err)
			}
		})
	}
}

func TestValidateVault_ValidVault(t *testing.T) {
	root := makeValidVault(t)
	report, err := ValidateVault(root)
	if err != nil {
		t.Fatalf("ValidateVault error inesperado: %v", err)
	}
	if !report.OK || len(report.Issues) != 0 {
		t.Fatalf("esperaba vault valida sin issues, obtuvo OK=%v issues=%#v", report.OK, report.Issues)
	}
	if report.FilesChecked != 7 {
		t.Fatalf("FilesChecked = %d, esperaba 7", report.FilesChecked)
	}
	if report.DocsParsed != 7 {
		t.Fatalf("DocsParsed = %d, esperaba 7", report.DocsParsed)
	}
}

func TestValidateVault_Chunk01BootstrapTemplates(t *testing.T) {
	schemasDir := filepath.Join("..", "schemas")
	mappings := map[string]string{
		"AGENTS.template.md":       "AGENTS.md",
		"index.template.md":        "index.md",
		"constitution.template.md": "constitution.md",
		"risk_zones.template.md":   "wiki/risk_zones.md",
	}

	root := t.TempDir()
	for srcName, dstRel := range mappings {
		raw, err := os.ReadFile(filepath.Join(schemasDir, srcName))
		if err != nil {
			t.Fatalf("leyendo plantilla %s: %v", srcName, err)
		}
		writeTestFile(t, root, dstRel, string(raw))
	}
	writeTestFile(t, root, "log.md", "# Bitacora Operativa\n\n- [2026-11-19] Vault inicializada desde schemas.\n")

	report, err := ValidateVault(root)
	if err != nil {
		t.Fatalf("ValidateVault sobre plantillas Chunk 01 error: %v", err)
	}
	if !report.OK || len(report.Issues) != 0 {
		t.Fatalf("se esperaba vault bootstrap valida sin issues, obtuvo %#v", report.Issues)
	}
}

func TestValidateVault_MissingCoreFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "AGENTS.md", "# Solo agents\n")

	report, err := ValidateVault(root)
	if err != nil {
		t.Fatalf("ValidateVault error: %v", err)
	}
	if report.OK {
		t.Fatalf("esperaba report.OK=false por archivos nucleo faltantes")
	}
	for _, req := range []string{"index.md", "constitution.md", "log.md"} {
		if !hasIssue(report.Issues, IssueMissingCoreFile, req) {
			t.Fatalf("faltaba IssueMissingCoreFile para %s en %#v", req, report.Issues)
		}
	}
}

func TestValidateVault_MissingAndMalformedFrontmatterAndFields(t *testing.T) {
	root := makeValidVault(t)
	writeTestFile(t, root, "wiki/sin_fm.md", "# Sin frontmatter\nContenido.\n")
	writeTestFile(t, root, "wiki/malformado.md", "---\nid: x\ntitle: roto\n")
	writeTestFile(t, root, "wiki/campos_faltantes.md", "---\nproject: black-factory\n---\n# Sin requeridos\n")

	report, err := ValidateVault(root)
	if err != nil {
		t.Fatalf("ValidateVault error: %v", err)
	}
	if report.OK {
		t.Fatalf("esperaba report.OK=false")
	}
	if !hasIssue(report.Issues, IssueMissingFrontmatter, "wiki/sin_fm.md") {
		t.Fatalf("esperaba IssueMissingFrontmatter en wiki/sin_fm.md: %#v", report.Issues)
	}
	if !hasIssue(report.Issues, IssueMalformedFrontmatter, "wiki/malformado.md") {
		t.Fatalf("esperaba IssueMalformedFrontmatter en wiki/malformado.md: %#v", report.Issues)
	}

	missingFieldCount := 0
	for _, iss := range report.Issues {
		if iss.File == "wiki/campos_faltantes.md" && iss.Code == IssueMissingField {
			missingFieldCount++
		}
	}
	if missingFieldCount != 4 {
		t.Fatalf("esperaba 4 IssueMissingField en wiki/campos_faltantes.md, obtuvo %d (%#v)", missingFieldCount, report.Issues)
	}
}

func TestValidateVault_DuplicateID(t *testing.T) {
	root := makeValidVault(t)
	writeTestFile(t, root, "wiki/dup.md", `---
id: ADR-0001-OKF
title: "Documento Duplicado"
type: wiki
project: black-factory
status: draft
tags: []
covers_paths: []
verified_head: ""
updated_at: "2026-11-19"
---

# Duplicado
`)

	report, err := ValidateVault(root)
	if err != nil {
		t.Fatalf("ValidateVault error: %v", err)
	}
	if report.OK {
		t.Fatalf("esperaba report.OK=false por ID duplicado")
	}
	if !hasIssue(report.Issues, IssueDuplicateID, "wiki/dup.md") {
		t.Fatalf("esperaba IssueDuplicateID en wiki/dup.md: %#v", report.Issues)
	}
}

func TestValidateVault_BrokenLinks(t *testing.T) {
	root := makeValidVault(t)
	fence := strings.Repeat("`", 3)
	writeTestFile(t, root, "wiki/enlaces.md", "---\n"+
		"id: wiki-enlaces\n"+
		"title: \"Enlaces\"\n"+
		"type: wiki\n"+
		"project: black-factory\n"+
		"status: active\n"+
		"---\n\n"+
		"Externo permitido [Web](https://example.org/doc).\n"+
		"Valido [Constitucion](../constitution.md#principios).\n"+
		"Roto [NoExiste](no_existe.md#ancla).\n"+
		"Escape [Fuera](../../secreto.md).\n"+
		fence+"markdown\n"+
		"[FencedRoto](tampoco_existe.md)\n"+
		fence+"\n")

	report, err := ValidateVault(root)
	if err != nil {
		t.Fatalf("ValidateVault error: %v", err)
	}
	if report.OK {
		t.Fatalf("esperaba report.OK=false por enlaces rotos")
	}
	if got := countIssueCode(report.Issues, IssueBrokenLink); got != 2 {
		t.Fatalf("esperaba exactamente 2 IssueBrokenLink, obtuvo %d: %#v", got, report.Issues)
	}

	var lines []int
	for _, iss := range report.Issues {
		if iss.Code == IssueBrokenLink && iss.File == "wiki/enlaces.md" {
			lines = append(lines, iss.Line)
		}
	}
	if len(lines) != 2 || lines[0] != 11 || lines[1] != 12 {
		t.Fatalf("lineas inesperadas para enlaces rotos: %v (esperaba [11 12])", lines)
	}
}

func TestValidateVault_G4Detection(t *testing.T) {
	tests := []struct {
		name      string
		targetRel string
		content   string
		secret    string
	}{
		{
			name:      "pem_private_key",
			targetRel: "wiki/g4_pem.md",
			secret:    "-----BEGIN RSA " + "PRIVATE KEY-----",
			content: "---\nid: g4-pem\ntitle: PEM\ntype: wiki\nstatus: draft\n---\n" +
				"-----BEGIN RSA " + "PRIVATE KEY-----\n",
		},
		{
			name:      "literal_private_key",
			targetRel: "wiki/g4_pk.md",
			secret:    "PRIVATE " + "KEY",
			content: "---\nid: g4-pk\ntitle: PK\ntype: wiki\nstatus: draft\n---\n" +
				"No guardar PRIVATE " + "KEY aqui.\n",
		},
		{
			name:      "windows_user_path_backslash",
			targetRel: "wiki/g4_win.md",
			secret:    `C:\` + `Users\demo\docs`,
			content: "---\nid: g4-win\ntitle: Win\ntype: wiki\nstatus: draft\n---\n" +
				"Ruta: C:\\" + "Users\\demo\\docs\\nota.md\n",
		},
		{
			name:      "windows_user_path_slash",
			targetRel: "wiki/g4_winslash.md",
			secret:    "C:/" + "Users/demo/docs",
			content: "---\nid: g4-winslash\ntitle: WinSlash\ntype: wiki\nstatus: draft\n---\n" +
				"Ruta: C:/" + "Users/demo/docs\n",
		},
		{
			name:      "unix_home_path",
			targetRel: "wiki/g4_unix.md",
			secret:    "/home/" + "jdoe/repo",
			content: "---\nid: g4-unix\ntitle: Unix\ntype: wiki\nstatus: draft\n---\n" +
				"Dir: /home/" + "jdoe/repo\n",
		},
		{
			name:      "bearer_and_ghp_token",
			targetRel: "wiki/g4_token.md",
			secret:    "ghp_" + "1234567890abcdefghij123456",
			content: "---\nid: g4-token\ntitle: Token\ntype: wiki\nstatus: draft\n---\n" +
				"Authorization: " + "Bearer " + "eyJhbGciOiJIUzI1NiJ9_demo\n" +
				"Token: " + "ghp_" + "1234567890abcdefghij123456\n",
		},
		{
			name:      "connection_string_password",
			targetRel: "wiki/g4_conn.md",
			secret:    "s3cr3tPass",
			content: "---\nid: g4-conn\ntitle: Conn\ntype: wiki\nstatus: draft\n---\n" +
				"dsn: postgres://admin:" + "s3cr3tPass" + "@db.internal:5432/prod\n" +
				"odbc: Server=db;Database=app;" + "Password=" + "topsecret\n",
		},
		{
			name:      "sensitive_email_in_log_without_frontmatter",
			targetRel: "log.md",
			secret:    "operador" + "@empresa.example",
			content:   "# Bitacora\nContacto: " + "operador" + "@empresa.example\n",
		},
		{
			name:      "g4_inside_malformed_frontmatter",
			targetRel: "wiki/g4_bad_fm.md",
			secret:    "topsecret123",
			content: "---\nid: broken\ntitle: x\n" +
				"api_key=" + "topsecret123\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeValidVault(t)
			writeTestFile(t, root, tc.targetRel, tc.content)

			report, err := ValidateVault(root)
			if err != nil {
				t.Fatalf("ValidateVault error: %v", err)
			}
			if report.OK {
				t.Fatalf("esperaba report.OK=false para %s", tc.name)
			}
			if !hasIssue(report.Issues, IssueG4Violation, tc.targetRel) {
				t.Fatalf("esperaba IssueG4Violation en %s, obtuvo %#v", tc.targetRel, report.Issues)
			}
			for _, iss := range report.Issues {
				if iss.Code == IssueG4Violation && strings.Contains(iss.Message, tc.secret) {
					t.Fatalf("el mensaje G4 transcribio el valor sensible %q: %q", tc.secret, iss.Message)
				}
			}
		})
	}
}
