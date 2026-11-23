package intake

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

var (
	// ErrIntakeLocked indica que runtime/.intake.lock permanecio ocupado tras el backoff de 250ms (Exit code 4).
	ErrIntakeLocked = errors.New("runtime/.intake.lock ocupado por otro proceso activo")
	// ErrDedupeConflict indica que ya existe un WorkItem ready con el mismo dedupe_key y diferente payload SHA-256 sin --update (Exit code 5).
	ErrDedupeConflict = errors.New("conflicto de dedupe_key: existe WorkItem ready con diferente payload SHA-256")
	// ErrIDCollision indica una colision de prefijo de 48 bits (WI-<hash12>) entre dos dedupe_key de 64 hex distintos (Exit code 1).
	ErrIDCollision = errors.New("colision de prefijo WI-<hash12> con diferente dedupe_key de 64 hex")
	// ErrWorkItemNotFound indica que el WorkItem solicitado no existe en workitems/ ni en quarantine/ (Exit code 1).
	ErrWorkItemNotFound = errors.New("WorkItem no encontrado")
)

const defaultLockMaxWait = 250 * time.Millisecond

// SaveResult resume el resultado de SaveTriaged o SaveQuarantined.
type SaveResult struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	Status        string `json:"status"`
	Lane          string `json:"lane,omitempty"`
	Revision      int    `json:"revision"`
	IdempotentHit bool   `json:"idempotent_hit"`
	Promoted      bool   `json:"promoted"`
	Updated       bool   `json:"updated"`
	DryRun        bool   `json:"dry_run"`
}

// ListFilter parametriza los filtros de Store.List.
type ListFilter struct {
	Status  string
	Lane    string
	Project string
	Risk    string
}

// Store gestiona la persistencia atomica Option i bajo <DataDir>/workitems/, quarantine/, intake_events.jsonl y .intake.lock.
type Store struct {
	DataDir     string
	LockMaxWait time.Duration
}

// NewStore crea una instancia de Store sobre dataDir con backoff de 250ms para .intake.lock.
func NewStore(dataDir string) *Store {
	return &Store{
		DataDir:     dataDir,
		LockMaxWait: defaultLockMaxWait,
	}
}

func (s *Store) workitemsDir() string  { return filepath.Join(s.DataDir, "workitems") }
func (s *Store) quarantineDir() string { return filepath.Join(s.DataDir, "quarantine") }
func (s *Store) eventsPath() string    { return filepath.Join(s.DataDir, "intake_events.jsonl") }
func (s *Store) lockPath() string      { return filepath.Join(s.DataDir, ".intake.lock") }

func (s *Store) withLock(fn func() error) error {
	if strings.TrimSpace(s.DataDir) == "" {
		return errors.New("dataDir vacio en Store")
	}
	if err := bfcore.CheckNoReparseChain("", s.DataDir); err != nil {
		return fmt.Errorf("verificando reparse points en dataDir %s: %w", s.DataDir, err)
	}
	for _, dir := range []string{s.DataDir, s.workitemsDir(), s.quarantineDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creando directorio runtime %s: %w", dir, err)
		}
	}

	wait := s.LockMaxWait
	if wait <= 0 {
		wait = defaultLockMaxWait
	}
	lock, err := bfcore.AcquireFileLock(s.lockPath(), wait)
	if err != nil {
		if errors.Is(err, bfcore.ErrLockBusy) {
			return ErrIntakeLocked
		}
		return err
	}
	defer func() { _ = lock.Unlock() }()

	return fn()
}

