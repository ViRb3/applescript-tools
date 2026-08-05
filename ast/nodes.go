package ast

import "applescript-tools/terminology"

type Origin struct {
	Function int `json:"function"`
	Start    int `json:"start"`
	End      int `json:"end"`
}

type Node interface {
	GetOrigin() Origin
}

type Base struct {
	Origin Origin `json:"origin"`
}

func (b Base) GetOrigin() Origin { return b.Origin }

type Expr interface {
	Node
	isExpr()
}

type Stmt interface {
	Node
	isStmt()
}

type Script struct {
	Base
	Uses       []Use
	Properties []Property
	Objects    []*ScriptObject
	Handlers   []*Handler
}

type Use struct {
	Base
	Name               string
	Alias              string
	Framework          bool
	ScriptingAdditions bool
}

type Property struct {
	Base
	Name  string
	Value Expr
}

type Handler struct {
	Base
	Name                 string
	EventCode            *terminology.EventCode
	Parameters           []Parameter
	Body                 []Stmt
	IsRunHandler         bool
	UnresolvedParameters bool
}

type Parameter struct {
	Name string
	Code *terminology.Code4
}

type StringLiteral struct {
	Base
	Value string
}

func (*StringLiteral) isExpr() {}

type NumberLiteral struct {
	Base
	Integer int64
	Real    float64
	IsReal  bool
}

func (*NumberLiteral) isExpr() {}

type BooleanLiteral struct {
	Base
	Value bool
}

func (*BooleanLiteral) isExpr() {}

type MissingLiteral struct{ Base }

func (*MissingLiteral) isExpr() {}

type DateLiteral struct {
	Base
	Value string
}

func (*DateLiteral) isExpr() {}

type RawDataLiteral struct {
	Base
	Type terminology.Code4
	Data []byte
}

func (*RawDataLiteral) isExpr() {}

// OpaqueLiteral retains a runtime class and payload whose source semantics are
// not known. It is deliberately rendered as an explicit unsupported form.
type OpaqueLiteral struct {
	Base
	RuntimeType byte
	Data        []byte
}

func (*OpaqueLiteral) isExpr() {}

type Keyword struct {
	Base
	Code     []byte
	Fallback string
}

func (*Keyword) isExpr() {}

type Variable struct {
	Base
	Name string
}

func (*Variable) isExpr() {}

type Application struct {
	Base
	Name string
}

func (*Application) isExpr() {}

type ScriptLibrary struct {
	Base
	Name string
}

func (*ScriptLibrary) isExpr() {}

type Me struct{ Base }

func (*Me) isExpr() {}

type It struct{ Base }

func (*It) isExpr() {}

type Undefined struct{ Base }

func (*Undefined) isExpr() {}

type List struct {
	Base
	Elements []Expr
}

func (*List) isExpr() {}

type RecordField struct {
	Label Expr
	Value Expr
}
type Record struct {
	Base
	Fields []RecordField
}

func (*Record) isExpr() {}

type UnaryKind string

const (
	UnaryNot    UnaryKind = "not"
	UnaryNegate UnaryKind = "-"
)

type Unary struct {
	Base
	Op    UnaryKind
	Value Expr
}

func (*Unary) isExpr() {}

type BinaryKind string

const (
	Equal        BinaryKind = "="
	NotEqual     BinaryKind = "≠"
	Greater      BinaryKind = ">"
	GreaterEqual BinaryKind = "≥"
	Less         BinaryKind = "<"
	LessEqual    BinaryKind = "≤"
	StartsWith   BinaryKind = "starts with"
	EndsWith     BinaryKind = "ends with"
	Contains     BinaryKind = "contains"
	And          BinaryKind = "and"
	Or           BinaryKind = "or"
	Add          BinaryKind = "+"
	Subtract     BinaryKind = "-"
	Multiply     BinaryKind = "*"
	Divide       BinaryKind = "/"
	Quotient     BinaryKind = "div"
	Remainder    BinaryKind = "mod"
	Power        BinaryKind = "^"
	Concatenate  BinaryKind = "&"
	Of           BinaryKind = "of"
)

