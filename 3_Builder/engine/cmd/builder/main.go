package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"blackfactory/builder"
	"blackfactory/pkg/bfcore"
)

// CLIHooks permite inyectar dobles deterministas en tests CLI sin alterar el binario productivo.
type CLIHooks struct {
	BrainClient bfcore.BrainClient
	Git         builder.GitClient
	Runner      builder.CommandRunner
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, CLIHooks{}))
}

func run(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(out)
		if len(args) == 0 {
			return builder.ExitUsage
		}
		return builder.ExitOK
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "init":
		return cmdInit(rest, out, errOut, hooks)
	case "context":
		return cmdContext(rest, out, errOut, hooks)
	case "tdd-red":
		return cmdTDDRed(rest, out, errOut, hooks)
	case "package":
		return cmdPackage(rest, out, errOut, hooks)
	case "apply-order":
		return cmdApplyOrder(rest, out, errOut, hooks)
	case "commit":
		return cmdCommit(rest, out, errOut, hooks)
	case "cleanup":
		return cmdCleanup(rest, out, errOut, hooks)
	case "verify":
		return cmdVerify(rest, out, errOut, hooks)
	default:
		printUsage(errOut)
		_, _ = fmt.Fprintf(errOut, "error: subcomando desconocido %q\n", sub)
		return builder.ExitUsage
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Uso: builder <init|context|tdd-red|package|apply-order|commit|cleanup|verify> [banderas]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Subcomandos:")
	_, _ = fmt.Fprintln(w, "  init         --task-dir <dir> [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--gengis-root <dir>] [--vault <dir>] [--brain-bin <exe>] [--project <name>] [--branch <name>] [--base-ref <ref>] [--json]")
	_, _ = fmt.Fprintln(w, "  context      --task-dir <dir> [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--vault <dir>] [--brain-bin <exe>] [--refresh-brain] [--json]")
	_, _ = fmt.Fprintln(w, "  tdd-red      --task-dir <dir> [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--cmd-index 0] [--timeout 300] [--json]")
	_, _ = fmt.Fprintln(w, "  package      --task-dir <dir> [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--vault <dir>] [--brain-bin <exe>] [--include-ui] [--include-ux] [--timeout 300] [--json]")
	_, _ = fmt.Fprintln(w, "  apply-order  --task-dir <dir> (--order <path> | --reset-bounces --reason <texto>) [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--json]")
	_, _ = fmt.Fprintln(w, "  commit       --task-dir <dir> [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--gengis-root <dir>] [--vault <dir>] [--brain-bin <exe>] [--timeout 300] [--json]")
	_, _ = fmt.Fprintln(w, "  cleanup      --task-dir <dir> [--repo-root .] [--factory-root <dir>] [--data-dir <dir>] [--worktree-root <dir>] [--force] [--delete-branch] [--json]")
	_, _ = fmt.Fprintln(w, "  verify       [--factory-root <dir>] [--repo-root .] [--vault <dir>] [--data-dir <dir>] [--brain-bin <exe>] [--json]")
}

func parseSubFlags(fs *flag.FlagSet, args []string, boolFlags, valueFlags map[string]bool, out, errOut io.Writer) (int, bool) {
	asJSON := hasJSONFlag(args)
	fs.SetOutput(io.Discard)
	reordered := reorderFlags(args, boolFlags, valueFlags)
	if err := fs.Parse(reordered); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprintf(out, "Uso de %s:\n", fs.Name())
			fs.SetOutput(out)
			fs.PrintDefaults()
			return builder.ExitOK, false
		}
		return emitUsageError(out, errOut, asJSON, err.Error()), false
	}
	if fs.NArg() > 0 {
		return emitUsageError(out, errOut, asJSON, fmt.Sprintf("argumentos posicionales inesperados: %v", fs.Args())), false
	}
	timeoutVisited := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "timeout" {
			timeoutVisited = true
		}
	})
	if timeoutVisited {
		if tf := fs.Lookup("timeout"); tf != nil {
			if g, ok := tf.Value.(flag.Getter); ok {
				if v, ok := g.Get().(int); ok && (v < 1 || v > builder.MaxVerifyTimeoutSec) {
					return emitUsageError(out, errOut, asJSON, fmt.Sprintf("--timeout debe estar en 1..%d", builder.MaxVerifyTimeoutSec)), false
				}
			}
		}
	}
	return builder.ExitOK, true
}

