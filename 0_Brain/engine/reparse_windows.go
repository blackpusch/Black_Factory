//go:build windows

package brain

import (
	"os"
	"syscall"
)

const fileAttributeReparsePoint = 0x00000400

// isReparseOrSymlink comprueba si un os.FileInfo corresponde a un enlace simbolico,
// directory junction NTFS u otro reparse point.
// En Go 1.23+, los junctions de Windows se marcan con os.ModeIrregular en lugar de
// os.ModeSymlink. Esta funcion verifica tanto los flags de modo como los atributos Win32 nativos.
func isReparseOrSymlink(fi os.FileInfo) bool {
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
