package ast

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"applescript-tools/internal/macroman"
	"applescript-tools/terminology"
)

type Formatter struct {
	Terms *terminology.Registry
}

func Format(script *Script, terms *terminology.Registry) (string, error) {
	f := &Formatter{Terms: terms}
	return f.Script(script)
}

func (f *Formatter) Script(script *Script) (string, error) {
	var sections []string
	var declarations []string
	for _, use := range script.Uses {
		switch {
		case use.Framework:
			declarations = append(declarations, `use framework `+quoteString(use.Name))
		case use.ScriptingAdditions:
			declarations = append(declarations, "use scripting additions")
		default:
			if use.Alias != "" {
				declarations = append(declarations, "use "+identifier(use.Alias)+" : script "+quoteString(use.Name))
			} else {
				declarations = append(declarations, `use script `+quoteString(use.Name))
			}
		}
	}
	for _, property := range script.Properties {
		declarations = append(declarations, "property "+identifier(property.Name)+" : "+f.expr(property.Value, 0))
	}
	if len(declarations) != 0 {
		sections = append(sections, strings.Join(declarations, "\n"))
	}
	for _, object := range script.Objects {
		sections = append(sections, f.scriptObject(object, 0))
	}
	for _, handler := range script.Handlers {
		sections = append(sections, f.handler(handler, 0))
	}
	return strings.Join(sections, "\n\n") + "\n", nil
}

func (f *Formatter) handler(handler *Handler, indent int) string {
	var b strings.Builder
	prefix := strings.Repeat("    ", indent)
	name := identifier(handler.Name)
	if handler.IsRunHandler {
		name = "run"
	}
	if handler.EventCode != nil && !handler.IsRunHandler && handler.UnresolvedParameters {
		eventName := "«event " + rawCode(handler.EventCode[:]) + "»"
		fmt.Fprintf(&b, "%son %s «unresolved parameters»\n", prefix, eventName)
		b.WriteString(f.statements(handler.Body, indent+1))
		fmt.Fprintf(&b, "%send %s", prefix, eventName)
		return b.String()
	}
	if handler.EventCode != nil && !handler.IsRunHandler && hasLabeledHandlerParameter(handler.Parameters) {
		eventName := "«event " + rawCode(handler.EventCode[:]) + "»"
		fmt.Fprintf(&b, "%son %s", prefix, eventName)
		wroteDirect := false
		wroteNamed := false
		for _, parameter := range handler.Parameters {
			if parameter.Code == nil {
				if !wroteDirect {
					b.WriteByte(' ')
				} else {
					b.WriteString(", ")
				}
				b.WriteString(identifier(parameter.Name))
				wroteDirect = true
				continue
			}
			if !wroteNamed {
				b.WriteString(" given ")
				wroteNamed = true
			} else {
				b.WriteString(", ")
			}
			b.WriteString("«class " + rawCode(parameter.Code[:]) + "»:" + identifier(parameter.Name))
		}
		b.WriteByte('\n')
		b.WriteString(f.statements(handler.Body, indent+1))
		fmt.Fprintf(&b, "%send %s", prefix, eventName)
		return b.String()
	}
	if handler.EventCode != nil && !handler.IsRunHandler {
		eventName := "«event " + rawCode(handler.EventCode[:]) + "»"
		if f.Terms != nil {
			if command, ok := f.Terms.Command(*handler.EventCode); ok {
				eventName = identifier(command.Name)
			}
		}
		fmt.Fprintf(&b, "%son %s", prefix, eventName)
		for _, parameter := range handler.Parameters {
			b.WriteByte(' ')
			b.WriteString(identifier(parameter.Name))
		}
		b.WriteByte('\n')
		b.WriteString(f.statements(handler.Body, indent+1))
		fmt.Fprintf(&b, "%send %s", prefix, eventName)
		return b.String()
	}
	parameters := make([]string, len(handler.Parameters))
	for i, parameter := range handler.Parameters {
		parameters[i] = identifier(parameter.Name)
	}
	fmt.Fprintf(&b, "%son %s", prefix, name)
	if !handler.IsRunHandler || len(parameters) != 0 {
		fmt.Fprintf(&b, "(%s)", strings.Join(parameters, ", "))
	}
	b.WriteByte('\n')
	b.WriteString(f.statements(handler.Body, indent+1))
	fmt.Fprintf(&b, "%send %s", prefix, name)
	return b.String()
}

