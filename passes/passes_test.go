package passes

import (
	"applescript-tools/ast"
	"testing"
)

func TestStringsPreservesListShapeWhileRewritingElements(t *testing.T) {
	list := &ast.List{Elements: []ast.Expr{
		concatenate(
			&ast.StringLiteral{Value: "a"},
			&ast.StringLiteral{Value: "b"},
		),
		standardCommand(
			"ASCII character",
			"sysontoc",
			ast.DirectArgument{Value: &ast.NumberLiteral{Integer: 65}},
		),
	}}
	script := &ast.Script{Properties: []ast.Property{{Name: "value", Value: list}}}
	Strings{}.Rewrite(script)
	got := script.Properties[0].Value.(*ast.List)
	if len(got.Elements) != 2 {
		t.Fatalf("got %#v", got)
	}
	for index, want := range []string{"ab", "A"} {
		literal, ok := got.Elements[index].(*ast.StringLiteral)
		if !ok || literal.Value != want {
			t.Fatalf("element %d = %#v, want %q", index, got.Elements[index], want)
		}
	}
}

func TestStringsFoldsLiteralAssignmentsAndDirectText(t *testing.T) {
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "_t"},
			Value:  &ast.StringLiteral{Value: "example"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "lib_name"},
			Value: concatenate(
				&ast.StringLiteral{Value: "lib"},
				&ast.Variable{Name: "_t"},
				&ast.StringLiteral{Value: "."},
				&ast.StringLiteral{Value: "d"},
				&ast.StringLiteral{Value: "y"},
				&ast.StringLiteral{Value: "l"},
				&ast.StringLiteral{Value: "i"},
				&ast.StringLiteral{Value: "b"},
			),
		},
		&ast.Set{
			Target: &ast.Variable{Name: "base"},
			Value:  standardCommand("do shell script", "sysoexec"),
		},
		&ast.Set{
			Target: &ast.Variable{Name: "path"},
			Value: concatenate(
				&ast.Variable{Name: "base"},
				&ast.StringLiteral{Value: "/"},
				&ast.StringLiteral{Value: "A"},
				&ast.StringLiteral{Value: "p"},
				&ast.StringLiteral{Value: "p"},
			),
		},
		&ast.Set{
			Target: &ast.Variable{Name: "nested"},
			Value: concatenate(
				&ast.Variable{Name: "path"},
				&ast.StringLiteral{Value: "/"},
				&ast.StringLiteral{Value: "x"},
			),
		},
	}}}}

	Strings{}.Rewrite(script)

	first := script.Handlers[0].Body[1].(*ast.Set).Value
	if literal, ok := first.(*ast.StringLiteral); !ok || literal.Value != "libexample.dylib" {
		t.Fatalf("constant propagation = %#v", first)
	}
	second := script.Handlers[0].Body[3].(*ast.Set).Value
	binary, ok := second.(*ast.Binary)
	if !ok {
		t.Fatalf("dynamic prefix = %#v", second)
	}
	if variable, ok := binary.Left.(*ast.Variable); !ok || variable.Name != "base" {
		t.Fatalf("dynamic prefix left = %#v", binary.Left)
	}
	if literal, ok := binary.Right.(*ast.StringLiteral); !ok || literal.Value != "/App" {
		t.Fatalf("dynamic prefix suffix = %#v", binary.Right)
	}
	nested := script.Handlers[0].Body[4].(*ast.Set).Value
	binary, ok = nested.(*ast.Binary)
	if !ok {
		t.Fatalf("transitive dynamic prefix = %#v", nested)
	}
	if variable, ok := binary.Left.(*ast.Variable); !ok || variable.Name != "path" {
		t.Fatalf("transitive dynamic prefix left = %#v", binary.Left)
	}
	if literal, ok := binary.Right.(*ast.StringLiteral); !ok || literal.Value != "/x" {
		t.Fatalf("transitive dynamic prefix suffix = %#v", binary.Right)
	}
}

