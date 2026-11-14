# goit — Zed syntax highlighting execution plan

**Owner:** one GPT Luna agent, executing the stages sequentially.  
**Status:** planned; extension not yet implemented.  
**Scope:** visual feedback for code inside `/*goit:test` and `goit:end*/`.

## 1. Expected outcome

When opening `examples/sum/main.go` in Zed, users should be able to visually distinguish keywords, functions, types, strings, numbers, and comments inside the block. Markers should have their own theme-compatible style. Go outside the block should retain normal highlighting.

This editor integration was explicitly requested and extends the original scope of `AGENTS.md`. Do not change the parser contract or `goit` behavior to accommodate the editor.

The MVP is visual: do not implement a custom LSP, inline autocompletion, semantic diagnostics, automatic execution, test buttons, or integration with other editors. Highlighting does not mean that the parser accepted the block or that its tests passed.

## 2. Technical evidence and initial decision

Research into official documentation and upstream code on November 14, 2026:

1. Zed uses Tree-sitter, `highlights.scm`, and `injections.scm`. A language-only extension can work without custom Rust code.
2. In the language registry examined, an extension with the same name as a native language is rejected. Therefore, creating `languages/go/` with `name = "Go"` is not a mechanism for extending native queries.
3. The `tree-sitter-go` grammar represents a comment as a single token. A query does not gain access to `func`, string, or import nodes inside it.
4. Injecting Go over the entire comment keeps `/* ... */` in the content, which will be interpreted as a comment again. A range containing only the body must be exposed.
5. The injection configuration processing examined handles `injection.language` and `injection.combined`; support for trimming ranges through `#offset!` was not established. Do not automatically transfer Neovim/Helix examples to Zed.
6. The Go grammar examined accepts snippets without `package` at the root node. Validate this capability in the pinned revision; do not add a virtual prefix to the user's buffer.
7. Native Go already injects a comment language into `(comment)`. The implementation must prevent that rule from competing with Go injection over the inline body.

**Recommended strategy:** an opt-in extension with a distinct language, **Go (goit)**, a grammar derived from Go with a structured node for the block, and body injection into the existing Go language. Reuse the Go grammar and its queries with minimal changes and attribution/license notices; do not reimplement Go with regexes.

Do not automatically associate all `.go` files with the extension. The first delivery may require manually selecting **Go (goit)**. Document an explicit per-project association only after verifying the configuration supported by the target version.

**Decision point:** if an official mechanism for composing queries for native Go is demonstrated in the installed version, prefer it over a derived grammar. Record the evidence and actual body range. If the solution requires a Zed patch, classify it as an upstream contribution, not a ready-to-install extension.

## 3. Proposed organization

```text
extensions/zed-goit/
  extension.toml
  README.md
  languages/goit/
    config.toml
    highlights.scm
    injections.scm
    ...                    # auxiliary queries that are actually needed
  grammar/                 # derived grammar source, if this strategy is confirmed
  tests/
    corpus/
    highlights/
    fixtures/
  scripts/                 # only checks requiring custom automation
  THIRD_PARTY_NOTICES.md    # as required by reused upstream files
```

This tree is a proposal, not a list of empty files to create. Adapt the corpus to Tree-sitter's required structure. Use unique grammar/language names without replacing the native `go` grammar in the registry.

`extension.toml` references grammars by repository and revision. For local development, the documentation allows `file://`: verify the required Git layout and document explicit setup, without silently initializing repositories or editing global settings. Do not place fictitious URLs or absolute personal paths in the distributable manifest. Publishing requires a real repository and immutable revision; installing a dev extension is not publishing.

## 4. Stages for GPT Luna

### Z0 — Confirm feasibility in the target Zed version

1. Read `AGENTS.md`, this plan, `specs/parser-execution-plan.md`, the examples, and the current Makefile.
2. Record the available Zed version and versions of Tree-sitter, the grammar, and build tools. Distinguish upstream `main` code from the installed stable version.
3. Verify language registration, injections, ranges, and grammar loading mechanisms. Do not assume inheritance through `; inherits: go` or support for `#offset!`.
4. Prototype a file with a valid block: identify a real comment, expose the body without delimiters, and highlight `import`, `func`, a string, and a number.
5. Verify that the injected Go language is available and that the content neither becomes a single comment again nor causes recursive injections.
6. Check whether selecting **Go (goit)** preserves `gopls`/formatting outside the block. A distinct name does not automatically inherit the native LSP. Document observed behavior; do not create a full server or adapter to hide a visual integration limitation.

