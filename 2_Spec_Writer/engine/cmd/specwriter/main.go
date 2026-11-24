package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"blackfactory/pkg/bfcore"
	"blackfactory/specwriter"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, nil))
}

func run(args []string, out, errOut io.Writer, client bfcore.BrainClient) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(out)
		if len(args) == 0 {
			return specwriter.ExitUsage
		}
		return specwriter.ExitOK
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "prepare":
		return cmdPrepare(rest, out, errOut, client)
	case "validate":
		return cmdValidate(rest, out, errOut, client)
	case "render":
		return cmdRender(rest, out, errOut)
	case "compile":
		return cmdCompile(rest, out, errOut, client)
	case "verify":
		return cmdVerify(rest, out, errOut, client)
	default:
		printUsage(errOut)
		fmt.Fprintf(errOut, "error: subcomando desconocido %q\n", sub)
		return specwriter.ExitUsage
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Uso: specwriter <prepare|validate|render|compile|verify> [banderas]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcomandos:")
	fmt.Fprintln(w, "  prepare   --workitem <path> [--repo-root .] [--factory-root <dir>] [--vault <dir>] [--data-dir <dir>] [--brain-bin <path>] [--refresh-context] [--reset-attempts --reason <texto>] [--json]")
	fmt.Fprintln(w, "  validate  --spec <path> --stage <draft|final> [--repo-root .] [--factory-root <dir>] [--vault <dir>] [--data-dir <dir>] [--brain-bin <path>] [--json]")
	fmt.Fprintln(w, "  render    --spec <path> [--json]")
	fmt.Fprintln(w, "  compile   --spec <path> --task-root <dir> --task-id <id> [--repo-root .] [--factory-root <dir>] [--vault <dir>] [--data-dir <dir>] [--brain-bin <path>] [--json]")
	fmt.Fprintln(w, "  verify    [--factory-root <dir>] [--repo-root .] [--vault <dir>] [--data-dir <dir>] [--brain-bin <path>] [--json]")
}

func cmdPrepare(args []string, out, errOut io.Writer, client bfcore.BrainClient) int {
	fs := flag.NewFlagSet("specwriter prepare", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var workItem, repoRoot, factoryRoot, vaultRoot, dataDir, brainBin, reason string
	var refreshCtx, resetAttempts, asJSON bool

	fs.StringVar(&workItem, "workitem", "", "Ruta al archivo WorkItem JSON v1.0")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria (por defecto igual a --repo-root)")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 2_Spec_Writer/runtime")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&refreshCtx, "refresh-context", false, "Actualiza spec_context.json sin sobrescribir spec.json")
	fs.BoolVar(&resetAttempts, "reset-attempts", false, "Reinicia contadores de attempts.json (exige --reason)")
	fs.StringVar(&reason, "reason", "", "Motivo auditable para --reset-attempts (>= 8 chars)")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	reordered := reorderFlags(args,
		map[string]bool{"refresh-context": true, "reset-attempts": true, "json": true},
		map[string]bool{"workitem": true, "repo-root": true, "factory-root": true, "vault": true, "data-dir": true, "brain-bin": true, "reason": true},
	)
	if err := fs.Parse(reordered); err != nil {
		return emitUsageError(out, errOut, hasJSONFlag(args), err.Error())
	}

	res, code := specwriter.Prepare(specwriter.PrepareOptions{
		WorkItemPath:   workItem,
		RepoRoot:       repoRoot,
		FactoryRoot:    factoryRoot,
		VaultRoot:      vaultRoot,
		DataDir:        dataDir,
		BrainBin:       brainBin,
		RefreshContext: refreshCtx,
		ResetAttempts:  resetAttempts,
		Reason:         reason,
		BrainClient:    client,
	})

	if asJSON {
		writeJSON(out, res)
	} else if res.OK {
		fmt.Fprintf(out, "Prepare OK: %s (spec=%s, refreshed=%v, reset=%v)\n", res.WorkItemID, res.SpecPath, res.Refreshed, res.Reset)
	} else {
		fmt.Fprintf(errOut, "Prepare FAIL [%d %s]: %s\n", res.ExitCode, res.Reason, res.Detail)
	}
	return code
}

