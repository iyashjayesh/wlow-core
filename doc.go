// Package core is the wlow workflow orchestrator — a DAG-based, NATS JetStream-native
// system for running sandboxed processor workloads (process, WASM, Firecracker microVM).
//
// Architecture:
//
//	┌─────────┐  {prefix}.submit    ┌─────────────┐  {prefix}.sandbox.>  ┌───────────┐
//	│ Client  │ ──────────────────► │ Orchestrator│ ─────────────────►   │  wlow-    │
//	│         │ ◄────────────────── │             │ ◄──────────────────   │  runner   │
//	└─────────┘  {prefix}.reply     └──────┬──────┘  {prefix}.result.*   └───────────┘
//	                                        │
//	                                        ▼
//	                                ┌─────────────┐          ┌─────────────┐
//	                                │  NATS KV    │          │  OCI / GAR  │
//	                                │  (state +   │          │  (artifact  │
//	                                │   metadata) │          │   payloads) │
//	                                └─────────────┘          └─────────────┘
//
// Default subjects (override via WORKFLOW_SUBJECT_PREFIX / PROCESSOR_SUBJECT_PREFIX):
//
//	workflow.submit           Submit workflow
//	workflow.result.<task>    Task completion
//	workflow.cancel           Cancel request
//	workflow.cancel.<wf>      Per-workflow cancel
//	workflow.reply.<wf>       Final result
//	wlow.processor.sandbox.>  Sandboxed task routing
//
// Task states: pending → queued → running → completed|failed|cancelled|timed_out
package core
