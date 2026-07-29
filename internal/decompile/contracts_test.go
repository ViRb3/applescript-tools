package decompile

import (
	"context"
	"testing"

	"applescript-tools/ast"
	"applescript-tools/internal/bytecode"
	"applescript-tools/internal/fas"
	"applescript-tools/internal/model"
	"applescript-tools/terminology"
)

func contractState(t *testing.T, literals ...fas.Value) *state {
	t.Helper()
	terms, err := terminology.Default()
	if err != nil {
		t.Fatal(err)
	}
	d := &decompiler{ctx: context.Background(), opts: Options{Terms: terms}}
	fn := &model.Function{Offset: 2, Literals: literals, Code: make([]byte, 64)}
	handler := &ast.Handler{Name: "test"}
	return &state{
		d: d, fn: fn, handler: handler, variables: []string{"first", "second"},
		parentScopes: [][]string{{"outer0", "outer1"}, {}, {"far0", "far1"}},
		blocks:       []*block{{kind: blockRoot}},
	}
}

func instruction(opcode byte, mnemonic string, offset int, operands ...bytecode.Operand) bytecode.Instruction {
	return bytecode.Instruction{
		Opcode: bytecode.Opcode(opcode), Mnemonic: mnemonic, Offset: offset,
		Raw: []byte{opcode}, Operands: operands,
	}
}

func TestArgumentAndFunctionNameRecovery(t *testing.T) {
	scalar := &fas.Bytes{Data: []byte("argv")}
	if got := recoverArgs(scalar); len(got) != 1 || got[0] != scalar {
		t.Fatalf("scalar arguments = %#v", got)
	}
	left, right := &fas.Bytes{Data: []byte("left")}, &fas.Bytes{Data: []byte("right")}
	table := &fas.Vector{Children: []fas.Value{fas.NIL, fas.NIL, &fas.Vector{Children: []fas.Value{left, right}}}}
	if got := recoverArgs(table); len(got) != 2 || got[0] != left || got[1] != right {
		t.Fatalf("argument table = %#v", got)
	}
	if got := recoverArgs(fas.NIL); len(got) != 0 {
		t.Fatalf("nil arguments = %#v", got)
	}

	if name, event, run := functionName(&fas.Bytes{Data: []byte("helper")}, nil); name != "helper" || event != nil || run {
		t.Fatalf("function name = %q, %#v, %v", name, event, run)
	}
	eventValue := &fas.EventIdentifier{}
	copy(eventValue.Fields[0][:], "aevt")
	copy(eventValue.Fields[1][:], "oapp")
	if name, event, run := functionName(&fas.Object{Value: eventValue}, nil); name != "run" || event == nil || !run {
		t.Fatalf("run event = %q, %#v, %v", name, event, run)
	}
}

func TestEventHandlerParameterInference(t *testing.T) {
	terms, err := terminology.Default()
	if err != nil {
		t.Fatal(err)
	}
	event, _ := terminology.ParseEventCode("emalcpma")
	got := inferEventParameters(event, []string{"theMessages", "theRule", "localValue"}, terms)
	if len(got) != 2 {
		t.Fatalf("parameters = %#v", got)
	}
	if got[0].Name != "theMessages" || got[0].Code != nil {
		t.Fatalf("direct parameter = %#v", got[0])
	}
	if got[1].Name != "theRule" || got[1].Code == nil || string(got[1].Code[:]) != "pmar" {
		t.Fatalf("labeled parameter = %#v", got[1])
	}
}

func TestLiteralAndParentIndexGuards(t *testing.T) {
	s := contractState(t, &fas.Bytes{Data: []byte("first")}, &fas.Bytes{Data: []byte("last")})
	if got := s.literal(-1).(*ast.Variable).Name; got != "literal_-1" {
		t.Fatalf("negative literal = %q", got)
	}
	if got := s.literal(1).(*ast.StringLiteral).Value; got != "last" {
		t.Fatalf("valid literal = %q", got)
	}
	if got := s.literal(2).(*ast.Variable).Name; got != "literal_2" {
		t.Fatalf("out-of-range literal = %q", got)
	}
	if got := s.parentVariable(2, 1); got != "far1" {
		t.Fatalf("parent variable across empty scope = %q", got)
	}
}