// SaveTriaged persiste de forma atomica un WorkItem con status == "ready" en workitems/<wi.ID>.json,
// verificando colisiones de 64 hex, idempotencia, conflictos de dedupe_key (Exit 5), promocion desde quarantine/
// y barrera pre-escritura bfcore.ScanG4All. Si dryRun == true, no adquiere .intake.lock ni escribe en disco.
func (s *Store) SaveTriaged(wi *WorkItem, prevQuarantineID string, update, dryRun bool, durationMs int64) (SaveResult, error) {
	if errs := ValidateWorkItem(wi); len(errs) > 0 {
		return SaveResult{}, fmt.Errorf("WorkItem invalido antes de guardar: %s", strings.Join(errs, "; "))
	}

	dstPath := filepath.Join(s.workitemsDir(), wi.ID+".json")
	if dryRun {
		return SaveResult{
			ID:       wi.ID,
			Path:     dstPath,
			Status:   wi.Status,
			Lane:     wi.Lane,
			Revision: wi.Revision,
			DryRun:   true,
		}, nil
	}

	var res SaveResult
	err := s.withLock(func() error {
		// 1. Inspeccionar si ya existe workitems/<wi.ID>.json
		existingReady, err := loadWorkItemFile(dstPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}

		// 2. Inspeccionar si existe un archivo previo en quarantine/ (por prevQuarantineID o por el mismo wi.ID)
		var existingQuar *WorkItem
		var quarPathsToDelete []string
		if prevQuarantineID != "" {
			qPath := filepath.Join(s.quarantineDir(), prevQuarantineID+".json")
			if qw, qErr := loadWorkItemFile(qPath); qErr == nil {
				existingQuar = qw
				quarPathsToDelete = append(quarPathsToDelete, qPath)
			}
		}
		sameIDQuarPath := filepath.Join(s.quarantineDir(), wi.ID+".json")
		if qw, qErr := loadWorkItemFile(sameIDQuarPath); qErr == nil {
			if existingQuar == nil {
				existingQuar = qw
			}
			if prevQuarantineID != wi.ID {
				quarPathsToDelete = append(quarPathsToDelete, sameIDQuarPath)
			}
		}

		eventType := "TRIAGED"
		prevIDForEvent := ""

		if existingReady != nil {
			if existingReady.DedupeKey != wi.DedupeKey {
				return fmt.Errorf("%w: archivo %s tiene dedupe_key %s != %s", ErrIDCollision, wi.ID, existingReady.DedupeKey, wi.DedupeKey)
			}
			if existingReady.Sanitization.SanitizedPayloadSHA256 == wi.Sanitization.SanitizedPayloadSHA256 && existingQuar == nil {
				*wi = *existingReady
				res = SaveResult{
					ID:            wi.ID,
					Path:          dstPath,
					Status:        wi.Status,
					Lane:          wi.Lane,
					Revision:      wi.Revision,
					IdempotentHit: true,
				}
				return s.appendEventLocked(IntakeEvent{
					Timestamp:   time.Now().UTC().Format(time.RFC3339),
					TraceID:     wi.TraceID,
					Event:       "IDEMPOTENT_HIT",
					ID:          wi.ID,
					Revision:    wi.Revision,
					Project:     wi.Project,
					Source:      wi.Source,
					SourceRef:   wi.SourceRef,
					Status:      wi.Status,
					Lane:        wi.Lane,
					RiskLevel:   wi.RiskLevel,
					ReasonCodes: extractReasonCodes(wi),
					DurationMs:  durationMs,
				})
			}
			if !update {
				_ = s.appendEventLocked(IntakeEvent{
					Timestamp:   time.Now().UTC().Format(time.RFC3339),
					TraceID:     wi.TraceID,
					Event:       "CONFLICT_DEDUPE",
					ID:          wi.ID,
					Revision:    existingReady.Revision,
					Project:     wi.Project,
					Source:      wi.Source,
					SourceRef:   wi.SourceRef,
					Status:      existingReady.Status,
					Lane:        existingReady.Lane,
					RiskLevel:   existingReady.RiskLevel,
					ReasonCodes: []string{"DEDUPE_CONFLICT"},
					DurationMs:  durationMs,
				})
				return ErrDedupeConflict
			}
			wi.CreatedAt = existingReady.CreatedAt
			wi.Revision = existingReady.Revision + 1
			eventType = "UPDATED"
		}

		if existingQuar != nil {
			if existingReady == nil {
				wi.CreatedAt = existingQuar.CreatedAt
				wi.Revision = existingQuar.Revision + 1
			} else if existingQuar.Revision >= wi.Revision {
				wi.Revision = existingQuar.Revision + 1
			}
			eventType = "PROMOTED"
			if prevQuarantineID != "" && prevQuarantineID != wi.ID {
				prevIDForEvent = prevQuarantineID
			}
		}

		if err := writeWorkItemAtomic(dstPath, wi); err != nil {
			return err
		}

		for _, qp := range quarPathsToDelete {
			_ = os.Remove(qp)
		}

		res = SaveResult{
			ID:       wi.ID,
			Path:     dstPath,
			Status:   wi.Status,
			Lane:     wi.Lane,
			Revision: wi.Revision,
			Promoted: eventType == "PROMOTED",
			Updated:  eventType == "UPDATED",
		}

		return s.appendEventLocked(IntakeEvent{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			TraceID:     wi.TraceID,
			Event:       eventType,
			ID:          wi.ID,
			PrevID:      prevIDForEvent,
			Revision:    wi.Revision,
			Project:     wi.Project,
			Source:      wi.Source,
			SourceRef:   wi.SourceRef,
			Status:      wi.Status,
			Lane:        wi.Lane,
			RiskLevel:   wi.RiskLevel,
			ReasonCodes: extractReasonCodes(wi),
			DurationMs:  durationMs,
		})
	})

	return res, err
}

