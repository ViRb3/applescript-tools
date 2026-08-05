package decompile

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"applescript-tools/ast"
	"applescript-tools/internal/bytecode"
	"applescript-tools/internal/fas"
	"applescript-tools/internal/model"
	"applescript-tools/terminology"
)

type Diagnostic struct {
	Function int
	Offset   int
	Message  string
}

type Options struct {
	Strict bool
	Terms  *terminology.Registry
}

type Result struct {
	Script      *ast.Script
	Diagnostics []Diagnostic
}

func Run(ctx context.Context, script *model.Script, opts Options) (*Result, error) {
	d := &decompiler{ctx: ctx, script: script, opts: opts}
	output := &ast.Script{}
	d.output = output
	d.extractRoot()
	rootScope := make([]string, len(script.RootNames))
	for i, value := range script.RootNames {
		rootScope[i] = identifierValue(value, fmt.Sprintf("parent_var_%d", i))
	}
	for _, actor := range script.Actors {
		if err := d.ctx.Err(); err != nil {
			return &Result{Script: output, Diagnostics: d.diagnostics}, err
		}
		output.Objects = append(output.Objects, d.actorTable(actor, [][]string{rootScope}))
	}
	for _, function := range script.Functions {
		handler, err := d.function(function, [][]string{rootScope})
		if err != nil {
			if opts.Strict {
				return &Result{Script: output, Diagnostics: d.diagnostics}, err
			}
			d.diagnostics = append(d.diagnostics, Diagnostic{Function: function.Offset, Offset: -1, Message: err.Error()})
			continue
		}
		if handler != nil {
			output.Handlers = append(output.Handlers, handler)
		}
	}
	return &Result{Script: output, Diagnostics: d.diagnostics}, nil
}

type decompiler struct {
	ctx         context.Context
	script      *model.Script
	opts        Options
	output      *ast.Script
	diagnostics []Diagnostic
}

func (d *decompiler) extractRoot() {
	actorOffsets := make(map[int]bool, len(d.script.Actors))
	for _, actor := range d.script.Actors {
		actorOffsets[actor.RootOffset] = true
	}
	for offset := 2; offset < len(d.script.Entries); offset++ {
		if actorOffsets[offset] {
			continue
		}
		entry := d.script.Entries[offset]
		if raw, ok := entry.(*fas.Vector); ok && len(raw.Children) >= 8 {
			if _, ok := raw.Children[7].(*fas.Bytes); ok {
				continue
			}
		}
		index := offset - 2
		if index >= len(d.script.RootNames) {
			continue
		}
		name := identifierValue(d.script.RootNames[index], "")
		if name == "" && isConstant(d.script.RootNames[index], "pimr") {
			d.extractUses(entry)
			continue
		}
		if name == "" {
			continue
		}
		value := d.rootInitializer(entry)
		if library, ok := value.(*ast.ScriptLibrary); ok {
			d.output.Uses = append(d.output.Uses, ast.Use{Name: library.Name, Alias: name})
			continue
		}
		d.output.Properties = append(d.output.Properties, ast.Property{Name: name, Value: value})
	}
}

func (d *decompiler) actorTable(actor *model.Actor, parentScopes [][]string) *ast.ScriptObject {
	name := identifierValue(actor.Name, "scriptObject")
	object := &ast.ScriptObject{Name: name}
	functionOffsets := make(map[int]bool, len(actor.Functions))
	for _, function := range actor.Functions {
		functionOffsets[function.Offset] = true
	}
	for offset := 2; offset < len(actor.Entries); offset++ {
		if functionOffsets[offset] {
			continue
		}
		index := offset - 2
		if index >= len(actor.Names) {
			continue
		}
		propertyName := identifierValue(actor.Names[index], "")
		if propertyName == "" || propertyName == "object" {
			continue
		}
		object.Properties = append(object.Properties, ast.Property{
			Name: propertyName, Value: d.rootInitializer(actor.Entries[offset]),
		})
	}
	actorScope := make([]string, len(actor.Names))
	for i, value := range actor.Names {
		actorScope[i] = identifierValue(value, fmt.Sprintf("var_%d", i))
	}
	scopes := append([][]string{actorScope}, parentScopes...)
	for _, function := range actor.Functions {
		handler, err := d.function(function, scopes)
		if err != nil {
			d.diagnostics = append(d.diagnostics, Diagnostic{Function: function.Offset, Offset: -1, Message: fmt.Sprintf("%s: %v", actor.Path, err)})
			continue
		}
		if handler != nil {
			object.Handlers = append(object.Handlers, handler)
		}
	}
	return object
}

func (d *decompiler) rootInitializer(entry fas.Value) ast.Expr {
	outer, ok := entry.(*fas.Vector)
	if !ok || outer.Type != 20 || len(outer.Children) <= 1 {
		return literal(entry)
	}
	descriptor, ok := outer.Children[1].(*fas.Vector)
	if !ok {
		return literal(entry)
	}
	if descriptor.Type == 21 && len(descriptor.Children) > 2 {
		target := literal(descriptor.Children[1])
		propertyName := identifierValue(descriptor.Children[2], "")
		if propertyName != "" {
			return &ast.CopyExpr{Value: &ast.Specifier{Kind: ast.PropertySpecifier, Object: &ast.Variable{Name: propertyName}, Container: target}}
		}
	}
	if descriptor.Type == 24 && len(descriptor.Children) > 3 && isConstant(descriptor.Children[2], "scpt") {
		if name, ok := literal(descriptor.Children[3]).(*ast.StringLiteral); ok {
			return &ast.ScriptLibrary{Name: name.Value}
		}
	}
	return literal(entry)
}

func isConstant(value fas.Value, code string) bool {
	object, ok := value.(*fas.Object)
	if !ok {
		return false
	}
	constant, ok := object.Value.(fas.Constant)
	if !ok {
		return false
	}
	raw := make([]byte, 4)
	n := uint64(constant)
	for i := 3; i >= 0; i-- {
		raw[i] = byte(n)
		n >>= 8
	}
	return string(raw) == code
}

