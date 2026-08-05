# Terminology generation

The default registry embeds the 24 scripting definitions in `data/`. They are
generated from the applications listed in `sources.json`.

Regenerate the complete corpus and its provenance with:

```sh
go run ./tools/terminology-generate
go test ./...
```

The generator resolves each configured application, verifies its bundle
identifier and version, runs `/usr/bin/sdef`, validates and parses the resulting
SDEF, then makes the `.sdef` files in `data/` exactly match the manifest. It
preserves unrelated files. Generation fails if any source is missing, has an
unexpected bundle identifier, or cannot produce a valid dictionary. All
sources are extracted before files are written.

`provenance.json` is generated in the same invocation. It records the
generation environment, resolved application paths and versions, plus raw and
terminology-semantic SHA-256 hashes of each generated file.

Run the compiled-fixture and compiler-oracle suites after generation.

The handwritten AppleScript language terms in `registry.go` are a separate
source. They deliberately override conflicting application terms and must be
reviewed against the AppleScript framework/compiler rather than generated from
application SDEFs.
