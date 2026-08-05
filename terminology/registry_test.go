package terminology

import (
	"strings"
	"testing"
)

const fixture = `<?xml version="1.0"?>
<dictionary><suite name="Port Suite" code="PORT">
  <command name="perform task" code="PORTtask">
    <direct-parameter type="text"/>
    <parameter name="quietly" code="quie" type="boolean"/>
    <parameter name="destination" code="dest" type="text"/>
    <parameter name="ignored without code"/>
  </command>
  <enumeration name="mode" code="mode"><enumerator name="fast mode" code="fast"/></enumeration>
  <class name="document" code="docu"><property name="title" code="ptit"/></class>
</suite></dictionary>`

func TestParseSDEF(t *testing.T) {
	d, err := ParseSDEF("Port", strings.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	event, _ := ParseEventCode("PORTtask")
	command := d.Commands[event]
	if command.Name != "perform task" || !command.HasDirectParameter {
		t.Fatalf("command = %#v", command)
	}
	quietly, _ := ParseCode4("quie")
	if command.Parameters[quietly].Name != "quietly" || command.Parameters[quietly].Type != "boolean" {
		t.Fatalf("parameter = %#v", command.Parameters[quietly])
	}
	destination, _ := ParseCode4("dest")
	if command.Parameters[destination].Name != "destination" || command.Parameters[destination].Type != "text" {
		t.Fatalf("parameter = %#v", command.Parameters[destination])
	}
	if len(command.Parameters) != 2 {
		t.Fatalf("parameters = %#v; parameter without code was not ignored", command.Parameters)
	}
	fast, _ := ParseCode4("fast")
	if d.Enums[fast] != "fast mode" {
		t.Fatalf("enum = %#v", d.Enums)
	}
	modeFast, _ := ParseEventCode("modefast")
	if d.Constants[modeFast] != "fast mode" {
		t.Fatalf("grouped enum = %#v", d.Constants)
	}
}

func TestBundledRegistry(t *testing.T) {
	r, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Dictionary("Finder"); !ok {
		t.Fatal("Finder dictionary missing")
	}
	if _, ok := r.Dictionary("Standard Additions"); !ok {
		t.Fatal("Standard Additions alias missing")
	}
	errn, _ := ParseCode4("errn")
	if got, ok := r.Parameter(errn); !ok || got != "number" {
		t.Fatalf("errn parameter = %q, %v", got, ok)
	}
}

func TestGeneratedLanguageTerminology(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		"txdl": "text item delimiters",
		"ascr": "AppleScript",
		"ldt ": "date",
		"citm": "text item",
		"obj ": "reference",
		"rvse": "reverse",
		"utxt": "Unicode text",
		"wkdy": "weekday",
		"ID  ": "id",
		"spac": "space",
	} {
		code, err := ParseCode4(text)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := registry.LanguageTerm(code); !ok || got != want {
			t.Errorf("LanguageTerm(%q) = %q, %v; want %q", text, got, ok, want)
		}
	}
	caseCode, _ := ParseCode4("case")
	if got, ok := registry.LanguageEnumeration(caseCode); !ok || got != "case" {
		t.Errorf("LanguageEnumeration(case) = %q, %v", got, ok)
	}
	consideringCase, _ := ParseEventCode("conscase")
	if got, ok := registry.Constant(consideringCase); !ok || got != "case" {
		t.Errorf("Constant(conscase) = %q, %v", got, ok)
	}
	expansion, _ := ParseCode4("expa")
	if _, ok := registry.LanguageTerm(expansion); ok {
		t.Error("framework terminology unexpectedly contains expa term")
	}
	if _, ok := registry.LanguageEnumeration(expansion); ok {
		t.Error("framework terminology unexpectedly contains expa enumeration")
	}
}

func TestLanguageBuiltinCommands(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for codeText, want := range map[string]string{
		"ascrnoop": "launch",
		"coredelo": "delete",
		"coredoex": "exists",
		"CoRedelo": "delete",
		"CoRedoex": "exists",
	} {
		code, err := ParseEventCode(codeText)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := registry.Command(code); !ok || got.Name != want {
			t.Errorf("Command(%q) = %#v, %v; want %q", codeText, got, ok, want)
		}
	}
}