func hasLabeledHandlerParameter(parameters []Parameter) bool {
	for _, parameter := range parameters {
		if parameter.Code != nil {
			return true
		}
	}
	return false
}

func (f *Formatter) statements(statements []Stmt, indent int) string {
	var b strings.Builder
	for _, statement := range statements {
		b.WriteString(f.stmt(statement, indent))
		b.WriteByte('\n')
	}
	return b.String()
}

func (f *Formatter) stmt(statement Stmt, indent int) string {
	prefix := strings.Repeat("    ", indent)
	switch node := statement.(type) {
	case *Set:
		return prefix + "set " + f.expr(node.Target, 0) + " to " + f.topLevelExpr(node.Value)
	case *Copy:
		return prefix + "copy " + f.expr(node.Source, 0) + " to " + f.expr(node.Target, 0)
	case *Expression:
		if object, ok := node.Value.(*ScriptObject); ok {
			return f.scriptObject(object, indent)
		}
		return prefix + f.expr(node.Value, 0)
	case *Declaration:
		names := make([]string, len(node.Names))
		for i, name := range node.Names {
			names[i] = identifier(name)
		}
		if node.Global {
			return prefix + "global " + strings.Join(names, ", ")
		}
		return prefix + "local " + strings.Join(names, ", ")
	case *If:
		var b strings.Builder
		fmt.Fprintf(&b, "%sif %s then\n", prefix, f.expr(node.Condition, 0))
		b.WriteString(f.statements(node.Then, indent+1))
		if len(node.Else) != 0 {
			fmt.Fprintf(&b, "%selse\n", prefix)
			b.WriteString(f.statements(node.Else, indent+1))
		}
		fmt.Fprintf(&b, "%send if", prefix)
		return b.String()
	case *Repeat:
		var header string
		switch node.Kind {
		case RepeatTimes:
			header = "repeat " + f.expr(node.Times, 0) + " times"
		case RepeatWhile:
			header = "repeat while " + f.expr(node.Condition, 0)
		case RepeatUntil:
			header = "repeat until " + f.expr(node.Condition, 0)
		case RepeatIn:
			header = "repeat with " + identifier(node.Variable) + " in " + f.expr(node.Collection, 0)
		case RepeatRange:
			header = "repeat with " + identifier(node.Variable) + " from " + f.expr(node.From, 0) + " to " + f.expr(node.To, 0)
			if node.By != nil {
				header += " by " + f.expr(node.By, 0)
			}
		default:
			header = "repeat"
		}
		return prefix + header + "\n" + f.statements(node.Body, indent+1) + prefix + "end repeat"
	case *Try:
		var b strings.Builder
		b.WriteString(prefix + "try\n")
		b.WriteString(f.statements(node.Body, indent+1))
		if len(node.ErrorBody) != 0 || node.ErrorName != "" || node.NumberName != "" {
			b.WriteString(prefix + "on error")
			if node.ErrorName != "" {
				b.WriteString(" " + identifier(node.ErrorName))
			}
			if node.NumberName != "" {
				b.WriteString(" number " + identifier(node.NumberName))
			}
			if node.PartialResultName != "" {
				b.WriteString(" partial result " + identifier(node.PartialResultName))
			}
			if node.FromName != "" {
				b.WriteString(" from " + identifier(node.FromName))
			}
			if node.ToName != "" {
				b.WriteString(" to " + identifier(node.ToName))
			}
			b.WriteByte('\n')
			b.WriteString(f.statements(node.ErrorBody, indent+1))
		}
		b.WriteString(prefix + "end try")
		return b.String()
	case *Tell:
		return prefix + "tell " + f.expr(node.Target, 0) + "\n" + f.statements(node.Body, indent+1) + prefix + "end tell"
	case *Considering:
		return prefix + "considering " + strings.Join(node.Options, ", ") + "\n" + f.statements(node.Body, indent+1) + prefix + "end considering"
	case *Timeout:
		return prefix + "with timeout of " + f.expr(node.Seconds, 0) + " seconds\n" + f.statements(node.Body, indent+1) + prefix + "end timeout"
	case *Transaction:
		return prefix + "with transaction\n" + f.statements(node.Body, indent+1) + prefix + "end transaction"
	case *Continue:
		if node.Call == nil {
			return prefix + "continue «unresolved»"
		}
		return prefix + "continue " + f.topLevelExpr(node.Call)
	case *Return:
		if node.Value == nil {
			return prefix + "return"
		}
		return prefix + "return " + f.topLevelExpr(node.Value)
	case *ExitRepeat:
		return prefix + "exit repeat"
	case *Comment:
		return prefix + "-- " + node.Text
	default:
		return prefix + "-- unsupported statement " + fmt.Sprintf("%T", statement)
	}
}