// SaveQuarantined persiste de forma atomica un WorkItem con status == "needs_clarification" en quarantine/<wi.ID>.json.
// Si dryRun == true, no adquiere .intake.lock ni escribe en disco.
func (s *Store) SaveQuarantined(wi *WorkItem, prevQuarantineID string, update, dryRun bool, durationMs int64) (SaveResult, error) {
	if errs := ValidateWorkItem(wi); len(errs) > 0 {
		return SaveResult{}, fmt.Errorf("WorkItem en cuarentena invalido antes de guardar: %s", strings.Join(errs, "; "))
	}

	dstPath := filepath.Join(s.quarantineDir(), wi.ID+".json")
	if dryRun {
		return SaveResult{
			ID:       wi.ID,
			Path:     dstPath,
			Status:   wi.Status,
			Revision: wi.Revision,
			DryRun:   true,
		}, nil
	}

	var res SaveResult
	err := s.withLock(func() error {
		// Si se paso --update y existia en workitems/<wi.ID>.json pero ahora fallo una compuerta, mover a quarantine/
		readyPath := filepath.Join(s.workitemsDir(), wi.ID+".json")
		existingReady, rErr := loadWorkItemFile(readyPath)
		if rErr == nil && existingReady != nil {
			if existingReady.DedupeKey != "" && wi.DedupeKey != "" && existingReady.DedupeKey != wi.DedupeKey {
				return ErrIDCollision
			}
			if !update {
				return ErrDedupeConflict
			}
			wi.CreatedAt = existingReady.CreatedAt
			wi.Revision = existingReady.Revision + 1
		}

		existingQuar, err := loadWorkItemFile(dstPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if existingQuar != nil {
			if existingQuar.DedupeKey != "" && wi.DedupeKey != "" && existingQuar.DedupeKey != wi.DedupeKey {
				return ErrIDCollision
			}
			if existingQuar.DedupeKey == "" && wi.DedupeKey == "" &&
				existingQuar.Sanitization.SanitizedPayloadSHA256 != wi.Sanitization.SanitizedPayloadSHA256 {
				return ErrIDCollision
			}
			wi.CreatedAt = existingQuar.CreatedAt
			if existingQuar.Sanitization.SanitizedPayloadSHA256 == wi.Sanitization.SanitizedPayloadSHA256 {
				wi.Revision = existingQuar.Revision
				wi.UpdatedAt = existingQuar.UpdatedAt
			} else {
				wi.Revision = existingQuar.Revision + 1
			}
		} else if prevQuarantineID != "" && prevQuarantineID != wi.ID {
			prevPath := filepath.Join(s.quarantineDir(), prevQuarantineID+".json")
			if prevQuar, pErr := loadWorkItemFile(prevPath); pErr == nil {
				wi.CreatedAt = prevQuar.CreatedAt
				wi.Revision = prevQuar.Revision + 1
				_ = os.Remove(prevPath)
			}
		}

		if err := writeWorkItemAtomic(dstPath, wi); err != nil {
			return err
		}
		if existingReady != nil && update {
			_ = os.Remove(readyPath)
		}

		res = SaveResult{
			ID:       wi.ID,
			Path:     dstPath,
			Status:   wi.Status,
			Revision: wi.Revision,
		}

		return s.appendEventLocked(IntakeEvent{
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			TraceID:     wi.TraceID,
			Event:       "QUARANTINED",
			ID:          wi.ID,
			Revision:    wi.Revision,
			Project:     wi.Project,
			Source:      wi.Source,
			SourceRef:   wi.SourceRef,
			Status:      wi.Status,
			Lane:        "",
			RiskLevel:   wi.RiskLevel,
			ReasonCodes: extractReasonCodes(wi),
			DurationMs:  durationMs,
		})
	})

	return res, err
}

// AppendRejectedG4Event sanitiza source y source_ref mediante bfcore.RedactAllG4, verifica bfcore.ScanG4All
// sobre la linea JSONL serializada y la anexa a intake_events.jsonl bajo .intake.lock.
func (s *Store) AppendRejectedG4Event(ev IntakeEvent, dryRun bool) error {
	ev.Event = "REJECTED_G4"
	ev.Source = bfcore.RedactAllG4(strings.TrimSpace(ev.Source))
	ev.SourceRef = bfcore.RedactAllG4(strings.TrimSpace(ev.SourceRef))
	ev.Project = bfcore.RedactAllG4(strings.TrimSpace(ev.Project))
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	lineBytes, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if findings := bfcore.ScanG4All(string(lineBytes)); len(findings) > 0 {
		return fmt.Errorf("evento REJECTED_G4 incumple ScanG4All: %s", findings[0].Rule)
	}
	if dryRun {
		return nil
	}

	return s.withLock(func() error {
		return s.appendEventLocked(ev)
	})
}

func (s *Store) appendEventLocked(ev IntakeEvent) error {
	lineBytes, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("serializando IntakeEvent: %w", err)
	}
	if findings := bfcore.ScanG4All(string(lineBytes)); len(findings) > 0 {
		return fmt.Errorf("barrera pre-escritura G4 bloqueo IntakeEvent (%s): regla %s", ev.Event, findings[0].Rule)
	}
	lineBytes = append(lineBytes, '\n')

	f, err := os.OpenFile(s.eventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("abriendo intake_events.jsonl: %w", err)
	}
	defer f.Close()
	_, err = f.Write(lineBytes)
	return err
}

