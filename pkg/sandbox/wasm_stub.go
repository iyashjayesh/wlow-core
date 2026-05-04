//go:build no_wasm

package sandbox

import (
	"context"
	"errors"

	"github.com/wlow/wlow/pkg/artifact"
)

type wasmStubExecutor struct{}

func newWasmExecutor(Policy) Executor {
	return wasmStubExecutor{}
}

func (wasmStubExecutor) Runtime() artifact.Runtime {
	return artifact.RuntimeWasm
}

func (wasmStubExecutor) Execute(context.Context, ExecuteRequest) (*ExecuteResult, error) {
	return nil, errors.New("wasm executor not compiled into this binary")
}
