# goit — inline tests for Go

`goit` runs tests written inside a `/*goit:test ... goit:end*/` comment without editing the original Go file. The parser validates and extracts the block; the runner creates a temporary test file and delegates execution to `go test`.

## Build and usage

```sh
make build
./bin/goit --help
./bin/goit examples/sum/main.go
# or: make test-inline
```

`go run ./examples/sum` still runs the program normally. Conventional `go test` does not discover inline tests that have not been materialized.

The example adds `2 + 3` and expects `5`. To see an actual failure, change the expected value from `5` to `6` in `TestSum` and run `./bin/goit examples/sum/main.go`; restore the value afterward. The command returns a nonzero status and displays the output from `go test`.

## Inspect the generated file

```sh
make generate-only FILE=examples/sum/main.go
# equivalent: ./bin/goit --generate-only examples/sum/main.go
```

The command exits after creating the complete `zz_goit_<id>_test.go` in the target file's directory and prints its path. It does not run tests and keeps the file for inspection; the staging file and lock are removed. Without `FILE`, it uses the example above.

After inspection, remove **only the file at the printed path** before running `goit` again in that package. While the file exists, conventional `go test` will also discover the materialized tests. Normal execution still removes its temporary files automatically.

## Checks

- `make test-example`: parser extraction test (`TestParseInlineExample`).
- `make test-unit`: fast tests (`go test -short ./...`).
- `make test-integration`: generator, runner, and CLI integrations in temporary modules.
- `make run FILE=path/to/file.go`: run inline tests from another file.
- `make test`, `make test-race`, `make vet`, `make fmt-check`, and `make check`.

The runner invokes `go test -count=1 -v .` in the target file's package: existing conventional tests in that package also run. A second invocation in the same package fails while the `.goit.lock` lock is active. Temporary files are cleaned up after normal completion, failure, or handled cancellation.

After `SIGKILL`, a system crash, or power loss, confirm that no `goit` or test process is still running. Inspect the files and manually remove only orphaned artifacts from that invocation (`zz_goit_*_test.go`, `.goit-stage-*`, and the empty `.goit.lock` directory). Never remove unknown files. The runner refuses to execute when a lock or generated test files remain.

Inline tests are executable code with the user's privileges; goit **is not a sandbox**. The module requires Go 1.26.0 and uses only the standard library. The current runner uses Unix process groups to cancel subprocesses as well; integration is validated on Linux.
