package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// all about RESP
const (
	STRING      = '+'
	ERROR       = '-'
	INTEGER     = ':'
	BULK_STRING = '$'
	ARRAY       = '*'
)

// 1. Define the "Enum" type
type ValueType string

// 2. Define the possible values
const (
	TypeArray  ValueType = "array"
	TypeBulk   ValueType = "bulk"
	TypeString ValueType = "string"
	TypeError  ValueType = "error"
	TypeInt    ValueType = "integer"
	TypeNull   ValueType = "null"
)

type Value struct {
	typ   ValueType // the data type, one of the constants defined above
	str   string
	num   string
	bulk  string
	array []Value // nested type
}

// Resp reader (RESP -> Value)
type Resp struct {
	reader *bufio.Reader
}

func NewResp(rd io.Reader) *Resp {
	return &Resp{reader: bufio.NewReader(rd)}
}

func (r *Resp) readLine() (line []byte, n int, err error) {
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

func (r *Resp) readInteger() (x int, n int, err error) {
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

func (r *Resp) readArray() (Value, error) {
	v := Value{}
	v.typ = TypeArray

	array_size, _, err := r.readInteger() // array size is already known
	if err != nil {
		return v, err
	}

	v.array = make([]Value, array_size)
	for i := 0; i < int(array_size); i++ {
		val, err := r.Read()
		if err != nil {
			return v, err
		}
		v.array[i] = val
	}

	return v, nil
}

func (r *Resp) readBulk() (Value, error) {
	v := Value{}
	v.typ = TypeBulk

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

	v.bulk = string(bulk)

	r.readLine() // trailing \r\n

	return v, nil
}

func (r *Resp) Read() (Value, error) {
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

// the reader performs (RESP -> Value), now we need to perform (Value -> RESP in raw bytes)
// assemble bytes from Value objects

type Writer struct {
	writer *bufio.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{writer: bufio.NewWriter(w)}
}

func (w *Writer) Write(v Value) error {
	bs, err := v.toBytes()
	if err != nil {
		return err
	}

	// for _, b := range bs {
	// 	fmt.Printf("%c", b)
	// }

	_, err = w.writer.Write(bs)
	if err != nil {
		return err
	}

	w.writer.Flush() // it's a buffered IO, we MUST flush to move data from bufio down to Go runtime
	return nil
}

func (v *Value) toBytes() ([]byte, error) {
	switch v.typ {
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
		return nil, fmt.Errorf("unknown data type: %s", v.typ)
	}
}

// since unlikely to fail, we can ignore the error
func (v *Value) stringToBytes() (bs []byte) {
	bs = append(bs, STRING)
	bs = append(bs, v.str...)
	bs = append(bs, '\r', '\n')
	return bs
}

// since unlikely to fail, we can ignore the error
func (v *Value) bulkToBytes() (bs []byte) {
	bs = append(bs, BULK_STRING)
	bs = append(bs, strconv.Itoa(len(v.bulk))...)
	bs = append(bs, '\r', '\n')
	bs = append(bs, v.bulk...)
	bs = append(bs, '\r', '\n')

	return bs
}

func (v *Value) arrayToBytes() (bs []byte, err error) {
	bs = append(bs, ARRAY)
	bs = append(bs, strconv.Itoa(len(v.array))...)
	bs = append(bs, '\r', '\n')

	for i := 0; i < len(v.array); i++ {
		unfolded_array, err := v.array[i].toBytes()
		if err != nil {
			return nil, err
		}
		bs = append(bs, unfolded_array...)
	}

	return bs, nil
}

func (v *Value) errorToBytes() (bs []byte) {
	bs = append(bs, ERROR)
	bs = append(bs, v.str...)
	bs = append(bs, '\r', '\n')
	return bs
}

func (v *Value) nullToBytes() (bs []byte) {
	return []byte("$-1\r\n")
}
