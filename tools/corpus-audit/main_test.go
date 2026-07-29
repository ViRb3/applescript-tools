package main

import (
	"path/filepath"
	"strings"
	"testing"

	applescript "applescript-tools"
)

func TestDecodeSource(t *testing.T) {
	if got, encoding := decodeSource([]byte("plain ✓")); got != "plain ✓" || encoding != "UTF-8" {
		t.Fatalf("UTF-8 decode = %q, %q", got, encoding)
	}
	if got, encoding := decodeSource([]byte{'c', 'a', 'f', 0x8e}); got != "café" || encoding != "MacRoman" {
		t.Fatalf("MacRoman decode = %q, %q", got, encoding)
	}
	if got, encoding := decodeSource([]byte{'1', ' ', 0xb2, ' ', '2'}); got != "1 ≤ 2" || encoding != "MacRoman" {
		t.Fatalf("MacRoman operator decode = %q, %q", got, encoding)
	}
}

func TestSemanticOpcodeSignature(t *testing.T) {
	got := semanticSignature([]string{"TestIf", "MessageSend", "Add", "Return"})
	if !equalStrings(got, []string{"TestIf", "MessageSend", "Add", "Return"}) {
		t.Fatalf("semantic signature = %#v", got)
	}
	got = semanticSignature([]string{"SetData", "PopVariable", "PopParentVariable", "PopGlobal", "Return"})
	if !equalStrings(got, []string{"Return"}) {
		t.Fatalf("assignment-filtered signature = %#v", got)
	}
}

func TestCorpusIdentityFallback(t *testing.T) {
	root := t.TempDir()
	if got := corpusIdentity(root); got != filepath.Base(root) {
		t.Fatalf("identity = %q, want %q", got, filepath.Base(root))
	}
}

func TestSourceLineCount(t *testing.T) {
	for _, test := range []struct {
		source string
		want   int
	}{
		{"", 0},
		{"one", 1},
		{"one\n", 1},
		{"one\r\ntwo\r", 2},
	} {
		if got := sourceLineCount(test.source); got != test.want {
			t.Errorf("sourceLineCount(%q) = %d, want %d", test.source, got, test.want)
		}
	}
}

func TestMarkdownSummary(t *testing.T) {
	report := markdown(t.TempDir(), []result{
		{path: "ok.applescript", status: "pass"},
		{path: "old.applescript", status: "compiler-blocked"},
	})
	for _, want := range []string{"Entries audited: **2**", "`pass`: 1", "`compiler-blocked`: 1"} {
		if !strings.Contains(report, want) {
			t.Errorf("report does not contain %q", want)
		}
	}
}

func TestCompilerErrorContext(t *testing.T) {
	source := "one\ntwo\nthree\nfour\nfive\n"
	got := compilerErrorContext(source, "/tmp/test:4: error: bad")
	if !strings.Contains(got, "    4  four") {
		t.Fatalf("context = %q", got)
	}
}

func TestFirstDifference(t *testing.T) {
	got := firstDifference("same\nleft\n", "same\nright\n")
	if !strings.Contains(got, "line 2") || !strings.Contains(got, "left") || !strings.Contains(got, "right") {
		t.Fatalf("difference = %q", got)
	}
}

func TestDiagnosticsClean(t *testing.T) {
	if !diagnosticsClean(nil) {
		t.Fatal("empty diagnostics should be clean")
	}
	if diagnosticsClean([]applescript.Diagnostic{{Message: "unsupported object"}}) {
		t.Fatal("unsupported diagnostic should not be clean")
	}
	if !diagnosticsClean([]applescript.Diagnostic{{Message: "RefID mismatch"}}) {
		t.Fatal("recoverable parser diagnostic should be clean")
	}
}
