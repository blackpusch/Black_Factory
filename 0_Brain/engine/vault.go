package brain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrAutorApprovalRequired indica que el destino es un documento gobernado
	// (constitution.md o risk_zones.md o transversal) y falta firma valida del [AUTOR] (Exit code 2).
	ErrAutorApprovalRequired = errors.New("se requiere firma Ed25519 valida del [AUTOR] (--autor-sig)")
	// ErrProposalValidation indica que una propuesta incumple esquema OKF, enlaces o reglas G4 (Exit code 1).
	ErrProposalValidation = errors.New("la propuesta no supera validacion OKF/G4")
	// ErrUnsafeTargetPath indica que target_path no cumple con la allowlist o contiene caracteres prohibidos (Exit code 1).
	ErrUnsafeTargetPath = errors.New("target_path no permitido o fuera de la allowlist")
	// ErrJournalCorrupt indica que el archivo de journal de la boveda esta truncado o corrupto (Exit code 3).
	ErrJournalCorrupt = errors.New("el journal .brain-journal.json esta truncado o corrupto (se requiere intervencion manual)")
	// ErrCommitDegraded indica que el estado de la boveda no coincide ni con base ni con destino durante la recuperacion (Exit code 3).
	ErrCommitDegraded = errors.New("el commit esta en estado degradado tras fallo (destino no coincide con base ni con contenido)")
	// ErrProposalAlreadyApplied indica que la propuesta ya fue aplicada previamente (Exit code 1).
	ErrProposalAlreadyApplied = errors.New("la propuesta ya fue aplicada previamente en la boveda")
	// ErrNoOpUpdate indica que el contenido propuesto es identico al existente (Exit code 1).
	ErrNoOpUpdate = errors.New("la propuesta es no-op (el contenido propuesto es identico a la base actual)")
	// ErrBaseMismatch indica que el archivo destino fue modificado concurrentemente y no coincide con base_sha256 (Exit code 1).
	ErrBaseMismatch = errors.New("discrepancia en base_sha256: el destino no coincide con la base esperada")
)

var (
	reValidWikiSlug         = regexp.MustCompile(`^[a-z0-9_-]+$`)
	reValidADRSlug          = regexp.MustCompile(`^ADR-[0-9]{4}-[a-z0-9_-]+$`)
	reValidProjectSlug      = regexp.MustCompile(`^[a-z0-9_-]+$`)
	reValidProposalFilename = regexp.MustCompile(`^patch-([0-9]{8}-[0-9]{6}-)?[a-z0-9_-]+\.md$`)
)

func isDOSDeviceName(name string) bool {
	switch strings.ToLower(name) {
	case "con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return true
	default:
		return false
	}
}

func isAllowedCanonicalTarget(norm string) bool {
	switch norm {
	case "AGENTS.md", "index.md", "constitution.md", "wiki/risk_zones.md":
		return true
	}

	if strings.HasPrefix(norm, "wiki/") {
		slug := strings.TrimPrefix(norm, "wiki/")
		if !strings.HasSuffix(slug, ".md") || strings.Contains(slug, "/") {
			return false
		}
		name := strings.TrimSuffix(slug, ".md")
		return reValidWikiSlug.MatchString(name) && !isDOSDeviceName(name)
	}

	if strings.HasPrefix(norm, "adr/") {
		slug := strings.TrimPrefix(norm, "adr/")
		if !strings.HasSuffix(slug, ".md") || strings.Contains(slug, "/") {
			return false
		}
		name := strings.TrimSuffix(slug, ".md")
		return reValidADRSlug.MatchString(name) && !isDOSDeviceName(name)
	}

	if strings.HasPrefix(norm, "projects/") {
		slug := strings.TrimPrefix(norm, "projects/")
		if !strings.HasSuffix(slug, ".md") || strings.Contains(slug, "/") {
			return false
		}
		name := strings.TrimSuffix(slug, ".md")
		return reValidProjectSlug.MatchString(name) && !isDOSDeviceName(name)
	}

	return false
}

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
	ProposalPath string           `json:"proposal_path"`
	AutorSig     string           `json:"autor_sig,omitempty"`
	ApproveAutor bool             `json:"approve_autor,omitempty"` // deprecado: mantenido para compatibilidad de pruebas previas
	NowFunc      func() time.Time `json:"-"`
}

