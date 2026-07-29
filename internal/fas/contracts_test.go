package fas

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

func contractParser(data []byte) *parser {
	limits := DefaultLimits()
	p := &parser{r: bytes.NewReader(data), opts: Options{Limits: limits}, refs: make([]refEntry, 32)}
	for index := range p.refs {
		p.refs[index] = refEntry{state: refReserved, value: NIL}
	}
	return p
}

func TestReferenceZeroAndReservedSlots(t *testing.T) {
	p := contractParser(nil)
	if value, ok := p.lookup(0); !ok || value != NIL {
		t.Fatalf("reference zero = %#v, %v", value, ok)
	}
	if value, ok := p.lookup(1); ok || value != NIL {
		t.Fatalf("reserved reference = %#v, %v", value, ok)
	}
}

func TestPrimitiveObjectContracts(t *testing.T) {
	t.Run("symbol", func(t *testing.T) {
		p := contractParser(nil)
		if value, err := p.loadBody(-1, 1, 0); err != nil || value != NIL {
			t.Fatalf("zero symbol = %#v, %v", value, err)
		}
		p = contractParser([]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88})
		value, err := p.loadBody(-1, 1, 8)
		if err != nil || value.(*Symbol).Number != 0x1122334455667788 {
			t.Fatalf("symbol = %#v, %v", value, err)
		}
	})

	t.Run("immediate-size-errors", func(t *testing.T) {
		p := contractParser(nil)
		if _, err := p.loadBody(-1, 7, 3); err == nil || !strings.Contains(err.Error(), "must be 4") {
			t.Fatalf("long integer error = %v", err)
		}
		if _, err := p.loadBody(-1, 8, 4); err == nil || !strings.Contains(err.Error(), "must be 8") {
			t.Fatalf("float error = %v", err)
		}
	})

	t.Run("long-and-float", func(t *testing.T) {
		data := make([]byte, 12)
		negative := int32(-2)
		binary.BigEndian.PutUint32(data, uint32(negative))
		binary.BigEndian.PutUint64(data[4:], math.Float64bits(3.25))
		p := contractParser(data)
		integer, err := p.loadBody(-1, 7, 4)
		if err != nil || integer != Integer(-2) {
			t.Fatalf("integer = %#v, %v", integer, err)
		}
		real, err := p.loadBody(-1, 8, 8)
		if err != nil || real != Float(3.25) {
			t.Fatalf("float = %#v, %v", real, err)
		}
	})

	t.Run("unicode-text-and-style", func(t *testing.T) {
		text, style := []byte{0, 'H', 0, 'i'}, []byte{1, 2}
		var data bytes.Buffer
		_ = binary.Write(&data, binary.BigEndian, uint16(len(text)))
		data.Write(text)
		_ = binary.Write(&data, binary.BigEndian, uint16(len(style)))
		data.Write(style)
		value, err := contractParser(data.Bytes()).loadBody(-1, 12, uint16(data.Len()))
		if err != nil {
			t.Fatal(err)
		}
		unicode := value.(*Object).Value.(*UnicodeText)
		if !bytes.Equal(unicode.Text, text) || !bytes.Equal(unicode.Style, style) {
			t.Fatalf("unicode = %#v", unicode)
		}
	})
}

func TestCodeIdentifierContracts(t *testing.T) {
	for _, test := range []struct {
		name string
		tag  byte
		size uint16
		data []byte
		want uint64
	}{
		{"eight-byte", 11, 8, []byte("ABCDEFGH"), 0x4142434445464748},
		{"four-byte", 10, 4, []byte("ABCD"), 0x41424344},
		{"alternate-four-byte", 47, 4, []byte("WXYZ"), 0x5758595a},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := contractParser(append([]byte{test.tag}, test.data...)).loadBody(-1, 10, test.size)
			if err != nil {
				t.Fatal(err)
			}
			if got := uint64(value.(*Object).Value.(Constant)); got != test.want {
				t.Fatalf("constant = %#x, want %#x", got, test.want)
			}
		})
	}
	for _, test := range []struct {
		tag  byte
		size uint16
	}{
		{11, 4}, {10, 8}, {46, 8},
	} {
		if _, err := contractParser([]byte{test.tag}).loadBody(-1, 10, test.size); err == nil {
			t.Errorf("tag %d size %d accepted", test.tag, test.size)
		}
	}
	if _, err := contractParser([]byte{0xff}).loadBody(-1, 10, 4); err == nil ||
		!strings.Contains(err.Error(), "unknown code identifier tag") {
		t.Fatalf("unknown tag error = %v", err)
	}
}

