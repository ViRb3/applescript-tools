package passes

import (
	"fmt"

	"applescript-tools/ast"
)

type Diagnostic struct {
	Message string
	Origin  ast.Origin
}

type Pass interface {
	Name() string
	Rewrite(*ast.Script) (*ast.Script, []Diagnostic)
}

func Named(name string) (Pass, error) {
	switch name {
	case "strings":
		return Strings{}, nil
	default:
		return nil, fmt.Errorf("unknown pass %q", name)
	}
}

func rewriteScript(script *ast.Script, rewrite func(ast.Expr) ast.Expr) {
	for i := range script.Properties {
		script.Properties[i].Value = walkExpr(script.Properties[i].Value, rewrite)
	}
	for _, handler := range script.Handlers {
		handler.Body = walkStatements(handler.Body, rewrite)
	}
	for _, object := range script.Objects {
		for i := range object.Properties {
			object.Properties[i].Value = walkExpr(object.Properties[i].Value, rewrite)
		}
		for _, handler := range object.Handlers {
			handler.Body = walkStatements(handler.Body, rewrite)
		}
	}
}

func walkStatements(statements []ast.Stmt, rewrite func(ast.Expr) ast.Expr) []ast.Stmt {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.Set:
			node.Target = walkExpr(node.Target, rewrite)
			node.Value = walkExpr(node.Value, rewrite)
		case *ast.Copy:
			node.Source = walkExpr(node.Source, rewrite)
			node.Target = walkExpr(node.Target, rewrite)
		case *ast.Expression:
			node.Value = walkExpr(node.Value, rewrite)
		case *ast.If:
			node.Condition = walkExpr(node.Condition, rewrite)
			node.Then = walkStatements(node.Then, rewrite)
			node.Else = walkStatements(node.Else, rewrite)
		case *ast.Repeat:
			node.Times = walkExpr(node.Times, rewrite)
			node.Condition = walkExpr(node.Condition, rewrite)
			node.Collection = walkExpr(node.Collection, rewrite)
			node.From = walkExpr(node.From, rewrite)
			node.To = walkExpr(node.To, rewrite)
			node.By = walkExpr(node.By, rewrite)
			node.Body = walkStatements(node.Body, rewrite)
		case *ast.Try:
			node.Body = walkStatements(node.Body, rewrite)
			node.ErrorBody = walkStatements(node.ErrorBody, rewrite)
		case *ast.Tell:
			node.Target = walkExpr(node.Target, rewrite)
			node.Body = walkStatements(node.Body, rewrite)
		case *ast.Considering:
			node.Body = walkStatements(node.Body, rewrite)
		case *ast.Timeout:
			node.Seconds = walkExpr(node.Seconds, rewrite)
			node.Body = walkStatements(node.Body, rewrite)
		case *ast.Return:
			node.Value = walkExpr(node.Value, rewrite)
		}
	}
	return statements
}

func walkExpr(expression ast.Expr, rewrite func(ast.Expr) ast.Expr) ast.Expr {
	if expression == nil {
		return nil
	}
	switch node := expression.(type) {
	case *ast.List:
		for i := range node.Elements {
			node.Elements[i] = walkExpr(node.Elements[i], rewrite)
		}
	case *ast.Record:
		for i := range node.Fields {
			node.Fields[i].Label = walkExpr(node.Fields[i].Label, rewrite)
			node.Fields[i].Value = walkExpr(node.Fields[i].Value, rewrite)
		}
	case *ast.Unary:
		node.Value = walkExpr(node.Value, rewrite)
	case *ast.Binary:
		node.Left = walkExpr(node.Left, rewrite)
		node.Right = walkExpr(node.Right, rewrite)
	case *ast.Coerce:
		node.Value = walkExpr(node.Value, rewrite)
		node.Type = walkExpr(node.Type, rewrite)
	case *ast.CopyExpr:
		node.Value = walkExpr(node.Value, rewrite)
	case *ast.CommandCall:
		node.Target = walkExpr(node.Target, rewrite)
		for i, argument := range node.Arguments {
			switch value := argument.(type) {
			case ast.DirectArgument:
				value.Value = walkExpr(value.Value, rewrite)
				node.Arguments[i] = value
			case ast.NamedArgument:
				value.Value = walkExpr(value.Value, rewrite)
				node.Arguments[i] = value
			}
		}
	case *ast.HandlerCall:
		node.Target = walkExpr(node.Target, rewrite)
		for i := range node.Arguments {
			node.Arguments[i] = walkExpr(node.Arguments[i], rewrite)
		}
	case *ast.Specifier:
		node.Object = walkExpr(node.Object, rewrite)
		node.Container = walkExpr(node.Container, rewrite)
		node.From = walkExpr(node.From, rewrite)
		node.To = walkExpr(node.To, rewrite)
	case *ast.Whose:
		node.Object = walkExpr(node.Object, rewrite)
		node.Predicate = walkExpr(node.Predicate, rewrite)
	case *ast.ScriptObject:
		for i := range node.Properties {
			node.Properties[i].Value = walkExpr(node.Properties[i].Value, rewrite)
		}
		for _, handler := range node.Handlers {
			handler.Body = walkStatements(handler.Body, rewrite)
		}
	}
	return rewrite(expression)
}
