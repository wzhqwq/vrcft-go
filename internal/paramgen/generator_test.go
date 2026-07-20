package paramgen

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/specparser"
)

func TestGenerateProducesParsableCompleteGoSource(t *testing.T) {
	doc, source, err := specparser.LoadFile(filepath.Join("..", "..", "spec", "vrcft_osc_parameters.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(doc, "parameters", source)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "definitions_gen.go", generated, parser.AllErrors); err != nil {
		t.Fatalf("generated source does not parse: %v", err)
	}
	text := string(generated)
	if !strings.Contains(text, "const ParameterCount ParameterID = 127") {
		t.Fatal("generated source does not contain expected parameter count")
	}
	if !regexp.MustCompile(`"v2/JawOpen":\s*ParameterJawOpen`).MatchString(text) {
		t.Fatal("generated source does not contain JawOpen lookup")
	}
}

func TestGenerateRejectsInvalidPackageName(t *testing.T) {
	if _, err := Generate(&specparser.Document{}, "not-valid", nil); err == nil {
		t.Fatal("Generate() accepted invalid package name")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