// List escanea workitems/ y quarantine/ devolviendo siempre un slice no nulo []WorkItem
// ordenado deterministicamente por updated_at DESC, id ASC.
func (s *Store) List(filter ListFilter) ([]WorkItem, error) {
	items := make([]WorkItem, 0)
	for _, dir := range []string{s.workitemsDir(), s.quarantineDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			wi, err := loadWorkItemFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			if !matchesListFilter(wi, filter) {
				continue
			}
			items = append(items, *wi)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return items[i].UpdatedAt > items[j].UpdatedAt
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func matchesListFilter(wi *WorkItem, f ListFilter) bool {
	if s := strings.TrimSpace(f.Status); s != "" && !strings.EqualFold(wi.Status, s) {
		return false
	}
	if l := strings.TrimSpace(f.Lane); l != "" && !strings.EqualFold(wi.Lane, l) {
		return false
	}
	if p := strings.TrimSpace(f.Project); p != "" && !strings.EqualFold(wi.Project, p) {
		return false
	}
	if r := strings.TrimSpace(f.Risk); r != "" && !strings.EqualFold(wi.RiskLevel, r) {
		return false
	}
	return true
}

// LoadByID busca un WorkItem por su ID (WI-<hash12>) en workitems/ y luego en quarantine/.
func (s *Store) LoadByID(id string) (*WorkItem, string, error) {
	cleanID := strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if !reWorkItemID.MatchString(cleanID) {
		return nil, "", fmt.Errorf("%w: formato de ID invalido %q", ErrWorkItemNotFound, id)
	}

	readyPath := filepath.Join(s.workitemsDir(), cleanID+".json")
	if wi, err := loadWorkItemFile(readyPath); err == nil {
		return wi, readyPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	quarPath := filepath.Join(s.quarantineDir(), cleanID+".json")
	if wi, err := loadWorkItemFile(quarPath); err == nil {
		return wi, quarPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	return nil, "", fmt.Errorf("%w: %s", ErrWorkItemNotFound, cleanID)
}

// LoadQuarantinedByID carga un WorkItem exclusivamente desde runtime/quarantine/<id>.json.
func (s *Store) LoadQuarantinedByID(id string) (*WorkItem, string, error) {
	cleanID := strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if !reWorkItemID.MatchString(cleanID) {
		return nil, "", fmt.Errorf("%w: formato de ID invalido %q", ErrWorkItemNotFound, id)
	}
	quarPath := filepath.Join(s.quarantineDir(), cleanID+".json")
	wi, err := loadWorkItemFile(quarPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("%w en quarantine: %s", ErrWorkItemNotFound, cleanID)
		}
		return nil, "", err
	}
	return wi, quarPath, nil
}

// VerifyRuntime ejecuta la verificacion integral de 5 puntos de intake.exe verify bajo .intake.lock:
//  (a) Cuando existe .git en repoRoot, confirma con git check-ignore que runtime/ esta ignorado (Opcion i).
//  (b) Cero IDs o dedupe_key no vacios duplicados simultaneamente entre workitems/ y quarantine/.
//  (c) Todo archivo .json cumple schema_version "1.0", coincide con su nombre WI-<hash12> y pasa bfcore.ScanG4All.
//  (d) Toda linea en intake_events.jsonl es JSON valido y pasa bfcore.ScanG4All.
//  (e) Preflight read-only de 0_Brain y consulta context --files README.md sin error de parseo en risk_zones.md.
func (s *Store) VerifyRuntime(repoRoot, vaultRoot string, client BrainClient) (VerifyReport, error) {
	report := VerifyReport{
		RepoRoot:   repoRoot,
		DataDir:    s.DataDir,
		VaultPath:  vaultRoot,
		GitIgnored: true,
		Errors:     []string{},
	}

	err := s.withLock(func() error {
		// (a) Git check-ignore cuando estamos dentro de un repositorio Git
		if strings.TrimSpace(repoRoot) != "" {
			gitDir := filepath.Join(repoRoot, ".git")
			if _, statErr := os.Stat(gitDir); statErr == nil {
				probePath := filepath.Join(s.DataDir, ".intake.lock")
				cmd := exec.Command("git", "-C", repoRoot, "check-ignore", "-q", probePath)
				if err := cmd.Run(); err != nil {
					report.GitIgnored = false
					report.Errors = append(report.Errors, fmt.Sprintf("Opcion i violada: %s no esta ignorado por Git (.gitignore)", s.DataDir))
				}
			}
		}

		readyIDs := make(map[string]string)
		readyDedupe := make(map[string]string)

		// (b) & (c) Inspeccionar workitems/
		wEntries, err := os.ReadDir(s.workitemsDir())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, e := range wEntries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
				continue
			}
			report.WorkItemsChecked++
			fullPath := filepath.Join(s.workitemsDir(), e.Name())
			wi, vErrs := verifySingleWorkItemFile(fullPath, "ready")
			if len(vErrs) > 0 {
				report.Errors = append(report.Errors, vErrs...)
				continue
			}
			readyIDs[wi.ID] = fullPath
			if wi.DedupeKey != "" {
				readyDedupe[wi.DedupeKey] = wi.ID
			}
		}

		// Inspeccionar quarantine/
		qEntries, err := os.ReadDir(s.quarantineDir())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, e := range qEntries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
				continue
			}
			report.QuarantineChecked++
			fullPath := filepath.Join(s.quarantineDir(), e.Name())
			wi, vErrs := verifySingleWorkItemFile(fullPath, "needs_clarification")
			if len(vErrs) > 0 {
				report.Errors = append(report.Errors, vErrs...)
				continue
			}
			if prevPath, dup := readyIDs[wi.ID]; dup {
				report.Errors = append(report.Errors, fmt.Sprintf("split-brain: ID %s existe simultaneamente en %s y %s", wi.ID, prevPath, fullPath))
			}
			if wi.DedupeKey != "" {
				if prevID, dup := readyDedupe[wi.DedupeKey]; dup {
					report.Errors = append(report.Errors, fmt.Sprintf("split-brain: dedupe_key %s existe en workitems (%s) y quarantine (%s)", wi.DedupeKey, prevID, wi.ID))
				}
			}
		}

		// (d) Inspeccionar intake_events.jsonl
		if rawEvents, err := os.ReadFile(s.eventsPath()); err == nil {
			scanner := bufio.NewScanner(bytes.NewReader(rawEvents))
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				report.EventsChecked++
				if findings := bfcore.ScanG4All(line); len(findings) > 0 {
					report.Errors = append(report.Errors, fmt.Sprintf("intake_events.jsonl:%d violacion G4 (%s)", lineNum, findings[0].Rule))
				}
				var ev IntakeEvent
				if jErr := json.Unmarshal([]byte(line), &ev); jErr != nil || ev.Event == "" {
					report.Errors = append(report.Errors, fmt.Sprintf("intake_events.jsonl:%d JSON invalido", lineNum))
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			report.Errors = append(report.Errors, fmt.Sprintf("error leyendo intake_events.jsonl: %v", err))
		}

		// (e) Verificar 0_Brain read-only preflight y risk_zones parseables
		if client == nil {
			report.BrainOK = false
			report.Errors = append(report.Errors, "BrainClient es nil")
		} else {
			dbPath := filepath.Join(vaultRoot, ".index", "brain.db")
			if _, pfErr := client.Preflight(vaultRoot, dbPath); pfErr != nil {
				report.BrainOK = false
				report.Errors = append(report.Errors, fmt.Sprintf("0_Brain preflight fallo: %v", pfErr))
			} else {
				ctxResp, _, ctxErr := client.Context(vaultRoot, dbPath, "black-factory", []string{"README.md"})
				if ctxErr != nil {
					report.BrainOK = false
					report.Errors = append(report.Errors, fmt.Sprintf("0_Brain context fallo: %v", ctxErr))
				} else if !ctxResp.HasActiveRiskZones || ctxResp.RiskZonesParseError != "" {
					report.BrainOK = false
					report.Errors = append(report.Errors, fmt.Sprintf("0_Brain risk_zones invalido o ausente: %s", ctxResp.RiskZonesParseError))
				} else {
					report.BrainOK = true
				}
			}
		}

		report.OK = len(report.Errors) == 0 && report.GitIgnored && report.BrainOK
		okBool := report.OK
		now := time.Now().UTC()
		_ = s.appendEventLocked(IntakeEvent{
			Timestamp:         now.Format(time.RFC3339),
			TraceID:           FormatTraceID(now, "verify00"),
			Event:             "VERIFIED",
			OK:                &okBool,
			WorkItemsChecked:  report.WorkItemsChecked,
			QuarantineChecked: report.QuarantineChecked,
			EventsChecked:     report.EventsChecked,
		})
		return nil
	})

	return report, err
}

