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

type operandEncoding uint8

const (
	encodeNone operandEncoding = iota
	encodeLiteral
	encodeVariable
	encodeParentVariable
	encodeErrorBindings
	encodeBranch
	encodeSignedWord
)

var operandEncodings = func() [256]operandEncoding {
	var out [256]operandEncoding
	for _, opcode := range []byte{12, 16, 43, 74, 75, 76, 77, 78, 95, 96, 97, 117} {
		out[opcode] = encodeLiteral
	}
	for _, opcode := range []byte{27, 28, 93, 94} {
		out[opcode] = encodeVariable
	}
	for _, opcode := range []byte{98, 99} {
		out[opcode] = encodeParentVariable
	}
	out[88] = encodeErrorBindings
	for _, opcode := range []byte{9, 10, 19, 20, 23, 29, 82, 87, 89, 112} {
		out[opcode] = encodeBranch
	}
	out[18] = encodeSignedWord
	return out
}()

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
		switch {
		case rawOpcode >= 224:
			inst.Operands = append(inst.Operands, Operand{Kind: OperandLiteralIndex, Value: int(rawOpcode & 0x0f)})
		case rawOpcode >= 160 && rawOpcode <= 191:
			inst.Operands = append(inst.Operands, Operand{Kind: OperandVariableIndex, Value: int(rawOpcode & 0x0f)})
		case rawOpcode >= 192 && rawOpcode <= 223:
			inst.Operands = append(inst.Operands, Operand{Kind: OperandLiteralIndex, Value: int(rawOpcode & 0x0f)})
		case operandEncodings[rawOpcode] == encodeLiteral:
			_, err = word(OperandLiteralIndex)
		case operandEncodings[rawOpcode] == encodeVariable:
			_, err = word(OperandVariableIndex)
		case operandEncodings[rawOpcode] == encodeParentVariable:
			_, err = word(OperandParentDepth)
			if err == nil {
				_, err = word(OperandVariableIndex)
			}
		case operandEncodings[rawOpcode] == encodeErrorBindings:
			_, err = word(OperandLiteralIndex)
			if err == nil {
				_, err = word(OperandLiteralIndex)
			}
		case operandEncodings[rawOpcode] == encodeBranch:
			err = branch(pc)
		case operandEncodings[rawOpcode] == encodeSignedWord:
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
