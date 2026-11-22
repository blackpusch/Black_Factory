package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"blackfactory/brain"
)

// BrainService abstrae las operaciones deterministas de 0_Brain para reutilizacion
// tanto desde el CLI actual como desde un futuro adaptador MCP (Fase 2).
type BrainService interface {
	InitVault(vaultRoot, schemasDir string) error
	BuildIndex(vaultRoot, dbPath string) (brain.IndexStats, error)
	Search(dbPath string, opts brain.QueryOptions) ([]brain.SearchResult, error)
	ContextForFiles(dbPath, project string, files []string) ([]brain.SearchResult, error)
	ValidateVault(vaultRoot string) (brain.ValidationReport, error)
	CheckDrift(vaultRoot string, projectRepos map[string]string) ([]brain.DriftItem, error)
	ProposeUpdate(vaultRoot string, req brain.ProposeRequest) (brain.ProposeResult, error)
	ApplyUpdate(vaultRoot string, opts brain.ApplyOptions) (brain.ApplyResult, error)
	SignProposal(vaultRoot, propPath, privKeyPath string) (string, error)
}

// LocalService implementa BrainService sobre la boveda y motor SQLite locales.
type LocalService struct{}

func (LocalService) SignProposal(vaultRoot, propPath, privKeyPath string) (string, error) {
	return brain.SignProposal(vaultRoot, propPath, privKeyPath, nil)
}

func (LocalService) InitVault(vaultRoot, schemasDir string) error {
	return brain.InitVault(vaultRoot, schemasDir)
}

func (LocalService) BuildIndex(vaultRoot, dbPath string) (brain.IndexStats, error) {
	if strings.TrimSpace(dbPath) != "" {
		return brain.BuildIndexAt(vaultRoot, dbPath)
	}
	return brain.BuildIndex(vaultRoot)
}

func (LocalService) Search(dbPath string, opts brain.QueryOptions) ([]brain.SearchResult, error) {
	return brain.Search(dbPath, opts)
}

func (LocalService) ContextForFiles(dbPath, project string, files []string) ([]brain.SearchResult, error) {
	return brain.ContextForFiles(dbPath, project, files)
}

func (LocalService) ValidateVault(vaultRoot string) (brain.ValidationReport, error) {
	return brain.ValidateVault(vaultRoot)
}

func (LocalService) CheckDrift(vaultRoot string, projectRepos map[string]string) ([]brain.DriftItem, error) {
	return brain.CheckDrift(vaultRoot, projectRepos)
}

func (LocalService) ProposeUpdate(vaultRoot string, req brain.ProposeRequest) (brain.ProposeResult, error) {
	return brain.ProposeUpdate(vaultRoot, req)
}

func (LocalService) ApplyUpdate(vaultRoot string, opts brain.ApplyOptions) (brain.ApplyResult, error) {
	return brain.ApplyUpdate(vaultRoot, opts)
}

type VerifyOutput struct {
	Validation brain.ValidationReport `json:"validation"`
	Drift      []brain.DriftItem      `json:"drift"`
	OK         bool                   `json:"ok"`
}

type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }
func (s *stringListFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, LocalService{}))
}

func run(args []string, out, errOut io.Writer, svc BrainService) int {
	if svc == nil {
		svc = LocalService{}
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(out)
		if len(args) == 0 {
			return 1
		}
		return 0
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "init-vault":
		return cmdInitVault(rest, out, errOut, svc)
	case "index":
		return cmdIndex(rest, out, errOut, svc)
	case "query":
		return cmdQuery(rest, out, errOut, svc)
	case "context":
		return cmdContext(rest, out, errOut, svc)
	case "verify":
		return cmdVerify(rest, out, errOut, svc)
	case "propose":
		return cmdPropose(rest, out, errOut, svc)
	case "apply":
		return cmdApply(rest, out, errOut, svc)
	case "sign":
		return cmdSign(rest, out, errOut, svc)
	case "mcp":
		fmt.Fprintln(errOut, "error: subcomando 'mcp' reservado para Fase 2 sobre BrainService")
		return 2
	default:
		printUsage(errOut)
		fmt.Fprintf(errOut, "error: subcomando desconocido %q\n", sub)
		return 1
	}
}

