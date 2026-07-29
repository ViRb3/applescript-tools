package model

import (
	"fmt"

	"applescript-tools/internal/fas"
)

type Script struct {
	Document  *fas.Document
	Top       *fas.Vector
	Root      *fas.Vector
	Entries   []fas.Value
	Functions []*Function
	RootNames []fas.Value
}

type Function struct {
	Offset    int
	Raw       *fas.Vector
	Name      fas.Value
	Arguments fas.Value
	Variables []fas.Value
	Literals  []fas.Value
	Code      []byte
}

func Normalize(doc *fas.Document) (*Script, error) {
	top, ok := doc.Root.(*fas.Vector)
	if !ok {
		return nil, fmt.Errorf("compiled document root is %T, want vector", doc.Root)
	}
	if len(top.Children) == 0 {
		return nil, fmt.Errorf("compiled document root is empty")
	}
	root, ok := top.Children[len(top.Children)-1].(*fas.Vector)
	if !ok {
		return nil, fmt.Errorf("compiled script table is %T, want vector", top.Children[len(top.Children)-1])
	}
	s := &Script{Document: doc, Top: top, Root: root, Entries: root.Children}
	if len(root.Children) > 1 {
		if names, ok := root.Children[1].(*fas.Vector); ok {
			s.RootNames = vectorValues(names)
		}
	}
	for offset := 2; offset < len(root.Children); offset++ {
		if raw, ok := root.Children[offset].(*fas.Vector); ok {
			if fn, ok := FunctionFromVector(offset, raw); ok {
				s.Functions = append(s.Functions, fn)
			}
		}
	}
	return s, nil
}

func FunctionFromVector(offset int, raw *fas.Vector) (*Function, bool) {
	if len(raw.Children) < 8 {
		return nil, false
	}
	codeValue, ok := raw.Children[7].(*fas.Bytes)
	if !ok {
		return nil, false
	}
	fn := &Function{Offset: offset, Raw: raw, Name: raw.Children[1], Arguments: raw.Children[3], Code: append([]byte(nil), codeValue.Data...)}
	if vars, ok := raw.Children[5].(*fas.Vector); ok {
		fn.Variables = vectorValues(vars)
	}
	if literals, ok := raw.Children[6].(*fas.Vector); ok {
		fn.Literals = vectorValues(literals)
	}
	return fn, true
}

func vectorValues(v *fas.Vector) []fas.Value {
	if !v.HasType {
		return v.Children
	}
	if len(v.Children) <= 1 {
		return nil
	}
	return v.Children[1:]
}

func (f *Function) Literal(index int) (fas.Value, bool) {
	if index < 0 || index >= len(f.Literals) {
		return nil, false
	}
	return f.Literals[index], true
}
