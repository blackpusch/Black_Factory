package brain

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrAutorApprovalRequired indica que el destino es un documento gobernado
	// (constitution.md o risk_zones.md) y falta aprobacion explicita del [AUTOR].
	ErrAutorApprovalRequired = errors.New("se requiere aprobacion explicita del [AUTOR] (--approve-autor)")
	// ErrProposalValidation indica que una propuesta incumple esquema OKF, enlaces o reglas G4.
	ErrProposalValidation = errors.New("la propuesta no supera validacion OKF/G4")
)

// ProposeRequest define los datos de entrada para crear un BrainPatch en _proposals/.
type ProposeRequest struct {
	TargetPath   string   `json:"target_path"`
	Title        string   `json:"title"`
	Project      string   `json:"project"`
	DocID        string   `json:"doc_id,omitempty"`
	DocType      string   `json:"doc_type,omitempty"`
	Status       string   `json:"status,omitempty"`
	Operation    string   `json:"operation,omitempty"`
	AuthorRole   string   `json:"author_role,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	CoversPaths  []string `json:"covers_paths,omitempty"`
	VerifiedHead string   `json:"verified_head,omitempty"`
	Content      string   `json:"content,omitempty"`
}

// ProposeResult describe la propuesta creada en _proposals/ sin haber mutado la boveda.
type ProposeResult struct {
	ProposalID            string `json:"proposal_id"`
	ProposalPath          string `json:"proposal_path"`
	TargetPath            string `json:"target_path"`
	RequiresAutorApproval bool   `json:"requires_autor_approval"`
}

// ApplyOptions configura la aplicacion controlada de una propuesta BrainPatch.
type ApplyOptions struct {
	ProposalPath string `json:"proposal_path"`
	ApproveAutor bool   `json:"approve_autor"`
}

// ApplyResult resume una propuesta aplicada con exito junto a su entrada en log.md e indexacion.
type ApplyResult struct {
	ProposalID string     `json:"proposal_id"`
	TargetPath string     `json:"target_path"`
	AppliedAt  string     `json:"applied_at"`
	AutorGate  bool       `json:"autor_gate"`
	IndexStats IndexStats `json:"index_stats"`
}

// IsProtectedVaultTarget indica si target exige compuerta explicita del [AUTOR].
func IsProtectedVaultTarget(relTarget string) bool {
	norm := NormalizeRelPath(relTarget)
	base := strings.ToLower(path.Base(norm))
	return strings.EqualFold(norm, "constitution.md") ||
		base == "constitution.md" ||
		strings.EqualFold(norm, "wiki/risk_zones.md") ||
		base == "risk_zones.md"
}

// InitVault inicializa la estructura base de una boveda OKF desde schemasDir sin
// sobrescribir documentos existentes.
func InitVault(vaultRoot string, schemasDir string) error {
	if strings.TrimSpace(vaultRoot) == "" {
		return errors.New("ruta de boveda vacia")
	}
	if strings.TrimSpace(schemasDir) == "" {
		return errors.New("ruta de esquemas vacia")
	}

	absVault, err := filepath.Abs(vaultRoot)
	if err != nil {
		return err
	}
	absSchemas, err := filepath.Abs(schemasDir)
	if err != nil {
		return err
	}
	sInfo, err := os.Stat(absSchemas)
	if err != nil || !sInfo.IsDir() {
		return fmt.Errorf("directorio de esquemas invalido %s", absSchemas)
	}

	for _, sub := range []string{"", "adr", "wiki", "projects", "_proposals", ".index"} {
		dir := filepath.Join(absVault, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creando directorio %s: %w", dir, err)
		}
	}

	today := time.Now().Format("2006-01-02")
	templates := []struct {
		src string
		dst string
	}{
		{"AGENTS.template.md", "AGENTS.md"},
		{"index.template.md", "index.md"},
		{"constitution.template.md", "constitution.md"},
		{"risk_zones.template.md", "wiki/risk_zones.md"},
	}

	for _, tpl := range templates {
		dstAbs := filepath.Join(absVault, filepath.FromSlash(tpl.dst))
		if _, err := os.Stat(dstAbs); err == nil {
			continue // nunca sobrescribir documentos existentes
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		srcAbs := filepath.Join(absSchemas, tpl.src)
		raw, err := os.ReadFile(srcAbs)
		if err != nil {
			return fmt.Errorf("leyendo plantilla %s: %w", srcAbs, err)
		}
		rendered := strings.Replace(string(raw), `updated_at: "YYYY-MM-DD"`, fmt.Sprintf(`updated_at: %q`, today), 1)
		if err := writeFileAtomic(dstAbs, []byte(rendered)); err != nil {
			return fmt.Errorf("escribiendo %s: %w", dstAbs, err)
		}
	}

	logAbs := filepath.Join(absVault, "log.md")
	if _, err := os.Stat(logAbs); errors.Is(err, os.ErrNotExist) {
		initLog := fmt.Sprintf("# Bitácora Operativa de `0_Brain`\n\nRegistro *append-only* de inicialización y cambios aplicados.\n\n- [%s] [INIT] Bóveda inicializada desde esquemas OKF.\n",
			time.Now().Format("2006-01-02 15:04:05"))
		if err := writeFileAtomic(logAbs, []byte(initLog)); err != nil {
			return fmt.Errorf("creando log.md: %w", err)
		}
	} else if err != nil {
		return err
	}

	return nil
}

// ProposeUpdate crea una propuesta estructurada BrainPatch en _proposals/ sin mutar
// los documentos activos de la boveda.
func ProposeUpdate(vaultRoot string, req ProposeRequest) (ProposeResult, error) {
	absVault, err := ensureVaultDir(vaultRoot)
	if err != nil {
		return ProposeResult{}, err
	}

	normTarget, err := validateSafeTargetPath(req.TargetPath)
	if err != nil {
		return ProposeResult{}, err
	}

	targetAbs := filepath.Join(absVault, filepath.FromSlash(normTarget))
	var existingDoc Document
	hasExisting := false
	if _, statErr := os.Stat(targetAbs); statErr == nil {
		if parsed, pErr := ParseFile(absVault, targetAbs); pErr == nil {
			existingDoc = parsed
			hasExisting = true
		}
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		if hasExisting && existingDoc.Meta.Title != "" {
			title = existingDoc.Meta.Title
		} else {
			title = "Actualizacion de " + normTarget
		}
	}

	project := strings.TrimSpace(req.Project)
	if project == "" {
		if hasExisting && existingDoc.Meta.Project != "" {
			project = existingDoc.Meta.Project
		} else {
			project = "black-factory"
		}
	}

	docID := strings.TrimSpace(req.DocID)
	if docID == "" {
		if hasExisting && existingDoc.Meta.ID != "" {
			docID = existingDoc.Meta.ID
		} else {
			docID = defaultDocIDFromTarget(normTarget)
		}
	}

	docType := strings.TrimSpace(req.DocType)
	if docType == "" {
		if hasExisting && existingDoc.Meta.Type != "" {
			docType = existingDoc.Meta.Type
		} else {
			docType = inferDocType(normTarget)
		}
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		if hasExisting && existingDoc.Meta.Status != "" {
			status = existingDoc.Meta.Status
		} else {
			status = "active"
		}
	}

	operation := strings.TrimSpace(req.Operation)
	if operation == "" {
		if hasExisting {
			operation = "update"
		} else {
			operation = "create"
		}
	}

	authorRole := strings.TrimSpace(req.AuthorRole)
	if authorRole == "" {
		authorRole = "0_Brain"
	}

	tags := dedupeTrimmed(req.Tags)
	if len(tags) == 0 && hasExisting {
		tags = dedupeTrimmed(existingDoc.Meta.Tags)
	}
	if len(tags) == 0 {
		tags = []string{docType}
	}

	covers := dedupeCoverPaths(req.CoversPaths)
	if len(covers) == 0 && hasExisting {
		covers = dedupeCoverPaths(existingDoc.Meta.CoversPaths)
	}

	verifiedHead := strings.TrimSpace(req.VerifiedHead)
	if verifiedHead == "" && hasExisting {
		verifiedHead = strings.TrimSpace(existingDoc.Meta.VerifiedHead)
	}

	today := time.Now().Format("2006-01-02")
	targetContent := buildProposedTargetContent(req.Content, Frontmatter{
		ID:           docID,
		Title:        title,
		Type:         docType,
		Project:      project,
		Status:       status,
		Tags:         tags,
		CoversPaths:  covers,
		VerifiedHead: verifiedHead,
		UpdatedAt:    today,
	}, normTarget)

	proposalsDir := filepath.Join(absVault, "_proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		return ProposeResult{}, err
	}

	proposalID, propAbs, err := allocateProposalPath(proposalsDir, normTarget)
	if err != nil {
		return ProposeResult{}, err
	}

	requiresAutor := IsProtectedVaultTarget(normTarget)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = fmt.Sprintf("Propuesta (%s) sobre %s.", operation, normTarget)
	}

	fence := strings.Repeat("`", maxBacktickRun(targetContent)+1)
	if len(fence) < 4 {
		fence = "````"
	}

	var pb strings.Builder
	pb.WriteString(renderFrontmatter(Frontmatter{
		ID:           proposalID,
		Title:        "BrainPatch: " + title,
		Type:         "brain_patch",
		Project:      project,
		Status:       "proposed",
		Tags:         []string{"proposal", "brain-patch"},
		CoversPaths:  covers,
		VerifiedHead: verifiedHead,
		UpdatedAt:    today,
		Extra: map[string]string{
			"target_path":             normTarget,
			"operation":               operation,
			"requires_autor_approval": fmt.Sprintf("%v", requiresAutor),
			"author_role":             authorRole,
		},
	}))
	pb.WriteString("\n# BrainPatch: " + title + "\n\n")
	pb.WriteString("## Resumen del Cambio\n\n")
	pb.WriteString(reason + "\n\n")
	pb.WriteString("## Compuerta de Aprobación `[AUTOR]`\n\n")
	if requiresAutor {
		pb.WriteString("- Documento crítico gobernado: requiere aprobación explícita con `--approve-autor`.\n\n")
	} else {
		pb.WriteString("- Documento estándar: aplicar tras validación OKF y G4 limpia.\n\n")
	}
	pb.WriteString("## Contenido Propuesto\n\n")
	pb.WriteString(fence + "markdown\n")
	pb.WriteString(strings.TrimRight(targetContent, "\r\n"))
	pb.WriteString("\n" + fence + "\n")

	if err := writeFileAtomic(propAbs, []byte(pb.String())); err != nil {
		return ProposeResult{}, err
	}

	return ProposeResult{
		ProposalID:            proposalID,
		ProposalPath:          propAbs,
		TargetPath:            normTarget,
		RequiresAutorApproval: requiresAutor,
	}, nil
}

