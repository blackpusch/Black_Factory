//go:build windows

package bfcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const fileAttributeReparsePoint = 0x00000400

// ErrReparsePoint indica que se detecto un symlink, junction o reparse point prohibido.
var ErrReparsePoint = errors.New("enlace simbolico o reparse point detectado en la cadena de ruta")

// IsReparseOrSymlink comprueba si un os.FileInfo corresponde a un enlace simbolico,
// directory junction NTFS u otro reparse point (incluyendo os.ModeIrregular en Go 1.23+
// y FILE_ATTRIBUTE_REPARSE_POINT 0x00000400 en Win32).
func IsReparseOrSymlink(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}
	if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return true
	}
	if data, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		if data.FileAttributes&fileAttributeReparsePoint != 0 {
			return true
		}
	}
	return false
}

// CheckNoReparseChain recorre con os.Lstat cada componente ancestro existente desde rootPath
// (o desde el volumen raiz si rootPath esta vacio o no contiene a targetPath) hasta targetPath
// y rechaza cualquier symlink, junction NTFS o punto de montaje.
func CheckNoReparseChain(rootPath, targetPath string) error {
	if strings.TrimSpace(targetPath) == "" {
		return errors.New("ruta objetivo vacia")
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	var absRoot string
	if strings.TrimSpace(rootPath) != "" {
		absRoot, err = filepath.Abs(rootPath)
		if err != nil {
			return err
		}
		if err := checkSinglePathNoReparse(absRoot); err != nil {
			return err
		}
	}

	// Recolectar la cadena de ancestros desde absTarget hacia arriba hasta absRoot (o raiz de volumen)
	var chain []string
	cur := filepath.Clean(absTarget)
	for {
		chain = append(chain, cur)
		if absRoot != "" && strings.EqualFold(cur, filepath.Clean(absRoot)) {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	for i := len(chain) - 1; i >= 0; i-- {
		if err := checkSinglePathNoReparse(chain[i]); err != nil {
			return err
		}
	}
	return nil
}

func checkSinglePathNoReparse(p string) error {
	vol := filepath.VolumeName(p)
	if vol != "" && (strings.EqualFold(p, vol) || strings.EqualFold(p, vol+`\`)) {
		return nil
	}
	fi, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if IsReparseOrSymlink(fi) {
		return fmt.Errorf("%w: %s", ErrReparsePoint, p)
	}
	return nil
}
