package fas

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegerHeaderIsSigned(t *testing.T) {
	p := &parser{r: bytes.NewReader([]byte{3, 0xff, 0xff, 0xff, 0xfe}), opts: Options{Limits: DefaultLimits()}}
	h, err := p.readHeader()
	if err != nil {
		t.Fatal(err)
	}
	if h.index != 3 || h.ref != -1 || h.inlined != 0xfffe {
		t.Fatalf("unexpected header: %#v", h)
	}
	v, err := p.loadBody(h.ref, h.index, h.inlined)
	if err != nil {
		t.Fatal(err)
	}
	if v != Integer(-2) {
		t.Fatalf("got %#v, want -2", v)
	}
}

func TestPreambleVersions(t *testing.T) {
	object := []byte{3, 0, 0, 0, 1}
	for _, prefix := range [][]byte{nil, []byte("#!/usr/bin/osascript\n")} {
		data := append(append(append(append([]byte{}, prefix...), []byte("FasdUAS ")...), []byte("1.10")...), []byte("1.10")...)
		data = append(data, object...)
		doc, err := Parse(bytes.NewReader(data), Options{})
		if err != nil {
			t.Fatal(err)
		}
		if doc.Version != [4]byte{'1', '.', '1', '0'} || doc.Root != Integer(1) {
			t.Fatalf("unexpected document: %#v", doc)
		}
	}
}

func TestPreambleRejectsUnsupportedVersions(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{"too-low", "FasdUAS 0.97", "version too low"},
		{"too-high", "FasdUAS 1.111.11", "version too high"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(test.data), Options{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v; want %q", err, test.want)
			}
		})
	}
}

func TestBundledScriptsParse(t *testing.T) {
	for _, name := range []string{"demo.scpt", "seccon.scpt"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "testdata", name)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			doc, err := Parse(f, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if doc.Root == nil {
				t.Fatal("nil root")
			}
			root, ok := doc.Root.(*Vector)
			if !ok {
				t.Fatalf("root type %T, want vector", doc.Root)
			}
			t.Logf("root tag=%d children=%d last=%T", root.Type, len(root.Children), root.Children[len(root.Children)-1])
		})
	}
}