func (d *decompiler) extractUses(entry fas.Value) {
	outer, ok := entry.(*fas.Vector)
	if !ok || len(outer.Children) <= 2 {
		return
	}
	list, ok := outer.Children[2].(*fas.Vector)
	if !ok {
		return
	}
	start := 0
	if list.HasType {
		start = 1
	}
	for _, child := range list.Children[start:] {
		binding, ok := child.(*fas.Binding)
		if !ok {
			continue
		}
		wrapper, ok := binding.Value.(*fas.Vector)
		if !ok || len(wrapper.Children) <= 1 {
			continue
		}
		descriptor, ok := wrapper.Children[1].(*fas.Vector)
		if !ok {
			continue
		}
		if descriptor.Type == 24 && len(descriptor.Children) > 3 && isConstant(descriptor.Children[2], "frmk") {
			if name, ok := literal(descriptor.Children[3]).(*ast.StringLiteral); ok {
				d.output.Uses = append(d.output.Uses, ast.Use{Name: name.Value, Framework: true})
			}
		} else if descriptor.Type == 22 && len(descriptor.Children) > 2 && isConstant(descriptor.Children[2], "osax") {
			d.output.Uses = append(d.output.Uses, ast.Use{ScriptingAdditions: true})
		}
	}
}

type blockKind uint8

const (
	blockRoot blockKind = iota
	blockIf
	blockRepeat
	blockTry
	blockTell
	blockConsider
	blockTimeout
	blockTransaction
	blockOf
)

type block struct {
	kind      blockKind
	node      ast.Stmt
	elseAt    int
	endAt     int
	inElse    bool
	container ast.Expr
	stackBase int
}

type state struct {
	d            *decompiler
	fn           *model.Function
	decoded      *bytecode.Function
	handler      *ast.Handler
	stack        []ast.Expr
	blocks       []*block
	pending      ast.Expr
	previous     string
	variables    []string
	parentScopes [][]string
	logic        []logicState
	undefined    int
	underflow    bool
}
type logicState struct {
	op   ast.BinaryKind
	left ast.Expr
	end  int
}

func (d *decompiler) function(fn *model.Function, parentScopes [][]string) (*ast.Handler, error) {
	decoded, err := bytecode.Decode(fn.Offset, fn.Code, d.opts.Strict)
	if err != nil {
		return nil, err
	}
	for _, item := range decoded.Diagnostics {
		d.diagnostics = append(d.diagnostics, Diagnostic{Function: fn.Offset, Offset: item.Offset, Message: item.Message})
	}
	args := recoverArgs(fn.Arguments)
	variables := make([]string, len(fn.Variables))
	for i, value := range fn.Variables {
		fallback := fmt.Sprintf("var_%d", i)
		if i < len(args) {
			fallback = fmt.Sprintf("arg_%d", i)
		}
		variables[i] = identifierValue(value, fallback)
	}
	name, event, isRun := functionName(fn.Name, d.opts.Terms)
	handler := &ast.Handler{Name: name, EventCode: event, IsRunHandler: isRun}
	if event != nil && !isRun {
		if d.opts.Terms != nil {
			_, known := d.opts.Terms.Command(*event)
			if known {
				var complete bool
				handler.Parameters, complete = inferEventParameters(*event, variables, d.opts.Terms)
				if !complete {
					handler.UnresolvedParameters = true
					d.diagnostics = append(d.diagnostics, Diagnostic{Function: fn.Offset, Offset: -1, Message: "event parameter mapping is ambiguous"})
				}
			}
		}
	}
	if len(handler.Parameters) == 0 {
		for i := range args {
			parameterName := fmt.Sprintf("arg_%d", i)
			if i < len(variables) {
				parameterName = variables[i]
			}
			handler.Parameters = append(handler.Parameters, ast.Parameter{Name: parameterName})
		}
	}
	s := &state{d: d, fn: fn, decoded: decoded, handler: handler, variables: variables, parentScopes: parentScopes}
	s.blocks = []*block{{kind: blockRoot}}
	for _, inst := range decoded.Instructions {
		if err := d.ctx.Err(); err != nil {
			return nil, err
		}
		s.closeAt(inst.Offset)
		s.closeLogic(inst.Offset)
		if err := s.instruction(inst); err != nil {
			d.diagnostics = append(d.diagnostics, Diagnostic{Function: fn.Offset, Offset: inst.Offset, Message: err.Error()})
			if d.opts.Strict {
				return nil, err
			}
		}
		if s.underflow {
			err := fmt.Errorf("expression stack underflow")
			d.diagnostics = append(d.diagnostics, Diagnostic{Function: fn.Offset, Offset: inst.Offset, Message: err.Error()})
			s.underflow = false
			if d.opts.Strict {
				return nil, err
			}
		}
		s.previous = inst.Mnemonic
	}
	s.closeAt(1 << 30)
	for len(s.blocks) > 1 {
		s.closeTop()
	}
	if s.pending != nil && len(s.stack) > 0 {
		s.emit(&ast.Set{Target: s.pending, Value: s.pop()})
		s.pending = nil
	}
	return handler, nil
}

// inferEventParameters reconstructs the parameter labels that AppleScript omits
// from a serialized handler's argument table. The direct parameter is the first
// variable. Named parameters are matched by meaningful words from their SDEF
// names, which mirrors how the compiler derives conventional variable names
// such as "theRule" from "for rule".
func inferEventParameters(event terminology.EventCode, variables []string, terms *terminology.Registry) ([]ast.Parameter, bool) {
	if terms == nil {
		return nil, false
	}
	command, ok := terms.Command(event)
	if !ok {
		return nil, false
	}
	remaining := append([]string(nil), variables...)
	var parameters []ast.Parameter
	if command.HasDirectParameter && len(remaining) != 0 {
		parameters = append(parameters, ast.Parameter{Name: remaining[0]})
		remaining = remaining[1:]
	}

	codes := append([]terminology.Code4(nil), command.ParameterOrder...)
	if len(codes) == 0 && len(command.Parameters) != 0 {
		for code := range command.Parameters {
			codes = append(codes, code)
		}
		sort.Slice(codes, func(i, j int) bool { return string(codes[i][:]) < string(codes[j][:]) })
	}
	for _, code := range codes {
		parameter := command.Parameters[code]
		words := meaningfulParameterWords(parameter.Name)
		matches := make([]int, 0, 1)
		for i, variable := range remaining {
			lower := strings.ToLower(variable)
			for _, word := range words {
				if strings.Contains(lower, word) {
					matches = append(matches, i)
					break
				}
			}
		}
		if len(matches) == 0 && len(codes) == 1 && len(remaining) == 1 {
			matches = append(matches, 0)
		}
		if len(matches) > 1 {
			return parameters, false
		}
		if len(matches) == 0 {
			continue
		}
		match := matches[0]
		codeCopy := code
		parameters = append(parameters, ast.Parameter{Name: remaining[match], Code: &codeCopy})
		remaining = append(remaining[:match], remaining[match+1:]...)
	}
	return parameters, true
}

