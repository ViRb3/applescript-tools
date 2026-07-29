package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	applescript "applescript-tools"
	"applescript-tools/passes"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "disassemble":
		return runDisassemble(ctx, args[1:], stdin, stdout, stderr)
	case "decompile":
		return runDecompile(ctx, args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		usage(stderr)
		return 2
	}
}

type stringFlags []string

func (f *stringFlags) String() string         { return fmt.Sprint([]string(*f)) }
func (f *stringFlags) Set(value string) error { *f = append(*f, value); return nil }

func runDecompile(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("decompile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	strict := fs.Bool("strict", false, "treat recovery diagnostics as fatal")
	noUnwrap := fs.Bool("no-unwrap", false, "do not unwrap embedded compiled scripts")
	var passNames stringFlags
	fs.Var(&passNames, "pass", "apply an AST pass (repeatable: strings)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: applescript-tools decompile [--strict] [--no-unwrap] [--pass name] <file|->")
		return 2
	}
	input, closeInput, err := openInput(fs.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeInput()
	options := applescript.DecompileOptions{Strict: *strict, DisableEmbeddedUnwrap: *noUnwrap}
	for _, name := range passNames {
		pass, err := passes.Named(name)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		options.Passes = append(options.Passes, pass)
	}
	result, err := applescript.Decompile(ctx, input, options)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintln(stderr, diagnostic.String())
	}
	if _, err := io.WriteString(stdout, result.Source); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runDisassemble(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("disassemble", flag.ContinueOnError)
	fs.SetOutput(stderr)
	strict := fs.Bool("strict", false, "treat recovery diagnostics as fatal")
	jsonOutput := fs.Bool("json", false, "write versioned JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: applescript-tools disassemble [--strict] [--json] <file|->")
		return 2
	}
	input, closeInput, err := openInput(fs.Arg(0), stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer closeInput()
	result, err := applescript.Disassemble(ctx, input, applescript.DisassembleOptions{Strict: *strict})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintln(stderr, d.String())
	}
	if *strict && len(result.Diagnostics) != 0 {
		return 1
	}
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if _, err := io.WriteString(stdout, result.Text()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

func openInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		return stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = f.Close() }, nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: applescript-tools <decompile|disassemble> [options] <file|->")
}
