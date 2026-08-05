package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSemanticDigestIgnoresXMLFormatting(t *testing.T) {
	compact := []byte(`<dictionary><suite name="test" code="test"><command name="do thing" code="testdoth"><direct-parameter type="text"/><parameter name="with value" code="valu" type="integer"/></command><class name="item" code="cobj"/><enumeration name="mode" code="mode"><enumerator name="fast" code="fast"/></enumeration></suite></dictionary>`)
	formatted := []byte(`<dictionary>
  <suite name="test" code="test">
    <command code="testdoth" name="do thing">
      <direct-parameter type="text" />
      <parameter type="integer" code="valu" name="with value" />
    </command>
    <class code="cobj" name="item" />
    <enumeration code="mode" name="mode"><enumerator code="fast" name="fast" /></enumeration>
  </suite>
</dictionary>`)
	left, err := semanticDigest("left", compact)
	if err != nil {
		t.Fatal(err)
	}
	right, err := semanticDigest("right", formatted)
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("formatting changed semantic digest: %s != %s", left, right)
	}
	changed := []byte(`<dictionary><suite name="test" code="test"><command name="different" code="testdoth"/></suite></dictionary>`)
	changedDigest, err := semanticDigest("changed", changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == left {
		t.Fatal("terminology change retained semantic digest")
	}
}

func TestRemoveUnlistedSnapshotsMirrorsManifest(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{
		"Finder.sdef":   "current",
		"Unlisted.sdef": "unlisted",
		"README.txt":    "unrelated",
		"nested.sdefx":  "unrelated",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeUnlistedSnapshots(dir, map[string]bool{"Finder": true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Finder.sdef", "README.txt", "nested.sdefx"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should remain: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Unlisted.sdef")); !os.IsNotExist(err) {
		t.Errorf("unlisted SDEF still exists: %v", err)
	}
}

func TestSourceManifestCoversBundledSnapshots(t *testing.T) {
	terminologyDir := filepath.Join("..", "..", "terminology")
	raw, err := os.ReadFile(filepath.Join(terminologyDir, "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sources manifest
	if err := json.Unmarshal(raw, &sources); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(terminologyDir, "data", "*.sdef"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources.Entries) != len(paths) {
		t.Fatalf("manifest entries=%d snapshots=%d", len(sources.Entries), len(paths))
	}
	for _, source := range sources.Entries {
		if source.Name == "" || source.BundleID == "" || len(source.Paths) == 0 {
			t.Errorf("incomplete source entry: %#v", source)
			continue
		}
		if _, err := os.Stat(filepath.Join(terminologyDir, "data", source.Name+".sdef")); err != nil {
			t.Errorf("%s has no bundled snapshot: %v", source.Name, err)
		}
	}
}

func TestCommittedProvenanceMatchesSnapshots(t *testing.T) {
	terminologyDir := filepath.Join("..", "..", "terminology")
	manifestRaw, err := os.ReadFile(filepath.Join(terminologyDir, "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sources manifest
	if err := json.Unmarshal(manifestRaw, &sources); err != nil {
		t.Fatal(err)
	}
	expectedBundles := make(map[string]string, len(sources.Entries))
	for _, source := range sources.Entries {
		expectedBundles[source.Name] = source.BundleID
	}
	raw, err := os.ReadFile(filepath.Join(terminologyDir, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded report
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.SchemaVersion != 3 {
		t.Fatalf("provenance schema version = %d, want 3", recorded.SchemaVersion)
	}
	if recorded.Host.ProductVersion == "" || recorded.Host.BuildVersion == "" || recorded.Host.Architecture == "" || recorded.Host.SdefPath == "" {
		t.Fatalf("incomplete provenance host: %#v", recorded.Host)
	}
	if recorded.Language.Source != "OSAGetSysTerminology" || recorded.Language.SHA256 == "" {
		t.Fatalf("incomplete language provenance: %#v", recorded.Language)
	}
	language, err := os.ReadFile(filepath.Join(terminologyDir, "language.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(language); got != recorded.Language.GeneratedSHA256 {
		t.Fatalf("language hash = %s, provenance records %s", got, recorded.Language.GeneratedSHA256)
	}
	paths, err := filepath.Glob(filepath.Join(terminologyDir, "data", "*.sdef"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded.Entries) != len(paths) {
		t.Fatalf("provenance entries=%d snapshots=%d", len(recorded.Entries), len(paths))
	}
	seen := make(map[string]bool)
	for _, entry := range recorded.Entries {
		if seen[entry.Name] {
			t.Errorf("duplicate provenance entry %q", entry.Name)
		}
		seen[entry.Name] = true
		if entry.BundleID == "" || entry.BundleID != expectedBundles[entry.Name] {
			t.Errorf("%s generated bundle %q does not match manifest %q", entry.Name, entry.BundleID, expectedBundles[entry.Name])
		}
		if entry.BundleVersion == "" {
			t.Errorf("%s has no recorded bundle version", entry.Name)
		}
		if entry.SHA256 == "" || entry.SemanticSHA256 == "" {
			t.Errorf("%s has incomplete generated hashes", entry.Name)
		}
		snapshot, err := os.ReadFile(filepath.Join(terminologyDir, "data", entry.Name+".sdef"))
		if err != nil {
			t.Errorf("read %s snapshot: %v", entry.Name, err)
			continue
		}
		if got := digest(snapshot); got != entry.SHA256 {
			t.Errorf("%s snapshot hash = %s, provenance records %s", entry.Name, got, entry.SHA256)
		}
		terms, err := semanticDigest(entry.Name, snapshot)
		if err != nil {
			t.Errorf("parse %s snapshot: %v", entry.Name, err)
		} else if terms != entry.SemanticSHA256 {
			t.Errorf("%s semantic hash = %s, provenance records %s", entry.Name, terms, entry.SemanticSHA256)
		}
	}
}
