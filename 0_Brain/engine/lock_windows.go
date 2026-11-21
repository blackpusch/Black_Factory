//go:build windows

package brain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

var (
	// ErrVaultLocked indica que la boveda esta tomada por otro proceso activo (Exit code 4).
	ErrVaultLocked = errors.New("la boveda esta bloqueada por otro proceso activo")
)

// FileLock representa un cerrojo exclusivo a nivel del kernel del SO.
// Persiste en disco (.brain.lock) y se libera cerrando el descriptor de archivo.
// Los recursos compartidos en red (SMB/NFS) no estan soportados.
type FileLock struct {
	f *os.File
}

// AcquireVaultLock adquiere el cerrojo de kernel exclusivo sobre .brain.lock en vaultRoot.
// Si otro proceso tiene el lock, devuelve inmediatamente ErrVaultLocked (Fail Immediately).
func AcquireVaultLock(vaultRoot string) (*FileLock, error) {
	lockPath := filepath.Join(vaultRoot, ".brain.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abriendo archivo de lock %s: %w", lockPath, err)
	}

	var ol windows.Overlapped
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrVaultLocked
		}
		return nil, fmt.Errorf("%w: %v", ErrVaultLocked, err)
	}
	return &FileLock{f: f}, nil
}

// Unlock libera el cerrojo del kernel cerrando el handle.
// NO borra .brain.lock de disco para evitar carreras en sistemas de archivos (J1 Enmienda 1).
func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	var ol windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &ol)
	err := l.f.Close()
	l.f = nil
	return err
}
