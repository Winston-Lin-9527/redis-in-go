package store

import (
	"fmt"
	"hash/fnv"
	"path"
	"sync"
	"time"
)

// SHARD_COUNT determines the number of shards for the database
// Using a non-power-of-2 to avoid linear-looking hash distribution
const SHARD_COUNT = 5

// RedisDB represents a single shard
type RedisDB struct {
	Data map[string]*RedisObject
	mu   sync.RWMutex
}

// NewRedisDB creates a new RedisDB shard
func NewRedisDB() *RedisDB {
	return &RedisDB{
		Data: make(map[string]*RedisObject),
	}
}

// ShardedRedisDB is the main data store with multiple shards
type ShardedRedisDB struct {
	Shards []*RedisDB
}

// NewShardedRedisDB creates a new sharded Redis database
func NewShardedRedisDB() *ShardedRedisDB {
	s := &ShardedRedisDB{Shards: make([]*RedisDB, SHARD_COUNT)}
	for i := 0; i < len(s.Shards); i++ {
		s.Shards[i] = NewRedisDB()
	}
	return s
}

// getShard returns the shard for a given key
// TODO: consistent hashing to allow dynamic shard insertion or deletion
func (sdb *ShardedRedisDB) getShard(key string) *RedisDB {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	index := int(hasher.Sum32() % uint32(SHARD_COUNT))
	return sdb.Shards[index]
}

// GetShardIndex returns the shard index for a key (debug purpose)
func (sdb *ShardedRedisDB) GetShardIndex(key string) int {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	index := int(hasher.Sum32() % uint32(SHARD_COUNT))
	return index
}

// SetKey sets a string value for a key
func (sdb *ShardedRedisDB) SetKey(key string, val string, expires time.Time) error {
	shard := sdb.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// if key does not exist, create it
	if _, ok := shard.Data[key]; !ok {
		shard.Data[key] = &RedisObject{
			StoreType: StoreTypeString,
			Val:       nil,
			Expires:   expires,
		}
	}

	// check if the existing key is of string type
	if shard.Data[key].StoreType != StoreTypeString {
		return fmt.Errorf("SetKey: key %s is not a string type", key)
	}

	// if all good, set (or overwrite) the value
	shard.Data[key].Val = []byte(val)
	shard.Data[key].Expires = expires

	return nil
}

// GetKey retrieves a key's RedisObject
func (sdb *ShardedRedisDB) GetKey(key string) (*RedisObject, error) {
	shard := sdb.getShard(key)

	shard.mu.RLock()
	value, ok := shard.Data[key]
	if !ok {
		shard.mu.RUnlock()
		return nil, fmt.Errorf("GetKey: key %s not found", key)
	}

	// Lazy Expiry check
	if !value.Expires.IsZero() && time.Now().After(value.Expires) {
		shard.mu.RUnlock() // Release RLock before acquiring Lock to avoid deadlock
		shard.mu.Lock()

		// Double-check existence and expiry under write lock
		if obj, ok := shard.Data[key]; ok && !obj.Expires.IsZero() && time.Now().After(obj.Expires) {
			delete(shard.Data, key)
		}
		shard.mu.Unlock()
		return nil, fmt.Errorf("GetKey: key %s not found", key)
	}

	shard.mu.RUnlock()
	return value, nil
}

// HSetKey sets a field in a hash
func (sdb *ShardedRedisDB) HSetKey(key string, field string, val string) error {
	shard := sdb.getShard(key)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if _, ok := shard.Data[key]; !ok { // if top-level key does not exist, create it
		shard.Data[key] = &RedisObject{
			StoreType: StoreTypeHash,
			Val:       make(map[string]string),
			Expires:   time.Time{}, // defaults to non-expiring
		}
	}

	// check if the existing key is of hash type
	if shard.Data[key].StoreType != StoreTypeHash {
		return fmt.Errorf("HSetKey: key %s is not a hash type", key)
	}

	// now set(or overwrite) the field in the hash
	shard.Data[key].Val.(map[string]string)[field] = val

	return nil
}

// HGetKey gets a field from a hash
func (sdb *ShardedRedisDB) HGetKey(key string, field string) (string, error) {
	shard := sdb.getShard(key)

	shard.mu.RLock()
	defer shard.mu.RUnlock()

	redisObj, ok := shard.Data[key]
	if !ok {
		return "", fmt.Errorf("HGetKey: key %s not found", key)
	}

	value, ok := redisObj.Val.(map[string]string)[field]
	if !ok {
		return "", fmt.Errorf("HGetKey: field %s not found", field)
	}

	return value, nil
}

// GetAllKeys returns all keys matching the given glob pattern
// Pattern examples: "*" (all), "user:*", "session:???", "[ab]*"
func (sdb *ShardedRedisDB) GetAllKeys(pattern string) []string {
	var allKeys []string
	var mu sync.Mutex // for the allKeys
	var wg sync.WaitGroup

	// Iterate through all shards in parallel for better performance
	for _, shard := range sdb.Shards {
		wg.Add(1)
		go func(s *RedisDB) {
			defer wg.Done()

			s.mu.RLock()
			localKeys := make([]string, 0)
			now := time.Now()

			for key, obj := range s.Data {
				// Skip expired keys (lazy expiry check)
				if !obj.Expires.IsZero() && now.After(obj.Expires) {
					continue
				}

				// Match pattern using glob-style matching
				matched, err := path.Match(pattern, key)
				if err == nil && matched {
					localKeys = append(localKeys, key)
				}
			}
			s.mu.RUnlock()

			// Collect results from all shards
			if len(localKeys) > 0 {
				mu.Lock()
				allKeys = append(allKeys, localKeys...)
				mu.Unlock()
			}
		}(shard)
	}

	wg.Wait()
	return allKeys
}

// PPrint pretty prints a RedisObject as string
func (ro *RedisObject) PPrint() string {
	switch ro.StoreType {
	case StoreTypeString:
		// try assert as []byte
		if b, ok := ro.Val.([]byte); ok {
			return string(b)
		}
		return "error: invalid string data type"
	case StoreTypeHash:
		// map[string]string
		if m, ok := ro.Val.(map[string]string); ok {
			for k, v := range m {
				return fmt.Sprintf("%s: %s\n", k, v)
			}
		}
		return "error: invalid hash data type"
	default:
		return "PPrint: UNDEFINED data type\n"
	}
}
