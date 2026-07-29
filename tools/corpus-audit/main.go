// Command corpus-audit validates a source corpus against a macOS AppleScript
// compiler oracle. It is an opt-in maintenance tool and is never run by tests.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	applescript "applescript-tools"
	"applescript-tools/internal/macroman"
)

var semanticOpcodes = map[string]bool{
	"TestIf": true, "Jump": true, "And": true, "Or": true,
	"LinkRepeat": true, "RepeatNTimes": true, "RepeatWhile": true,
	"RepeatUntil": true, "RepeatInCollection": true, "RepeatInRange": true,
	"Exit": true, "ErrorHandler": true, "EndErrorHandler": true,
	"HandleError": true, "BeginTimeout": true, "EndTimeout": true,
	"Return": true, "MessageSend": true, "PositionalMessageSend": true,
	"Error": true, "Add": true, "Subtract": true, "Multiply": true,
	"Divide": true, "Quotient": true, "Remainder": true, "Power": true,
	"Equal": true, "NotEqual": true, "LessThan": true,
	"LessThanOrEqual": true, "GreaterThan": true,
	"GreaterThanOrEqual": true, "Contains": true, "StartsWith": true,
	"EndsWith": true, "Negate": true, "Not": true, "MakeComp": true,
}

type result struct {
	path              string
	hash              string
	lines             int
	status            string
	checks            []check
	details           []string
	originalOpcodes   int
	recompiledOpcodes int
}

type check struct {
	name string
	pass bool
}

type oracle struct {
	directory  string
	executable string
	temporary  bool
}

func main() {
	oracleDirectory := flag.String("oracle", defaultOracleDirectory(), "compiler oracle work directory")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: corpus-audit [flags] <corpus-directory> <report.md>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	corpus, err := filepath.Abs(flag.Arg(0))
	must(err)
	report, err := filepath.Abs(flag.Arg(1))
	must(err)
	compiler, err := resolveOracle(*oracleDirectory)
	must(err)
	defer compiler.cleanup()
	sources, err := sourceFiles(corpus)
	must(err)
	results := make([]result, 0, len(sources))
	for index, path := range sources {
		item := audit(context.Background(), compiler, corpus, path, index+1)
		results = append(results, item)
		fmt.Printf("[%d/%d] %-18s %s\n", index+1, len(sources), item.status, item.path)
	}
	must(os.WriteFile(report, []byte(markdown(corpus, results)), 0o644))
	fmt.Printf("wrote %s\n", report)
}

func defaultOracleDirectory() string {
	return os.Getenv("APPLESCRIPT_COMPILER_ORACLE_DIR")
}

func resolveOracle(directory string) (*oracle, error) {
	if directory != "" {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf(
				"compiler oracle directory is unavailable: %s",
				directory,
			)
		}
		return &oracle{directory: directory}, nil
	}
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf(
			"set -oracle or APPLESCRIPT_COMPILER_ORACLE_DIR",
		)
	}
	executable, err := exec.LookPath("osacompile")
	if err != nil {
		return nil, fmt.Errorf("find osacompile: %w", err)
	}
	directory, err = os.MkdirTemp("", "applescript-corpus-audit-")
	if err != nil {
		return nil, err
	}
	return &oracle{
		directory:  directory,
		executable: executable,
		temporary:  true,
	}, nil
}

func (o *oracle) cleanup() {
	if o.temporary {
		_ = os.RemoveAll(o.directory)
	}
}

func sourceFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".applescript") {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func audit(ctx context.Context, compiler *oracle, root, path string, index int) result {
	data, err := os.ReadFile(path)
	if err != nil {
		return result{path: relative(root, path), status: "source-failed", details: []string{err.Error()}}
	}
	source, encoding := decodeSource(data)
	item := result{
		path: relative(root, path), hash: fmt.Sprintf("%x", sha256.Sum256(data)),
		lines: sourceLineCount(source), status: "pending",
	}
	if encoding != "UTF-8" {
		item.details = append(item.details, "Source encoding: "+encoding+".")
	}

	original, err := compiler.compile(ctx, source, fmt.Sprintf("corpus_%03d_original", index))
	item.addCheck("Original compilation", err == nil)
	if err != nil {
		item.status = "compiler-blocked"
		item.details = append(item.details, cleanDetail(err.Error()))
		return item
	}

	first, err := decompile(ctx, original)
	item.addCheck("Clean decompilation", err == nil && diagnosticsClean(first.diagnostics))
	if err != nil {
		item.status = "decompiler-failed"
		item.details = append(item.details, cleanDetail(err.Error()))
		return item
	}
	if !diagnosticsClean(first.diagnostics) {
		item.details = append(item.details, formatDiagnostics("First decompilation", first.diagnostics))
	}

	recompiled, err := compiler.compile(ctx, first.source, fmt.Sprintf("corpus_%03d_recompiled", index))
	item.addCheck("Decompiled source recompiles", err == nil)
	if err != nil {
		item.status = "recompile-failed"
		item.details = append(item.details, cleanDetail(err.Error()))
		if excerpt := compilerErrorContext(first.source, err.Error()); excerpt != "" {
			item.details = append(item.details, excerpt)
		}
		return item
	}

	second, err := decompile(ctx, recompiled)
	item.addCheck("Clean second decompilation", err == nil && diagnosticsClean(second.diagnostics))
	if err != nil {
		item.status = "second-decompile-failed"
		item.details = append(item.details, cleanDetail(err.Error()))
		return item
	}
	if !diagnosticsClean(second.diagnostics) {
		item.details = append(item.details, formatDiagnostics("Second decompilation", second.diagnostics))
	}
	item.addCheck("Decompiler fixed point", first.source == second.source)

	originalSignature, originalCount, err := opcodeSignature(ctx, original)
	if err != nil {
		item.details = append(item.details, "Original disassembly: "+cleanDetail(err.Error()))
	}
	recompiledSignature, recompiledCount, secondErr := opcodeSignature(ctx, recompiled)
	if secondErr != nil {
		item.details = append(item.details, "Recompiled disassembly: "+cleanDetail(secondErr.Error()))
	}
	item.originalOpcodes = originalCount
	item.recompiledOpcodes = recompiledCount
	item.addCheck("Semantic opcode stream matches",
		err == nil && secondErr == nil && equalStrings(originalSignature, recompiledSignature))

	item.status = "pass"
	for _, check := range item.checks {
		if !check.pass {
			item.status = "review"
			break
		}
	}
	if first.source != second.source {
		item.details = append(item.details, "Successive decompilations did not reach a fixed point.")
		item.details = append(item.details, firstDifference(first.source, second.source))
	}
	if !equalStrings(originalSignature, recompiledSignature) {
		item.details = append(item.details, "The original and recompiled semantic opcode streams differ.")
		item.details = append(item.details, signatureDifference(originalSignature, recompiledSignature))
	}
	return item
}

type decompilation struct {
	source      string
	diagnostics []applescript.Diagnostic
}

func decompile(ctx context.Context, script []byte) (decompilation, error) {
	output, err := applescript.Decompile(ctx, bytes.NewReader(script), applescript.DecompileOptions{})
	if err != nil {
		return decompilation{}, err
	}
	return decompilation{source: output.Source, diagnostics: output.Diagnostics}, nil
}

func opcodeSignature(ctx context.Context, script []byte) ([]string, int, error) {
	output, err := applescript.Disassemble(ctx, bytes.NewReader(script), applescript.DisassembleOptions{})
	if err != nil {
		return nil, 0, err
	}
	var signature []string
	count := 0
	for _, function := range output.Functions {
		for _, instruction := range function.Instructions {
			count++
			signature = append(signature, instruction.Mnemonic)
		}
	}
	return semanticSignature(signature), count, nil
}

func semanticSignature(mnemonics []string) []string {
	var signature []string
	for _, mnemonic := range mnemonics {
		if semanticOpcodes[mnemonic] {
			signature = append(signature, mnemonic)
		}
	}
	return signature
}

func (o *oracle) compile(ctx context.Context, source, label string) ([]byte, error) {
	token := make([]byte, 10)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	stem := "go_audit_" + label + "_" + hex.EncodeToString(token)
	sourcePath := filepath.Join(o.directory, stem+".applescript")
	scriptPath := filepath.Join(o.directory, stem+".scpt")
	logPath := filepath.Join(o.directory, stem+".log")
	defer os.Remove(sourcePath)
	defer os.Remove(scriptPath)
	defer os.Remove(logPath)
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		return nil, err
	}
	if o.executable != "" {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		output, err := exec.CommandContext(
			ctx,
			o.executable,
			"-o",
			scriptPath,
			sourcePath,
		).CombinedOutput()
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf(
					"compiler timed out for %s: %w",
					label,
					ctx.Err(),
				)
			}
			return nil, fmt.Errorf("%s", output)
		}
		return os.ReadFile(scriptPath)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(logPath); err == nil {
			if script, err := os.ReadFile(scriptPath); err == nil {
				return script, nil
			}
			log, _ := os.ReadFile(logPath)
			return nil, fmt.Errorf("%s", log)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("compiler oracle timed out for %s", label)
}

