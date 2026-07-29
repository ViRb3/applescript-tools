package bytecode

import "fmt"

type Opcode byte

var opcodeNames = [256]string{}

func init() {
	names := []string{
		"Equal", "NotEqual", "GreaterThan", "GreaterThanOrEqual", "LessThan", "LessThanOrEqual",
		"StartsWith", "EndsWith", "Contains", "And", "Or", "Not", "MessageSend", "MakeList",
		"MakeRecord", "Return", "Continue", "ObjectAliasQuote", "Tell", "Consider", "ErrorHandler",
		"Error", "Exit", "LinkRepeat", "RepeatNTimes", "RepeatWhile", "RepeatUntil",
		"RepeatInCollection", "RepeatInRange", "TestIf", "Add", "Subtract", "Multiply", "Divide",
		"Quotient", "Remainder", "Power", "Concatenate", "Coerce", "Negate", "GetData", "PushMe",
		"PushIt", "PositionalMessageSend",
	}
	for i, name := range names {
		opcodeNames[i] = name
	}
	for i := 44; i <= 55; i++ {
		opcodeNames[i] = "MakeObjectAlias"
	}
	for i := 56; i <= 68; i++ {
		opcodeNames[i] = "MakeComp"
	}
	more := map[int]string{
		69: "GetData", 70: "SetData", 71: "CopyData", 72: "Undefined", 73: "Undefined",
		74: "PositionalContinue", 75: "DefineActor", 76: "DefineProcedure", 77: "DefineClosure",
		78: "DefineProperty", 79: "StoreResult", 80: "GetResult", 81: "Clone", 82: "Of",
		83: "EndDefineActor", 84: "EndOf", 85: "EndTell", 86: "EndConsider",
		87: "EndErrorHandler", 88: "HandleError", 89: "Jump", 90: "Pop", 91: "Dup",
		92: "GCSwap", 93: "PushVariableExtended", 94: "PopVariableExtended",
		95: "PushGlobalExtended", 96: "PopGlobalExtended", 97: "PushLiteralExtended",
		98: "PushParentVariable", 99: "PopParentVariable", 100: "PushNext", 101: "PushTrue",
		102: "PushFalse", 103: "PushEmpty", 104: "PushUndefined", 105: "PushMinus1",
		106: "Push0", 107: "Push1", 108: "Push2", 109: "Push3", 110: "BeginTimeout",
		111: "EndTimeout", 112: "BeginTransaction", 113: "EndTransaction", 114: "Undefined",
		115: "Undefined", 116: "Undefined", 117: "MatchLiteral", 118: "MakeVector",
	}
	for i, name := range more {
		opcodeNames[i] = name
	}
	for i := 119; i <= 127; i++ {
		opcodeNames[i] = "None"
	}
	for i := 128; i <= 159; i++ {
		opcodeNames[i] = "Undefined"
	}
	for i := 160; i <= 175; i++ {
		opcodeNames[i] = "PushVariable"
	}
	for i := 176; i <= 191; i++ {
		opcodeNames[i] = "PopVariable"
	}
	for i := 192; i <= 207; i++ {
		opcodeNames[i] = "PushGlobal"
	}
	for i := 208; i <= 223; i++ {
		opcodeNames[i] = "PopGlobal"
	}
	for i := 224; i <= 255; i++ {
		opcodeNames[i] = "PushLiteral"
	}
}

func (o Opcode) Name() string {
	if name := opcodeNames[byte(o)]; name != "" {
		return name
	}
	return fmt.Sprintf("Opcode%02X", byte(o))
}

func Names() [256]string { return opcodeNames }

type OperandKind string

const (
	OperandSignedWord    OperandKind = "signed-word"
	OperandLiteralIndex  OperandKind = "literal-index"
	OperandVariableIndex OperandKind = "variable-index"
	OperandParentDepth   OperandKind = "parent-depth"
	OperandBranchTarget  OperandKind = "branch-target"
)

type Operand struct {
	Kind  OperandKind `json:"kind"`
	Value int         `json:"value"`
}

type Instruction struct {
	Offset   int       `json:"offset"`
	Opcode   Opcode    `json:"opcode"`
	Mnemonic string    `json:"mnemonic"`
	Raw      []byte    `json:"raw"`
	Operands []Operand `json:"operands,omitempty"`
}
