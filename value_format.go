package applescript

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"applescript-tools/internal/fas"
)

func displayValue(v fas.Value) string {
	switch v := v.(type) {
	case nil:
		return "<missing>"
	case fas.Nil:
		return "nil"
	case fas.Bool:
		return strconv.FormatBool(bool(v))
	case fas.Integer:
		return strconv.FormatInt(int64(v), 10)
	case fas.Float:
		return strconv.FormatFloat(float64(v), 'g', -1, 64)
	case fas.Special:
		return fmt.Sprintf("special(0x%x)", uint64(v))
	case fas.Constant:
		return fmt.Sprintf("constant(0x%x)", uint64(v))
	case *fas.Symbol:
		return fmt.Sprintf("symbol(0x%x)", v.Number)
	case *fas.Bytes:
		if printable(v.Data) {
			return strconv.Quote(string(v.Data))
		}
		return "0x" + hex.EncodeToString(v.Data)
	case *fas.UnicodeText:
		return strconv.Quote(decodeUTF16BE(v.Text))
	case *fas.Object:
		return displayValue(v.Value)
	case *fas.RawData:
		return fmt.Sprintf("data[%d](%s)", len(v.Data), previewHex(v.Data))
	case *fas.Descriptor:
		return fmt.Sprintf("descriptor(%q,%s)", v.Type, previewHex(v.Content))
	case *fas.EventIdentifier:
		parts := make([]string, len(v.Fields))
		for i := range v.Fields {
			parts[i] = codeDisplay(v.Fields[i])
		}
		return "event(" + strings.Join(parts, ",") + ")"
	case *fas.Pair:
		if v.Empty {
			return "[]"
		}
		return "pair(" + displayValue(v.Head) + ",...)"
	case *fas.Binding:
		if v.Empty {
			return "{}"
		}
		return "binding(" + displayValue(v.Key) + ",...)"
	case *fas.Vector:
		if v.HasType {
			return fmt.Sprintf("vector(type=%d,len=%d)", v.Type, len(v.Children)-1)
		}
		return fmt.Sprintf("vector(len=%d)", len(v.Children))
	case *fas.Statement:
		return fmt.Sprintf("statement(type=%d,span=%d:%d)", v.TypeInfo, v.Start, v.End)
	case fas.SecondActor:
		return "second-actor"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func printable(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for _, b := range data {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func previewHex(data []byte) string {
	const max = 24
	if len(data) <= max {
		return hex.EncodeToString(data)
	}
	return hex.EncodeToString(data[:max]) + "..."
}

func codeDisplay(code [4]byte) string {
	if printable(code[:]) {
		return strconv.Quote(string(code[:]))
	}
	return "0x" + hex.EncodeToString(code[:])
}

func decodeUTF16BE(data []byte) string {
	if len(data)%2 != 0 {
		return string(data)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = uint16(data[i*2])<<8 | uint16(data[i*2+1])
	}
	return string(utf16.Decode(units))
}
