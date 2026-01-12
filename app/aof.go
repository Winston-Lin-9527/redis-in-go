package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Aof struct {
	file   *os.File
	rd     *RespReader
	wr     *RespWriter
	muLock sync.Mutex
}

// note this is not a class method
func NewAof(path string) (*Aof, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	aof := Aof{
		file:   f, // save it for closing at the end
		rd:     NewRespReader(f),
		wr:     NewRespWriter(f),
		muLock: sync.Mutex{},
	}

	// sync every second
	go func() {
		for {
			aof.muLock.Lock()

			aof.wr.Flush() // flush the write buffer to the disk

			aof.muLock.Unlock()

			time.Sleep(time.Second * 5)
		}
	}()

	return &aof, nil
}

func (a *Aof) CloseAof() error {
	a.muLock.Lock()
	defer a.muLock.Unlock()

	a.wr.Flush()

	return a.file.Close()
}

func (a *Aof) WriteCommand(v Value) error {
	a.muLock.Lock()
	defer a.muLock.Unlock()

	return a.wr.Write(v)
}

// assumes AOF valid
func (a *Aof) Reconstruct(callback func(v Value)) error {
	a.muLock.Lock()
	defer a.muLock.Unlock()

	for {
		v, err := a.rd.Read()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read command from AOF: %v\n", err)
			break
		}

		if err == io.EOF {
			break // finished reading the AOF
		}

		callback(v)
	}

	return nil
}