func emitCLIResult(res builder.CLIResult, code int, asJSON bool, label string, out, errOut io.Writer) int {
	if asJSON {
		writeJSON(out, res)
		return code
	}
	if res.OK {
		_, _ = fmt.Fprintf(out, "%s OK: task=%s chunk=%s attempt=%d cycle=%d", label, res.TaskID, res.ChunkID, res.Attempt, res.BounceCycle)
		if res.BundleID != "" {
			_, _ = fmt.Fprintf(out, " bundle=%s sha256=%s", res.BundleID, res.BundleSHA256)
		}
		if res.Decision != "" {
			_, _ = fmt.Fprintf(out, " decision=%s", res.Decision)
		}
		if res.CommitRef != "" {
			_, _ = fmt.Fprintf(out, " commit=%s", res.CommitRef)
		}
		_, _ = fmt.Fprintln(out)
		if res.NextAutorCommand != "" {
			_, _ = fmt.Fprintf(out, "Siguiente paso [AUTOR]: %s\n", res.NextAutorCommand)
		}
		return code
	}
	_, _ = fmt.Fprintf(errOut, "%s FAIL [%d %s] retryable=%v\n", label, res.ExitCode, res.Reason, res.Retryable)
	for _, iss := range res.Issues {
		_, _ = fmt.Fprintf(errOut, "  - %s [%s]: %s\n", iss.Pointer, iss.Code, iss.Message)
	}
	if res.NextAutorCommand != "" {
		_, _ = fmt.Fprintf(out, "Siguiente paso [AUTOR]: %s\n", res.NextAutorCommand)
	}
	return code
}

func cmdInit(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder init", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, repoRoot, factoryRoot, dataDir, wtRoot, gengisRoot, vaultRoot, brainBin, project, branch, baseRef string
	var asJSON bool
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.StringVar(&gengisRoot, "gengis-root", "", "Ruta a GengiSWAP/")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.StringVar(&project, "project", "", "Nombre del proyecto para 0_Brain")
	fs.StringVar(&branch, "branch", "", "Nombre de rama aislada (default build/<task_id>)")
	fs.StringVar(&baseRef, "base-ref", "", "Commit/ref base en Secuencia A")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"json": true},
		map[string]bool{"task-dir": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true, "gengis-root": true, "vault": true, "brain-bin": true, "project": true, "branch": true, "base-ref": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.Init(builder.InitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		GengisRoot:   gengisRoot,
		VaultRoot:    vaultRoot,
		BrainBin:     brainBin,
		Project:      project,
		Branch:       branch,
		BaseRef:      baseRef,
		BrainClient:  hooks.BrainClient,
		Git:          hooks.Git,
	})
	return emitCLIResult(res, code, asJSON, "Init", out, errOut)
}

func cmdContext(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder context", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, repoRoot, factoryRoot, dataDir, wtRoot, vaultRoot, brainBin string
	var refreshBrain, asJSON bool
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&refreshBrain, "refresh-brain", false, "Re-consulta 0_Brain y actualiza BrainContext")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"refresh-brain": true, "json": true},
		map[string]bool{"task-dir": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true, "vault": true, "brain-bin": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.Context(builder.ContextOptions{
		TaskDir:      taskDir,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vaultRoot,
		BrainBin:     brainBin,
		RefreshBrain: refreshBrain,
		BrainClient:  hooks.BrainClient,
		Git:          hooks.Git,
	})
	if asJSON {
		writeJSON(out, res)
	} else if res.OK {
		_, _ = fmt.Fprintf(out, "Context OK: task=%s chunk=%s attempt=%d cycle=%d worktree=%s branch=%s risk=%s\n",
			res.TaskID, res.ChunkID, res.Attempt, res.BounceCycle, res.WorktreePath, res.BuildBranch, res.BrainContext.HighestRiskLevel)
	} else {
		_, _ = fmt.Fprintf(errOut, "Context FAIL [%d %s] retryable=%v\n", res.ExitCode, res.Reason, res.Retryable)
	}
	return code
}

