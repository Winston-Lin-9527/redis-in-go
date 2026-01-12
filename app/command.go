package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type Handler func(args []Value) Value

// function closures as factory
var Handlers = map[string]func() Command{
	"COMMAND": func() Command { return &NoOpCommand{} },
	"SET":     func() Command { return &SetCommand{} },
	"GET":     func() Command { return &GetCommand{} },
	"PING":    func() Command { return &PingCommand{} },
	"HSET":    func() Command { return &HSetCommand{} },
	"HGET":    func() Command { return &HGetCommand{} },
}

type Command interface {
	Init(args []Value) // custom arg handling logic
	Execute() Value    // custom execution logic
}

// Handle CLI interactive mode: do nothing as the user input is just an enter with no content
type NoOpCommand struct {
}

func (c *NoOpCommand) Init(args []Value) {
}

func (c *NoOpCommand) Execute() Value {
	return Value{typ: TypeString, str: "OK"}
}

var SETs = map[string]string{}
var SETsMu = sync.RWMutex{}

type SetCommand struct {
	db    string // unused, later could specify which db to set
	key   string
	value string
	err   string
}

func (c *SetCommand) Init(args []Value) {
	if len(args) != 2 {
		c.err = "ERR wrong number of arguments for 'set' command, expected 2, received " + strconv.Itoa(len(args))
		return
	}
	c.key = args[0].bulk
	c.value = args[1].bulk
}

func (c *SetCommand) Execute() Value {
	if c.err != "" {
		return Value{typ: TypeError, str: c.err}
	}

	SETsMu.Lock()
	SETs[c.key] = c.value
	SETsMu.Unlock()

	return Value{typ: TypeString, str: "OK"}
}

type GetCommand struct {
	db  string
	key string
	err string
}

func (c *GetCommand) Init(args []Value) {
	if len(args) != 1 {
		c.err = "ERR wrong number of arguments for 'get' command, expected 1, received " + strconv.Itoa(len(args))
		return
	}
	c.key = args[0].bulk
}

func (c *GetCommand) Execute() Value {
	if c.err != "" {
		return Value{typ: TypeError, str: c.err}
	}

	fmt.Println("Getting key: ", c.key)

	SETsMu.RLock()
	value, ok := SETs[c.key]
	SETsMu.RUnlock()

	if !ok {
		return Value{typ: TypeNull}
	}

	return Value{typ: TypeBulk, bulk: value}
}

var HSETs = map[string]map[string]string{}
var HSETsMu = sync.RWMutex{}

type HSetCommand struct {
	hash  string
	key   string
	value string
	err   string
}

func (c *HSetCommand) Init(args []Value) {
	if len(args) != 3 {
		c.err = "ERR wrong number of arguments for 'hset' command, expected 3, received " + strconv.Itoa(len(args))
		return
	}
	c.hash = args[0].bulk
	c.key = args[1].bulk
	c.value = args[2].bulk
}

func (c *HSetCommand) Execute() Value {
	if c.err != "" {
		return Value{typ: TypeError, str: c.err}
	}

	HSETsMu.Lock()
	if _, ok := HSETs[c.hash]; !ok { // if hashmap doesn't exist, create it
		HSETs[c.hash] = make(map[string]string)
	}

	HSETs[c.hash][c.key] = c.value
	HSETsMu.Unlock()

	return Value{typ: TypeString, str: "OK"}
}

type HGetCommand struct {
	hash string
	key  string
	err  string
}

func (c *HGetCommand) Init(args []Value) {
	if len(args) != 2 {
		c.err = "ERR wrong number of arguments for 'hget' command, expected 2, received " + strconv.Itoa(len(args))
		return
	}
	c.hash = args[0].bulk
	c.key = args[1].bulk
}

func (c *HGetCommand) Execute() Value {
	if c.err != "" {
		return Value{typ: TypeError, str: c.err}
	}

	HSETsMu.RLock()
	value, ok := HSETs[c.hash][c.key]
	HSETsMu.RUnlock()

	if !ok {
		return Value{typ: TypeNull}
	}

	return Value{typ: TypeBulk, bulk: value}
}

type PingCommand struct {
	arg string
}

func (c *PingCommand) Init(args []Value) {
	if len(args) != 0 {
		c.arg = args[0].bulk
	}
}

func (c *PingCommand) Execute() Value {
	if c.arg != "" {
		return Value{typ: TypeString, str: c.arg}
	}

	return Value{typ: TypeString, str: "Pong"}
}

func selectCommand(command string, args []Value) (cmd Command, err Value) {
	fmt.Printf("Selected command: %s, args: %v\n", command, args)
	cmd_ptr := Handlers[strings.ToUpper(command)] // call factory (a function closure) to generate a new instance
	if cmd_ptr == nil {
		fmt.Println("Unknown command: ", command)
		return nil, Value{typ: TypeError, str: "ERR unknown command '" + command + "'"}
	}

	cmd = cmd_ptr()
	cmd.Init(args)
	return cmd, Value{typ: TypeString, str: "OK"}
}
