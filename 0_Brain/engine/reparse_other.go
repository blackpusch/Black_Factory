//go:build !windows

package brain

import (
	"os"

	"blackfactory/pkg/bfcore"
)

// isReparseOrSymlink delega en bfcore.IsReparseOrSymlink.
func isReparseOrSymlink(fi os.FileInfo) bool {
	return bfcore.IsReparseOrSymlink(fi)
}
