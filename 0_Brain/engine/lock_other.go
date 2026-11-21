//go:build !windows

package brain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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

// AcquireVaultLock adquiere el cerrojo de kernel exclusivo via flock.
func AcquireVaultLock(vaultRoot string) (*FileLock, error) {
	lockPath := filepath.Join(vaultRoot, ".brain.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abriendo archivo de lock %s: %w", lockPath, err)
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %v", ErrVaultLocked, err)
	}
	return &FileLock{f: f}, nil
}

// Unlock libera el cerrojo del kernel cerrando el descriptor.
// NO borra .brain.lock de disco para evitar carreras de inodo en POSIX (J1 Enmienda 1).
func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
