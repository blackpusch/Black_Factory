//go:build !windows

package brain

import (
	"errors"
	"fmt"
	"path/filepath"

	"blackfactory/pkg/bfcore"
)

var (
	// ErrVaultLocked indica que la boveda esta tomada por otro proceso activo (Exit code 4).
	ErrVaultLocked = errors.New("la boveda esta bloqueada por otro proceso activo")
)

// FileLock representa un cerrojo exclusivo a nivel del kernel del SO.
type FileLock struct {
	inner *bfcore.FileLock
}

// AcquireVaultLock adquiere el cerrojo de kernel exclusivo via flock.
func AcquireVaultLock(vaultRoot string) (*FileLock, error) {
	lockPath := filepath.Join(vaultRoot, ".brain.lock")
	fl, err := bfcore.AcquireFileLock(lockPath, 0)
	if err != nil {
		if errors.Is(err, bfcore.ErrLockBusy) {
			return nil, ErrVaultLocked
		}
		return nil, fmt.Errorf("%w: %v", ErrVaultLocked, err)
	}
	return &FileLock{inner: fl}, nil
}

// Unlock libera el cerrojo del kernel cerrando el descriptor sin borrar .brain.lock.
func (l *FileLock) Unlock() error {
	if l == nil || l.inner == nil {
		return nil
	}
	err := l.inner.Unlock()
	l.inner = nil
	return err
}
