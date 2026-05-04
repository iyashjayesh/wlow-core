package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const LocalityBucket = "wlow-node-inventory"

type NodeInventory struct {
	NodeID      string    `json:"node_id"`
	Chunks      []string  `json:"chunks,omitempty"`
	Snapshots   []string  `json:"snapshots,omitempty"`
	Warm        []string  `json:"warm,omitempty"`
	Concurrency int       `json:"concurrency"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PrefetchHint struct {
	WorkflowID string   `json:"workflow_id"`
	TaskID     string   `json:"task_id"`
	Artifacts  []string `json:"artifacts"`
}

type LocalityScheduler struct {
	kv jetstream.KeyValue
}

func NewLocalityScheduler(ctx context.Context, js jetstream.JetStream) (*LocalityScheduler, error) {
	if js == nil {
		return nil, errors.New("jetstream required")
	}
	kv, err := js.KeyValue(ctx, LocalityBucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: LocalityBucket, History: 8})
	}
	if err != nil {
		return nil, err
	}
	return &LocalityScheduler{kv: kv}, nil
}

func (s *LocalityScheduler) PutInventory(ctx context.Context, inv NodeInventory) error {
	if inv.NodeID == "" {
		return errors.New("node id required")
	}
	inv.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	_, err = s.kv.Put(ctx, "node."+inv.NodeID+".warm", data)
	return err
}

func (s *LocalityScheduler) PickNode(ctx context.Context, artifacts []string) (string, error) {
	keys, err := s.kv.Keys(ctx)
	if err != nil {
		return "", err
	}
	bestNode := ""
	bestScore := -1
	for idx := 0; idx < len(keys) && idx < maxInventoryKeys; idx++ {
		inv, err := s.inventory(ctx, keys[idx])
		if err != nil {
			return "", err
		}
		score := scoreInventory(inv, artifacts)
		if score > bestScore {
			bestNode = inv.NodeID
			bestScore = score
		}
	}
	return bestNode, nil
}

func (s *LocalityScheduler) inventory(ctx context.Context, key string) (NodeInventory, error) {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return NodeInventory{}, err
	}
	var inv NodeInventory
	return inv, json.Unmarshal(entry.Value(), &inv)
}

func PrefetchSubject(nodeID string) string {
	return fmt.Sprintf("wlow.prefetch.%s", nodeID)
}

func scoreInventory(inv NodeInventory, artifacts []string) int {
	score := 0
	for _, artifact := range artifacts {
		if contains(inv.Warm, artifact) {
			score += 100
		}
		if contains(inv.Snapshots, artifact) {
			score += 50
		}
		if contains(inv.Chunks, artifact) {
			score += 1
		}
	}
	return score
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

const maxInventoryKeys = 1 << 20
