package decompile

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"applescript-tools/ast"
	"applescript-tools/internal/fas"
)

func TestLiteralPrimitiveContracts(t *testing.T) {
	cases := []struct {
		name  string
		value fas.Value
		check func(ast.Expr) bool
	}{
		{"nil", fas.NIL, func(value ast.Expr) bool { _, ok := value.(*ast.MissingLiteral); return ok }},
		{"true", fas.SpecialTrue, func(value ast.Expr) bool { got, ok := value.(*ast.BooleanLiteral); return ok && got.Value }},
		{"false", fas.SpecialFalse, func(value ast.Expr) bool { got, ok := value.(*ast.BooleanLiteral); return ok && !got.Value }},
		{"integer", fas.Integer(-2935), func(value ast.Expr) bool { got, ok := value.(*ast.NumberLiteral); return ok && got.Integer == -2935 }},
		{"zero-extended negative integer", fas.Integer(64104), func(value ast.Expr) bool {
			got, ok := value.(*ast.NumberLiteral)
			return ok && got.Integer == -1432
		}},
		{"float", fas.Float(3.25), func(value ast.Expr) bool {
			got, ok := value.(*ast.NumberLiteral)
			return ok && got.IsReal && got.Real == 3.25
		}},
		{"bytes", &fas.Bytes{Data: []byte("text")}, func(value ast.Expr) bool { got, ok := value.(*ast.StringLiteral); return ok && got.Value == "text" }},
		{"unicode", &fas.UnicodeText{Text: []byte{0, 'H', 0, 'i'}}, func(value ast.Expr) bool { got, ok := value.(*ast.StringLiteral); return ok && got.Value == "Hi" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := literal(test.value); !test.check(got) {
				t.Fatalf("literal(%T) = %#v", test.value, got)
			}
		})
	}
}

func TestPackedConstantContracts(t *testing.T) {
	current := constant(binary.BigEndian.Uint64([]byte("misccura")))
	if keyword, ok := current.(*ast.Keyword); !ok || keyword.Fallback != "current application" {
		t.Fatalf("current application = %#v", current)
	}
	packed := constant(binary.BigEndian.Uint64([]byte("EAlTcriT")))
	keyword, ok := packed.(*ast.Keyword)
	if !ok || string(keyword.Code) != "EAlTcriT" {
		t.Fatalf("packed constant = %#v", packed)
	}
	unknown := constant(binary.BigEndian.Uint64([]byte("test\x00\x00\x00\x01")))
	keyword = unknown.(*ast.Keyword)
	if len(keyword.Code) != 8 || keyword.Code[7] != 1 {
		t.Fatalf("unknown packed constant = %#v", keyword)
	}
}

