package builder

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"blackfactory/pkg/bfcore"
)

const defaultLockWait = 250 * time.Millisecond

var (
	ErrLockBusy       = errors.New("cerrojo de archivo ocupado tras backoff")
	ErrSessionMissing = errors.New("session.json ausente")
	ErrSessionCorrupt = errors.New("session.json corrupto o invalido")
	ErrG4Violation    = errors.New("violacion G4 detectada")
)

// AcquireSessionLock adquiere el cerrojo granular <taskStateDir>/.session.lock con backoff acotado de 250 ms.
func AcquireSessionLock(taskStateDir string) (*bfcore.FileLock, error) {
	if err := bfcore.CheckNoReparseUpToNearestExisting("", taskStateDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(taskStateDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(taskStateDir, ".session.lock")
	if err := bfcore.CheckNoReparseChain(taskStateDir, lockPath); err != nil {
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

// AcquireRepoLock adquiere el cerrojo por repositorio <dataDir>/.repo-<repoSHA16>.lock
// respetando el orden estricto .session.lock -> .repo-<sha16>.lock -> .events.lock.
func AcquireRepoLock(dataDir, repoSHA256 string) (*bfcore.FileLock, error) {
	if err := bfcore.CheckNoReparseUpToNearestExisting("", dataDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	short := repoSHA256
	if len(short) > 16 {
		short = short[:16]
	}
	if short == "" {
		short = "default"
	}
	lockPath := filepath.Join(dataDir, fmt.Sprintf(".repo-%s.lock", short))
	if err := bfcore.CheckNoReparseChain(dataDir, lockPath); err != nil {
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

// ScanRawAndDecodedG4 ejecuta la barrera G4 de doble fase (11 reglas, fail-closed sin redaccion Tier 2):
//   - Fase-A sobre bytes crudos (sin UTF-8 BOM).
//   - Parseo JSON (si falla sintaxis tras pasar Fase-A, devuelve nil, jsonErr).
//   - Fase-B recursiva sobre todas las claves y strings JSON decodificados (captura escapes \uXXXX).
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

func sanitizeG4Pointer(rawPtr string) string {
	if rawPtr == "" {
		return "/"
	}
	if len(bfcore.ScanG4All(rawPtr)) > 0 {
		return "/redacted_pointer"
	}
	return rawPtr
}

// WriteFileAtomicBytes escribe data en dstPath de forma atomica (temp hermano + Sync + Close + Rename).
func WriteFileAtomicBytes(dstPath string, data []byte) error {
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
	tmp, err := os.CreateTemp(dir, ".bf-*.tmp")
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

// WriteJSONAtomicG4 serializa v como JSON indentado, verifica ScanRawAndDecodedG4 == 0 en memoria
// y escribe de forma atomica.
func WriteJSONAtomicG4(dstPath string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando JSON para %s: %w", filepath.Base(dstPath), err)
	}
	raw = append(raw, '\n')
	findings, jsonErr := ScanRawAndDecodedG4(raw)
	if jsonErr != nil {
		return fmt.Errorf("validando JSON serializado de %s: %w", filepath.Base(dstPath), jsonErr)
	}
	if len(findings) > 0 {
		return fmt.Errorf("%w: regla %s (linea %d) al escribir %s", ErrG4Violation, findings[0].Rule, findings[0].Line, filepath.Base(dstPath))
	}
	return WriteFileAtomicBytes(dstPath, raw)
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

// ComputePathIdentity deriva basename saneado (sin G4) y SHA-256 sobre ruta absoluta normalizada
// sin persistir rutas absolutas de host en runtime/.
func ComputePathIdentity(rawPath string) (basename string, sha256Hex string, absClean string, err error) {
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "."
	}
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", "", "", err
	}
	absClean = filepath.Clean(abs)
	normSlash := strings.ToLower(strings.TrimRight(filepath.ToSlash(absClean), "/"))
	sum := sha256.Sum256([]byte(normSlash))
	base := filepath.Base(absClean)
	parentBase := strings.ToLower(filepath.Base(filepath.Dir(absClean)))
	if base == "" || len(bfcore.ScanG4All(base)) > 0 ||
		(bfcore.MatchesPersonalPathG4(absClean) && (parentBase == "users" || parentBase == "home")) {
		base = "dir"
	}
	return base, hex.EncodeToString(sum[:]), absClean, nil
}

// LoadSessionStrict carga y valida estrictamente <taskStateDir>/session.json en modo fail-closed.
func LoadSessionStrict(taskStateDir string) (SessionState, error) {
	sessionPath := filepath.Join(taskStateDir, "session.json")
	if err := bfcore.CheckNoReparseChain("", sessionPath); err != nil {
		return SessionState{}, err
	}
	raw, err := ReadFileWithShortRetry(sessionPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionState{}, fmt.Errorf("%w: %s", ErrSessionMissing, filepath.Base(sessionPath))
		}
		return SessionState{}, err
	}
	return parseSessionBytesStrict(raw)
}

func parseSessionBytesStrict(raw []byte) (SessionState, error) {
	cleaned := bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	var st SessionState
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&st); err != nil {
		return SessionState{}, fmt.Errorf("%w: %v", ErrSessionCorrupt, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return SessionState{}, fmt.Errorf("%w: contenido JSON adicional inesperado", ErrSessionCorrupt)
	}
	if st.SchemaVersion != SchemaVersion ||
		strings.TrimSpace(st.TaskID) == "" ||
		strings.TrimSpace(st.BuildBranch) == "" ||
		strings.TrimSpace(st.RepoRootSHA256) == "" ||
		strings.TrimSpace(st.WorktreePathSHA256) == "" ||
		st.ActiveAttempt < 0 ||
		st.ResetEpoch < 0 ||
		st.BounceCycle < 0 ||
		st.BounceCycle > MaxBounceCycles ||
		(st.WorktreeStatus != "active" && st.WorktreeStatus != "removed") {
		return SessionState{}, fmt.Errorf("%w: campos de session.json fuera de contrato", ErrSessionCorrupt)
	}
	return st, nil
}

// AppendEvent sanitiza con RedactAllG4, verifica ScanG4All == 0 en memoria y anexa una linea JSONL
// a <dataDir>/builder_events.jsonl bajo el cerrojo breve <dataDir>/.events.lock.
func AppendEvent(dataDir string, ev BuilderEvent) error {
	if strings.TrimSpace(dataDir) == "" {
		return errors.New("dataDir vacio en AppendEvent")
	}
	if err := bfcore.CheckNoReparseUpToNearestExisting("", dataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	eventsPath := filepath.Join(dataDir, "builder_events.jsonl")
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
	ev.TaskID = bfcore.RedactAllG4(ev.TaskID)
	ev.ChunkID = bfcore.RedactAllG4(ev.ChunkID)
	ev.Reason = bfcore.RedactAllG4(ev.Reason)
	ev.BundleSHA256 = bfcore.RedactAllG4(ev.BundleSHA256)
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

	f, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
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
