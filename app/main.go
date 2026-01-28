package main

import (
	"flag"
	"fmt"
	"net"
	"os"
)

// Ensures gofmt doesn't remove the "net" and "os" imports in stage 1 (feel free to remove this!)
var _ = net.Listen
var _ = os.Exit

func main() {
	portStrPtr := flag.String("port", "6379", "Port to listen on")
	flag.Parse()

	redisdb := NewShardedRedisDB()
	redisdb.StartJanitor()
	redisServer := NewRedisServer(redisdb)

	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", *portStrPtr))
	if err != nil {
		fmt.Println("Failed to bind to port " + *portStrPtr)
		os.Exit(1)
	}

	fmt.Println("Listening on port " + *portStrPtr)

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}
		go redisServer.handleConnection(conn)
	}
}