func cmdInitVault(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain init-vault", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, schemasDir string
	var asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&schemasDir, "schemas", "", "Ruta del directorio schemas/")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{"vault": true, "schemas": true})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	if vaultRoot == "" && fs.NArg() > 0 {
		vaultRoot = fs.Arg(0)
	}
	vaultRoot = resolveVaultPath(vaultRoot)
	schemasDir = resolveSchemasDir(vaultRoot, schemasDir)
	if schemasDir == "" {
		fmt.Fprintln(errOut, "error: no se pudo resolver --schemas; indica la ruta a 0_Brain/schemas")
		return 1
	}

	if err := svc.InitVault(vaultRoot, schemasDir); err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	if asJSON {
		writeJSON(out, map[string]any{"ok": true, "vault": vaultRoot, "schemas": schemasDir})
	} else {
		fmt.Fprintf(out, "Boveda inicializada en %s (esquemas: %s)\n", vaultRoot, schemasDir)
	}
	return 0
}

func cmdIndex(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain index", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, dbPath string
	var asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&dbPath, "db", "", "Ruta alternativa de brain.db")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{"vault": true, "db": true})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	if vaultRoot == "" && fs.NArg() > 0 {
		vaultRoot = fs.Arg(0)
	}
	vaultRoot = resolveVaultPath(vaultRoot)

	stats, err := svc.BuildIndex(vaultRoot, dbPath)
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	if asJSON {
		writeJSON(out, stats)
	} else {
		fmt.Fprintf(out, "Indice construido en %s (%d documentos, %d rutas, %dms)\n",
			stats.DBPath, stats.DocsIndexed, stats.PathsIndexed, stats.DurationMs)
	}
	return 0
}

func cmdQuery(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain query", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, dbPath, project, docType, tag, qFlag string
	var limit int
	var asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&dbPath, "db", "", "Ruta de brain.db")
	fs.StringVar(&project, "project", "", "Filtrar por proyecto")
	fs.StringVar(&docType, "type", "", "Filtrar por tipo OKF")
	fs.StringVar(&tag, "tag", "", "Filtrar por etiqueta exacta")
	fs.IntVar(&limit, "limit", 20, "Limite de resultados")
	fs.StringVar(&qFlag, "q", "", "Consulta FTS5")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"vault": true, "db": true, "project": true, "type": true, "tag": true, "limit": true, "q": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	vaultRoot = resolveVaultPath(vaultRoot)
	db := resolveDBPath(vaultRoot, dbPath)

	queryStr := strings.TrimSpace(qFlag)
	if queryStr == "" && fs.NArg() > 0 {
		queryStr = strings.Join(fs.Args(), " ")
	}

	results, err := svc.Search(db, brain.QueryOptions{
		Query:   queryStr,
		Project: project,
		Type:    docType,
		Tag:     tag,
		Limit:   limit,
	})
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}

	if asJSON {
		writeJSON(out, results)
	} else {
		printResultsText(out, results)
	}
	return 0
}

func cmdContext(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain context", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, dbPath, project string
	var fileFlags, filesCSVFlags stringListFlag
	var asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&dbPath, "db", "", "Ruta de brain.db")
	fs.StringVar(&project, "project", "", "Proyecto objetivo")
	fs.Var(&fileFlags, "file", "Archivo modificado (repetible)")
	fs.Var(&filesCSVFlags, "files", "Lista de archivos separados por coma")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"vault": true, "db": true, "project": true, "file": true, "files": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	vaultRoot = resolveVaultPath(vaultRoot)
	db := resolveDBPath(vaultRoot, dbPath)

	var files []string
	for _, csv := range filesCSVFlags {
		files = append(files, splitCSV(csv)...)
	}
	files = append(files, fileFlags...)
	files = append(files, fs.Args()...)

	results, err := svc.ContextForFiles(db, project, files)
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	if asJSON {
		writeJSON(out, results)
	} else {
		printResultsText(out, results)
	}
	return 0
}

