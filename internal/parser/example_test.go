package parser_test

import (
	"bytes"
	"os"
	"slices"
	"testing"

	"goit/internal/parser"
)

func TestParseInlineExample(t *testing.T) {
	const filename = "../../examples/sum/main.go"
	source, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	result, err := parser.Parse(filename, source)
	if err != nil {
		t.Fatal(err)
	}
	if result.PackageName != "main" {
		t.Fatalf("package = %q, want main", result.PackageName)
	}
	if result.Block == nil {
		t.Fatal("inline block not found")
	}
	block := result.Block
	if want := []string{"TestBirth", "TestSum", "TestSumNegative"}; !slices.Equal(block.TestNames, want) {
		t.Fatalf("test names = %v, want %v", block.TestNames, want)
	}
	if !bytes.Equal(block.TestSource, source[block.BodySpan.Start.Offset:block.BodySpan.End.Offset]) {
		t.Fatal("extracted body differs from source bytes")
	}
	t.Logf("package: %s; discovered tests: %v", result.PackageName, block.TestNames)
	t.Logf("extracted body:\n%s", block.TestSource)
}
