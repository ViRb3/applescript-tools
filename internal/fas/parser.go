package fas

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

type Limits struct {
	MaxInputBytes int64
	MaxObjects    int
	MaxReferences int
	MaxDepth      int
	MaxBlobBytes  int
}

func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes: 64 << 20,
		MaxObjects:    1 << 20,
		MaxReferences: 1 << 20,
		MaxDepth:      1024,
		MaxBlobBytes:  64 << 20,
	}
}

type Options struct {
	Strict bool
	Limits Limits
}

type Diagnostic struct {
	Offset  int64
	Message string
}

type Document struct {
	Version     [4]byte
	Root        Value
	Diagnostics []Diagnostic
}

type refState uint8

const (
	refUnseen refState = iota
	refReserved
	refResolved
)

type refEntry struct {
	state refState
	value Value
}

type header struct {
	index   byte
	ref     int16
	inlined uint16
	offset  int64
}

type parser struct {
	r           *bytes.Reader
	opts        Options
	refs        []refEntry
	objects     int
	depth       int
	reuse       *header
	refErrors   int
	diagnostics []Diagnostic
}

func Parse(r io.Reader, opts Options) (*Document, error) {
	defaults := DefaultLimits()
	if opts.Limits.MaxInputBytes <= 0 {
		opts.Limits.MaxInputBytes = defaults.MaxInputBytes
	}
	if opts.Limits.MaxObjects <= 0 {
		opts.Limits.MaxObjects = defaults.MaxObjects
	}
	if opts.Limits.MaxReferences <= 0 {
		opts.Limits.MaxReferences = defaults.MaxReferences
	}
	if opts.Limits.MaxDepth <= 0 {
		opts.Limits.MaxDepth = defaults.MaxDepth
	}
	if opts.Limits.MaxBlobBytes <= 0 {
		opts.Limits.MaxBlobBytes = defaults.MaxBlobBytes
	}
	limited := io.LimitReader(r, opts.Limits.MaxInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read FAS stream: %w", err)
	}
	if int64(len(data)) > opts.Limits.MaxInputBytes {
		return nil, fmt.Errorf("FAS stream exceeds %d-byte limit", opts.Limits.MaxInputBytes)
	}
	p := &parser{r: bytes.NewReader(data), opts: opts, refs: make([]refEntry, 32)}
	for i := range p.refs {
		p.refs[i] = refEntry{state: refReserved, value: NIL}
	}
	doc, err := p.parse()
	if doc != nil {
		doc.Diagnostics = append(doc.Diagnostics, p.diagnostics...)
	}
	return doc, err
}

func (p *parser) parse() (*Document, error) {
	if p.r.Len() >= 2 {
		pos, _ := p.r.Seek(0, io.SeekCurrent)
		var prefix [2]byte
		if _, err := io.ReadFull(p.r, prefix[:]); err != nil {
			return nil, err
		}
		if prefix == [2]byte{'#', '!'} {
			br := bufio.NewReader(p.r)
			for {
				b, err := br.ReadByte()
				if err != nil {
					if errors.Is(err, io.EOF) {
						return nil, errors.New("unterminated hashbang")
					}
					return nil, err
				}
				if b == '\n' {
					break
				}
			}
			remaining, _ := io.ReadAll(br)
			p.r = bytes.NewReader(remaining)
		} else {
			_, _ = p.r.Seek(pos, io.SeekStart)
		}
	}
	magic, err := p.readN(4)
	if err != nil {
		return nil, p.wrap("read Fasd magic", err)
	}
	if string(magic) != "Fasd" {
		return nil, fmt.Errorf("invalid FAS magic %q", magic)
	}
	uas, err := p.readN(4)
	if err != nil {
		return nil, p.wrap("read UAS magic", err)
	}
	if string(uas) != "UAS " {
		return nil, fmt.Errorf("invalid UAS magic %q", uas)
	}
	versionBytes, err := p.readN(4)
	if err != nil {
		return nil, p.wrap("read version", err)
	}
	if bytes.Compare(versionBytes, []byte("1.10")) >= 0 {
		versionBytes, err = p.readN(4)
		if err != nil {
			return nil, p.wrap("read extended version", err)
		}
	}
	if bytes.Compare(versionBytes, []byte("0.97")) <= 0 {
		return nil, fmt.Errorf("FAS version too low: %q", versionBytes)
	}
	if bytes.Compare(versionBytes, []byte("1.11")) >= 0 {
		return nil, fmt.Errorf("FAS version too high: %q", versionBytes)
	}
	var version [4]byte
	copy(version[:], versionBytes)
	root, err := p.loadObject(0)
	if err != nil {
		return &Document{Version: version}, err
	}
	return &Document{Version: version, Root: root}, nil
}