func TestDataBlockContracts(t *testing.T) {
	t.Run("typed-short-and-long", func(t *testing.T) {
		short := contractParser(append([]byte{13}, []byte("scptDATA")...))
		value, err := short.loadBody(9, 15, 8)
		if err != nil || !bytes.Equal(value.(*RawData).Data, []byte("scptDATA")) {
			t.Fatalf("short data = %#v, %v", value, err)
		}
		if registered, ok := short.lookup(9); !ok || registered != value {
			t.Fatal("short data was not registered by identity")
		}

		var data bytes.Buffer
		data.WriteByte(13)
		_ = binary.Write(&data, binary.BigEndian, uint32(8))
		data.WriteString("scptDATA")
		value, err = contractParser(data.Bytes()).loadBody(-1, 18, 0)
		if err != nil || !bytes.Equal(value.(*RawData).Data, []byte("scptDATA")) {
			t.Fatalf("long data = %#v, %v", value, err)
		}
	})

	t.Run("untyped-short-and-long", func(t *testing.T) {
		value, err := contractParser([]byte("hello")).loadBody(-1, 17, 5)
		if err != nil || string(value.(*Bytes).Data) != "hello" {
			t.Fatalf("short bytes = %#v, %v", value, err)
		}
		var data bytes.Buffer
		_ = binary.Write(&data, binary.BigEndian, uint32(5))
		data.WriteString("world")
		value, err = contractParser(data.Bytes()).loadBody(-1, 19, 0)
		if err != nil || string(value.(*Bytes).Data) != "world" {
			t.Fatalf("long bytes = %#v, %v", value, err)
		}
	})

	t.Run("descriptor", func(t *testing.T) {
		content := []byte("payload")
		data := append([]byte{8}, make([]byte, 82)...)
		data = append(data, make([]byte, 8)...)
		data = append(data, []byte("alis")...)
		data = append(data, content...)
		p := contractParser(data)
		value, err := p.loadBody(3, 15, uint16(94+len(content)))
		if err != nil {
			t.Fatal(err)
		}
		descriptor := value.(*Descriptor)
		if string(descriptor.Type[:]) != "alis" || !bytes.Equal(descriptor.Content, content) {
			t.Fatalf("descriptor = %#v", descriptor)
		}
		if registered, ok := p.lookup(3); !ok || registered != value {
			t.Fatal("descriptor was not registered by identity")
		}
	})
}

func TestUserIdentifierContracts(t *testing.T) {
	encode := func(key, value string) []byte {
		var data bytes.Buffer
		data.WriteByte(48)
		_ = binary.Write(&data, binary.BigEndian, uint16(len(key)))
		data.WriteString(key)
		_ = binary.Write(&data, binary.BigEndian, uint16(len(value)))
		data.WriteString(value)
		return data.Bytes()
	}
	for _, test := range []struct {
		key, value, want string
	}{
		{"sourceName", "compiledName", "compiledName"},
		{"sourceName", "", "sourceName"},
	} {
		value, err := contractParser(encode(test.key, test.value)).loadBody(-1, 11, 0)
		if err != nil || string(value.(*Bytes).Data) != test.want {
			t.Fatalf("identifier = %#v, %v; want %q", value, err, test.want)
		}
	}
	if _, err := contractParser([]byte{0}).loadBody(-1, 11, 0); err == nil {
		t.Fatal("invalid marker accepted")
	}
	oversized := append([]byte{48, 1, 0}, bytes.Repeat([]byte{'a'}, 256)...)
	oversized = append(oversized, 0, 0)
	if _, err := contractParser(oversized).loadBody(-1, 11, 0); err == nil {
		t.Fatal("oversized identifier accepted")
	}
}