func cmdValidate(args []string, out, errOut io.Writer, client bfcore.BrainClient) int {
	fs := flag.NewFlagSet("specwriter validate", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var specPath, stage, repoRoot, factoryRoot, vaultRoot, dataDir, brainBin string
	var asJSON bool

	fs.StringVar(&specPath, "spec", "", "Ruta a spec.json")
	fs.StringVar(&stage, "stage", "", "Etapa de validacion (draft|final)")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 2_Spec_Writer/runtime")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	reordered := reorderFlags(args,
		map[string]bool{"json": true},
		map[string]bool{"spec": true, "stage": true, "repo-root": true, "factory-root": true, "vault": true, "data-dir": true, "brain-bin": true},
	)
	if err := fs.Parse(reordered); err != nil {
		return emitUsageError(out, errOut, hasJSONFlag(args), err.Error())
	}

	res, code := specwriter.Validate(specwriter.ValidateOptions{
		SpecPath:    specPath,
		Stage:       stage,
		RepoRoot:    repoRoot,
		FactoryRoot: factoryRoot,
		VaultRoot:   vaultRoot,
		DataDir:     dataDir,
		BrainBin:    brainBin,
		BrainClient: client,
	})

	if asJSON {
		writeJSON(out, res)
	} else if res.OK {
		fmt.Fprintf(out, "Validate OK (%s): spec_sha256=%s trace_id=%s\n", stage, res.SpecSHA256, res.TraceID)
	} else {
		fmt.Fprintf(errOut, "Validate FAIL [%d %s] retryable=%v (%d issues)\n", res.ExitCode, res.Reason, res.Retryable, len(res.Issues))
		for _, iss := range res.Issues {
			fmt.Fprintf(errOut, "  - %s [%s]: %s\n", iss.Pointer, iss.Code, iss.Message)
		}
	}
	return code
}

