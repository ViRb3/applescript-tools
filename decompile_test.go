package applescript

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"applescript-tools/passes"
)

func TestDemoDecompiles(t *testing.T) {
	f, err := os.Open("testdata/demo.scpt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result, err := Decompile(context.Background(), f, DecompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"on run", "collectEnvironmentInfo", "repeat", "end run"} {
		if !strings.Contains(result.Source, want) {
			t.Errorf("source missing %q:\n%s", want, result.Source)
		}
	}
}

func TestCompiledOracleFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/compiled/*.scpt")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 58 {
		t.Fatalf("found %d compiled fixtures, want 58", len(paths))
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			options := DecompileOptions{Strict: true}
			switch name {
			case "ascii_recovery", "naive_list_edges", "naive_empty_concat":
				options.Passes = []passes.Pass{passes.Strings{}}
			}
			result, err := Decompile(context.Background(), f, options)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join("testdata", "compiled", name+".expected.applescript"))
			if err != nil {
				t.Fatal(err)
			}
			if result.Source != string(expected) {
				t.Fatalf("source differs from committed semantic golden")
			}
		})
	}
}

func TestFixtureLayout(t *testing.T) {
	oracle, err := filepath.Glob("testdata/oracle/*.applescript")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := filepath.Glob("testdata/compiled/*.scpt")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.Glob("testdata/compiled/*.expected.applescript")
	if err != nil {
		t.Fatal(err)
	}
	if len(oracle) != 58 || len(compiled) != 58 || len(expected) != 58 {
		t.Fatalf("fixture counts: oracle=%d compiled=%d expected=%d; want 58 each", len(oracle), len(compiled), len(expected))
	}
	for _, source := range oracle {
		name := strings.TrimSuffix(filepath.Base(source), ".applescript")
		for _, path := range []string{
			filepath.Join("testdata", "compiled", name+".scpt"),
			filepath.Join("testdata", "compiled", name+".expected.applescript"),
		} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s has no matching fixture: %v", source, err)
			}
		}
	}
	if _, err := os.Stat("testdata/refinements.applescript"); !os.IsNotExist(err) {
		t.Errorf("obsolete combined refinement fixture exists or cannot be checked: %v", err)
	}
}

func TestSecconEmbeddedPayload(t *testing.T) {
	data, err := os.ReadFile("testdata/seccon.scpt")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "c32c327cf28404739a7873dae1bc8dfecc1d342f5484bd16338cc82b5d81712d" {
		t.Fatalf("SECCon sample digest = %s", got)
	}
	f, err := os.Open("testdata/seccon.scpt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result, err := Decompile(context.Background(), f, DecompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Embedded {
		t.Fatal("embedded payload was not detected")
	}
	for _, fragment := range []string{
		`use framework "Foundation"`,
		"use scripting additions",
		"considering numeric strings",
		"with timeout of 10 seconds",
		"copy codex to claude",
		"character id cid",
		"U1F99C:-1432",
		"U1F41C:-9063",
		"Pennsylvania div alaska",
		"virginia's |length|()",
		"(NSString of current application)'s stringWithString_(t)",
		"{space, tab, return, linefeed}",
		"U1F41D of tennessee",
		"on Iidabashi(candidate)",
		"on Roppongi(newyorkcity)",
		"on Otemachi(cid)",
		"on Kanda(nsPayload, NewJersey, U1F9E7, westchester)",
		"on Sugamo(nsPayload, NewJersey, U1F9E7)",
		"on Jimbocho(t)",
		"on trimNSString(ns)",
		"on splitNSString(ns, delim)",
		"on Shimbashi(washingtondc, colorado, idaho, kansas)",
		"on Ginza(t)",
		"on stripWhitespace(t)",
		"on split(t, delim)",
	} {
		if !strings.Contains(result.Source, fragment) {
			t.Errorf("embedded source missing %q", fragment)
		}
	}
	for _, unwanted := range []string{"parent_var_", ": ,", "«unsupported"} {
		if strings.Contains(result.Source, unwanted) {
			t.Errorf("embedded source contains %q", unwanted)
		}
	}

	wrapped, err := Decompile(context.Background(), bytes.NewReader(data), DecompileOptions{DisableEmbeddedUnwrap: true})
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.Embedded || !strings.Contains(wrapped.Source, "«data scpt") {
		t.Fatalf("wrapped source did not retain embedded data:\n%s", wrapped.Source)
	}
}