func (p *parser) offset() int64 {
	pos, _ := p.r.Seek(0, io.SeekCurrent)
	return pos
}

func (p *parser) wrap(action string, err error) error {
	return fmt.Errorf("%s at 0x%x: %w", action, p.offset(), err)
}

func (p *parser) readN(n int) ([]byte, error) {
	if n < 0 || n > p.opts.Limits.MaxBlobBytes {
		return nil, fmt.Errorf("invalid allocation size %d", n)
	}
	out := make([]byte, n)
	_, err := io.ReadFull(p.r, out)
	return out, err
}
func (p *parser) u8() (byte, error) {
	var x byte
	err := binary.Read(p.r, binary.BigEndian, &x)
	return x, err
}
func (p *parser) u16() (uint16, error) {
	var x uint16
	err := binary.Read(p.r, binary.BigEndian, &x)
	return x, err
}
func (p *parser) s16() (int16, error) {
	var x int16
	err := binary.Read(p.r, binary.BigEndian, &x)
	return x, err
}
func (p *parser) u32() (uint32, error) {
	var x uint32
	err := binary.Read(p.r, binary.BigEndian, &x)
	return x, err
}
func (p *parser) s32() (int32, error) {
	var x int32
	err := binary.Read(p.r, binary.BigEndian, &x)
	return x, err
}
func (p *parser) u64() (uint64, error) {
	var x uint64
	err := binary.Read(p.r, binary.BigEndian, &x)
	return x, err
}

func (p *parser) readHeader() (header, error) {
	off := p.offset()
	index, err := p.u8()
	if err != nil {
		return header{}, err
	}
	ref, err := p.s16()
	if err != nil {
		return header{}, err
	}
	size, err := p.u16()
	if err != nil {
		return header{}, err
	}
	return header{index: index, ref: ref, inlined: size, offset: off}, nil
}

func (p *parser) loadObject(expected int16) (Value, error) {
	if p.depth >= p.opts.Limits.MaxDepth {
		return nil, fmt.Errorf("object nesting exceeds %d", p.opts.Limits.MaxDepth)
	}
	p.depth++
	defer func() { p.depth-- }()
	var h header
	var err error
	if p.reuse != nil {
		h = *p.reuse
		p.reuse = nil
	} else {
		h, err = p.readHeader()
	}
	if err != nil {
		return nil, p.wrap("read object header", err)
	}
	if h.ref != expected {
		msg := fmt.Sprintf("RefID mismatch: expected %d, found %d", expected, h.ref)
		p.diagnostics = append(p.diagnostics, Diagnostic{Offset: h.offset, Message: msg})
		p.reuse = &h
		p.refErrors++
		if p.opts.Strict || p.refErrors >= 6 {
			return nil, errors.New(msg)
		}
		return NIL, nil
	}
	value, err := p.loadBody(h.ref, h.index, h.inlined)
	if err != nil {
		return nil, err
	}
	if err := p.register(h.ref, value); err != nil {
		return nil, err
	}
	return value, nil
}

func (p *parser) lookup(ref int16) (Value, bool) {
	if ref == 0 {
		return NIL, true
	}
	if ref < 0 || int(ref) >= len(p.refs) {
		return nil, false
	}
	e := p.refs[ref]
	return e.value, e.state == refResolved
}