func TestStringsPropagatesImmutableTextAssignments(t *testing.T) {
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "first"},
			Value:  &ast.StringLiteral{Value: "value"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "second"},
			Value:  &ast.Variable{Name: "first"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "result"},
			Value: concatenate(
				&ast.Variable{Name: "second"},
				&ast.StringLiteral{Value: "-suffix"},
			),
		},
		&ast.Set{
			Target: &ast.Variable{Name: "first_result"},
			Value: concatenate(
				&ast.Variable{Name: "first"},
				&ast.StringLiteral{Value: "-suffix"},
			),
		},
	}}}}

	Strings{}.Rewrite(script)

	value := script.Handlers[0].Body[2].(*ast.Set).Value
	if literal, ok := value.(*ast.StringLiteral); !ok || literal.Value != "value-suffix" {
		t.Fatalf("assigned text = %#v", value)
	}
	first := script.Handlers[0].Body[3].(*ast.Set).Value
	if literal, ok := first.(*ast.StringLiteral); !ok || literal.Value != "value-suffix" {
		t.Fatalf("source text = %#v", first)
	}
}

func TestStringsFoldsASCIICharacterChain(t *testing.T) {
	var expressions []ast.Expr
	for _, character := range "open https://example.invalid/" {
		expressions = append(expressions, standardCommand(
			"ASCII character",
			"sysontoc",
			ast.DirectArgument{Value: &ast.NumberLiteral{Integer: int64(character)}},
		))
	}
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "cmd"},
			Value:  concatenate(expressions...),
		},
	}}}}

	Strings{}.Rewrite(script)

	value := script.Handlers[0].Body[0].(*ast.Set).Value
	literal, ok := value.(*ast.StringLiteral)
	if !ok || literal.Value != "open https://example.invalid/" {
		t.Fatalf("ASCII chain = %#v", value)
	}
}

func TestStringsMergesOnlyAgreedBranchValues(t *testing.T) {
	makeScript := func(left, right string) *ast.Script {
		return &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
			&ast.If{
				Condition: &ast.BooleanLiteral{Value: true},
				Then: []ast.Stmt{&ast.Set{
					Target: &ast.Variable{Name: "part"},
					Value:  &ast.StringLiteral{Value: left},
				}},
				Else: []ast.Stmt{&ast.Set{
					Target: &ast.Variable{Name: "part"},
					Value:  &ast.StringLiteral{Value: right},
				}},
			},
			&ast.Set{
				Target: &ast.Variable{Name: "result"},
				Value: concatenate(
					&ast.StringLiteral{Value: "a"},
					&ast.Variable{Name: "part"},
					&ast.StringLiteral{Value: "b"},
				),
			},
		}}}}
	}

	agreed := makeScript("x", "x")
	Strings{}.Rewrite(agreed)
	if got := agreed.Handlers[0].Body[1].(*ast.Set).Value.(*ast.StringLiteral).Value; got != "axb" {
		t.Fatalf("agreed branches = %q", got)
	}

	disagreed := makeScript("x", "y")
	Strings{}.Rewrite(disagreed)
	if _, ok := disagreed.Handlers[0].Body[1].(*ast.Set).Value.(*ast.Binary); !ok {
		t.Fatalf("disagreed branches were propagated: %#v", disagreed.Handlers[0].Body[1])
	}
}

func TestStringsDoesNotPropagateLoopMutations(t *testing.T) {
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "value"},
			Value:  &ast.StringLiteral{},
		},
		&ast.Repeat{
			Kind: ast.RepeatForever,
			Body: []ast.Stmt{&ast.Set{
				Target: &ast.Variable{Name: "value"},
				Value: concatenate(
					&ast.Variable{Name: "value"},
					&ast.StringLiteral{Value: "x"},
				),
			}},
		},
	}}}}

	Strings{}.Rewrite(script)

	value := script.Handlers[0].Body[1].(*ast.Repeat).Body[0].(*ast.Set).Value
	if binary, ok := value.(*ast.Binary); !ok {
		t.Fatalf("loop-carried value was propagated: %#v", value)
	} else if _, ok := binary.Left.(*ast.Variable); !ok {
		t.Fatalf("loop-carried left operand = %#v", binary.Left)
	}
}

