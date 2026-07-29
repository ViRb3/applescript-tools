package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestUsageExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("exit code %d", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestDisassembleJSONFromStdin(t *testing.T) {
	data, err := os.ReadFile("../../testdata/demo.scpt")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"disassemble", "--json", "-"}, bytes.NewReader(data), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	for _, want := range []string{`"schema_version": 1`, `"mnemonic": "MessageSend"`, `"raw_hex"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("JSON missing %q", want)
		}
	}
}

func TestDecompileFromStdin(t *testing.T) {
	data, err := os.ReadFile("../../testdata/demo.scpt")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"decompile", "-"}, bytes.NewReader(data), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "on run") {
		t.Fatalf("source missing run handler")
	}
}
