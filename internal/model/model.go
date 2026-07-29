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
	Actors    []*Actor
}

// Actor is a nested script-object table. Its layout mirrors the root script
// table: child 1 is the name table and children 2 onward are properties and
// typed function vectors.
type Actor struct {
	Raw        *fas.Vector
	Path       string
	RootOffset int
	Name       fas.Value
	Names      []fas.Value
	Entries    []fas.Value
	Functions  []*Function
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
	seen := make(map[fas.Value]bool)
	for offset := 2; offset < len(root.Children); offset++ {
		if raw, ok := root.Children[offset].(*fas.Vector); ok {
			if fn, ok := FunctionFromVector(offset, raw); ok {
				s.Functions = append(s.Functions, fn)
				continue
			}
		}
		var name fas.Value
		if index := offset - 2; index < len(s.RootNames) {
			name = s.RootNames[index]
		}
		discoverActors(root.Children[offset], fmt.Sprintf("root[%d]", offset), offset, name, seen, &s.Actors)
	}
	return s, nil
}

func discoverActors(value fas.Value, path string, rootOffset int, name fas.Value, seen map[fas.Value]bool, actors *[]*Actor) {
	if value == nil {
		return
	}
	switch value := value.(type) {
	case *fas.Object:
		if seen[value] {
			return
		}
		seen[value] = true
		discoverActors(value.Value, path+".object", rootOffset, name, seen, actors)
	case *fas.Vector:
		if seen[value] {
			return
		}
		seen[value] = true
		if actor, ok := actorFromVector(value, path, rootOffset, name); ok {
			*actors = append(*actors, actor)
			for offset := 2; offset < len(actor.Entries); offset++ {
				if isTypedFunctionVector(actor.Entries[offset]) {
					continue
				}
				var childName fas.Value
				if index := offset - 2; index < len(actor.Names) {
					childName = actor.Names[index]
				}
				discoverActors(actor.Entries[offset], fmt.Sprintf("%s[%d]", path, offset), rootOffset, childName, seen, actors)
			}
			return
		}
		for index, child := range value.Children {
			discoverActors(child, fmt.Sprintf("%s.vector[%d]", path, index), rootOffset, name, seen, actors)
		}
	case *fas.Pair:
		if seen[value] {
			return
		}
		seen[value] = true
		discoverActors(value.Head, path+".head", rootOffset, name, seen, actors)
		discoverActors(value.Tail, path+".tail", rootOffset, name, seen, actors)
	case *fas.Binding:
		if seen[value] {
			return
		}
		seen[value] = true
		discoverActors(value.Key, path+".key", rootOffset, name, seen, actors)
		discoverActors(value.Value, path+".value", rootOffset, name, seen, actors)
		discoverActors(value.Extra, path+".extra", rootOffset, name, seen, actors)
		discoverActors(value.Next, path+".next", rootOffset, name, seen, actors)
	case *fas.Statement:
		if seen[value] {
			return
		}
		seen[value] = true
		for index, child := range value.Children {
			discoverActors(child, fmt.Sprintf("%s.statement[%d]", path, index), rootOffset, name, seen, actors)
		}
	}
}

func actorFromVector(raw *fas.Vector, path string, rootOffset int, name fas.Value) (*Actor, bool) {
	if raw.HasType || len(raw.Children) < 3 {
		return nil, false
	}
	namesRaw, ok := raw.Children[1].(*fas.Vector)
	if !ok {
		return nil, false
	}
	names := vectorValues(namesRaw)
	if len(names) != len(raw.Children)-2 {
		return nil, false
	}
	actor := &Actor{
		Raw: raw, Path: path, RootOffset: rootOffset, Name: name,
		Names: names, Entries: raw.Children,
	}
	for offset := 2; offset < len(raw.Children); offset++ {
		rawFunction, ok := raw.Children[offset].(*fas.Vector)
		if !ok || !isTypedFunctionVector(rawFunction) {
			continue
		}
		function, _ := FunctionFromVector(offset, rawFunction)
		actor.Functions = append(actor.Functions, function)
	}
	if len(actor.Functions) == 0 {
		return nil, false
	}
	return actor, true
}

func isTypedFunctionVector(value fas.Value) bool {
	raw, ok := value.(*fas.Vector)
	if !ok || !raw.HasType || len(raw.Children) != 8 {
		return false
	}
	if raw.Type != 16 && raw.Type != 17 {
		return false
	}
	_, ok = raw.Children[7].(*fas.Bytes)
	return ok
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
