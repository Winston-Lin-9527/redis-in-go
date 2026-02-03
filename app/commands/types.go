package commands

import (
	"github.com/Winston-Lin-9527/redis-in-go/app/config"
	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
	"github.com/Winston-Lin-9527/redis-in-go/app/store"
)

// CommandFlags represents command properties
type CommandFlags int

const (
	FLAG_NONE     CommandFlags = 0
	FLAG_WRITE    CommandFlags = 1 << 0 // Command modifies data
	FLAG_READONLY CommandFlags = 1 << 1 // Command only reads data
)

// CommandContext provides context for command execution
type CommandContext struct {
	DB     *store.ShardedRedisDB
	Config *config.Config
}

// CommandHandler is the function signature for command implementation
type CommandHandler func(args []protocol.Value, ctx *CommandContext) protocol.Value

// Command represents a Redis command definition
type Command struct {
	Name    string
	Handler CommandHandler
	Arity   int // Positive: exact match, Negative: >= -Arity
	Flags   CommandFlags
}

func (c *Command) Execute(args []protocol.Value, ctx *CommandContext) protocol.Value {
	return c.Handler(args, ctx)
}
