package ast

import (
	"strings"
	"testing"

	"applescript-tools/terminology"
)

func contractFormatter(t *testing.T) *Formatter {
	t.Helper()
	terms, err := terminology.Default()
	if err != nil {
		t.Fatal(err)
	}
	return &Formatter{Terms: terms}
}

func TestScriptLayoutAndDeclarationContracts(t *testing.T) {
	script := &Script{
		Uses: []Use{
			{Name: "Foundation", Framework: true},
			{ScriptingAdditions: true},
			{Name: "Kevin's Library", Alias: "KevinLib"},
		},
		Properties: []Property{{Name: "greeting_prefix", Value: &StringLiteral{Value: "Hello"}}},
		Handlers: []*Handler{{
			Name: "run", IsRunHandler: true,
			Body: []Stmt{
				&Declaration{Names: []string{"localValue"}},
				&Declaration{Names: []string{"sharedValue"}, Global: true},
				&Comment{Text: "port contract"},
				&Return{Value: &MissingLiteral{}, Explicit: true},
			},
		}},
	}
	source, err := contractFormatter(t).Script(script)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`use framework "Foundation"`,
		"use scripting additions",
		`use KevinLib : script "Kevin's Library"`,
		`property greeting_prefix : "Hello"`,
		"on run\n",
		"    local localValue\n",
		"    global sharedValue\n",
		"    -- port contract\n",
		"    return missing value\n",
		"end run\n",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("source missing %q:\n%s", want, source)
		}
	}
}

