package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wzhqwq/vrcft-go/internal/paramgen"
	"github.com/wzhqwq/vrcft-go/internal/specparser"
)

func main() {
	var input string
	var output string
	var packageName string
	flag.StringVar(&input, "in", "", "input YAML file")
	flag.StringVar(&output, "out", "", "output Go file")
	flag.StringVar(&packageName, "package", "parameters", "generated Go package")
	flag.Parse()

	if input == "" || output == "" {
		fatalf("-in and -out are required")
	}

	doc, source, err := specparser.LoadFile(input)
	if err != nil {
		fatalf("load specification: %v", err)
	}
	generated, err := paramgen.Generate(doc, packageName, source)
	if err != nil {
		fatalf("generate source: %v", err)
	}
	if err := os.WriteFile(output, generated, 0o644); err != nil {
		fatalf("write output: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "paramgen: "+format+"\n", args...)
	os.Exit(1)
}
