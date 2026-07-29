package fas

import "fmt"

// Value is a node in the serialized AppleScript runtime graph.
type Value interface {
	isValue()
}

type Nil struct{}

func (Nil) isValue() {}

var NIL Value = Nil{}

type Bool bool

func (Bool) isValue() {}

type Integer int64

func (Integer) isValue() {}

type Float float64

func (Float) isValue() {}

type Symbol struct{ Number uint64 }

func (*Symbol) isValue() {}

type Bytes struct{ Data []byte }

func (*Bytes) isValue() {}

type UnicodeText struct {
	Text  []byte
	Style []byte
}

func (*UnicodeText) isValue() {}

type Special uint64

func (Special) isValue() {}

const (
	SpecialNil   Special = 0
	SpecialFalse Special = 0x79
	SpecialTrue  Special = 0x7a
)

type Constant uint64

func (Constant) isValue() {}

type Object struct{ Value Value }

func (*Object) isValue() {}

type RawData struct{ Data []byte }

func (*RawData) isValue() {}

type Descriptor struct {
	Type    [4]byte
	Content []byte
}

func (*Descriptor) isValue() {}

type EventIdentifier struct {
	Fields [6][4]byte
}

func (*EventIdentifier) isValue() {}

type Pair struct {
	Empty bool
	Head  Value
	Tail  Value
}

func (*Pair) isValue() {}

type Binding struct {
	Empty bool
	Key   Value
	Value Value
	Extra Value
	Next  Value
}

func (*Binding) isValue() {}

type Vector struct {
	Type     byte
	HasType  bool
	Children []Value
}

func (*Vector) isValue() {}

type Statement struct {
	Tag      byte
	TypeInfo uint16
	Start    uint16
	End      uint16
	Children []Value
}

func (*Statement) isValue() {}

type SecondActor struct{}

func (SecondActor) isValue() {}

var Actor2 Value = SecondActor{}

func TypeName(v Value) string {
	switch v.(type) {
	case Nil:
		return "nil"
	case Bool:
		return "bool"
	case Integer:
		return "integer"
	case Float:
		return "float"
	case *Symbol:
		return "symbol"
	case *Bytes:
		return "bytes"
	case *UnicodeText:
		return "unicode-text"
	case Special:
		return "special"
	case Constant:
		return "constant"
	case *Object:
		return "object"
	case *RawData:
		return "raw-data"
	case *Descriptor:
		return "descriptor"
	case *EventIdentifier:
		return "event-identifier"
	case *Pair:
		return "pair"
	case *Binding:
		return "binding"
	case *Vector:
		return "vector"
	case *Statement:
		return "statement"
	case SecondActor:
		return "second-actor"
	default:
		return fmt.Sprintf("%T", v)
	}
}