func meaningfulParameterWords(name string) []string {
	ignored := map[string]bool{"in": true, "for": true, "with": true, "to": true}
	var words []string
	for _, word := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(word) > 1 && !ignored[word] {
			words = append(words, word)
		}
	}
	return words
}

func recoverArgs(value fas.Value) []fas.Value {
	switch value := value.(type) {
	case *fas.Bytes:
		return []fas.Value{value}
	case *fas.Vector:
		if len(value.Children) > 2 {
			if nested, ok := value.Children[2].(*fas.Vector); ok {
				start := 0
				if nested.HasType {
					start = 1
				}
				return nested.Children[start:]
			}
		}
	}
	return nil
}

func functionName(value fas.Value, terms *terminology.Registry) (string, *terminology.EventCode, bool) {
	if bytes, ok := value.(*fas.Bytes); ok {
		return string(bytes.Data), nil, false
	}
	if object, ok := value.(*fas.Object); ok {
		if event, ok := object.Value.(*fas.EventIdentifier); ok {
			var code terminology.EventCode
			copy(code[:4], event.Fields[0][:])
			copy(code[4:], event.Fields[1][:])
			if string(code[:]) == "aevtoapp" {
				return "run", &code, true
			}
			if terms != nil {
				if command, ok := terms.Command(code); ok {
					return command.Name, &code, false
				}
			}
			return fmt.Sprintf("event_%x", code), &code, false
		}
	}
	return "handler", nil, false
}

func (s *state) emit(statement ast.Stmt) {
	current := s.blocks[len(s.blocks)-1]
	switch current.kind {
	case blockRoot:
		s.handler.Body = append(s.handler.Body, statement)
	case blockIf:
		node := current.node.(*ast.If)
		if current.inElse {
			node.Else = append(node.Else, statement)
		} else {
			node.Then = append(node.Then, statement)
		}
	case blockRepeat:
		node := current.node.(*ast.Repeat)
		node.Body = append(node.Body, statement)
	case blockTry:
		node := current.node.(*ast.Try)
		if current.inElse {
			node.ErrorBody = append(node.ErrorBody, statement)
		} else {
			node.Body = append(node.Body, statement)
		}
	case blockTell:
		node := current.node.(*ast.Tell)
		node.Body = append(node.Body, statement)
	case blockConsider:
		node := current.node.(*ast.Considering)
		node.Body = append(node.Body, statement)
	case blockTimeout:
		node := current.node.(*ast.Timeout)
		node.Body = append(node.Body, statement)
	case blockTransaction:
		node := current.node.(*ast.Transaction)
		node.Body = append(node.Body, statement)
	case blockOf:
		// Of/EndOf scopes an expression, not a source statement block.
	}
}
func (s *state) closeTop() {
	b := s.blocks[len(s.blocks)-1]
	s.blocks = s.blocks[:len(s.blocks)-1]
	if b.kind == blockOf {
		value := ast.Expr(&ast.MissingLiteral{})
		if len(s.stack) > b.stackBase {
			value = s.pop()
			s.stack = s.stack[:b.stackBase]
		}
		s.push(&ast.Binary{Op: ast.Of, Left: value, Right: b.container})
		return
	}
	if b.kind == blockTry {
		annotateExtendedErrorBindings(b.node.(*ast.Try))
	}
	if b.kind == blockTell {
		node := b.node.(*ast.Tell)
		if keyword, ok := node.Target.(*ast.Keyword); ok && keyword.Fallback == "current application" {
			for _, statement := range node.Body {
				s.emit(statement)
			}
			return
		}
	}
	s.emit(b.node)
}

func annotateExtendedErrorBindings(node *ast.Try) {
	var visit func([]ast.Stmt)
	visit = func(statements []ast.Stmt) {
		for _, statement := range statements {
			switch statement := statement.(type) {
			case *ast.Expression:
				command, ok := statement.Value.(*ast.CommandCall)
				if !ok || command.Name != "error" {
					continue
				}
				for _, argument := range command.Arguments {
					named, ok := argument.(ast.NamedArgument)
					if !ok {
						continue
					}
					variable, ok := named.Value.(*ast.Variable)
					if !ok {
						continue
					}
					switch string(named.Code[:]) {
					case "ptlr":
						node.PartialResultName = variable.Name
					case "erob":
						node.FromName = variable.Name
					case "errt":
						node.ToName = variable.Name
					}
				}
			case *ast.If:
				visit(statement.Then)
				visit(statement.Else)
			case *ast.Try:
				// A nested handler owns its own error bindings. Letting its
				// rethrow annotate the outer handler changes the outer
				// signature and prevents compiler fixed points.
				continue
			}
		}
	}
	visit(node.ErrorBody)
}
func (s *state) closeAt(pc int) {
	for len(s.blocks) > 1 {
		b := s.blocks[len(s.blocks)-1]
		switch b.kind {
		case blockIf:
			if b.endAt > 0 {
				if pc >= b.endAt {
					s.flushPendingExpression()
					s.closeTop()
					continue
				}
				if pc >= b.elseAt {
					b.inElse = true
				}
			} else if pc >= b.elseAt {
				s.flushPendingExpression()
				s.closeTop()
				continue
			}
		case blockRepeat, blockConsider:
			if b.endAt > 0 && pc >= b.endAt {
				s.flushPendingExpression()
				s.closeTop()
				continue
			}
		case blockTry:
			if b.endAt > 0 && pc >= b.endAt {
				s.flushPendingExpression()
				s.closeTop()
				continue
			}
		}
		break
	}
}

