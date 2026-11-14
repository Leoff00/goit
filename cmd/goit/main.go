// Command goit runs tests embedded in a Go source file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"goit/internal/generator"
	"goit/internal/parser"
	"goit/internal/runner"
)

const version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	file, action, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "goit: %v\n", err)
		return 2
	}
	if action == "version" {
		fmt.Fprintf(stdout, "goit %s\n", version)
		return 0
	}
	if action == "help" {
		printUsage(stdout)
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := execute(ctx, file, stdout, stderr, action == "generate-only"); err != nil {
		fmt.Fprintf(stderr, "goit: %v\n", err)
		if errors.Is(err, context.Canceled) {
			return 130
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

func parseArgs(args []string, stderr io.Writer) (string, string, error) {
	fs := flag.NewFlagSet("goit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("help", false, "Show help")
	versionFlag := fs.Bool("version", false, "Show version")
	generateOnly := fs.Bool("generate-only", false, "Create and keep the test file without running tests")
	fs.Usage = func() { printUsage(stderr) }
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	if *help && *versionFlag {
		return "", "", errors.New("--help and --version cannot be used together")
	}
	if *generateOnly && (*help || *versionFlag) {
		return "", "", errors.New("--generate-only cannot be used with --help or --version")
	}
	if *help {
		if fs.NArg() != 0 {
			return "", "", errors.New("--help does not accept a target")
		}
		return "", "help", nil
	}
	if *versionFlag {
		if fs.NArg() != 0 {
			return "", "", errors.New("--version does not accept a target")
		}
		return "", "version", nil
	}
	if fs.NArg() != 1 {
		return "", "", errors.New("expected exactly one Go source file (try --help)")
	}
	target := fs.Arg(0)
	if filepath.Ext(target) != ".go" {
		return "", "", errors.New("target must have a .go extension")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve target path: %w", err)
	}
	if *generateOnly {
		return abs, "generate-only", nil
	}
	return abs, "run", nil
}

func execute(ctx context.Context, filename string, stdout, stderr io.Writer, generateOnly bool) error {
	source, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	result, err := parser.Parse(filename, source)
	if err != nil {
		return err
	}
	if result.Block == nil {
		return errors.New("no inline tests found in target file")
	}
	generated, err := generator.Generate(result)
	if err != nil {
		return fmt.Errorf("generate inline tests: %w", err)
	}
	dir := filepath.Dir(filename)
	if generateOnly {
		path, err := runner.GenerateOnly(ctx, dir, generated)
		if path != "" {
			_, outputErr := fmt.Fprintln(stdout, path)
			err = errors.Join(err, outputErr)
		}
		return err
	}
	if err := runner.Execute(ctx, dir, generated, stdout, stderr); err != nil {
		return err
	}
	return nil
}

func printUsage(w io.Writer) {
	_, _ = io.WriteString(w, "Usage: goit [--generate-only] <file.go>\n       goit --help | --version\n\nRun inline tests in a Go source file. Tests are materialized temporarily and executed by go test in the target package.\n\nOptions:\n  --help           Show this help\n  --version        Show version\n  --generate-only  Create and keep the test file, print its path, and exit without running tests\n\nRemove the retained file manually before running goit again in the same package.\n")
}