// ApplyUpdate valida un BrainPatch contra G4 y OKF, exige --approve-autor si afecta
// constitution.md o risk_zones.md, aplica el archivo, escribe en log.md y reindexa.
func ApplyUpdate(vaultRoot string, opts ApplyOptions) (ApplyResult, error) {
	absVault, err := ensureVaultDir(vaultRoot)
	if err != nil {
		return ApplyResult{}, err
	}
	propAbs, err := resolveProposalFile(absVault, opts.ProposalPath)
	if err != nil {
		return ApplyResult{}, err
	}

	propRelRaw, err := filepath.Rel(absVault, propAbs)
	if err != nil {
		return ApplyResult{}, err
	}
	propRel := NormalizeRelPath(propRelRaw)

	rawBytes, err := os.ReadFile(propAbs)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("leyendo propuesta %s: %w", propAbs, err)
	}
	rawStr := string(rawBytes)

	if g4 := ScanG4(propRel, rawStr); len(g4) > 0 {
		return ApplyResult{}, fmt.Errorf("%w: %s (linea %d)", ErrProposalValidation, g4[0].Message, g4[0].Line)
	}

	propDoc, err := ParseDocument(propRel, rawStr)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w: %v", ErrProposalValidation, err)
	}
	if !propDoc.HasFrontmatter {
		return ApplyResult{}, fmt.Errorf("%w: la propuesta carece de frontmatter YAML", ErrProposalValidation)
	}

	rawTarget := propDoc.Meta.Extra["target_path"]
	normTarget, err := validateSafeTargetPath(rawTarget)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w: %v", ErrProposalValidation, err)
	}

	targetContent, err := extractProposedContent(propDoc.Body)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w: %v", ErrProposalValidation, err)
	}

	autorGate := IsProtectedVaultTarget(normTarget) || strings.EqualFold(strings.TrimSpace(propDoc.Meta.Extra["requires_autor_approval"]), "true")
	if autorGate && !opts.ApproveAutor {
		return ApplyResult{}, fmt.Errorf("%w para modificar %s", ErrAutorApprovalRequired, normTarget)
	}

	if g4 := ScanG4(normTarget, targetContent); len(g4) > 0 {
		return ApplyResult{}, fmt.Errorf("%w en contenido destino: %s (linea %d)", ErrProposalValidation, g4[0].Message, g4[0].Line)
	}

	targetDoc, err := ParseDocument(normTarget, targetContent)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("%w en contenido destino: %v", ErrProposalValidation, err)
	}
	if !isOptionalFrontmatterFile(normTarget) || targetDoc.HasFrontmatter {
		if issues := ValidateDocumentMeta(targetDoc); len(issues) > 0 {
			return ApplyResult{}, fmt.Errorf("%w: %s", ErrProposalValidation, issues[0].Message)
		}
	}

	if err := checkUniqueIDForTarget(absVault, normTarget, targetDoc.Meta.ID); err != nil {
		return ApplyResult{}, err
	}

	targetAbs := filepath.Join(absVault, filepath.FromSlash(normTarget))
	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("creando directorio destino: %w", err)
	}
	if err := writeFileAtomic(targetAbs, []byte(targetContent)); err != nil {
		return ApplyResult{}, fmt.Errorf("aplicando cambio en %s: %w", normTarget, err)
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	proposalID := strings.TrimSpace(propDoc.Meta.ID)
	if proposalID == "" {
		proposalID = propRel
	}
	logLine := fmt.Sprintf("- [%s] [APPLY] proposal=%s target=%s autor_approved=%v status=OK\n",
		nowStr, proposalID, normTarget, opts.ApproveAutor)
	if err := appendVaultLog(filepath.Join(absVault, "log.md"), logLine); err != nil {
		return ApplyResult{}, fmt.Errorf("registrando log.md: %w", err)
	}

	idxStats, err := BuildIndex(absVault)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("reindexando boveda tras apply: %w", err)
	}

	return ApplyResult{
		ProposalID: proposalID,
		TargetPath: normTarget,
		AppliedAt:  nowStr,
		AutorGate:  autorGate,
		IndexStats: idxStats,
	}, nil
}