func (s *state) flushPendingExpression() {
	if len(s.stack) == 0 {
		return
	}
	if s.pending != nil {
		value := s.pop()
		s.emit(&ast.Set{Target: s.pending, Value: value})
		s.pending = nil
		return
	}
	value := s.stack[len(s.stack)-1]
	switch value.(type) {
	case *ast.CommandCall, *ast.HandlerCall:
		s.pop()
		if s.pending != nil {
			s.emit(&ast.Set{Target: s.pending, Value: value})
			s.pending = nil
		} else {
			s.emit(&ast.Expression{Value: value})
		}
	}
}
func (s *state) closeLogic(pc int) {
	for len(s.logic) > 0 && pc >= s.logic[len(s.logic)-1].end {
		item := s.logic[len(s.logic)-1]
		s.logic = s.logic[:len(s.logic)-1]
		right := s.pop()
		s.push(&ast.Binary{Op: item.op, Left: item.left, Right: right})
	}
}
func (s *state) push(value ast.Expr) { s.stack = append(s.stack, value) }
func (s *state) pop() ast.Expr {
	if len(s.stack) == 0 {
		s.underflow = true
		return &ast.MissingLiteral{}
	}
	v := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	return v
}
func (s *state) popNumber() int {
	v := s.pop()
	if n, ok := v.(*ast.NumberLiteral); ok {
		if n.IsReal {
			return int(n.Real)
		}
		return int(n.Integer)
	}
	return 0
}
func (s *state) operand(inst bytecode.Instruction, index int) int {
	if index >= len(inst.Operands) {
		return 0
	}
	return inst.Operands[index].Value
}
func (s *state) variable(index int) string {
	if index >= 0 && index < len(s.variables) {
		return s.variables[index]
	}
	return fmt.Sprintf("var_%d", index)
}
func (s *state) literal(index int) ast.Expr {
	value, ok := s.fn.Literal(index)
	if !ok {
		return &ast.Variable{Name: fmt.Sprintf("literal_%d", index)}
	}
	if opaque, ok := value.(*fas.TypedData); ok {
		s.d.diagnostics = append(s.d.diagnostics, Diagnostic{
			Function: s.fn.Offset, Offset: -1,
			Message: fmt.Sprintf("unresolved runtime data type %d", opaque.Type),
		})
	}
	return literal(value)
}

var binaryOps = map[string]ast.BinaryKind{
	"Equal": ast.Equal, "NotEqual": ast.NotEqual, "GreaterThan": ast.Greater, "GreaterThanOrEqual": ast.GreaterEqual,
	"LessThan": ast.Less, "LessThanOrEqual": ast.LessEqual, "StartsWith": ast.StartsWith, "EndsWith": ast.EndsWith,
	"Contains": ast.Contains, "Add": ast.Add, "Subtract": ast.Subtract, "Multiply": ast.Multiply, "Divide": ast.Divide,
	"Quotient": ast.Quotient, "Remainder": ast.Remainder, "Power": ast.Power, "Concatenate": ast.Concatenate,
}