// JournalRecord registra la intencion de mutacion atomica antes de modificar el destino.
type JournalRecord struct {
	ProposalID    string `json:"proposal_id"`
	TargetPath    string `json:"target_path"`
	Operation     string `json:"operation"`
	BaseSHA256    string `json:"base_sha256"`
	ContentSHA256 string `json:"content_sha256"`
	SigSHA256     string `json:"sig_sha256,omitempty"`
	Timestamp     string `json:"timestamp"`
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

func isGovernedDoc(normTarget, docType string) bool {
	if IsProtectedVaultTarget(normTarget) {
		return true
	}
	return IsTransversalType(docType)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
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

// ApplyUpdate valida un BrainPatch contra G4 y OKF, exige firma Ed25519 del [AUTOR]
// si afecta documentos gobernados, aplica el cambio de forma atomica con Journal y reindexa.
func ApplyUpdate(vaultRoot string, opts ApplyOptions) (ApplyResult, error) {
	absVault, err := ensureVaultDir(vaultRoot)
	if err != nil {
		return ApplyResult{}, err
	}

	// 1. Bloqueo exclusivo a nivel de kernel del SO (J1)
	lock, lockErr := AcquireVaultLock(absVault)
	if lockErr != nil {
		return ApplyResult{}, lockErr
	}
	defer lock.Unlock()

	// 2. Recuperacion automatica de journal previo si existe (J2)
	if err := recoverJournal(absVault); err != nil {
		return ApplyResult{}, err
	}

	// 3. Resolucion y validacion de contencion fisica de propuesta
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
	normTargetContent := normalizeMarkdownContent(targetContent)
	contentSHA256 := sha256Hex(normTargetContent)

	// 4. Verificacion previa de idempotencia contra log.md
	proposalID := strings.TrimSpace(propDoc.Meta.ID)
	if proposalID == "" {
		proposalID = propRel
	}
	logged, err := isProposalLogged(absVault, proposalID, "", contentSHA256)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("verificando log de boveda: %w", err)
	}
	if logged {
		return ApplyResult{}, fmt.Errorf("%w: propuesta %s con hash %s", ErrProposalAlreadyApplied, proposalID, contentSHA256)
	}

	// 5. Determinacion estricta de operacion y base_sha256
	targetAbs := filepath.Join(absVault, filepath.FromSlash(normTarget))
	operation := strings.ToLower(strings.TrimSpace(propDoc.Meta.Extra["operation"]))
	if operation == "" {
		if _, statErr := os.Stat(targetAbs); statErr == nil {
			operation = "update"
		} else {
			operation = "create"
		}
	}

	var baseSHA256 string
	targetExists := false
	if rawTargetBytes, statErr := os.ReadFile(targetAbs); statErr == nil {
		targetExists = true
		normBaseContent := normalizeMarkdownContent(string(rawTargetBytes))
		baseSHA256 = sha256Hex(normBaseContent)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return ApplyResult{}, fmt.Errorf("inspeccionando destino %s: %w", normTarget, statErr)
	}

	if operation == "create" {
		if targetExists {
			return ApplyResult{}, fmt.Errorf("%w: el destino %s ya existe para operacion create", ErrProposalValidation, normTarget)
		}
		baseSHA256 = ""
	} else if operation == "update" {
		if !targetExists {
			return ApplyResult{}, fmt.Errorf("%w: el destino %s no existe para operacion update", ErrProposalValidation, normTarget)
		}
		if contentSHA256 == baseSHA256 {
			return ApplyResult{}, fmt.Errorf("%w sobre %s", ErrNoOpUpdate, normTarget)
		}
	} else {
		return ApplyResult{}, fmt.Errorf("%w: operacion no soportada %q", ErrProposalValidation, operation)
	}

	// 6. Validacion de contenido destino OKF y G4
	if g4 := ScanG4(normTarget, normTargetContent); len(g4) > 0 {
		return ApplyResult{}, fmt.Errorf("%w en contenido destino: %s (linea %d)", ErrProposalValidation, g4[0].Message, g4[0].Line)
	}

	targetDoc, err := ParseDocument(normTarget, normTargetContent)
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

	// 7. Compuerta [AUTOR] criptografica
	autorGate := isGovernedDoc(normTarget, targetDoc.Meta.Type) || strings.EqualFold(strings.TrimSpace(propDoc.Meta.Extra["requires_autor_approval"]), "true")
	var sigSHA256 string
	if autorGate {
		if strings.TrimSpace(opts.AutorSig) != "" {
			expiresAt := propDoc.Meta.Extra["expires_at"]
			if expiresAt == "" {
				now := time.Now().UTC()
				if opts.NowFunc != nil {
					now = opts.NowFunc().UTC()
				}
				expiresAt = now.Add(12 * time.Hour).Format(time.RFC3339)
			}
			if err := verifyAutorSignature(absVault, normTarget, operation, baseSHA256, contentSHA256, opts.AutorSig, expiresAt, opts.NowFunc); err != nil {
				return ApplyResult{}, err
			}
			sigSHA256 = sha256Hex(opts.AutorSig)
		} else if opts.ApproveAutor {
			// Si existe autor.pub, la firma es obligatoria incondicionalmente
			pubPath := filepath.Join(absVault, ".autor", "autor.pub")
			if _, statErr := os.Stat(pubPath); statErr == nil {
				return ApplyResult{}, fmt.Errorf("%w: clave publica del autor configurada, se requiere firma Ed25519 (--autor-sig)", ErrAutorApprovalRequired)
			}
			// En ausencia de autor.pub (ej. fixtures mock de pruebas previas), se permite approve-autor
		} else {
			return ApplyResult{}, fmt.Errorf("%w para modificar %s", ErrAutorApprovalRequired, normTarget)
		}
	}

	// 8. Escribir Journal de forma atomica con Sync y Rename
	timestamp := time.Now().UTC().Format(time.RFC3339)
	rec := JournalRecord{
		ProposalID:    proposalID,
		TargetPath:    normTarget,
		Operation:     operation,
		BaseSHA256:    baseSHA256,
		ContentSHA256: contentSHA256,
		SigSHA256:     sigSHA256,
		Timestamp:     timestamp,
	}
	recBytes, err := json.Marshal(rec)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("serializando journal: %w", err)
	}

	journalTmp := filepath.Join(absVault, fmt.Sprintf(".brain-journal-%d.tmp", os.Getpid()))
	if err := writeFileSync(journalTmp, recBytes); err != nil {
		return ApplyResult{}, fmt.Errorf("escribiendo journal temporal: %w", err)
	}
	journalFinal := filepath.Join(absVault, ".brain-journal.json")
	if err := os.Rename(journalTmp, journalFinal); err != nil {
		_ = os.Remove(journalTmp)
		return ApplyResult{}, fmt.Errorf("promoviendo journal: %w", err)
	}

	// 9. Verificacion estricta a prueba de fallos (Fail-Closed) de ruta destino y ancestros (F1 y F3)
	realVault, err := filepath.EvalSymlinks(absVault)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("resolviendo boveda %s: %w", absVault, err)
	}
	realVaultClean := filepath.Clean(realVault)

	targetDirRel := path.Dir(normTarget)
	var expectedParentDir string
	if targetDirRel == "." || targetDirRel == "" {
		expectedParentDir = realVaultClean
	} else {
		expectedParentDir = filepath.Clean(filepath.Join(realVaultClean, filepath.FromSlash(targetDirRel)))
	}

	fiTarget, lstatErr := os.Lstat(targetAbs)
	if lstatErr == nil {
		if isReparseOrSymlink(fiTarget) {
			return ApplyResult{}, fmt.Errorf("el archivo destino %s es un enlace simbolico o junction no permitido", normTarget)
		}
	} else if !errors.Is(lstatErr, os.ErrNotExist) {
		return ApplyResult{}, fmt.Errorf("error inspeccionando archivo destino %s: %w", normTarget, lstatErr)
	}

	parentDir := filepath.Dir(targetAbs)
	pInfo, pErr := os.Lstat(parentDir)
	if pErr == nil {
		if isReparseOrSymlink(pInfo) {
			return ApplyResult{}, fmt.Errorf("directorio ancestro de %s es un symlink/junction no permitido", normTarget)
		}
		realParent, evalErr := filepath.EvalSymlinks(parentDir)
		if evalErr != nil {
			return ApplyResult{}, fmt.Errorf("error resolviendo enlaces de ancestro %s: %w", parentDir, evalErr)
		}
		if !strings.EqualFold(filepath.Clean(realParent), expectedParentDir) {
			return ApplyResult{}, fmt.Errorf("directorio destino no coincide exactamente con la ruta canónica esperada %s (resuelve a %s)", expectedParentDir, realParent)
		}
	} else if errors.Is(pErr, os.ErrNotExist) {
		// Ancestor no existe todavia: resolver el ancestro existente mas cercano
		curr := parentDir
		for {
			parentOfCurr := filepath.Dir(curr)
			if parentOfCurr == curr {
				break
			}
			curr = parentOfCurr
			cInfo, cErr := os.Lstat(curr)
			if cErr == nil {
				if isReparseOrSymlink(cInfo) {
					return ApplyResult{}, fmt.Errorf("ancestro existente %s es un symlink/junction no permitido", curr)
				}
				realCurr, evalErr := filepath.EvalSymlinks(curr)
				if evalErr != nil {
					return ApplyResult{}, fmt.Errorf("error resolviendo enlaces de ancestro %s: %w", curr, evalErr)
				}
				relToVault, relErr := filepath.Rel(absVault, curr)
				if relErr != nil {
					return ApplyResult{}, relErr
				}
				expectedCurr := filepath.Clean(filepath.Join(realVaultClean, relToVault))
				if !strings.EqualFold(filepath.Clean(realCurr), expectedCurr) {
					return ApplyResult{}, fmt.Errorf("ancestro %s no coincide con ruta canonica esperada %s", curr, expectedCurr)
				}
				break
			} else if !errors.Is(cErr, os.ErrNotExist) {
				return ApplyResult{}, fmt.Errorf("error inspeccionando ancestro %s: %w", curr, cErr)
			}
		}

		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("creando directorio destino: %w", err)
		}

		// F1: Repetir comprobacion tras MkdirAll
		pInfoPost, pErrPost := os.Lstat(parentDir)
		if pErrPost != nil {
			return ApplyResult{}, fmt.Errorf("error verificando directorio creado %s: %w", parentDir, pErrPost)
		}
		if isReparseOrSymlink(pInfoPost) {
			return ApplyResult{}, fmt.Errorf("directorio creado %s es un symlink/junction no permitido", parentDir)
		}
		realParentPost, evalErrPost := filepath.EvalSymlinks(parentDir)
		if evalErrPost != nil {
			return ApplyResult{}, fmt.Errorf("error resolviendo enlaces de directorio creado %s: %w", parentDir, evalErrPost)
		}
		if !strings.EqualFold(filepath.Clean(realParentPost), expectedParentDir) {
			return ApplyResult{}, fmt.Errorf("directorio creado no coincide con la ruta canonica esperada %s", expectedParentDir)
		}
	} else {
		// F1: Cualquier otro error aborta
		return ApplyResult{}, fmt.Errorf("error inspeccionando directorio destino %s: %w", parentDir, pErr)
	}

	// 10. Escribir contenido destino
	if err := writeFileAtomic(targetAbs, []byte(normTargetContent)); err != nil {
		return ApplyResult{}, fmt.Errorf("aplicando cambio en %s: %w", normTarget, err)
	}

	// 11. Actualizar estado de la propuesta a applied
	_ = markProposalApplied(propAbs, timestamp)

	// 12. Asentar bitacora en log.md
	approval := "standard"
	if sigSHA256 != "" {
		approval = "sig"
	} else if opts.ApproveAutor {
		approval = "autor-flag"
	}
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("- [%s] [APPLY] proposal=%s target=%s base=%s content=%s approval=%s status=OK\n",
		nowStr, proposalID, normTarget, baseSHA256, contentSHA256, approval)
	if err := appendVaultLog(filepath.Join(absVault, "log.md"), logLine); err != nil {
		return ApplyResult{}, fmt.Errorf("registrando log.md: %w", err)
	}

	// 13. Reindexar SQLite FTS5
	idxStats, err := BuildIndex(absVault)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("reindexando boveda tras apply: %w", err)
	}

	// 14. Eliminar Journal completado con exito
	_ = os.Remove(journalFinal)

	return ApplyResult{
		ProposalID: proposalID,
		TargetPath: normTarget,
		AppliedAt:  nowStr,
		AutorGate:  autorGate,
		IndexStats: idxStats,
	}, nil
}

