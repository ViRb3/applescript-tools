package applescript

import "fmt"

type Severity uint8

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

type Stage string

const (
	StageParse     Stage = "parse"
	StageDecode    Stage = "decode"
	StageDecompile Stage = "decompile"
	StageTransform Stage = "transform"
	StageFormat    Stage = "format"
)

// Diagnostic describes a recoverable problem. Fatal failures are returned as
// errors, optionally alongside diagnostics collected before the failure.
type Diagnostic struct {
	Severity Severity `json:"severity"`
	Stage    Stage    `json:"stage"`
	Offset   int64    `json:"offset,omitempty"`
	Function int      `json:"function,omitempty"`
	Opcode   *byte    `json:"opcode,omitempty"`
	Message  string   `json:"message"`
}

func (d Diagnostic) String() string {
	location := ""
	if d.Offset >= 0 {
		location = fmt.Sprintf(" at 0x%x", d.Offset)
	}
	return fmt.Sprintf("%s%s: %s", d.Stage, location, d.Message)
}
