# AppleScript Tools

`applescript-tools` is an AppleScript compiled-script (`.scpt`) disassembler, decompiler and deobfuscator written in Go. It supports run-only scripts, embedded `scpt` descriptors, exact Apple event codes, application terminology, nested script objects, and best-effort recovery from damaged serialized data.

The implementation uses a typed FAS value graph, a decoded instruction layer shared by both tools, a typed AST, explicit transformation passes, and a formatter independent of binary parsing.

## Build and use

Go 1.24 or newer is required.

```sh
go build -o applescript-tools ./cmd/applescript-tools

./applescript-tools decompile example.scpt
./applescript-tools decompile --strict --pass strings example.scpt
./applescript-tools disassemble example.scpt
./applescript-tools disassemble --json example.scpt
```

Both commands accept `-` for stdin. Decompiled source or disassembly is written to stdout; diagnostics are written to stderr. Recovery is best-effort unless `--strict` is supplied. Embedded compiled scripts are unwrapped by default and can be retained with `decompile --no-unwrap`.

Available AST passes are:

- `strings`: folds ASCII-character calls, literal runs within proven-text concatenations, and locally assigned constant strings when scope, type, and control flow prove their value. List shape and concatenations containing untyped operands are preserved.

## Go API

```go
result, err := applescript.Decompile(ctx, reader, applescript.DecompileOptions{
    Strict: true,
})

listing, err := applescript.Disassemble(ctx, reader, applescript.DisassembleOptions{
    Strict: true,
})
```

The public `ast`, `terminology`, and `passes` packages support structured AST processing, custom SDEF dictionaries, and custom transformations. Library operations return diagnostics and never write to process output.

## Validation

Fast, platform-independent tests include all 58 focused compiled fixtures:

```sh
go test -short ./...
go vet ./...
go test -race -short ./...
```

On macOS, the normal suite automatically uses the local `osacompile` command to compile and round-trip every focused fixture and the embedded samples:

```sh
go test ./...
```

Setting `APPLESCRIPT_COMPILER_ORACLE_DIR` selects an external compiler service on any platform. Without it, macOS uses local `osacompile`; other platforms skip compiler-oracle tests.

```sh
APPLESCRIPT_COMPILER_ORACLE_DIR=/path/to/shared/workdir go test ./...
```

The repository includes a macOS host-side directory service at `tools/osacompile-service/watch.py`. Run it against a folder shared with the machine running the Go tests:

```sh
python3 tools/osacompile-service/watch.py /path/to/shared/workdir
```

Then select that folder from the test machine:

```sh
APPLESCRIPT_COMPILER_ORACLE_DIR=/path/to/shared/workdir go test ./...
```

The watcher publishes `.scpt` and `.log` files atomically. The `.log` file is the completion signal consumed by the external oracle backend.

Committed compiled fixtures and reviewed Go source goldens provide platform-independent regressions. When a compiler oracle is available, every authored oracle source must compile to the same canonical Go output and remain stable over a second compile/decompile cycle.

Regenerate committed compiler fixtures only when intentionally updating the compiler baseline. Maintenance commands use local `osacompile` automatically on macOS and otherwise require `APPLESCRIPT_COMPILER_ORACLE_DIR`:

```sh
go run ./tools/generate-fixtures
```

Audit a source corpus through the same compile/decompile/recompile fixed-point checks:

```sh
go run ./tools/corpus-audit /path/to/corpus report.md
```

Some examples:
- https://github.com/kevin-funderburg/AppleScripts
- https://github.com/abbeycode/AppleScripts

## References

Based on work by:

- https://github.com/Jinmo/applescript-disassembler
- https://github.com/pberba/applescript-decompiler
