package builder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"blackfactory/pkg/bfcore"
)

// Evidence registra la salida literal capturada por el ejecutor.
type Evidence struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated"`
	DurationMs int64  `json:"duration_ms"`
}

// CommandRunner abstrae la ejecucion de VerifyCommands dentro del worktree.
type CommandRunner interface {
	Run(ctx context.Context, dir, command string) (Evidence, error)
}

// ShellRunner ejecuta comandos con el interprete del SO (paridad con GengiSWAP/engine/runner.go).
type ShellRunner struct {
	Timeout time.Duration
}

func (r ShellRunner) Run(ctx context.Context, dir, command string) (Evidence, error) {
	cctx := ctx
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := shellCommand(cctx, command)
	cmd.Dir = dir
	cmd.Env = withUTF8PythonEnv(os.Environ())
	cmd.WaitDelay = 3 * time.Second

	start := time.Now()
	out, err := cmd.CombinedOutput()
	ev := Evidence{
		Command:    command,
		DurationMs: time.Since(start).Milliseconds(),
		Output:     strings.ToValidUTF8(string(out), ""),
	}

	if ctx.Err() != nil {
		return ev, ctx.Err()
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case cctx.Err() != nil:
		ev.ExitCode = -1
		ev.Output += "\n[interrumpido por tiempo limite: " + cctx.Err().Error() + "]"
	case errors.As(err, &ee):
		ev.ExitCode = ee.ExitCode()
	default:
		return ev, err
	}
	return ev, nil
}

func withUTF8PythonEnv(base []string) []string {
	env := make([]string, 0, len(base)+2)
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if ok && (strings.EqualFold(k, "PYTHONUTF8") || strings.EqualFold(k, "PYTHONIOENCODING")) {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
}

// SanitizeCommandOutputG4 redacta el comando, rechaza secretos Tier 1 en la salida cruda,
// redacta Tier 2 antes de truncar estrictamente dentro de maxBytes (8000 B en frontera UTF-8)
// y verifica ScanG4All == 0 antes de devolver el extracto.
func SanitizeCommandOutputG4(rawCmd, rawOut string, maxBytes int) (sanitizedCmd, excerpt string, truncated bool, tier1 *bfcore.G4Finding, err error) {
	sanitizedCmd = bfcore.RedactAllG4(rawCmd)
	validOut := strings.ToValidUTF8(rawOut, "")

	if hits := bfcore.ScanTier1("command_output", validOut, false); len(hits) > 0 {
		f := hits[0]
		return sanitizedCmd, "", false, &f, nil
	}

	redacted, _ := bfcore.RedactTier2Text("command_output", validOut)
	if maxBytes <= 0 {
		maxBytes = MaxCommandOutputBytes
	}

	const marker = "\n…[truncado]…\n" // 18 bytes UTF-8
	excerpt = redacted
	if len(redacted) > maxBytes {
		truncated = true
		if maxBytes <= len(marker)+2 {
			excerpt = trimUTF8Prefix(redacted, maxBytes)
		} else {
			avail := maxBytes - len(marker)
			half := avail / 2
			head := trimUTF8Prefix(redacted, half)
			tailBudget := avail - len(head)
			tail := trimUTF8Suffix(redacted, tailBudget)
			excerpt = head + marker + tail
		}
	}

	if hits := bfcore.ScanG4All(excerpt); len(hits) > 0 {
		return sanitizedCmd, "", truncated, nil, fmt.Errorf("%w: extracto residual no supero ScanG4All (%s)", ErrG4Violation, hits[0].Rule)
	}
	return sanitizedCmd, excerpt, truncated, nil, nil
}

func trimUTF8Prefix(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	n := maxBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func trimUTF8Suffix(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}
