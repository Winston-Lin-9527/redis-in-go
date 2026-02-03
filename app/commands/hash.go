package commands

import (
	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
)

// HSet handles the HSET command
func HSet(args []protocol.Value, ctx *CommandContext) protocol.Value {
	// Args: [HSET, key, field, value]
	if len(args) < 3 {
		return protocol.Value{Typ: protocol.TypeError, Str: "HSET command requires at least 3 arguments"}
	}

	key := args[0].Bulk
	field := args[1].Bulk
	value := args[2].Bulk

	if err := ctx.DB.HSetKey(key, field, value); err != nil {
		return protocol.Value{Typ: protocol.TypeError, Str: err.Error()}
	}

	return protocol.Value{Typ: protocol.TypeString, Str: "OK"}
}

// HGet handles the HGET command
func HGet(args []protocol.Value, ctx *CommandContext) protocol.Value {
	// Args: [HGET, hash, key]
	hash := args[0].Bulk
	key := args[1].Bulk

	value, err := ctx.DB.HGetKey(hash, key)
	if err != nil {
		return protocol.Value{Typ: protocol.TypeNull}
	}

	return protocol.Value{Typ: protocol.TypeBulk, Bulk: value}
}