func verifySingleWorkItemFile(fullPath, expectedStatus string) (*WorkItem, []string) {
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, []string{fmt.Sprintf("leyendo %s: %v", fullPath, err)}
	}
	var errs []string
	if findings := bfcore.ScanG4All(string(raw)); len(findings) > 0 {
		errs = append(errs, fmt.Sprintf("%s contiene violacion G4 (%s en linea %d)", filepath.Base(fullPath), findings[0].Rule, findings[0].Line))
	}
	var wi WorkItem
	if err := json.Unmarshal(raw, &wi); err != nil {
		return nil, append(errs, fmt.Sprintf("%s JSON malformado: %v", filepath.Base(fullPath), err))
	}
	baseID := strings.TrimSuffix(filepath.Base(fullPath), ".json")
	if wi.ID != baseID {
		errs = append(errs, fmt.Sprintf("%s tiene id interno %q != nombre de archivo %q", filepath.Base(fullPath), wi.ID, baseID))
	}
	if wi.Status != expectedStatus {
		errs = append(errs, fmt.Sprintf("%s tiene status %q en directorio de %q", filepath.Base(fullPath), wi.Status, expectedStatus))
	}
	errs = append(errs, ValidateWorkItem(&wi)...)
	return &wi, errs
}

func loadWorkItemFile(path string) (*WorkItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wi WorkItem
	if err := json.Unmarshal(raw, &wi); err != nil {
		return nil, fmt.Errorf("decodificando %s: %w", path, err)
	}
	return &wi, nil
}

func writeWorkItemAtomic(dstPath string, wi *WorkItem) error {
	data, err := json.MarshalIndent(wi, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando WorkItem %s: %w", wi.ID, err)
	}
	data = append(data, '\n')

	if findings := bfcore.ScanG4All(string(data)); len(findings) > 0 {
		return fmt.Errorf("barrera pre-escritura ScanG4All bloqueo %s: regla %s (linea %d)", wi.ID, findings[0].Rule, findings[0].Line)
	}

	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wi-write-*.tmp")
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
	if err := tmp.Sync(); err != nil {
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

func replaceFileAtomic(tmpPath, dstPath string) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lastErr = os.Rename(tmpPath, dstPath)
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, os.ErrPermission) {
			return lastErr
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return lastErr
}

func extractReasonCodes(wi *WorkItem) []string {
	if wi == nil {
		return nil
	}
	if wi.QuarantineInfo != nil && len(wi.QuarantineInfo.ReasonCodes) > 0 {
		return wi.QuarantineInfo.ReasonCodes
	}
	codes := make([]string, 0, len(wi.TriageReasons))
	for _, r := range wi.TriageReasons {
		codes = append(codes, r.Code)
	}
	return codes
}