func TestScriptObjectLayoutContract(t *testing.T) {
	object := &ScriptObject{
		Name:       "Worker",
		Properties: []Property{{Name: "results", Value: &List{}}},
		Handlers: []*Handler{{
			Name: "collect",
			Body: []Stmt{&Return{Value: &Variable{Name: "results"}, Explicit: true}},
		}},
	}
	want := "script Worker\n" +
		"    property results : {}\n" +
		"\n" +
		"    on collect()\n" +
		"        return results\n" +
		"    end collect\n" +
		"end script"
	if got := contractFormatter(t).scriptObject(object, 0); got != want {
		t.Fatalf("script object layout:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRawEventHandlerRendering(t *testing.T) {
	event, _ := terminology.ParseEventCode("emalcpma")
	rule, _ := terminology.ParseCode4("pmar")
	handler := &Handler{
		Name:      "perform mail action with messages",
		EventCode: &event,
		Parameters: []Parameter{
			{Name: "theMessages"},
			{Name: "theRule", Code: &rule},
		},
		Body: []Stmt{&Return{Explicit: true}},
	}
	got := contractFormatter(t).handler(handler, 0)
	want := "on «event emalcpma» theMessages given «class pmar»:theRule\n" +
		"    return\n" +
		"end «event emalcpma»"
	if got != want {
		t.Fatalf("handler =\n%s\nwant:\n%s", got, want)
	}
}

func TestEveryRepeatHeader(t *testing.T) {
	f := contractFormatter(t)
	cases := []struct {
		node *Repeat
		want string
	}{
		{&Repeat{Kind: RepeatForever}, "repeat\nend repeat"},
		{&Repeat{Kind: RepeatWhile, Condition: &BooleanLiteral{Value: true}, Body: []Stmt{&ExitRepeat{}}}, "repeat while true\n    exit repeat\nend repeat"},
		{&Repeat{Kind: RepeatUntil, Condition: &BooleanLiteral{}}, "repeat until false\nend repeat"},
		{&Repeat{Kind: RepeatTimes, Times: &NumberLiteral{Integer: 3}}, "repeat 3 times\nend repeat"},
		{&Repeat{Kind: RepeatRange, Variable: "i", From: &NumberLiteral{Integer: 1}, To: &NumberLiteral{Integer: 4}}, "repeat with i from 1 to 4\nend repeat"},
		{&Repeat{Kind: RepeatRange, Variable: "i", From: &NumberLiteral{Integer: 1}, To: &NumberLiteral{Integer: 5}, By: &NumberLiteral{Integer: 2}}, "repeat with i from 1 to 5 by 2\nend repeat"},
		{&Repeat{Kind: RepeatIn, Variable: "i", Collection: &List{Elements: []Expr{&NumberLiteral{Integer: 1}}}}, "repeat with i in {1}\nend repeat"},
	}
	for _, test := range cases {
		if got := f.stmt(test.node, 0); got != test.want {
			t.Errorf("repeat = %q, want %q", got, test.want)
		}
	}
}

func TestIfTryAndErrorBindingRendering(t *testing.T) {
	node := &Try{
		Body: []Stmt{&If{
			Condition: &BooleanLiteral{Value: true},
			Then:      []Stmt{&Set{Target: &Variable{Name: "answer"}, Value: &NumberLiteral{Integer: 1}}},
			Else:      []Stmt{&Return{Value: &NumberLiteral{}, Explicit: true}},
		}},
		ErrorName:  "messageText",
		NumberName: "numberValue",
		ErrorBody:  []Stmt{&Return{Value: &Variable{Name: "numberValue"}, Explicit: true}},
	}
	want := "try\n" +
		"    if true then\n" +
		"        set answer to 1\n" +
		"    else\n" +
		"        return 0\n" +
		"    end if\n" +
		"on error messageText number numberValue\n" +
		"    return numberValue\n" +
		"end try"
	if got := contractFormatter(t).stmt(node, 0); got != want {
		t.Fatalf("try =\n%s\nwant:\n%s", got, want)
	}
}

func TestExtendedErrorBindingRendering(t *testing.T) {
	node := &Try{
		ErrorName: "messageText", NumberName: "numberValue",
		PartialResultName: "partialValue", FromName: "fromValue", ToName: "toValue",
		ErrorBody: []Stmt{&Return{Value: &Variable{Name: "numberValue"}, Explicit: true}},
	}
	got := contractFormatter(t).stmt(node, 0)
	want := "on error messageText number numberValue partial result partialValue from fromValue to toValue"
	if !strings.Contains(got, want) {
		t.Fatalf("try does not contain %q:\n%s", want, got)
	}
}

func TestLiteralRecordAndSpecifierContracts(t *testing.T) {
	f := contractFormatter(t)
	if got := f.expr(&DateLiteral{Value: "1 January 2025"}, 0); got != `date "1 January 2025"` {
		t.Errorf("date = %q", got)
	}
	record := &Record{Fields: []RecordField{
		{Label: &StringLiteral{Value: "safe_name"}, Value: &NumberLiteral{Integer: 1}},
		{Label: &StringLiteral{Value: "not valid"}, Value: &NumberLiteral{Integer: 2}},
	}}
	if got := f.expr(record, 0); got != "{safe_name:1, |not valid|:2}" {
		t.Errorf("record = %q", got)
	}
	container := &Variable{Name: "values"}
	cases := []struct {
		node Expr
		want string
	}{
		{&Specifier{Kind: IndexSpecifier, Object: &Keyword{Fallback: "item"}, From: &NumberLiteral{Integer: 2}, Container: container}, "item 2 of values"},
		{&Specifier{Kind: KeySpecifier, Object: &Keyword{Fallback: "item"}, KeyName: "id", From: &NumberLiteral{Integer: 7}, Container: &Me{}}, "item id 7"},
		{&Specifier{Kind: KeySpecifier, Object: &Keyword{Fallback: "item"}, KeyName: "name", From: &StringLiteral{Value: "target"}, Container: container}, `item name "target" of values`},
		{&Specifier{Kind: PropertySpecifier, Object: &Keyword{Fallback: "name"}, Container: container}, "name of values"},
		{&Specifier{Kind: EverySpecifier, Object: &Keyword{Fallback: "item"}, Container: container}, "every item of values"},
		{&Specifier{Kind: MiddleSpecifier, Object: &Keyword{Fallback: "item"}, Container: container}, "middle item of values"},
		{&Specifier{Kind: SomeSpecifier, Object: &Keyword{Fallback: "item"}, Container: container}, "some item of values"},
		{&Specifier{Kind: BeginningSpecifier, Container: container}, "beginning of values"},
		{&Specifier{Kind: EndSpecifier, Container: container}, "end of (values)"},
		{&Specifier{Kind: RangeSpecifier, Object: &Keyword{Fallback: "text"}, From: &NumberLiteral{Integer: 1}, To: &NumberLiteral{Integer: -2}, Container: &Variable{Name: "ret"}}, "text 1 thru -2 of ret"},
	}
	for _, test := range cases {
		if got := f.expr(test.node, 0); got != test.want {
			t.Errorf("%T = %q, want %q", test.node, got, test.want)
		}
	}
	whose := &Whose{
		Object: &Specifier{Kind: PropertySpecifier, Object: &Keyword{Fallback: "name"}, Container: &Specifier{
			Kind: IndexSpecifier, Object: &Variable{Name: "process"}, From: &NumberLiteral{Integer: 1}, Container: &It{},
		}},
		Predicate: &Binary{Op: Equal, Left: &Variable{Name: "frontmost"}, Right: &BooleanLiteral{Value: true}},
	}
	if got := f.expr(whose, 0); got != "name of process 1 whose frontmost = true" {
		t.Errorf("whose = %q", got)
	}
}

func TestEveryExpressionOperator(t *testing.T) {
	f := contractFormatter(t)
	left, right := &Variable{Name: "left"}, &Variable{Name: "right"}
	cases := []struct {
		op   BinaryKind
		want string
	}{
		{Equal, "left = right"}, {NotEqual, "left ≠ right"},
		{Greater, "left > right"}, {GreaterEqual, "left ≥ right"},
		{Less, "left < right"}, {LessEqual, "left ≤ right"},
		{StartsWith, "left starts with right"}, {EndsWith, "left ends with right"},
		{Contains, "left contains right"}, {And, "left and right"}, {Or, "left or right"},
		{Add, "left + right"}, {Subtract, "left - right"}, {Multiply, "left * right"},
		{Divide, "left / right"}, {Quotient, "left div right"}, {Remainder, "left mod right"},
		{Power, "left ^ right"}, {Concatenate, "left & right"}, {Of, "left of right"},
	}
	for _, test := range cases {
		if got := f.expr(&Binary{Op: test.op, Left: left, Right: right}, 0); got != test.want {
			t.Errorf("%s = %q, want %q", test.op, got, test.want)
		}
	}
	if got := f.expr(&Unary{Op: UnaryNegate, Value: &NumberLiteral{Integer: 2}}, 0); got != "-(2)" {
		t.Errorf("negate = %q", got)
	}
	if got := f.expr(&Unary{Op: UnaryNot, Value: &BooleanLiteral{Value: true}}, 0); got != "not (true)" {
		t.Errorf("not = %q", got)
	}
	if got := f.expr(&Coerce{Value: left, Type: &Keyword{Fallback: "text"}}, 0); got != "(left as text)" {
		t.Errorf("coerce = %q", got)
	}
}

func TestCommandAndHandlerCallContracts(t *testing.T) {
	f := contractFormatter(t)
	badm, _ := terminology.ParseCode4("badm")
	execCode, _ := terminology.ParseEventCode("sysoexec")
	call := &CommandCall{Code: execCode, Name: "do shell script", Arguments: []Argument{
		DirectArgument{Value: &StringLiteral{Value: "id"}},
		FlagArgument{Code: badm, Name: "administrator privileges", Enabled: true},
	}}
	if got := f.expr(call, 0); got != `(do shell script "id" with administrator privileges)` {
		t.Errorf("command = %q", got)
	}
	call.Arguments[1] = FlagArgument{Code: badm, Name: "administrator privileges", Enabled: false}
	if got := f.expr(call, 0); got != `(do shell script "id" without administrator privileges)` {
		t.Errorf("negative flag = %q", got)
	}
	if got := f.expr(&CommandCall{Name: "activate"}, 0); got != "(activate)" {
		t.Errorf("zero argument command = %q", got)
	}
	if got := f.expr(&CommandCall{Name: "path to", Arguments: []Argument{DirectArgument{Value: &Me{}}}}, 0); got != "(path to me)" {
		t.Errorf("path to me = %q", got)
	}
	if got := f.expr(&CommandCall{Name: "error", Arguments: []Argument{DirectArgument{Value: &StringLiteral{Value: "boom"}}}}, 0); got != `error "boom"` {
		t.Errorf("error command = %q", got)
	}
	if got := f.expr(&HandlerCall{Name: "helper", Target: &Me{}}, 0); got != "my helper()" {
		t.Errorf("me handler = %q", got)
	}
	if got := f.expr(&HandlerCall{Name: "length", Target: &Variable{Name: "nsValue"}}, 0); got != "nsValue's |length|()" {
		t.Errorf("reserved selector = %q", got)
	}
	objcTarget := &Specifier{
		Kind:      PropertySpecifier,
		Object:    &Variable{Name: "NSString"},
		Container: &Keyword{Fallback: "current application"},
	}
	if got := f.expr(&HandlerCall{Name: "stringWithString_", Target: objcTarget, Arguments: []Expr{&Variable{Name: "candidate"}}}, 0); got != "(NSString of current application)'s stringWithString_(candidate)" {
		t.Errorf("specifier receiver = %q", got)
	}
}

func TestTopLevelExpressionOmitsRedundantParentheses(t *testing.T) {
	f := contractFormatter(t)
	if got := f.topLevelExpr(&CommandCall{Name: "path to", Arguments: []Argument{
		DirectArgument{Value: &Me{}},
	}}); got != "path to me" {
		t.Errorf("top-level command = %q", got)
	}
	if got := f.topLevelExpr(&Coerce{
		Value: &StringLiteral{Value: "value"},
		Type:  &Keyword{Fallback: "string"},
	}); got != `"value" as string` {
		t.Errorf("top-level coercion = %q", got)
	}
}

func TestControlAndCopyStatementContracts(t *testing.T) {
	node := &Considering{Options: []string{"numeric strings"}, Body: []Stmt{
		&Timeout{Seconds: &NumberLiteral{Integer: 10}, Body: []Stmt{
			&Copy{Source: &Variable{Name: "originalValues"}, Target: &Variable{Name: "clonedValues"}},
		}},
	}}
	want := "considering numeric strings\n" +
		"    with timeout of 10 seconds\n" +
		"        copy originalValues to clonedValues\n" +
		"    end timeout\n" +
		"end considering"
	if got := contractFormatter(t).stmt(node, 0); got != want {
		t.Fatalf("control statement =\n%s\nwant:\n%s", got, want)
	}
}
