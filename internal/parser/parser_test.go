package parser

import (
	"bytes"
	"errors"
	"testing"
)

func TestParseValidAndPreservesBody(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src := []byte("package p\nvar s = `/*goit:test*/`\n/*goit:test" + nl + "import \"testing\"\nfunc Test(t *testing.T) {}\nfunc TestHTTP200(t *testing.T) {}\n" + nl + "goit:end*/\n")
		before := append([]byte(nil), src...)
		r, err := Parse("absent.go", src)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !bytes.Equal(src, before) {
			t.Fatal("input mutated")
		}
		if r.PackageName != "p" || r.Block == nil {
			t.Fatalf("unexpected result: %#v", r)
		}
		if got, want := string(src[r.Block.BodySpan.Start.Offset:r.Block.BodySpan.End.Offset]), string(r.Block.TestSource); got != want {
			t.Fatalf("body mismatch: %q != %q", got, want)
		}
		if len(r.Block.TestNames) != 2 || r.Block.TestNames[0] != "Test" || r.Block.TestNames[1] != "TestHTTP200" {
			t.Fatalf("names: %v", r.Block.TestNames)
		}
		r.Block.TestSource[0] ^= 1
		if !bytes.Equal(src, before) {
			t.Fatal("body aliases input")
		}
	}
}

func TestParseDiagnostics(t *testing.T) {
	cases := []struct {
		name, src string
		code      ErrorCode
	}{
		{"empty source", "", ErrSourceSyntax}, {"source syntax", "package p\nfunc {", ErrSourceSyntax},
		{"no marker", "package p\nvar s=\"goit:test\"", ""},
		{"invalid opening", "package p\n/*goit:test extra\n*/", ErrInvalidStart},
		{"missing end", "package p\n/*goit:test\nfunc TestA(){}\n*/", ErrMissingEnd},
		{"isolated end", "package p\n/*goit:end*/", ErrUnexpectedEnd},
		{"empty", "package p\n/*goit:test\n\ngoit:end*/", ErrEmptyBlock},
		{"no tests", "package p\n/*goit:test\nfunc helper(){}\ngoit:end*/", ErrNoTests},
		{"test syntax", "package p\n/*goit:test\nfunc TestA(\ngoit:end*/", ErrTestSyntax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("fixture.go", []byte(tc.src))
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("not ParseError: %T", err)
			}
			if pe.Code != tc.code {
				t.Fatalf("code %s, want %s (%v)", pe.Code, tc.code, err)
			}
		})
	}
}

func TestValidSourceWithoutBlockAndIndependentResults(t *testing.T) {
	source := []byte("package p\n// prose mentions goit:test\nvar s = `goit:end`\n")
	first, err := Parse("missing-file.go", source)
	if err != nil {
		t.Fatal(err)
	}
	if first.Block != nil || first.PackageName != "p" {
		t.Fatalf("unexpected result: %#v", first)
	}
	second, err := Parse("missing-file.go", source)
	if err != nil {
		t.Fatal(err)
	}
	if first.PackageName != second.PackageName || first.Filename != second.Filename || second.Block != nil {
		t.Fatal("repeated parse differed")
	}
}

func TestNamesAndPhysicalLine(t *testing.T) {
	src := []byte("//line virtual.go:90\npackage p\n/*goit:test\nfunc Testuser(){}\nfunc TestÉ(){}\nfunc (x T) TestMethod(){}\nfunc Testicular(){}\ngoit:end*/")
	r, err := Parse("physical.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.Block.TestNames, []string{"TestÉ"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("names: %v", got)
	}
}

func TestFailuresAndNoIOContract(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code ErrorCode
	}{
		{"no filesystem fallback", "", ErrSourceSyntax},
		{"multiple", "package p\n/*goit:test\nfunc TestA(){}\ngoit:end*/\n/*goit:test\nfunc TestB(){}\ngoit:end*/", ErrMultipleBlocks},
		{"scope", "package p\nfunc f(){\n/*goit:test\nfunc TestA(){}\ngoit:end*/\n}", ErrInvalidScope},
		{"stray end after block", "package p\n/*goit:test\nfunc TestA(){}\ngoit:end*/\n/*goit:end*/", ErrUnexpectedEnd},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("/definitely/not/a/real/source.go", []byte(tc.src))
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Code != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}

func TestVirtualSyntaxErrorMapsToPhysicalSource(t *testing.T) {
	src := []byte("package p\n/*goit:test\nfunc TestA() {}\nfunc TestB(\ngoit:end*/")
	_, err := Parse("physical.go", src)
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Code != ErrTestSyntax || pe.Filename != "physical.go" || pe.Position.Line != 4 {
		t.Fatalf("unexpected mapped diagnostic: %#v (%v)", pe, err)
	}
}
