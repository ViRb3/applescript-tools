package model

import (
	"testing"

	"applescript-tools/internal/fas"
)

func TestFunctionVectorNormalization(t *testing.T) {
	name := &fas.Bytes{Data: []byte("handler")}
	arguments := &fas.Bytes{Data: []byte("argument")}
	variables := &fas.Vector{HasType: true, Children: []fas.Value{fas.NIL, &fas.Bytes{Data: []byte("argument")}, &fas.Bytes{Data: []byte("local")}}}
	literals := &fas.Vector{HasType: true, Children: []fas.Value{fas.NIL, fas.Integer(1), fas.Integer(2)}}
	code := &fas.Bytes{Data: []byte{0x6b, 0x0f}}
	raw := &fas.Vector{Children: []fas.Value{
		fas.NIL, name, fas.NIL, arguments, fas.NIL, variables, literals, code,
	}}
	function, ok := FunctionFromVector(7, raw)
	if !ok {
		t.Fatal("valid function vector rejected")
	}
	if function.Offset != 7 || function.Name != name || function.Arguments != arguments ||
		len(function.Variables) != 2 || len(function.Literals) != 2 ||
		len(function.Code) != 2 {
		t.Fatalf("function = %#v", function)
	}
	code.Data[0] = 0
	if function.Code[0] != 0x6b {
		t.Fatal("function bytecode aliases serialized storage")
	}
}

func TestFunctionLiteralIndexGuard(t *testing.T) {
	function := &Function{Literals: []fas.Value{fas.Integer(1), fas.Integer(2)}}
	if value, ok := function.Literal(1); !ok || value != fas.Integer(2) {
		t.Fatalf("valid literal = %#v, %v", value, ok)
	}
	for _, index := range []int{-1, 2} {
		if value, ok := function.Literal(index); ok || value != nil {
			t.Errorf("literal %d = %#v, %v", index, value, ok)
		}
	}
}

func TestNormalizeRejectsInvalidShapes(t *testing.T) {
	if _, err := Normalize(&fas.Document{Root: fas.Integer(1)}); err == nil {
		t.Fatal("scalar root accepted")
	}
	if _, err := Normalize(&fas.Document{Root: &fas.Vector{}}); err == nil {
		t.Fatal("empty root accepted")
	}
	if _, err := Normalize(&fas.Document{Root: &fas.Vector{Children: []fas.Value{fas.Integer(1)}}}); err == nil {
		t.Fatal("scalar script table accepted")
	}
}
