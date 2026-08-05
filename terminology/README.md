# Terminology generation

The default registry embeds two generated terminology sources:

- `language.json`, derived from `OSAGetSysTerminology` for the AppleScript
  language itself.
- The 24 application scripting definitions in `data/`, derived from the
  applications listed in `sources.json`.

Regenerate the complete corpus and its provenance with:

```sh
go run ./tools/terminology-generate
go test ./...
```

The generator obtains AppleScript's system terminology through the framework,
parses its AEUT structure into context-preserving commands, parameters, terms,
and enumerations, and writes `language.json`. It also resolves each configured
application, verifies its bundle identifier and version, runs `/usr/bin/sdef`,
validates and parses the resulting SDEF, then makes the `.sdef` files in `data/`
exactly match the manifest. Generation fails if any source is missing, has an
unexpected bundle identifier, or cannot produce a valid dictionary.

`provenance.json` is generated in the same invocation. It records the framework
terminology source and hashes, resolved application paths and versions, plus
raw and terminology-semantic SHA-256 hashes of each generated file.

Run the compiled-fixture and compiler-oracle suites after generation.