func markdown(corpus string, results []result) string {
	counts := make(map[string]int)
	for _, item := range results {
		counts[item.status]++
	}
	var b strings.Builder
	b.WriteString("# AppleScripts corpus validation checklist\n\n")
	fmt.Fprintf(&b, "Corpus: `%s`\n", corpusIdentity(corpus))
	fmt.Fprintf(&b, "Corpus revision: `%s`\n", corpusRevision(corpus))
	fmt.Fprintf(&b, "Entries audited: **%d**\n\n", len(results))
	b.WriteString("A passing entry compiled through the macOS oracle, decompiled without diagnostics, recompiled, reached a source fixed point, and retained the same semantic control-flow, command, and computation opcode stream. Assignment opcodes are intentionally excluded because the compiler can fold script-property assignments into binding-table initializers.\n\n")
	b.WriteString("## Summary\n\n")
	for _, status := range []string{"pass", "review", "compiler-blocked", "decompiler-failed", "recompile-failed", "second-decompile-failed", "source-failed"} {
		fmt.Fprintf(&b, "- `%s`: %d\n", status, counts[status])
	}
	for _, item := range results {
		fmt.Fprintf(&b, "\n## `%s`\n\n", item.path)
		fmt.Fprintf(&b, "- Overall: **%s**\n", item.status)
		fmt.Fprintf(&b, "- Source: %d lines; SHA-256 `%s`\n", item.lines, item.hash)
		for _, check := range item.checks {
			mark := " "
			if check.pass {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, check.name)
		}
		if item.originalOpcodes != 0 || item.recompiledOpcodes != 0 {
			fmt.Fprintf(&b, "- Opcodes: %d original / %d recompiled\n", item.originalOpcodes, item.recompiledOpcodes)
		}
		if len(item.details) != 0 {
			b.WriteString("- Notes:\n")
			for _, detail := range item.details {
				b.WriteString("\n  ```text\n")
				for line := range strings.SplitSeq(detail, "\n") {
					b.WriteString("  " + line + "\n")
				}
				b.WriteString("  ```\n")
			}
		}
	}
	return b.String()
}

func (r *result) addCheck(name string, pass bool) {
	r.checks = append(r.checks, check{name: name, pass: pass})
}

func decodeSource(data []byte) (string, string) {
	if utf8.Valid(data) {
		return string(data), "UTF-8"
	}
	return macroman.Decode(data), "MacRoman"
}

