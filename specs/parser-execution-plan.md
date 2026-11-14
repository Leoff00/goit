# goit — Parser execution plan for agents

**Reference contract:** Parser Contract Specification 0.1.0  
**Component:** `internal/parser`  
**Status:** completed (P0–P5; parser and validation implemented)

## 1. Goal and initial state

Implement `Parse(filename, source)` as a pure function that validates a Go file, recognizes a single inline block, preserves the body bytes, discovers test candidates, and returns physical positions and typed errors.

At the initial inspection, the repository contains only `AGENTS.md`. There are no `go.mod`, `README.md`, packages, or tests yet. P0 must prepare the minimal foundation before the implementation stages.

Use only the standard library. This delivery covers the parser and its documentation/tests. The CLI, generator, runner, cleanup, manifest, type checking, and inline test execution are outside this delivery's scope.

## 2. Contract all agents must preserve

### API

```go
package parser

type Position struct {
    Offset int
    Line   int
    Column int
}

type Span struct {
    Start Position
    End   Position
}

type Block struct {
    Span       Span
    BodySpan   Span
    TestSource []byte
    TestNames  []string
}

type Result struct {
    Filename    string
    PackageName string
    Block       *Block
}

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

type ParseError struct {
    Code     ErrorCode
    Filename string
    Position Position
    Message  string
    Cause    error
}

func Parse(filename string, source []byte) (*Result, error)
func (e *ParseError) Error() string
func (e *ParseError) Unwrap() error
```

### Invariants

- Success: non-nil result; `Block == nil` when there are no directives.
- Failure: nil result and an error accessible through `errors.As` as `*ParseError`.
- Input is preserved on every path; `TestSource` is an independent copy.
- `source[BodySpan.Start.Offset:BodySpan.End.Offset]` is identical to `TestSource`.
- Offsets and columns are byte-based; offsets start at zero, lines/columns at one; spans are half-open.
- Diagnostic positions and filenames refer to the original physical input.
- No I/O, subprocesses, mutable global state, or body normalization.
- Discover only package-level functions without receivers; names follow declaration order.
- Recognize `Test` and the prefix `Test` followed by a rune that is not a lowercase letter, using Go's Unicode rule.
- Compilation, types, imports, and final signature validation remain the toolchain's responsibility.

## 3. Contract decisions — finalize in P0

These decisions were adopted in P0 and guide the completed implementation. D1–D7 follow the proposals below; D8 was updated according to the maintainer's explicit choice. Corresponding success/failure examples are in `internal/parser/parser_test.go`.

| ID | Topic | Implementation proposal |
| --- | --- | --- |
| D1 | Body boundaries | Exclude the opening marker and its newline; exclude the newline immediately before the closing marker and the closing marker itself. Preserve all other newlines and whitespace. When the only newline directly separates the markers (`/*goit:test\ngoit:end*/`), return `E_EMPTY_BLOCK` with an empty span and no inverted offsets. |
| D2 | Candidates and ordinary comments | Recognize an attempted directive at the start of a comment's content, after optional whitespace, with the literal prefix `goit:test` or `goit:end`. The canonical form still requires exact bytes. Thus, `// goit:test`, `/* goit:test`, and `/*goit:test extra` produce `E_INVALID_START`; `// goit:end` produces `E_UNEXPECTED_END`. Prose that merely mentions directives and case differences are ignored. Do not search for isolated words in ordinary comment bodies. |
| D3 | Dedicated lines | Require markers on their own lines, without indentation, extra spaces, or adjacent code. Accept a closing marker at EOF without a final newline. Validate the bytes before and after the comment on the marker lines. An invalid opening produces `E_INVALID_START`; a noncanonical closing produces `E_MISSING_END`. |
| D4 | Scope | Accept comments between declarations and documentation comments outside declaration ranges. Reject comments inside any declaration, including grouped imports, structs, and signatures. Reject a block before the `package` clause with `E_INVALID_SCOPE`. |
| D5 | Diagnostic ties | After validating the original syntax, collect candidates before validating a block. If there are multiple openings, return `E_MULTIPLE_BLOCKS` at the second; otherwise, return `E_UNEXPECTED_END` at the first isolated closing. In other phases, use the first failure by physical offset. A closing belonging to an opening candidate is not isolated. |
| D6 | Empty body | Treat a zero-length body or a body containing only whitespace according to `bytes.TrimSpace` as empty, using that operation only as a predicate. Never use its result as the extracted content. A body containing only valid comments results in `E_NO_TESTS`. |
| D7 | Error locations | Opening/scope errors point to the start of the comment; a missing closing points to the opening; an isolated closing points to the directive; empty/no-test errors point to the start of the body. Syntax errors point to the parser's first physical error; an error at virtual EOF maps to the end of the body. |
| D8 | Module and Go version | `module goit` and `go 1.26.0`, explicitly specified by the maintainer. Verified toolchain: Go 1.26.2. Uses `parser.SkipObjectResolution`; no external dependencies. |

