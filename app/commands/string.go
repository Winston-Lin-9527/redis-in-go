package commands

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
)

// Set handles the SET command
func Set(args []protocol.Value, ctx *CommandContext) protocol.Value {
	// Args: [SET, key, value, ...]
	// Arity check handled by caller, but we expect at least 3 args (SET, key, value)
	if len(args) < 3 {
		return protocol.Value{Typ: protocol.TypeError, Str: "ERR wrong number of arguments for 'set' command"}
	}

	key := args[1].Bulk
	value := args[2].Bulk
	var expires time.Time
	var errStr string

	if len(args) > 3 {
		// handle options
		for i := 3; i < len(args); i++ {
			opt := args[i].Bulk
			if i+1 >= len(args) {
				return protocol.Value{Typ: protocol.TypeError, Str: "ERR syntax error"}
			}

			valArg := args[i+1].Bulk
			deltaNum, err := strconv.ParseInt(valArg, 10, 0)
			if err != nil {
				return protocol.Value{Typ: protocol.TypeError, Str: "ERR value is not an integer or out of range"}
			}
			i++ // skip value arg

			switch opt {
			case "EX":
				expires = time.Now().Add(time.Duration(deltaNum) * time.Second)
			case "PX":
				expires = time.Now().Add(time.Duration(deltaNum) * time.Millisecond)
			default:
				return protocol.Value{Typ: protocol.TypeError, Str: "ERR syntax error"}
			}
		}
	}

	if errStr != "" {
		return protocol.Value{Typ: protocol.TypeError, Str: errStr}
	}

	fmt.Println("key is set to shard: " + strconv.Itoa(ctx.DB.GetShardIndex(key)))
	if err := ctx.DB.SetKey(key, value, expires); err != nil {
		return protocol.Value{Typ: protocol.TypeError, Str: err.Error()}
	}

	return protocol.Value{Typ: protocol.TypeString, Str: "OK"}
}

// Get handles the GET command
func Get(args []protocol.Value, ctx *CommandContext) protocol.Value {
	// Args: [GET, key]
	key := args[1].Bulk

	redis_obj, err := ctx.DB.GetKey(key)
	if err != nil {
		return protocol.Value{Typ: protocol.TypeNull}
	}

	val_str := redis_obj.PPrint()
	return protocol.Value{Typ: protocol.TypeBulk, Bulk: val_str}
}
