package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// TestGreenfieldApplicationContractsDoNotExposeDomainTypes protects the
// independently reviewed v2 contracts. Older Application APIs remain outside
// this gate until they are migrated explicitly; adding a new v2 public
// contract requires adding its file here in the same change.
func TestGreenfieldApplicationContractsDoNotExposeDomainTypes(t *testing.T) {
	root := repositoryRoot(t)
	files := []string{
		"internal/modules/ingestion/application/citation.go",
		"internal/modules/ingestion/application/document_recall_projection.go",
		"internal/modules/ingestion/application/document_dto.go",
		"internal/modules/ingestion/application/document_version.go",
		"internal/modules/ingestion/application/document_projection.go",
		"internal/modules/ingestion/application/hybrid_recall.go",
		"internal/modules/ingestion/application/source_document_generation.go",
		"internal/modules/knowledge/application/projection.go",
		"internal/modules/knowledge/application/projection_read.go",
		"internal/modules/monitor/application/intent_control_service.go",
		"internal/modules/monitor/application/intent_dto.go",
		"internal/modules/monitor/application/intent_execution_service.go",
		"internal/modules/monitor/application/intent_expansion_preparation.go",
		"internal/modules/monitor/application/intent_mapper.go",
		"internal/modules/monitor/application/intent_ports.go",
		"internal/modules/monitor/application/intent_service.go",
		"internal/modules/source/application/evidence_selection.go",
		"internal/modules/source/application/raw_archive.go",
		"internal/modules/source/application/raw_evidence_collection.go",
		"internal/modules/source/application/raw_evidence_dto.go",
		"internal/modules/source/application/raw_evidence_rights.go",
		"internal/modules/source/application/rights_management.go",
		"internal/modules/source/application/rights_management_projection.go",
		"internal/modules/source/application/rights_management_validation.go",
		"internal/modules/source/application/source_document_scheduling.go",
	}

	for _, relative := range files {
		relative := relative
		t.Run(filepath.Base(relative), func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(relative))
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}

			domainAliases := make(map[string]struct{})
			for _, spec := range file.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(importPath, "/domain") {
					continue
				}
				alias := filepath.Base(importPath)
				if spec.Name != nil {
					alias = spec.Name.Name
				}
				domainAliases[alias] = struct{}{}
			}

			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if isFunction && function.Name != nil && exportedIdentifier(function.Name.Name) {
					assertNoDomainSelector(t, function.Name.Name, function.Type, domainAliases)
					continue
				}
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, rawSpec := range general.Specs {
					typeSpec, ok := rawSpec.(*ast.TypeSpec)
					if !ok || !exportedIdentifier(typeSpec.Name.Name) {
						continue
					}
					assertNoDomainSelector(t, typeSpec.Name.Name, typeSpec.Type, domainAliases)
				}
			}
		})
	}
}

func assertNoDomainSelector(t *testing.T, contractName string, node ast.Node, domainAliases map[string]struct{}) {
	t.Helper()
	ast.Inspect(node, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, forbidden := domainAliases[identifier.Name]; forbidden {
			t.Errorf("%s exposes Domain type %s.%s", contractName, identifier.Name, selector.Sel.Name)
		}
		return true
	})
}

func exportedIdentifier(value string) bool {
	for _, character := range value {
		return unicode.IsUpper(character)
	}
	return false
}