// RecoverJournal recupera de forma segura el estado de la boveda tras una interrupcion previa.
// Adquiere el lock del SO antes de inspeccionar o recuperar (J2).
func RecoverJournal(vaultRoot string) error {
	absVault, err := ensureVaultDir(vaultRoot)
	if err != nil {
		return err
	}
	return recoverJournal(absVault)
}

func recoverJournal(absVault string) error {
	journalPath := filepath.Join(absVault, ".brain-journal.json")
	raw, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: leyendo journal: %v", ErrJournalCorrupt, err)
	}
	var rec JournalRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return fmt.Errorf("%w: decodificando json: %v", ErrJournalCorrupt, err)
	}

	targetAbs := filepath.Join(absVault, filepath.FromSlash(rec.TargetPath))
	var currentHash string
	if targetBytes, err := os.ReadFile(targetAbs); err == nil {
		normCurrent := normalizeMarkdownContent(string(targetBytes))
		currentHash = sha256Hex(normCurrent)
	}

	if currentHash == rec.ContentSHA256 {
		// Forward recovery: el destino se escribio completamente
		_, _ = BuildIndex(absVault)
		propAbs := filepath.Join(absVault, "_proposals", rec.ProposalID+".md")
		if _, statErr := os.Stat(propAbs); statErr == nil {
			_ = markProposalApplied(propAbs, rec.Timestamp)
		}
		logged, _ := isProposalLogged(absVault, rec.ProposalID, rec.BaseSHA256, rec.ContentSHA256)
		if !logged {
			approval := "standard"
			if rec.SigSHA256 != "" {
				approval = "sig"
			}
			logLine := fmt.Sprintf("- [%s] [RECOVER_FORWARD] proposal=%s target=%s base=%s content=%s approval=%s status=OK\n",
				rec.Timestamp, rec.ProposalID, rec.TargetPath, rec.BaseSHA256, rec.ContentSHA256, approval)
			_ = appendVaultLog(filepath.Join(absVault, "log.md"), logLine)
		}
		_ = os.Remove(journalPath)
		return nil
	}

	if currentHash == rec.BaseSHA256 || (currentHash == "" && rec.BaseSHA256 == "") {
		// Rollback recovery: el destino conserva el estado base previo
		_ = os.Remove(journalPath)
		return nil
	}

	return fmt.Errorf("%w: destino %s tiene hash %s, se esperaba base %s o contenido %s",
		ErrCommitDegraded, rec.TargetPath, currentHash, rec.BaseSHA256, rec.ContentSHA256)
}

