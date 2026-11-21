//go:build !windows

package brain

import "os"

// isReparseOrSymlink comprueba si un os.FileInfo corresponde a un enlace simbolico.
func isReparseOrSymlink(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}
	return fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}
