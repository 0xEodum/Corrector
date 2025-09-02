package customdict

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// CustomDict wraps a Redis client to store custom dictionary words.
type CustomDict struct {
	client *redis.Client
	key    string
}

// New creates a new CustomDict with the provided Redis client.
func New(client *redis.Client) *CustomDict {
	return &CustomDict{client: client, key: "custom_dict"}
}

// Add inserts a word into the custom dictionary.
func (cd *CustomDict) Add(word string) error {
	return cd.client.SAdd(context.Background(), cd.key, word).Err()
}

// Remove deletes a word from the custom dictionary.
func (cd *CustomDict) Remove(word string) error {
	return cd.client.SRem(context.Background(), cd.key, word).Err()
}

// All returns all words stored in the custom dictionary.
func (cd *CustomDict) All() ([]string, error) {
	return cd.client.SMembers(context.Background(), cd.key).Result()
}
