# MCP Server

`wlow-mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server that exposes wlow workflow operations as tools for AI agents and IDE integrations (Cursor, Claude Desktop, etc.).

## Start

```sh
./bin/wlow-mcp --nats nats://localhost:4222 --addr :8088
```

Or with env vars:

```sh
NATS_URL=nats://localhost:4222 WLOW_MCP_ADDR=:8088 ./bin/wlow-mcp
```

The server listens for HTTP POST requests at `/mcp` using JSON-RPC 2.0.

## Connect from Cursor / Claude Desktop

Add to your MCP config:

```json
{
  "mcpServers": {
    "wlow": {
      "url": "http://localhost:8088/mcp"
    }
  }
}
```

For Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "wlow": {
      "command": "wlow-mcp",
      "args": ["--nats", "nats://localhost:4222"]
    }
  }
}
```

## Tools

### `submit_workflow`

Submit a workflow and wait for its result. Tasks execute in dependency order across registered processors.

**Arguments:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `workflow` | object | Yes | Workflow definition (see format below) |
| `timeout_seconds` | integer | No | Max wait time (default: 120, max: 600) |

**Workflow format:**

```json
{
  "id": "job-001",
  "tasks": {
    "step-a": {
      "processor_id": "my-proc",
      "processor_version": "latest",
      "input": { "text": "hello world" }
    },
    "step-b": {
      "processor_id": "my-other-proc",
      "processor_version": "latest",
      "input": {}
    }
  },
  "dependencies": {
    "step-b": ["step-a"]
  }
}
```

**Response:**

```json
{
  "workflow_id": "job-001",
  "status": "completed",
  "task_results": [
    {
      "task_id": "step-a",
      "processor_id": "my-proc",
      "status": "completed",
      "output": { "result": "..." }
    }
  ]
}
```

**Example prompt to AI agent:**

> Submit a wlow workflow that runs my-proc with input `{"text": "hello"}` and return the output.

---

### `list_processors`

List all processors registered in NATS KV.

**Arguments:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tenant` | string | No | Tenant namespace (default: `"default"`) |

**Response:**

```json
{
  "processors": [
    { "id": "my-proc",  "version": "v1",         "runtime": "process" },
    { "id": "my-wasm",  "version": "v1",         "runtime": "wasm" },
    { "id": "my-vm",    "version": "cold-v1",    "runtime": "microvm" }
  ]
}
```

---

### `get_workflow_result` / `push_processor`

Not implemented over MCP.

- **`get_workflow_result`**: Use `submit_workflow` — it waits inline and returns the result.
- **`push_processor`**: Use the CLI: `wlow push --id <id> --runtime <runtime> ...`

## Kubernetes

The `wlow-mcp` binary is included in the runtime image. To expose it:

```yaml
containers:
  - name: mcp
    image: ghcr.io/wlow-io/wlow-runtime:latest
    command: ["/usr/local/bin/wlow-mcp"]
    args: ["--nats", "nats://nats.nats:4222", "--addr", ":8088"]
    ports:
      - name: mcp
        containerPort: 8088
```

## Protocol details

- **Transport**: HTTP POST
- **Protocol version**: `2024-11-05`
- **Methods**: `initialize`, `tools/list`, `tools/call`
- SSE (streaming) is not supported
