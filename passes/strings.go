package passes

import (
	"maps"
	"strings"

	"applescript-tools/ast"
)

// Strings recovers deliberately fragmented text while refusing to
// reassociate AppleScript's polymorphic & operator through an untyped value.
type Strings struct{}

func (Strings) Name() string { return "strings" }

func (Strings) Rewrite(script *ast.Script) (*ast.Script, []Diagnostic) {
	rewriteStringsScope(script.Properties, script.Handlers, true)
	for _, object := range script.Objects {
		rewriteStringsScope(object.Properties, object.Handlers, true)
	}
	return script, nil
}

type stringFact struct {
	Value    string
	Constant bool
}

type stringEnvironment map[string]stringFact

func (environment stringEnvironment) clone() stringEnvironment {
	clone := make(stringEnvironment, len(environment))
	maps.Copy(clone, environment)
	return clone
}

func rewriteStringsScope(
	properties []ast.Property,
	handlers []*ast.Handler,
	trackLocals bool,
) {
	for i := range properties {
		properties[i].Value = rewriteStringsExpression(
			properties[i].Value,
			stringEnvironment{},
		)
	}
	for _, handler := range handlers {
		rewriteStringsBlock(
			handler.Body,
			stringEnvironment{},
			blockedVariables(properties, handler),
			trackLocals && !containsLocalEscape(handler.Body),
		)
	}
}

func blockedVariables(properties []ast.Property, handler *ast.Handler) map[string]bool {
	blocked := make(map[string]bool, len(properties))
	for _, property := range properties {
		blocked[variableName(property.Name)] = true
	}
	for _, parameter := range handler.Parameters {
		delete(blocked, variableName(parameter.Name))
	}
	locals, globals := declaredVariables(handler.Body)
	for name := range locals {
		delete(blocked, name)
	}
	for name := range globals {
		blocked[name] = true
	}
	return blocked
}

func rewriteStringsBlock(
	statements []ast.Stmt,
	environment stringEnvironment,
	blocked map[string]bool,
	trackLocals bool,
) {
	rewrite := func(expression ast.Expr) ast.Expr {
		return rewriteStringsExpression(expression, environment)
	}

	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.Set:
			node.Target = rewrite(node.Target)
			node.Value = rewrite(node.Value)
			updateEnvironment(
				environment,
				blocked,
				trackLocals,
				node.Target,
				node.Value,
			)
		case *ast.Copy:
			node.Source = rewrite(node.Source)
			node.Target = rewrite(node.Target)
			updateEnvironment(
				environment,
				blocked,
				trackLocals,
				node.Target,
				node.Source,
			)
		case *ast.Expression:
			node.Value = rewrite(node.Value)
		case *ast.Declaration:
			for _, name := range node.Names {
				delete(environment, variableName(name))
			}
		case *ast.If:
			node.Condition = rewrite(node.Condition)
			thenEnvironment := environment.clone()
			elseEnvironment := environment.clone()
			rewriteStringsBlock(node.Then, thenEnvironment, blocked, trackLocals)
			rewriteStringsBlock(node.Else, elseEnvironment, blocked, trackLocals)
			mergeEnvironments(environment, thenEnvironment, elseEnvironment)
		case *ast.Repeat:
			empty := stringEnvironment{}
			node.Times = rewriteStringsExpression(node.Times, empty)
			node.Condition = rewriteStringsExpression(node.Condition, empty)
			node.Collection = rewriteStringsExpression(node.Collection, empty)
			node.From = rewriteStringsExpression(node.From, empty)
			node.To = rewriteStringsExpression(node.To, empty)
			node.By = rewriteStringsExpression(node.By, empty)
			rewriteStringsBlock(node.Body, stringEnvironment{}, blocked, trackLocals)
			clear(environment)
		case *ast.Try:
			rewriteStringsBlock(node.Body, environment.clone(), blocked, trackLocals)
			rewriteStringsBlock(node.ErrorBody, stringEnvironment{}, blocked, trackLocals)
			clear(environment)
		case *ast.Tell:
			node.Target = rewrite(node.Target)
			rewriteStringsBlock(node.Body, environment, blocked, trackLocals)
		case *ast.Considering:
			rewriteStringsBlock(node.Body, environment, blocked, trackLocals)
		case *ast.Timeout:
			node.Seconds = rewrite(node.Seconds)
			rewriteStringsBlock(node.Body, environment, blocked, trackLocals)
		case *ast.Return:
			node.Value = rewrite(node.Value)
		case *ast.ExitRepeat, *ast.Comment:
			// These statements contain no expressions and cannot change facts.
		default:
			// Keep newly added statement kinds safe until their control-flow
			// and mutation behavior is handled explicitly.
			clear(environment)
		}
	}
}

