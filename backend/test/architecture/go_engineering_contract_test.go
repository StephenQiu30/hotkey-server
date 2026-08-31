package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestGoPackagesAvoidJavaLayersAndGrabBagNames(t *testing.T) {
	root := filepath.Join(repositoryRoot(t), "internal")
	forbidden := map[string]bool{
		"common":     true,
		"controller": true,
		"dao":        true,
		"mapper":     true,
		"pojo":       true,
		"service":    true,
		"utils":      true,
	}

	var violations []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == root || !forbidden[entry.Name()] {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot(t), path)
		if err != nil {
			return err
		}
		violations = append(violations, filepath.ToSlash(relative))
		return filepath.SkipDir
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("Go packages use Java-style layers or grab-bag names:\n%s", strings.Join(violations, "\n"))
	}
}

func TestGoTypesAvoidJavaInterfaceAndImplementationNames(t *testing.T) {
	root := repositoryRoot(t)
	var violations []string
	for _, sourceRoot := range []string{filepath.Join(root, "cmd"), filepath.Join(root, "internal")} {
		err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}

			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
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
					_, isInterface := typeSpec.Type.(*ast.InterfaceType)
					javaInterface := isInterface && hasJavaInterfacePrefix(typeSpec.Name.Name)
					javaImplementation := strings.HasSuffix(typeSpec.Name.Name, "Impl")
					if !javaInterface && !javaImplementation {
						continue
					}
					relative, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					position := fileSet.Position(typeSpec.Pos())
					violations = append(violations, filepath.ToSlash(relative)+":"+strconv.Itoa(position.Line)+" "+typeSpec.Name.Name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("Go types use Ixxx/Impl Java naming:\n%s", strings.Join(violations, "\n"))
	}
}

func hasJavaInterfacePrefix(name string) bool {
	runes := []rune(name)
	if len(runes) < 2 || runes[0] != 'I' || !unicode.IsUpper(runes[1]) {
		return false
	}
	for _, initialism := range []string{"ID", "IO", "IP"} {
		if strings.HasPrefix(name, initialism) {
			return false
		}
	}
	return true
}