func (f *Formatter) topLevelExpr(expression Expr) string {
	formatted := f.expr(expression, 0)
	switch expression.(type) {
	case *Coerce, *CommandCall:
		if len(formatted) >= 2 && formatted[0] == '(' && formatted[len(formatted)-1] == ')' {
			return formatted[1 : len(formatted)-1]
		}
	}
	return formatted
}

var precedence = map[BinaryKind]int{
	Or: 1, And: 2, Equal: 3, NotEqual: 3, Greater: 3, GreaterEqual: 3, Less: 3, LessEqual: 3,
	StartsWith: 3, EndsWith: 3, Contains: 3, Concatenate: 4, Add: 5, Subtract: 5,
	Multiply: 6, Divide: 6, Quotient: 6, Remainder: 6, Power: 7, Of: 8,
}

func (f *Formatter) expr(expression Expr, parent int) string {
	switch node := expression.(type) {
	case nil:
		return "missing value"
	case *StringLiteral:
		return quoteString(node.Value)
	case *NumberLiteral:
		if node.IsReal {
			return strconv.FormatFloat(node.Real, 'g', -1, 64)
		}
		return strconv.FormatInt(node.Integer, 10)
	case *BooleanLiteral:
		if node.Value {
			return "true"
		}
		return "false"
	case *MissingLiteral:
		return "missing value"
	case *DateLiteral:
		return `date ` + quoteString(node.Value)
	case *RawDataLiteral:
		return "«data " + rawCode(node.Type[:]) + hex.EncodeToString(node.Data) + "»"
	case *OpaqueLiteral:
		return fmt.Sprintf("«unsupported runtime data type %d: %s»", node.RuntimeType, hex.EncodeToString(node.Data))
	case *Keyword:
		if node.Fallback != "" {
			return node.Fallback
		}
		if len(node.Code) == 4 {
			var code terminology.Code4
			copy(code[:], node.Code)
			if f.Terms != nil {
				if name, ok := f.Terms.Term(code); ok {
					return name
				}
				if name, ok := f.Terms.Enumeration(code); ok {
					return name
				}
			}
			return "«class " + rawCode(node.Code) + "»"
		}
		if len(node.Code) == 8 {
			var member terminology.Code4
			copy(member[:], node.Code[4:])
			if f.Terms != nil {
				if name, ok := f.Terms.Term(member); ok {
					return name
				}
				if name, ok := f.Terms.Enumeration(member); ok {
					return name
				}
			}
			return "«constant " + rawCode(node.Code[:4]) + rawCode(node.Code[4:]) + "»"
		}
		return "«constant " + hex.EncodeToString(node.Code) + "»"
	case *Variable:
		return identifier(node.Name)
	case *Application:
		return `application ` + quoteString(node.Name)
	case *ScriptLibrary:
		return `script ` + quoteString(node.Name)
	case *Me:
		return "me"
	case *It:
		return "it"
	case *Undefined:
		return "missing value"
	case *List:
		items := make([]string, len(node.Elements))
		for i, item := range node.Elements {
			items[i] = f.expr(item, 0)
		}
		return "{" + strings.Join(items, ", ") + "}"
	case *Record:
		fields := make([]string, len(node.Fields))
		for i, field := range node.Fields {
			label := f.expr(field.Label, 0)
			if text, ok := field.Label.(*StringLiteral); ok {
				label = identifier(text.Value)
			}
			fields[i] = label + ":" + f.expr(field.Value, 0)
		}
		return "{" + strings.Join(fields, ", ") + "}"
	case *Unary:
		if node.Op == UnaryNot {
			return "not (" + f.expr(node.Value, 0) + ")"
		}
		return "-(" + f.expr(node.Value, 0) + ")"
	case *Binary:
		p := precedence[node.Op]
		left, right := node.Left, node.Right
		if node.Op == And || node.Op == Or {
			left = stripBooleanCoercion(left)
			right = stripBooleanCoercion(right)
		}
		value := f.expr(left, p) + " " + string(node.Op) + " " + f.expr(right, p+1)
		if p < parent {
			return "(" + value + ")"
		}
		return value
	case *Coerce:
		return "(" + f.expr(node.Value, 4) + " as " + f.expr(node.Type, 5) + ")"
	case *CopyExpr:
		return "a reference to " + f.expr(node.Value, 0)
	case *CommandCall:
		name := node.Name
		if name == "" && f.Terms != nil {
			if command, ok := f.Terms.Command(node.Code); ok {
				name = command.Name
			}
		}
		if name == "" {
			name = "«event " + rawCode(node.Code[:]) + "»"
		}
		var args []string
		for _, argument := range node.Arguments {
			switch value := argument.(type) {
			case DirectArgument:
				args = append(args, f.expr(value.Value, 0))
			case NamedArgument:
				n := value.Name
				if n == "" {
					if f.Terms != nil {
						n, _ = f.Terms.Term(value.Code)
					}
					if n == "" {
						n = "«class " + rawCode(value.Code[:]) + "»"
					}
				}
				args = append(args, n+" "+f.expr(value.Value, 0))
			case FlagArgument:
				n := value.Name
				if n == "" {
					if f.Terms != nil {
						n, _ = f.Terms.Term(value.Code)
					}
					if n == "" {
						n = "«class " + rawCode(value.Code[:]) + "»"
					}
				}
				if value.Enabled {
					args = append(args, "with "+n)
				} else {
					args = append(args, "without "+n)
				}
			}
		}
		call := name
		if len(args) > 0 {
			call += " " + strings.Join(args, " ")
		}
		if node.Target != nil {
			call = f.expr(node.Target, 9) + "'s " + call
		}
		if node.Name == "error" {
			return call
		}
		return "(" + call + ")"
	case *HandlerCall:
		args := make([]string, len(node.Arguments))
		for i, arg := range node.Arguments {
			args[i] = f.expr(arg, 0)
		}
		call := identifier(node.Name) + "(" + strings.Join(args, ", ") + ")"
		if node.Target != nil {
			if _, ok := node.Target.(*Me); ok {
				return "my " + call
			}
			target := f.expr(node.Target, 9)
			switch node.Target.(type) {
			case *Specifier, *Whose:
				target = "(" + target + ")"
			}
			return target + "'s " + call
		}
		return call
	case *Specifier:
		switch node.Kind {
		case RangeSpecifier:
			return f.expr(node.Object, 0) + " " + f.expr(node.From, 0) + " thru " + f.expr(node.To, 0) + " of " + f.expr(node.Container, 8)
		case IndexSpecifier:
			// A computed selector must bind before the following "of"
			// container clause (for example menu item ("Sync " & name)).
			value := f.expr(node.Object, 8) + " " + f.expr(node.From, 8)
			switch node.Container.(type) {
			case *Me, *It:
				return value
			}
			if node.Container != nil {
				return value + " of " + f.expr(node.Container, 8)
			}
			return value
		case KeySpecifier:
			keyName := node.KeyName
			if keyName == "" {
				keyName = "id"
			}
			value := f.expr(node.Object, 8) + " " + keyName + " " + f.expr(node.From, 8)
			switch node.Container.(type) {
			case *Me, *It:
				return value
			}
			if node.Container != nil {
				return value + " of " + f.expr(node.Container, 8)
			}
			return value
		case PropertySpecifier:
			switch node.Container.(type) {
			case *Me, *It:
				return f.expr(node.Object, 8)
			}
			return f.expr(node.Object, 8) + " of " + f.expr(node.Container, 8)
		case EverySpecifier:
			return "every " + f.expr(node.Object, 0) + " of " + f.expr(node.Container, 8)
		case BeginningSpecifier:
			return "beginning of " + f.expr(node.Container, 8)
		case EndSpecifier:
			container := f.expr(node.Container, 0)
			if nested, ok := node.Container.(*Specifier); ok && nested.Kind == EverySpecifier {
				container = "(" + container + ")"
			}
			return "end of (" + container + ")"
		case MiddleSpecifier:
			return "middle " + f.expr(node.Object, 0) + " of " + f.expr(node.Container, 8)
		case SomeSpecifier:
			return "some " + f.expr(node.Object, 0) + " of " + f.expr(node.Container, 8)
		default:
			return f.expr(node.Object, 8) + " of " + f.expr(node.Container, 8)
		}
	case *Whose:
		return f.expr(node.Object, 0) + " whose " + f.expr(node.Predicate, 0)
	case *ScriptObject:
		return f.scriptObject(node, 0)
	default:
		return "«unsupported " + fmt.Sprintf("%T", expression) + "»"
	}
}

