package bytecode

import (
	"encoding/binary"
	"fmt"
)

type Diagnostic struct {
	Offset  int
	Message string
}

type Function struct {
	Offset       int
	Instructions []Instruction
	Diagnostics  []Diagnostic
}

func Decode(functionOffset int, code []byte, strict bool) (*Function, error) {
	out := &Function{Offset: functionOffset}
	for pc := 0; pc < len(code); {
		start := pc
		rawOpcode := code[pc]
		pc++
		op := Opcode(rawOpcode)
		inst := Instruction{Offset: start, Opcode: op, Mnemonic: op.Name()}
		word := func(kind OperandKind) (int, error) {
			if pc+2 > len(code) {
				return 0, fmt.Errorf("truncated operand for %s at 0x%x", inst.Mnemonic, start)
			}
			value := int(int16(binary.BigEndian.Uint16(code[pc : pc+2])))
			pc += 2
			inst.Operands = append(inst.Operands, Operand{Kind: kind, Value: value})
			return value, nil
		}
		branch := func(base int) error {
			value, err := word(OperandSignedWord)
			if err != nil {
				return err
			}
			inst.Operands[len(inst.Operands)-1] = Operand{Kind: OperandBranchTarget, Value: base + value}
			return nil
		}
		var err error
		switch inst.Mnemonic {
		case "PushLiteral":
			inst.Operands = append(inst.Operands, Operand{Kind: OperandLiteralIndex, Value: int(rawOpcode & 0x0f)})
		case "PushVariable", "PopVariable":
			inst.Operands = append(inst.Operands, Operand{Kind: OperandVariableIndex, Value: int(rawOpcode & 0x0f)})
		case "PushGlobal", "PopGlobal":
			inst.Operands = append(inst.Operands, Operand{Kind: OperandLiteralIndex, Value: int(rawOpcode & 0x0f)})
		case "PushLiteralExtended", "PushGlobalExtended", "PopGlobalExtended", "MessageSend", "PositionalMessageSend", "DefineActor":
			_, err = word(OperandLiteralIndex)
		case "PushVariableExtended", "PopVariableExtended", "RepeatInCollection", "RepeatInRange":
			_, err = word(OperandVariableIndex)
		case "PushParentVariable", "PopParentVariable":
			_, err = word(OperandParentDepth)
			if err == nil {
				_, err = word(OperandVariableIndex)
			}
		case "HandleError":
			_, err = word(OperandLiteralIndex)
			if err == nil {
				_, err = word(OperandLiteralIndex)
			}
		case "Jump", "And", "Or", "TestIf", "Consider", "ErrorHandler", "EndErrorHandler":
			err = branch(pc)
		case "Tell":
			_, err = word(OperandSignedWord)
		case "LinkRepeat":
			err = branch(start + 1)
		case "DefineProcedure":
			_, err = word(OperandSignedWord)
		}
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{Offset: start, Message: err.Error()})
			if strict {
				return out, err
			}
			inst.Raw = append([]byte(nil), code[start:]...)
			out.Instructions = append(out.Instructions, inst)
			break
		}
		inst.Raw = append([]byte(nil), code[start:pc]...)
		out.Instructions = append(out.Instructions, inst)
	}
	return out, nil
}
