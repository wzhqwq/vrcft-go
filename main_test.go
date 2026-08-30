package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMainWiresExactLifecycleCallbacksAndBindAllowlist(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	optionsLiteral := findWailsOptionsLiteral(t, file)

	startup := keyedElement(t, optionsLiteral, "OnStartup")
	shutdown := keyedElement(t, optionsLiteral, "OnShutdown")
	if got := selectorName(startup); got != "app.startup" {
		t.Fatalf("OnStartup = %q, want app.startup", got)
	}
	if got := selectorName(shutdown); got != "app.shutdown" {
		t.Fatalf("OnShutdown = %q, want app.shutdown", got)
	}

	bind, ok := keyedElement(t, optionsLiteral, "Bind").(*ast.CompositeLit)
	if !ok {
		t.Fatalf("Bind value = %T, want composite literal", keyedElement(t, optionsLiteral, "Bind"))
	}
	got := make([]string, len(bind.Elts))
	for index, element := range bind.Elts {
		got[index] = selectorName(element)
	}
	want := []string{"app.runtime", "app.plugins", "app.settings"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bind allowlist = %v, want %v", got, want)
	}
}

func TestMainPreservesFrontendDistEmbedAndRemovesTemplateGreet(t *testing.T) {
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSource), "//go:embed all:frontend/dist") || !strings.Contains(string(mainSource), "var assets embed.FS") {
		t.Fatal("main.go no longer preserves the frontend/dist embed contract")
	}

	appFile, err := parser.ParseFile(token.NewFileSet(), "app.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range appFile.Decls {
		method, ok := declaration.(*ast.FuncDecl)
		if ok && method.Recv != nil && method.Name.Name == "Greet" {
			t.Fatal("template Greet method remains Wails-bindable")
		}
	}
}

func findWailsOptionsLiteral(t *testing.T, file *ast.File) *ast.CompositeLit {
	t.Helper()
	var result *ast.CompositeLit
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || selectorName(call.Fun) != "wails.Run" || len(call.Args) != 1 {
			return true
		}
		address, ok := call.Args[0].(*ast.UnaryExpr)
		if !ok || address.Op != token.AND {
			t.Fatalf("wails.Run argument = %T, want &options.App literal", call.Args[0])
		}
		literal, ok := address.X.(*ast.CompositeLit)
		if !ok || selectorName(literal.Type) != "options.App" {
			t.Fatalf("wails.Run argument = %#v, want &options.App literal", address.X)
		}
		if result != nil {
			t.Fatal("main.go contains multiple wails.Run calls")
		}
		result = literal
		return true
	})
	if result == nil {
		t.Fatal("main.go contains no wails.Run(&options.App{...})")
	}
	return result
}

func keyedElement(t *testing.T, literal *ast.CompositeLit, name string) ast.Expr {
	t.Helper()
	for _, element := range literal.Elts {
		keyed, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if identifier, ok := keyed.Key.(*ast.Ident); ok && identifier.Name == name {
			return keyed.Value
		}
	}
	t.Fatalf("options.App has no %s field", name)
	return nil
}

func selectorName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := selectorName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	default:
		return ""
	}
}
