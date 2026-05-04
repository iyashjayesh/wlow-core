package sandbox

import (
	"context"
	"errors"

	"github.com/wlow/wlow/pkg/artifact"
)

type unsupportedMicroVMExecutor struct {
	runtime artifact.Runtime
}

func NewMicroVMExecutor(_ string) Executor {
	return unsupportedMicroVMExecutor{runtime: artifact.RuntimeMicroVM}
}

func NewSnapshotExecutor(_ string) Executor {
	return unsupportedMicroVMExecutor{runtime: artifact.RuntimeSnapshot}
}

func (e unsupportedMicroVMExecutor) Runtime() artifact.Runtime {
	return e.runtime
}

func (e unsupportedMicroVMExecutor) Execute(context.Context, ExecuteRequest) (*ExecuteResult, error) {
	if e.runtime == "" {
		return nil, errors.New("microvm runtime required")
	}
	return nil, errors.New("go microvm executor removed; use runner-rs wlow-runner")
}
