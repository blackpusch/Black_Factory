//go:build !windows

package bfcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrReparsePoint indica que se detecto un symlink o nodo irregular prohibido.
var ErrReparsePoint = errors.New("enlace simbolico o reparse point detectado en la cadena de ruta")

// IsReparseOrSymlink comprueba si un os.FileInfo corresponde a un enlace simbolico.
func IsReparseOrSymlink(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}
	return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}

// CheckNoReparseChain recorre con os.Lstat cada componente ancestro existente desde rootPath
// hasta targetPath y rechaza cualquier symlink.
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

	var chain []string
	cur := filepath.Clean(absTarget)
	for {
		chain = append(chain, cur)
		if absRoot != "" && cur == filepath.Clean(absRoot) {
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
	if p == "/" {
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
