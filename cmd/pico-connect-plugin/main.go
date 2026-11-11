package main

import (
	"fmt"
	"os"

	"github.com/wzhqwq/vrcft-go/internal/picoconnect"
	"github.com/wzhqwq/vrcft-go/pkg/pluginruntime"
)

func main() {
	if err := pluginruntime.Main(picoconnect.New()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
