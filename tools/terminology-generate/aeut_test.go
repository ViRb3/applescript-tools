package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type aeutFixture struct {
	bytes.Buffer
}

func (f *aeutFixture) u16(value uint16) {
	_ = binary.Write(&f.Buffer, binary.LittleEndian, value)
}

func (f *aeutFixture) code(value string) {
	if len(value) != 4 {
		panic("AEUT fixture code must be four bytes")
	}
	_ = binary.Write(&f.Buffer, binary.LittleEndian, binary.BigEndian.Uint32([]byte(value)))
}

func (f *aeutFixture) pstring(value string) {
	f.WriteByte(byte(len(value)))
	f.WriteString(value)
}

func (f *aeutFixture) align() {
	if f.Len()&1 != 0 {
		f.WriteByte(0)
	}
}

func systemTerminologyFixture() []byte {
	var f aeutFixture
	f.Write([]byte{1, 0})
	f.u16(0)
	f.u16(0)
	f.u16(1)

	f.pstring("Test Suite")
	f.pstring("")
	f.align()
	f.code("TEST")
	f.u16(1)
	f.u16(1)

	f.u16(1)
	f.pstring("do thing")
	f.pstring("")
	f.align()
	f.code("TEST")
	f.code("doth")
	f.code("null")
	f.pstring("")
	f.align()
	f.u16(0)
	f.code("TEXT")
	f.pstring("")
	f.align()
	f.u16(0)
	f.u16(1)
	f.pstring("with value")
	f.align()
	f.code("valu")
	f.code("long")
	f.pstring("")
	f.align()
	f.u16(0)

	f.u16(1)
	f.pstring("document")
	f.align()
	f.code("docu")
	f.pstring("")
	f.align()
	f.u16(1)
	f.pstring("title")
	f.align()
	f.code("ptit")
	f.code("TEXT")
	f.pstring("")
	f.align()
	f.u16(0)
	f.u16(0)

	f.u16(0)
	f.u16(1)
	f.code("mode")
	f.u16(1)
	f.pstring("fast mode")
	f.align()
	f.code("fast")
	f.pstring("")
	f.align()
	return f.Bytes()
}

func TestParseSystemTerminology(t *testing.T) {
	definition, err := parseSystemTerminology(systemTerminologyFixture())
	if err != nil {
		t.Fatal(err)
	}
	if definition.Source != "OSAGetSysTerminology" || len(definition.Commands) != 1 {
		t.Fatalf("definition = %#v", definition)
	}
	command := definition.Commands[0]
	if command.Code != "TESTdoth" || command.Name != "do thing" || !command.HasDirectParameter {
		t.Fatalf("command = %#v", command)
	}
	if len(command.Parameters) != 1 || command.Parameters[0].Code != "valu" || command.Parameters[0].Name != "with value" {
		t.Fatalf("parameters = %#v", command.Parameters)
	}
	wantTerms := map[string]string{"docu": "document", "ptit": "title"}
	for _, term := range definition.Terms {
		delete(wantTerms, term.Code)
	}
	if len(wantTerms) != 0 {
		t.Fatalf("missing terms: %#v", wantTerms)
	}
	if len(definition.Enumerations) != 1 || definition.Enumerations[0].Group != "mode" || definition.Enumerations[0].Code != "fast" || definition.Enumerations[0].Name != "fast mode" {
		t.Fatalf("enumerations = %#v", definition.Enumerations)
	}
}

func TestParseSystemTerminologyRejectsDamage(t *testing.T) {
	fixture := systemTerminologyFixture()
	for name, data := range map[string][]byte{
		"truncated": fixture[:len(fixture)-1],
		"trailing":  append(append([]byte(nil), fixture...), 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSystemTerminology(data); err == nil {
				t.Fatal("damaged system terminology parsed successfully")
			}
		})
	}
}
