package terminology

import (
	"embed"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"applescript-tools/internal/macroman"
)

type Code4 [4]byte
type EventCode [8]byte

func ParseCode4(value string) (Code4, error) {
	var out Code4
	raw, err := parseCode(value, 4)
	if err != nil {
		return out, err
	}
	copy(out[:], raw)
	return out, nil
}

func ParseEventCode(value string) (EventCode, error) {
	var out EventCode
	raw, err := parseCode(value, 8)
	if err != nil {
		return out, err
	}
	copy(out[:], raw)
	return out, nil
}

func parseCode(value string, width int) ([]byte, error) {
	if strings.HasPrefix(value, "0x") {
		raw, err := hex.DecodeString(value[2:])
		if err != nil || len(raw) != width {
			return nil, fmt.Errorf("invalid %d-byte hexadecimal code %q", width, value)
		}
		return raw, nil
	}
	raw := make([]byte, 0, width)
	for _, character := range value {
		encoded, ok := macroman.EncodeRune(character)
		if !ok {
			return nil, fmt.Errorf("code %q contains a character unavailable in MacRoman", value)
		}
		raw = append(raw, encoded)
	}
	if len(raw) != width {
		return nil, fmt.Errorf("invalid %d-byte code %q", width, value)
	}
	return raw, nil
}

type Parameter struct {
	Code Code4
	Name string
	Type string
}

type Command struct {
	Code               EventCode
	Name               string
	Parameters         map[Code4]Parameter
	ParameterOrder     []Code4
	HasDirectParameter bool
}

type Dictionary struct {
	Name     string
	Commands map[EventCode]Command
	Terms    map[Code4]string
	Enums    map[Code4]string
}

type Registry struct {
	dictionaries map[string]*Dictionary
	commands     map[EventCode]Command
	terms        map[Code4]string
	enums        map[Code4]string
}

func New() *Registry {
	return &Registry{
		dictionaries: make(map[string]*Dictionary),
		commands:     make(map[EventCode]Command),
		terms:        make(map[Code4]string),
		enums:        make(map[Code4]string),
	}
}

func (r *Registry) Dictionary(name string) (*Dictionary, bool) {
	value, ok := r.dictionaries[name]
	return value, ok
}

func (r *Registry) Command(code EventCode) (Command, bool) {
	value, ok := r.commands[code]
	return value, ok
}

func (r *Registry) Term(code Code4) (string, bool) {
	value, ok := r.terms[code]
	return value, ok
}

func (r *Registry) Enumeration(code Code4) (string, bool) {
	value, ok := r.enums[code]
	return value, ok
}

type xmlNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xmlNode  `xml:",any"`
}

