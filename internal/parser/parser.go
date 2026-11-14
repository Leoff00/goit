// Package parser parses Go files containing an optional goit inline-test block.
package parser

import (
	"bytes"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"
	"unicode"
)

// Position is a physical byte position in the original source.
type Position struct{ Offset, Line, Column int }

// Span is a half-open source range.
type Span struct{ Start, End Position }

// Block describes the inline-test comment and its exact body bytes.
type Block struct {
	Span, BodySpan Span
	TestSource     []byte
	TestNames      []string
}

// Result describes a parsed Go source file.
type Result struct {
	Filename, PackageName string
	Block                 *Block
}

// ErrorCode is a stable parser diagnostic identifier.
type ErrorCode string

const (
	ErrSourceSyntax   ErrorCode = "E_SOURCE_SYNTAX"
	ErrInvalidStart   ErrorCode = "E_INVALID_START"
	ErrMissingEnd     ErrorCode = "E_MISSING_END"
	ErrUnexpectedEnd  ErrorCode = "E_UNEXPECTED_END"
	ErrMultipleBlocks ErrorCode = "E_MULTIPLE_BLOCKS"
	ErrInvalidScope   ErrorCode = "E_INVALID_SCOPE"
	ErrEmptyBlock     ErrorCode = "E_EMPTY_BLOCK"
	ErrTestSyntax     ErrorCode = "E_TEST_SYNTAX"
	ErrNoTests        ErrorCode = "E_NO_TESTS"
)

// ParseError is a machine-inspectable diagnostic with a physical source position.
type ParseError struct {
	Code     ErrorCode
	Filename string
	Position Position
	Message  string
	Cause    error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", e.Filename, e.Position.Line, e.Position.Column, e.Code, e.Message)
}
func (e *ParseError) Unwrap() error { return e.Cause }

func position(src []byte, off int) Position {
	if off < 0 {
		off = 0
	}
	if off > len(src) {
		off = len(src)
	}
	line, start := 1, 0
	for i := 0; i < off; i++ {
		if src[i] == '\n' {
			line++
			start = i + 1
		}
	}
	return Position{off, line, off - start + 1}
}
func diagnostic(code ErrorCode, file string, src []byte, off int, msg string, cause error) (*Result, error) {
	return nil, &ParseError{Code: code, Filename: file, Position: position(src, off), Message: msg, Cause: cause}
}

type candidate struct {
	start, end int
	open       bool
}