func cmdVerify(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain verify", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot string
	var repoFlags stringListFlag
	var failOnDrift, asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.Var(&repoFlags, "repo", "Mapeo proyecto=ruta_repo para detectar drift Git")
	fs.BoolVar(&failOnDrift, "fail-on-drift", false, "Retornar codigo != 0 si se detecta drift")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"fail-on-drift": true, "json": true}, map[string]bool{
		"vault": true, "repo": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	if vaultRoot == "" && fs.NArg() > 0 {
		vaultRoot = fs.Arg(0)
	}
	vaultRoot = resolveVaultPath(vaultRoot)

	// Adquirir lock del SO y recuperar journal previo si existe (J2)
	lock, lockErr := brain.AcquireVaultLock(vaultRoot)
	if lockErr != nil {
		fmt.Fprintf(errOut, "error: %v\n", lockErr)
		return 4
	}
	defer lock.Unlock()

	if recErr := brain.RecoverJournal(vaultRoot); recErr != nil {
		fmt.Fprintf(errOut, "error recuperando journal: %v\n", recErr)
		return 3
	}

	projectRepos, err := parseRepoMappings(repoFlags)
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}

	report, err := svc.ValidateVault(vaultRoot)
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}

	drift := []brain.DriftItem{}
	if len(projectRepos) > 0 {
		drift, err = svc.CheckDrift(vaultRoot, projectRepos)
		if err != nil {
			fmt.Fprintf(errOut, "error: %v\n", err)
			return 1
		}
	}

	ok := report.OK && (!failOnDrift || len(drift) == 0)
	payload := VerifyOutput{
		Validation: report,
		Drift:      drift,
		OK:         ok,
	}

	if asJSON {
		writeJSON(out, payload)
	} else {
		fmt.Fprintf(out, "Verify OK=%v | archivos=%d | parseados=%d | issues=%d | drift=%d\n",
			payload.OK, report.FilesChecked, report.DocsParsed, len(report.Issues), len(drift))
		for _, iss := range report.Issues {
			fmt.Fprintf(out, "  [%s] %s:%d - %s\n", iss.Code, iss.File, iss.Line, iss.Message)
		}
		for _, d := range drift {
			fmt.Fprintf(out, "  [DRIFT] %s (%s): verified=%s current=%s files=%v\n",
				d.RelPath, d.Project, d.VerifiedHead, d.CurrentHead, d.ChangedFiles)
		}
	}
	if !ok {
		return 1
	}
	return 0
}

func cmdPropose(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain propose", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, target, title, project, docID, docType, status, operation, authorRole, reason string
	var tagsCSV, coversCSV, verifiedHead, body, bodyFile string
	var asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&target, "target", "", "Ruta relativa destino dentro de la boveda")
	fs.StringVar(&title, "title", "", "Titulo del documento")
	fs.StringVar(&project, "project", "black-factory", "Proyecto")
	fs.StringVar(&docID, "id", "", "ID OKF del documento destino")
	fs.StringVar(&docType, "type", "", "Tipo OKF (wiki|adr|constitution|risk_zones)")
	fs.StringVar(&status, "status", "", "Estado OKF (active|proposed|draft)")
	fs.StringVar(&operation, "operation", "", "Operacion (create|update)")
	fs.StringVar(&authorRole, "author", "", "Rol del equipo autor")
	fs.StringVar(&reason, "reason", "", "Resumen de justificacion")
	fs.StringVar(&tagsCSV, "tags", "", "Etiquetas separadas por coma")
	fs.StringVar(&coversCSV, "covers", "", "Rutas cubiertas separadas por coma")
	fs.StringVar(&verifiedHead, "verified-head", "", "Commit verificado del repositorio")
	fs.StringVar(&body, "body", "", "Contenido Markdown propuesto")
	fs.StringVar(&bodyFile, "body-file", "", "Archivo con contenido Markdown propuesto")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"vault": true, "target": true, "title": true, "project": true, "id": true,
		"type": true, "status": true, "operation": true, "author": true, "reason": true,
		"tags": true, "covers": true, "verified-head": true, "body": true, "body-file": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	vaultRoot = resolveVaultPath(vaultRoot)

	content := body
	if strings.TrimSpace(bodyFile) != "" {
		b, err := os.ReadFile(bodyFile)
		if err != nil {
			fmt.Fprintf(errOut, "error leyendo --body-file: %v\n", err)
			return 1
		}
		content = string(b)
	}

	res, err := svc.ProposeUpdate(vaultRoot, brain.ProposeRequest{
		TargetPath:   target,
		Title:        title,
		Project:      project,
		DocID:        docID,
		DocType:      docType,
		Status:       status,
		Operation:    operation,
		AuthorRole:   authorRole,
		Reason:       reason,
		Tags:         splitCSV(tagsCSV),
		CoversPaths:  splitCSV(coversCSV),
		VerifiedHead: verifiedHead,
		Content:      content,
	})
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	if asJSON {
		writeJSON(out, res)
	} else {
		fmt.Fprintf(out, "Propuesta creada: %s (target=%s, autor_gate=%v)\n",
			res.ProposalPath, res.TargetPath, res.RequiresAutorApproval)
	}
	return 0
}

