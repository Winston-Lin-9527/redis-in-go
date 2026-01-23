package main

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

type StoreValueType uint8

const (
	StoreTypeString StoreValueType = iota
	StoreTypeList
	StoreTypeSet
	StoreTypeHash
)

// unified data structure for strings, hashes, sets, lists, since there are universal commands like DEL, EXPIRE, TYPE, RENAME etc
type RedisObject struct {
	StoreType StoreValueType
	val       any
	expires   time.Time
}

// pretty print as string
func (ro *RedisObject) PPrint() string {
	switch ro.StoreType {
	case StoreTypeString:
		// try assert as []byte
		if b, ok := ro.val.([]byte); ok {
			return string(b)
		}
		return "error: invalid string data type"
	default:
		return "PPrint: UNDEFINED data type\n"
	}
}

// a shard
type RedisDB struct {
	data map[string]*RedisObject
	mu   sync.RWMutex
}

func NewRedisDB() *RedisDB {
	return &RedisDB{
		data: make(map[string]*RedisObject),
		// mu sync.RWMutex zero-value is already usable
	}
}

// apparently using a number that is power of 2 will make the modulo only looks at the lower bits of the hash, causing results look linear
const SHARD_COUNT = 5

type ShardedRedisDB struct {
	shards []*RedisDB
}

func NewShardedRedisDB() *ShardedRedisDB {
	s := &ShardedRedisDB{shards: make([]*RedisDB, SHARD_COUNT)}
	for i := 0; i < len(s.shards); i++ {
		s.shards[i] = NewRedisDB()
	}

	return s
}

// router function
// TODO: consistent hashing to allow dynamic shard insertion or deletion
func (sdb *ShardedRedisDB) getShard(key string) *RedisDB {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	index := int(hasher.Sum32() % uint32(SHARD_COUNT))

	return sdb.shards[index]
}

// debug purpose only
func (sdb *ShardedRedisDB) GetShardIndex(key string) int {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	index := int(hasher.Sum32() % uint32(SHARD_COUNT))
	return index
}

func (sdb *ShardedRedisDB) SetKey(key string, val string, expires time.Time) {
	db := sdb.getShard(key)

	db.mu.Lock()
	defer db.mu.Unlock()

	db.data[key] = &RedisObject{
		StoreType: StoreTypeString,
		val:       []byte(val),
		expires:   expires,
	}
}

// returns pointer to the redis object
func (sdb *ShardedRedisDB) GetKey(key string) (*RedisObject, bool) {
	shard := sdb.getShard(key)

	shard.mu.RLock()
	value, ok := shard.data[key]
	if !ok {
		shard.mu.RUnlock()
		return nil, false
	}

	// Lazy Expiry check
	if !value.expires.IsZero() && time.Now().After(value.expires) {
		shard.mu.RUnlock() // Release RLock before acquiring Lock to avoid deadlock
		shard.mu.Lock()

		// Double-check existence and expiry under write lock
		if obj, ok := shard.data[key]; ok && !obj.expires.IsZero() && time.Now().After(obj.expires) {
			delete(shard.data, key)
		}
		shard.mu.Unlock()
		return nil, false
	}

	shard.mu.RUnlock()
	return value, true
}

func (sdb *ShardedRedisDB) StartJanitor() {
	ticker := time.NewTicker(5 * time.Second) // runs every 5 seconds
	go func() {
		for range ticker.C {
			sdb.purgeExpiredKeys()
		}
	}()
}

func (sdb *ShardedRedisDB) purgeExpiredKeys() {
	maxPurges := 10 // purge for 10 times maximum

	for i := 0; i < maxPurges; i++ {
		// clean only one random shard at a time
		shardIndex := rand.Intn(len(sdb.shards))
		shard := sdb.shards[shardIndex]

		shard.mu.Lock()

		iterationCnt := 0
		maxSampleSize := 20
		expiredCnt := 0
		fmt.Println("purging shard: " + strconv.Itoa(shardIndex))

		for key, obj := range shard.data {
			if iterationCnt >= maxSampleSize {
				break
			}
			iterationCnt++

			if !obj.expires.IsZero() && time.Now().After(obj.expires) {
				delete(shard.data, key)
				expiredCnt++
				fmt.Println("purged key: " + key)
			}
		}

		shard.mu.Unlock()

		// TODO: This janitor logic doesn't seem right.... need to re-work

		// adaptive strategy, exit the loop ONLY if we checked enough keys and less than 25% are expired
		// but we still want to give other shards a chance in the next iterations of the outer loop
		// so we break out of THIS specific purge attempt if the current shard is clean
		if iterationCnt > 0 && float64(expiredCnt)/float64(iterationCnt) < 0.25 {
			// Instead of breaking the whole loop, we just continue to the next random shard
			// unless we've reached maxPurges.
			continue
		}
	}

	fmt.Println("purged finished")
}