func (s *state) instruction(inst bytecode.Instruction) error {
	op := inst.Mnemonic
	switch {
	case op == "PushLiteral" || op == "PushLiteralExtended":
		s.push(s.literal(s.operand(inst, 0)))
	case op == "Push0" || op == "Push1" || op == "Push2" || op == "Push3":
		s.push(&ast.NumberLiteral{Integer: int64(op[len(op)-1] - '0')})
	case op == "PushMinus1":
		s.push(&ast.NumberLiteral{Integer: -1})
	case op == "PushTrue":
		s.push(&ast.BooleanLiteral{Value: true})
	case op == "PushFalse":
		s.push(&ast.BooleanLiteral{Value: false})
	case op == "PushEmpty":
		s.push(&ast.List{})
	case op == "PushIt":
		s.push(&ast.It{})
	case op == "PushMe":
		s.push(&ast.Me{})
	case op == "PushUndefined":
		s.undefined++
	case op == "PushVariable" || op == "PushVariableExtended":
		s.push(&ast.Variable{Name: s.variable(s.operand(inst, 0))})
	case op == "PopVariable" || op == "PopVariableExtended":
		s.pending = &ast.Variable{Name: s.variable(s.operand(inst, 0))}
	case op == "PushGlobal" || op == "PushGlobalExtended":
		s.push(s.global(s.operand(inst, 0)))
	case op == "PopGlobal" || op == "PopGlobalExtended":
		s.pending = s.global(s.operand(inst, 0))
	case op == "PushParentVariable":
		s.push(&ast.Variable{Name: s.parentVariable(s.operand(inst, 0), s.operand(inst, 1))})
	case op == "PopParentVariable":
		s.pending = &ast.Variable{Name: s.parentVariable(s.operand(inst, 0), s.operand(inst, 1))}
	case op == "Dup":
		if len(s.stack) > 0 {
			s.push(s.stack[len(s.stack)-1])
		}
	case op == "GCSwap":
		if len(s.stack) >= 2 {
			s.stack[len(s.stack)-1], s.stack[len(s.stack)-2] = s.stack[len(s.stack)-2], s.stack[len(s.stack)-1]
		}
	case op == "Pop":
		if s.previous == "PushUndefined" && s.undefined > 0 {
			s.undefined--
		} else if len(s.stack) > 0 {
			s.pop()
		}
	case binaryOps[op] != "":
		right, left := s.pop(), s.pop()
		s.push(&ast.Binary{Op: binaryOps[op], Left: left, Right: right})
	case op == "Coerce":
		right, left := s.pop(), s.pop()
		s.push(&ast.Coerce{Value: left, Type: right})
	case op == "Negate":
		s.push(&ast.Unary{Op: ast.UnaryNegate, Value: s.pop()})
	case op == "Not":
		s.push(&ast.Unary{Op: ast.UnaryNot, Value: s.pop()})
	case op == "And" || op == "Or":
		kind := ast.And
		if op == "Or" {
			kind = ast.Or
		}
		s.logic = append(s.logic, logicState{op: kind, left: s.pop(), end: s.operand(inst, 0)})
	case op == "Jump":
		if len(s.blocks) > 1 {
			b := s.blocks[len(s.blocks)-1]
			if b.kind == blockIf && s.operand(inst, 0) > inst.Offset {
				s.flushPendingExpression()
				b.endAt = s.operand(inst, 0)
			}
		}
	case op == "TestIf":
		node := &ast.If{Condition: s.pop()}
		s.blocks = append(s.blocks, &block{kind: blockIf, node: node, elseAt: s.operand(inst, 0)})
	case op == "LinkRepeat":
		node := &ast.Repeat{Kind: ast.RepeatForever}
		s.blocks = append(s.blocks, &block{kind: blockRepeat, node: node, endAt: s.operand(inst, 0)})
	case op == "RepeatNTimes":
		if b := s.current(blockRepeat); b != nil {
			b.node.(*ast.Repeat).Kind = ast.RepeatTimes
			s.pop()
			b.node.(*ast.Repeat).Times = s.pop()
		}
	case op == "RepeatWhile":
		if b := s.current(blockRepeat); b != nil {
			b.node.(*ast.Repeat).Kind = ast.RepeatWhile
			b.node.(*ast.Repeat).Condition = s.pop()
		}
	case op == "RepeatUntil":
		if b := s.current(blockRepeat); b != nil {
			b.node.(*ast.Repeat).Kind = ast.RepeatUntil
			b.node.(*ast.Repeat).Condition = s.pop()
		}
	case op == "RepeatInCollection":
		if b := s.current(blockRepeat); b != nil {
			node := b.node.(*ast.Repeat)
			node.Kind = ast.RepeatIn
			node.Variable = s.variable(s.operand(inst, 0))
			s.pop()
			s.pop()
			node.Collection = s.pop()
		}
	case op == "RepeatInRange":
		if b := s.current(blockRepeat); b != nil {
			node := b.node.(*ast.Repeat)
			node.Kind = ast.RepeatRange
			node.Variable = s.variable(s.operand(inst, 0))
			node.By = s.pop()
			node.To = s.pop()
			node.From = s.pop()
		}
	case op == "Exit":
		s.emit(&ast.ExitRepeat{})
	case op == "Tell":
		node := &ast.Tell{Target: s.pop()}
		s.blocks = append(s.blocks, &block{kind: blockTell, node: node})
	case op == "EndTell":
		if b := s.current(blockTell); b != nil {
			node := b.node.(*ast.Tell)
			keyword, currentApplication := node.Target.(*ast.Keyword)
			if !currentApplication || keyword.Fallback != "current application" {
				s.flushPendingExpression()
			}
			s.closeThrough(b)
		}
	case op == "Consider":
		optionsValue := s.pop()
		if _, ok := optionsValue.(*ast.List); ok && len(s.stack) > 0 {
			optionsValue = s.pop()
		}
		options := considerationNames(optionsValue, s.d.opts.Terms)
		if len(options) == 0 {
			options = []string{"case"}
		}
		node := &ast.Considering{Options: options}
		s.blocks = append(s.blocks, &block{kind: blockConsider, node: node, endAt: s.operand(inst, 0)})
	case op == "EndConsider":
		if b := s.current(blockConsider); b != nil {
			s.closeThrough(b)
		}
	case op == "BeginTimeout":
		node := &ast.Timeout{Seconds: s.pop()}
		s.blocks = append(s.blocks, &block{kind: blockTimeout, node: node})
	case op == "EndTimeout":
		if b := s.current(blockTimeout); b != nil {
			s.closeThrough(b)
		}
	case op == "BeginTransaction":
		node := &ast.Transaction{}
		s.blocks = append(s.blocks, &block{kind: blockTransaction, node: node, endAt: s.operand(inst, 0)})
	case op == "EndTransaction":
		if b := s.current(blockTransaction); b != nil {
			s.flushPendingExpression()
			s.closeThrough(b)
		}
	case op == "ErrorHandler":
		node := &ast.Try{}
		s.blocks = append(s.blocks, &block{kind: blockTry, node: node, elseAt: s.operand(inst, 0)})
	case op == "EndErrorHandler":
		if b := s.current(blockTry); b != nil {
			s.flushExpression()
			b.inElse = true
			b.endAt = s.operand(inst, 0)
		}
	case op == "HandleError":
		if b := s.current(blockTry); b != nil {
			node := b.node.(*ast.Try)
			node.ErrorName = s.literalName(s.operand(inst, 0))
			node.NumberName = s.literalName(s.operand(inst, 1))
		}
	case op == "MakeVector" || op == "MakeList":
		count := s.popNumber()
		items := make([]ast.Expr, count)
		for i := count - 1; i >= 0; i-- {
			items[i] = s.pop()
		}
		s.push(&ast.List{Elements: items})
	case op == "MakeRecord":
		count := s.popNumber()
		values := make([]ast.Expr, count)
		for i := count - 1; i >= 0; i-- {
			values[i] = s.pop()
		}
		record := &ast.Record{}
		for i := 0; i+1 < len(values); i += 2 {
			record.Fields = append(record.Fields, ast.RecordField{Label: values[i], Value: values[i+1]})
		}
		s.push(record)
	case op == "MakeObjectAlias":
		s.makeSpecifier(byte(inst.Opcode) - 23)
	case op == "MakeComp":
		s.makeComparison(byte(inst.Opcode) - 56)
	case op == "MessageSend":
		s.message(s.operand(inst, 0), false)
	case op == "PositionalMessageSend":
		s.message(s.operand(inst, 0), true)
	case op == "Clone":
		s.push(&ast.CopyExpr{Value: s.pop()})
	case op == "DefineActor":
		rawValue, _ := s.fn.Literal(s.operand(inst, 0))
		raw, ok := rawValue.(*fas.Vector)
		name := "scriptObject"
		if variable, ok := s.pending.(*ast.Variable); ok {
			name = variable.Name
			s.pending = nil
		} else {
			for len(s.stack) > 0 && name == "scriptObject" {
				nameExpr := s.pop()
				if text, ok := nameExpr.(*ast.StringLiteral); ok {
					name = text.Value
				} else if variable, ok := nameExpr.(*ast.Variable); ok {
					name = variable.Name
				}
			}
		}
		if ok {
			s.push(s.d.actor(raw, name, append([][]string{s.variables}, s.parentScopes...)))
		} else {
			s.push(&ast.ScriptObject{Name: name})
		}
	case op == "SetData":
		s.pending = s.pop()
	case op == "CopyData":
		target, source := s.pop(), s.pop()
		s.emit(&ast.Copy{Source: source, Target: target})
	case op == "StoreResult":
		if len(s.stack) == 0 {
			if s.undefined > 0 {
				s.undefined--
			}
			break
		}
		value := s.pop()
		if len(s.stack) > 0 && s.stack[len(s.stack)-1] == value {
			s.pop()
		}
		if object, ok := value.(*ast.ScriptObject); ok {
			s.pending = nil
			s.emit(&ast.Expression{Value: object})
		} else if s.pending != nil {
			if copied, ok := value.(*ast.CopyExpr); ok {
				s.emit(&ast.Copy{Source: copied.Value, Target: s.pending})
			} else {
				s.emit(&ast.Set{Target: s.pending, Value: value})
			}
			s.pending = nil
		} else {
			s.emit(&ast.Expression{Value: value})
		}
	case op == "Return":
		if s.pending != nil && len(s.stack) > 0 {
			s.emit(&ast.Set{Target: s.pending, Value: s.pop()})
			s.pending = nil
		} else if len(s.stack) > 0 {
			value := s.pop()
			if inst.Offset+len(inst.Raw) < len(s.fn.Code) {
				s.emit(&ast.Return{Value: value, Explicit: true})
			} else {
				s.emit(&ast.Expression{Value: value})
			}
		} else if inst.Offset+len(inst.Raw) < len(s.fn.Code) {
			// A valueless return before the function epilogue is explicit.
			// The final valueless Return is compiler-generated and omitted.
			s.emit(&ast.Return{Explicit: true})
		}
	case op == "Error":
		message := s.pop()
		count := s.popNumber()
		values := s.popMany(count)
		if len(s.stack) > 0 {
			s.pop()
		}
		args := []ast.Argument{ast.DirectArgument{Value: message}}
		args = append(args, typedArguments(values, s.d.opts.Terms, terminology.EventCode{}, "")...)
		s.emit(&ast.Expression{Value: &ast.CommandCall{Name: "error", Arguments: args}})
	case op == "ObjectAliasQuote":
		s.push(&ast.CopyExpr{Value: s.pop()})
	case op == "Of":
		container := s.pop()
		s.blocks = append(s.blocks, &block{kind: blockOf, container: container, stackBase: len(s.stack), endAt: s.operand(inst, 0)})
	case op == "EndOf":
		if b := s.current(blockOf); b != nil {
			value := s.pop()
			for len(s.blocks) > 1 && s.blocks[len(s.blocks)-1] != b {
				s.closeTop()
			}
			if len(s.blocks) > 1 {
				s.blocks = s.blocks[:len(s.blocks)-1]
			}
			if len(s.stack) > b.stackBase {
				s.stack = s.stack[:b.stackBase]
			}
			s.push(&ast.Binary{Op: ast.Of, Left: value, Right: b.container})
		}
	case op == "PushNext":
		// Continue bytecode carries a runtime next-handler value. The source
		// call is reconstructed by the following Continue opcode.
	case op == "Continue" || op == "PositionalContinue":
		s.continueCall(s.operand(inst, 0), op == "PositionalContinue")
	case op == "GetData" || op == "GetResult" || op == "EndDefineActor" || op == "DefineProcedure" || op == "DefineClosure" || op == "DefineProperty" || op == "MatchLiteral":
	default:
		return fmt.Errorf("unsupported opcode %s (0x%02x)", op, byte(inst.Opcode))
	}
	return nil
}