func (p *parser) register(ref int16, value Value) error {
	if ref < 0 {
		return nil
	}
	if int(ref) >= p.opts.Limits.MaxReferences {
		return fmt.Errorf("reference %d exceeds limit", ref)
	}
	if int(ref) >= len(p.refs) {
		p.refs = append(p.refs, make([]refEntry, int(ref)-len(p.refs)+1)...)
	}
	p.refs[ref] = refEntry{state: refResolved, value: value}
	return nil
}

func (p *parser) find(ref int16) (Value, error) {
	if v, ok := p.lookup(ref); ok {
		return v, nil
	}
	return p.loadObject(ref)
}

func (p *parser) loadBody(ref int16, index byte, size uint16) (Value, error) {
	p.objects++
	if p.objects > p.opts.Limits.MaxObjects {
		return nil, fmt.Errorf("object count exceeds %d", p.opts.Limits.MaxObjects)
	}
	switch index {
	case 1:
		if size == 0 {
			return NIL, nil
		}
		n, err := p.u64()
		if err != nil {
			return nil, err
		}
		return &Symbol{Number: n}, nil
	case 2:
		return p.loadList(size)
	case 3:
		return Integer(int16(size)), nil
	case 4, 14:
		return p.loadValueBlock(ref, size)
	case 6:
		return p.loadRecord(ref, size)
	case 7:
		if size != 4 {
			return nil, errors.New("long integer size must be 4")
		}
		n, err := p.s32()
		return Integer(n), err
	case 8:
		if size != 8 {
			return nil, errors.New("float size must be 8")
		}
		n, err := p.u64()
		return Float(math.Float64frombits(n)), err
	case 9:
		return Bool(size != 0), nil
	case 10:
		return p.loadCodeIdentifier(size)
	case 11:
		return p.loadUserIdentifier()
	case 12:
		return p.loadUnicodeText()
	case 13:
		return p.loadStatement(ref, size)
	case 15:
		return p.loadDataBlock(ref, size)
	case 16:
		return p.loadPointerBlock(ref, size)
	case 17:
		b, err := p.readN(int(size))
		return &Bytes{Data: b}, err
	case 18:
		return p.loadLongDataBlock()
	case 19:
		n, err := p.u32()
		if err != nil {
			return nil, err
		}
		b, err := p.readN(int(n))
		return &Bytes{Data: b}, err
	default:
		return nil, fmt.Errorf("unknown FAS object type %d", index)
	}
}

