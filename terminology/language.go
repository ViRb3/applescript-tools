package terminology

import (
	"encoding/json"
	"fmt"
)

const LanguageSchemaVersion = 1

// LanguageDefinition is generated from AppleScript's system terminology.
// Terms, enumerations, and command parameters remain distinct because the
// same four-byte code can legitimately have different names in each context.
type LanguageDefinition struct {
	SchemaVersion int               `json:"schema_version"`
	Source        string            `json:"source"`
	Commands      []LanguageCommand `json:"commands"`
	Terms         []LanguageTerm    `json:"terms"`
	Enumerations  []LanguageTerm    `json:"enumerations"`
}

type LanguageCommand struct {
	Code               string              `json:"code"`
	Name               string              `json:"name"`
	HasDirectParameter bool                `json:"has_direct_parameter,omitempty"`
	Parameters         []LanguageParameter `json:"parameters,omitempty"`
}

type LanguageParameter struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type LanguageTerm struct {
	Group string `json:"group,omitempty"`
	Code  string `json:"code"`
	Name  string `json:"name"`
}

func ParseLanguageDefinition(data []byte) (*Dictionary, error) {
	var definition LanguageDefinition
	if err := json.Unmarshal(data, &definition); err != nil {
		return nil, fmt.Errorf("parse AppleScript language terminology: %w", err)
	}
	if definition.SchemaVersion != LanguageSchemaVersion {
		return nil, fmt.Errorf("unsupported AppleScript language terminology version %d", definition.SchemaVersion)
	}
	dictionary := &Dictionary{
		Name:      "AppleScript Language",
		Commands:  make(map[EventCode]Command),
		Terms:     make(map[Code4]string),
		Enums:     make(map[Code4]string),
		Constants: make(map[EventCode]string),
	}
	for _, item := range definition.Commands {
		code, err := ParseEventCode(item.Code)
		if err != nil {
			return nil, fmt.Errorf("language command %q: %w", item.Name, err)
		}
		command := Command{
			Code: code, Name: item.Name, HasDirectParameter: item.HasDirectParameter,
			Parameters: make(map[Code4]Parameter),
		}
		for _, item := range item.Parameters {
			parameterCode, err := ParseCode4(item.Code)
			if err != nil {
				return nil, fmt.Errorf("language command %q parameter %q: %w", command.Name, item.Name, err)
			}
			command.Parameters[parameterCode] = Parameter{Code: parameterCode, Name: item.Name, Type: item.Type}
			command.ParameterOrder = append(command.ParameterOrder, parameterCode)
		}
		dictionary.Commands[code] = command
	}
	for _, item := range definition.Terms {
		code, err := ParseCode4(item.Code)
		if err != nil {
			return nil, fmt.Errorf("language term %q: %w", item.Name, err)
		}
		dictionary.Terms[code] = item.Name
	}
	for _, item := range definition.Enumerations {
		code, err := ParseCode4(item.Code)
		if err != nil {
			return nil, fmt.Errorf("language enumeration %q: %w", item.Name, err)
		}
		if _, exists := dictionary.Enums[code]; !exists {
			dictionary.Enums[code] = item.Name
		}
		if item.Group != "" {
			constant, err := ParseEventCode(item.Group + item.Code)
			if err != nil {
				return nil, fmt.Errorf("language enumeration %q group %q: %w", item.Name, item.Group, err)
			}
			dictionary.Constants[constant] = item.Name
		}
	}
	return dictionary, nil
}