func TestStringsDoesNotPropagatePartialTryAssignments(t *testing.T) {
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "value"},
			Value:  &ast.StringLiteral{Value: "before"},
		},
		&ast.Try{
			Body: []ast.Stmt{&ast.Set{
				Target: &ast.Variable{Name: "value"},
				Value:  &ast.StringLiteral{Value: "during"},
			}},
			ErrorBody: []ast.Stmt{&ast.Set{
				Target: &ast.Variable{Name: "result"},
				Value: concatenate(
					&ast.Variable{Name: "value"},
					&ast.StringLiteral{Value: "-error"},
				),
			}},
		},
	}}}}

	Strings{}.Rewrite(script)

	value := script.Handlers[0].Body[1].(*ast.Try).ErrorBody[0].(*ast.Set).Value
	if binary, ok := value.(*ast.Binary); !ok {
		t.Fatalf("partial try value was propagated: %#v", value)
	} else if _, ok := binary.Left.(*ast.Variable); !ok {
		t.Fatalf("partial try left operand = %#v", binary.Left)
	}
}

func TestStringsRefusesUnknownConcatenationTypes(t *testing.T) {
	value := concatenate(
		&ast.Variable{Name: "unknown"},
		&ast.StringLiteral{Value: "a"},
		&ast.StringLiteral{Value: "b"},
	)
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{Target: &ast.Variable{Name: "result"}, Value: value},
	}}}}

	Strings{}.Rewrite(script)

	got := script.Handlers[0].Body[0].(*ast.Set).Value
	outer, ok := got.(*ast.Binary)
	if !ok {
		t.Fatalf("unknown concatenation = %#v", got)
	}
	if literal, ok := outer.Right.(*ast.StringLiteral); !ok || literal.Value != "b" {
		t.Fatalf("unknown concatenation was reassociated: %#v", got)
	}
}

func TestStringsDoesNotPropagateSharedVariables(t *testing.T) {
	for _, test := range []struct {
		name       string
		properties []ast.Property
		prefix     []ast.Stmt
	}{
		{
			name:       "property",
			properties: []ast.Property{{Name: "shared", Value: &ast.StringLiteral{Value: "initial"}}},
		},
		{
			name: "explicit-global",
			prefix: []ast.Stmt{&ast.Declaration{
				Names:  []string{"shared"},
				Global: true,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := append([]ast.Stmt{}, test.prefix...)
			body = append(body,
				&ast.Set{
					Target: &ast.Variable{Name: "shared"},
					Value:  &ast.StringLiteral{Value: "local-looking"},
				},
				&ast.Set{
					Target: &ast.Variable{Name: "result"},
					Value: concatenate(
						&ast.Variable{Name: "shared"},
						&ast.StringLiteral{Value: "-suffix"},
					),
				},
			)
			script := &ast.Script{
				Properties: test.properties,
				Handlers:   []*ast.Handler{{Body: body}},
			}

			Strings{}.Rewrite(script)

			got := script.Handlers[0].Body[len(body)-1].(*ast.Set).Value
			if _, ok := got.(*ast.Binary); !ok {
				t.Fatalf("shared variable was propagated: %#v", got)
			}
		})
	}
}

func TestStringsDoesNotPropagateRepeatConditionMutations(t *testing.T) {
	loop := &ast.Repeat{
		Kind: ast.RepeatWhile,
		Condition: concatenate(
			&ast.Variable{Name: "value"},
			&ast.StringLiteral{Value: "-condition"},
		),
		Body: []ast.Stmt{&ast.Set{
			Target: &ast.Variable{Name: "value"},
			Value:  &ast.StringLiteral{Value: "changed"},
		}},
	}
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "value"},
			Value:  &ast.StringLiteral{Value: "initial"},
		},
		loop,
	}}}}

	Strings{}.Rewrite(script)

	if _, ok := loop.Condition.(*ast.Binary); !ok {
		t.Fatalf("repeat condition mutation was propagated: %#v", loop.Condition)
	}
}