func isProposalLogged(absVault, proposalID, baseSHA, contentSHA string) (bool, error) {
	logPath := filepath.Join(absVault, "log.md")
	raw, err := os.ReadFile(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(raw), "\n")
	propKey := "proposal=" + proposalID
	contentKey := "content=" + contentSHA
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.Contains(trimmed, propKey) && strings.Contains(trimmed, contentKey) {
			return true, nil
		}
	}
	return false, nil
}

func markProposalApplied(propAbs, appliedAt string) error {
	rawBytes, err := os.ReadFile(propAbs)
	if err != nil {
		return err
	}
	doc, err := ParseDocument(path.Base(propAbs), string(rawBytes))
	if err != nil {
		return err
	}
	doc.Meta.Status = "applied"
	if doc.Meta.Extra == nil {
		doc.Meta.Extra = make(map[string]string)
	}
	doc.Meta.Extra["applied_at"] = appliedAt
	renderedFM := renderFrontmatter(doc.Meta)
	newContent := renderedFM + "\n" + doc.Body
	return writeFileAtomic(propAbs, []byte(newContent))
}

func writeFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func verifyAutorSignature(absVault, normTarget, operation, baseSHA, contentSHA, sigB64, expiresAt string, nowFunc func() time.Time) error {
	pubPath := filepath.Join(absVault, ".autor", "autor.pub")
	rawPub, err := os.ReadFile(pubPath)
	if err != nil {
		return fmt.Errorf("%w: clave publica del autor no encontrada en %s (%v)", ErrAutorApprovalRequired, pubPath, err)
	}
	pubBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawPub)))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: clave publica invalida en %s (debe ser base64 de 32 bytes)", ErrAutorApprovalRequired, pubPath)
	}

	trimmedExp := strings.TrimSpace(expiresAt)
	if trimmedExp == "" {
		return fmt.Errorf("%w: expires_at ausente en propuesta gobernada", ErrAutorApprovalRequired)
	}
	expTime, err := time.Parse(time.RFC3339, trimmedExp)
	if err != nil {
		return fmt.Errorf("%w: expires_at %q no tiene formato RFC3339 valido", ErrAutorApprovalRequired, trimmedExp)
	}

	now := time.Now().UTC()
	if nowFunc != nil {
		now = nowFunc().UTC()
	}

	if expTime.Before(now) {
		return fmt.Errorf("%w: firma Ed25519 expirada (%s antes de %s)", ErrAutorApprovalRequired, expTime.Format(time.RFC3339), now.Format(time.RFC3339))
	}
	if expTime.After(now.Add(24 * time.Hour)) {
		return fmt.Errorf("%w: expires_at excede el limite maximo permitido de 24 horas", ErrAutorApprovalRequired)
	}

	trimmedSig := strings.TrimSpace(sigB64)
	if trimmedSig == "" {
		return fmt.Errorf("%w: firma Ed25519 ausente (--autor-sig)", ErrAutorApprovalRequired)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(trimmedSig)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("%w: firma Ed25519 invalida (debe ser base64 de 64 bytes)", ErrAutorApprovalRequired)
	}

	payload := fmt.Sprintf("brain-autor-v1\n"+
		"target_path:%s\n"+
		"operation:%s\n"+
		"base_sha256:%s\n"+
		"content_sha256:%s\n"+
		"expires_at:%s\n",
		normTarget,
		operation,
		baseSHA,
		contentSHA,
		trimmedExp,
	)

	if !ed25519.Verify(pubBytes, []byte(payload), sigBytes) {
		return fmt.Errorf("%w: la firma Ed25519 no corresponde a los parametros de la propuesta", ErrAutorApprovalRequired)
	}
	return nil
}

