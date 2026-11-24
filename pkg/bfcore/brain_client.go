package bfcore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrBrainUnavailable indica que 0_Brain no esta disponible o fallo al ejecutarse.
	ErrBrainUnavailable = errors.New("0_Brain no disponible o preflight fallido")
	// ErrBrainIndexMissing indica que .index/brain.db no existe en la boveda.
	ErrBrainIndexMissing = errors.New("indice .index/brain.db ausente")
	// ErrBrainJournalActive indica que existe un diario pendiente .brain-journal.json en la boveda (NG2 read-only).
	ErrBrainJournalActive = errors.New("diario transaccional .brain-journal.json pendiente")

	riskLevelRankMap = map[string]int{
		"NONE":    0,
		"BAJO":    1,
		"MEDIO":   2,
		"ALTO":    3,
		"CRITICO": 4,
	}
)

// RiskHit representa una zona de riesgo de wiki/risk_zones.md que coincidio con las rutas consultadas.
type RiskHit struct {
	ID            string   `json:"id"`
	Project       string   `json:"project"`
	Level         string   `json:"level"`
	MatchedPaths  []string `json:"matched_paths"`
	MandatoryGate string   `json:"mandatory_gate"`
}

// BrainInvocation registra la evidencia literal G1 de una consulta a 0_Brain/brain.exe.
type BrainInvocation struct {
	Command     string `json:"command"`
	ExitCode    int    `json:"exit_code"`
	DurationMs  int64  `json:"duration_ms"`
	ResultCount int    `json:"result_count"`
}

// BrainDocMatch resume un documento OKF coincidente devuelto por 0_Brain.
type BrainDocMatch struct {
	ID                  string    `json:"id"`
	RelPath             string    `json:"rel_path"`
	Type                string    `json:"type"`
	Status              string    `json:"status"`
	Project             string    `json:"project,omitempty"`
	VerifiedHead        string    `json:"verified_head"`
	Transversal         bool      `json:"transversal"`
	CoversPaths         []string  `json:"covers_paths,omitempty"`
	MatchedPaths        []string  `json:"matched_paths,omitempty"`
	RiskHits            []RiskHit `json:"risk_hits,omitempty"`
	RiskZonesSHA256     string    `json:"risk_zones_sha256,omitempty"`
	RiskZonesParseError string    `json:"risk_zones_parse_error,omitempty"`
}

// BrainPreflight contiene los metadatos de verificacion read-only de la boveda 0_Brain.
type BrainPreflight struct {
	VaultRoot    string
	DBPath       string
	BrainDBMTime string
}

// BrainContextResponse agrupa la respuesta estructurada de 0_Brain/brain.exe context --json.
type BrainContextResponse struct {
	MatchedDocs         []BrainDocMatch
	RiskHits            []RiskHit
	RiskZonesSHA256     string
	RiskZonesParseError string
	HasActiveRiskZones  bool
}

// BrainClient abstrae las operaciones read-only compartidas con 0_Brain (Preflight y Context).
type BrainClient interface {
	Preflight(vaultRoot, dbPath string) (BrainPreflight, error)
	Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error)
}

// SubprocessBrainClient implementa BrainClient invocando 0_Brain/brain.exe como subproceso.
type SubprocessBrainClient struct {
	BrainBin string
	Timeout  time.Duration
}

// NewSubprocessBrainClient construye un SubprocessBrainClient con timeout por defecto de 5s.
func NewSubprocessBrainClient(brainBin string, timeout time.Duration) *SubprocessBrainClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SubprocessBrainClient{
		BrainBin: brainBin,
		Timeout:  timeout,
	}
}