func TestStringsRespectsErrorBindings(t *testing.T) {
	handler := &ast.Try{
		ErrorName: "problem",
		ErrorBody: []ast.Stmt{&ast.Set{
			Target: &ast.Variable{Name: "result"},
			Value: concatenate(
				&ast.Variable{Name: "problem"},
				&ast.StringLiteral{Value: "-suffix"},
			),
		}},
	}
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "problem"},
			Value:  &ast.StringLiteral{Value: "outer"},
		},
		handler,
	}}}}

	Strings{}.Rewrite(script)

	value := handler.ErrorBody[0].(*ast.Set).Value
	if _, ok := value.(*ast.Binary); !ok {
		t.Fatalf("error binding was propagated: %#v", value)
	}
}

func TestStringsIsolatesNestedScriptScopes(t *testing.T) {
	object := &ast.ScriptObject{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "result"},
			Value: concatenate(
				&ast.Variable{Name: "fragment"},
				&ast.StringLiteral{Value: "-inner"},
			),
		},
	}}}}
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "fragment"},
			Value:  &ast.StringLiteral{Value: "outer"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "object"},
			Value:  object,
		},
		&ast.Set{
			Target: &ast.Variable{Name: "fragment"},
			Value:  &ast.StringLiteral{Value: "after"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "result"},
			Value: concatenate(
				&ast.Variable{Name: "fragment"},
				&ast.StringLiteral{Value: "-outer"},
			),
		},
	}}}}

	Strings{}.Rewrite(script)

	value := object.Handlers[0].Body[0].(*ast.Set).Value
	if _, ok := value.(*ast.Binary); !ok {
		t.Fatalf("outer value leaked into nested script: %#v", value)
	}
	outer := script.Handlers[0].Body[3].(*ast.Set).Value
	if _, ok := outer.(*ast.Binary); !ok {
		t.Fatalf("handler containing nested script propagated locals: %#v", outer)
	}
}

func TestStringsInvalidatesEscapedReferences(t *testing.T) {
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "reference"},
			Value: &ast.CopyExpr{
				Value: &ast.Variable{Name: "fragment"},
			},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "fragment"},
			Value:  &ast.StringLiteral{Value: "before"},
		},
		&ast.Set{
			Target: &ast.Specifier{
				Kind:      ast.PropertySpecifier,
				Object:    &ast.Keyword{Fallback: "contents"},
				Container: &ast.Variable{Name: "reference"},
			},
			Value: &ast.StringLiteral{Value: "after"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "result"},
			Value: concatenate(
				&ast.Variable{Name: "fragment"},
				&ast.StringLiteral{Value: "-after"},
			),
		},
	}}}}

	Strings{}.Rewrite(script)

	value := script.Handlers[0].Body[3].(*ast.Set).Value
	if _, ok := value.(*ast.Binary); !ok {
		t.Fatalf("persistent reference was propagated through: %#v", value)
	}
}

func TestStringsStillFoldsExpressionsWhenFlowTrackingIsDisabled(t *testing.T) {
	script := &ast.Script{Handlers: []*ast.Handler{{Body: []ast.Stmt{
		&ast.Set{
			Target: &ast.Variable{Name: "reference"},
			Value:  &ast.CopyExpr{Value: &ast.Variable{Name: "fragment"}},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "fragment"},
			Value:  &ast.StringLiteral{Value: "dynamic"},
		},
		&ast.Set{
			Target: &ast.Variable{Name: "flow_result"},
			Value: concatenate(
				&ast.Variable{Name: "fragment"},
				&ast.StringLiteral{Value: "-suffix"},
			),
		},
		&ast.Set{
			Target: &ast.Variable{Name: "literal_result"},
			Value: concatenate(
				&ast.StringLiteral{Value: "a"},
				&ast.StringLiteral{Value: "b"},
				standardCommand(
					"ASCII character",
					"sysontoc",
					ast.DirectArgument{Value: &ast.NumberLiteral{Integer: 67}},
				),
			),
		},
	}}}}

	Strings{}.Rewrite(script)

	if _, ok := script.Handlers[0].Body[2].(*ast.Set).Value.(*ast.Binary); !ok {
		t.Fatal("flow-sensitive value was propagated")
	}
	value := script.Handlers[0].Body[3].(*ast.Set).Value
	if literal, ok := value.(*ast.StringLiteral); !ok || literal.Value != "abC" {
		t.Fatalf("context-free value = %#v", value)
	}
}

