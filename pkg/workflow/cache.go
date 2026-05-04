package workflow

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/zeebo/blake3"
)

const OutputCacheBucket = "wlow-output-cache"

type OutputCache interface {
	Get(ctx context.Context, key string) (*TaskResult, bool, error)
	Put(ctx context.Context, key string, result *TaskResult) error
}

type NATSOutputCache struct {
	kv jetstream.KeyValue
}

func NewNATSOutputCache(ctx context.Context, js jetstream.JetStream, ttl time.Duration) (*NATSOutputCache, error) {
	if js == nil {
		return nil, errors.New("jetstream required")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	kv, err := js.KeyValue(ctx, OutputCacheBucket)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: OutputCacheBucket, TTL: ttl})
	}
	if err != nil {
		return nil, err
	}
	return &NATSOutputCache{kv: kv}, nil
}

func (c *NATSOutputCache) Get(ctx context.Context, key string) (*TaskResult, bool, error) {
	entry, err := c.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var result TaskResult
	return &result, true, json.Unmarshal(entry.Value(), &result)
}

func (c *NATSOutputCache) Put(ctx context.Context, key string, result *TaskResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = c.kv.Put(ctx, key, data)
	return err
}

func OutputCacheKey(processorID, version string, input map[string]any) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	inputHash := blake3.Sum256(data)
	h := blake3.New()
	_, _ = h.Write([]byte(processorID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(version))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(inputHash[:])
	return fmt.Sprintf("out.%s", hex.EncodeToString(h.Sum(nil))), nil
}
