package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gengiswap/ring"
)

type optBool struct {
	set bool
	val bool
}

func (b *optBool) String() string { return strconv.FormatBool(b.val) }
func (b *optBool) IsBoolFlag() bool {
	return true
}
func (b *optBool) Set(s string) error {
	v, err := strconv.ParseBool(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	b.set = true
	b.val = v
	return nil
}

type optString struct {
	set bool
	val string
}

func (o *optString) String() string { return o.val }
func (o *optString) Set(s string) error {
	o.set = true
	o.val = s
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(out)
		if len(args) == 0 {
			return errors.New("se requiere un subcomando (plan | check | decide | status)")
		}
		return nil
	}

	sub := args[0]
	rest := args[1:]

	var rootFlag string
	var qaPass optBool
	var qaNotes optString

	fs := flag.NewFlagSet("gengis "+sub, flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&rootFlag, "root", "", "Raíz de GengiSWAP")
	if sub == "decide" {
		fs.Var(&qaPass, "qa-pass", "Resultado QA del Preparador (true|false)")
		fs.Var(&qaNotes, "qa-notes", "Notas QA del Preparador")
	}

	reordered := reorderFlags(rest, map[string]bool{"qa-pass": true}, map[string]bool{"root": true, "qa-notes": true})
	if err := fs.Parse(reordered); err != nil {
		return err
	}
	pos := fs.Args()
	if len(pos) != 1 || strings.TrimSpace(pos[0]) == "" {
		printUsage(errOut)
		return fmt.Errorf("uso: gengis %s <TAREA_ID>", sub)
	}

	gengisRoot, taskDir, err := resolvePaths(rootFlag, pos[0])
	if err != nil {
		return err
	}

	cfg := ring.CLIConfig{
		GengisRoot: gengisRoot,
		TaskDir:    taskDir,
		In:         in,
		Out:        out,
	}
	ctx := context.Background()

	switch sub {
	case "plan":
		return ring.PlanTask(ctx, cfg)
	case "check":
		return ring.CheckChunk(ctx, cfg)
	case "decide":
		opts := ring.DecideOptions{
			HasQAPass:  qaPass.set,
			QAPass:     qaPass.val,
			HasQANotes: qaNotes.set,
			QANotes:    qaNotes.val,
		}
		return ring.DecideChunk(ctx, cfg, opts)
	case "status":
		return ring.ShowStatus(cfg)
	default:
		printUsage(errOut)
		return fmt.Errorf("subcomando desconocido: %q", sub)
	}
}

// reorderFlags mueve banderas antes de los argumentos posicionales para aceptar
// tanto `gengis decide T1 --qa-pass=false` como `gengis decide --qa-pass=false T1`.
func reorderFlags(args []string, boolFlags, valFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			flags = append(flags, a)
			continue
		}
		if valFlags[name] && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i++
			continue
		}
		if boolFlags[name] && i+1 < len(args) {
			low := strings.ToLower(args[i+1])
			if low == "true" || low == "false" {
				flags = append(flags, a+"="+low)
				i++
				continue
			}
		}
		flags = append(flags, a)
	}
	return append(flags, pos...)
}

func resolvePaths(rootOverride, tareaArg string) (gengisRoot, taskDir string, err error) {
	rootTrim := strings.TrimSpace(rootOverride)
	envRoot := strings.TrimSpace(os.Getenv("GENGIS_ROOT"))
	noRoot := rootTrim == "" && envRoot == ""

	if rootTrim != "" {
		gengisRoot, err = filepath.Abs(rootOverride)
		if err != nil {
			return "", "", err
		}
	} else if envRoot != "" {
		gengisRoot, err = filepath.Abs(envRoot)
		if err != nil {
			return "", "", err
		}
	} else {
		gengisRoot = detectGengisRoot()
	}

	if looksLikeDirectTaskDir(tareaArg) {
		taskDir, err = filepath.Abs(tareaArg)
		if err != nil {
			return "", "", err
		}
		if noRoot {
			parent := filepath.Dir(filepath.Dir(taskDir))
			if isGengisRoot(parent) {
				gengisRoot = parent
			}
		}
		return gengisRoot, taskDir, nil
	}

	taskDir = filepath.Join(gengisRoot, "gengis_task", tareaArg)
	return gengisRoot, taskDir, nil
}

func looksLikeDirectTaskDir(arg string) bool {
	if strings.ContainsAny(arg, `/\`) {
		return true
	}
	if info, err := os.Stat(filepath.Join(arg, "index.md")); err == nil && !info.IsDir() {
		return true
	}
	if info, err := os.Stat(filepath.Join(arg, ".ring")); err == nil && info.IsDir() {
		return true
	}
	return false
}

func detectGengisRoot() string {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		cur := start
		for {
			if isGengisRoot(cur) {
				return cur
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	if len(starts) > 0 {
		return starts[0]
	}
	return "."
}

func isGengisRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "gengis_global_log.md")); err == nil {
		return true
	}
	if info, err := os.Stat(filepath.Join(dir, "gengis_task")); err == nil && info.IsDir() {
		return true
	}
	return false
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Uso: gengis <subcomando> <TAREA_ID> [banderas]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcomandos:")
	fmt.Fprintln(w, "  plan   <TAREA_ID>                             Valida index.md y chunks/, solicita aprobación [s/N] del Autor y sincroniza")
	fmt.Fprintln(w, "  check  <TAREA_ID>                             Ejecuta entrega/verificación del chunk activo (GitWorkspace + ShellRunner)")
	fmt.Fprintln(w, "  decide <TAREA_ID> [--qa-pass] [--qa-notes]    Registra decisión del Autor (GO_NEXT|RETRY_CHUNK|RESPLIT_CHUNK|ABORT)")
	fmt.Fprintln(w, "  status <TAREA_ID>                             Muestra estado actual y regenera status.md")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Banderas globales:")
	fmt.Fprintln(w, "  --root <DIR>   Ruta explícita a la raíz de GengiSWAP (por defecto se detecta automáticamente o GENGIS_ROOT)")
}
