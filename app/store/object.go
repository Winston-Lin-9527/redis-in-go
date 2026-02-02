package store

import (
	"time"
)

// StoreValueType represents the type of Redis value
type StoreValueType uint8

const (
	StoreTypeString StoreValueType = iota
	StoreTypeList
	StoreTypeSet
	StoreTypeHash
)

// RedisObject is a unified data structure for strings, hashes, sets, lists
// since there are universal commands like DEL, EXPIRE, TYPE, RENAME etc
type RedisObject struct {
	StoreType StoreValueType
	Val       any
	Expires   time.Time
}
