//go:build !windows

package bfcore

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// ErrLockBusy indica que el archivo de lock permanece ocupado tras agotar maxWait.
var ErrLockBusy = errors.New("archivo de lock ocupado por otro proceso activo")

// FileLock representa un cerrojo exclusivo a nivel del kernel del SO.
type FileLock struct {
	f *os.File
}

// AcquireFileLock adquiere un cerrojo exclusivo sobre lockPath usando flock,
// reintentando cada 25ms hasta maxWait antes de retornar ErrLockBusy.
func AcquireFileLock(lockPath string, maxWait time.Duration) (*FileLock, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("abriendo archivo de lock %s: %w", lockPath, err)
	}

	deadline := time.Now().Add(maxWait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &FileLock{f: f}, nil
		}
		if maxWait <= 0 || time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: %v", ErrLockBusy, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// Unlock libera el cerrojo del kernel cerrando el descriptor sin eliminar el archivo de lock.
func (l *FileLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
