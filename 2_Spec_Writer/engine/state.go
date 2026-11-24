package specwriter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

const defaultLockWait = 250 * time.Millisecond

var (
	ErrLockBusy        = errors.New("cerrojo de archivo ocupado tras backoff")
	ErrAttemptsMissing = errors.New("attempts.json ausente")
	ErrAttemptsCorrupt = errors.New("attempts.json corrupto o invalido")
	ErrG4Violation     = errors.New("violacion G4 detectada")
)

// AcquireWorkItemLock adquiere el cerrojo granular <wiDir>/.wi.lock con backoff acotado de 250 ms.
func AcquireWorkItemLock(wiDir string) (*bfcore.FileLock, error) {
	lockPath := filepath.Join(wiDir, ".wi.lock")
	if err := bfcore.CheckNoReparseChain(wiDir, lockPath); err != nil {
		return nil, err
	}
	lock, err := bfcore.AcquireFileLock(lockPath, defaultLockWait)
	if err != nil {
		if errors.Is(err, bfcore.ErrLockBusy) {
			return nil, fmt.Errorf("%w: %v", ErrLockBusy, err)
		}
		return nil, err
	}
	return lock, nil
}

// AcquireCompileLock adquiere el cerrojo estatico <taskRoot>/.compile.lock con backoff acotado de 250 ms.
func AcquireCompileLock(taskRoot string) (*bfcore.FileLock, error) {
	lockPath := filepath.Join(taskRoot, ".compile.lock")
	if err := bfcore.CheckNoReparseChain(taskRoot, lockPath); err != nil {
		return nil, err
	}
	lock, err := bfcore.AcquireFileLock(lockPath, defaultLockWait)
	if err != nil {
		if errors.Is(err, bfcore.ErrLockBusy) {
			return nil, fmt.Errorf("%w: %v", ErrLockBusy, err)
		}
		return nil, err
	}
	return lock, nil
}

// WriteJSONAtomicG4 serializa v como JSON identado, verifica bfcore.ScanG4All == 0 en memoria
// y escribe de forma atomica mediante archivo temporal hermano + Sync + os.Rename con reintentos Windows.
func WriteJSONAtomicG4(dstPath string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando JSON para %s: %w", filepath.Base(dstPath), err)
	}
	raw = append(raw, '\n')
	if findings := bfcore.ScanG4All(string(raw)); len(findings) > 0 {
		return fmt.Errorf("%w: regla %s (linea %d) al escribir %s", ErrG4Violation, findings[0].Rule, findings[0].Line, filepath.Base(dstPath))
	}
	return writeFileAtomicBytes(dstPath, raw)
}