// Preflight ejecuta la verificacion estrictamente de solo lectura (NG2) sobre vaultRoot y dbPath:
//  1. Verifica CheckNoReparseChain sobre vaultRoot y dbPath.
//  2. Comprueba os.Lstat sobre dbPath (.index/brain.db). Si falta, devuelve multi-wrap ErrBrainUnavailable + ErrBrainIndexMissing.
//  3. Comprueba os.Lstat sobre <vaultRoot>/.brain-journal.json. Si existe, devuelve multi-wrap ErrBrainUnavailable + ErrBrainJournalActive
//     sin invocar brain.exe verify (evitando disparar RecoverJournal que mutaria la boveda).
func (c *SubprocessBrainClient) Preflight(vaultRoot, dbPath string) (BrainPreflight, error) {
	if c == nil {
		return BrainPreflight{}, fmt.Errorf("%w: cliente brain nil", ErrBrainUnavailable)
	}
	if strings.TrimSpace(vaultRoot) == "" {
		return BrainPreflight{}, fmt.Errorf("%w: ruta de boveda vacia", ErrBrainUnavailable)
	}
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(vaultRoot, ".index", "brain.db")
	}

	if err := CheckNoReparseChain("", vaultRoot); err != nil {
		return BrainPreflight{}, fmt.Errorf("%w: reparse point en boveda: %s", ErrBrainUnavailable, RedactAllG4(err.Error()))
	}
	if err := CheckNoReparseChain(vaultRoot, dbPath); err != nil {
		return BrainPreflight{}, fmt.Errorf("%w: reparse point en brain.db: %s", ErrBrainUnavailable, RedactAllG4(err.Error()))
	}

	dbDisplay := RedactAllG4(filepath.ToSlash(dbPath))
	fiDB, err := os.Lstat(dbPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BrainPreflight{}, fmt.Errorf("%w: %w: %s (ejecute '0_Brain/brain.exe index')", ErrBrainUnavailable, ErrBrainIndexMissing, dbDisplay)
		}
		return BrainPreflight{}, fmt.Errorf("%w: inspeccionando %s: %s", ErrBrainUnavailable, dbDisplay, RedactAllG4(err.Error()))
	}
	if fiDB.IsDir() || IsReparseOrSymlink(fiDB) || !fiDB.Mode().IsRegular() {
		return BrainPreflight{}, fmt.Errorf("%w: %s no es un archivo regular", ErrBrainUnavailable, dbDisplay)
	}

	journalPath := filepath.Join(vaultRoot, ".brain-journal.json")
	journalDisplay := RedactAllG4(filepath.ToSlash(journalPath))
	if _, jErr := os.Lstat(journalPath); jErr == nil {
		return BrainPreflight{}, fmt.Errorf("%w: %w: diario pendiente %s detectado (NG2 read-only: ejecute '0_Brain/brain.exe verify' manualmente)", ErrBrainUnavailable, ErrBrainJournalActive, journalDisplay)
	} else if !errors.Is(jErr, os.ErrNotExist) {
		return BrainPreflight{}, fmt.Errorf("%w: inspeccionando %s: %s", ErrBrainUnavailable, journalDisplay, RedactAllG4(jErr.Error()))
	}

	return BrainPreflight{
		VaultRoot:    vaultRoot,
		DBPath:       dbPath,
		BrainDBMTime: fiDB.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Context invoca 0_Brain/brain.exe context --vault <v> --db <db> --project <p> --files <csv> --json
// y extrae los documentos activos junto con los risk_hits estructurados de wiki/risk_zones.md.
func (c *SubprocessBrainClient) Context(vaultRoot, dbPath, project string, files []string) (BrainContextResponse, BrainInvocation, error) {
	if c == nil {
		return BrainContextResponse{}, BrainInvocation{ExitCode: -1}, fmt.Errorf("%w: cliente brain nil", ErrBrainUnavailable)
	}
	if strings.TrimSpace(dbPath) == "" {
		dbPath = filepath.Join(vaultRoot, ".index", "brain.db")
	}
	csvFiles := strings.Join(files, ",")
	args := []string{"context", "--vault", vaultRoot, "--db", dbPath}
	if strings.TrimSpace(project) != "" {
		args = append(args, "--project", strings.TrimSpace(project))
	}
	if csvFiles != "" {
		args = append(args, "--files", csvFiles)
	}
	args = append(args, "--json")

	outBytes, inv, err := c.ExecBrain(args)
	if err != nil {
		return BrainContextResponse{}, inv, err
	}

	var docs []BrainDocMatch
	trimmed := bytes.TrimSpace(outBytes)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &docs); err != nil {
			return BrainContextResponse{}, inv, fmt.Errorf("%w: decodificando salida de brain context: %v", ErrBrainUnavailable, err)
		}
	}
	if docs == nil {
		docs = []BrainDocMatch{}
	}
	inv.ResultCount = len(docs)

	resp := BrainContextResponse{
		MatchedDocs: make([]BrainDocMatch, 0, len(docs)),
		RiskHits:    []RiskHit{},
	}
	for _, d := range docs {
		if !strings.EqualFold(d.Status, "active") {
			continue
		}
		if strings.EqualFold(d.Type, "risk_zones") {
			resp.HasActiveRiskZones = true
			if d.RiskZonesSHA256 != "" {
				resp.RiskZonesSHA256 = d.RiskZonesSHA256
			}
			if d.RiskZonesParseError != "" {
				resp.RiskZonesParseError = d.RiskZonesParseError
			}
			if len(d.RiskHits) > 0 {
				resp.RiskHits = append(resp.RiskHits, d.RiskHits...)
			}
		}
		d.RiskHits = nil
		d.RiskZonesSHA256 = ""
		d.RiskZonesParseError = ""
		resp.MatchedDocs = append(resp.MatchedDocs, d)
	}
	return resp, inv, nil
}

