package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"os"

	"github.com/wlow/wlow/pkg/sdk"
	"github.com/wlow/wlow/pkg/workflow"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func main() {
	addr := flag.String("addr", ":8088", "listen address")
	natsURL := flag.String("nats", "nats://localhost:4222", "NATS URL")
	flag.Parse()
	client, err := sdk.NewClient(sdk.ClientConfig{NATSUrl: *natsURL})
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	defer client.Close()
	http.HandleFunc("/mcp", handler(client))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func handler(client *sdk.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.Error(w, "sse not enabled", http.StatusMethodNotAllowed)
			return
		}
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: err.Error()})
			return
		}
		result, err := dispatch(r.Context(), client, req)
		if err != nil {
			writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: err.Error()})
			return
		}
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
	}
}

func dispatch(ctx context.Context, client *sdk.Client, req rpcRequest) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "wlow", "version": "0.1.0"}}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		return callTool(ctx, client, req.Params)
	default:
		return nil, errors.New("unsupported method")
	}
}

func tools() []map[string]any {
	return []map[string]any{
		{"name": "submit_workflow", "description": "Submit a wlow workflow"},
		{"name": "get_workflow_result", "description": "Reserved for workflow result lookup"},
		{"name": "list_processors", "description": "Reserved for processor listing"},
		{"name": "push_processor", "description": "Reserved for processor push"},
	}
}

func callTool(ctx context.Context, client *sdk.Client, params json.RawMessage) (any, error) {
	var req struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, err
	}
	if req.Name != "submit_workflow" {
		return nil, errors.New("tool not implemented")
	}
	var wfReq struct {
		Workflow json.RawMessage `json:"workflow"`
	}
	if err := json.Unmarshal(req.Arguments, &wfReq); err != nil {
		return nil, err
	}
	return submitRawWorkflow(ctx, client, wfReq.Workflow)
}

func submitRawWorkflow(ctx context.Context, client *sdk.Client, data json.RawMessage) (any, error) {
	wf, err := workflow.ParseWorkflow(data)
	if err != nil {
		return nil, err
	}
	if err := client.Submit(ctx, wf); err != nil {
		return nil, err
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": "submitted"}}}, nil
}

func writeRPC(w http.ResponseWriter, res rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