func (s *state) flushExpression() {
	if len(s.stack) == 0 {
		return
	}
	value := s.pop()
	if s.pending != nil {
		s.emit(&ast.Set{Target: s.pending, Value: value})
		s.pending = nil
		return
	}
	s.emit(&ast.Expression{Value: value})
}

func considerationNames(value ast.Expr, terms *terminology.Registry) []string {
	if list, ok := value.(*ast.List); ok {
		var names []string
		for _, element := range list.Elements {
			names = append(names, considerationNames(element, terms)...)
		}
		return names
	}
	keyword, ok := value.(*ast.Keyword)
	if !ok {
		return nil
	}
	if keyword.Fallback != "" {
		return []string{keyword.Fallback}
	}
	codeBytes := keyword.Code
	if len(codeBytes) == 8 {
		codeBytes = codeBytes[4:]
	}
	if len(codeBytes) == 4 && terms != nil {
		var code terminology.Code4
		copy(code[:], codeBytes)
		if name, ok := terms.Term(code); ok {
			return []string{name}
		}
		if name, ok := terms.Enumeration(code); ok {
			return []string{name}
		}
	}
	if len(codeBytes) != 0 {
		return []string{string(codeBytes)}
	}
	return nil
}

func (d *decompiler) actor(raw *fas.Vector, name string, parentScopes [][]string) *ast.ScriptObject {
	initializer, ok := model.FunctionFromVector(2, raw)
	if !ok {
		return &ast.ScriptObject{Name: name}
	}
	if name == "scriptObject" {
		name = identifierValue(initializer.Name, name)
	}
	object := &ast.ScriptObject{Name: name}
	handler, err := d.function(initializer, parentScopes)
	if err != nil {
		d.diagnostics = append(d.diagnostics, Diagnostic{Function: initializer.Offset, Offset: -1, Message: err.Error()})
	} else if handler != nil {
		for _, statement := range handler.Body {
			if set, ok := statement.(*ast.Set); ok {
				if variable, ok := set.Target.(*ast.Variable); ok {
					propertyName := variable.Name
					var index int
					if _, err := fmt.Sscanf(propertyName, "var_%d", &index); err == nil && len(parentScopes) > 0 && index < len(parentScopes[0]) {
						propertyName = parentScopes[0][index]
					}
					object.Properties = append(object.Properties, ast.Property{Name: propertyName, Value: set.Value})
				}
			}
		}
	}
	actorScope := make([]string, len(initializer.Variables))
	for i, value := range initializer.Variables {
		actorScope[i] = identifierValue(value, fmt.Sprintf("var_%d", i))
	}
	scopes := append([][]string{actorScope}, parentScopes...)
	offset := 3
	for _, value := range initializer.Literals {
		procedureRaw, ok := value.(*fas.Vector)
		if !ok {
			continue
		}
		procedure, ok := model.FunctionFromVector(offset, procedureRaw)
		if !ok {
			continue
		}
		child, childErr := d.function(procedure, scopes)
		if childErr != nil {
			d.diagnostics = append(d.diagnostics, Diagnostic{Function: offset, Offset: -1, Message: childErr.Error()})
		} else if child != nil {
			object.Handlers = append(object.Handlers, child)
		}
		offset++
	}
	return object
}