// ExecBrain ejecuta el binario configurado de 0_Brain con timeout y redaccion G4 del comando y errores.
func (c *SubprocessBrainClient) ExecBrain(args []string) ([]byte, BrainInvocation, error) {
	if c == nil {
		return nil, BrainInvocation{ExitCode: -1}, fmt.Errorf("%w: cliente brain nil", ErrBrainUnavailable)
	}
	bin := strings.TrimSpace(c.BrainBin)
	if bin == "" {
		bin = filepath.Join("0_Brain", "brain.exe")
	}
	cmdStr := formatCommandDisplay(bin, args)
	cmdStr, _ = RedactTier2Text("command", cmdStr)
	binDisplay := RedactAllG4(filepath.ToSlash(bin))

	if err := CheckNoReparseChain("", bin); err != nil {
		return nil, BrainInvocation{Command: cmdStr, ExitCode: -1}, fmt.Errorf("%w: reparse point en binario %s: %s", ErrBrainUnavailable, binDisplay, RedactAllG4(err.Error()))
	}
	fiBin, err := os.Lstat(bin)
	if err != nil {
		return nil, BrainInvocation{Command: cmdStr, ExitCode: -1}, fmt.Errorf("%w: binario brain.exe no encontrado en %s: %s", ErrBrainUnavailable, binDisplay, RedactAllG4(err.Error()))
	}
	if fiBin.IsDir() || IsReparseOrSymlink(fiBin) || !fiBin.Mode().IsRegular() {
		return nil, BrainInvocation{Command: cmdStr, ExitCode: -1}, fmt.Errorf("%w: binario %s no es un archivo regular", ErrBrainUnavailable, binDisplay)
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	dur := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		inv := BrainInvocation{Command: cmdStr, ExitCode: exitCode, DurationMs: dur}
		errMsg := RedactAllG4(err.Error())
		stderrMsg := RedactAllG4(strings.TrimSpace(stderr.String()))
		return nil, inv, fmt.Errorf("%w: comando %q fallo (exit=%d): %s (%s)", ErrBrainUnavailable, cmdStr, exitCode, errMsg, stderrMsg)
	}

	return stdout.Bytes(), BrainInvocation{Command: cmdStr, ExitCode: 0, DurationMs: dur}, nil
}

func formatCommandDisplay(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, filepath.ToSlash(bin))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			parts = append(parts, fmt.Sprintf("%q", a))
		} else {
			parts = append(parts, filepath.ToSlash(a))
		}
	}
	return strings.Join(parts, " ")
}

// FindUncoveredPaths devuelve el subconjunto de paths que no estan cubiertos por ningun documento
// activo de tipo "wiki" o "adr" en matchedDocs (evaluando MatchedPaths y CoversPaths).
func FindUncoveredPaths(paths []string, matchedDocs []BrainDocMatch) []string {
	var uncovered []string
	for _, file := range paths {
		covered := false
		for _, doc := range matchedDocs {
			if !strings.EqualFold(doc.Status, "active") {
				continue
			}
			if !strings.EqualFold(doc.Type, "wiki") && !strings.EqualFold(doc.Type, "adr") {
				continue
			}
			for _, mp := range doc.MatchedPaths {
				if strings.EqualFold(NormalizeCoverPath(mp), NormalizeCoverPath(file)) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
			for _, cp := range doc.CoversPaths {
				if MatchCoverPathFold(cp, file) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			uncovered = append(uncovered, file)
		}
	}
	return uncovered
}

// RiskLevelRank devuelve la severidad numerica (0..4) de un nivel de riesgo canonico.
func RiskLevelRank(level string) int {
	if r, ok := riskLevelRankMap[strings.ToUpper(strings.TrimSpace(level))]; ok {
		return r
	}
	return 0
}
