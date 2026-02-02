package server

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/Winston-Lin-9527/redis-in-go/app/commands"
	"github.com/Winston-Lin-9527/redis-in-go/app/persistence"
	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
	"github.com/Winston-Lin-9527/redis-in-go/app/store"
)

// RedisServer is the main server struct
type RedisServer struct {
	db      *store.ShardedRedisDB
	clients map[uint64]*Client
	aof     *persistence.Aof
}

// NewRedisServer creates a new Redis server
func NewRedisServer() *RedisServer {
	return &RedisServer{clients: make(map[uint64]*Client)}
}

// Run starts the server on the given port
func (rs *RedisServer) Run(port string) {
	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", port))
	if err != nil {
		fmt.Println("Failed to bind to port " + port)
		os.Exit(1)
	}
	fmt.Println("Listening on port " + port)

	// storage: db
	redisdb := store.NewShardedRedisDB()
	rs.db = redisdb
	redisdb.StartJanitor()

	// storage: aof
	aof, err := persistence.NewAof("aof")
	if err != nil {
		panic(err)
	}
	rs.aof = aof

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

	aof, err := persistence.NewAof("aof")
	if err != nil {
		fmt.Println("Failed to create AOF file: ", err.Error())
		return
	}
	aof.Reconstruct(func(v protocol.Value) {
		cmd, errval := SelectCommand(v.Array[0].Bulk, v.Array[1:])
		if errval.Typ == protocol.TypeError {
			fmt.Println("Error selecting command from AOF: ", errval.Str)
			return
		}

		ctx := &commands.CommandContext{DB: rs.db}
		result := cmd.Handler(v.Array, ctx)
		if result.Typ == protocol.TypeError {
			fmt.Println("Error executing command from AOF: ", result.Str)
			return
		}
	})
	go aof.StartSyncLoop()
	defer aof.CloseAof()

	for {
		timeoutDuration := 1 * time.Minute
		client.conn.SetReadDeadline(time.Now().Add(timeoutDuration))

		value, err := resp.Read()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("Connection timed out (idle for too long)")
				return
			}
			if errors.Is(err, io.EOF) {
				fmt.Println("Client disconnected")
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

		ctx := &commands.CommandContext{DB: rs.db}
		result := cmd.Execute(args, ctx)

		writer.Write(result)

		if resp.Buffered() == 0 {
			writer.Flush()
		}

		// Write to AOF
		if cmd.Flags&commands.FLAG_WRITE != 0 {
			aof.WriteCommand(value)
		}
	}
}