func (n xmlNode) attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func ParseSDEF(name string, input io.Reader) (*Dictionary, error) {
	var root xmlNode
	if err := xml.NewDecoder(input).Decode(&root); err != nil {
		return nil, fmt.Errorf("parse %s SDEF: %w", name, err)
	}
	d := &Dictionary{Name: name, Commands: make(map[EventCode]Command), Terms: make(map[Code4]string), Enums: make(map[Code4]string)}
	var walk func(xmlNode) error
	walk = func(node xmlNode) error {
		switch node.XMLName.Local {
		case "command":
			codeText := node.attr("code")
			if codeText != "" {
				code, err := ParseEventCode(codeText)
				if err != nil {
					return err
				}
				command := Command{Code: code, Name: node.attr("name"), Parameters: make(map[Code4]Parameter)}
				for _, child := range node.Nodes {
					switch child.XMLName.Local {
					case "direct-parameter":
						command.HasDirectParameter = true
					case "parameter":
						codeText, parameterName := child.attr("code"), child.attr("name")
						if codeText == "" || parameterName == "" {
							continue
						}
						parameterCode, err := ParseCode4(codeText)
						if err != nil {
							return err
						}
						command.Parameters[parameterCode] = Parameter{Code: parameterCode, Name: parameterName, Type: child.attr("type")}
						command.ParameterOrder = append(command.ParameterOrder, parameterCode)
					}
				}
				d.Commands[code] = command
			}
		case "class", "class-extension", "property", "type":
			codeText, termName := node.attr("code"), node.attr("name")
			if codeText != "" && termName != "" {
				code, err := ParseCode4(codeText)
				if err == nil {
					if _, exists := d.Terms[code]; !exists {
						d.Terms[code] = termName
					}
				}
			}
		case "enumerator":
			codeText, enumName := node.attr("code"), node.attr("name")
			if codeText != "" && enumName != "" {
				code, err := ParseCode4(codeText)
				if err == nil {
					if _, exists := d.Enums[code]; !exists {
						d.Enums[code] = enumName
					}
				}
			}
		}
		for _, child := range node.Nodes {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, fmt.Errorf("parse %s terminology: %w", name, err)
	}
	return d, nil
}

func (r *Registry) Add(dictionary *Dictionary) {
	r.dictionaries[dictionary.Name] = dictionary
	for code, command := range dictionary.Commands {
		if _, exists := r.commands[code]; !exists {
			r.commands[code] = command
		}
	}
	for code, name := range dictionary.Terms {
		if _, exists := r.terms[code]; !exists {
			r.terms[code] = name
		}
	}
	for code, name := range dictionary.Enums {
		if _, exists := r.enums[code]; !exists {
			r.enums[code] = name
		}
	}
}

//go:embed data/*.sdef
var bundled embed.FS

var (
	defaultOnce     sync.Once
	defaultRegistry *Registry
	defaultError    error
)

func Default() (*Registry, error) {
	defaultOnce.Do(func() {
		defaultRegistry = New()
		entries, err := fs.Glob(bundled, "data/*.sdef")
		if err != nil {
			defaultError = err
			return
		}
		sort.Strings(entries)
		for _, path := range entries {
			f, err := bundled.Open(path)
			if err != nil {
				defaultError = err
				return
			}
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			dictionary, parseErr := ParseSDEF(name, f)
			_ = f.Close()
			if parseErr != nil {
				defaultError = parseErr
				return
			}
			defaultRegistry.Add(dictionary)
		}
		if additions, ok := defaultRegistry.dictionaries["StandardAdditions"]; ok {
			defaultRegistry.dictionaries["Standard Additions"] = additions
		}
		addBuiltins(defaultRegistry)
	})
	return defaultRegistry, defaultError
}

func addBuiltins(registry *Registry) {
	for codeText, name := range map[string]string{
		"ascrcmnt": "log",
		"ascrnoop": "launch",
		"aevtoapp": "run",
		"CoRedelo": "delete",
		"CoRedoex": "exists",
	} {
		code, err := ParseEventCode(codeText)
		if err != nil {
			continue
		}
		if _, exists := registry.commands[code]; !exists {
			registry.commands[code] = Command{Code: code, Name: name, Parameters: map[Code4]Parameter{}}
		}
	}
	for codeText, name := range map[string]string{
		"leng": "length", "bool": "boolean", "long": "integer", "doub": "real", "TEXT": "string", "ctxt": "text",
		"cobj": "item", "pcnt": "contents", "pnam": "name", "psxf": "POSIX file", "scpt": "script",
		"citm": "text item", "obj ": "reference", "rvse": "reverse", "utxt": "Unicode text",
		"null": "null", "insl": "location reference", "qdrt": "bounding rectangle",
		"prdt": "with properties", "alrp": "replacing", "kocl": "new", "insh": "at",
		"errn": "number", "ptlr": "partial result", "erob": "from", "errt": "to", "from": "from",
		"prun": "running", "msng": "missing value", "rtyp": "as",
		"txdl": "text item delimiters", "ascr": "AppleScript",
		"fltp": "as", "kfil": "in", "ldt ": "date",
		"wkdy": "weekday", "mnth": "month", "day ": "day", "year": "year",
		"hour": "hours", "min ": "minutes", "scnd": "seconds", "days": "days",
		"jan ": "January", "feb ": "February", "mar ": "March", "apr ": "April",
		"may ": "May", "jun ": "June", "jul ": "July", "aug ": "August",
		"sep ": "September", "oct ": "October", "nov ": "November", "dec ": "December",
		"sun ": "Sunday", "mon ": "Monday", "tue ": "Tuesday", "wed ": "Wednesday",
		"thu ": "Thursday", "fri ": "Friday", "sat ": "Saturday",
		"FTPc": "path", "lnfd": "linefeed", "rslt": "result", "spac": "space", "strq": "quoted form",
		"fixd": "fixed", "scrƒ": "scripts folder",
		"asup": "application support",
		"ID  ": "id", "tab ": "tab", "ret ": "return",
		"case": "case", "diac": "diacriticals", "whit": "white space",
		"hyph": "hyphens", "expa": "expansion", "punc": "punctuation",
		"nume": "numeric strings",
	} {
		code, err := ParseCode4(codeText)
		if err != nil {
			continue
		}
		// AppleScript language terms are authoritative when an application
		// dictionary reuses the same four-byte code (for example Mail's rtyp).
		registry.terms[code] = name
	}
}
