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
	dirPtr := flag.String("dir", ".", "Directory for RDB/AOF files")
	dbfilenamePtr := flag.String("dbfilename", "dump.rdb", "RDB filename")
	flag.Parse()

	redisServer := server.NewRedisServer()

	// Update config from flags
	redisServer.SetConfig("port", *portStrPtr)
	redisServer.SetConfig("dir", *dirPtr)
	redisServer.SetConfig("dbfilename", *dbfilenamePtr)

	redisServer.Run()
}
