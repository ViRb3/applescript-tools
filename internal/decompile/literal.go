package decompile

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"applescript-tools/ast"
	"applescript-tools/internal/fas"
	"applescript-tools/terminology"
)

func literal(value fas.Value) ast.Expr {
	switch value := value.(type) {
	case nil, fas.Nil:
		return &ast.MissingLiteral{}
	case fas.Bool:
		return &ast.BooleanLiteral{Value: bool(value)}
	case fas.Integer:
		number := int64(value)
		// Some compiled scripts store negative 16-bit literals in a long
		// integer object with a zero-extended payload.
		if number >= 0x8000 && number <= 0xffff {
			number -= 0x10000
		}
		return &ast.NumberLiteral{Integer: number}
	case fas.Float:
		return &ast.NumberLiteral{Real: float64(value), IsReal: true}
	case fas.Special:
		switch value {
		case fas.SpecialTrue:
			return &ast.BooleanLiteral{Value: true}
		case fas.SpecialFalse:
			return &ast.BooleanLiteral{Value: false}
		default:
			return &ast.MissingLiteral{}
		}
	case fas.Constant:
		return constant(uint64(value))
	case *fas.Bytes:
		return &ast.StringLiteral{Value: string(value.Data)}
	case *fas.UnicodeText:
		return &ast.StringLiteral{Value: utf16String(value.Text)}
	case *fas.Object:
		return literal(value.Value)
	case *fas.RawData:
		return rawData(value.Data)
	case *fas.Descriptor:
		if string(value.Type[:]) == "ldt " {
			if result := longDate(value.Content); result != nil {
				return result
			}
		}
		return &ast.Application{Name: aliasName(value.Content)}
	case *fas.Pair:
		var elements []ast.Expr
		seen := map[*fas.Pair]bool{}
		for current := value; current != nil && !current.Empty && !seen[current]; {
			seen[current] = true
			elements = append(elements, literal(current.Head))
			next, ok := current.Tail.(*fas.Pair)
			if !ok {
				break
			}
			current = next
		}
		return &ast.List{Elements: elements}
	case *fas.Binding:
		var fields []ast.RecordField
		seen := map[*fas.Binding]bool{}
		for current := value; current != nil && !current.Empty && !seen[current]; {
			seen[current] = true
			fields = append(fields, ast.RecordField{Label: literal(current.Key), Value: literal(current.Value)})
			next, ok := current.Next.(*fas.Binding)
			if !ok {
				break
			}
			current = next
		}
		return &ast.Record{Fields: fields}
	case *fas.Vector:
		if value.HasType && value.Type == 19 {
			return &ast.It{}
		}
		if value.HasType && len(value.Children) > 1 {
			if value.Type == 177 {
				switch text := value.Children[1].(type) {
				case *fas.Bytes:
					return &ast.StringLiteral{Value: utf16String(text.Data)}
				case *fas.UnicodeText:
					return &ast.StringLiteral{Value: utf16String(text.Text)}
				}
			}
			if value.Type == 4 {
				if nested, ok := value.Children[2].(*fas.Vector); ok {
					var elements []ast.Expr
					start := 0
					if nested.HasType {
						start = 1
					}
					for _, child := range nested.Children[start:] {
						elements = append(elements, literal(child))
					}
					return &ast.List{Elements: elements}
				}
			}
			return literal(value.Children[1])
		}
		if len(value.Children) == 1 {
			return literal(value.Children[0])
		}
		var elements []ast.Expr
		for _, child := range value.Children {
			elements = append(elements, literal(child))
		}
		return &ast.List{Elements: elements}
	default:
		return &ast.Keyword{Fallback: fmt.Sprintf("«unsupported %T»", value)}
	}
}

func constant(value uint64) ast.Expr {
	width := 4
	for n := value; n > 0xffffffff; n >>= 8 {
		width++
	}
	raw := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		raw[i] = byte(value)
		value >>= 8
	}
	switch string(raw) {
	case "misccura":
		return &ast.Keyword{Fallback: "current application"}
	case "FTPc":
		return &ast.Keyword{Code: raw, Fallback: "path"}
	case "lnfd":
		return &ast.Keyword{Code: raw, Fallback: "linefeed"}
	case "prun":
		return &ast.Keyword{Code: raw, Fallback: "running"}
	case "rslt":
		return &ast.Keyword{Code: raw, Fallback: "result"}
	case "spac":
		return &ast.Keyword{Code: raw, Fallback: "space"}
	case "strq":
		return &ast.Keyword{Code: raw, Fallback: "quoted form"}
	}
	if len(raw) == 8 {
		return &ast.Keyword{Code: raw}
	}
	return &ast.Keyword{Code: raw}
}

func rawData(data []byte) ast.Expr {
	var code terminology.Code4
	content := data
	if len(data) >= 4 {
		copy(code[:], data[:4])
		printable := true
		for _, b := range code {
			if b < 0x20 || b > 0x7e {
				printable = false
			}
		}
		if printable {
			content = data[4:]
		} else {
			copy(code[:], []byte("****"))
		}
	} else {
		copy(code[:], []byte("****"))
	}
	if string(code[:]) == "ldt " {
		if result := longDate(content); result != nil {
			return result
		}
	}
	return &ast.RawDataLiteral{Type: code, Data: append([]byte(nil), content...)}
}

func longDate(data []byte) ast.Expr {
	if len(data) != 8 {
		return nil
	}
	seconds := int64(binary.BigEndian.Uint64(data))
	value := time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
	text := fmt.Sprintf("%d/%d/%d", value.Month(), value.Day(), value.Year())
	if value.Hour() != 0 || value.Minute() != 0 || value.Second() != 0 {
		hour := value.Hour() % 12
		if hour == 0 {
			hour = 12
		}
		suffix := "AM"
		if value.Hour() >= 12 {
			suffix = "PM"
		}
		text += fmt.Sprintf(" %d:%02d:%02d %s", hour, value.Minute(), value.Second(), suffix)
	}
	return &ast.DateLiteral{Value: text}
}

func aliasName(content []byte) string {
	if len(content) > 51 && content[7] == 2 {
		n := int(content[50])
		if 51+n <= len(content) {
			name := string(content[51 : 51+n])
			return legacyApplicationName(strings.TrimSuffix(name, ".app"))
		}
	}
	value := string(content)
	if before, _, ok := strings.Cut(value, ".app/"); ok {
		value = before
	}
	if index := strings.LastIndex(value, ":"); index >= 0 {
		value = value[index+1:]
	}
	return legacyApplicationName(strings.Trim(value, "\x00"))
}

func legacyApplicationName(name string) string {
	if name == "System Settings" {
		return "System Preferences"
	}
	return name
}

func utf16String(data []byte) string {
	if len(data)%2 != 0 {
		return string(data)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(data[i*2:])
	}
	return string(utf16.Decode(units))
}

func identifierValue(value fas.Value, fallback string) string {
	switch value := value.(type) {
	case *fas.Bytes:
		if validIdentifier(string(value.Data)) {
			return string(value.Data)
		}
	case *fas.Object:
		if keyword, ok := literal(value).(*ast.Keyword); ok && keyword.Fallback != "" && validIdentifier(keyword.Fallback) {
			return keyword.Fallback
		}
	}
	return fallback
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