func (f *Formatter) scriptObject(node *ScriptObject, indent int) string {
	prefix := strings.Repeat("    ", indent)
	var b strings.Builder
	b.WriteString(prefix + "script " + identifier(node.Name) + "\n")
	for _, p := range node.Properties {
		b.WriteString(prefix + "    property " + identifier(p.Name) + " : " + f.expr(p.Value, 0) + "\n")
	}
	if len(node.Properties) != 0 && len(node.Handlers) != 0 {
		b.WriteByte('\n')
	}
	for _, h := range node.Handlers {
		b.WriteString(f.handler(h, indent+1) + "\n")
	}
	b.WriteString(prefix + "end script")
	return b.String()
}

func stripBooleanCoercion(value Expr) Expr {
	for {
		coerce, ok := value.(*Coerce)
		if !ok {
			return value
		}
		keyword, ok := coerce.Type.(*Keyword)
		if !ok || string(keyword.Code) != "bool" {
			return value
		}
		value = coerce.Value
	}
}

func rawCode(code []byte) string {
	var decoded strings.Builder
	for _, b := range code {
		if b >= 0x20 && b <= 0x7e {
			decoded.WriteByte(b)
			continue
		}
		if b < 0x20 || b == 0x7f {
			return "0x" + hex.EncodeToString(code)
		}
		decoded.WriteRune(macroman.DecodeByte(b))
	}
	return decoded.String()
}

// quoteString follows AppleScript's string escape syntax while preserving
// printable Unicode verbatim. strconv.Quote escapes private-use characters as
// \uXXXX, but AppleScript does not recognize Go-style Unicode escapes.
func quoteString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

var reserved = map[string]bool{
	"and": true, "as": true, "class": true, "considering": true, "continue": true, "copy": true, "else": true,
	"end": true, "error": true, "exit": true, "false": true, "from": true, "if": true, "in": true,
	"item": true, "items": true, "length": true, "local": true, "me": true, "missing": true, "name": true, "not": true, "of": true, "on": true, "or": true,
	"path": true, "property": true, "record": true, "repeat": true, "return": true, "script": true, "set": true, "tell": true,
	"then": true, "to": true, "true": true, "try": true, "whose": true, "with": true, "without": true,
}

func identifier(value string) string {
	if value == "" {
		return "|unnamed|"
	}
	valid := !reserved[strings.ToLower(value)]
	for i, r := range value {
		if !(unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r))) {
			valid = false
			break
		}
	}
	if valid {
		return value
	}
	return "|" + strings.ReplaceAll(value, "|", "\\|") + "|"
}
