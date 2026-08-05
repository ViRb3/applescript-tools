// Command terminology-generate derives the bundled scripting dictionaries and
// their provenance from the applications listed in terminology/sources.json.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"applescript-tools/terminology"
)

type manifest struct {
	SchemaVersion int           `json:"schema_version"`
	Entries       []sourceEntry `json:"entries"`
}

type sourceEntry struct {
	Name     string   `json:"name"`
	BundleID string   `json:"bundle_id"`
	Paths    []string `json:"paths"`
}

type report struct {
	SchemaVersion int           `json:"schema_version"`
	Host          hostInfo      `json:"host"`
	Entries       []reportEntry `json:"entries"`
}

type hostInfo struct {
	ProductVersion string `json:"product_version"`
	BuildVersion   string `json:"build_version"`
	Architecture   string `json:"architecture"`
	SdefPath       string `json:"sdef_path"`
}

type reportEntry struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	BundleID       string `json:"bundle_id"`
	BundleVersion  string `json:"bundle_version"`
	SHA256         string `json:"sha256"`
	SemanticSHA256 string `json:"semantic_sha256"`
}

type generatedSnapshot struct {
	name string
	data []byte
}

func main() {
	manifestPath := flag.String("manifest", "terminology/sources.json", "terminology source manifest")
	outputDir := flag.String("output-dir", "terminology/data", "generated SDEF directory")
	reportPath := flag.String("provenance", "terminology/provenance.json", "generated JSON provenance path")
	flag.Parse()

	result, err := generate(*manifestPath, *outputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*reportPath, encoded, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("generated %d SDEFs in %s and provenance in %s\n", len(result.Entries), *outputDir, *reportPath)
}

func generate(manifestPath, outputDir string) (*report, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read source manifest: %w", err)
	}
	var sources manifest
	if err := json.Unmarshal(raw, &sources); err != nil {
		return nil, fmt.Errorf("parse source manifest: %w", err)
	}
	if sources.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported source manifest version %d", sources.SchemaVersion)
	}
	sdefPath, err := exec.LookPath("sdef")
	if err != nil {
		return nil, fmt.Errorf("find sdef: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	result := &report{SchemaVersion: 2, Host: hostInfo{
		ProductVersion: commandText("sw_vers", "-productVersion"),
		BuildVersion:   commandText("sw_vers", "-buildVersion"),
		Architecture:   runtime.GOARCH,
		SdefPath:       sdefPath,
	}}
	generated := make([]generatedSnapshot, 0, len(sources.Entries))
	seenNames := make(map[string]bool)
	for _, source := range sources.Entries {
		if source.Name == "" || source.BundleID == "" || len(source.Paths) == 0 {
			return nil, fmt.Errorf("incomplete source entry for %q", source.Name)
		}
		if seenNames[source.Name] {
			return nil, fmt.Errorf("duplicate source name %q", source.Name)
		}
		seenNames[source.Name] = true
		entry := reportEntry{Name: source.Name}
		entry.Path = firstExisting(source.Paths)
		if entry.Path == "" {
			return nil, fmt.Errorf("%s: none of the configured bundle paths exists", source.Name)
		}
		infoPath := filepath.Join(entry.Path, "Contents", "Info.plist")
		entry.BundleID = plistValue(infoPath, "CFBundleIdentifier")
		entry.BundleVersion = plistValue(infoPath, "CFBundleShortVersionString")
		if entry.BundleVersion == "" {
			entry.BundleVersion = plistValue(infoPath, "CFBundleVersion")
		}
		if entry.BundleID != source.BundleID {
			return nil, fmt.Errorf("%s: bundle identifier %q, want %q", source.Name, entry.BundleID, source.BundleID)
		}
		if entry.BundleVersion == "" {
			return nil, fmt.Errorf("%s: bundle has no version", source.Name)
		}
		extracted, extractionErr := exec.Command(sdefPath, entry.Path).Output()
		if extractionErr != nil || !isXML(extracted) {
			return nil, fmt.Errorf("%s: %s", source.Name, extractionMessage(sdefPath, entry.Path, extractionErr, extracted))
		}
		entry.SHA256 = digest(extracted)
		entry.SemanticSHA256, err = semanticDigest(source.Name, extracted)
		if err != nil {
			return nil, fmt.Errorf("parse generated %s SDEF: %w", source.Name, err)
		}
		generated = append(generated, generatedSnapshot{name: source.Name, data: extracted})
		result.Entries = append(result.Entries, entry)
	}
	for _, snapshot := range generated {
		path := filepath.Join(outputDir, snapshot.name+".sdef")
		if err := os.WriteFile(path, snapshot.data, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	}
	if err := removeUnlistedSnapshots(outputDir, seenNames); err != nil {
		return nil, err
	}
	return result, nil
}

