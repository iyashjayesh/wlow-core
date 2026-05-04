// Package sdk provides the processor development kit.
//
// Quick start:
//
//	sdk.Run("echo", "PROCESSOR.echo.process", func(ctx context.Context, in map[string]any) (map[string]any, error) {
//	    return in, nil
//	})
//
// For configurable processors, use the Registry:
//
//	sdk.DefineProcessor("transform").
//	    Subjects("PROCESSOR.transform.process").
//	    Config("batch", "int", "batch size", false, 100).
//	    Factory(NewTransformProcessor).
//	    MustRegister(registry)
//
// External language support:
//
//	sdk.NewPythonProcessor("./script.py")
//	sdk.NewHTTPProcessor("http://localhost:8080/process")
package sdk
