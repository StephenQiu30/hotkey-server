package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const ginImportPath = "github.com/gin-gonic/gin"

var forbiddenMethods = map[string]bool{
	"JSON":                true,
	"AbortWithStatusJSON": true,
	"String":              true,
}

type violation struct {
	path   string
	line   int
	column int
	method string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: validate-http-transport <internal/modules path>")
		os.Exit(2)
	}

	fileSet := token.NewFileSet()
	violations := make([]violation, 0)
	err := filepath.WalkDir(os.Args[1], func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || !isTransportHTTP(path) {
			return nil
		}
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return err
		}
		aliases := ginAliases(file)
		if len(aliases) == 0 {
			return nil
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			violations = append(violations, inspectFunction(fileSet, path, function.Type, function.Body, aliases, nil)...)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "inspect module HTTP transports: %v\n", err)
		os.Exit(1)
	}
	if len(violations) == 0 {
		return
	}
	for _, found := range violations {
		fmt.Fprintf(os.Stderr, "%s:%d:%d direct Gin response output via Context.%s is forbidden\n", found.path, found.line, found.column, found.method)
	}
	os.Exit(1)
}

func isTransportHTTP(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/transport/http/")
}

func ginAliases(file *ast.File) map[string]bool {
	aliases := make(map[string]bool)
	for _, imported := range file.Imports {
		if strings.Trim(imported.Path.Value, "\"") != ginImportPath || imported.Name != nil && imported.Name.Name == "_" {
			continue
		}
		name := "gin"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = true
	}
	return aliases
}

func inspectFunction(fileSet *token.FileSet, path string, functionType *ast.FuncType, body *ast.BlockStmt, aliases map[string]bool, inherited map[string]bool) []violation {
	contexts := copyContextNames(inherited)
	for name := range contextParameters(functionType.Params, aliases) {
		contexts[name] = true
	}
	violations := make([]violation, 0)
	ast.Inspect(body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.FuncLit:
			violations = append(violations, inspectFunction(fileSet, path, current.Type, current.Body, aliases, contexts)...)
			return false
		case *ast.AssignStmt:
			registerContextAliases(contexts, current.Lhs, current.Rhs)
		case *ast.ValueSpec:
			registerContextAliases(contexts, expressionsFromNames(current.Names), current.Values)
		case *ast.CallExpr:
			selector, ok := current.Fun.(*ast.SelectorExpr)
			if !ok || !forbiddenMethods[selector.Sel.Name] {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || !contexts[receiver.Name] {
				return true
			}
			position := fileSet.Position(current.Pos())
			violations = append(violations, violation{path: path, line: position.Line, column: position.Column, method: selector.Sel.Name})
		}
		return true
	})
	return violations
}

func contextParameters(parameters *ast.FieldList, aliases map[string]bool) map[string]bool {
	contexts := make(map[string]bool)
	if parameters == nil {
		return contexts
	}
	for _, field := range parameters.List {
		if !isGinContext(field.Type, aliases) {
			continue
		}
		for _, name := range field.Names {
			contexts[name.Name] = true
		}
	}
	return contexts
}

func isGinContext(expression ast.Expr, aliases map[string]bool) bool {
	pointer, ok := expression.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Context" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && aliases[packageName.Name]
}

func registerContextAliases(contexts map[string]bool, left, right []ast.Expr) {
	if len(left) != len(right) {
		return
	}
	for index, value := range right {
		rightName, ok := value.(*ast.Ident)
		if !ok || !contexts[rightName.Name] {
			continue
		}
		leftName, ok := left[index].(*ast.Ident)
		if ok {
			contexts[leftName.Name] = true
		}
	}
}

func expressionsFromNames(names []*ast.Ident) []ast.Expr {
	expressions := make([]ast.Expr, len(names))
	for index, name := range names {
		expressions[index] = name
	}
	return expressions
}

func copyContextNames(names map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(names))
	for name, known := range names {
		copied[name] = known
	}
	return copied
}