func TestStackBookkeepingContracts(t *testing.T) {
	s := contractState(t)
	first, second := &ast.Variable{Name: "first"}, &ast.Variable{Name: "second"}
	s.stack = []ast.Expr{first, second}
	if err := s.instruction(instruction(92, "GCSwap", 0)); err != nil {
		t.Fatal(err)
	}
	if s.stack[0] != second || s.stack[1] != first {
		t.Fatalf("swap = %#v", s.stack)
	}
	if err := s.instruction(instruction(90, "Pop", 1)); err != nil {
		t.Fatal(err)
	}
	if len(s.stack) != 1 || s.stack[0] != second {
		t.Fatalf("pop = %#v", s.stack)
	}
	if err := s.instruction(instruction(91, "Dup", 2)); err != nil {
		t.Fatal(err)
	}
	if len(s.stack) != 2 || s.stack[0] != s.stack[1] {
		t.Fatalf("dup = %#v", s.stack)
	}
}

func TestStackUnderflowIsExplicit(t *testing.T) {
	s := contractState(t)
	if _, ok := s.pop().(*ast.MissingLiteral); !ok || !s.underflow {
		t.Fatalf("underflow was not recorded: value=%#v underflow=%v", s.stack, s.underflow)
	}
}

func TestSourceFreeResultOpcodesEmitNothing(t *testing.T) {
	s := contractState(t)
	value := &ast.NumberLiteral{Integer: 7}
	s.stack = []ast.Expr{value}
	for _, mnemonic := range []string{"GetData", "GetResult"} {
		if err := s.instruction(instruction(80, mnemonic, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.stack) != 1 || s.stack[0] != value || len(s.handler.Body) != 0 {
		t.Fatalf("source-free opcode changed state: stack=%#v body=%#v", s.stack, s.handler.Body)
	}
}

func TestStoreResultDiscardsCompilerDuplicate(t *testing.T) {
	s := contractState(t)
	value := &ast.NumberLiteral{Integer: 7}
	s.stack = []ast.Expr{value, value}
	if err := s.instruction(instruction(79, "StoreResult", 0)); err != nil {
		t.Fatal(err)
	}
	if len(s.stack) != 0 || len(s.handler.Body) != 1 {
		t.Fatalf("store result left duplicate: stack=%#v body=%#v", s.stack, s.handler.Body)
	}
	expression, ok := s.handler.Body[0].(*ast.Expression)
	if !ok || expression.Value != value {
		t.Fatalf("stored result = %#v", s.handler.Body)
	}
}

func TestBlockBoundaryAttachmentContracts(t *testing.T) {
	t.Run("shared-boundary-stays-in-then", func(t *testing.T) {
		s := contractState(t)
		parent := &ast.If{Condition: &ast.BooleanLiteral{Value: true}}
		child := &ast.If{Condition: &ast.BooleanLiteral{Value: true}}
		s.blocks = append(s.blocks,
			&block{kind: blockIf, node: parent, elseAt: 10},
			&block{kind: blockIf, node: child, elseAt: 10},
		)
		s.closeAt(10)
		if len(parent.Then) != 1 || parent.Then[0] != child || len(parent.Else) != 0 {
			t.Fatalf("parent = %#v", parent)
		}
	})
	t.Run("child-at-else-boundary-stays-in-then", func(t *testing.T) {
		s := contractState(t)
		parent := &ast.If{Condition: &ast.BooleanLiteral{Value: true}}
		child := &ast.Repeat{Kind: ast.RepeatForever}
		s.blocks = append(s.blocks,
			&block{kind: blockIf, node: parent, elseAt: 10, endAt: 20},
			&block{kind: blockRepeat, node: child, endAt: 10},
		)
		s.closeAt(10)
		if len(parent.Then) != 1 || parent.Then[0] != child || len(parent.Else) != 0 {
			t.Fatalf("parent = %#v", parent)
		}
	})
	t.Run("after-else-target-goes-in-else", func(t *testing.T) {
		s := contractState(t)
		parent := &ast.If{Condition: &ast.BooleanLiteral{Value: true}}
		s.blocks = append(s.blocks, &block{kind: blockIf, node: parent, elseAt: 10, endAt: 20})
		s.closeAt(10)
		statement := &ast.Return{Explicit: true}
		s.emit(statement)
		if len(parent.Then) != 0 || len(parent.Else) != 1 || parent.Else[0] != statement {
			t.Fatalf("parent = %#v", parent)
		}
	})
}

func TestCurrentApplicationTellPreservesExpressionResult(t *testing.T) {
	s := contractState(t)
	call := &ast.CommandCall{Name: "ASCII character", Arguments: []ast.Argument{
		ast.DirectArgument{Value: &ast.NumberLiteral{Integer: 65}},
	}}
	tell := &ast.Tell{Target: &ast.Keyword{Fallback: "current application"}}
	s.blocks = append(s.blocks, &block{kind: blockTell, node: tell})
	s.stack = []ast.Expr{call}
	if err := s.instruction(instruction(85, "EndTell", 0)); err != nil {
		t.Fatal(err)
	}
	if len(s.stack) != 1 || s.stack[0] != call {
		t.Fatalf("implicit tell consumed expression result: %#v", s.stack)
	}
	if len(s.handler.Body) != 0 {
		t.Fatalf("implicit tell emitted source statements: %#v", s.handler.Body)
	}
}

func TestErrorBoundaryKeepsGeneralExpressionInsideTry(t *testing.T) {
	s := contractState(t)
	node := &ast.Try{}
	s.blocks = append(s.blocks, &block{kind: blockTry, node: node})
	expression := &ast.Binary{
		Op: ast.Add, Left: &ast.NumberLiteral{Integer: 1}, Right: &ast.NumberLiteral{Integer: 2},
	}
	s.stack = []ast.Expr{expression}
	inst := instruction(87, "EndErrorHandler", 0,
		bytecode.Operand{Kind: bytecode.OperandBranchTarget, Value: 10},
	)
	if err := s.instruction(inst); err != nil {
		t.Fatal(err)
	}
	if len(node.Body) != 1 || node.Body[0].(*ast.Expression).Value != expression {
		t.Fatalf("try body = %#v", node.Body)
	}
}

func TestCopyAndReturnBoundaryContracts(t *testing.T) {
	s := contractState(t)
	source := &ast.Variable{Name: "oldValue"}
	target := &ast.Variable{Name: "newValue"}
	s.stack = []ast.Expr{source, target}
	if err := s.instruction(instruction(71, "CopyData", 0)); err != nil {
		t.Fatal(err)
	}
	copyStatement, ok := s.handler.Body[0].(*ast.Copy)
	if !ok || copyStatement.Source != source || copyStatement.Target != target {
		t.Fatalf("copy = %#v", s.handler.Body)
	}

	s = contractState(t)
	s.pending = target
	s.stack = []ast.Expr{source}
	if err := s.instruction(instruction(15, "Return", 1)); err != nil {
		t.Fatal(err)
	}
	set, ok := s.handler.Body[0].(*ast.Set)
	if !ok || set.Target != target || set.Value != source {
		t.Fatalf("return assignment = %#v", s.handler.Body)
	}

	s = contractState(t)
	if err := s.instruction(instruction(15, "Return", 1)); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.handler.Body[0].(*ast.Return); !ok {
		t.Fatalf("bare return = %#v", s.handler.Body)
	}
}

func TestErrorOpcodeStackOrder(t *testing.T) {
	s := contractState(t)
	s.stack = []ast.Expr{
		&ast.Me{},
		&ast.Keyword{Code: []byte("errn")},
		&ast.NumberLiteral{Integer: 42},
		&ast.NumberLiteral{Integer: 2},
		&ast.StringLiteral{Value: "boom"},
	}
	if err := s.instruction(instruction(21, "Error", 0)); err != nil {
		t.Fatal(err)
	}
	expression := s.handler.Body[0].(*ast.Expression)
	command := expression.Value.(*ast.CommandCall)
	if command.Name != "error" || len(command.Arguments) != 2 {
		t.Fatalf("error command = %#v", command)
	}
	if message := command.Arguments[0].(ast.DirectArgument).Value.(*ast.StringLiteral).Value; message != "boom" {
		t.Fatalf("error message = %q", message)
	}
	named := command.Arguments[1].(ast.NamedArgument)
	if string(named.Code[:]) != "errn" || named.Value.(*ast.NumberLiteral).Integer != 42 {
		t.Fatalf("error named argument = %#v", named)
	}
}

func TestHandleErrorLiteralBindings(t *testing.T) {
	numberBinding := &fas.Binding{Value: &fas.Bytes{Data: []byte("numberValue")}}
	s := contractState(t, &fas.Bytes{Data: []byte("messageText")}, numberBinding)
	node := &ast.Try{}
	s.blocks = append(s.blocks, &block{kind: blockTry, node: node})
	inst := instruction(88, "HandleError", 0,
		bytecode.Operand{Kind: bytecode.OperandLiteralIndex, Value: 0},
		bytecode.Operand{Kind: bytecode.OperandLiteralIndex, Value: 1},
	)
	if err := s.instruction(inst); err != nil {
		t.Fatal(err)
	}
	if node.ErrorName != "messageText" || node.NumberName != "numberValue" {
		t.Fatalf("error bindings = %#v", node)
	}
}

func TestSpecifierStackContracts(t *testing.T) {
	f := contractState(t)
	container := &ast.Variable{Name: "values"}
	object := &ast.Keyword{Fallback: "item"}

	f.stack = []ast.Expr{container, object, &ast.NumberLiteral{Integer: 2}}
	f.makeSpecifier(24)
	index := f.pop().(*ast.Specifier)
	if index.Kind != ast.IndexSpecifier || index.Container != container ||
		index.From.(*ast.NumberLiteral).Integer != 2 {
		t.Fatalf("index specifier = %#v", index)
	}

	f.stack = []ast.Expr{container, object, &ast.NumberLiteral{Integer: 7}, &ast.Keyword{Code: []byte("ID  ")}}
	f.makeSpecifier(25)
	key := f.pop().(*ast.Specifier)
	if key.Kind != ast.KeySpecifier || key.KeyName != "id" {
		t.Fatalf("key specifier = %#v", key)
	}

	f.stack = []ast.Expr{container, object, &ast.NumberLiteral{Integer: 1}, &ast.NumberLiteral{Integer: 3}}
	f.makeSpecifier(27)
	ranged := f.pop().(*ast.Specifier)
	if ranged.Kind != ast.RangeSpecifier || ranged.From.(*ast.NumberLiteral).Integer != 1 ||
		ranged.To.(*ast.NumberLiteral).Integer != 3 {
		t.Fatalf("range specifier = %#v", ranged)
	}

	predicate := &ast.BooleanLiteral{Value: true}
	f.stack = []ast.Expr{object, predicate}
	f.makeSpecifier(26)
	whose := f.pop().(*ast.Whose)
	if whose.Object != object || whose.Predicate != predicate {
		t.Fatalf("whose = %#v", whose)
	}
}

func TestComparisonContracts(t *testing.T) {
	s := contractState(t)
	left, right := &ast.NumberLiteral{Integer: 1}, &ast.NumberLiteral{Integer: 2}
	for opcode, want := range map[byte]ast.BinaryKind{
		0: ast.Equal, 1: ast.NotEqual, 2: ast.Greater, 3: ast.GreaterEqual,
		4: ast.Less, 5: ast.LessEqual, 6: ast.StartsWith, 7: ast.EndsWith, 8: ast.Contains,
	} {
		s.stack = []ast.Expr{left, right}
		s.makeComparison(opcode)
		if got := s.pop().(*ast.Binary).Op; got != want {
			t.Errorf("comparison %d = %s, want %s", opcode, got, want)
		}
	}
	s.stack = []ast.Expr{left}
	s.makeComparison(11)
	if got := s.pop().(*ast.Unary).Op; got != ast.UnaryNot {
		t.Errorf("not comparison = %s", got)
	}
}

func TestMessageEventCodeContracts(t *testing.T) {
	var want terminology.EventCode
	copy(want[:], "coregetd")
	for _, value := range []fas.Value{
		&fas.Bytes{Data: []byte("coregetd")},
		&fas.Object{Value: func() *fas.EventIdentifier {
			event := &fas.EventIdentifier{}
			copy(event.Fields[0][:], "core")
			copy(event.Fields[1][:], "getd")
			return event
		}()},
		&fas.Vector{Children: []fas.Value{fas.NIL, &fas.Bytes{Data: []byte("coregetd")}}},
	} {
		var got terminology.EventCode
		if !eventCodeFromValue(value, &got) || got != want {
			t.Errorf("event code from %T = %#v", value, got)
		}
	}
}

func TestRawAndEventIdentifierMessageSend(t *testing.T) {
	event := &fas.EventIdentifier{}
	copy(event.Fields[0][:], "core")
	copy(event.Fields[1][:], "getd")
	for _, literalValue := range []fas.Value{
		&fas.Bytes{Data: []byte("coregetd")},
		&fas.Object{Value: event},
	} {
		s := contractState(t, literalValue)
		s.stack = []ast.Expr{&ast.It{}, &ast.NumberLiteral{}}
		s.message(0, false)
		command, ok := s.pop().(*ast.CommandCall)
		if !ok || string(command.Code[:]) != "coregetd" {
			t.Fatalf("message from %T = %#v", literalValue, command)
		}
	}
}

func TestMessageSendPreservesExplicitMeArgument(t *testing.T) {
	event := &fas.EventIdentifier{}
	copy(event.Fields[0][:], "ears")
	copy(event.Fields[1][:], "ffdr")
	s := contractState(t, &fas.Object{Value: event})
	s.stack = []ast.Expr{&ast.Me{}, &ast.NumberLiteral{}}
	s.message(0, false)
	command := s.pop().(*ast.CommandCall)
	if len(command.Arguments) != 1 {
		t.Fatalf("arguments = %#v, want explicit me", command.Arguments)
	}
	direct, ok := command.Arguments[0].(ast.DirectArgument)
	if !ok {
		t.Fatalf("argument = %#v", command.Arguments[0])
	}
	if _, ok := direct.Value.(*ast.Me); !ok {
		t.Fatalf("direct value = %#v, want me", direct.Value)
	}
}

func TestTypedArgumentContracts(t *testing.T) {
	terms, err := terminology.Default()
	if err != nil {
		t.Fatal(err)
	}
	event, _ := terminology.ParseEventCode("sysoexec")
	values := []ast.Expr{
		&ast.StringLiteral{Value: "id"},
		&ast.Keyword{Code: []byte("badm")},
		&ast.BooleanLiteral{Value: true},
	}
	arguments := typedArguments(values, terms, event)
	if len(arguments) != 2 {
		t.Fatalf("arguments = %#v", arguments)
	}
	if _, ok := arguments[0].(ast.DirectArgument); !ok {
		t.Fatalf("direct argument = %#v", arguments[0])
	}
	flag, ok := arguments[1].(ast.FlagArgument)
	if !ok || flag.Name != "administrator privileges" || !flag.Enabled {
		t.Fatalf("flag argument = %#v", arguments[1])
	}
}

func TestConsiderationOptionRecovery(t *testing.T) {
	terms, err := terminology.Default()
	if err != nil {
		t.Fatal(err)
	}
	if got := considerationNames(&ast.Keyword{Code: []byte("nume")}, terms); len(got) != 1 || got[0] != "numeric strings" {
		t.Fatalf("consideration names = %#v", got)
	}
	s := contractState(t)
	s.stack = []ast.Expr{
		&ast.Keyword{Code: []byte("nume")},
		&ast.List{},
	}
	inst := instruction(19, "Consider", 0, bytecode.Operand{Kind: bytecode.OperandBranchTarget, Value: 10})
	if err := s.instruction(inst); err != nil {
		t.Fatal(err)
	}
	node := s.blocks[len(s.blocks)-1].node.(*ast.Considering)
	if len(node.Options) != 1 || node.Options[0] != "numeric strings" {
		t.Fatalf("consider block = %#v", node)
	}
}

func TestExtendedErrorBindingAnnotation(t *testing.T) {
	code := func(value string) terminology.Code4 {
		result, _ := terminology.ParseCode4(value)
		return result
	}
	node := &ast.Try{ErrorBody: []ast.Stmt{&ast.Expression{Value: &ast.CommandCall{
		Name: "error",
		Arguments: []ast.Argument{
			ast.NamedArgument{Code: code("ptlr"), Value: &ast.Variable{Name: "partialValue"}},
			ast.NamedArgument{Code: code("erob"), Value: &ast.Variable{Name: "fromValue"}},
			ast.NamedArgument{Code: code("errt"), Value: &ast.Variable{Name: "toValue"}},
		},
	}}}}
	annotateExtendedErrorBindings(node)
	if node.PartialResultName != "partialValue" || node.FromName != "fromValue" || node.ToName != "toValue" {
		t.Fatalf("extended bindings = %#v", node)
	}
}

func TestNestedErrorBindingsDoNotLeak(t *testing.T) {
	partial, _ := terminology.ParseCode4("ptlr")
	nested := &ast.Try{ErrorBody: []ast.Stmt{&ast.Expression{Value: &ast.CommandCall{
		Name: "error",
		Arguments: []ast.Argument{ast.NamedArgument{
			Code: partial, Value: &ast.Variable{Name: "nestedPartial"},
		}},
	}}}}
	outer := &ast.Try{ErrorBody: []ast.Stmt{nested}}
	annotateExtendedErrorBindings(outer)
	if outer.PartialResultName != "" {
		t.Fatalf("nested binding leaked to outer handler: %#v", outer)
	}
}

func TestUnsupportedOpcodeIsExplicit(t *testing.T) {
	s := contractState(t)
	err := s.instruction(instruction(0x72, "Undefined", 0))
	if err == nil {
		t.Fatal("unsupported opcode did not return an error")
	}
}
