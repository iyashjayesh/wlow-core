# Go SDK

The SDK is at `pkg/sdk`. Import path: `github.com/wlow/wlow/pkg/sdk`.

## Submit a workflow

```go
client, err := sdk.NewClient(sdk.ClientConfig{
    NATSUrl:               "nats://localhost:4222",
    WorkflowSubjectPrefix: "wlow.workflow", // matches orchestrator config
})
if err != nil {
    return err
}
defer client.Close()

wf := sdk.NewWorkflow("job-001").
    AddTask("fetch", "PROCESSOR.fetch.process", map[string]any{"url": "https://example.com"}).
    AddTask("parse", "PROCESSOR.parse.process", nil, "fetch"). // depends on fetch
    MustBuild()

result, err := client.SubmitAndWait(ctx, wf, 2*time.Minute)
```

## Write a typed processor

```go
type Input struct {
    URL string `json:"url"`
}

type Output struct {
    StatusCode int `json:"status_code"`
}

type FetchProcessor struct{}

func (p *FetchProcessor) Process(ctx context.Context, in Input) (Output, error) {
    resp, err := http.Get(in.URL)
    if err != nil {
        return Output{}, err
    }
    defer resp.Body.Close()
    return Output{StatusCode: resp.StatusCode}, nil
}
```

Wire it into a runner:

```go
runner, err := sdk.NewRunner(sdk.RunnerConfig{
    ProcessorID: "fetch",
    Subjects:    []string{"PROCESSOR.fetch.process"},
    Concurrency: 4,
}, sdk.Wrap(&FetchProcessor{}))
if err != nil {
    return err
}
runner.Run(ctx)
```

## Dynamic (untyped) processor

```go
sdk.Wrap(sdk.Func[sdk.Dynamic, sdk.Dynamic](func(ctx context.Context, in sdk.Dynamic) (sdk.Dynamic, error) {
    return sdk.Dynamic{"echo": in}, nil
}))
```

## Subject alignment

The subject a task is published on must match what the runner subscribes to. The subject is specified in each `Task.Subject` field and must be routed by a JetStream stream the runner's consumer is subscribed to.

For **sandboxed tasks** (process, WASM, microVM) dispatched by the orchestrator, subjects follow the pattern:

```
{PROCESSOR_SUBJECT_PREFIX}.sandbox.{runtime}.{processorID}
```

Default prefix: `wlow.processor`. Override via `PROCESSOR_SUBJECT_PREFIX`.

For **classic SDK runners** (pkg/sdk), the subject is whatever you set in `RunnerConfig.Subjects`. The processor stream defaults to `WLOW_PROCESSOR`; override via `PROCESSOR_STREAM`.

## Processor registry

Define processors with schema metadata:

```go
sdk.DefineProcessor("transform").
    Name("Data Transformer").
    Version("1.0.0").
    Subjects("PROCESSOR.transform.process").
    Input("items", "array", "Items to transform", true).
    Output("results", "array", "Transformed items").
    Factory(NewTransformProcessor).
    MustRegister(registry)
```

## External processors (any language)

```go
processor := sdk.NewPythonProcessor("./script.py")
processor := sdk.NewNodeProcessor("./script.js")
processor := sdk.NewExecProcessor("mybin", "--flag")
```

External processors receive JSON on stdin and write JSON to stdout:

```python
import json, sys
req = json.load(sys.stdin)
print(json.dumps({"result": req["value"] * 2}))
```
