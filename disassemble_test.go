package applescript

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDemoDisassembly(t *testing.T) {
	f, err := os.Open("testdata/demo.scpt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d, err := Disassemble(context.Background(), f, DisassembleOptions{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Functions) == 0 {
		t.Fatal("no functions")
	}
	text := d.Text()
	if strings.Contains(text, " owner=") || strings.Contains(text, " path=") {
		t.Fatalf("ordinary root disassembly gained nested ownership metadata:\n%s", text)
	}
	for _, want := range []string{
		"PushLiteral", "PushGlobal", "PopGlobal", "PushVariable", "PopVariable",
		"PushParentVariable", "PositionalMessageSend", "MessageSend",
		"MakeObjectAlias", "StoreResult", "LinkRepeat", "TestIf", "Jump", "Return",
		"ErrorHandler", "RepeatInRange",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("disassembly missing %q", want)
		}
	}
	expectedNames := map[int]string{
		5:  "collectEnvironmentInfo",
		6:  "deobfuscateStringList",
		7:  "demoRepeats",
		14: "escapeQuotes",
	}
	for offset, name := range expectedNames {
		found := false
		for _, function := range d.Functions {
			if function.Offset == offset && strings.Contains(function.Name, name) {
				found = true
			}
		}
		if !found {
			t.Errorf("function[%d] %q missing from stable demo shape", offset, name)
		}
	}
}

func TestNestedFunctionOwnershipText(t *testing.T) {
	d := &Disassembly{
		Version: "1.10",
		Functions: []DisassemblyFunction{{
			Offset: 8, Name: `"clicked_"`, Arguments: "nil",
			Owner: `"AppDelegate"`, Path: "root[2].vector[4][8]",
		}},
	}
	text := d.Text()
	for _, want := range []string{`owner="AppDelegate"`, "path=root[2].vector[4][8]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("nested disassembly missing %q:\n%s", want, text)
		}
	}
}