func removeUnlistedSnapshots(outputDir string, generatedNames map[string]bool) error {
	paths, err := filepath.Glob(filepath.Join(outputDir, "*.sdef"))
	if err != nil {
		return fmt.Errorf("list generated SDEF directory: %w", err)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".sdef")
		if generatedNames[name] {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove unlisted SDEF %s: %w", path, err)
		}
	}
	return nil
}

func firstExisting(paths []string) string {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func plistValue(path, key string) string {
	return commandText("/usr/libexec/PlistBuddy", "-c", "Print :"+key, path)
}

func commandText(name string, args ...string) string {
	output, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type canonicalDictionary struct {
	Commands []canonicalCommand `json:"commands"`
	Terms    []canonicalTerm    `json:"terms"`
	Enums    []canonicalTerm    `json:"enums"`
}

type canonicalCommand struct {
	Code       string               `json:"code"`
	Name       string               `json:"name"`
	Direct     bool                 `json:"direct"`
	Parameters []canonicalParameter `json:"parameters"`
}

type canonicalParameter struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type canonicalTerm struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func semanticDigest(name string, data []byte) (string, error) {
	dictionary, err := terminology.ParseSDEF(name, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	canonical := canonicalDictionary{}
	commandCodes := make([]terminology.EventCode, 0, len(dictionary.Commands))
	for code := range dictionary.Commands {
		commandCodes = append(commandCodes, code)
	}
	sort.Slice(commandCodes, func(i, j int) bool {
		return bytes.Compare(commandCodes[i][:], commandCodes[j][:]) < 0
	})
	for _, code := range commandCodes {
		command := dictionary.Commands[code]
		item := canonicalCommand{Code: hex.EncodeToString(code[:]), Name: command.Name, Direct: command.HasDirectParameter}
		seen := make(map[terminology.Code4]bool)
		for _, parameterCode := range command.ParameterOrder {
			parameter, ok := command.Parameters[parameterCode]
			if !ok || seen[parameterCode] {
				continue
			}
			seen[parameterCode] = true
			item.Parameters = append(item.Parameters, canonicalParameter{
				Code: hex.EncodeToString(parameterCode[:]), Name: parameter.Name, Type: parameter.Type,
			})
		}
		var remaining []terminology.Code4
		for parameterCode := range command.Parameters {
			if !seen[parameterCode] {
				remaining = append(remaining, parameterCode)
			}
		}
		sort.Slice(remaining, func(i, j int) bool { return bytes.Compare(remaining[i][:], remaining[j][:]) < 0 })
		for _, parameterCode := range remaining {
			parameter := command.Parameters[parameterCode]
			item.Parameters = append(item.Parameters, canonicalParameter{
				Code: hex.EncodeToString(parameterCode[:]), Name: parameter.Name, Type: parameter.Type,
			})
		}
		canonical.Commands = append(canonical.Commands, item)
	}
	canonical.Terms = canonicalTerms(dictionary.Terms)
	canonical.Enums = canonicalTerms(dictionary.Enums)
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return digest(raw), nil
}

func canonicalTerms(values map[terminology.Code4]string) []canonicalTerm {
	codes := make([]terminology.Code4, 0, len(values))
	for code := range values {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool { return bytes.Compare(codes[i][:], codes[j][:]) < 0 })
	terms := make([]canonicalTerm, 0, len(codes))
	for _, code := range codes {
		terms = append(terms, canonicalTerm{Code: hex.EncodeToString(code[:]), Name: values[code]})
	}
	return terms
}

func isXML(data []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name.Local == "dictionary"
		}
	}
}

func extractionMessage(sdefPath, path string, original error, stdout []byte) string {
	command := exec.Command(sdefPath, path)
	output, err := command.CombinedOutput()
	if len(output) != 0 {
		return strings.TrimSpace(string(output))
	}
	if err != nil {
		return err.Error()
	}
	if original != nil {
		return original.Error()
	}
	if len(stdout) == 0 {
		return "sdef produced no XML output"
	}
	return "sdef output was not a dictionary XML document"
}