Add executable test examples for each decision. Do not introduce new error codes to resolve ambiguities without updating the contract.

## 4. Owners and dependencies

The names below represent agent roles, not processes that need to remain active simultaneously.

| Stage | Agent | Depends on | Main deliverable |
| --- | --- | --- | --- |
| P0 | Contract coordinator | — | Decisions D1–D8, module, and initial documentation |
| P1 | Foundation implementer | P0 | API, errors, original parsing, and positions |
| P2 | Directive implementer | P1 | Identification, structural validation, and extraction |
| P3 | Body implementer | P2 | Virtual validation and test discovery |
| P4 | Contract verifier | P1; full execution after P3 | Independent public API test suite |
| P5 | Integrator/reviewer | P3 and P4 | Review, checks, and final documentation |

```text
P0 → P1 → P2 → P3 ─┐
       └────→ P4 ───┼→ P5
```

P4 can prepare tests in parallel with P2/P3 after P1 stabilizes the API. The other steps are sequential to reduce artificial interfaces and conflicts.

### File ownership

- P0: `go.mod`, `README.md`, and decisions in this plan.
- P1–P3: production files and tests for each slice in `internal/parser/`, with sequential ownership handoff.
- P4: `internal/parser/contract_test.go`, preferably using the external `parser_test` package; do not edit production code in parallel.
- P5: integration and required fixes after parallel writes have ended.
- Do not create empty packages, a temporary CLI, or interfaces merely to distribute work.

Suggested production files: `parser.go`, `errors.go`, `positions.go`, `directives.go`, and `body.go`. Create each file only when its responsibility justifies the separation.

## 5. Work packages

### P0 — Prepare the foundation and finalize the contract — completed

**Agent instruction:** read `AGENTS.md`, this plan, and the provided contract. Reinspect existing files before editing, since the repository may have evolved.

1. Resolve D1–D8 and record expected examples.
2. Create `go.mod` with the confirmed identity/version and no external dependencies.
3. Create a minimal README explaining the parser's syntax, purity, and scope.
4. Record toolchain availability and any environment limitations.

**Acceptance:** explicit decisions, documented Go version, and no claims that the CLI is already implemented.

### P1 — API, errors, and original parsing — completed

**Agent instruction:** implement the first observable slice of `Parse`: valid input without a block and syntactically invalid input.

1. Implement types, constants, and Go doc comments in English.
2. Implement `ParseError.Error()` with the filename, position, code, and a readable English message; `Unwrap()` returns `Cause`.
3. Use an alias such as `goparser` to import `go/parser` without confusion with the local package.
4. Run `ParseFile` with `ParseComments | AllErrors | SkipObjectResolution` and reject partial ASTs on error.
5. Ensure `source == nil` is treated as empty in-memory content: `go/parser` may read `filename` when given a nil source. Never allow this fallback.
6. Build physical positions from offsets. Go `//line` directives must not redirect the API's filename, line, or column; use unadjusted positions such as `PositionFor(pos, false)` where applicable.
7. Convert syntax errors to `E_SOURCE_SYNTAX`, preserving the cause and deterministically selecting the first error by offset.

**Tests:** file without a block, nonexistent filename, nil/empty input, invalid syntax, `errors.As`, `errors.Unwrap`, Unicode/CRLF, and `//line` directives.

**Acceptance:** `go test ./internal/parser` passes for this slice; `Parse` neither accesses the filesystem nor accepts a partial AST.