func writeFileAtomicBytes(dstPath string, data []byte) error {
	dir := filepath.Dir(dstPath)
	if err := bfcore.CheckNoReparseUpToNearestExisting("", dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := bfcore.CheckNoReparseChain(dir, dstPath); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sw-*.tmp")
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
	if err := renameWithWindowsRetry(tmpPath, dstPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func renameWithWindowsRetry(src, dst string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := os.Rename(src, dst); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return lastErr
}

// ReadFileWithShortRetry lee un archivo en modo estrictamente de solo lectura con 3 reintentos de 25 ms
// ante bloqueos momentaneos en Windows; retorna inmediatamente ante os.ErrNotExist.
func ReadFileWithShortRetry(path string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		b, err := os.ReadFile(path)
		if err == nil {
			return b, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
		time.Sleep(25 * time.Millisecond)
	}
	return nil, lastErr
}

// LoadAttemptsStrict carga y valida estrictamente <wiDir>/attempts.json en modo fail-closed.
func LoadAttemptsStrict(wiDir string) (AttemptsState, error) {
	attemptsPath := filepath.Join(wiDir, "attempts.json")
	if err := bfcore.CheckNoReparseChain(wiDir, attemptsPath); err != nil {
		return AttemptsState{}, err
	}
	raw, err := ReadFileWithShortRetry(attemptsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AttemptsState{}, fmt.Errorf("%w: %s", ErrAttemptsMissing, filepath.Base(attemptsPath))
		}
		return AttemptsState{}, err
	}
	return parseAttemptsBytesStrict(raw)
}

func parseAttemptsBytesStrict(raw []byte) (AttemptsState, error) {
	var st AttemptsState
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&st); err != nil {
		return AttemptsState{}, fmt.Errorf("%w: %v", ErrAttemptsCorrupt, err)
	}
	if dec.More() {
		return AttemptsState{}, fmt.Errorf("%w: contenido JSON adicional inesperado", ErrAttemptsCorrupt)
	}
	if st.SchemaVersion != SchemaVersion ||
		strings.TrimSpace(st.WIID) == "" ||
		strings.TrimSpace(st.Project) == "" ||
		st.DraftAttempts < 0 ||
		st.FinalAttempts < 0 ||
		st.ExternalCycles < 0 {
		return AttemptsState{}, fmt.Errorf("%w: campos de attempts.json fuera de contrato", ErrAttemptsCorrupt)
	}
	if st.LastPassedStage != "" && st.LastPassedStage != "draft" && st.LastPassedStage != "final" {
		return AttemptsState{}, fmt.Errorf("%w: last_passed_stage invalido %q", ErrAttemptsCorrupt, st.LastPassedStage)
	}
	return st, nil
}

// AppendEvent sanitiza con RedactAllG4, verifica ScanG4All == 0 en memoria y anexa una linea JSONL
// a <dataDir>/specwriter_events.jsonl bajo el cerrojo breve <dataDir>/.events.lock.
func AppendEvent(dataDir string, ev SpecWriterEvent) error {
	if strings.TrimSpace(dataDir) == "" {
		return errors.New("dataDir vacio en AppendEvent")
	}
	if err := bfcore.CheckNoReparseUpToNearestExisting("", dataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	eventsPath := filepath.Join(dataDir, "specwriter_events.jsonl")
	lockPath := filepath.Join(dataDir, ".events.lock")
	if err := bfcore.CheckNoReparseChain(dataDir, eventsPath); err != nil {
		return err
	}
	if err := bfcore.CheckNoReparseChain(dataDir, lockPath); err != nil {
		return err
	}

	if strings.TrimSpace(ev.Timestamp) == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	ev.Event = bfcore.RedactAllG4(ev.Event)
	ev.TraceID = bfcore.RedactAllG4(ev.TraceID)
	ev.Project = bfcore.RedactAllG4(ev.Project)
	ev.WorkItemID = bfcore.RedactAllG4(ev.WorkItemID)
	ev.Stage = bfcore.RedactAllG4(ev.Stage)
	ev.Reason = bfcore.RedactAllG4(ev.Reason)
	ev.SpecSHA256 = bfcore.RedactAllG4(ev.SpecSHA256)
	ev.Detail = bfcore.RedactAllG4(ev.Detail)

	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if findings := bfcore.ScanG4All(string(line)); len(findings) > 0 {
		return fmt.Errorf("%w: evento JSONL no supero ScanG4All (regla %s)", ErrG4Violation, findings[0].Rule)
	}

	lock, err := bfcore.AcquireFileLock(lockPath, defaultLockWait)
	if err != nil {
		if errors.Is(err, bfcore.ErrLockBusy) {
			return fmt.Errorf("%w: %v", ErrLockBusy, err)
		}
		return err
	}
	defer func() { _ = lock.Unlock() }()

	f, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ScanRawAndDecodedG4 ejecuta la barrera G4 de doble fase (11 reglas, fail-closed sin redaccion Tier 2):
//   - Fase-A sobre bytes crudos (antes de parsear JSON).
//   - Parseo JSON (si falla sintaxis tras superar Fase-A, devuelve nil, jsonErr).
//   - Fase-B recursiva sobre todas las claves y strings JSON decodificados (captura escapes Unicode \u0067hp_...).
func ScanRawAndDecodedG4(raw []byte) ([]bfcore.G4Finding, error) {
	cleaned := bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if findings := bfcore.ScanG4All(string(cleaned)); len(findings) > 0 {
		return findings, nil
	}
	var decoded any
	if err := json.Unmarshal(cleaned, &decoded); err != nil {
		return nil, err
	}
	var decodedFindings []bfcore.G4Finding
	walkJSONStringsForG4(decoded, "", &decodedFindings)
	if len(decodedFindings) > 0 {
		return decodedFindings, nil
	}
	return nil, nil
}

func walkJSONStringsForG4(v any, ptr string, out *[]bfcore.G4Finding) {
	switch t := v.(type) {
	case string:
		if hits := bfcore.ScanG4All(t); len(hits) > 0 {
			for _, h := range hits {
				h.Field = ptr
				*out = append(*out, h)
			}
		}
	case []any:
		for i, elem := range t {
			walkJSONStringsForG4(elem, fmt.Sprintf("%s/%d", ptr, i), out)
		}
	case map[string]any:
		for k, val := range t {
			if hits := bfcore.ScanG4All(k); len(hits) > 0 {
				for _, h := range hits {
					h.Field = ptr
					*out = append(*out, h)
				}
			}
			walkJSONStringsForG4(val, ptr+"/"+escapeJSONPointer(k), out)
		}
	}
}

func escapeJSONPointer(token string) string {
	s := strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}
