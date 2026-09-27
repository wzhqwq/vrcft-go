package main

import (
	"fmt"
	"os"

	"github.com/wzhqwq/vrcft-go/internal/steamlink"
	"github.com/wzhqwq/vrcft-go/pkg/pluginruntime"
)

func main() {
	if err := pluginruntime.Main(steamlink.New()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
