package plugins

import (
	"context"
	"errors"

	"github.com/tetratelabs/wazero"
)

type WASMRuntime struct{ runtime wazero.Runtime }

func NewWASMRuntime(ctx context.Context) *WASMRuntime {
	return &WASMRuntime{runtime: wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())}
}

func (w *WASMRuntime) Close(ctx context.Context) error { return w.runtime.Close(ctx) }

func (w *WASMRuntime) Call(ctx context.Context, moduleBytes []byte, function string, parameters ...uint64) ([]uint64, error) {
	module, err := w.runtime.Instantiate(ctx, moduleBytes)
	if err != nil {
		return nil, err
	}
	defer module.Close(ctx)
	exported := module.ExportedFunction(function)
	if exported == nil {
		return nil, errors.New("WASM function is not exported")
	}
	return exported.Call(ctx, parameters...)
}
