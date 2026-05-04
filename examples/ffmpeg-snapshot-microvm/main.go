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
	dockerfile := flag.String("dockerfile", "examples/ffmpeg-snapshot-microvm/Dockerfile", "Dockerfile path")
	flag.Parse()
	spec, err := build.Build(context.Background(), build.Options{
		Kind:        build.SourceDockerfile,
		Path:        *dockerfile,
		Runtime:     artifact.RuntimeSnapshot,
		ProcessorID: "ffmpeg-snapshot",
		Version:     "v1",
		Tags:        []string{"latest"},
		Entrypoint:  []string{"python", "/app/server.py"},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	spec.Manifest.IOProtocol = artifact.IOProtocolJSONVsockStream
	spec.Manifest.RuntimeConfig["before_snapshot"] = []string{"python", "/app/before_snapshot.py"}
	spec.Manifest.RuntimeConfig["after_restore"] = []string{"python", "/app/after_restore.py"}
	fmt.Printf("built snapshot microvm spec bytes=%d runtime=%s\n", len(spec.Data), spec.Manifest.Runtime)
}