func (p *parser) loadRefs(count int, offset int) ([]Value, error) {
	if count < 0 || count > p.opts.Limits.MaxReferences {
		return nil, fmt.Errorf("invalid reference vector length %d", count)
	}
	refs := make([]int16, count)
	for i := offset; i < count; i++ {
		r, err := p.s16()
		if err != nil {
			return nil, err
		}
		refs[i] = r
	}
	out := make([]Value, count)
	for i := 0; i < offset && i < count; i++ {
		out[i] = NIL
	}
	for i := offset; i < count; i++ {
		v, err := p.find(refs[i])
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (p *parser) loadPointerBlock(ref int16, size uint16) (Value, error) {
	children, err := p.loadRefs(int(size), 0)
	if err != nil {
		return nil, err
	}
	v := &Vector{Children: children}
	if err = p.register(ref, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (p *parser) loadValueBlock(ref int16, size uint16) (Value, error) {
	tag, err := p.u8()
	if err != nil {
		return nil, err
	}
	if size == 0 && tag == 15 {
		if err = p.register(ref, Actor2); err != nil {
			return nil, err
		}
		return Actor2, nil
	}
	children, err := p.loadRefs(int(size)+1, 1)
	if err != nil {
		return nil, err
	}
	v := &Vector{Type: tag, HasType: true, Children: children}
	if err = p.register(ref, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (p *parser) loadList(size uint16) (Value, error) {
	if size != 2 {
		return &Pair{Empty: true}, nil
	}
	head := &Pair{Head: NIL, Tail: &Pair{Empty: true}}
	cur := head
	for {
		a, err := p.s16()
		if err != nil {
			return nil, err
		}
		b, err := p.s16()
		if err != nil {
			return nil, err
		}
		cur.Head, err = p.find(a)
		if err != nil {
			return nil, err
		}
		if tail, ok := p.lookup(b); ok {
			cur.Tail = tail
			return head, nil
		}
		h, err := p.readHeader()
		if err != nil {
			return nil, err
		}
		if h.index != 2 {
			cur.Tail, err = p.loadBody(h.ref, h.index, h.inlined)
			if err != nil {
				return nil, err
			}
			if err = p.register(h.ref, cur.Tail); err != nil {
				return nil, err
			}
			return head, nil
		}
		if h.inlined != 2 {
			if h.inlined != 0 {
				return nil, errors.New("list tail size must be zero")
			}
			cur.Tail = &Pair{Empty: true}
			return head, nil
		}
		next := &Pair{Head: NIL, Tail: &Pair{Empty: true}}
		cur.Tail = next
		cur = next
		if err = p.register(h.ref, cur); err != nil {
			return nil, err
		}
	}
}

func (p *parser) loadRecord(ref int16, size uint16) (Value, error) {
	if size == 1 {
		return &Binding{Empty: true}, nil
	}
	if size != 3 {
		return nil, fmt.Errorf("unknown FAS record type %d", size)
	}
	head := &Binding{Key: NIL, Value: NIL, Extra: NIL, Next: NIL}
	cur := head
	if err := p.register(ref, head); err != nil {
		return nil, err
	}
	for size == 3 {
		a, e := p.s16()
		if e != nil {
			return nil, e
		}
		b, e := p.s16()
		if e != nil {
			return nil, e
		}
		c, e := p.s16()
		if e != nil {
			return nil, e
		}
		cur.Key, e = p.find(a)
		if e != nil {
			return nil, e
		}
		cur.Value, e = p.find(b)
		if e != nil {
			return nil, e
		}
		if v, ok := p.lookup(c); ok {
			cur.Next = v
			break
		}
		h, e := p.readHeader()
		if e != nil {
			return nil, e
		}
		if h.index != 6 {
			cur.Next, e = p.loadBody(h.ref, h.index, h.inlined)
			if e != nil {
				return nil, e
			}
			if e = p.register(h.ref, cur.Next); e != nil {
				return nil, e
			}
			break
		}
		size = h.inlined
		if size != 3 {
			break
		}
		next := &Binding{Key: NIL, Value: NIL, Extra: NIL, Next: NIL}
		cur.Next = next
		cur = next
	}
	return head, nil
}

func (p *parser) loadCodeIdentifier(size uint16) (Value, error) {
	tag, err := p.u8()
	if err != nil {
		return nil, err
	}
	switch tag {
	case 11:
		if size != 8 {
			return nil, fmt.Errorf("code identifier size %d, want 8", size)
		}
		n, e := p.u64()
		return &Object{Value: Constant(n)}, e
	case 10, 47:
		if size != 4 {
			return nil, fmt.Errorf("code identifier size %d, want 4", size)
		}
		n, e := p.u32()
		return &Object{Value: Constant(n)}, e
	case 46:
		if size != 24 {
			return nil, fmt.Errorf("event identifier size %d, want 24", size)
		}
		var words [6][4]byte
		for i := range words {
			b, e := p.readN(4)
			if e != nil {
				return nil, e
			}
			copy(words[i][:], b)
		}
		words[4], words[5] = words[5], words[4] // Intentional abcdfe order.
		return &Object{Value: &EventIdentifier{Fields: words}}, nil
	default:
		return nil, fmt.Errorf("unknown code identifier tag %d", tag)
	}
}

func (p *parser) loadUserIdentifier() (Value, error) {
	tag, err := p.u8()
	if err != nil {
		return nil, err
	}
	if tag != 48 {
		return nil, errors.New("invalid user identifier marker")
	}
	a, err := p.u16()
	if err != nil {
		return nil, err
	}
	if a >= 0x100 {
		return nil, errors.New("oversized user identifier key")
	}
	key, err := p.readN(int(a))
	if err != nil {
		return nil, err
	}
	b, err := p.u16()
	if err != nil {
		return nil, err
	}
	if b >= 0x100 {
		return nil, errors.New("oversized user identifier value")
	}
	value, err := p.readN(int(b))
	if err != nil {
		return nil, err
	}
	if b == 0 {
		value = key
	}
	return &Bytes{Data: value}, nil
}

func (p *parser) loadUnicodeText() (Value, error) {
	n, err := p.u16()
	if err != nil {
		return nil, err
	}
	text, err := p.readN(int(n))
	if err != nil {
		return nil, err
	}
	s, err := p.u16()
	if err != nil {
		return nil, err
	}
	style, err := p.readN(int(s))
	if err != nil {
		return nil, err
	}
	return &Object{Value: &UnicodeText{Text: text, Style: style}}, nil
}

func (p *parser) loadStatement(ref int16, size uint16) (Value, error) {
	tag, err := p.u8()
	if err != nil {
		return nil, err
	}
	ti, err := p.u16()
	if err != nil {
		return nil, err
	}
	start, err := p.u16()
	if err != nil {
		return nil, err
	}
	end, err := p.u16()
	if err != nil {
		return nil, err
	}
	children, err := p.loadRefs(int(size)+3, 3)
	if err != nil {
		return nil, err
	}
	v := &Statement{Tag: tag, TypeInfo: ti, Start: start, End: end, Children: children}
	if err = p.register(ref, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (p *parser) loadDataBlock(ref int16, size uint16) (Value, error) {
	tag, err := p.u8()
	if err != nil {
		return nil, err
	}
	var v Value
	if tag == 8 {
		if size < 94 {
			return nil, fmt.Errorf("descriptor size %d below header size", size)
		}
		if _, err = p.readN(82); err != nil {
			return nil, err
		}
		if _, err = p.u32(); err != nil {
			return nil, err
		}
		if _, err = p.readN(4); err != nil {
			return nil, err
		}
		t, err := p.readN(4)
		if err != nil {
			return nil, err
		}
		content, err := p.readN(int(size) - 94)
		if err != nil {
			return nil, err
		}
		d := &Descriptor{Content: content}
		copy(d.Type[:], t)
		v = d
	} else {
		b, err := p.readN(int(size))
		if err != nil {
			return nil, err
		}
		v, err = runtimeValue(tag, b)
		if err != nil {
			return nil, err
		}
	}
	if err = p.register(ref, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (p *parser) loadLongDataBlock() (Value, error) {
	tag, err := p.u8()
	if err != nil {
		return nil, err
	}
	n, err := p.u32()
	if err != nil {
		return nil, err
	}
	b, err := p.readN(int(n))
	if err != nil {
		return nil, err
	}
	return runtimeValue(tag, b)
}

func runtimeValue(tag byte, b []byte) (Value, error) {
	switch tag {
	case 2:
		var n uint64
		for _, x := range b {
			n = n<<8 | uint64(x)
		}
		return Special(n), nil
	case 6:
		var n uint64
		for _, x := range b {
			n = n<<8 | uint64(x)
		}
		return Integer(n), nil
	case 11:
		var n uint64
		for _, x := range b {
			n = n<<8 | uint64(x)
		}
		return Constant(n), nil
	case 13:
		return &RawData{Data: b}, nil
	case 0:
		return &Bytes{Data: b}, nil
	case 0xb1:
		return &UnicodeText{Text: b}, nil
	default:
		return nil, fmt.Errorf("unknown runtime value type %d", tag)
	}
}