func rewriteStringsExpression(
	expression ast.Expr,
	environment stringEnvironment,
) ast.Expr {
	if expression == nil {
		return nil
	}
	rewrite := func(child ast.Expr) ast.Expr {
		return rewriteStringsExpression(child, environment)
	}

	switch node := expression.(type) {
	case *ast.List:
		for i := range node.Elements {
			node.Elements[i] = rewrite(node.Elements[i])
		}
	case *ast.Record:
		for i := range node.Fields {
			node.Fields[i].Label = rewrite(node.Fields[i].Label)
			node.Fields[i].Value = rewrite(node.Fields[i].Value)
		}
	case *ast.Unary:
		node.Value = rewrite(node.Value)
	case *ast.Binary:
		node.Left = rewrite(node.Left)
		node.Right = rewrite(node.Right)
	case *ast.Coerce:
		node.Value = rewrite(node.Value)
		node.Type = rewrite(node.Type)
	case *ast.CopyExpr:
		node.Value = rewrite(node.Value)
	case *ast.CommandCall:
		node.Target = rewrite(node.Target)
		for i, argument := range node.Arguments {
			switch value := argument.(type) {
			case ast.DirectArgument:
				value.Value = rewrite(value.Value)
				node.Arguments[i] = value
			case ast.NamedArgument:
				value.Value = rewrite(value.Value)
				node.Arguments[i] = value
			}
		}
	case *ast.HandlerCall:
		node.Target = rewrite(node.Target)
		for i := range node.Arguments {
			node.Arguments[i] = rewrite(node.Arguments[i])
		}
	case *ast.Specifier:
		node.Object = rewrite(node.Object)
		node.Container = rewrite(node.Container)
		node.From = rewrite(node.From)
		node.To = rewrite(node.To)
	case *ast.Whose:
		node.Object = rewrite(node.Object)
		node.Predicate = rewrite(node.Predicate)
	case *ast.ScriptObject:
		// A nested script may capture bindings from its containing handler.
		// Rewrite context-free expressions, but do not propagate its locals.
		rewriteStringsScope(node.Properties, node.Handlers, false)
	}

	expression = stringExpression(expression)
	if concatenation, ok := expression.(*ast.Binary); ok &&
		concatenation.Op == ast.Concatenate {
		expression = foldConcatenation(concatenation, environment)
	}
	return expression
}

func foldConcatenation(
	expression *ast.Binary,
	environment stringEnvironment,
) ast.Expr {
	var operands []ast.Expr
	flattenConcatenation(expression, &operands)

	compacted := make([]ast.Expr, 0, len(operands))
	for _, operand := range operands {
		fact, isString := inferString(operand, environment)
		if !isString {
			return expression
		}
		if fact.Constant {
			operand = &ast.StringLiteral{
				Base:  ast.Base{Origin: operand.GetOrigin()},
				Value: fact.Value,
			}
		}
		if literal, ok := operand.(*ast.StringLiteral); ok && len(compacted) > 0 {
			if previous, ok := compacted[len(compacted)-1].(*ast.StringLiteral); ok {
				compacted[len(compacted)-1] = &ast.StringLiteral{
					Base:  previous.Base,
					Value: previous.Value + literal.Value,
				}
				continue
			}
		}
		compacted = append(compacted, operand)
	}

	result := compacted[0]
	for _, operand := range compacted[1:] {
		result = &ast.Binary{
			Base:  expression.Base,
			Op:    ast.Concatenate,
			Left:  result,
			Right: operand,
		}
	}
	return result
}

func inferString(
	expression ast.Expr,
	environment stringEnvironment,
) (stringFact, bool) {
	switch node := expression.(type) {
	case *ast.StringLiteral:
		return stringFact{Value: node.Value, Constant: true}, true
	case *ast.Variable:
		fact, ok := environment[variableName(node.Name)]
		return fact, ok
	case *ast.Binary:
		if node.Op == ast.Of && namedKeyword(node.Left, "quoted form", "POSIX path") {
			return stringFact{}, true
		}
		if node.Op != ast.Concatenate {
			return stringFact{}, false
		}
		left, leftString := inferString(node.Left, environment)
		right, rightString := inferString(node.Right, environment)
		if !leftString || !rightString {
			return stringFact{}, false
		}
		if left.Constant && right.Constant {
			return stringFact{Value: left.Value + right.Value, Constant: true}, true
		}
		return stringFact{}, true
	case *ast.Coerce:
		if namedKeyword(node.Type, "string", "text", "Unicode text") {
			return stringFact{}, true
		}
	case *ast.CommandCall:
		if standardAdditionsCall(node, "sysoexec") {
			return stringFact{}, true
		}
	case *ast.Specifier:
		if namedKeyword(node.Object, "quoted form", "POSIX path") {
			return stringFact{}, true
		}
	case *ast.Keyword:
		if namedKeyword(node, "space", "tab", "linefeed", "return") {
			return stringFact{}, true
		}
	}
	return stringFact{}, false
}

func flattenConcatenation(expression ast.Expr, operands *[]ast.Expr) {
	if binary, ok := expression.(*ast.Binary); ok && binary.Op == ast.Concatenate {
		flattenConcatenation(binary.Left, operands)
		flattenConcatenation(binary.Right, operands)
		return
	}
	*operands = append(*operands, expression)
}

