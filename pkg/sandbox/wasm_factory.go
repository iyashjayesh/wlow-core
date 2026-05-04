//go:build !no_wasm

package sandbox

func newWasmExecutor(policy Policy) Executor {
	return NewWasmExecutor(policy)
}