// Parse validates source bytes without filesystem access or mutation.
func Parse(filename string, source []byte) (*Result, error) {
	if source == nil {
		source = []byte{}
	}
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, filename, source, goparser.ParseComments|goparser.AllErrors|goparser.SkipObjectResolution)
	if err != nil {
		off := 0
		var es scanner.ErrorList
		if errorsAsScanner(err, &es) && len(es) > 0 {
			sort.SliceStable(es, func(i, j int) bool { return es[i].Pos.Offset < es[j].Pos.Offset })
			off = es[0].Pos.Offset
		}
		return diagnostic(ErrSourceSyntax, filename, source, off, "invalid Go source", err)
	}
	result := &Result{Filename: filename, PackageName: file.Name.Name}
	var candidates []candidate
	for _, group := range file.Comments {
		for _, comment := range group.List {
			a := fset.PositionFor(comment.Pos(), false).Offset
			b := a + len(comment.Text)
			if strings.HasPrefix(comment.Text, "/*") {
				if i := bytes.Index(source[a:], []byte("*/")); i >= 0 {
					b = a + i + 2
				}
			}
			if b > len(source) {
				b = len(source)
			}
			content := source[a:b]
			if bytes.HasPrefix(content, []byte("/*")) {
				content = content[2:]
				if len(content) >= 2 && bytes.HasSuffix(content, []byte("*/")) {
					content = content[:len(content)-2]
				}
			} else if bytes.HasPrefix(content, []byte("//")) {
				content = content[2:]
			}
			trim := bytes.TrimLeft(content, " \t\r\n")
			if strings.HasPrefix(comment.Text, "/*") && bytes.HasPrefix(trim, []byte("goit:test")) {
				candidates = append(candidates, candidate{a, b, true})
				if i := bytes.LastIndex(source[a:b], []byte("goit:end*/")); i >= 0 {
					candidates = append(candidates, candidate{a + i, b, false})
				}
			} else if bytes.HasPrefix(trim, []byte("goit:test")) {
				candidates = append(candidates, candidate{a, b, true})
			} else if bytes.HasPrefix(trim, []byte("goit:end")) {
				candidates = append(candidates, candidate{a, b, false})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].start < candidates[j].start })
	var opens, ends []candidate
	for _, c := range candidates {
		if c.open {
			opens = append(opens, c)
		} else {
			ends = append(ends, c)
		}
	}
	if len(opens) > 1 {
		return diagnostic(ErrMultipleBlocks, filename, source, opens[1].start, "multiple opening markers", nil)
	}
	if len(opens) == 0 {
		if len(ends) > 0 {
			return diagnostic(ErrUnexpectedEnd, filename, source, ends[0].start, "closing marker without opening marker", nil)
		}
		return result, nil
	}
	open := opens[0]
	openBody := open.start + 2 + len("goit:test")
	if openBody >= open.end || (source[openBody] != '\n' && !(source[openBody] == '\r' && openBody+1 < open.end && source[openBody+1] == '\n')) {
		return diagnostic(ErrInvalidStart, filename, source, open.start, "opening marker must be exact and followed by a newline", nil)
	}
	if source[openBody] == '\r' {
		openBody++
	}
	openBody++
	openLineEnd := openBody - 1
	if openBody >= 2 && source[openBody-2] == '\r' {
		openLineEnd--
	}
	if !markerLine(source, open.start, openLineEnd) {
		return diagnostic(ErrInvalidStart, filename, source, open.start, "opening marker must occupy its own line", nil)
	}
	var close *candidate
	for i := range ends {
		if ends[i].start > open.start {
			close = &ends[i]
			break
		}
	}
	if close == nil {
		return diagnostic(ErrMissingEnd, filename, source, open.start, "missing closing marker", nil)
	}
	if len(ends) > 1 {
		for _, e := range ends {
			if e.start < open.start || e.start > close.start {
				return diagnostic(ErrUnexpectedEnd, filename, source, e.start, "isolated closing marker", nil)
			}
		}
	}
	if close.start < open.start+2 || !bytes.Equal(source[close.start:close.start+len("goit:end*/")], []byte("goit:end*/")) || !markerLine(source, close.start, close.start+len("goit:end*/")) {
		return diagnostic(ErrMissingEnd, filename, source, open.start, "closing marker must be exact and occupy its own line", nil)
	}
	bodyEnd := close.start
	if bodyEnd > openBody && source[bodyEnd-1] == '\n' {
		bodyEnd--
		if bodyEnd > openBody && source[bodyEnd-1] == '\r' {
			bodyEnd--
		}
	}
	if bodyEnd < openBody {
		bodyEnd = openBody
	}
	for _, d := range file.Decls {
		a := fset.PositionFor(d.Pos(), false).Offset
		b := fset.PositionFor(d.End(), false).Offset
		if open.start >= a && open.start < b {
			return diagnostic(ErrInvalidScope, filename, source, open.start, "inline block is inside a declaration", nil)
		}
		if a > open.start && a < close.end {
			return diagnostic(ErrInvalidScope, filename, source, open.start, "inline block overlaps a declaration", nil)
		}
	}
	if open.start < fset.PositionFor(file.Name.End(), false).Offset {
		return diagnostic(ErrInvalidScope, filename, source, open.start, "inline block precedes package clause", nil)
	}
	if len(bytes.TrimSpace(source[openBody:bodyEnd])) == 0 {
		return diagnostic(ErrEmptyBlock, filename, source, openBody, "inline-test body is empty", nil)
	}
	prefix := []byte("package " + file.Name.Name + "\n\n")
	virtual := make([]byte, 0, len(prefix)+bodyEnd-openBody)
	virtual = append(virtual, prefix...)
	virtual = append(virtual, source[openBody:bodyEnd]...)
	vset := token.NewFileSet()
	vf, verr := goparser.ParseFile(vset, "inline.go", virtual, goparser.AllErrors|goparser.SkipObjectResolution)
	if verr != nil {
		off := 0
		var es scanner.ErrorList
		if errorsAsScanner(verr, &es) && len(es) > 0 {
			sort.SliceStable(es, func(i, j int) bool { return es[i].Pos.Offset < es[j].Pos.Offset })
			off = es[0].Pos.Offset - len(prefix)
		}
		if off < 0 {
			off = 0
		}
		if off > bodyEnd-openBody {
			off = bodyEnd - openBody
		}
		return diagnostic(ErrTestSyntax, filename, source, openBody+off, "invalid inline-test declarations", verr)
	}
	var names []string
	for _, d := range vf.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		r := []rune(fn.Name.Name[len("Test"):])
		if len(r) == 0 || !unicode.IsLower(r[0]) {
			names = append(names, fn.Name.Name)
		}
	}
	if len(names) == 0 {
		return diagnostic(ErrNoTests, filename, source, openBody, "no discoverable Test functions", nil)
	}
	result.Block = &Block{Span: Span{position(source, open.start), position(source, close.end)}, BodySpan: Span{position(source, openBody), position(source, bodyEnd)}, TestSource: append([]byte(nil), source[openBody:bodyEnd]...), TestNames: names}
	return result, nil
}

func markerLine(src []byte, a, b int) bool {
	start := bytes.LastIndexByte(src[:a], '\n') + 1
	before := bytes.Trim(src[start:a], "\r\t ")
	lineEnd := bytes.IndexByte(src[b:], '\n')
	if lineEnd < 0 {
		lineEnd = len(src) - b
	}
	after := src[b : b+lineEnd]
	if len(after) > 0 && after[len(after)-1] == '\r' {
		after = after[:len(after)-1]
	}
	return len(before) == 0 && len(after) == 0
}
func errorsAsScanner(err error, out *scanner.ErrorList) bool {
	switch e := err.(type) {
	case scanner.ErrorList:
		*out = e
		return true
	case *scanner.ErrorList:
		*out = *e
		return true
	}
	return false
}
