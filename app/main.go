package main

import (
	"flag"
	"net"
	"os"

	"github.com/Winston-Lin-9527/redis-in-go/app/server"
)

// Ensures gofmt doesn't remove the "net" and "os" imports in stage 1
var _ = net.Listen
var _ = os.Exit

func main() {
	portStrPtr := flag.String("port", "6379", "Port to listen on")
	flag.Parse()

	redisServer := server.NewRedisServer()
	redisServer.Run(*portStrPtr)
}