func (s *state) current(kind blockKind) *block {
	for i := len(s.blocks) - 1; i > 0; i-- {
		if s.blocks[i].kind == kind {
			return s.blocks[i]
		}
	}
	return nil
}
func (s *state) closeThrough(target *block) {
	for len(s.blocks) > 1 {
		current := s.blocks[len(s.blocks)-1]
		s.closeTop()
		if current == target {
			return
		}
	}
}
func (s *state) popMany(count int) []ast.Expr {
	if count < 0 {
		count = 0
	}
	if count > len(s.stack) {
		count = len(s.stack)
	}
	start := len(s.stack) - count
	out := append([]ast.Expr(nil), s.stack[start:]...)
	s.stack = s.stack[:start]
	return out
}
func (s *state) parentVariable(depth, index int) string {
	start := max(depth-1, 0)
	for _, scope := range s.parentScopes[start:] {
		if index >= 0 && index < len(scope) {
			return scope[index]
		}
	}
	return fmt.Sprintf("parent_var_%d", index)
}
func (s *state) literalName(index int) string {
	value, ok := s.fn.Literal(index)
	if !ok {
		return ""
	}
	if b, ok := value.(*fas.Bytes); ok {
		return string(b.Data)
	}
	if binding, ok := value.(*fas.Binding); ok {
		if b, ok := binding.Value.(*fas.Bytes); ok {
			return string(b.Data)
		}
	}
	return ""
}

func (s *state) global(index int) ast.Expr {
	value, ok := s.fn.Literal(index)
	if !ok {
		return &ast.Variable{Name: fmt.Sprintf("global_%d", index)}
	}
	if raw, ok := value.(*fas.Bytes); ok {
		return &ast.Variable{Name: string(raw.Data)}
	}
	return literal(value)
}

func (s *state) makeComparison(index byte) {
	name := bytecode.Opcode(index).Name()
	if kind := binaryOps[name]; kind != "" {
		right, left := s.pop(), s.pop()
		s.push(&ast.Binary{Op: kind, Left: left, Right: right})
		return
	}
	if name == "Not" {
		s.push(&ast.Unary{Op: ast.UnaryNot, Value: s.pop()})
		return
	}
	if name == "And" || name == "Or" {
		right, left := s.pop(), s.pop()
		kind := ast.And
		if name == "Or" {
			kind = ast.Or
		}
		s.push(&ast.Binary{Op: kind, Left: left, Right: right})
	}
}

func (s *state) makeSpecifier(sub byte) {
	switch sub {
	case 21:
		object, container := s.pop(), s.pop()
		if text, ok := object.(*ast.StringLiteral); ok && validIdentifier(text.Value) {
			object = &ast.Variable{Name: text.Value}
		}
		s.push(&ast.Specifier{Kind: ast.PropertySpecifier, Object: object, Container: container})
	case 22:
		object, container := s.pop(), s.pop()
		if keyword, ok := object.(*ast.Keyword); ok && string(keyword.Code) == "prop" {
			// AppleScript's compiler serializes the special plural `properties`
			// as every `prop`; osadecompile canonicalizes that representation back
			// to `properties of ...`.
			object = &ast.Keyword{Code: keyword.Code, Fallback: "properties"}
			s.push(&ast.Specifier{Kind: ast.PropertySpecifier, Object: object, Container: container})
			return
		}
		s.push(&ast.Specifier{Kind: ast.EverySpecifier, Object: object, Container: container})
	case 23:
		object, container := s.pop(), s.pop()
		s.push(&ast.Specifier{Kind: ast.SomeSpecifier, Object: object, Container: container})
	case 24:
		index, object, container := s.pop(), s.pop(), s.pop()
		s.push(&ast.Specifier{Kind: ast.IndexSpecifier, Object: object, From: index, Container: container})
	case 25:
		keyForm, key := s.pop(), s.pop()
		object, container := s.pop(), s.pop()
		keyName := "id"
		if keyword, ok := keyForm.(*ast.Keyword); ok && len(keyword.Code) == 4 {
			switch string(keyword.Code) {
			case "ID  ":
				keyName = "id"
			case "indx":
				keyName = "index"
			case "name":
				keyName = "name"
			}
		}
		s.push(&ast.Specifier{Kind: ast.KeySpecifier, Object: object, From: key, Container: container, KeyName: keyName})
	case 26:
		predicate, target := s.pop(), s.pop()
		s.push(&ast.Whose{Object: target, Predicate: predicate})
	case 27:
		to, from, object := s.pop(), s.pop(), s.pop()
		if len(s.stack) >= 3 {
			s.pop()
		}
		container := s.pop()
		if len(s.stack) > 0 && s.stack[len(s.stack)-1] == container {
			s.pop()
		}
		s.push(&ast.Specifier{Kind: ast.RangeSpecifier, Object: object, Container: container, From: from, To: to})
	case 30:
		s.push(&ast.Specifier{Kind: ast.BeginningSpecifier, Container: s.pop()})
	case 31:
		s.push(&ast.Specifier{Kind: ast.EndSpecifier, Container: s.pop()})
	case 32:
		object, container := s.pop(), s.pop()
		s.push(&ast.Specifier{Kind: ast.MiddleSpecifier, Object: object, Container: container})
	default:
		s.push(&ast.MissingLiteral{})
	}
}

