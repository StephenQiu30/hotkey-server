package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRightsManagementContractsKeepPOJOLayerBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"internal/modules/source/application/rights_management.go",
		"internal/modules/source/application/rights_management_projection.go",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(relative)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		domainAliases := make(map[string]struct{})
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(path, "/domain") {
				alias := filepath.Base(path)
				if spec.Name != nil {
					alias = spec.Name.Name
				}
				domainAliases[alias] = struct{}{}
			}
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, rawSpec := range general.Specs {
				typeSpec, ok := rawSpec.(*ast.TypeSpec)
				if !ok || !exportedIdentifier(typeSpec.Name.Name) {
					continue
				}
				ast.Inspect(typeSpec.Type, func(node ast.Node) bool {
					selector, ok := node.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					identifier, ok := selector.X.(*ast.Ident)
					if ok {
						if _, forbidden := domainAliases[identifier.Name]; forbidden {
							t.Errorf("%s exposes Domain type %s.%s", typeSpec.Name.Name, identifier.Name, selector.Sel.Name)
						}
					}
					return true
				})
			}
		}
	}

	for _, relative := range []string{
		"internal/modules/source/infrastructure/postgres/rights_management_record.go",
		"internal/modules/source/infrastructure/postgres/rights_management_projection_record.go",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(relative)), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, rawSpec := range general.Specs {
				typeSpec, ok := rawSpec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if strings.HasSuffix(typeSpec.Name.Name, "Record") && exportedIdentifier(typeSpec.Name.Name) {
					t.Errorf("PostgreSQL record %s must remain infrastructure-private", typeSpec.Name.Name)
				}
			}
		}
	}

	transportFile, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal/modules/source/transport/http/rights_management_dto.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range transportFile.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, rawSpec := range general.Specs {
			typeSpec, ok := rawSpec.(*ast.TypeSpec)
			if !ok || !exportedIdentifier(typeSpec.Name.Name) {
				continue
			}
			if !strings.HasSuffix(typeSpec.Name.Name, "RequestDTO") && !strings.HasSuffix(typeSpec.Name.Name, "ResponseDTO") {
				t.Errorf("Transport DTO %s must use a RequestDTO or ResponseDTO semantic suffix", typeSpec.Name.Name)
			}
		}
	}
}