func TestStrictReferenceMismatch(t *testing.T) {
	data := append([]byte("FasdUAS 1.10"), []byte("1.10")...)
	data = append(data, 3, 0, 1, 0, 1)
	_, err := Parse(bytes.NewReader(data), Options{Strict: true})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestPermissiveReferenceMismatchReusesHeader(t *testing.T) {
	p := &parser{
		r:     bytes.NewReader([]byte{3, 0xff, 0xe5, 0x00, 0x07}),
		opts:  Options{Limits: DefaultLimits()},
		refs:  make([]refEntry, 32),
		reuse: nil,
	}
	for index := range p.refs {
		p.refs[index] = refEntry{state: refReserved, value: NIL}
	}

	value, err := p.loadObject(0)
	if err != nil {
		t.Fatal(err)
	}
	if value != NIL || p.reuse == nil || len(p.diagnostics) != 1 ||
		!strings.Contains(p.diagnostics[0].Message, "expected 0, found -27") {
		t.Fatalf("mismatch recovery = value %#v, reuse %#v, diagnostics %#v", value, p.reuse, p.diagnostics)
	}

	value, err = p.loadObject(-27)
	if err != nil {
		t.Fatal(err)
	}
	if value != Integer(7) || p.reuse != nil {
		t.Fatalf("reused header = value %#v, reuse %#v", value, p.reuse)
	}
}

func TestPermissiveReferenceMismatchRecoveryIsNotLimitedToFive(t *testing.T) {
	p := &parser{
		r:    bytes.NewReader([]byte{3, 0, 47, 0, 7}),
		opts: Options{Limits: DefaultLimits()},
		refs: make([]refEntry, 48),
	}
	for index := range p.refs {
		p.refs[index] = refEntry{state: refReserved, value: NIL}
	}

	for expected := int16(32); expected < 47; expected++ {
		value, err := p.loadObject(expected)
		if err != nil {
			t.Fatalf("missing reference %d: %v", expected, err)
		}
		if value != NIL {
			t.Fatalf("missing reference %d = %#v, want nil", expected, value)
		}
	}
	value, err := p.loadObject(47)
	if err != nil {
		t.Fatal(err)
	}
	if value != Integer(7) || p.reuse != nil || len(p.diagnostics) != 15 {
		t.Fatalf("recovery = value %#v, reuse %#v, diagnostics %d", value, p.reuse, len(p.diagnostics))
	}
}

func TestPermissiveReferenceMismatchRecoveryRespectsReferenceLimit(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxReferences = 3
	p := &parser{
		r:     bytes.NewReader([]byte{3, 0, 47, 0, 7}),
		opts:  Options{Limits: limits},
		refs:  make([]refEntry, 1),
		reuse: nil,
	}

	for expected := int16(0); expected < 2; expected++ {
		if _, err := p.loadObject(expected); err != nil {
			t.Fatalf("mismatch %d: %v", expected, err)
		}
	}
	if _, err := p.loadObject(2); err == nil ||
		!strings.Contains(err.Error(), "expected 2, found 47") {
		t.Fatalf("third mismatch error = %v", err)
	}
}

func TestPrimitiveObjectReferenceIsReusable(t *testing.T) {
	p := &parser{
		r: bytes.NewReader([]byte{
			3, 0, 47, 0, 10, // integer 10 with reference ID 47
		}),
		opts: Options{Limits: DefaultLimits()},
		refs: make([]refEntry, 48),
	}
	for index := range p.refs {
		p.refs[index] = refEntry{state: refReserved, value: NIL}
	}

	value, err := p.loadObject(47)
	if err != nil {
		t.Fatal(err)
	}
	if value != Integer(10) {
		t.Fatalf("value = %#v, want 10", value)
	}
	if reused, ok := p.lookup(47); !ok || reused != value {
		t.Fatalf("reference 47 = %#v, %v; want %#v, true", reused, ok, value)
	}
}

func TestPrimitiveObjectReaders(t *testing.T) {
	p := &parser{r: bytes.NewReader(nil), opts: Options{Limits: DefaultLimits()}}
	if value, err := p.loadBody(-1, 3, 0xffff); err != nil || value != Integer(-1) {
		t.Fatalf("inline integer: %#v, %v", value, err)
	}
	if value, err := p.loadBody(-1, 9, 1); err != nil || value != Bool(true) {
		t.Fatalf("boolean: %#v, %v", value, err)
	}
	long := make([]byte, 4)
	negative := int32(-42)
	binary.BigEndian.PutUint32(long, uint32(negative))
	p.r = bytes.NewReader(long)
	if value, err := p.loadBody(-1, 7, 4); err != nil || value != Integer(-42) {
		t.Fatalf("long: %#v, %v", value, err)
	}
	real := make([]byte, 8)
	binary.BigEndian.PutUint64(real, math.Float64bits(3.25))
	p.r = bytes.NewReader(real)
	if value, err := p.loadBody(-1, 8, 8); err != nil || value != Float(3.25) {
		t.Fatalf("float: %#v, %v", value, err)
	}
}

func TestCodeIdentifierEventOrder(t *testing.T) {
	data := append([]byte{46}, []byte("aaaabbbbccccddddeeeeffff")...)
	p := &parser{r: bytes.NewReader(data), opts: Options{Limits: DefaultLimits()}}
	value, err := p.loadBody(-1, 10, 24)
	if err != nil {
		t.Fatal(err)
	}
	event := value.(*Object).Value.(*EventIdentifier)
	var got strings.Builder
	for _, field := range event.Fields {
		got.WriteString(string(field[:]))
	}
	if got.String() != "aaaabbbbccccddddffffeeee" {
		t.Fatalf("event order %q", got.String())
	}
}

func TestReferenceRegistrationGrowth(t *testing.T) {
	p := &parser{opts: Options{Limits: DefaultLimits()}, refs: make([]refEntry, 1)}
	value := Integer(42)
	if err := p.register(40, value); err != nil {
		t.Fatal(err)
	}
	if got, ok := p.lookup(40); !ok || got != value {
		t.Fatalf("lookup = %#v, %v", got, ok)
	}
	if _, ok := p.lookup(-1); ok {
		t.Fatal("negative reference resolved")
	}
}