func cmdTDDRed(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder tdd-red", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, repoRoot, factoryRoot, dataDir, wtRoot string
	var cmdIndex, timeoutSec int
	var asJSON bool
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.IntVar(&cmdIndex, "cmd-index", 0, "Indice de VerifyCommands a ejecutar en fase Red")
	fs.IntVar(&timeoutSec, "timeout", builder.MaxVerifyTimeoutSec, "Timeout por comando en segundos (1..300)")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"json": true},
		map[string]bool{"task-dir": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true, "cmd-index": true, "timeout": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.TDDRed(builder.TDDRedOptions{
		TaskDir:      taskDir,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		CmdIndex:     cmdIndex,
		TimeoutSec:   timeoutSec,
		Git:          hooks.Git,
		Runner:       hooks.Runner,
	})
	return emitCLIResult(res, code, asJSON, "TDD-Red", out, errOut)
}

func cmdPackage(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder package", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, repoRoot, factoryRoot, dataDir, wtRoot, vaultRoot, brainBin string
	var includeUI, includeUX, asJSON bool
	var timeoutSec int
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&includeUI, "include-ui", false, "Incluir aditivamente revisor UI aunque auto=false")
	fs.BoolVar(&includeUX, "include-ux", false, "Incluir aditivamente revisor UX aunque auto=false")
	fs.IntVar(&timeoutSec, "timeout", builder.MaxVerifyTimeoutSec, "Timeout por comando en segundos (1..300)")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"include-ui": true, "include-ux": true, "json": true},
		map[string]bool{"task-dir": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true, "vault": true, "brain-bin": true, "timeout": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.Package(builder.PackageOptions{
		TaskDir:      taskDir,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		VaultRoot:    vaultRoot,
		BrainBin:     brainBin,
		IncludeUI:    includeUI,
		IncludeUX:    includeUX,
		TimeoutSec:   timeoutSec,
		BrainClient:  hooks.BrainClient,
		Git:          hooks.Git,
		Runner:       hooks.Runner,
	})
	return emitCLIResult(res, code, asJSON, "Package", out, errOut)
}

func cmdApplyOrder(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder apply-order", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, orderPath, repoRoot, factoryRoot, dataDir, wtRoot, reason string
	var resetBounces, asJSON bool
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&orderPath, "order", "", "Ruta a BounceOrder JSON v1.0")
	fs.BoolVar(&resetBounces, "reset-bounces", false, "Incrementa ResetEpoch y limpia freno operativo")
	fs.StringVar(&reason, "reason", "", "Motivo auditable para --reset-bounces (>= 8 chars)")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"reset-bounces": true, "json": true},
		map[string]bool{"task-dir": true, "order": true, "reason": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.ApplyOrder(builder.ApplyOrderOptions{
		TaskDir:      taskDir,
		OrderPath:    orderPath,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		ResetBounces: resetBounces,
		Reason:       reason,
		Git:          hooks.Git,
	})
	return emitCLIResult(res, code, asJSON, "ApplyOrder", out, errOut)
}

func cmdCommit(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder commit", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, repoRoot, factoryRoot, dataDir, wtRoot, gengisRoot, vaultRoot, brainBin string
	var timeoutSec int
	var asJSON bool
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.StringVar(&gengisRoot, "gengis-root", "", "Ruta a GengiSWAP/")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.IntVar(&timeoutSec, "timeout", builder.MaxVerifyTimeoutSec, "Timeout por comando en segundos (1..300)")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"json": true},
		map[string]bool{"task-dir": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true, "gengis-root": true, "vault": true, "brain-bin": true, "timeout": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.Commit(builder.CommitOptions{
		TaskDir:      taskDir,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		GengisRoot:   gengisRoot,
		VaultRoot:    vaultRoot,
		BrainBin:     brainBin,
		TimeoutSec:   timeoutSec,
		BrainClient:  hooks.BrainClient,
		Git:          hooks.Git,
	})
	return emitCLIResult(res, code, asJSON, "Commit", out, errOut)
}

func cmdCleanup(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder cleanup", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var taskDir, repoRoot, factoryRoot, dataDir, wtRoot string
	var force, deleteBranch, asJSON bool
	fs.StringVar(&taskDir, "task-dir", "", "Ruta a carpeta de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&wtRoot, "worktree-root", "", "Raiz de git worktrees")
	fs.BoolVar(&force, "force", false, "Forzar limpieza en tareas no terminales o worktree sucio")
	fs.BoolVar(&deleteBranch, "delete-branch", false, "Eliminar rama build/<task_id>")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"force": true, "delete-branch": true, "json": true},
		map[string]bool{"task-dir": true, "repo-root": true, "factory-root": true, "data-dir": true, "worktree-root": true},
		out, errOut,
	); !ok {
		return code
	}

	res, code := builder.Cleanup(builder.CleanupOptions{
		TaskDir:      taskDir,
		RepoRoot:     repoRoot,
		FactoryRoot:  factoryRoot,
		DataDir:      dataDir,
		WorktreeRoot: wtRoot,
		Force:        force,
		DeleteBranch: deleteBranch,
		Git:          hooks.Git,
	})
	return emitCLIResult(res, code, asJSON, "Cleanup", out, errOut)
}