func sourceLineCount(source string) int {
	if source == "" {
		return 0
	}
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	count := strings.Count(normalized, "\n")
	if !strings.HasSuffix(normalized, "\n") {
		count++
	}
	return count
}

var compilerLinePattern = regexp.MustCompile(`:(\d+): error`)

func compilerErrorContext(source, diagnostic string) string {
	match := compilerLinePattern.FindStringSubmatch(diagnostic)
	if len(match) != 2 {
		return ""
	}
	var line int
	if _, err := fmt.Sscanf(match[1], "%d", &line); err != nil {
		return ""
	}
	lines := strings.Split(source, "\n")
	start, end := line-3, line+2
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	b.WriteString("Generated source near compiler error:\n")
	for index := start; index < end; index++ {
		fmt.Fprintf(&b, "%5d  %s\n", index+1, lines[index])
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func firstDifference(first, second string) string {
	left, right := strings.Split(first, "\n"), strings.Split(second, "\n")
	line := 0
	for line < len(left) && line < len(right) && left[line] == right[line] {
		line++
	}
	start, end := line-2, line+3
	if start < 0 {
		start = 0
	}
	leftEnd, rightEnd := end, end
	if leftEnd > len(left) {
		leftEnd = len(left)
	}
	if rightEnd > len(right) {
		rightEnd = len(right)
	}
	return fmt.Sprintf("First source around line %d:\n%s\nSecond source:\n%s",
		line+1, strings.Join(left[start:leftEnd], "\n"), strings.Join(right[start:rightEnd], "\n"))
}

func relative(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(value)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func diagnosticsClean(diagnostics []applescript.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		message := strings.ToLower(diagnostic.Message)
		if strings.Contains(message, "unsupported") ||
			strings.Contains(message, "not implemented") ||
			strings.Contains(message, "unknown opcode") ||
			strings.Contains(message, "cannot recover") {
			return false
		}
	}
	return true
}

func signatureDifference(first, second []string) string {
	index := 0
	for index < len(first) && index < len(second) && first[index] == second[index] {
		index++
	}
	start, end := index-3, index+4
	if start < 0 {
		start = 0
	}
	firstEnd, secondEnd := end, end
	if firstEnd > len(first) {
		firstEnd = len(first)
	}
	if secondEnd > len(second) {
		secondEnd = len(second)
	}
	return fmt.Sprintf("Semantic opcode difference at index %d:\noriginal: %s\nrecompiled: %s",
		index, strings.Join(first[start:firstEnd], ", "), strings.Join(second[start:secondEnd], ", "))
}

func formatDiagnostics(label string, diagnostics []applescript.Diagnostic) string {
	var values []string
	for _, diagnostic := range diagnostics {
		values = append(values, fmt.Sprintf("%s at 0x%x: %s", diagnostic.Stage, diagnostic.Offset, diagnostic.Message))
	}
	return label + ": " + strings.Join(values, "; ")
}

var artifactPattern = regexp.MustCompile(`[^\s:]*/go_audit_[^\s:]+`)

func cleanDetail(value string) string {
	value = artifactPattern.ReplaceAllString(strings.TrimSpace(value), "<oracle-artifact>")
	if value == "" {
		return "No diagnostic text was provided."
	}
	return value
}

func corpusRevision(root string) string {
	output, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

var remoteIdentity = regexp.MustCompile(`[:/]([^/:]+)/([^/]+?)(?:\.git)?$`)

func corpusIdentity(root string) string {
	output, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").Output()
	if err == nil {
		if match := remoteIdentity.FindStringSubmatch(strings.TrimSpace(string(output))); len(match) == 3 {
			return match[1] + "/" + match[2]
		}
	}
	return filepath.Base(root)
}

func must(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, values ...any) {
	fmt.Fprintf(os.Stderr, "corpus-audit: "+format+"\n", values...)
	os.Exit(1)
}