// SignProposal genera la firma Ed25519 canonica en Base64 para una propuesta.
func SignProposal(vaultRoot, propPath, privKeyPath string, nowFunc func() time.Time) (string, error) {
	absVault, err := ensureVaultDir(vaultRoot)
	if err != nil {
		return "", err
	}
	propAbs, err := resolveProposalFile(absVault, propPath)
	if err != nil {
		return "", err
	}
	rawBytes, err := os.ReadFile(propAbs)
	if err != nil {
		return "", err
	}
	propDoc, err := ParseDocument(path.Base(propAbs), string(rawBytes))
	if err != nil {
		return "", err
	}
	rawTarget := propDoc.Meta.Extra["target_path"]
	normTarget, err := validateSafeTargetPath(rawTarget)
	if err != nil {
		return "", err
	}
	targetContent, err := extractProposedContent(propDoc.Body)
	if err != nil {
		return "", err
	}
	normTargetContent := normalizeMarkdownContent(targetContent)
	contentSHA256 := sha256Hex(normTargetContent)

	targetAbs := filepath.Join(absVault, filepath.FromSlash(normTarget))
	operation := strings.ToLower(strings.TrimSpace(propDoc.Meta.Extra["operation"]))
	if operation == "" {
		if _, statErr := os.Stat(targetAbs); statErr == nil {
			operation = "update"
		} else {
			operation = "create"
		}
	}

	var baseSHA256 string
	if operation == "update" {
		rawExisting, err := os.ReadFile(targetAbs)
		if err != nil {
			return "", fmt.Errorf("leyendo destino para update: %w", err)
		}
		baseSHA256 = sha256Hex(normalizeMarkdownContent(string(rawExisting)))
	}

	expiresAt := propDoc.Meta.Extra["expires_at"]
	if expiresAt == "" {
		now := time.Now().UTC()
		if nowFunc != nil {
			now = nowFunc().UTC()
		}
		expiresAt = now.Add(12 * time.Hour).Format(time.RFC3339)
		if propDoc.Meta.Extra == nil {
			propDoc.Meta.Extra = make(map[string]string)
		}
		propDoc.Meta.Extra["expires_at"] = expiresAt
		renderedFM := renderFrontmatter(propDoc.Meta)
		_ = writeFileAtomic(propAbs, []byte(renderedFM+"\n"+propDoc.Body))
	}

	keyRaw, err := os.ReadFile(privKeyPath)
	if err != nil {
		return "", fmt.Errorf("leyendo clave privada %s: %w", privKeyPath, err)
	}
	keyStr := strings.TrimSpace(string(keyRaw))

	var privKey ed25519.PrivateKey
	if hexBytes, hErr := hex.DecodeString(keyStr); hErr == nil {
		if len(hexBytes) == ed25519.SeedSize {
			privKey = ed25519.NewKeyFromSeed(hexBytes)
		} else if len(hexBytes) == ed25519.PrivateKeySize {
			privKey = ed25519.PrivateKey(hexBytes)
		}
	}
	if privKey == nil {
		if b64Bytes, bErr := base64.StdEncoding.DecodeString(keyStr); bErr == nil {
			if len(b64Bytes) == ed25519.SeedSize {
				privKey = ed25519.NewKeyFromSeed(b64Bytes)
			} else if len(b64Bytes) == ed25519.PrivateKeySize {
				privKey = ed25519.PrivateKey(b64Bytes)
			}
		}
	}
	if privKey == nil && len(keyRaw) == ed25519.PrivateKeySize {
		privKey = ed25519.PrivateKey(keyRaw)
	}
	if privKey == nil && len(keyRaw) == ed25519.SeedSize {
		privKey = ed25519.NewKeyFromSeed(keyRaw)
	}
	if privKey == nil {
		return "", errors.New("formato de clave privada no reconocido (debe ser seed de 32 bytes o privkey de 64 bytes en hex, base64 o binario)")
	}

	payload := fmt.Sprintf("brain-autor-v1\n"+
		"target_path:%s\n"+
		"operation:%s\n"+
		"base_sha256:%s\n"+
		"content_sha256:%s\n"+
		"expires_at:%s\n",
		normTarget,
		operation,
		baseSHA256,
		contentSHA256,
		expiresAt,
	)

	sig := ed25519.Sign(privKey, []byte(payload))
	return base64.StdEncoding.EncodeToString(sig), nil
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

	// 1. Prohibir caracteres especiales, ADS (:), short names (~), control characters
	if strings.ContainsAny(trimmed, ":~*?\"<>|\x00") {
		return "", fmt.Errorf("%w: %q contiene caracteres prohibidos (: ~ * ? \" < > |)", ErrUnsafeTargetPath, rawTarget)
	}

	// 2. Rechazar rutas absolutas o letras de unidad
	slash := strings.ReplaceAll(trimmed, `\`, "/")
	if strings.HasPrefix(slash, "/") || (len(slash) >= 2 && slash[1] == ':') {
		return "", fmt.Errorf("%w: %q debe ser relativo a la boveda", ErrUnsafeTargetPath, rawTarget)
	}

	// 3. Normalizar segmentos relativos
	norm := NormalizeRelPath(slash)
	if norm == "" || norm == "." || norm == ".." || strings.HasPrefix(norm, "../") {
		return "", fmt.Errorf("%w: ruta relativa insegura %q", ErrUnsafeTargetPath, rawTarget)
	}

	// 4. Exigir extension .md estricta
	if !strings.HasSuffix(norm, ".md") {
		return "", fmt.Errorf("%w: %q debe tener extension .md", ErrUnsafeTargetPath, rawTarget)
	}

	// 5. Validar cada segmento individualmente contra espacios y puntos en extremos
	segments := strings.Split(norm, "/")
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("%w: segmento vacio o invalido en %q", ErrUnsafeTargetPath, rawTarget)
		}
		if strings.HasPrefix(seg, " ") || strings.HasSuffix(seg, " ") ||
			strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".") {
			return "", fmt.Errorf("%w: segmento %q con espacios o puntos en extremos en %q", ErrUnsafeTargetPath, seg, rawTarget)
		}
	}

	// 6. Verificar nombres de dispositivo DOS reservados en el nombre base
	baseNoExt := strings.TrimSuffix(path.Base(norm), ".md")
	if isDOSDeviceName(baseNoExt) {
		return "", fmt.Errorf("%w: %q utiliza un nombre de dispositivo DOS reservado", ErrUnsafeTargetPath, rawTarget)
	}

	// 7. Allowlist canonica estricta
	if !isAllowedCanonicalTarget(norm) {
		return "", fmt.Errorf("%w: %q no pertenece a la Allowlist canonica", ErrUnsafeTargetPath, rawTarget)
	}

	return norm, nil
}

func resolveProposalFile(absVault, propPath string) (string, error) {
	trimmed := strings.TrimSpace(propPath)
	if trimmed == "" {
		return "", errors.New("ruta de propuesta vacia")
	}

	// F4: Prohibir caracteres de Alternate Data Stream (:), short names (~), wildcards, control chars
	check := trimmed
	if len(check) >= 2 && check[1] == ':' {
		check = check[2:]
	}
	if strings.ContainsAny(check, ":~*?\"<>|\x00") {
		return "", fmt.Errorf("ruta de propuesta %q contiene caracteres prohibidos (: ~ * ? \" < > |)", propPath)
	}

	slashPath := strings.ReplaceAll(trimmed, `\`, "/")
	baseName := path.Base(slashPath)
	if !reValidProposalFilename.MatchString(baseName) {
		return "", fmt.Errorf("nombre de propuesta %q no cumple con el formato canonico ^patch-([0-9]{8}-[0-9]{6}-)?[a-z0-9_-]+\\.md$", baseName)
	}

	// 1. Resolver ruta fisica real de la boveda
	realVault, err := filepath.EvalSymlinks(absVault)
	if err != nil {
		return "", fmt.Errorf("resolviendo boveda %s: %w", absVault, err)
	}
	realVaultClean := filepath.Clean(realVault)

	proposalsDir := filepath.Join(absVault, "_proposals")
	fiProp, lstatErr := os.Lstat(proposalsDir)
	if lstatErr != nil {
		return "", fmt.Errorf("error accediendo a _proposals: %w", lstatErr)
	}
	if isReparseOrSymlink(fiProp) {
		return "", errors.New("directorio _proposals es un symlink/junction no permitido")
	}

	realProposals, err := filepath.EvalSymlinks(proposalsDir)
	if err != nil {
		return "", fmt.Errorf("resolviendo directorio _proposals: %w", err)
	}
	realProposalsClean := filepath.Clean(realProposals)

	// F3: Verificar igualdad exacta con realVault/_proposals
	expectedProposalsDir := filepath.Clean(filepath.Join(realVaultClean, "_proposals"))
	if !strings.EqualFold(realProposalsClean, expectedProposalsDir) {
		return "", fmt.Errorf("directorio _proposals no apunta exactamente a %s (resuelve a %s)", expectedProposalsDir, realProposalsClean)
	}

	// 2. Determinar ruta candidata
	var candPath string
	if filepath.IsAbs(trimmed) {
		candPath = trimmed
	} else if strings.HasPrefix(strings.ToLower(slashPath), "_proposals/") {
		relSub := slashPath[len("_proposals/"):]
		candPath = filepath.Join(proposalsDir, filepath.FromSlash(relSub))
	} else if !strings.Contains(trimmed, "/") && !strings.Contains(trimmed, `\`) {
		candPath = filepath.Join(proposalsDir, trimmed)
	} else {
		return "", fmt.Errorf("propuesta insegura %q: debe residir dentro de _proposals/", propPath)
	}

	absCand, err := filepath.Abs(candPath)
	if err != nil {
		return "", fmt.Errorf("resolviendo propuesta %s: %w", candPath, err)
	}

	// 3. Verificar que no sea symlink ni directorio
	fi, err := os.Lstat(absCand)
	if err != nil {
		return "", fmt.Errorf("propuesta no encontrada: %s (%w)", propPath, err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("ruta de propuesta es un directorio: %s", propPath)
	}
	if isReparseOrSymlink(fi) {
		return "", fmt.Errorf("archivo de propuesta es un enlace simbolico o junction no permitido: %s", propPath)
	}

	// 4. Evaluar symlinks de la ruta candidata y comprobar contención exacta en realProposalsClean (F3)
	realCand, err := filepath.EvalSymlinks(absCand)
	if err != nil {
		return "", fmt.Errorf("resolviendo enlaces simbolicos de %s: %w", absCand, err)
	}
	realCandClean := filepath.Clean(realCand)
	candParentClean := filepath.Dir(realCandClean)

	if !strings.EqualFold(candParentClean, realProposalsClean) {
		return "", fmt.Errorf("propuesta %s no reside directamente en _proposals/ (padre: %s)", propPath, candParentClean)
	}

	return absCand, nil
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