### P2 — Directives, scope, and byte preservation — completed

**Agent instruction:** implement structural recognition using only real comments identified by the AST.

1. Enumerate comments in physical order and collect candidates according to D2.
2. Use the comment's physical start to recover its original bytes. Do not trust the length of `ast.Comment.Text` or `Comment.End()` as the raw end: the Go library may have removed CR characters from the text.
3. For block comments, locate the first Go terminator `*/` from the known start; for line comments, locate the physical end of the line. Restrict this search to real comments, without scanning globally for markers.
4. Resolve multiplicity and isolated closings before individual validation, according to D5.
5. Validate exact shape, LF/CRLF, dedicated lines, and body boundaries, including the empty case.
6. Check scope using declaration/`package` clause ranges, without relying on comments being associated with `Doc`.
7. Produce physical spans and copy the body without formatting, trimming, or newline conversion.
8. Apply `E_EMPTY_BLOCK` only after structural and scope checks.

**Tests:** fake markers in strings, ordinary comments, invalid openings, isolated/missing closings, multiple blocks, forbidden scopes, extra lines, LF/CRLF, EOF, exact spans, and independent copies.

**Acceptance:** the slice's tests pass and equality between the original slice and returned body is demonstrated. This intermediate slice still depends on P3 to fulfill the complete `Parse` contract.

### P3 — Virtual syntax and discovery — completed

**Agent instruction:** complete `Parse` without adding semantic validation.

1. Build `package <name>\n\n` in memory, followed by the exact body bytes.
2. Validate declarations with `go/parser`; a duplicate package clause and file-level statements produce `E_TEST_SYNTAX`.
3. Map virtual offsets to `BodySpan.Start.Offset + (virtualOffset - prefixLength)`, explicitly handling EOF errors and positions outside the body.
4. Recalculate line/column in the original file; do not fix diagnostics merely by subtracting lines or honor `//line` remapping.
5. Traverse declarations and select `*ast.FuncDecl` nodes without receivers according to Go's naming rule, using `unicode.IsLower` for the first rune after `Test`.
6. Preserve declaration order; return `E_NO_TESTS` when there are no candidates.
7. Accept syntactically valid code even when types, imports, or signatures can only be rejected by the toolchain. Do not enforce the full signature or add special semantic handling for `TestMain`.

**Tests:** imports, helpers, types, constants, and variables; `Test`, `TestUser`, `TestHTTP200`, `Testuser`, `Testicular`, Unicode names, methods, local functions, benchmarks/fuzz/examples without tests, an incorrect but syntactically valid signature, and mapped virtual errors.

**Acceptance:** the complete pipeline and P1–P3 tests pass, without `go/types`, code execution, or external access.

### P4 — Independent contract verification — completed

**Agent instruction:** test public behavior without coupling expectations to internal helpers.

1. Create table-driven cases with `t.Run` covering the matrix below.
2. Check for a nil result on every failure and a non-nil result on every success.
3. Compare input bytes before/after successes and failures.
4. Modify the returned `TestSource` and prove that the input and another independent result remain intact.
5. Verify equivalent repeated calls and concurrent calls with the same read-only input.
6. Use explicit expected offsets/lines/columns rather than replicating the production algorithm in tests.
7. Validate codes through `errors.As`, without relying solely on messages. Also verify physical diagnostic fields.
8. Hand reproducible failures to the implementer/integrator; run the full suite after P3.

**Acceptance:** all required scenarios have automated evidence and no fixture depends on the network, a home directory, or sleeps.

### P5 — Integrate, review, and deliver — completed

**Agent instruction:** review the implementation against the contract and run applicable checks.

1. Resolve P4 failures with relevant regression tests.
2. Audit indirect I/O, shared state, byte preservation, CRLF, `//line`, and error precedence.
3. Confirm that `Result` provides the package, body, and positions needed by the generator, without implementing the generator.
4. Synchronize the README with actual behavior and adopted decisions.
5. Run the commands in section 7 and record actual results.

**Acceptance:** the Definition of Done is satisfied, with a delivery report listing changed files, executed commands, and verifiable limitations.

