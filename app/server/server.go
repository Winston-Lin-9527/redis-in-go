package server

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/Winston-Lin-9527/redis-in-go/app/commands"
	"github.com/Winston-Lin-9527/redis-in-go/app/config"
	"github.com/Winston-Lin-9527/redis-in-go/app/persistence"
	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
	"github.com/Winston-Lin-9527/redis-in-go/app/store"
)

// RedisServer is the main server struct
type RedisServer struct {
	db      *store.ShardedRedisDB
	clients map[uint64]*Client
	aof     *persistence.Aof
	rdb     *persistence.RDB
	config  *config.Config
}

// NewRedisServer creates a new Redis server
func NewRedisServer() *RedisServer {
	return &RedisServer{
		clients: make(map[uint64]*Client),
		config:  config.DefaultConfig(),
	}
}

// SetConfig sets a configuration value, for main.go to use
func (rs *RedisServer) SetConfig(key, value string) error {
	return rs.config.Set(key, value)
}

// Run starts the server on the given port
func (rs *RedisServer) Run() {
	port := rs.config.Get("port") // already set by main.go

	listenAddress := fmt.Sprintf("0.0.0.0:%s", port)
	l, err := net.Listen("tcp", listenAddress)
	if err != nil {
		fmt.Println("Failed to bind to port " + port)
		os.Exit(1)
	}
	fmt.Println("Listening on port " + port)

	// storage: db
	redisdb := store.NewShardedRedisDB()
	rs.db = redisdb

	// storage: rdb - load from disk before starting janitor
	rs.rdb = persistence.NewRDB(rs.config)
	if err := rs.rdb.LoadRDB(func(key, val string, expires time.Time) {
		if err := rs.db.SetKey(key, val, expires); err != nil {
			fmt.Println("Error loading key from RDB: ", err.Error())
		}
	}); err != nil {
		fmt.Println("Warning: Could not load RDB file: ", err.Error())
	}

	redisdb.StartJanitor()

	// storage: aof
	// Check config for AOF
	if rs.config.Get("aofEnabled") == "yes" {
		aof, err := persistence.NewAof(rs.config.Get("appendfilename"))
		if err != nil {
			fmt.Println("Failed to create AOF file: ", err.Error())
			// Use panic or exit? Original code panicked on NewAof error, let's stick to panic or strict failure
			panic(err)
		}
		rs.aof = aof

		// Reconstruct from AOF
		rs.aof.Reconstruct(func(v protocol.Value) {
			cmd, errval := SelectCommand(v.Array[0].Bulk, v.Array[1:])
			if errval.Typ == protocol.TypeError {
				fmt.Println("Error selecting command from AOF: ", errval.Str)
				return
			}

			ctx := &commands.CommandContext{
				DB:     rs.db,
				Config: rs.config,
			}
			result := cmd.Handler(v.Array, ctx)
			if result.Typ == protocol.TypeError {
				fmt.Println("Error executing command from AOF: ", result.Str)
				return
			}
		})

		go rs.aof.StartSyncLoop()
		defer rs.aof.CloseAof()
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}
		// register the client
		client := NewClient(conn)
		rs.clients[client.id] = client
		go rs.handleConnection(client)
	}
}

func (rs *RedisServer) handleConnection(client *Client) {
	defer client.conn.Close()

	resp := protocol.NewRespReader(client.conn)
	writer := protocol.NewRespWriter(client.conn)

	// main event loop
	for {
		timeoutDuration := 5 * time.Minute // Increased timeout for debugging/usability
		client.conn.SetReadDeadline(time.Now().Add(timeoutDuration))

		value, err := resp.Read()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Don't spam logs with timeout in dev/test often, but it's fine
				// fmt.Println("Connection timed out (idle for too long)")
				return
			}
			if errors.Is(err, io.EOF) {
				// fmt.Println("Client disconnected")
				return
			}
			fmt.Println("Error reading request: ", err.Error())
			return
		}

		if value.Typ != protocol.TypeArray {
			fmt.Println("Error: expected array type, got: ", value.Typ)
			return
		}

		if len(value.Array) < 1 {
			fmt.Println("Error: expected at least 1 element, got: ", len(value.Array))
			return
		}

		command := value.Array[0].Bulk
		args := value.Array[1:]

		cmd, errVal := SelectCommand(command, args)
		if errVal.Typ == protocol.TypeError {
			writer.Write(errVal)
			fmt.Printf("Error selecting command: %s\n", errVal.Str)
			return
		}

		ctx := &commands.CommandContext{
			DB:     rs.db,
			Config: rs.config,
		}
		// the actual command execution
		result := cmd.Execute(args, ctx)

		writer.Write(result)

		currentTime := time.Now().Format(time.RFC3339)
		fmt.Printf("[%s] Executed command: %s, args: %+v, result: %v\n", currentTime, command, args, result)

		if resp.Buffered() == 0 {
			writer.Flush()
		}

		// Write to AOF
		if rs.aof != nil && (cmd.Flags&commands.FLAG_WRITE != 0) {
			rs.aof.WriteCommand(value)
		}
	}
}