func TestUnknownPackedConstantPreservesTypeAndMember(t *testing.T) {
	value := uint64(0)
	for _, b := range []byte("testABCD") {
		value = value<<8 | uint64(b)
	}
	keyword, ok := constant(value).(*ast.Keyword)
	if !ok || string(keyword.Code) != "testABCD" {
		t.Fatalf("packed constant = %#v", keyword)
	}
	formatted, err := ast.Format(&ast.Script{Handlers: []*ast.Handler{{
		Name: "probe", Body: []ast.Stmt{&ast.Return{Value: keyword, Explicit: true}},
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(formatted, "return «constant testABCD»") {
		t.Fatalf("packed constant source = %q", formatted)
	}
}

func TestListAndRecordLiteralContracts(t *testing.T) {
	list := &fas.Pair{Head: fas.Integer(9), Tail: &fas.Pair{Head: fas.Integer(1), Tail: &fas.Pair{Empty: true}}}
	gotList := literal(list).(*ast.List)
	if len(gotList.Elements) != 2 ||
		gotList.Elements[0].(*ast.NumberLiteral).Integer != 9 ||
		gotList.Elements[1].(*ast.NumberLiteral).Integer != 1 {
		t.Fatalf("list = %#v", gotList)
	}

	second := &fas.Binding{Key: &fas.Bytes{Data: []byte("b")}, Value: fas.Integer(7), Next: fas.NIL}
	first := &fas.Binding{Key: &fas.Bytes{Data: []byte("a")}, Value: fas.Integer(9), Next: second}
	gotRecord := literal(first).(*ast.Record)
	if len(gotRecord.Fields) != 2 ||
		gotRecord.Fields[0].Label.(*ast.StringLiteral).Value != "a" ||
		gotRecord.Fields[1].Value.(*ast.NumberLiteral).Integer != 7 {
		t.Fatalf("record = %#v", gotRecord)
	}
}

func TestRawDataAndLongDateContracts(t *testing.T) {
	raw := literal(&fas.RawData{Data: []byte("scptFasd")}).(*ast.RawDataLiteral)
	if string(raw.Type[:]) != "scpt" || string(raw.Data) != "Fasd" {
		t.Fatalf("raw data = %#v", raw)
	}
	unknown := literal(&fas.RawData{Data: []byte{0, 1, 2, 3, 'x'}}).(*ast.RawDataLiteral)
	if string(unknown.Type[:]) != "****" || len(unknown.Data) != 5 {
		t.Fatalf("unknown raw data = %#v", unknown)
	}

	epoch := make([]byte, 8)
	date := literal(&fas.Descriptor{Type: [4]byte{'l', 'd', 't', ' '}, Content: epoch})
	if got, ok := date.(*ast.DateLiteral); !ok || got.Value != "1/1/1904" {
		t.Fatalf("descriptor date = %#v", date)
	}
	seconds := uint64((time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Sub(time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC))) / time.Second)
	binary.BigEndian.PutUint64(epoch, seconds)
	date = literal(&fas.RawData{Data: append([]byte("ldt "), epoch...)})
	if got, ok := date.(*ast.DateLiteral); !ok || got.Value != "1/1/2025" {
		t.Fatalf("raw date = %#v", date)
	}
}

func TestAliasNameContracts(t *testing.T) {
	descriptor := make([]byte, 80)
	descriptor[7] = 2
	descriptor[50] = byte(len("System Settings.app"))
	copy(descriptor[51:], "System Settings.app")
	if got, ok := aliasName(descriptor); !ok || got != "System Preferences" {
		t.Fatalf("alias name = %q", got)
	}
	if got, ok := aliasName([]byte("Disk:Folder:Extensionless")); ok || got != "" {
		t.Fatalf("malformed alias = %q, %v", got, ok)
	}
}

func TestOnlyValidatedAliasDescriptorsBecomeApplications(t *testing.T) {
	content := make([]byte, 80)
	content[7] = 2
	content[50] = byte(len("Finder.app"))
	copy(content[51:], "Finder.app")
	application, ok := literal(&fas.Descriptor{Type: [4]byte{'a', 'l', 'i', 's'}, Content: content}).(*ast.Application)
	if !ok || application.Name != "Finder" {
		t.Fatalf("validated alias = %#v", application)
	}
	raw, ok := literal(&fas.Descriptor{Type: [4]byte{'b', 'o', 'o', 'k'}, Content: content}).(*ast.RawDataLiteral)
	if !ok || string(raw.Type[:]) != "book" {
		t.Fatalf("non-alias descriptor = %#v", raw)
	}
	content[7] = 1
	if _, ok := literal(&fas.Descriptor{Type: [4]byte{'a', 'l', 'i', 's'}, Content: content}).(*ast.Application); ok {
		t.Fatal("malformed alias descriptor became an application target")
	}
}

func TestIdentifierRecoveryContracts(t *testing.T) {
	if got := identifierValue(&fas.Bytes{Data: []byte("process_name")}, "fallback"); got != "process_name" {
		t.Fatalf("valid identifier = %q", got)
	}
	if got := identifierValue(&fas.Bytes{Data: []byte("not valid")}, "arg_0"); got != "arg_0" {
		t.Fatalf("fallback identifier = %q", got)
	}
	if validIdentifier("9invalid") || !validIdentifier("_valid9") {
		t.Fatal("identifier validity contract failed")
	}
	path := &fas.Object{Value: fas.Constant(0x46545063)}
	if got := identifierValue(path, "arg_0"); got != "path" {
		t.Fatalf("runtime-object identifier = %q, want path", got)
	}
	for code, want := range map[string]string{
		"pnam": "name",
		"vers": "version",
	} {
		value := uint64(0)
		for _, b := range []byte(code) {
			value = value<<8 | uint64(b)
		}
		if got := identifierValue(&fas.Object{Value: fas.Constant(value)}, "fallback"); got != want {
			t.Fatalf("runtime-object identifier %q = %q, want %q", code, got, want)
		}
	}
}

func TestAppleScriptSpecialConstants(t *testing.T) {
	for code, want := range map[string]string{
		"FTPc": "path",
		"lnfd": "linefeed",
		"prun": "running",
		"rslt": "result",
		"spac": "space",
		"strq": "quoted form",
	} {
		value := uint64(0)
		for _, b := range []byte(code) {
			value = value<<8 | uint64(b)
		}
		keyword, ok := constant(value).(*ast.Keyword)
		if !ok || keyword.Fallback != want {
			t.Errorf("%s = %#v, want fallback %q", code, keyword, want)
		}
	}
}

func TestUnicodeWrapperPreservesDamagedASCIIPayload(t *testing.T) {
	value := &fas.Vector{
		HasType: true,
		Type:    177,
		Children: []fas.Value{
			fas.NIL,
			&fas.Bytes{Data: []byte("/dev/disk")},
		},
	}
	got, ok := literal(value).(*ast.StringLiteral)
	if !ok || got.Value != "/dev/disk" {
		t.Fatalf("literal = %#v, want printable ASCII string", got)
	}

	utf16 := &fas.Vector{
		HasType: true,
		Type:    177,
		Children: []fas.Value{
			fas.NIL,
			&fas.Bytes{Data: []byte{0, 'o', 0, 'k'}},
		},
	}
	got, ok = literal(utf16).(*ast.StringLiteral)
	if !ok || got.Value != "ok" {
		t.Fatalf("literal = %#v, want UTF-16BE string", got)
	}
}