func updateEnvironment(
	environment stringEnvironment,
	blocked map[string]bool,
	trackLocals bool,
	target, value ast.Expr,
) {
	name, ok := assignedVariable(target)
	if !ok {
		return
	}
	key := variableName(name)
	if !trackLocals || blocked[key] {
		delete(environment, key)
		return
	}
	if _, direct := target.(*ast.Variable); !direct {
		delete(environment, key)
		return
	}
	if literal, ok := value.(*ast.StringLiteral); ok {
		environment[key] = stringFact{Value: literal.Value, Constant: true}
	} else if fact, isString := inferString(value, environment); isString {
		// Dynamic producers establish only a type fact; known constants retain
		// their value. Both can be shared safely because AppleScript text is
		// immutable.
		environment[key] = fact
	} else {
		delete(environment, key)
	}
}

func containsLocalEscape(statements []ast.Stmt) bool {
	found := false
	walkStatements(statements, func(expression ast.Expr) ast.Expr {
		switch expression.(type) {
		case *ast.CopyExpr, *ast.ScriptObject:
			found = true
		}
		return expression
	})
	return found
}

func mergeEnvironments(destination, left, right stringEnvironment) {
	clear(destination)
	for name, value := range left {
		if other, ok := right[name]; ok && other == value {
			destination[name] = value
		}
	}
}

func assignedVariable(expression ast.Expr) (string, bool) {
	switch node := expression.(type) {
	case *ast.Variable:
		return node.Name, true
	case *ast.Specifier:
		if name, ok := assignedVariable(node.Container); ok {
			return name, true
		}
		return assignedVariable(node.Object)
	}
	return "", false
}

func declaredVariables(statements []ast.Stmt) (map[string]bool, map[string]bool) {
	locals := map[string]bool{}
	globals := map[string]bool{}
	var visit func([]ast.Stmt)
	visit = func(statements []ast.Stmt) {
		for _, statement := range statements {
			switch node := statement.(type) {
			case *ast.Declaration:
				destination := locals
				if node.Global {
					destination = globals
				}
				for _, name := range node.Names {
					destination[variableName(name)] = true
				}
			case *ast.If:
				visit(node.Then)
				visit(node.Else)
			case *ast.Repeat:
				visit(node.Body)
			case *ast.Try:
				visit(node.Body)
				visit(node.ErrorBody)
			case *ast.Tell:
				visit(node.Body)
			case *ast.Considering:
				visit(node.Body)
			case *ast.Timeout:
				visit(node.Body)
			}
		}
	}
	visit(statements)
	return locals, globals
}

func variableName(name string) string {
	return strings.ToLower(name)
}

var stringKeywordNames = map[string]string{
	"strq": "quoted form",
	"psxp": "POSIX path",
	"spac": "space",
	"tab ": "tab",
	"lnfd": "linefeed",
	"ret ": "return",
	"TEXT": "string",
	"ctxt": "text",
	"utxt": "Unicode text",
}

func namedKeyword(expression ast.Expr, names ...string) bool {
	keyword, ok := expression.(*ast.Keyword)
	if !ok {
		return false
	}
	code := string(keyword.Code)
	if len(code) >= 4 {
		code = code[len(code)-4:]
	}
	if keywordName, ok := stringKeywordNames[code]; ok {
		for _, name := range names {
			if strings.EqualFold(keywordName, name) {
				return true
			}
		}
		return false
	}
	if len(keyword.Code) != 0 {
		return false
	}
	for _, name := range names {
		if strings.EqualFold(keyword.Fallback, name) {
			return true
		}
	}
	return false
}

func standardAdditionsCall(node *ast.CommandCall, code string) bool {
	if string(node.Code[:]) != code {
		return false
	}
	switch node.Target.(type) {
	case nil, *ast.Me:
		return true
	default:
		return false
	}
}

// stringExpression contains the context-free rewrites and remains independently
// testable. Lists are deliberately not collapsed because that changes shape.
func stringExpression(expression ast.Expr) ast.Expr {
	switch node := expression.(type) {
	case *ast.Binary:
		if node.Op == ast.Concatenate {
			left, leftString := node.Left.(*ast.StringLiteral)
			right, rightString := node.Right.(*ast.StringLiteral)
			if leftString && rightString {
				return &ast.StringLiteral{
					Base:  node.Base,
					Value: left.Value + right.Value,
				}
			}
		}
	case *ast.CommandCall:
		if standardAdditionsCall(node, "sysontoc") && len(node.Arguments) == 1 {
			direct, ok := node.Arguments[0].(ast.DirectArgument)
			if !ok {
				return expression
			}
			number, ok := direct.Value.(*ast.NumberLiteral)
			if !ok || number.IsReal || number.Integer < 0 || number.Integer > 127 {
				return expression
			}
			return &ast.StringLiteral{
				Base:  node.Base,
				Value: string(rune(number.Integer)),
			}
		}
	}
	return expression
}