func cmdApply(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain apply", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, proposalPath, autorSig string
	var approveAutor, asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&proposalPath, "proposal", "", "Ruta del archivo BrainPatch")
	fs.StringVar(&autorSig, "autor-sig", "", "Firma Ed25519 en Base64 emitida por el [AUTOR]")
	fs.BoolVar(&approveAutor, "approve-autor", false, "Aprobacion explicita del [AUTOR] (deprecado)")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"approve-autor": true, "json": true}, map[string]bool{
		"vault": true, "proposal": true, "autor-sig": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	if proposalPath == "" && fs.NArg() > 0 {
		proposalPath = fs.Arg(0)
	}
	vaultRoot = resolveVaultPath(vaultRoot)

	res, err := svc.ApplyUpdate(vaultRoot, brain.ApplyOptions{
		ProposalPath: proposalPath,
		AutorSig:     autorSig,
		ApproveAutor: approveAutor,
	})
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		if errors.Is(err, brain.ErrAutorApprovalRequired) {
			return 2
		}
		if errors.Is(err, brain.ErrJournalCorrupt) || errors.Is(err, brain.ErrCommitDegraded) {
			return 3
		}
		if errors.Is(err, brain.ErrVaultLocked) {
			return 4
		}
		return 1
	}
	if asJSON {
		writeJSON(out, res)
	} else {
		fmt.Fprintf(out, "Propuesta %s aplicada en %s (%d docs indexados)\n",
			res.ProposalID, res.TargetPath, res.IndexStats.DocsIndexed)
	}
	return 0
}

func cmdSign(args []string, out, errOut io.Writer, svc BrainService) int {
	fs := flag.NewFlagSet("brain sign", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var vaultRoot, proposalPath, keyPath string
	var asJSON bool
	fs.StringVar(&vaultRoot, "vault", "", "Ruta de la boveda local")
	fs.StringVar(&proposalPath, "proposal", "", "Ruta del archivo BrainPatch a firmar")
	fs.StringVar(&keyPath, "key", "", "Ruta del archivo con la clave privada Ed25519")
	fs.BoolVar(&asJSON, "json", false, "Salida JSON")

	reordered := reorderFlags(args, map[string]bool{"json": true}, map[string]bool{
		"vault": true, "proposal": true, "key": true,
	})
	if err := fs.Parse(reordered); err != nil {
		return 1
	}
	if proposalPath == "" && fs.NArg() > 0 {
		proposalPath = fs.Arg(0)
	}
	vaultRoot = resolveVaultPath(vaultRoot)

	if strings.TrimSpace(proposalPath) == "" {
		fmt.Fprintln(errOut, "error: se requiere especificar --proposal")
		return 1
	}
	if strings.TrimSpace(keyPath) == "" {
		fmt.Fprintln(errOut, "error: se requiere especificar --key con la clave privada")
		return 1
	}

	sigB64, err := svc.SignProposal(vaultRoot, proposalPath, keyPath)
	if err != nil {
		fmt.Fprintf(errOut, "error firmando propuesta: %v\n", err)
		return 1
	}

	if asJSON {
		writeJSON(out, map[string]string{
			"proposal":  proposalPath,
			"autor_sig": sigB64,
		})
	} else {
		fmt.Fprintf(out, "Firma Ed25519 generada con exito:\n\n%s\n\nPara aplicar ejecute:\n  brain apply --vault %q --proposal %q --autor-sig %s\n",
			sigB64, vaultRoot, proposalPath, sigB64)
	}
	return 0
}

func resolveVaultPath(vaultFlag string) string {
	if strings.TrimSpace(vaultFlag) != "" {
		return vaultFlag
	}
	if env := strings.TrimSpace(os.Getenv("BRAIN_VAULT")); env != "" {
		return env
	}
	defaultRel := filepath.Join("0_Brain", "vault")
	if fi, err := os.Stat(defaultRel); err == nil && fi.IsDir() {
		return defaultRel
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "vault")
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return defaultRel
}

func resolveDBPath(vaultRoot, dbFlag string) string {
	if strings.TrimSpace(dbFlag) != "" {
		return dbFlag
	}
	return brain.DefaultIndexPath(vaultRoot)
}

func resolveSchemasDir(vaultRoot, schemasFlag string) string {
	if strings.TrimSpace(schemasFlag) != "" {
		return schemasFlag
	}
	candidates := []string{
		filepath.Join(filepath.Dir(filepath.Clean(vaultRoot)), "schemas"),
		filepath.Join("0_Brain", "schemas"),
		"schemas",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "schemas"))
	}
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			return cand
		}
	}
	return ""
}

