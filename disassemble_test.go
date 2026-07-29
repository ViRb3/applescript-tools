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
