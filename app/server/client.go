package server

import (
	"math/rand"
	"net"
	"time"
)

// ClientFlags represents various client states
type ClientFlags uint8

const (
	CLIENT_REPLICA ClientFlags = 1 << iota // This is a replica connection
	CLIENT_MASTER                          // This is our master (we're a replica)
	CLIENT_PUBSUB                          // In pub/sub mode (can't do normal commands)
)

// Client represents a connected Redis client
type Client struct {
	conn    net.Conn
	id      uint64
	flags   ClientFlags
	lastCmd time.Time
}

// NewClient creates a new Client from a connection
func NewClient(conn net.Conn) *Client {
	return &Client{
		conn:    conn,
		id:      rand.Uint64(),
		lastCmd: time.Now(),
	}
}
