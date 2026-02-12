package commands

import (
	"strings"

	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
)

// Config handles CONFIG GET and CONFIG SET
func Config(args []protocol.Value, ctx *CommandContext) protocol.Value {
	// Args: [CONFIG, GET/SET, payload...]
	if len(args) < 2 {
		return protocol.Value{Typ: protocol.TypeError, Str: "Config command requires at least 2 arguments"}
	}

	op := strings.ToUpper(args[0].Bulk)

	switch op {
	case "GET":
		parameter := args[1].Bulk
		value := ctx.Config.Get(parameter)
		// todo checks and if multiple params are fetched at a time

		return protocol.Value{
			Typ: protocol.TypeArray,
			Array: []protocol.Value{
				{Typ: protocol.TypeBulk, Bulk: parameter},
				{Typ: protocol.TypeBulk, Bulk: value},
			},
		}
	case "SET":
		if len(args) < 3 {
			return protocol.Value{Typ: protocol.TypeError, Str: "Config set command requires at least 3 arguments"}
		}
		parameter := args[1].Bulk
		value := args[2].Bulk

		err := ctx.Config.Set(parameter, value)
		if err != nil {
			return protocol.Value{Typ: protocol.TypeError, Str: "ERR " + err.Error()}
		}
		return protocol.Value{Typ: protocol.TypeString, Str: "OK"}
	}

	return protocol.Value{Typ: protocol.TypeError, Str: "ERR unknown CONFIG operation"}
}
