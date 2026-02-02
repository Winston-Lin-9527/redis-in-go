package persistence

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Winston-Lin-9527/redis-in-go/app/protocol"
)

// Aof represents the Append-Only File for persistence
type Aof struct {
	file     *os.File
	rd       *protocol.RespReader
	wr       *protocol.RespWriter
	muLock   sync.Mutex
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewAof creates a new AOF handler
func NewAof(path string) (*Aof, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	aof := Aof{
		file:     f,
		rd:       protocol.NewRespReader(f),
		wr:       protocol.NewRespWriter(f),
		muLock:   sync.Mutex{},
		stopChan: make(chan struct{}),
	}

	aof.wg.Add(1)

	return &aof, nil
}

// StartSyncLoop starts the background sync loop
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

// CloseAof closes the AOF file
func (a *Aof) CloseAof() error {
	close(a.stopChan)
	a.wg.Wait()

	a.muLock.Lock()
	a.wr.Flush()
	a.muLock.Unlock()

	return a.file.Close()
}

// WriteCommand writes a command to the AOF
func (a *Aof) WriteCommand(v protocol.Value) error {
	a.muLock.Lock()
	defer a.muLock.Unlock()

	return a.wr.Write(v)
}

// Reconstruct replays commands from the AOF file
func (a *Aof) Reconstruct(callback func(v protocol.Value)) error {
	a.muLock.Lock()
	defer a.muLock.Unlock()

	num_command_loaded := 0
	v, err := a.rd.Read()
	if err == io.EOF {
		fmt.Fprintf(os.Stderr, "AOF file is empty, but that's ok! Just created one.\n")
		return nil
	}
	_ = v // consume first read

	for {
		v, err = a.rd.Read()
		if err == io.EOF {
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