func ensureVaultDir(vaultRoot string) (string, error) {
	if strings.TrimSpace(vaultRoot) == "" {
		return "", errors.New("ruta de boveda vacia")
	}
	absVault, err := filepath.Abs(vaultRoot)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absVault)
	if err != nil {
		return "", fmt.Errorf("accediendo a boveda %s: %w", absVault, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s no es un directorio", absVault)
	}
	return absVault, nil
}

func validateSafeTargetPath(rawTarget string) (string, error) {
	trimmed := strings.TrimSpace(rawTarget)
	if trimmed == "" {
		return "", errors.New("target_path vacio")
	}
	slash := strings.ReplaceAll(trimmed, `\`, "/")
	if strings.HasPrefix(slash, "/") || (len(slash) >= 2 && slash[1] == ':') {
		return "", fmt.Errorf("target_path %q debe ser relativo a la boveda", rawTarget)
	}
	norm := NormalizeRelPath(slash)
	if norm == "" || norm == "." || norm == ".." || strings.HasPrefix(norm, "../") {
		return "", fmt.Errorf("target_path inseguro %q", rawTarget)
	}
	if strings.HasPrefix(norm, "_proposals/") || strings.HasPrefix(norm, ".index/") || norm == "log.md" {
		return "", fmt.Errorf("target_path %q no puede apuntar a _proposals/, .index/ ni log.md", norm)
	}
	return norm, nil
}

func resolveProposalFile(absVault, propPath string) (string, error) {
	trimmed := strings.TrimSpace(propPath)
	if trimmed == "" {
		return "", errors.New("ruta de propuesta vacia")
	}
	var candidates []string
	if filepath.IsAbs(trimmed) {
		candidates = append(candidates, trimmed)
	} else {
		candidates = append(candidates,
			filepath.Join(absVault, filepath.FromSlash(trimmed)),
			filepath.Join(absVault, "_proposals", filepath.FromSlash(trimmed)),
		)
	}
	for _, cand := range candidates {
		absCand, err := filepath.Abs(cand)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(absCand); err == nil && !fi.IsDir() {
			return absCand, nil
		}
	}
	return "", fmt.Errorf("propuesta no encontrada: %s", propPath)
}

func extractProposedContent(body string) (string, error) {
	lines := strings.Split(normalizeMarkdownContent(body), "\n")
	sectionStart := 0
	for i, ln := range lines {
		if strings.EqualFold(strings.TrimSpace(ln), "## Contenido Propuesto") {
			sectionStart = i + 1
			break
		}
	}

	var fenceChar byte
	fenceLen := 0
	contentStart := -1

	for i := sectionStart; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if fenceLen == 0 {
			if ch, run, ok := parseFenceOpener(trimmed); ok {
				fenceChar, fenceLen = ch, run
				contentStart = i + 1
			}
			continue
		}
		if isFenceCloser(trimmed, fenceChar, fenceLen) {
			extracted := strings.Join(lines[contentStart:i], "\n")
			if strings.TrimSpace(extracted) == "" {
				return "", errors.New("el bloque de contenido propuesto esta vacio")
			}
			return extracted + "\n", nil
		}
	}

	return "", errors.New("no se encontro bloque fenced con contenido bajo '## Contenido Propuesto'")
}

func parseFenceOpener(trimmed string) (byte, int, bool) {
	if len(trimmed) < 3 {
		return 0, 0, false
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	run := 0
	for run < len(trimmed) && trimmed[run] == ch {
		run++
	}
	if run < 3 {
		return 0, 0, false
	}
	return ch, run, true
}

func isFenceCloser(trimmed string, ch byte, minLen int) bool {
	if len(trimmed) < minLen {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != ch {
			return false
		}
	}
	return true
}

func checkUniqueIDForTarget(absVault, normTarget, newID string) error {
	idKey := strings.ToLower(strings.TrimSpace(newID))
	if idKey == "" {
		return nil
	}
	files, err := ListVaultMarkdownFiles(absVault)
	if err != nil {
		return err
	}
	for _, absFile := range files {
		doc, err := ParseFile(absVault, absFile)
		if err != nil {
			continue
		}
		if strings.HasPrefix(doc.RelPath, "_proposals/") || strings.EqualFold(doc.RelPath, normTarget) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(doc.Meta.ID), idKey) {
			return fmt.Errorf("%w: id %q ya existe en %s", ErrProposalValidation, newID, doc.RelPath)
		}
	}
	return nil
}

func buildProposedTargetContent(rawContent string, fm Frontmatter, normTarget string) string {
	norm := strings.TrimPrefix(normalizeMarkdownContent(rawContent), "\n")
	if strings.HasPrefix(strings.TrimSpace(norm), "---") {
		if !strings.HasSuffix(norm, "\n") {
			norm += "\n"
		}
		return norm
	}

	body := strings.TrimSpace(norm)
	if body == "" {
		body = fmt.Sprintf("# %s\n\nDocumento actualizado en `%s`.\n", fm.Title, normTarget)
	}
	return renderFrontmatter(fm) + "\n" + body + "\n"
}

func renderFrontmatter(fm Frontmatter) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + fm.ID + "\n")
	b.WriteString(fmt.Sprintf("title: %q\n", fm.Title))
	b.WriteString("type: " + fm.Type + "\n")
	b.WriteString("project: " + fm.Project + "\n")
	b.WriteString("status: " + fm.Status + "\n")
	writeYAMLList(&b, "tags", fm.Tags)
	writeYAMLList(&b, "covers_paths", fm.CoversPaths)
	b.WriteString(fmt.Sprintf("verified_head: %q\n", fm.VerifiedHead))
	b.WriteString(fmt.Sprintf("updated_at: %q\n", fm.UpdatedAt))

	for _, k := range []string{"target_path", "operation", "requires_autor_approval", "author_role"} {
		if v, ok := fm.Extra[k]; ok {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}
	b.WriteString("---\n")
	return b.String()
}

func writeYAMLList(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		b.WriteString(key + ": []\n")
		return
	}
	b.WriteString(key + ":\n")
	for _, it := range items {
		b.WriteString("  - " + it + "\n")
	}
}

func allocateProposalPath(proposalsDir, normTarget string) (string, string, error) {
	baseSlug := slugify(strings.TrimSuffix(normTarget, path.Ext(normTarget)))
	stamp := time.Now().Format("20060102-150405")
	for n := 0; n < 100; n++ {
		id := fmt.Sprintf("patch-%s-%s", stamp, baseSlug)
		if n > 0 {
			id = fmt.Sprintf("%s-%d", id, n+1)
		}
		cand := filepath.Join(proposalsDir, id+".md")
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return id, cand, nil
		}
	}
	return "", "", errors.New("no se pudo asignar nombre unico de propuesta en _proposals/")
}

func defaultDocIDFromTarget(normTarget string) string {
	noExt := strings.TrimSuffix(normTarget, path.Ext(normTarget))
	return slugify(noExt)
}

func inferDocType(normTarget string) string {
	lower := strings.ToLower(normTarget)
	switch {
	case lower == "constitution.md":
		return "constitution"
	case lower == "wiki/risk_zones.md" || lower == "risk_zones.md":
		return "risk_zones"
	case lower == "index.md":
		return "index"
	case lower == "agents.md":
		return "agents"
	case strings.HasPrefix(lower, "adr/"):
		return "adr"
	default:
		return "wiki"
	}
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "doc"
	}
	return out
}

func maxBacktickRun(s string) int {
	maxRun, cur := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			cur++
			if cur > maxRun {
				maxRun = cur
			}
		} else {
			cur = 0
		}
	}
	return maxRun
}

func writeFileAtomic(dstPath string, data []byte) error {
	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".brain-write-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFileAtomic(tmpPath, dstPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func appendVaultLog(logPath, line string) error {
	needsNewline := false
	if raw, err := os.ReadFile(logPath); err == nil && len(raw) > 0 && raw[len(raw)-1] != '\n' {
		needsNewline = true
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if needsNewline {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(line)
	return err
}