## 6. Minimum verification matrix

| Group | Scenarios | Expectation |
| --- | --- | --- |
| Foundation | Valid Go without a block; filename without a physical file | Correct package; `Block == nil`; no I/O |
| Invalid source | Invalid Go, nil/empty input, unterminated comment | `E_SOURCE_SYNTAX` |
| Valid cases | One test; multiple tests; imports/helpers/types/constants/variables | Names in order and intact bytes |
| Preservation | LF, CRLF, blank lines, tabs, trailing spaces, Unicode before/in the body | Exact spans and content |
| False positives | Interpreted/raw strings; prose in comments; case differences | Ignored according to D2 |
| Opening | Line comment; space in the marker; extra suffix; adjacent code | `E_INVALID_START` |
| Closing | Missing in a terminated comment; malformed; isolated | `E_MISSING_END` / `E_UNEXPECTED_END` |
| Multiplicity | Two blocks; multiple openings with another defect | `E_MULTIPLE_BLOCKS` according to D5 |
| Scope | Function, struct, grouped import, signature; before the package | `E_INVALID_SCOPE` |
| Allowed scope | Between declarations; documentation comment in an allowed scope | Success |
| Empty | No bytes; shared newline; whitespace only | `E_EMPTY_BLOCK` |
| Extracted syntax | Second package clause; loose statements; truncated declaration | `E_TEST_SYNTAX` at the original location |
| No candidates | Helpers; comments only; TestXxx methods; only `Testuser` | `E_NO_TESTS` |
| Names | `Test`, `TestUser`, `TestHTTP200`, `Testicular`, uppercase/lowercase Unicode rune | Go's rule, without methods |
| Semantic boundary | Unknown type; unused import; incorrect test signature | Parsing may succeed |
| Outside discovery | Only Benchmark/Fuzz/Example | `E_NO_TESTS`; no special support |
| Diagnostics | Unicode, CRLF, `//line`, end of body, and virtual EOF | Physical filename and byte-based position |
| Precedence | Invalid source + invalid directive; multiple + format; format + scope; scope + empty; syntax + no tests | First applicable precedence level |
| Immutability | Success/failure; mutation of the returned copy; repeated/concurrent calls | Intact source, independent results, no races |
| Go limitation | `*/` sequence in the body, including inside a string | First terminator closes the Go comment; diagnostics follow original parsing and precedence, without repair |

Required precedence: original syntax → multiplicity/isolated closing → format → scope → empty body → body syntax → no tests.

## 7. Validation and handoff

During each slice, format only changed Go files with `gofmt -w <changed-files>` and run `go test ./internal/parser`.

During integration, from the root:

```sh
gofmt -l <changed-go-files>
go test ./internal/parser
go test ./...
go test -race ./...
go vet ./...
```

Replace the file placeholder with the actual list. `gofmt -l` output must be empty. Also run additional CI checks if any exist at implementation time. At completion, `make test-unit`, `go test ./...`, `go test -race ./...`, `go vet ./...`, `make fmt-check`, and `make check` were executed; all passed with Go 1.26.2.

`go build ./cmd/goit`, `--help`, `--version`, and the MVP executable smoke tests depend on the CLI, which is absent from this delivery. Record them as not applicable while that component does not exist, without creating an executable merely to satisfy the command. Compilation and execution of generated inline tests belong to the future parser/generator/runner integration.

Each agent delivers:

1. Completed stage and changed files.
2. Applied decisions and any discrepancies found.
3. Executed commands and their results.
4. Concrete pending items for the next agent.

## 8. Definition of Done

- [x] API and codes implemented according to section 2.
- [x] Decisions D1–D8 recorded; D8 follows the version explicitly specified by the maintainer.
- [x] Comments identified by the AST, extraction performed on original bytes.
- [x] Markers, uniqueness, and scope validated deterministically.
- [x] Independent body without normalization, with physical spans.
- [x] Virtual syntax and name discovery without type checking.
- [x] Typed errors with causes and original positions.
- [x] Nil input handled in memory; parser performs no I/O.
- [x] Listed tests and checks executed; see task history/report.
- [x] README synchronized and no out-of-scope implementation added.
- [x] Structured result available to the future generator.
