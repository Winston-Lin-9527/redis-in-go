package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// RESP protocol constants
const (
	STRING      = '+'
	ERROR       = '-'
	INTEGER     = ':'
	BULK_STRING = '$'
	ARRAY       = '*'
)

// ValueType represents the type of RESP value
type ValueType string

const (
	TypeArray  ValueType = "array"
	TypeBulk   ValueType = "bulk"
	TypeString ValueType = "string"
	TypeError  ValueType = "error"
	TypeInt    ValueType = "integer"
	TypeNull   ValueType = "null"
)

// Value represents a RESP value
type Value struct {
	Typ   ValueType
	Str   string
	Num   string
	Bulk  string
	Array []Value
}

// RespReader reads RESP protocol (RESP -> Value)
type RespReader struct {
	reader *bufio.Reader
}

// NewRespReader creates a new RespReader
func NewRespReader(rd io.Reader) *RespReader {
	return &RespReader{reader: bufio.NewReader(rd)}
}

func (r *RespReader) readLine() (line []byte, n int, err error) {
	line, err = r.reader.ReadBytes('\n')
	if err != nil {
		return nil, 0, err
	}

	n = len(line)
	if n >= 2 && line[len(line)-2] == '\r' {
		return line[:len(line)-2], len(line), nil
	}

	// fallback for \n, but not preceded by \r (non-compliant RESP)
	return line[:n-1], n, nil
}

func (r *RespReader) readInteger() (x int, n int, err error) {
	line, n, err := r.readLine()
	if err != nil {
		return 0, 0, err
	}
	i64, err := strconv.ParseInt(string(line), 10, 64)
	if err != nil {
		return 0, n, err
	}
	return int(i64), n, nil
}

func (r *RespReader) readArray() (Value, error) {
	v := Value{}
	v.Typ = TypeArray

	array_size, _, err := r.readInteger()
	if err != nil {
		return v, err
	}

	v.Array = make([]Value, array_size)
	for i := 0; i < int(array_size); i++ {
		val, err := r.Read()
		if err != nil {
			return v, err
		}
		v.Array[i] = val
	}

	return v, nil
}

func (r *RespReader) readBulk() (Value, error) {
	v := Value{}
	v.Typ = TypeBulk

	bulk_size, _, err := r.readInteger()
	if err != nil {
		return v, err
	}

	bulk := make([]byte, bulk_size)

	// read until bulk_size bytes are read, assumes integrity of the input request
	_, err = io.ReadFull(r.reader, bulk)
	if err != nil {
		return v, err
	}

	v.Bulk = string(bulk)

	r.readLine() // trailing \r\n

	return v, nil
}

// Read reads a single RESP command
func (r *RespReader) Read() (Value, error) {
	// first byte is the data type
	data_type, err := r.reader.ReadByte()
	if err != nil {
		return Value{}, err
	}

	switch data_type {
	case ARRAY:
		return r.readArray()
	case BULK_STRING:
		return r.readBulk()
	default:
		fmt.Printf("Unknown data type: %c", data_type)
		return Value{}, fmt.Errorf("unknown data type: %c", data_type)
	}
}

// Buffered returns the number of bytes that can be read from the current buffer
func (r *RespReader) Buffered() int {
	return r.reader.Buffered()
}

// RespWriter writes RESP protocol (Value -> bytes)
type RespWriter struct {
	writer *bufio.Writer
}

// NewRespWriter creates a new RespWriter
func NewRespWriter(w io.Writer) *RespWriter {
	return &RespWriter{writer: bufio.NewWriter(w)}
}

// Write writes a Value to the underlying writer
func (w *RespWriter) Write(v Value) error {
	bs, err := v.ToBytes()
	if err != nil {
		return err
	}

	_, err = w.writer.Write(bs)
	if err != nil {
		return err
	}

	return nil
}

// Flush flushes the underlying buffer
func (w *RespWriter) Flush() error {
	return w.writer.Flush()
}

// ToBytes converts a Value to RESP bytes
func (v *Value) ToBytes() ([]byte, error) {
	switch v.Typ {
	case TypeString:
		return v.stringToBytes(), nil
	case TypeBulk:
		return v.bulkToBytes(), nil
	case TypeArray:
		return v.arrayToBytes()
	case TypeError:
		return v.errorToBytes(), nil
	case TypeNull:
		return v.nullToBytes(), nil
	default:
		return nil, fmt.Errorf("unknown data type: %s", v.Typ)
	}
}

func (v *Value) stringToBytes() (bs []byte) {
	bs = append(bs, STRING)
	bs = append(bs, v.Str...)
	bs = append(bs, '\r', '\n')
	return bs
}

func (v *Value) bulkToBytes() (bs []byte) {
	bs = append(bs, BULK_STRING)
	bs = append(bs, strconv.Itoa(len(v.Bulk))...)
	bs = append(bs, '\r', '\n')
	bs = append(bs, v.Bulk...)
	bs = append(bs, '\r', '\n')
	return bs
}

func (v *Value) arrayToBytes() (bs []byte, err error) {
	bs = append(bs, ARRAY)
	bs = append(bs, strconv.Itoa(len(v.Array))...)
	bs = append(bs, '\r', '\n')

	for i := 0; i < len(v.Array); i++ {
		unfolded_array, err := v.Array[i].ToBytes()
		if err != nil {
			return nil, err
		}
		bs = append(bs, unfolded_array...)
	}

	return bs, nil
}

func (v *Value) errorToBytes() (bs []byte) {
	bs = append(bs, ERROR)
	bs = append(bs, v.Str...)
	bs = append(bs, '\r', '\n')
	return bs
}

func (v *Value) nullToBytes() (bs []byte) {
	return []byte("$-1\r\n")
}
