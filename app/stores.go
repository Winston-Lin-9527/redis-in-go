package main

import (
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

func (db *RedisDB) SetKey(key string, val string) {
	// TODO: ignore if key already exists
	// if key exists but different type, error out (wrong type)

	// otherwise create new KV pair or overwrite
	db.mu.Lock()

	newObj := RedisObject{
		StoreType: StoreTypeString,
		val:       []byte(val), // TODO: confirm with AI that this is ok
		expires:   time.Time{},
	}

	db.data[key] = &newObj

	db.mu.Unlock()
}

// returns pointer to the redis object
func (db *RedisDB) GetKey(key string) (*RedisObject, bool) {
	db.mu.RLock()

	value, ok := db.data[key]

	db.mu.RUnlock()

	return value, ok
}
