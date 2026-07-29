package ast

import (
	"strings"
	"testing"
)

func TestFormatterPrecedenceAndBlocks(t *testing.T) {
	script := &Script{Handlers: []*Handler{{
		Name: "calculate", Parameters: []Parameter{{Name: "x"}}, Body: []Stmt{
			&Set{Target: &Variable{Name: "answer"}, Value: &Binary{Op: Multiply, Left: &Binary{Op: Add, Left: &Variable{Name: "x"}, Right: &NumberLiteral{Integer: 2}}, Right: &NumberLiteral{Integer: 3}}},
			&If{Condition: &Binary{Op: Greater, Left: &Variable{Name: "answer"}, Right: &NumberLiteral{Integer: 5}}, Then: []Stmt{&Return{Value: &Variable{Name: "answer"}, Explicit: true}}},
		},
	}}}
	source, err := Format(script, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"on calculate(x)", "set answer to (x + 2) * 3", "if answer > 5 then", "return answer", "end calculate"} {
		if !strings.Contains(source, want) {
			t.Errorf("source missing %q:\n%s", want, source)
		}
	}
}

func TestReservedIdentifier(t *testing.T) {
	if got := identifier("class"); got != "|class|" {
		t.Fatalf("got %q", got)
	}
	if got := identifier("items"); got != "|items|" {
		t.Fatalf("got %q", got)
	}
	if got := identifier("name"); got != "|name|" {
		t.Fatalf("got %q", got)
	}
	if got := identifier("normal_name2"); got != "normal_name2" {
		t.Fatalf("got %q", got)
	}
}

func TestQuoteStringPreservesPrivateUseCharacters(t *testing.T) {
	got := quoteString("tv \"name\"")
	if got != "\"tv \\\"name\\\"\"" {
		t.Fatalf("got %q", got)
	}
}

func TestRawCodeDecodesMacRoman(t *testing.T) {
	if got := rawCode([]byte{'c', 'a', 'f', 0x8e}); got != "café" {
		t.Fatalf("rawCode = %q", got)
	}
	if got := rawCode([]byte{'a', 0, 'b', 'c'}); got != "0x61006263" {
		t.Fatalf("rawCode with control byte = %q", got)
	}
}

func TestComputedSpecifierSelectorIsParenthesized(t *testing.T) {
	script := &Script{Handlers: []*Handler{{
		Name: "run", IsRunHandler: true,
		Body: []Stmt{&Expression{Value: &Specifier{
			Kind: IndexSpecifier, Object: &Keyword{Fallback: "menu item"},
			From:      &Binary{Op: Concatenate, Left: &StringLiteral{Value: "Sync "}, Right: &Variable{Name: "device"}},
			Container: &Variable{Name: "menu"},
		}}},
	}}}
	source, err := Format(script, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, `menu item ("Sync " & device) of menu`) {
		t.Fatalf("source = %q", source)
	}
}
