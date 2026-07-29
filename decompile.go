package applescript

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"applescript-tools/ast"
	"applescript-tools/internal/decompile"
	"applescript-tools/internal/fas"
	"applescript-tools/internal/model"
	"applescript-tools/passes"
	"applescript-tools/terminology"
)

type DecompileOptions struct {
	Strict                bool
	DisableEmbeddedUnwrap bool
	Limits                Limits
	Terminology           *terminology.Registry
	Passes                []passes.Pass
}

type DecompileResult struct {
	Script      *ast.Script
	Source      string
	Diagnostics []Diagnostic
	Embedded    bool
}

func Decompile(ctx context.Context, r io.Reader, opts DecompileOptions) (*DecompileResult, error) {
	terms := opts.Terminology
	if terms == nil {
		var err error
		terms, err = terminology.Default()
		if err != nil {
			return nil, err
		}
	}
	limits := fas.Limits{
		MaxInputBytes: opts.Limits.MaxInputBytes, MaxObjects: opts.Limits.MaxObjects,
		MaxReferences: opts.Limits.MaxReferences, MaxDepth: opts.Limits.MaxDepth,
		MaxBlobBytes: opts.Limits.MaxBlobBytes,
	}
	doc, err := fas.Parse(r, fas.Options{Strict: opts.Strict, Limits: limits})
	if err != nil {
		return nil, err
	}
	if !opts.DisableEmbeddedUnwrap {
		if payload := embeddedScript(doc.Root); payload != nil {
			nested, nestedErr := Decompile(ctx, bytes.NewReader(payload), opts)
			if nestedErr != nil {
				return nil, nestedErr
			}
			nested.Embedded = true
			return nested, nil
		}
	}
	script, err := model.Normalize(doc)
	if err != nil {
		return nil, err
	}
	internal, err := decompile.Run(ctx, script, decompile.Options{Strict: opts.Strict, Terms: terms})
	if err != nil && opts.Strict {
		return nil, err
	}
	result := &DecompileResult{Script: internal.Script}
	for _, item := range doc.Diagnostics {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Stage: StageParse, Offset: item.Offset, Function: -1, Message: item.Message})
	}
	for _, item := range internal.Diagnostics {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: SeverityWarning, Stage: StageDecompile, Offset: int64(item.Offset), Function: item.Function, Message: item.Message})
	}
	for _, pass := range opts.Passes {
		rewritten, diagnostics := pass.Rewrite(result.Script)
		result.Script = rewritten
		for _, item := range diagnostics {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: SeverityWarning, Stage: StageTransform, Offset: int64(item.Origin.Start),
				Function: item.Origin.Function, Message: fmt.Sprintf("%s: %s", pass.Name(), item.Message),
			})
		}
	}
	result.Source, err = ast.Format(result.Script, terms)
	if err != nil {
		return result, err
	}
	if opts.Strict && len(result.Diagnostics) != 0 {
		return result, fmt.Errorf("strict decompilation produced %d diagnostics", len(result.Diagnostics))
	}
	return result, nil
}

func embeddedScript(root fas.Value) []byte {
	seen := make(map[fas.Value]bool)
	var visit func(fas.Value) []byte
	visit = func(value fas.Value) []byte {
		if value == nil {
			return nil
		}
		switch value := value.(type) {
		case *fas.RawData:
			if len(value.Data) >= 8 && string(value.Data[:8]) == "scptFasd" {
				return value.Data[4:]
			}
		case *fas.Object:
			return visit(value.Value)
		case *fas.Vector:
			if seen[value] {
				return nil
			}
			seen[value] = true
			for _, child := range value.Children {
				if result := visit(child); result != nil {
					return result
				}
			}
		case *fas.Pair:
			if seen[value] {
				return nil
			}
			seen[value] = true
			if result := visit(value.Head); result != nil {
				return result
			}
			return visit(value.Tail)
		case *fas.Binding:
			if seen[value] {
				return nil
			}
			seen[value] = true
			for _, child := range []fas.Value{value.Key, value.Value, value.Extra, value.Next} {
				if result := visit(child); result != nil {
					return result
				}
			}
		case *fas.Statement:
			if seen[value] {
				return nil
			}
			seen[value] = true
			for _, child := range value.Children {
				if result := visit(child); result != nil {
					return result
				}
			}
		}
		return nil
	}
	return visit(root)
}