func TestApplicationTermsAreNotOverwrittenByLanguageTerminology(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for codeText, want := range map[string]string{
		"rtyp": "rule type",
		"asup": "application support folder",
		"ID  ": "calendarIdentifier",
	} {
		code, _ := ParseCode4(codeText)
		if got, ok := registry.Term(code); !ok || got != want {
			t.Errorf("%s = %q, %v; want %q", codeText, got, ok, want)
		}
	}
	id, _ := ParseCode4("ID  ")
	if got, ok := registry.LanguageTerm(id); !ok || got != "id" {
		t.Errorf("language ID = %q, %v; want id", got, ok)
	}
	rtyp, _ := ParseCode4("rtyp")
	if got, ok := registry.Parameter(rtyp); !ok || got != "as" {
		t.Errorf("language rtyp parameter = %q, %v; want as", got, ok)
	}
}

func TestContextualCommandParameters(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	kocl, _ := ParseCode4("kocl")
	for codeText, want := range map[string]string{
		"corecrel": "new",
		"corecnte": "each",
	} {
		event, err := ParseEventCode(codeText)
		if err != nil {
			t.Fatal(err)
		}
		command, ok := registry.Command(event)
		if !ok {
			t.Fatalf("command %q missing", codeText)
		}
		if got := command.Parameters[kocl].Name; got != want {
			t.Errorf("%s kocl = %q, want %q", command.Name, got, want)
		}
	}
	if got, ok := registry.Parameter(kocl); ok {
		t.Errorf("ambiguous kocl received context-free name %q", got)
	}
}

func TestGroupedEnumerationsRemainDistinct(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for codeText, want := range map[string]string{
		"afdrasup": "application support",
		"afdmasup": "application support folder",
	} {
		code, err := ParseEventCode(codeText)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := registry.Constant(code); !ok || got != want {
			t.Errorf("Constant(%q) = %q, %v; want %q", codeText, got, ok, want)
		}
	}
}

func TestSDEFEnumerationsTermsAndDirectParameters(t *testing.T) {
	const source = `<dictionary><suite name="Events" code="evnt">
	  <command name="handler event" code="evnthndl"><direct-parameter type="list"/>
	    <parameter name="quietly" code="quie" type="boolean"/></command>
	  <command name="ordinary event" code="evntordn"/>
	  <enumeration name="mode" code="mode"><enumerator name="fast mode" code="fast"/></enumeration>
	  <class name="document" code="docu"><property name="title" code="ptit"/></class>
	  <type name="custom type" code="cust"/>
	</suite></dictionary>`
	dictionary, err := ParseSDEF("Events", strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	handlerCode, _ := ParseEventCode("evnthndl")
	ordinaryCode, _ := ParseEventCode("evntordn")
	if !dictionary.Commands[handlerCode].HasDirectParameter {
		t.Fatal("direct parameter was not recorded")
	}
	if dictionary.Commands[ordinaryCode].HasDirectParameter {
		t.Fatal("ordinary command gained a direct parameter")
	}
	for codeText, want := range map[string]string{
		"fast": "fast mode",
		"docu": "document",
		"ptit": "title",
		"cust": "custom type",
	} {
		code, _ := ParseCode4(codeText)
		got := dictionary.Terms[code]
		if enum := dictionary.Enums[code]; enum != "" {
			got = enum
		}
		if got != want {
			t.Errorf("%s = %q, want %q", codeText, got, want)
		}
	}
}

func TestMacRomanFourCharacterCode(t *testing.T) {
	code, err := ParseCode4("scrƒ")
	if err != nil {
		t.Fatal(err)
	}
	if code != (Code4{'s', 'c', 'r', 0xc4}) {
		t.Fatalf("code = %#v", code)
	}
}

func TestMacRomanFourCharacterCodeUsesCompleteRepertoire(t *testing.T) {
	code, err := ParseCode4("café")
	if err != nil {
		t.Fatal(err)
	}
	if code != (Code4{'c', 'a', 'f', 0x8e}) {
		t.Fatalf("code = %#v", code)
	}
}
