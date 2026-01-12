package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// Ensures gofmt doesn't remove the "net" and "os" imports in stage 1 (feel free to remove this!)
var _ = net.Listen
var _ = os.Exit

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	resp := NewRespReader(conn)
	writer := NewRespWriter(conn)

	aof, err := NewAof("aof") // todo: make it user/store specific, since each store should have its own AOF file
	if err != nil {
		fmt.Println("Failed to create AOF file: ", err.Error())
		return
	}
	aof.Reconstruct(func(v Value) {
		handler, errval := selectCommand(v.array[0].bulk, v.array[1:])
		if errval.typ == TypeError {
			fmt.Println("Error selecting command from AOF: ", errval.str)
			return
		}

		result := handler.Execute()
		if result.typ == TypeError {
			fmt.Println("Error executing command from AOF: ", result.str)
			return
		}
	})
	defer aof.CloseAof()

	for {
		timeoutDuration := 1 * time.Minute // only waits the client for 1 minute
		conn.SetReadDeadline(time.Now().Add(timeoutDuration))

		// read request
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

		// every RESP must be array type
		if value.typ != TypeArray {
			fmt.Println("Error: expected array type, got: ", value.typ)
			return
		}

		// every RESP must have length at least 1
		if len(value.array) < 1 {
			fmt.Println("Error: expected at least 1 element, got: ", len(value.array))
			return
		}

		fmt.Printf("Received %s\n", value)

		command := value.array[0].bulk
		args := value.array[1:]

		handler, errVal := selectCommand(command, args)
		if errVal.typ == TypeError {
			writer.Write(errVal)
			fmt.Printf("Error selecting command: %s\n", errVal.str)
			return
		}

		result := handler.Execute()

		writer.Write(result) // write response to write buffer

		// support pipelined writes, flush only when no more data to read, reduce syscalls & tcp overheads
		if resp.reader.Buffered() == 0 {
			writer.Flush()
		}

		aof.WriteCommand(value) // write the command, not execution result
	}
}