**Deliverable:** a short decision in the extension README, versions/revisions, and prototype reproduction steps. Proceed to packaging only after proving the body range and tokens. Without UI access, mark visual validation as pending.

### Z1 — Expose the body in the syntax tree

For the derived grammar strategy:

1. Pin the upstream revision, preserve its license, and record the minimal delta applied.
2. Create a conceptual `goit_test_block` node with opening, body, and closing children. Actual names are left to the implementation.
3. Preserve the ordinary Go grammar. Resolve lexical competition with the original comment token: it normally consumes the entire block before the new children are recognized. Adding a grammar node alone does not prove that it will be produced.
4. End the body before the correct closing marker. Respect Go's first `*/`, even when it appears inside a string written in the comment; do not reinterpret that terminator as allowed content.
5. Use an external scanner only if delimiting cannot be implemented correctly/simply with Tree-sitter rules. In that case, test EOF, incremental editing, CRLF, and state serialization when persistent state exists.
6. Do not recognize markers inside interpreted/raw strings or ordinary comment prose. Recognize the case-sensitive opening on its own line, with LF and CRLF support.
7. Capture positions from the original buffer, without rewriting bytes or normalizing line endings.

**Acceptance:** the corpus proves correct nodes and ranges, including false positives. `tree-sitter test` only provides grammar evidence; highlights/injections still need to be demonstrated in Zed.

### Z2 — Queries and editing behavior

1. Reuse relevant Go highlights outside the block, recording their origin and revision.
2. Capture markers in a supported theme category, such as `@preproc` or `@punctuation.special`; no hard-coded RGB colors.
3. Inject Go only into the body node. Conceptual example, to be adjusted to the actual generated names:

   ```scheme
   ((goit_test_block
      body: (goit_test_body) @injection.content)
    (#set! injection.language "go"))
   ```

4. Confirm resolution of `"go"` in the target version. Avoid `injection.combined`: distinct blocks must not accidentally share an injected parse.
5. Do not capture the entire block as `@comment` in a way that suppresses internal highlights. Preserve the behavior of ordinary comments and any reused Go injections.
6. Test snippets without `package`, imports, helpers, strings, numbers, operators, types, and line comments. Use Tree-sitter syntax recovery for incomplete code.
7. Define predictable editing behavior:
   - Valid block: full body highlighting.
   - Incomplete declaration inside a closed block: highlight recoverable tokens without requiring `parser.Parse` to succeed.
   - Opening without a closing: the first version may retain comment styling until the block is closed; do not leak Go highlighting outside the real comment. Record this choice.
   - Malformed marker: fall back to comment styling, without promising a diagnostic.
   - Multiple blocks or a block in an invalid scope: visual tolerance is allowed but does not imply CLI acceptance. Scope/uniqueness validation remains in the parser.

**Acceptance:** keywords, functions, and literals have their own captures; tests verify tokens and ranges, not merely the presence of text in the `.scm` file.

### Z3 — Package and install as a dev extension

1. Create `extension.toml` and the opt-in language configuration. Use real authorship/repository information; state local limitations when publication is not yet available.
2. Confirm whether the extension works with only a grammar and queries. Add Rust/Cargo only when justified by a demonstrated need.
3. Document installation through **Install Dev Extension** / `zed: install dev extension`, selecting the extension directory.
4. Document selecting **Go (goit)**, opening the example, and switching back to native Go/uninstalling.
5. Document the toolchain actually required. Current documentation mentions `wasi-sdk` for grammars and `wasm32-wasip2` for extension Rust code; do not confuse the two pipelines.
6. Check `zed: open log` for query/grammar errors. Use `zed --foreground` when needed for diagnosis.

**Acceptance:** reproducible installation on the recorded version, without modifying the Zed installation or editing `.go` files to enable highlighting.

### Z4 — Tests and Makefile

Add explicit targets after choosing and pinning the toolchain:

| Proposed target | Responsibility |
| --- | --- |
| `zed-help` | Explain dependencies and local installation |
| `zed-test-grammar` | Parsing and incremental editing corpus |
| `zed-test-highlights` | Capture/range assertions and injection body boundaries |
| `zed-check` | Automated extension checks |

Use real commands supported by the chosen Tree-sitter version. If `tree-sitter test` does not exercise incremental editing or Zed injections, add a specific test and document the difference. Do not invent a headless Zed API to make the target appear complete.

