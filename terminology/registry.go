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
	Name      string
	Commands  map[EventCode]Command
	Terms     map[Code4]string
	Enums     map[Code4]string
	Constants map[EventCode]string
}

type Registry struct {
	dictionaries map[string]*Dictionary
	commands     map[EventCode]Command
	terms        map[Code4]string
	enums        map[Code4]string
	constants    map[EventCode]string
	language     *Dictionary
	parameters   map[Code4]string
}

func New() *Registry {
	return &Registry{
		dictionaries: make(map[string]*Dictionary),
		commands:     make(map[EventCode]Command),
		terms:        make(map[Code4]string),
		enums:        make(map[Code4]string),
		constants:    make(map[EventCode]string),
		parameters:   make(map[Code4]string),
	}
}

func (r *Registry) Dictionary(name string) (*Dictionary, bool) {
	value, ok := r.dictionaries[name]
	return value, ok
}

func (r *Registry) Command(code EventCode) (Command, bool) {
	value, ok := r.commands[code]
	if !ok {
		if canonical, alias := compatibilityEventAliases[code]; alias {
			value, ok = r.commands[canonical]
		}
	}
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

func (r *Registry) Constant(code EventCode) (string, bool) {
	value, ok := r.constants[code]
	return value, ok
}

func (r *Registry) LanguageTerm(code Code4) (string, bool) {
	if r.language == nil {
		return "", false
	}
	value, ok := r.language.Terms[code]
	return value, ok
}

func (r *Registry) LanguageEnumeration(code Code4) (string, bool) {
	if r.language == nil {
		return "", false
	}
	value, ok := r.language.Enums[code]
	return value, ok
}

// Parameter returns a context-free language parameter name only when every
// AppleScript command using code gives it the same name. Ambiguous codes must
// be resolved through the specific Command instead.
func (r *Registry) Parameter(code Code4) (string, bool) {
	value, ok := r.parameters[code]
	return value, ok
}

func (r *Registry) addLanguage(dictionary *Dictionary) {
	r.language = dictionary
	ambiguous := make(map[Code4]bool)
	for _, command := range dictionary.Commands {
		for code, parameter := range command.Parameters {
			if name, exists := r.parameters[code]; exists && name != parameter.Name {
				delete(r.parameters, code)
				ambiguous[code] = true
			} else if !ambiguous[code] {
				r.parameters[code] = parameter.Name
			}
		}
	}
	r.Add(dictionary)
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
	d := &Dictionary{
		Name: name, Commands: make(map[EventCode]Command), Terms: make(map[Code4]string),
		Enums: make(map[Code4]string), Constants: make(map[EventCode]string),
	}
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
		case "enumeration":
			groupText := node.attr("code")
			if groupText != "" {
				for _, child := range node.Nodes {
					memberText, enumName := child.attr("code"), child.attr("name")
					if child.XMLName.Local != "enumerator" || memberText == "" || enumName == "" {
						continue
					}
					code, err := ParseEventCode(groupText + memberText)
					if err == nil {
						d.Constants[code] = enumName
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
	for code, name := range dictionary.Constants {
		if _, exists := r.constants[code]; !exists {
			r.constants[code] = name
		}
	}
}

//go:embed data/*.sdef
var bundled embed.FS

//go:embed language.json
var bundledLanguage []byte

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
		language, err := ParseLanguageDefinition(bundledLanguage)
		if err != nil {
			defaultError = err
			return
		}
		defaultRegistry.addLanguage(language)
	})
	return defaultRegistry, defaultError
}

var compatibilityEventAliases = func() map[EventCode]EventCode {
	aliases := make(map[EventCode]EventCode)
	for alias, canonical := range map[string]string{
		"CoRedelo": "coredelo",
		"CoRedoex": "coredoex",
	} {
		aliasCode, aliasErr := ParseEventCode(alias)
		canonicalCode, canonicalErr := ParseEventCode(canonical)
		if aliasErr == nil && canonicalErr == nil {
			aliases[aliasCode] = canonicalCode
		}
	}
	return aliases
}()