func parseRepoMappings(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, raw := range items {
		k, v, ok := strings.Cut(raw, "=")
		if !ok {
			out["*"] = strings.TrimSpace(raw)
			continue
		}
		proj := strings.TrimSpace(k)
		repo := strings.TrimSpace(v)
		if proj == "" || repo == "" {
			return nil, fmt.Errorf("mapeo --repo invalido %q (usa proyecto=ruta)", raw)
		}
		out[proj] = repo
	}
	return out, nil
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func printResultsText(out io.Writer, results []brain.SearchResult) {
	if len(results) == 0 {
		fmt.Fprintln(out, "Sin resultados.")
		return
	}
	for i, r := range results {
		tag := ""
		if r.Transversal {
			tag = " [TRANSVERSAL]"
		}
		fmt.Fprintf(out, "%d. [%s] %s (%s)%s\n", i+1, r.Type, r.Title, r.RelPath, tag)
		if len(r.MatchedPaths) > 0 {
			fmt.Fprintf(out, "   matched: %s\n", strings.Join(r.MatchedPaths, ", "))
		}
		if r.Snippet != "" {
			fmt.Fprintf(out, "   %s\n", r.Snippet)
		}
	}
}

func writeJSON(out io.Writer, v any) {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// reorderFlags permite ubicar flags antes o despues de argumentos posicionales.
func reorderFlags(args []string, boolFlags, valFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			flags = append(flags, a)
			continue
		}
		if valFlags[name] && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i++
			continue
		}
		if boolFlags[name] && i+1 < len(args) {
			low := strings.ToLower(args[i+1])
			if low == "true" || low == "false" {
				flags = append(flags, a+"="+low)
				i++
				continue
			}
		}
		flags = append(flags, a)
	}
	return append(flags, pos...)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Uso: brain <subcomando> [banderas]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcomandos:")
	fmt.Fprintln(w, "  init-vault  Inicializa la boveda OKF desde schemas/ sin sobrescribir archivos existentes")
	fmt.Fprintln(w, "  index       Construye/reconstruye .index/brain.db (SQLite FTS5 pure-Go)")
	fmt.Fprintln(w, "  query       Busca documentos por FTS5/BM25 y filtros (--project, --type, --tag)")
	fmt.Fprintln(w, "  context     Obtiene contexto por covers_paths y documentos transversales")
	fmt.Fprintln(w, "  verify      Valida OKF, enlaces internos, reglas G4 y drift Git opcional (--repo)")
	fmt.Fprintln(w, "  propose     Crea un BrainPatch en _proposals/ sin mutar la boveda")
	fmt.Fprintln(w, "  apply       Valida y aplica un BrainPatch de forma atomica (--autor-sig para documentos gobernados)")
	fmt.Fprintln(w, "  sign        Firma una propuesta con clave privada Ed25519 del [AUTOR]")
}
