package commands

import "github.com/Winston-Lin-9527/redis-in-go/app/protocol"

// for generic / key commands like DEL, EXISTS, EXPIRE, RENAME, TTL
// that apply to any key regardless of the type

func Keys(args []protocol.Value, ctx *CommandContext) protocol.Value {
	if len(args) != 1 {
		return protocol.Value{
			Typ: protocol.TypeError,
			Str: "ERR wrong number of arguments for 'KEYS' command",
		}
	}

	pattern := args[0].Bulk

	// Get all matching keys from the sharded database
	keys := ctx.DB.GetAllKeys(pattern)

	// Return as an array response
	values := make([]protocol.Value, len(keys))
	for i, key := range keys {
		values[i] = protocol.Value{
			Typ:  protocol.TypeBulk,
			Bulk: key,
		}
	}

	return protocol.Value{
		Typ:   protocol.TypeArray,
		Array: values,
	}
}
