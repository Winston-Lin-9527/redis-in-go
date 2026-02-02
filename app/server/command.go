package server

import (
	"fmt"
	"strings"

	"github.com/Winston-Lin-9527/redis-in-go/app/commands"
	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
)

// Handlers is the command registry
var Handlers = map[string]*commands.Command{
	"COMMAND": {Name: "command", Handler: NoOp, Arity: -1, Flags: commands.FLAG_READONLY},
	"PING":    {Name: "ping", Handler: Ping, Arity: -1, Flags: commands.FLAG_READONLY},
	"SET":     {Name: "set", Handler: commands.Set, Arity: -3, Flags: commands.FLAG_WRITE},
	"GET":     {Name: "get", Handler: commands.Get, Arity: 2, Flags: commands.FLAG_READONLY},
	"HSET":    {Name: "hset", Handler: commands.HSet, Arity: 4, Flags: commands.FLAG_WRITE},
	"HGET":    {Name: "hget", Handler: commands.HGet, Arity: 3, Flags: commands.FLAG_READONLY},
}

// NoOp handles CLI interactive mode and COMMAND command
func NoOp(args []protocol.Value, ctx *commands.CommandContext) protocol.Value {
	return protocol.Value{Typ: protocol.TypeString, Str: "OK"}
}

// Ping handles PING
func Ping(args []protocol.Value, ctx *commands.CommandContext) protocol.Value {
	if len(args) > 1 {
		return protocol.Value{Typ: protocol.TypeString, Str: args[1].Bulk}
	}
	return protocol.Value{Typ: protocol.TypeString, Str: "PONG"}
}

// SelectCommand finds and validates the appropriate command
func SelectCommand(commandName string, args []protocol.Value) (cmd *commands.Command, err protocol.Value) {
	cmd, ok := Handlers[strings.ToUpper(commandName)]
	if !ok {
		return nil, protocol.Value{Typ: protocol.TypeError, Str: fmt.Sprintf("ERR unknown command '%s'", commandName)}
	}

	// Validate Arity
	// Note: args passed here excludes the command name, but Redis arity counts it.
	totalArgs := len(args) + 1

	if cmd.Arity > 0 {
		if totalArgs != cmd.Arity {
			return nil, protocol.Value{Typ: protocol.TypeError, Str: "ERR wrong number of arguments for '" + cmd.Name + "' command"}
		}
	} else if cmd.Arity < 0 {
		if totalArgs < -cmd.Arity {
			return nil, protocol.Value{Typ: protocol.TypeError, Str: "ERR wrong number of arguments for '" + cmd.Name + "' command"}
		}
	}

	return cmd, protocol.Value{Typ: protocol.TypeString, Str: "OK"}
}
