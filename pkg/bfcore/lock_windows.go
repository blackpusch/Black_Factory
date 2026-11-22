//go:build windows

package bfcore

import (
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// ErrLockBusy indica que el archivo de lock permanece ocupado tras agotar maxWait.
var ErrLockBusy = errors.New("archivo de lock ocupado por otro proceso activo")

// FileLock representa un cerrojo exclusivo a nivel del kernel del SO.
// Persiste en disco y se libera cerrando el descriptor sin eliminar el archivo.
type FileLock struct {
	f *os.File
}

// AcquireFileLock adquiere un cerrojo exclusivo sobre lockPath usando LockFileEx,
// reintentando cada 25ms hasta maxWait unicamente ante ERROR_LOCK_VIOLATION antes de retornar ErrLockBusy.
// Otros errores del SO (permisos, ruta invalida) se devuelven de inmediato sin envolver en ErrLockBusy.
func AcquireFileLock(lockPath string, maxWait time.Duration) (*FileLock, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abriendo archivo de lock %s: %w", lockPath, err)
	}

	deadline := time.Now().Add(maxWait)
	for {
		var ol windows.Overlapped
		err = windows.LockFileEx(
			windows.Handle(f.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, &ol,
		)
		if err == nil {
			return &FileLock{f: f}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			_ = f.Close()
			return nil, fmt.Errorf("bloqueando archivo de lock %s: %w", lockPath, err)
		}
		if maxWait <= 0 || time.Now().After(deadline) {
			_ = f.Close()
			return nil, ErrLockBusy
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Unlock libera el cerrojo del kernel cerrando el handle sin eliminar el archivo de lock.
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