func TestStringsRejectsInvalidASCIICharacterArguments(t *testing.T) {
	for _, test := range []struct {
		name   string
		number *ast.NumberLiteral
	}{
		{"real", &ast.NumberLiteral{IsReal: true, Real: 65.5}},
		{"surrogate", &ast.NumberLiteral{Integer: 0xd800}},
		{"negative", &ast.NumberLiteral{Integer: -1}},
		{"primary-encoding-dependent", &ast.NumberLiteral{Integer: 128}},
		{"out-of-range", &ast.NumberLiteral{Integer: 256}},
		{"too-large", &ast.NumberLiteral{Integer: 0x110000}},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := standardCommand(
				"ASCII character",
				"sysontoc",
				ast.DirectArgument{Value: test.number},
			)
			if got := stringExpression(call); got != call {
				t.Fatalf("invalid ASCII argument was folded: %#v", got)
			}
		})
	}
	t.Run("foreign-same-name-command", func(t *testing.T) {
		call := standardCommand(
			"ASCII character",
			"fakefake",
			ast.DirectArgument{Value: &ast.NumberLiteral{Integer: 65}},
		)
		if got := stringExpression(call); got != call {
			t.Fatalf("foreign command was folded: %#v", got)
		}
	})
}

func TestInferStringRecognizesTrustedTextProducers(t *testing.T) {
	keyword := func(code, fallback string) *ast.Keyword {
		return &ast.Keyword{Code: []byte(code), Fallback: fallback}
	}
	for _, test := range []struct {
		name       string
		expression ast.Expr
	}{
		{
			name: "quoted-form-operator",
			expression: &ast.Binary{
				Op:    ast.Of,
				Left:  keyword("strq", "quoted form"),
				Right: &ast.Variable{Name: "value"},
			},
		},
		{
			name: "posix-path-operator",
			expression: &ast.Binary{
				Op:    ast.Of,
				Left:  keyword("psxp", "POSIX path"),
				Right: &ast.Variable{Name: "value"},
			},
		},
		{
			name: "string-coercion",
			expression: &ast.Coerce{
				Value: &ast.Variable{Name: "value"},
				Type:  keyword("TEXT", "string"),
			},
		},
		{
			name: "text-coercion",
			expression: &ast.Coerce{
				Value: &ast.Variable{Name: "value"},
				Type:  keyword("ctxt", "text"),
			},
		},
		{
			name: "unicode-text-coercion",
			expression: &ast.Coerce{
				Value: &ast.Variable{Name: "value"},
				Type:  keyword("utxt", "Unicode text"),
			},
		},
		{
			name:       "do-shell-script",
			expression: standardCommand("do shell script", "sysoexec"),
		},
		{
			name: "quoted-form-specifier",
			expression: &ast.Specifier{
				Kind:   ast.PropertySpecifier,
				Object: keyword("strq", "quoted form"),
			},
		},
		{
			name: "posix-path-specifier",
			expression: &ast.Specifier{
				Kind:   ast.PropertySpecifier,
				Object: keyword("psxp", "POSIX path"),
			},
		},
		{name: "space", expression: keyword("spac", "space")},
		{name: "tab", expression: keyword("tab ", "tab")},
		{name: "linefeed", expression: keyword("lnfd", "linefeed")},
		{name: "return", expression: keyword("ret ", "return")},
	} {
		t.Run(test.name, func(t *testing.T) {
			fact, ok := inferString(test.expression, stringEnvironment{})
			if !ok {
				t.Fatal("trusted text producer was not recognized")
			}
			if fact.Constant {
				t.Fatalf("dynamic producer was treated as constant: %#v", fact)
			}
		})
	}
}

