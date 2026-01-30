package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

type Aof struct {
	file   *os.File
	rd     *RespReader
	wr     *RespWriter
	muLock sync.Mutex

	stopChan chan struct{}
	wg       sync.WaitGroup
}

// note this is not a class method
func NewAof(path string) (*Aof, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	aof := Aof{
		file:     f, // save it for closing at the end
		rd:       NewRespReader(f),
		wr:       NewRespWriter(f),
		muLock:   sync.Mutex{},
		stopChan: make(chan struct{}),
	}

	aof.wg.Add(1)

	return &aof, nil
}

func (a *Aof) StartSyncLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopChan:
			return
		case <-ticker.C:
			a.muLock.Lock()
			a.wr.Flush()
			a.muLock.Unlock()
		}
	}
}

func (a *Aof) CloseAof() error {
	close(a.stopChan)
	a.wg.Wait() // blocks until counter reaches 0

	a.muLock.Lock()
	a.wr.Flush()
	a.muLock.Unlock()

	return a.file.Close()
}

func (a *Aof) WriteCommand(v Value) error {
	a.muLock.Lock()
	defer a.muLock.Unlock()

	return a.wr.Write(v)
}

// assumes AOF file valid
func (a *Aof) Reconstruct(callback func(v Value)) error {
	// TODO: potentially AOF sync could happen before reconstruction, although unlikely but its still a risk
	// can pause the sync before reconstruction
	a.muLock.Lock()
	defer a.muLock.Unlock()

	num_command_loaded := 0
	v, err := a.rd.Read()
	if err == io.EOF {
		fmt.Fprintf(os.Stderr, "AOF file is empty, but that's ok! Just created one.\n")
		return nil
	}

	for {
		v, err = a.rd.Read()
		if err == io.EOF { // finished reading the AOF
			fmt.Println("AOF: Loaded " + strconv.Itoa(num_command_loaded) + " commands")
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read command from AOF: %v\n", err)
			break
		}

		callback(v)
		num_command_loaded += 1
	}

	return nil
}
