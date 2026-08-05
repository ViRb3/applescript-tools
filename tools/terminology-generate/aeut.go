package main

import (
	"encoding/binary"
	"fmt"
	"sort"

	"applescript-tools/internal/macroman"
	"applescript-tools/terminology"
)

type aeutReader struct {
	data   []byte
	offset int
	err    error
}

func (r *aeutReader) bytes(size int) []byte {
	if r.err != nil {
		return nil
	}
	if size < 0 || r.offset+size > len(r.data) {
		r.err = fmt.Errorf("truncated system terminology at byte %d", r.offset)
		return nil
	}
	value := r.data[r.offset : r.offset+size]
	r.offset += size
	return value
}

func (r *aeutReader) byte() byte {
	value := r.bytes(1)
	if len(value) == 0 {
		return 0
	}
	return value[0]
}

func (r *aeutReader) uint16() uint16 {
	value := r.bytes(2)
	if len(value) != 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(value)
}

func (r *aeutReader) code() string {
	value := r.bytes(4)
	if len(value) != 4 {
		return ""
	}
	numeric := binary.LittleEndian.Uint32(value)
	var code [4]byte
	binary.BigEndian.PutUint32(code[:], numeric)
	return macroman.Decode(code[:])
}

func (r *aeutReader) pstring() string {
	return macroman.Decode(r.bytes(int(r.byte())))
}

func (r *aeutReader) align() {
	if r.offset&1 != 0 {
		r.bytes(1)
	}
}

func parseSystemTerminology(data []byte) (*terminology.LanguageDefinition, error) {
	r := &aeutReader{data: data}
	major, minor := r.byte(), r.byte()
	language, script := r.uint16(), r.uint16()
	if major != 1 || minor != 0 || language != 0 || script != 0 {
		return nil, fmt.Errorf("unsupported system terminology header %d.%d language=%d script=%d", major, minor, language, script)
	}
	definition := &terminology.LanguageDefinition{
		SchemaVersion: terminology.LanguageSchemaVersion,
		Source:        "OSAGetSysTerminology",
	}
	commands := make(map[string]terminology.LanguageCommand)
	terms := make(map[string]string)
	enumerations := make(map[string]terminology.LanguageTerm)
	for range int(r.uint16()) {
		r.pstring() // suite name
		r.pstring() // suite description
		r.align()
		r.code()   // suite code
		r.uint16() // suite level
		r.uint16() // suite version
		for range int(r.uint16()) {
			name := r.pstring()
			r.pstring() // description
			r.align()
			eventClass, eventID := r.code(), r.code()
			r.code()    // reply type
			r.pstring() // reply description
			r.align()
			r.uint16() // reply flags
			directType := r.code()
			r.pstring() // direct parameter description
			r.align()
			r.uint16() // direct parameter flags
			command := terminology.LanguageCommand{
				Code: eventClass + eventID, Name: name, HasDirectParameter: directType != "null",
			}
			for range int(r.uint16()) {
				parameterName := r.pstring()
				r.align()
				parameterCode, parameterType := r.code(), r.code()
				r.pstring() // parameter description
				r.align()
				r.uint16() // parameter flags
				command.Parameters = append(command.Parameters, terminology.LanguageParameter{
					Code: parameterCode, Name: parameterName, Type: parameterType,
				})
			}
			if _, exists := commands[command.Code]; !exists {
				commands[command.Code] = command
			}
		}
		for range int(r.uint16()) {
			name := r.pstring()
			r.align()
			code := r.code()
			if _, exists := terms[code]; !exists {
				terms[code] = name
			}
			r.pstring() // class description
			r.align()
			for range int(r.uint16()) {
				propertyName := r.pstring()
				r.align()
				propertyCode := r.code()
				if _, exists := terms[propertyCode]; !exists {
					terms[propertyCode] = propertyName
				}
				r.code()    // property type
				r.pstring() // property description
				r.align()
				r.uint16() // property flags
			}
			for range int(r.uint16()) {
				r.code() // element class
				for range int(r.uint16()) {
					r.code() // key form
				}
			}
		}
		for range int(r.uint16()) {
			r.pstring() // comparison name
			r.align()
			r.code()
			r.pstring() // comparison description
			r.align()
		}
		for range int(r.uint16()) {
			group := r.code()
			for range int(r.uint16()) {
				name := r.pstring()
				r.align()
				code := r.code()
				key := group + code
				if _, exists := enumerations[key]; !exists {
					enumerations[key] = terminology.LanguageTerm{Group: group, Code: code, Name: name}
				}
				r.pstring() // enumerator description
				r.align()
			}
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.offset != len(data) {
		return nil, fmt.Errorf("system terminology has %d unparsed bytes", len(data)-r.offset)
	}
	commandCodes := sortedKeys(commands)
	for _, code := range commandCodes {
		definition.Commands = append(definition.Commands, commands[code])
	}
	for _, code := range sortedKeys(terms) {
		definition.Terms = append(definition.Terms, terminology.LanguageTerm{Code: code, Name: terms[code]})
	}
	for _, code := range sortedKeys(enumerations) {
		definition.Enumerations = append(definition.Enumerations, enumerations[code])
	}
	return definition, nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