func TestNamedKeywordPrefersRawCode(t *testing.T) {
	if namedKeyword(
		&ast.Keyword{Code: []byte("bad!"), Fallback: "text"},
		"text",
	) {
		t.Fatal("conflicting fallback overrode raw code")
	}
	if !namedKeyword(&ast.Keyword{Fallback: "text"}, "text") {
		t.Fatal("fallback-only keyword was not recognized")
	}
}

func concatenate(expressions ...ast.Expr) ast.Expr {
	if len(expressions) == 0 {
		return &ast.StringLiteral{}
	}
	result := expressions[0]
	for _, expression := range expressions[1:] {
		result = &ast.Binary{Op: ast.Concatenate, Left: result, Right: expression}
	}
	return result
}

func standardCommand(name, code string, arguments ...ast.Argument) *ast.CommandCall {
	call := &ast.CommandCall{Name: name, Arguments: arguments}
	copy(call.Code[:], code)
	return call
}

func TestStringContracts(t *testing.T) {
	t.Run("preserves-general-list", func(t *testing.T) {
		list := &ast.List{Elements: []ast.Expr{
			&ast.StringLiteral{Value: "killall"},
			&ast.StringLiteral{Value: "-9"},
			&ast.Variable{Name: "process_name"},
		}}
		if got := stringExpression(list); got != list {
			t.Fatalf("general list was rewritten: %#v", got)
		}
	})
	t.Run("empty-list", func(t *testing.T) {
		list := &ast.List{}
		if got := stringExpression(list); got != list {
			t.Fatalf("empty list was rewritten: %#v", got)
		}
	})
	t.Run("single-character-unicode-list", func(t *testing.T) {
		list := &ast.List{Elements: []ast.Expr{
			&ast.StringLiteral{Value: "a"},
			&ast.StringLiteral{Value: `"`},
			&ast.StringLiteral{Value: "é"},
		}}
		got := stringExpression(list).(*ast.List)
		if len(got.Elements) != 3 {
			t.Fatalf("list = %#v", got)
		}
	})
	t.Run("numbers-remain-numeric", func(t *testing.T) {
		number := &ast.NumberLiteral{Integer: 85}
		if got := stringExpression(number); got != number {
			t.Fatalf("number was rewritten: %#v", got)
		}
	})
	t.Run("ascii-character-command", func(t *testing.T) {
		call := standardCommand(
			"sysontoc",
			"sysontoc",
			ast.DirectArgument{Value: &ast.NumberLiteral{Integer: 85}},
		)
		got, ok := stringExpression(call).(*ast.StringLiteral)
		if !ok || got.Value != "U" {
			t.Fatalf("ASCII character = %#v", got)
		}
	})
	t.Run("empty-concatenation", func(t *testing.T) {
		expression := &ast.Binary{
			Op: ast.Concatenate, Left: &ast.StringLiteral{}, Right: &ast.StringLiteral{},
		}
		got, ok := stringExpression(expression).(*ast.StringLiteral)
		if !ok || got.Value != "" {
			t.Fatalf("empty concatenation = %#v", got)
		}
	})
}

func TestNamedPasses(t *testing.T) {
	pass, err := Named("strings")
	if err != nil || pass.Name() != "strings" {
		t.Errorf("Named(strings) = %#v, %v", pass, err)
	}
	for _, name := range []string{"naive-strings", "osaminer", "missing"} {
		if _, err := Named(name); err == nil {
			t.Errorf("removed or unknown pass %q accepted", name)
		}
	}
}