Invalid `.go` fixtures must live in `testdata` or `.txt` files to avoid breaking `go test ./...`. Do not run `gofmt` on CRLF/offset fixtures.

Keep the Go project's `make check` independent of Node/Rust/Tree-sitter. Isolate extension dependencies, pin their versions, and document their purpose; do not add libraries to the Go module for the editor. Avoid implicit downloads on every test run.

**Acceptance:** checks fail with a nonzero code when there is a problem or a missing tool; skipped/manual checks are declared and never reported as a visual PASS.

### Z5 — Validate the experience and deliver

1. Open `examples/sum/main.go`, select **Go (goit)**, and check colors in light and dark themes.
2. Type/remove part of `func`, a string, and the markers; confirm incremental updates and restoration after undo.
3. Open Go without directives and verify that ordinary comments/strings and external highlighting remain correct.
4. Record `gopls`, formatting, and language selection behavior, stating any limitations explicitly.
5. Confirm that installing/selecting the extension does not alter file bytes. Run `make test-inline` to demonstrate compatibility with the actual example.
6. Run `make check` and `make zed-check`. Record commands, versions, and results, separating automated tests from visual inspection.
7. Update the extension README and the index in `specs/`. Any visual evidence collected must come from the actual Zed UI, with the version/theme identified.

**Acceptance:** a short report covering files, installation, evidence, and limitations. Without Zed/UI access, Z5 remains pending and the visual goal cannot be declared complete.

## 5. Minimum test matrix

| Case | Result to demonstrate |
| --- | --- |
| Actual example `examples/sum/main.go` | Body with Go tokens; separate markers |
| Imports, helpers, types, and multiple TestXxx functions | Highlights equivalent to ordinary Go tokens |
| LF and CRLF; Unicode before and inside the block | Correct ranges without byte changes |
| Interpreted/raw string containing markers | No false goit block |
| Ordinary comment, TODO, and prose about goit | Normal comment styling |
| `//` comments inside the body | Comment styling inside injected Go |
| Malformed opening/closing | Documented fallback without leakage |
| Literal `*/` inside inline content | Respect the end of the Go comment |
| Empty block; snippet without a package clause | Stable editor and correct injection boundaries |
| Incomplete function/string | Visual recovery during editing |
| Two blocks and invalid scope | Documented visual behavior; CLI still validates |
| Code immediately after the block | No unintended injection capture |
| Consecutive edits and undo | Incremental tree consistent with a full parse |
| Light/dark theme and semantic tokens enabled/disabled | Record potential overlaps and supported configuration |
| Large file with many comments | No recursion or obvious degradation; record size and timing |
| Install/uninstall and native Go | Explicit, reversible activation |

## 6. Definition of Done

- [ ] Strategy demonstrated on the target version, with pinned revisions.
- [ ] Locally installable extension and documented activation.
- [ ] Body actually highlighted as Go, without delimiters in the injection.
- [ ] Visible markers with theme-compatible captures.
- [ ] False positives, CRLF, Unicode, and incomplete editing covered.
- [ ] No source changes or changes to goit's execution contract.
- [ ] Editor dependencies isolated from the Go project.
- [ ] Automated tests and real Makefile targets documented.
- [ ] Visual validation recorded or explicitly reported as pending.
- [ ] Native Go/gopls behavior and activation limitations recorded.

## 7. Verified references

Official documentation:

- [Developing Zed extensions](https://zed.dev/docs/extensions/developing-extensions).
- [Languages, highlights, and injections](https://zed.dev/docs/extensions/languages).
- [Tree-sitter query language](https://tree-sitter.github.io/tree-sitter/using-parsers/queries/index.html).

Research sources, pinned for reproducible reference (these are not guaranteed minimum Zed versions):

- [Language registration and rejection of native names](https://github.com/zed-industries/zed/blob/f0fa51bb510fc91f6da7ec55f3e0f317f1f975ec/crates/language/src/language_registry.rs).
- [Injection configuration](https://github.com/zed-industries/zed/blob/f0fa51bb510fc91f6da7ec55f3e0f317f1f975ec/crates/language_core/src/grammar.rs).
- [Existing Go injections in Zed](https://github.com/zed-industries/zed/blob/f0fa51bb510fc91f6da7ec55f3e0f317f1f975ec/crates/grammars/src/go/injections.scm).
- [Go grammar: comments and snippets](https://github.com/tree-sitter/tree-sitter-go/blob/2346a3ab1bb3857b48b29d779a1ef9799a248cd7/grammar.js).
