// Package generator turns parsed inline tests into Go test source.
package generator

import (
	"fmt"
	goparser "go/parser"
	"go/token"

	"goit/internal/parser"
)

// Generate emits a test file using the package and exact test-body bytes from result.
func Generate(result *parser.Result) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("generate test source: result is nil")
	}
	if result.Block == nil {
		return nil, fmt.Errorf("generate test source: result has no inline test block")
	}
	if !token.IsIdentifier(result.PackageName) || token.Lookup(result.PackageName).IsKeyword() {
		return nil, fmt.Errorf("generate test source: invalid package name %q", result.PackageName)
	}

	output := make([]byte, 0, len(result.PackageName)+len(result.Block.TestSource)+12)
	output = append(output, "package "...)
	output = append(output, result.PackageName...)
	output = append(output, '\n', '\n')
	output = append(output, result.Block.TestSource...)
	if len(output) > 0 && output[len(output)-1] != '\n' {
		output = append(output, '\n')
	}

	if _, err := goparser.ParseFile(token.NewFileSet(), "generated_test.go", output, goparser.AllErrors|goparser.SkipObjectResolution); err != nil {
		return nil, fmt.Errorf("generate test source: generated Go syntax is invalid: %w", err)
	}
	return output, nil
}
