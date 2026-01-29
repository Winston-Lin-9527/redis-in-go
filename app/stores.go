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
	maxDuration := 25 * time.Millisecond
	startTime := time.Now()

	maxSampleSize := 20
	numShardsToCheck := (SHARD_COUNT + 2 - 1) / 2 // (a+b-1)/b gives the ceiling

	perm := rand.Perm(len(sdb.shards)) // randomized, then select

	for i := 0; i < numShardsToCheck; i++ {
		// clean only one random shard at a time
		shardIndex := perm[i]
		shard := sdb.shards[shardIndex]
		fmt.Println("purging shard: " + strconv.Itoa(shardIndex))

		for { // check a particular shard
			shard.mu.Lock()

			expiredCnt := 0
			checkedCnt := 0

			for key, obj := range shard.data {
				if checkedCnt >= maxSampleSize {
					break
				}
				checkedCnt++

				if !obj.expires.IsZero() && time.Now().After(obj.expires) {
					delete(shard.data, key)
					expiredCnt++
					fmt.Println("purged key: " + key)
				}

			}
			shard.mu.Unlock()

			if checkedCnt == 0 { // if shard empty, break to skip this shard
				break
			}

			if float64(expiredCnt)/float64(checkedCnt) < 0.25 {
				// shard is CLEAN!! continue working on the next shard
				// unless we've reached maxShardsToCheck // TODO: the next random shard could be the same
				// therefore selecting without replacement would be better
				break
			}

			// SAFETY CHECK: don't linger too long in a purge
			if time.Since(startTime) > maxDuration {
				return
			}

			// otherwise, continue working on this shard, cuz this shard is DIRTYY!!s
		}
	}

	fmt.Println("purged finished")
}
