package applescript

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"applescript-tools/internal/bytecode"
	"applescript-tools/internal/fas"
	"applescript-tools/internal/model"
)

const DisassemblySchemaVersion = 1

type DisassembleOptions struct {
	Strict bool
	Limits Limits
}

type Limits struct {
	MaxInputBytes int64
	MaxObjects    int
	MaxReferences int
	MaxDepth      int
	MaxBlobBytes  int
}

type Disassembly struct {
	SchemaVersion int                   `json:"schema_version"`
	Version       string                `json:"fas_version"`
	Functions     []DisassemblyFunction `json:"functions"`
	Diagnostics   []Diagnostic          `json:"diagnostics,omitempty"`
}

type DisassemblyFunction struct {
	Offset       int                      `json:"offset"`
	Name         string                   `json:"name"`
	Arguments    string                   `json:"arguments"`
	Instructions []DisassemblyInstruction `json:"instructions"`
}

type DisassemblyInstruction struct {
	Offset      int                `json:"offset"`
	Opcode      byte               `json:"opcode"`
	Mnemonic    string             `json:"mnemonic"`
	RawHex      string             `json:"raw_hex"`
	Operands    []bytecode.Operand `json:"operands,omitempty"`
	Annotations []string           `json:"annotations,omitempty"`
}

func Disassemble(ctx context.Context, r io.Reader, opts DisassembleOptions) (*Disassembly, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limits := fas.Limits{
		MaxInputBytes: opts.Limits.MaxInputBytes,
		MaxObjects:    opts.Limits.MaxObjects,
		MaxReferences: opts.Limits.MaxReferences,
		MaxDepth:      opts.Limits.MaxDepth,
		MaxBlobBytes:  opts.Limits.MaxBlobBytes,
	}
	doc, err := fas.Parse(r, fas.Options{Strict: opts.Strict, Limits: limits})
	if err != nil {
		return nil, err
	}
	script, err := model.Normalize(doc)
	if err != nil {
		return nil, err
	}
	out := &Disassembly{SchemaVersion: DisassemblySchemaVersion, Version: string(doc.Version[:])}
	for _, d := range doc.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, Diagnostic{Severity: SeverityWarning, Stage: StageParse, Offset: d.Offset, Function: -1, Message: d.Message})
	}
	for _, fn := range script.Functions {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		decoded, decodeErr := bytecode.Decode(fn.Offset, fn.Code, opts.Strict)
		if decodeErr != nil {
			return out, decodeErr
		}
		publicFn := DisassemblyFunction{Offset: fn.Offset, Name: displayValue(fn.Name), Arguments: displayValue(fn.Arguments)}
		for _, d := range decoded.Diagnostics {
			opcode := fn.Code[d.Offset]
			out.Diagnostics = append(out.Diagnostics, Diagnostic{Severity: SeverityWarning, Stage: StageDecode, Offset: int64(d.Offset), Function: fn.Offset, Opcode: &opcode, Message: d.Message})
		}
		for _, inst := range decoded.Instructions {
			item := DisassemblyInstruction{
				Offset: inst.Offset, Opcode: byte(inst.Opcode), Mnemonic: inst.Mnemonic,
				RawHex: hex.EncodeToString(inst.Raw), Operands: inst.Operands,
			}
			for _, operand := range inst.Operands {
				if operand.Kind == bytecode.OperandLiteralIndex {
					if literal, ok := fn.Literal(operand.Value); ok {
						item.Annotations = append(item.Annotations, displayValue(literal))
					} else {
						item.Annotations = append(item.Annotations, fmt.Sprintf("literal[%d] out of range", operand.Value))
					}
				}
			}
			publicFn.Instructions = append(publicFn.Instructions, item)
		}
		out.Functions = append(out.Functions, publicFn)
	}
	return out, nil
}

func (d *Disassembly) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "FAS %s\n", d.Version)
	for _, fn := range d.Functions {
		fmt.Fprintf(&b, "\nfunction[%d] %s arguments=%s\n", fn.Offset, fn.Name, fn.Arguments)
		for _, inst := range fn.Instructions {
			fmt.Fprintf(&b, "  %05x  %-12s %-24s", inst.Offset, inst.RawHex, inst.Mnemonic)
			for i, operand := range inst.Operands {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%s=%s", operand.Kind, formatOperand(operand))
			}
			if len(inst.Annotations) != 0 {
				b.WriteString("  ; ")
				b.WriteString(strings.Join(inst.Annotations, ", "))
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formatOperand(op bytecode.Operand) string {
	if op.Kind == bytecode.OperandBranchTarget {
		return fmt.Sprintf("0x%x", op.Value)
	}
	return strconv.Itoa(op.Value)
}
