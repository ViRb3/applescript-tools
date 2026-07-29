package bytecode

import "testing"

func TestOpcodeTableAndOperands(t *testing.T) {
	names := Names()
	if len(names) != 256 || names[12] != "MessageSend" || names[89] != "Jump" || names[110] != "BeginTimeout" {
		t.Fatalf("unexpected opcode table")
	}
	code := []byte{0xe3, 97, 0xff, 0xfe, 89, 0, 4, 98, 0, 1, 0, 2}
	fn, err := Decode(2, code, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := fn.Instructions[0].Operands[0].Value; got != 3 {
		t.Fatalf("compact literal = %d", got)
	}
	if got := fn.Instructions[1].Operands[0].Value; got != -2 {
		t.Fatalf("extended literal = %d", got)
	}
	if got := fn.Instructions[2].Operands[0].Value; got != 9 {
		t.Fatalf("jump target = %d", got)
	}
	if len(fn.Instructions[3].Operands) != 2 {
		t.Fatalf("parent variable operands = %#v", fn.Instructions[3].Operands)
	}
}

func TestTruncatedOperand(t *testing.T) {
	fn, err := Decode(2, []byte{97, 0}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fn.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v", fn.Diagnostics)
	}
	if _, err := Decode(2, []byte{97, 0}, true); err == nil {
		t.Fatal("strict decode accepted truncated operand")
	}
}

func TestOpcodeNumericContracts(t *testing.T) {
	expected := map[byte]string{
		0: "Equal", 12: "MessageSend", 15: "Return", 18: "Tell",
		21: "Error", 29: "TestIf", 30: "Add", 31: "Subtract",
		32: "Multiply", 33: "Divide", 34: "Quotient", 35: "Remainder",
		36: "Power", 37: "Concatenate", 38: "Coerce", 39: "Negate",
		40: "GetData", 71: "CopyData", 89: "Jump",
		97: "PushLiteralExtended", 110: "BeginTimeout",
	}
	names := Names()
	for opcode, want := range expected {
		if got := names[opcode]; got != want {
			t.Errorf("opcode %#02x = %q, want %q", opcode, got, want)
		}
	}
	for suboperation, opcode := range map[byte]byte{
		21: 44, 22: 45, 24: 47, 25: 48, 27: 50, 30: 53, 31: 54, 32: 55,
	} {
		if names[opcode] != "MakeObjectAlias" || opcode-23 != suboperation {
			t.Errorf("specifier suboperation %d has opcode %d (%q)", suboperation, opcode, names[opcode])
		}
	}
}

func TestEveryOperandFamily(t *testing.T) {
	code := []byte{
		12, 0, 1, // message literal
		43, 0, 2, // positional message literal
		75, 0, 3, // actor literal
		93, 0, 4, // extended variable
		27, 0, 5, // repeat collection variable
		98, 0, 2, 0, 6, // parent depth and variable
		88, 0, 7, 0, 8, // error binding literals
		29, 0, 3, // branch target
		18, 0xff, 0xfe, // signed tell word
		23, 0, 2, // link-repeat relative to operand start
		76, 0, 9, // procedure word
		0xaf, // compact variable
		0xbf, // compact pop variable
		0xcf, // compact global
		0xdf, // compact pop global
		0xff, // compact literal
	}
	decoded, err := Decode(4, code, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Instructions) != 16 {
		t.Fatalf("instruction count = %d", len(decoded.Instructions))
	}
	checks := []struct {
		instruction int
		kinds       []OperandKind
		values      []int
	}{
		{0, []OperandKind{OperandLiteralIndex}, []int{1}},
		{3, []OperandKind{OperandVariableIndex}, []int{4}},
		{5, []OperandKind{OperandParentDepth, OperandVariableIndex}, []int{2, 6}},
		{6, []OperandKind{OperandLiteralIndex, OperandLiteralIndex}, []int{7, 8}},
		{7, []OperandKind{OperandBranchTarget}, []int{29}},
		{8, []OperandKind{OperandSignedWord}, []int{-2}},
		{11, []OperandKind{OperandVariableIndex}, []int{15}},
		{13, []OperandKind{OperandLiteralIndex}, []int{15}},
		{15, []OperandKind{OperandLiteralIndex}, []int{15}},
	}
	for _, check := range checks {
		got := decoded.Instructions[check.instruction].Operands
		if len(got) != len(check.kinds) {
			t.Fatalf("instruction %d operands = %#v", check.instruction, got)
		}
		for index := range got {
			if got[index].Kind != check.kinds[index] || got[index].Value != check.values[index] {
				t.Errorf("instruction %d operand %d = %#v, want %s=%d",
					check.instruction, index, got[index], check.kinds[index], check.values[index])
			}
		}
	}
}