func cmdRender(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("specwriter render", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var specPath string
	var asJSON bool

	fs.StringVar(&specPath, "spec", "", "Ruta a spec.json")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	reordered := reorderFlags(args,
		map[string]bool{"json": true},
		map[string]bool{"spec": true},
	)
	if err := fs.Parse(reordered); err != nil {
		return emitUsageError(out, errOut, hasJSONFlag(args), err.Error())
	}
	if strings.TrimSpace(specPath) == "" {
		return emitUsageError(out, errOut, asJSON, "--spec es obligatorio")
	}

	if err := bfcore.CheckNoReparseChain("", specPath); err != nil {
		code := specwriter.ExitInfraOrRefusal
		reason := specwriter.ReasonIOError
		if errors.Is(err, bfcore.ErrReparsePoint) {
			reason = specwriter.ReasonReparsePointDetected
		}
		res := specwriter.CLIResult{OK: false, ExitCode: code, Reason: reason, Retryable: specwriter.IsRetryableReason(code, reason)}
		if asJSON {
			writeJSON(out, res)
		} else {
			fmt.Fprintf(errOut, "Render FAIL [%d %s]\n", code, reason)
		}
		return code
	}

	fi, err := os.Lstat(specPath)
	if err != nil {
		code := specwriter.ExitInfraOrRefusal
		reason := specwriter.ReasonIOError
		if errors.Is(err, os.ErrNotExist) {
			code = specwriter.ExitUsage
			reason = specwriter.ReasonInvalidUsage
		}
		res := specwriter.CLIResult{OK: false, ExitCode: code, Reason: reason, Retryable: specwriter.IsRetryableReason(code, reason)}
		if asJSON {
			writeJSON(out, res)
		} else {
			fmt.Fprintf(errOut, "Render FAIL [%d %s]\n", code, reason)
		}
		return code
	}
	if bfcore.IsReparseOrSymlink(fi) {
		res := specwriter.CLIResult{OK: false, ExitCode: specwriter.ExitInfraOrRefusal, Reason: specwriter.ReasonReparsePointDetected, Retryable: false}
		if asJSON {
			writeJSON(out, res)
		} else {
			fmt.Fprintf(errOut, "Render FAIL [6 REPARSE_POINT_DETECTED]\n")
		}
		return specwriter.ExitInfraOrRefusal
	}
	if fi.IsDir() || !fi.Mode().IsRegular() {
		return emitUsageError(out, errOut, asJSON, "--spec no es un archivo regular")
	}
	if fi.Size() > specwriter.MaxSpecBytes {
		res := specwriter.CLIResult{OK: false, ExitCode: specwriter.ExitFixable, Reason: specwriter.ReasonSpecTooLarge, Retryable: true}
		if asJSON {
			writeJSON(out, res)
		} else {
			fmt.Fprintf(errOut, "Render FAIL [1 SPEC_TOO_LARGE]\n")
		}
		return specwriter.ExitFixable
	}

	raw, err := os.ReadFile(specPath)
	if err != nil {
		res := specwriter.CLIResult{OK: false, ExitCode: specwriter.ExitInfraOrRefusal, Reason: specwriter.ReasonIOError, Retryable: true}
		if asJSON {
			writeJSON(out, res)
		} else {
			fmt.Fprintf(errOut, "Render FAIL [6 IO_ERROR]\n")
		}
		return specwriter.ExitInfraOrRefusal
	}

	md, spec, code, reason, rErr := specwriter.RenderSpecMarkdown(raw)
	if code != specwriter.ExitOK {
		res := specwriter.CLIResult{
			OK:        false,
			ExitCode:  code,
			Reason:    reason,
			Retryable: specwriter.IsRetryableReason(code, reason),
			Issues: []specwriter.ValidationIssue{{
				Pointer: "/",
				Code:    reason,
				Message: bfcore.RedactAllG4(rErr.Error()),
			}},
		}
		if asJSON {
			writeJSON(out, res)
		} else {
			fmt.Fprintf(errOut, "Render FAIL [%d %s]: %s\n", code, reason, bfcore.RedactAllG4(rErr.Error()))
		}
		return code
	}

	specSHA, _ := specwriter.CanonicalSpecSHA256(spec)
	if asJSON {
		writeJSON(out, specwriter.CLIResult{
			OK:         true,
			ExitCode:   specwriter.ExitOK,
			Reason:     specwriter.ReasonOK,
			Retryable:  false,
			TraceID:    spec.WorkItemRef.TraceID,
			SpecSHA256: specSHA,
			Markdown:   md,
		})
	} else {
		fmt.Fprint(out, md)
	}
	return specwriter.ExitOK
}

func cmdCompile(args []string, out, errOut io.Writer, client bfcore.BrainClient) int {
	fs := flag.NewFlagSet("specwriter compile", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var specPath, taskRoot, taskID, repoRoot, factoryRoot, vaultRoot, dataDir, brainBin string
	var asJSON bool

	fs.StringVar(&specPath, "spec", "", "Ruta a spec.json")
	fs.StringVar(&taskRoot, "task-root", "", "Directorio raiz de tareas GengiSWAP")
	fs.StringVar(&taskID, "task-id", "", "Identificador de tarea GengiSWAP")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 2_Spec_Writer/runtime")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	reordered := reorderFlags(args,
		map[string]bool{"json": true},
		map[string]bool{"spec": true, "task-root": true, "task-id": true, "repo-root": true, "factory-root": true, "vault": true, "data-dir": true, "brain-bin": true},
	)
	if err := fs.Parse(reordered); err != nil {
		return emitUsageError(out, errOut, hasJSONFlag(args), err.Error())
	}

	res, code := specwriter.Compile(specwriter.CompileOptions{
		SpecPath:    specPath,
		TaskRoot:    taskRoot,
		TaskID:      taskID,
		RepoRoot:    repoRoot,
		FactoryRoot: factoryRoot,
		VaultRoot:   vaultRoot,
		DataDir:     dataDir,
		BrainBin:    brainBin,
		BrainClient: client,
	})

	if asJSON {
		writeJSON(out, res)
	} else if res.OK {
		fmt.Fprintf(out, "Compile OK: task_dir=%s\n", res.TaskDir)
		fmt.Fprintf(out, "  spec_sha256: %s | trace_id: %s\n", res.SpecSHA256, res.TraceID)
		fmt.Fprintf(out, "  Siguiente paso [AUTOR]: gengis.exe plan %q\n", res.TaskDir)
	} else {
		fmt.Fprintf(errOut, "Compile FAIL [%d %s] retryable=%v\n", res.ExitCode, res.Reason, res.Retryable)
		for _, iss := range res.Issues {
			fmt.Fprintf(errOut, "  - %s [%s]: %s\n", iss.Pointer, iss.Code, iss.Message)
		}
	}
	return code
}

func cmdVerify(args []string, out, errOut io.Writer, client bfcore.BrainClient) int {
	fs := flag.NewFlagSet("specwriter verify", flag.ContinueOnError)
	fs.SetOutput(errOut)

	var factoryRoot, repoRoot, vaultRoot, dataDir, brainBin string
	var asJSON bool

	fs.StringVar(&factoryRoot, "factory-root", "", "Raiz de la factoria")
	fs.StringVar(&repoRoot, "repo-root", ".", "Raiz del repositorio objetivo")
	fs.StringVar(&vaultRoot, "vault", "", "Ruta a 0_Brain/vault")
	fs.StringVar(&dataDir, "data-dir", "", "Ruta a 2_Spec_Writer/runtime")
	fs.StringVar(&brainBin, "brain-bin", "", "Ruta a 0_Brain/brain.exe")
	fs.BoolVar(&asJSON, "json", false, "Emitir salida JSON estructurada")

	reordered := reorderFlags(args,
		map[string]bool{"json": true},
		map[string]bool{"factory-root": true, "repo-root": true, "vault": true, "data-dir": true, "brain-bin": true},
	)
	if err := fs.Parse(reordered); err != nil {
		return emitUsageError(out, errOut, hasJSONFlag(args), err.Error())
	}

	rep, code := specwriter.VerifyRuntime(specwriter.VerifyOptions{
		FactoryRoot: factoryRoot,
		RepoRoot:    repoRoot,
		VaultRoot:   vaultRoot,
		DataDir:     dataDir,
		BrainBin:    brainBin,
		BrainClient: client,
	})

	if asJSON {
		writeJSON(out, rep)
	} else if rep.OK {
		fmt.Fprintf(out, "Verify OK=true (gitignore=%v, gengis_hashes=%v, brain=%v, artifacts=%d, g4_issues=%d)\n",
			rep.GitIgnoreOK, rep.GengisHashesOK, rep.BrainOK, rep.ArtifactsChecked, rep.G4Issues)
	} else {
		fmt.Fprintf(errOut, "Verify FAIL [%d %s] retryable=%v (gitignore=%v, gengis_hashes=%v, brain=%v, g4_issues=%d)\n",
			rep.ExitCode, rep.Reason, rep.Retryable, rep.GitIgnoreOK, rep.GengisHashesOK, rep.BrainOK, rep.G4Issues)
		for _, iss := range rep.Issues {
			fmt.Fprintf(errOut, "  - %s [%s]: %s\n", iss.Pointer, iss.Code, iss.Message)
		}
	}
	return code
}

func emitUsageError(out, errOut io.Writer, asJSON bool, msg string) int {
	res := specwriter.CLIResult{
		OK:        false,
		ExitCode:  specwriter.ExitUsage,
		Reason:    specwriter.ReasonInvalidUsage,
		Retryable: false,
		Issues: []specwriter.ValidationIssue{{
			Pointer: "/cli",
			Code:    specwriter.ReasonInvalidUsage,
			Message: bfcore.RedactAllG4(msg),
		}},
	}
	if asJSON {
		writeJSON(out, res)
	} else {
		fmt.Fprintf(errOut, "error [2 INVALID_USAGE]: %s\n", bfcore.RedactAllG4(msg))
	}
	return specwriter.ExitUsage
}

func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" || a == "--json=true" {
			return true
		}
	}
	return false
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
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