type Binary struct {
	Base
	Op          BinaryKind
	Left, Right Expr
}

func (*Binary) isExpr() {}

type Coerce struct {
	Base
	Value, Type Expr
}

func (*Coerce) isExpr() {}

type CopyExpr struct {
	Base
	Value Expr
}

func (*CopyExpr) isExpr() {}

type Argument interface{ isArgument() }
type DirectArgument struct{ Value Expr }

func (DirectArgument) isArgument() {}

type NamedArgument struct {
	Code  terminology.Code4
	Name  string
	Value Expr
}

func (NamedArgument) isArgument() {}

type FlagArgument struct {
	Code    terminology.Code4
	Name    string
	Enabled bool
}

func (FlagArgument) isArgument() {}

type CommandCall struct {
	Base
	Code      terminology.EventCode
	Name      string
	Target    Expr
	Arguments []Argument
}

func (*CommandCall) isExpr() {}

type HandlerCall struct {
	Base
	Name      string
	Target    Expr
	Arguments []Expr
}

func (*HandlerCall) isExpr() {}

type SpecifierKind string

const (
	PropertySpecifier  SpecifierKind = "property"
	EverySpecifier     SpecifierKind = "every"
	SomeSpecifier      SpecifierKind = "some"
	IndexSpecifier     SpecifierKind = "index"
	KeySpecifier       SpecifierKind = "key"
	RangeSpecifier     SpecifierKind = "range"
	BeginningSpecifier SpecifierKind = "beginning"
	EndSpecifier       SpecifierKind = "end"
	MiddleSpecifier    SpecifierKind = "middle"
)

type Specifier struct {
	Base
	Kind      SpecifierKind
	Object    Expr
	Container Expr
	From      Expr
	To        Expr
	KeyName   string
}

func (*Specifier) isExpr() {}

type Whose struct {
	Base
	Object, Predicate Expr
}

func (*Whose) isExpr() {}

type ScriptObject struct {
	Base
	Name       string
	Properties []Property
	Handlers   []*Handler
}

func (*ScriptObject) isExpr() {}

type Set struct {
	Base
	Target, Value Expr
}

func (*Set) isStmt() {}

type Copy struct {
	Base
	Source, Target Expr
}

func (*Copy) isStmt() {}

type Expression struct {
	Base
	Value Expr
}

func (*Expression) isStmt() {}

type Declaration struct {
	Base
	Names  []string
	Global bool
}

func (*Declaration) isStmt() {}

type If struct {
	Base
	Condition  Expr
	Then, Else []Stmt
}

func (*If) isStmt() {}

type RepeatKind string

const (
	RepeatForever RepeatKind = "forever"
	RepeatTimes   RepeatKind = "times"
	RepeatWhile   RepeatKind = "while"
	RepeatUntil   RepeatKind = "until"
	RepeatIn      RepeatKind = "in"
	RepeatRange   RepeatKind = "range"
)

type Repeat struct {
	Base
	Kind                                       RepeatKind
	Variable                                   string
	Times, Condition, Collection, From, To, By Expr
	Body                                       []Stmt
}

func (*Repeat) isStmt() {}

type Try struct {
	Base
	Body                                []Stmt
	ErrorName, NumberName               string
	PartialResultName, FromName, ToName string
	ErrorBody                           []Stmt
}

func (*Try) isStmt() {}

type Tell struct {
	Base
	Target Expr
	Body   []Stmt
}

func (*Tell) isStmt() {}

type Considering struct {
	Base
	Options []string
	Body    []Stmt
}

func (*Considering) isStmt() {}

type Timeout struct {
	Base
	Seconds Expr
	Body    []Stmt
}

func (*Timeout) isStmt() {}

type Transaction struct {
	Base
	Body []Stmt
}

func (*Transaction) isStmt() {}

type Continue struct {
	Base
	Call Expr
}

func (*Continue) isStmt() {}

type Return struct {
	Base
	Value    Expr
	Explicit bool
}

func (*Return) isStmt() {}

type ExitRepeat struct{ Base }

func (*ExitRepeat) isStmt() {}

type Comment struct {
	Base
	Text string
}

func (*Comment) isStmt() {}
