// Package runner materializes generated tests safely and delegates execution to go test.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const lockName = ".goit.lock"

// Execute publishes generated tests in directory, invokes go test, and removes only
// artifacts created by this invocation. Existing goit artifacts require manual recovery.
func Execute(ctx context.Context, directory string, generated []byte, stdout, stderr io.Writer) error {
	_, err := execute(ctx, directory, generated, stdout, stderr, false)
	return err
}

// GenerateOnly publishes a test file without executing it and returns its path.
// The caller owns the published file; staging files and the package lock are removed.
func GenerateOnly(ctx context.Context, directory string, generated []byte) (string, error) {
	return execute(ctx, directory, generated, nil, nil, true)
}

func execute(ctx context.Context, directory string, generated []byte, stdout, stderr io.Writer, generateOnly bool) (path string, resultErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	lock := filepath.Join(directory, lockName)
	if err := os.Mkdir(lock, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("goit: package is locked (remove %s only after confirming no goit process is running)", lock)
		}
		return "", fmt.Errorf("goit: acquire package lock: %w", err)
	}
	defer func() {
		if err := os.Remove(lock); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("goit: release package lock: %w", err))
		}
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("goit: inspect package directory: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "zz_goit_") && strings.HasSuffix(entry.Name(), "_test.go") {
			return "", fmt.Errorf("goit: existing generated test %s found; inspect and remove it manually before retrying", filepath.Join(directory, entry.Name()))
		}
	}

	stage, err := os.CreateTemp(directory, ".goit-stage-*")
	if err != nil {
		return "", fmt.Errorf("goit: create staging file: %w", err)
	}
	stageName := stage.Name()
	defer func() {
		if err := os.Remove(stageName); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("goit: remove staging file: %w", err))
		}
	}()
	if _, err = stage.Write(generated); err != nil {
		_ = stage.Close()
		return "", fmt.Errorf("goit: write staging file: %w", err)
	}
	if err = stage.Close(); err != nil {
		return "", fmt.Errorf("goit: close staging file: %w", err)
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("goit: generate test filename: %w", err)
	}
	published := filepath.Join(directory, "zz_goit_"+hex.EncodeToString(id[:])+"_test.go")
	if err = os.Link(stageName, published); err != nil {
		return "", fmt.Errorf("goit: publish generated test: %w", err)
	}
	keepPublished := false
	defer func() {
		if keepPublished {
			return
		}
		if err := os.Remove(published); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("goit: remove generated test: %w", err))
		}
	}()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if generateOnly {
		keepPublished = true
		return published, nil
	}

	if err = ctx.Err(); err == nil {
		cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-v", ".")
		cmd.Dir = directory
		cmd.Stdout, cmd.Stderr = stdout, stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return os.ErrProcessDone
			}
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		err = cmd.Run()
		if err != nil {
			err = fmt.Errorf("goit: go test: %w", err)
			if ctx.Err() != nil {
				err = errors.Join(err, ctx.Err())
			}
		}
	}
	return "", err
}
