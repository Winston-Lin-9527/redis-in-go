package main

import (
	"fmt"
	"net"
	"os"
)

// Ensures gofmt doesn't remove the "net" and "os" imports in stage 1 (feel free to remove this!)
var _ = net.Listen
var _ = os.Exit

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	fmt.Println("Logs from your program will appear here!")

	//Uncomment the code below to pass the first stage
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

	resp := NewResp(conn)
	value, err := resp.Read()
	if err != nil {
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
	writer := NewWriter(conn)

	handler, errVal := selectCommand(command, args)
	if errVal.typ == TypeError {
		writer.Write(errVal)
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