func (s *state) message(index int, positional bool) {
	count := s.popNumber()
	values := s.popMany(count)
	target := s.pop()
	nameExpr := s.literal(index)
	if positional {
		name := "handler"
		if text, ok := nameExpr.(*ast.StringLiteral); ok {
			name = text.Value
		}
		if _, ok := target.(*ast.It); ok {
			target = nil
		}
		s.push(&ast.HandlerCall{Name: name, Target: target, Arguments: values})
		return
	}
	if _, ok := target.(*ast.It); !ok {
		values = append([]ast.Expr{target}, values...)
	}
	var code terminology.EventCode
	foundCode := false
	rawName := ""
	var rawEventValue fas.Value
	if value, ok := s.fn.Literal(index); ok {
		rawEventValue = value
		foundCode = eventCodeFromValue(value, &code)
	}
	if !foundCode {
		switch value := nameExpr.(type) {
		case *ast.StringLiteral:
			if len([]byte(value.Value)) == 8 {
				copy(code[:], []byte(value.Value))
				foundCode = true
			}
		case *ast.Keyword:
			if len(value.Code) == 8 {
				copy(code[:], value.Code)
				foundCode = true
			}
		case *ast.RawDataLiteral:
			if len(value.Data) >= 8 {
				copy(code[:], value.Data[:8])
				foundCode = true
			}
		}
	}
	if !foundCode {
		if bytesValue, ok := rawEventValue.(*fas.Bytes); ok && len(bytesValue.Data) > 0 {
			rawName = string(bytesValue.Data)
		}
		if rawName == "" {
			s.d.diagnostics = append(s.d.diagnostics, Diagnostic{Function: s.fn.Offset, Offset: -1, Message: fmt.Sprintf("cannot recover event code from %T (literal %T)", rawEventValue, nameExpr)})
		}
	}
	name := rawName
	if s.d.opts.Terms != nil {
		if command, ok := s.d.opts.Terms.Command(code); ok {
			name = command.Name
		}
	}
	if rawName == "path" && hasKeyword(values, "to  ") {
		name = "path to"
	}
	s.push(&ast.CommandCall{Code: code, Name: name, Arguments: typedArguments(values, s.d.opts.Terms, code, rawName)})
}

func hasKeyword(values []ast.Expr, code string) bool {
	for _, value := range values {
		if keyword, ok := value.(*ast.Keyword); ok && string(keyword.Code) == code {
			return true
		}
	}
	return false
}

func (s *state) continueCall(index int, positional bool) {
	if !positional {
		count := len(s.handler.Parameters)
		if count == 0 {
			count = len(recoverArgs(s.fn.Arguments))
		}
		values := s.popMany(count)
		if len(s.stack) > 0 {
			if _, ok := s.stack[len(s.stack)-1].(*ast.NumberLiteral); ok {
				s.pop() // compiler next-handler slot
			}
		}
		if len(s.stack) > 0 {
			s.pop() // current receiver (normally me)
		}
		s.push(&ast.It{})
		for _, value := range values {
			s.push(value)
		}
		s.push(&ast.NumberLiteral{Integer: int64(len(values))})
	}
	if len(s.stack) > 0 {
		if _, ok := s.stack[len(s.stack)-1].(*ast.It); ok {
			s.pop()
		}
	}
	s.message(index, positional)
	call := s.pop()
	if handler, ok := call.(*ast.HandlerCall); ok {
		if _, isMe := handler.Target.(*ast.Me); isMe {
			handler.Target = nil
		}
	}
	s.emit(&ast.Continue{Call: call})
}

func eventCodeFromValue(value fas.Value, code *terminology.EventCode) bool {
	switch value := value.(type) {
	case *fas.Object:
		if event, ok := value.Value.(*fas.EventIdentifier); ok {
			copy(code[:4], event.Fields[0][:])
			copy(code[4:], event.Fields[1][:])
			return true
		}
		return eventCodeFromValue(value.Value, code)
	case *fas.Bytes:
		if len(value.Data) == 8 {
			copy(code[:], value.Data)
			return true
		}
	case *fas.Vector:
		start := 0
		if value.HasType {
			start = 1
		}
		for _, child := range value.Children[start:] {
			if eventCodeFromValue(child, code) {
				return true
			}
		}
	}
	return false
}

func typedArguments(values []ast.Expr, terms *terminology.Registry, event terminology.EventCode, rawName string) []ast.Argument {
	var output []ast.Argument
	var command terminology.Command
	if terms != nil {
		command, _ = terms.Command(event)
	}
	for index := 0; index < len(values); index++ {
		keyword, ok := values[index].(*ast.Keyword)
		if ok && len(keyword.Code) == 4 && index+1 < len(values) {
			var code terminology.Code4
			copy(code[:], keyword.Code)
			// The compiler uses the internal keyword "to  " to delimit the
			// direct argument of commands such as Standard Additions' “path to”.
			if string(code[:]) == "to  " && (command.HasDirectParameter || rawName == "path") {
				output = append(output, ast.DirectArgument{Value: values[index+1]})
				index++
				continue
			}
			name := ""
			parameter, known := command.Parameters[code]
			if known {
				name = parameter.Name
			}
			if boolean, ok := values[index+1].(*ast.BooleanLiteral); ok && known && parameter.Type == "boolean" {
				output = append(output, ast.FlagArgument{Code: code, Name: name, Enabled: boolean.Value})
				index++
				continue
			}
			output = append(output, ast.NamedArgument{Code: code, Name: name, Value: values[index+1]})
			index++
			continue
		}
		output = append(output, ast.DirectArgument{Value: values[index]})
	}
	return output
}
