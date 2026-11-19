package ring

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Evidence es la salida literal de un comando de verificación. La captura el
// motor, nunca un modelo: así "pasó" es un hecho y no una afirmación.
type Evidence struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated"`
	DurationMs int64  `json:"duration_ms"`
}

type CommandRunner interface {
	// Run devuelve error solo si no pudo ejecutar el comando (o el contexto padre
	// se canceló). Un código de salida distinto de 0 es evidencia, no un error.
	Run(ctx context.Context, command string) (Evidence, error)
}

// ShellRunner ejecuta comandos con el intérprete del sistema (cmd en Windows, sh en el resto).
type ShellRunner struct {
	Dir       string
	Timeout   time.Duration // 0 = sin límite
	MaxOutput int           // bytes conservados (mitad inicio, mitad final); 0 = 8000
}

func (r ShellRunner) Run(ctx context.Context, command string) (Evidence, error) {
	cctx := ctx
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := shellCommand(cctx, command)
	cmd.Dir = r.Dir
	cmd.Env = withUTF8PythonEnv(os.Environ())
	cmd.WaitDelay = 3 * time.Second

	start := time.Now()
	out, err := cmd.CombinedOutput()
	ev := Evidence{Command: command, DurationMs: time.Since(start).Milliseconds()}

	text := strings.ToValidUTF8(string(out), "")
	max := r.MaxOutput
	if max <= 0 {
		max = 8000
	}
	if len(text) > max {
		text = strings.ToValidUTF8(text[:max/2], "") + "\n…[truncado]…\n" + strings.ToValidUTF8(text[len(text)-max/2:], "")
		ev.Truncated = true
	}
	ev.Output = text

	if ctx.Err() != nil {
		return ev, ctx.Err() // pausa o cancelación del anillo: no es evidencia
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case cctx.Err() != nil: // venció el Timeout propio
		ev.ExitCode = -1
		ev.Output += "\n[interrumpido por tiempo límite: " + cctx.Err().Error() + "]"
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

// ---------------------------------------------------------------- workspace

// Workspace abstrae el repositorio de destino. Nunca destruye historial:
// la reversión se hace con commits de reversión, no con reset --hard ni clean.
type Workspace interface {
	// Head devuelve la referencia actual ("" si no hay repositorio).
	Head(ctx context.Context) (string, error)
	// ChangedSince lista los archivos cambiados desde base: commits posteriores
	// y cambios aún sin commitear.
	ChangedSince(ctx context.Context, base string) ([]string, error)
	// Commit crea UN commit con los cambios pendientes de exactamente esos archivos
	// (si no hay cambios pendientes no crea nada) y devuelve la referencia resultante.
	Commit(ctx context.Context, msg string, files []string) (string, error)
	// Revert deshace todo lo posterior a base sin borrar historial.
	Revert(ctx context.Context, base string) error
}

// NopWorkspace sirve para tareas de solo lectura o sin repositorio.
type NopWorkspace struct{}

func (NopWorkspace) Head(context.Context) (string, error)                     { return "", nil }
func (NopWorkspace) ChangedSince(context.Context, string) ([]string, error)   { return nil, nil }
func (NopWorkspace) Commit(context.Context, string, []string) (string, error) { return "", nil }
func (NopWorkspace) Revert(context.Context, string) error                     { return nil }

// GitWorkspace usa git.
//   - Commit: git add -A -- <archivos> + git commit -- <archivos>; nunca "git add ." ni "add -A" sin ruta.
//   - Revert: los cambios sin commitear se guardan con git stash (recuperables) y los
//     commits posteriores a base se deshacen con git revert --no-edit. Nunca reset --hard ni clean.
type GitWorkspace struct{ Dir string }

func (g GitWorkspace) git(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func parsePorcelainZ(b []byte) []string {
	parts := strings.Split(string(b), "\x00")
	var files []string
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		files = append(files, p[3:])
		if strings.ContainsAny(p[:2], "RC") && i+1 < len(parts) { // renombrado/copia: sigue la ruta de origen
			i++
			if parts[i] != "" {
				files = append(files, parts[i])
			}
		}
	}
	return files
}

func (g GitWorkspace) dirty(ctx context.Context) ([]string, error) {
	out, err := g.git(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parsePorcelainZ(out), nil
}

func (g GitWorkspace) Head(ctx context.Context) (string, error) {
	out, err := g.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g GitWorkspace) ChangedSince(ctx context.Context, base string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	add := func(f string) {
		if f != "" && !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	d, err := g.dirty(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range d {
		add(f)
	}
	if base != "" {
		out, err := g.git(ctx, "diff", "--name-only", "-z", base, "HEAD")
		if err != nil {
			return nil, err
		}
		for _, f := range strings.Split(string(out), "\x00") {
			add(f)
		}
	}
	return files, nil
}

func (g GitWorkspace) Commit(ctx context.Context, msg string, files []string) (string, error) {
	d, err := g.dirty(ctx)
	if err != nil {
		return "", err
	}
	pending := map[string]bool{}
	for _, f := range d {
		pending[f] = true
	}
	var todo []string
	for _, f := range files {
		if pending[f] {
			todo = append(todo, f)
		}
	}
	if len(todo) > 0 {
		if _, err := g.git(ctx, append([]string{"add", "-A", "--"}, todo...)...); err != nil {
			return "", err
		}
		staged, err := g.git(ctx, append([]string{"diff", "--cached", "--name-only", "--"}, todo...)...)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(staged)) != "" {
			if _, err := g.git(ctx, append([]string{"commit", "-m", msg, "--"}, todo...)...); err != nil {
				return "", err
			}
		}
	}
	return g.Head(ctx)
}

func (g GitWorkspace) Revert(ctx context.Context, base string) error {
	if base == "" {
		return nil
	}
	d, err := g.dirty(ctx)
	if err != nil {
		return err
	}
	if len(d) > 0 {
		if _, err := g.git(ctx, "stash", "push", "--include-untracked", "-m", "gengiswap: cambios sin commitear descartados"); err != nil {
			return err
		}
	}
	cnt, err := g.git(ctx, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(cnt)) == "0" {
		return nil
	}
	if _, err := g.git(ctx, "revert", "--no-edit", base+"..HEAD"); err != nil {
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = g.git(abortCtx, "revert", "--abort")
		return err
	}
	return nil
}
