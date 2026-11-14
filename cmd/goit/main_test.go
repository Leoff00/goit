package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantAction string
		wantErr    bool
	}{
		{"help", []string{"--help"}, "help", false},
		{"version", []string{"--version"}, "version", false},
		{"file", []string{"x.go"}, "run", false},
		{"generate only", []string{"--generate-only", "x.go"}, "generate-only", false},
		{"generate missing file", []string{"--generate-only"}, "", true},
		{"generate with help", []string{"--generate-only", "--help"}, "", true},
		{"generate with version", []string{"--generate-only", "--version"}, "", true},
		{"missing", nil, "", true},
		{"extra", []string{"x.go", "y.go"}, "", true},
		{"wrong extension", []string{"x.txt"}, "", true},
		{"conflicting options", []string{"--help", "--version"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, action, err := parseArgs(tc.args, &bytes.Buffer{})
			if (err != nil) != tc.wantErr || action != tc.wantAction {
				t.Fatalf("parseArgs() action=%q err=%v", action, err)
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := run([]string{"--help"}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "Usage: goit") {
		t.Fatalf("help code=%d output=%q stderr=%q", code, out.String(), stderr.String())
	}
	out.Reset()
	if code := run([]string{"--version"}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "goit dev") {
		t.Fatalf("version code=%d output=%q", code, out.String())
	}
}

func TestRunErrors(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := run(nil, &out, &stderr); code == 0 {
		t.Fatal("missing target unexpectedly succeeded")
	}
	if !strings.Contains(stderr.String(), "expected exactly one") {
		t.Fatalf("stderr=%q", stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"missing.go"}, &out, &stderr); code == 0 || !strings.Contains(stderr.String(), "read") {
		t.Fatalf("missing file code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunGenerateOnly(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, tc := range []struct {
		name, body string
		wantCode   int
	}{
		{"valid", "/*goit:test\nimport \"testing\"\nfunc TestInline(t *testing.T) { t.Fatal(\"must not execute\") }\ngoit:end*/\n", 0},
		{"invalid syntax", "/*goit:test\nfunc TestInline( {\ngoit:end*/\n", 1},
		{"no block", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			filename := filepath.Join(dir, "source.go")
			source := []byte("package fixture\n" + tc.body)
			if err := os.WriteFile(filename, source, 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			if code := run([]string{"--generate-only", "--", filename}, &out, &stderr); code != tc.wantCode {
				t.Fatalf("exit = %d; want %d; stderr = %q", code, tc.wantCode, stderr.String())
			}
			if tc.wantCode == 0 {
				path := strings.TrimSpace(out.String())
				if filepath.Dir(path) != dir || !strings.HasSuffix(path, "_test.go") {
					t.Fatalf("unexpected output path: %q", path)
				}
				generated, err := os.ReadFile(path)
				if err != nil || !bytes.Contains(generated, []byte("func TestInline")) || bytes.Contains(generated, []byte("goit:test")) {
					t.Fatalf("generated = %q, error = %v", generated, err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else if out.Len() != 0 {
				t.Fatalf("unexpected success output: %q", out.String())
			}
			got, err := os.ReadFile(filename)
			if err != nil || !bytes.Equal(got, source) {
				t.Fatalf("source changed: %q, %v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected artifacts: %v, %v", entries, err)
			}
		})
	}
}

func TestIntegrationCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test in disposable modules")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	for _, tc := range []struct {
		name, expected string
		wantCode       int
	}{
		{"success", "5", 0}, {"failure", "6", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, data string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", "module fixture\n\ngo 1.26.0\n")
			source := `package fixture
func sum(a,b int) int { return a+b }
/*goit:test
import "testing"
func TestSum(t *testing.T) { if got := sum(2,3); got != EXPECT { t.Fatalf("got %d, want EXPECT", got) } }
goit:end*/
`
			source = strings.ReplaceAll(source, "EXPECT", tc.expected)
			write("main.go", source)
			before, err := os.ReadFile(filepath.Join(dir, "main.go"))
			if err != nil {
				t.Fatal(err)
			}
			write("existing_test.go", "package fixture\n")
			beforeExisting, err := os.ReadFile(filepath.Join(dir, "existing_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			code := run([]string{filepath.Join(dir, "main.go")}, &out, &stderr)
			if code != tc.wantCode {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, tc.wantCode, out.String(), stderr.String())
			}
			if tc.wantCode == 0 && !strings.Contains(out.String(), "PASS") {
				t.Fatalf("missing PASS: %q", out.String())
			}
			if tc.wantCode != 0 && !strings.Contains(out.String()+stderr.String(), "FAIL") {
				t.Fatalf("missing failure output: stdout=%q stderr=%q", out.String(), stderr.String())
			}
			after, err := os.ReadFile(filepath.Join(dir, "main.go"))
			if err != nil {
				t.Fatal(err)
			}
			afterExisting, err := os.ReadFile(filepath.Join(dir, "existing_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !bytes.Equal(beforeExisting, afterExisting) {
				t.Fatal("CLI modified a source or preexisting test file")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if e.Name() == ".goit.lock" || strings.HasPrefix(e.Name(), ".goit-stage-") || strings.HasPrefix(e.Name(), "zz_goit_") {
					t.Fatalf("temporary test left behind: %s", e.Name())
				}
			}
		})
	}
}

func TestIntegrationNoInlineBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test in disposable modules")
	}
	dir := t.TempDir()
	filename := filepath.Join(dir, "plain.go")
	if err := os.WriteFile(filename, []byte("package plain\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run([]string{filename}, &out, &stderr); code == 0 || !strings.Contains(stderr.String(), "no inline tests") || strings.Contains(out.String(), "PASS") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}
