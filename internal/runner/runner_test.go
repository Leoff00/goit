package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	cases := []struct {
		name, generated string
		wantFailure     bool
	}{
		{"success", "package fixture\nimport \"testing\"\nfunc TestInline(t *testing.T) { t.Log(\"inline\") }\n", false},
		{"assertion failure", "package fixture\nimport \"testing\"\nfunc TestInline(t *testing.T) { t.Fatal(\"inline failure\") }\n", true},
		{"compile failure", "package fixture\nfunc TestInline( {\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t)
			source := []byte("package fixture\nconst source = \"unchanged\"\n")
			if err := os.WriteFile(filepath.Join(dir, "source.go"), source, 0600); err != nil {
				t.Fatal(err)
			}
			conventional := []byte("package fixture\nimport \"testing\"\nfunc TestConventional(t *testing.T) { t.Log(\"conventional\") }\n")
			if err := os.WriteFile(filepath.Join(dir, "fixture_test.go"), conventional, 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			err := Execute(context.Background(), dir, []byte(tc.generated), &out, &stderr)
			var exitErr *exec.ExitError
			if tc.wantFailure {
				if !errors.As(err, &exitErr) {
					t.Fatalf("expected *exec.ExitError, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			output := out.String() + stderr.String()
			if tc.name == "success" && (!strings.Contains(output, "TestInline") || !strings.Contains(output, "TestConventional") || !strings.Contains(output, "PASS")) {
				t.Fatalf("missing test output: %s", output)
			}
			if tc.name == "assertion failure" && !strings.Contains(output, "inline failure") {
				t.Fatalf("missing failure output: %s", output)
			}
			if tc.name == "compile failure" && !strings.Contains(output, "FAIL") {
				t.Fatalf("missing compile output: %s", output)
			}
			got, readErr := os.ReadFile(filepath.Join(dir, "source.go"))
			if readErr != nil || !bytes.Equal(got, source) {
				t.Fatalf("source changed: %q, %v", got, readErr)
			}
			got, readErr = os.ReadFile(filepath.Join(dir, "fixture_test.go"))
			if readErr != nil || !bytes.Equal(got, conventional) {
				t.Fatalf("conventional test changed: %q, %v", got, readErr)
			}
			assertClean(t, dir)
		})
	}
}

func TestLockAndOrphanPreserved(t *testing.T) {
	dir := fixture(t)
	lock := filepath.Join(dir, lockName)
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Execute(context.Background(), dir, []byte("package fixture"), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected lock error")
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("lock removed: %v", err)
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(dir, "zz_goit_orphan_test.go")
	if err := os.WriteFile(orphan, []byte("owned by prior process"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Execute(context.Background(), dir, nil, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected orphan error")
	}
	got, err := os.ReadFile(orphan)
	if err != nil || string(got) != "owned by prior process" {
		t.Fatalf("orphan altered: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock was not removed: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".goit-stage-") {
			t.Fatalf("staging artifact remains: %s", entry.Name())
		}
	}
}

type cancelOnRun struct {
	cancel context.CancelFunc
	fired  bool
}

func (w *cancelOnRun) Write(p []byte) (int, error) {
	if !w.fired && bytes.Contains(p, []byte("TestInline")) {
		w.fired = true
		w.cancel()
	}
	return len(p), nil
}

func TestIntegrationCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	writer := &cancelOnRun{cancel: cancel}
	err := Execute(ctx, dir, []byte("package fixture\nimport \"testing\"\nfunc TestInline(t *testing.T) { for {} }\n"), writer, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	assertClean(t, dir)
}

func TestIntegrationConcurrentExecuteLock(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := make(chan struct{}, 1)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- Execute(ctx, dir, []byte("package fixture\nimport \"testing\"\nfunc TestInline(t *testing.T) { for {} }\n"), &notifyOnRun{started: started}, &bytes.Buffer{})
	}()
	select {
	case <-started:
	case err := <-firstDone:
		t.Fatalf("first invocation ended before test started: %v", err)
	case <-ctx.Done():
		t.Fatalf("first invocation did not start in time: %v", ctx.Err())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var published string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "zz_goit_") && strings.HasSuffix(entry.Name(), "_test.go") {
			published = entry.Name()
		}
	}
	if published == "" {
		t.Fatal("first invocation did not publish its test")
	}
	secondErr := Execute(ctx, dir, []byte("package fixture"), &bytes.Buffer{}, &bytes.Buffer{})
	if secondErr == nil || !strings.Contains(secondErr.Error(), "locked") {
		t.Fatalf("second invocation should fail on lock, got %v", secondErr)
	}
	if _, err := os.Stat(filepath.Join(dir, published)); err != nil {
		t.Fatalf("second invocation removed first test: %v", err)
	}
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first invocation should cancel, got %v", err)
	}
	assertClean(t, dir)
}

type notifyOnRun struct{ started chan<- struct{} }

func (w *notifyOnRun) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("TestInline")) {
		select {
		case w.started <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func TestCanceledBeforeExecute(t *testing.T) {
	dir := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Execute(ctx, dir, nil, &bytes.Buffer{}, &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
	assertClean(t, dir)
}

func TestMissingGoCommand(t *testing.T) {
	dir := fixture(t)
	t.Setenv("PATH", t.TempDir())
	source := []byte("package fixture\nconst value = 42\n")
	if err := os.WriteFile(filepath.Join(dir, "source.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	err := Execute(context.Background(), dir, []byte("package fixture\n"), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "executable file not found") {
		t.Fatalf("expected go invocation error, got %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "source.go"))
	if err != nil || !bytes.Equal(got, source) {
		t.Fatalf("source changed: %q, %v", got, err)
	}
	assertClean(t, dir)
}

func TestGenerateOnly(t *testing.T) {
	dir := fixture(t)
	t.Setenv("PATH", t.TempDir()) // Generation must not invoke the Go toolchain.
	generated := []byte("package fixture\r\nfunc TestInline() {}\r\n")
	path, err := GenerateOnly(context.Background(), dir, generated)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir || !strings.HasPrefix(filepath.Base(path), "zz_goit_") || !strings.HasSuffix(path, "_test.go") {
		t.Fatalf("unexpected generated path: %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, generated) {
		t.Fatalf("generated content = %q, error = %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == lockName || strings.HasPrefix(entry.Name(), ".goit-stage-") {
			t.Fatalf("unexpected artifact: %s", entry.Name())
		}
	}
	if other, err := GenerateOnly(context.Background(), dir, generated); err == nil || other != "" {
		t.Fatalf("second generation = %q, %v; want rejection", other, err)
	}
	if err := Execute(context.Background(), dir, generated, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("execution should reject the retained file")
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, generated) {
		t.Fatalf("retained file changed: %q, %v", got, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	assertClean(t, dir)
}

func TestGenerateOnlyCanceledOrLocked(t *testing.T) {
	for _, mode := range []string{"canceled", "locked"} {
		t.Run(mode, func(t *testing.T) {
			dir := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			} else if err := os.Mkdir(filepath.Join(dir, lockName), 0700); err != nil {
				t.Fatal(err)
			}
			path, err := GenerateOnly(ctx, dir, []byte("package fixture\n"))
			if err == nil || path != "" {
				t.Fatalf("GenerateOnly = %q, %v; want rejection", path, err)
			}
			if mode == "canceled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context cancellation: %v", err)
				}
			} else if err := os.Remove(filepath.Join(dir, lockName)); err != nil {
				t.Fatalf("preexisting lock was not preserved: %v", err)
			}
			assertClean(t, dir)
		})
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.26.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	return dir
}

func assertClean(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == lockName || strings.HasPrefix(e.Name(), ".goit-stage-") || strings.HasPrefix(e.Name(), "zz_goit_") {
			t.Errorf("artifact remains: %s", e.Name())
		}
	}
}
