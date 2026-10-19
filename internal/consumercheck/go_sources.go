package consumercheck

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func declarations(raw []byte, types, functions []string) ([]byte, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "consumer.go", raw, 0)
	if err != nil {
		return nil, err
	}
	wantedTypes, wantedFunctions := map[string]bool{}, map[string]bool{}
	for _, name := range types {
		wantedTypes[name] = true
	}
	for _, name := range functions {
		wantedFunctions[name] = true
	}
	var output bytes.Buffer
	for _, declaration := range file.Decls {
		var selected ast.Node
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if wantedFunctions[declaration.Name.Name] {
				selected = declaration
				delete(wantedFunctions, declaration.Name.Name)
			}
		case *ast.GenDecl:
			if declaration.Tok != token.TYPE {
				continue
			}
			var specs []ast.Spec
			for _, spec := range declaration.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && wantedTypes[typeSpec.Name.Name] {
					specs = append(specs, spec)
					delete(wantedTypes, typeSpec.Name.Name)
				}
			}
			if len(specs) > 0 {
				selected = &ast.GenDecl{Tok: token.TYPE, Specs: specs}
			}
		}
		if selected == nil {
			continue
		}
		if err := printer.Fprint(&output, set, selected); err != nil {
			return nil, err
		}
		if _, err := io.WriteString(&output, "\n\n"); err != nil {
			return nil, err
		}
	}
	if len(wantedTypes) != 0 || len(wantedFunctions) != 0 {
		return nil, fmt.Errorf("required consumer declarations missing: types=%v functions=%v", wantedTypes, wantedFunctions)
	}
	return output.Bytes(), nil
}

func (s *session) module(dir, module string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data := "module " + module + "\n\ngo 1.27.2\n\nrequire github.com/Yui-Qi-Tang/calx v0.0.0\n\nreplace github.com/Yui-Qi-Tang/calx => " + strconv.Quote(filepath.ToSlash(s.config.CalxDir)) + "\n"
	return os.WriteFile(filepath.Join(dir, "go.mod"), []byte(data), 0600)
}

func (s *session) checkAHE(ctx context.Context, kind, root string) error {
	raw, err := s.source(kind, root, "internal/evidenceingestion/consistency_check.go")
	if err != nil {
		return err
	}
	types, err := declarations(raw, []string{"ConsistencyAtom", "ConsistencyLiteral", "ConsistencyCondition", "ConsistencyRequest"}, []string{"compileConsistency"})
	if err != nil {
		return err
	}
	raw, err = s.source(kind, root, "internal/evidenceingestion/consistency_scope.go")
	if err != nil {
		return err
	}
	helper, err := declarations(raw, nil, []string{"consistencyText"})
	if err != nil {
		return err
	}
	header := []byte("package consumer\nimport(\"fmt\";\"slices\";\"strings\";\"unicode/utf8\"; logicresolver \"github.com/Yui-Qi-Tang/calx/sat\")\n")
	generated, err := format.Source(bytes.Join([][]byte{header, types, helper}, []byte("\n")))
	if err != nil {
		return err
	}
	dir := filepath.Join(s.config.OutputDir, kind)
	if err := s.module(dir, "example.com/"+kind); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "compiler.go"), generated, 0600); err != nil {
		return err
	}
	for _, name := range []string{"stubs.go", "compiler_test.go"} {
		if err := copyFixture("ahe/"+name, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return s.testGo(ctx, dir, kind, 1, "./...")
}

func (s *session) checkPouch(ctx context.Context) error {
	dir := filepath.Join(s.config.OutputDir, "pouch")
	if err := s.module(dir, "github.com/Yui-Qi-Tang/pouch"); err != nil {
		return err
	}
	for _, packageName := range []string{"planning", "validation", "projection"} {
		relative := filepath.Join("internal", packageName)
		entries, err := os.ReadDir(filepath.Join(s.config.PouchDir, relative))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, relative), 0700); err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || packageName == "validation" && strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(relative, entry.Name())
			raw, err := s.source("pouch", s.config.PouchDir, path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, path), raw, 0600); err != nil {
				return err
			}
		}
	}
	if err := copyFixture("pouch/adapter_test.go", filepath.Join(dir, "internal/planning/calx_test.go")); err != nil {
		return err
	}
	if err := copyFixture("pouch/projection_test.go", filepath.Join(dir, "internal/projection/calx_test.go")); err != nil {
		return err
	}
	return s.testGo(ctx, dir, "pouch", 3, "./internal/planning", "./internal/projection")
}
