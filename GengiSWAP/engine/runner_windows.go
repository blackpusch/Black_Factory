//go:build windows

package ring

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	comspec := strings.TrimSpace(os.Getenv("COMSPEC"))
	if comspec == "" {
		comspec = "cmd.exe"
	}
	trimmed := strings.TrimSpace(command)
	cmd := exec.CommandContext(ctx, comspec)
	// /S retira únicamente la primera y última comilla envolvente, evitando que
	// syscall.EscapeArg convierta comillas internas en \" y rompa rutas con espacios.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `"` + comspec + `" /S /C "` + trimmed + `"`,
	}
	return cmd
}
