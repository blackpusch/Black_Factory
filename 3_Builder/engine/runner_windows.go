//go:build windows

package builder

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
	// /S retira unicamente la primera y ultima comilla envolvente (paridad GengiSWAP runner_windows.go:20-24).
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `"` + comspec + `" /S /C "` + trimmed + `"`,
	}
	return cmd
}
