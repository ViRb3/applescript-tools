// Command generate-fixtures materializes compiler-oracle artifacts for the
// platform-independent regression suite. It is intentionally not run by tests.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	applescript "applescript-tools"
	"applescript-tools/passes"
)

func main() {
	compileSource := resolveCompiler()
	sources, err := filepath.Glob("testdata/oracle/*.applescript")
	must(err)
	must(os.MkdirAll("testdata/compiled", 0o755))
	for _, sourcePath := range sources {
		name := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		source, err := os.ReadFile(sourcePath)
		must(err)
		script := compileSource(string(source), name)
		destination := filepath.Join("testdata/compiled", name+".scpt")
		must(os.WriteFile(destination, script, 0o644))
		options := applescript.DecompileOptions{}
		switch name {
		case "ascii_recovery", "naive_list_edges", "naive_empty_concat":
			options.Passes = []passes.Pass{passes.Strings{}}
		}
		file, err := os.Open(destination)
		must(err)
		result, err := applescript.Decompile(context.Background(), file, options)
		_ = file.Close()
		must(err)
		must(os.WriteFile(filepath.Join("testdata/compiled", name+".expected.applescript"), []byte(result.Source), 0o644))
		fmt.Println(name)
	}
}

type compileFunc func(source, label string) []byte

func resolveCompiler() compileFunc {
	if directory := os.Getenv("APPLESCRIPT_COMPILER_ORACLE_DIR"); directory != "" {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			panic(fmt.Sprintf("compiler oracle directory is unavailable: %s", directory))
		}
		return func(source, label string) []byte {
			return compileExternal(directory, source, label)
		}
	}
	if runtime.GOOS != "darwin" {
		panic("set APPLESCRIPT_COMPILER_ORACLE_DIR to a macOS compiler service directory")
	}
	executable, err := exec.LookPath("osacompile")
	must(err)
	return func(source, label string) []byte {
		return compileLocal(executable, source, label)
	}
}

func compileExternal(directory, source, label string) []byte {
	token := make([]byte, 10)
	_, err := rand.Read(token)
	must(err)
	stem := "go_fixture_" + label + "_" + hex.EncodeToString(token)
	sourcePath := filepath.Join(directory, stem+".applescript")
	scriptPath := filepath.Join(directory, stem+".scpt")
	logPath := filepath.Join(directory, stem+".log")
	defer os.Remove(sourcePath)
	defer os.Remove(scriptPath)
	defer os.Remove(logPath)
	must(os.WriteFile(sourcePath, []byte(source), 0o644))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(logPath); err == nil {
			if _, err := os.Stat(scriptPath); err == nil {
				data, readErr := os.ReadFile(scriptPath)
				must(readErr)
				return data
			}
			log, _ := os.ReadFile(logPath)
			panic(string(log))
		}
		time.Sleep(50 * time.Millisecond)
	}
	panic("compiler oracle timeout")
}

func compileLocal(executable, source, label string) []byte {
	directory, err := os.MkdirTemp("", "applescript-fixture-"+label+"-")
	must(err)
	defer os.RemoveAll(directory)
	sourcePath := filepath.Join(directory, label+".applescript")
	scriptPath := filepath.Join(directory, label+".scpt")
	must(os.WriteFile(sourcePath, []byte(source), 0o644))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(
		ctx,
		executable,
		"-o",
		scriptPath,
		sourcePath,
	).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("osacompile failed for %s: %v\n%s", label, err, output))
	}
	script, err := os.ReadFile(scriptPath)
	must(err)
	return script
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