func cmdVerify(args []string, out, errOut io.Writer, hooks CLIHooks) int {
	fs := flag.NewFlagSet("builder verify", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var factoryRoot, repoRoot, vaultRoot, dataDir, brainBin string
	var asJSON bool
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 3_Builder/runtime")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	if code, ok := parseSubFlags(fs, args,
		map[string]bool{"json": true},
		map[string]bool{"factory-root": true, "repo-root": true, "vault": true, "data-dir": true, "brain-bin": true},
		out, errOut,
	); !ok {
		return code
	}

	rep, code := builder.VerifyRuntime(builder.VerifyOptions{
		FactoryRoot: factoryRoot,
		RepoRoot:    repoRoot,
		VaultRoot:   vaultRoot,
		DataDir:     dataDir,
		BrainBin:    brainBin,
		BrainClient: hooks.BrainClient,
	})
	if asJSON {
		writeJSON(out, rep)
	} else if rep.OK {
		_, _ = fmt.Fprintf(out, "Verify OK=true (gitignore=%v, gengis_hashes=%v, brain=%v, artifacts=%d, g4_issues=%d)\n",
			rep.GitIgnoreOK, rep.GengisHashesOK, rep.BrainOK, rep.ArtifactsChecked, rep.G4Issues)
	} else {
		_, _ = fmt.Fprintf(errOut, "Verify FAIL [%d %s] retryable=%v (gitignore=%v, gengis_hashes=%v, brain=%v, g4_issues=%d)\n",
			rep.ExitCode, rep.Reason, rep.Retryable, rep.GitIgnoreOK, rep.GengisHashesOK, rep.BrainOK, rep.G4Issues)
		for _, iss := range rep.Issues {
			_, _ = fmt.Fprintf(errOut, "  - %s [%s]: %s\n", iss.Pointer, iss.Code, iss.Message)
		}
	}
	return code
}

func emitUsageError(out, errOut io.Writer, asJSON bool, msg string) int {
	res := builder.CLIResult{
		OK:        false,
		ExitCode:  builder.ExitUsage,
		Reason:    builder.ReasonInvalidUsage,
		Retryable: false,
		Issues: []builder.ValidationIssue{{
			Pointer: "/cli",
			Code:    builder.ReasonInvalidUsage,
			Message: bfcore.RedactAllG4(msg),
		}},
	}
	if asJSON {
		writeJSON(out, res)
	} else {
		_, _ = fmt.Fprintf(errOut, "error [2 INVALID_USAGE]: %s\n", bfcore.RedactAllG4(msg))
	}
	return builder.ExitUsage
}

func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		norm := strings.ToLower(strings.TrimSpace(a))
		if norm == "--json" || norm == "-json" || norm == "--json=true" || norm == "-json=true" {
			return true
		}
	}
	return false
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func reorderFlags(args []string, boolFlags, valueFlags map[string]bool) []string {
	var flagsOut, posOut []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			posOut = append(posOut, args[i:]...)
			break
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			trimmed := strings.TrimLeft(a, "-")
			name := trimmed
			hasEq := false
			if eq := strings.IndexByte(trimmed, '='); eq >= 0 {
				name = trimmed[:eq]
				hasEq = true
			}
			if boolFlags[name] {
				if !hasEq && i+1 < len(args) && (args[i+1] == "true" || args[i+1] == "false") {
					flagsOut = append(flagsOut, a+"="+args[i+1])
					i++
				} else {
					flagsOut = append(flagsOut, a)
				}
				continue
			}
			if valueFlags[name] {
				flagsOut = append(flagsOut, a)
				if !hasEq && i+1 < len(args) {
					i++
					flagsOut = append(flagsOut, args[i])
				}
				continue
			}
		}
		posOut = append(posOut, a)
	}
	return append(flagsOut, posOut...)
}
