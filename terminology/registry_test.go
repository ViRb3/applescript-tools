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
	if got, ok := r.Term(errn); !ok || got != "number" {
		t.Fatalf("errn term = %q, %v", got, ok)
	}
}

func TestLanguageBuiltins(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		"rtyp": "as",
		"txdl": "text item delimiters",
		"ascr": "AppleScript",
		"fltp": "as",
		"kfil": "in",
		"ldt ": "date",
	} {
		code, err := ParseCode4(text)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := registry.Term(code); !ok || got != want {
			t.Errorf("Term(%q) = %q, %v; want %q", text, got, ok, want)
		}
	}
}

func TestLanguageSpecialConstantsOverrideApplicationTerms(t *testing.T) {
	registry, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for codeText, want := range map[string]string{
		"FTPc": "path",
		"lnfd": "linefeed",
		"rslt": "result",
		"spac": "space",
		"strq": "quoted form",
		"TEXT": "string",
		"ctxt": "text",
		"asup": "application support",
		"ID  ": "id",
		"tab ": "tab",
		"ret ": "return",
	} {
		code, _ := ParseCode4(codeText)
		if got, ok := registry.Term(code); !ok || got != want {
			t.Errorf("%s = %q, %v; want %q", codeText, got, ok, want)
		}
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
