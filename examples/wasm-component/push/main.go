package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/wlow/wlow/pkg/artifact"
	"github.com/wlow/wlow/pkg/build"
)

func main() {
	component := flag.String("component", "examples/wasm-component/echo.wasm", "WASIp2 component path")
	flag.Parse()
	spec, err := build.Build(context.Background(), build.Options{
		Kind:          build.SourceWasm,
		Path:          *component,
		Runtime:       artifact.RuntimeWasm,
		ProcessorID:   "wasm-component-echo",
		Version:       "v1",
		Tags:          []string{"latest"},
		Deterministic: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("built wasm component spec bytes=%d world=%s\n", len(spec.Data), spec.Manifest.WITWorld)
}
