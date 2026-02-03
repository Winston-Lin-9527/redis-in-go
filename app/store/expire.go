package store

import (
	"fmt"
	"math/rand"
	"time"
)

// StartJanitor starts the background goroutine that purges expired keys
func (sdb *ShardedRedisDB) StartJanitor() {
	ticker := time.NewTicker(5 * time.Second) // runs every 5 seconds
	go func() {
		for range ticker.C {
			sdb.purgeExpiredKeys()
		}
	}()
}

// purgeExpiredKeys removes expired keys from shards
func (sdb *ShardedRedisDB) purgeExpiredKeys() {
	maxDuration := 25 * time.Millisecond
	startTime := time.Now()

	maxSampleSize := 20
	numShardsToCheck := (SHARD_COUNT + 2 - 1) / 2 // (a+b-1)/b gives the ceiling

	perm := rand.Perm(len(sdb.Shards)) // randomized, then select

	for i := 0; i < numShardsToCheck; i++ {
		// clean only one random shard at a time
		shardIndex := perm[i]
		shard := sdb.Shards[shardIndex]
		// fmt.Println("purging shard: " + strconv.Itoa(shardIndex))

		for { // check a particular shard
			shard.mu.Lock()

			expiredCnt := 0
			checkedCnt := 0

			for key, obj := range shard.Data {
				if checkedCnt >= maxSampleSize {
					break
				}
				checkedCnt++

				if !obj.Expires.IsZero() && time.Now().After(obj.Expires) {
					delete(shard.Data, key)
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
				break
			}

			// SAFETY CHECK: don't linger too long in a purge
			if time.Since(startTime) > maxDuration {
				return
			}

			// otherwise, continue working on this shard, cuz this shard is DIRTYY!!
		}
	}
}
