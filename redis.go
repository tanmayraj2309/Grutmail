package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const DLQKey = "email:dlq"

// DLQMessage represents the structured payload stored in Redis DLQ.
type DLQMessage struct {
	Recipient Recipient `json:"recipient"`
	Error     string    `json:"error"`
	WorkerID  int       `json:"worker_id"`
	FailedAt  time.Time `json:"failed_at"`
}

// initRedis initializes and verifies connection to the Redis server.
func initRedis() (*redis.Client, error) {
	var rdb *redis.Client

	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
		}
		rdb = redis.NewClient(opt)
	} else {
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "localhost:6379"
		}
		rdb = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: "", // no password by default
			DB:       0,  // default DB
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return rdb, nil
}

// pushToDLQ stores a failed email event into the Redis Dead Letter Queue (DLQ) list.
func pushToDLQ(ctx context.Context, rdb *redis.Client, recipient Recipient, errMsg string, workerID int) error {
	msg := DLQMessage{
		Recipient: recipient,
		Error:     errMsg,
		WorkerID:  workerID,
		FailedAt:  time.Now(),
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal dlq message: %w", err)
	}

	// LPush pushes to the head of the Redis list
	err = rdb.LPush(ctx, DLQKey, payload).Err()
	if err != nil {
		return fmt.Errorf("failed to push to redis dlq: %w", err)
	}

	return nil
}

// getDLQCount returns the current number of failed messages waiting in the Redis DLQ.
func getDLQCount(ctx context.Context, rdb *redis.Client) (int64, error) {
	return rdb.LLen(ctx, DLQKey).Result()
}

// fetchDLQMessages retrieves all items from the Redis DLQ list.
func fetchDLQMessages(ctx context.Context, rdb *redis.Client) ([]DLQMessage, error) {
	items, err := rdb.LRange(ctx, DLQKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	var messages []DLQMessage
	for _, item := range items {
		var msg DLQMessage
		if err := json.Unmarshal([]byte(item), &msg); err == nil {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

// clearDLQ deletes the entire Redis DLQ list.
func clearDLQ(ctx context.Context, rdb *redis.Client) error {
	return rdb.Del(ctx, DLQKey).Err()
}