func TestCompositeObjectContracts(t *testing.T) {
	t.Run("value-block-and-pointer-order", func(t *testing.T) {
		p := contractParser([]byte{15})
		value, err := p.loadBody(5, 4, 0)
		if err != nil || value != Actor2 {
			t.Fatalf("actor = %#v, %v", value, err)
		}
		if registered, ok := p.lookup(5); !ok || registered != Actor2 {
			t.Fatal("actor not registered")
		}

		p = contractParser([]byte{4, 0, 2, 0, 3})
		_ = p.register(2, Integer(10))
		_ = p.register(3, Integer(20))
		value, err = p.loadBody(6, 4, 2)
		if err != nil {
			t.Fatal(err)
		}
		vector := value.(*Vector)
		if vector.Type != 4 || len(vector.Children) != 3 ||
			vector.Children[1] != Integer(10) || vector.Children[2] != Integer(20) {
			t.Fatalf("value block = %#v", vector)
		}

		p = contractParser([]byte{0, 4, 0, 5})
		_ = p.register(4, Integer(1))
		_ = p.register(5, Integer(2))
		value, err = p.loadBody(8, 16, 2)
		if err != nil {
			t.Fatal(err)
		}
		children := value.(*Vector).Children
		if len(children) != 2 || children[0] != Integer(1) || children[1] != Integer(2) {
			t.Fatalf("pointer block = %#v", children)
		}
	})

	t.Run("statement", func(t *testing.T) {
		data := []byte{9, 0, 10, 0, 20, 0, 30, 0, 4, 0, 5}
		p := contractParser(data)
		_ = p.register(4, Integer(1))
		_ = p.register(5, Integer(2))
		value, err := p.loadBody(7, 13, 2)
		if err != nil {
			t.Fatal(err)
		}
		statement := value.(*Statement)
		if statement.Tag != 9 || statement.TypeInfo != 10 || statement.Start != 20 || statement.End != 30 ||
			len(statement.Children) != 5 || statement.Children[3] != Integer(1) || statement.Children[4] != Integer(2) {
			t.Fatalf("statement = %#v", statement)
		}
	})

	t.Run("empty-and-linked-list", func(t *testing.T) {
		value, err := contractParser(nil).loadBody(-1, 2, 0)
		if err != nil || !value.(*Pair).Empty {
			t.Fatalf("empty list = %#v, %v", value, err)
		}
		p := contractParser([]byte{0, 1, 0, 2})
		_ = p.register(1, Integer(1))
		tail := &Pair{Head: Integer(2), Tail: &Pair{Empty: true}}
		_ = p.register(2, tail)
		value, err = p.loadBody(-1, 2, 2)
		if err != nil {
			t.Fatal(err)
		}
		head := value.(*Pair)
		if head.Head != Integer(1) || head.Tail != tail {
			t.Fatalf("linked list = %#v", head)
		}
	})

	t.Run("record-shapes", func(t *testing.T) {
		value, err := contractParser(nil).loadBody(-1, 6, 1)
		if err != nil || !value.(*Binding).Empty {
			t.Fatalf("empty record = %#v, %v", value, err)
		}
		if _, err := contractParser(nil).loadBody(-1, 6, 2); err == nil {
			t.Fatal("unknown record shape accepted")
		}
		p := contractParser([]byte{0, 1, 0, 2, 0, 3})
		_ = p.register(1, &Bytes{Data: []byte("alpha")})
		_ = p.register(2, Integer(9))
		_ = p.register(3, NIL)
		value, err = p.loadBody(12, 6, 3)
		if err != nil {
			t.Fatal(err)
		}
		head := value.(*Binding)
		if string(head.Key.(*Bytes).Data) != "alpha" || head.Value != Integer(9) || head.Next != NIL {
			t.Fatalf("record = %#v", head)
		}
		if registered, ok := p.lookup(12); !ok || registered != head {
			t.Fatal("record head identity was not registered")
		}
	})
}

func TestParserResourceLimits(t *testing.T) {
	preamble := append([]byte("FasdUAS 1.10"), []byte("1.10")...)
	data := append(preamble, 3, 0, 0, 0, 1)
	if _, err := Parse(bytes.NewReader(data), Options{Limits: Limits{MaxInputBytes: 4}}); err == nil {
		t.Fatal("input limit was not enforced")
	}
	if _, err := contractParser(nil).loadBody(-1, 255, 0); err == nil ||
		!strings.Contains(err.Error(), "unknown FAS object type 255") {
		t.Fatalf("unknown type error = %v", err)
	}
	if _, err := contractParser([]byte{0xfe}).loadBody(-1, 15, 0); err == nil ||
		!strings.Contains(err.Error(), "unknown runtime value type 254") {
		t.Fatalf("unknown runtime type error = %v", err)
	}
}
