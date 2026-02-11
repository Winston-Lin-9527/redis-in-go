package persistence

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/Winston-Lin-9527/redis-in-go/app/config"
)

const RDB_MAX_STRING_LEN = 1 << 24 // strings in rdb file can be at most 16MB which is beyond reasonable..

const (
	METADATA_OPCODE = 0xFA
	DATABASE_OPCODE = 0xFE
	EOF_OPCODE      = 0xFF
)

type RDB struct {
	dir        string
	dbfilename string

	reader *bufio.Reader
}

// not usable until LoadRDB() is called
func NewRDB(config *config.Config) *RDB {
	dir := config.Get("dir")
	dbfilename := config.Get("dbfilename")
	return &RDB{dir: dir, dbfilename: dbfilename}
}

// header struct of the RDB file, contains the magic string "REDIS" and the version number (currently "0011")
type rdbHeader struct {
	Magic   [5]byte // "REDIS"
	Version [4]byte // "0011"
}

func (rdb *RDB) LoadRDB(onKeyValue func(key, val string, expires time.Time)) error {
	rdbFilePath := path.Join(rdb.dir, rdb.dbfilename)
	file, err := os.Open(rdbFilePath)
	if err != nil {
		fmt.Println("RDB file not found at: " + rdbFilePath)
		return err
	}
	defer file.Close()

	rdb.reader = bufio.NewReader(file)

	if err := rdb.read(onKeyValue); err != nil {
		return err
	}

	return nil
}

func (rdb *RDB) read(onKeyValue func(key, val string, expires time.Time)) error {
	// Sections of a RDB file
	// 1. Header section
	if err := rdb.readHeaderSection(); err != nil {
		return err
	}

	for {
		opCode, err := rdb.reader.Peek(1)

		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("Error reading RDB opcode: %s", err)
		}

		switch opCode[0] {
		case METADATA_OPCODE: // 2. Metadata section
			if err := rdb.readMetadataSection(); err != nil {
				return err
			}
		case DATABASE_OPCODE: // 3. Database section
			if err := rdb.readDatabase(onKeyValue); err != nil {
				return err
			}
		case EOF_OPCODE: // 4. EOF section
			fmt.Println("RDB EOF opcode reached")
			if err := rdb.readEOFSection(); err != nil {
				return err
			}
			return nil // EOF reached, we're done reading the RDB file
		default:
			return fmt.Errorf("Invalid RDB opcode: %d", opCode)
		}
	}

	return nil
}

func (rdb *RDB) readHeaderSection() error {
	var header rdbHeader
	if err := binary.Read(rdb.reader, binary.LittleEndian, &header); err != nil {
		return err
	}

	if string(header.Magic[:]) != "REDIS" {
		return fmt.Errorf("Invalid RDB magic: %s", header.Magic)
	}
	version, err := strconv.Atoi(string(header.Version[:]))
	if err != nil {
		return fmt.Errorf("Invalid RDB version: %s", header.Version)
	}

	fmt.Printf("RDB Version: %d\n", version)
	return nil
}

// reads in a metadata section
func (rdb *RDB) readMetadataSection() error {
	rdb.reader.Discard(1) // consume that 0xFA we peeked earlier

	key, err := readRdbString(rdb.reader)
	if err != nil {
		return err
	}
	val, err := readRdbString(rdb.reader)
	if err != nil {
		return err
	}

	fmt.Printf("Metadata: %s -> %s\n", key, val)

	return nil
}

const (
	DB_STRING_KV_PAIR_OPCODE    = 0x00 // just a normal string-type kv pair
	DB_HT_SZ_OPCODE             = 0xFB // hashtable size info follows
	DB_KEY_EXPIRE_MILLI_OPCODE  = 0xFC // key expire time in milliseconds follows
	DB_KEY_EXPIRE_SECOND_OPCODE = 0xFD // key expire time in seconds follows
)

// reads in a database section, which contains multiple KV pairs and ends when we reach the next database section or EOF
func (rdb *RDB) readDatabase(onKeyValue func(key, val string, expires time.Time)) error {
	rdb.reader.Discard(1) // consume the 0xFE opcode that we peeked earlier in read()

	db_index, _, err := readRdbLength(rdb.reader)
	if err != nil {
		return err
	}

	fmt.Printf("Database index: %d\n", db_index)

	for {
		opCode, err := rdb.reader.Peek(1)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		currByte := opCode[0]

		// if we reach the next database section or EOF, break out of this loop and return to read() to handle it
		if currByte == DATABASE_OPCODE || currByte == EOF_OPCODE {
			break
		}

		switch currByte {
		case DB_HT_SZ_OPCODE: // Hashtable size information section, contains the count of KV pairs and the count of expiring KV pairs in this database
			rdb.reader.Discard(1) // consume that 0xFB we peeked earlier
			kv_count, _, err := readRdbLength(rdb.reader)
			if err != nil {
				return err
			}
			kv_expire_count, _, err := readRdbLength(rdb.reader)
			if err != nil {
				return err
			}
			fmt.Printf("KV count: %d, KV expire count: %d\n", kv_count, kv_expire_count)
		case DB_STRING_KV_PAIR_OPCODE: // just a normal string-type kv pair
			key, val, err := readKVPair(rdb.reader)
			if err != nil {
				return err
			}
			fmt.Printf("KV pair: %s -> %s\n", key, val)
			onKeyValue(key, val, time.Time{}) // No expiration

		case DB_KEY_EXPIRE_MILLI_OPCODE: // expirable keys that are measured in milliseconds
			rdb.reader.Discard(1) // consume that 0xFC we peeked earlier
			var expireTime uint64 // 8bytes, uint64 little-endian encoding
			err = binary.Read(rdb.reader, binary.LittleEndian, &expireTime)
			if err != nil {
				return err
			}

			key, val, err := readKVPair(rdb.reader)
			if err != nil {
				return err
			}
			fmt.Printf("KV pair: %s -> %s (expires: %d ms)\n", key, val, expireTime)
			expires := time.UnixMilli(int64(expireTime))
			onKeyValue(key, val, expires)

		case DB_KEY_EXPIRE_SECOND_OPCODE: // expirable keys that are measured in seconds
			rdb.reader.Discard(1) // consume that 0xFD we peeked earlier
			var expireTime uint32 // 4bytes, uint32 little-endian encoding
			err = binary.Read(rdb.reader, binary.LittleEndian, &expireTime)
			if err != nil {
				return err
			}

			key, val, err := readKVPair(rdb.reader)
			if err != nil {
				return err
			}
			fmt.Printf("KV pair: %s -> %s (expires: %d s)\n", key, val, expireTime)
			expires := time.Unix(int64(expireTime), 0)
			onKeyValue(key, val, expires)
		default:
			return fmt.Errorf("Unexpected RDB byte: 0x%02x", currByte)
		}
	}
	return nil
}

// reads in an EOF section
func (rdb *RDB) readEOFSection() error {
	rdb.reader.Discard(1) // consume the 0xFF opcode that we peeked earlier

	// an 8-byte CRC64 checksum follows
	var checksum uint64
	if err := binary.Read(rdb.reader, binary.LittleEndian, &checksum); err != nil {
		return err
	}

	fmt.Printf("RDB Checksum: 0x%016x\n", checksum)
	// todo: verify the checksum

	return nil
}

// just read in a normal KV pair (including the value type byte)
func readKVPair(r *bufio.Reader) (string, string, error) {
	valType, err := r.ReadByte()
	if err != nil {
		return "", "", err
	}
	if valType != 0 {
		return "", "", fmt.Errorf("Unsupported value type %d", valType)
	}

	key, err := readRdbString(r)
	if err != nil {
		return "", "", err
	}
	val, err := readRdbString(r)
	if err != nil {
		return "", "", err
	}
	return key, val, nil
}

func readRdbString(r *bufio.Reader) (string, error) {
	length, isEncoded, err := readRdbLength(r)
	if err != nil {
		return "", err
	}

	if isEncoded {
		// LZF compression
		return readEncodedRdbString(r, length) // in the case of LZF, length is the type of encoding
	}

	// not encoded, read raw bytes
	if length > RDB_MAX_STRING_LEN {
		return "", fmt.Errorf("String length too large: %d", length)
	}

	ret_val := make([]byte, length)
	if _, err := io.ReadFull(r, ret_val); err != nil {
		return "", err
	}

	return string(ret_val), nil
}

// readRdbLength reads/parses a length/size-encoded integer length from the RDB file
// Returns:
// - uint64: the length
// - bool: true if the next object is a special encoding rather than raw bytes
// - error: if any error occurs
func readRdbLength(r *bufio.Reader) (uint64, bool, error) {
	firstByte, err := r.ReadByte()
	if err != nil {
		return 0, false, err
	}

	// isolate the top 2 bits (flag)
	flag := (firstByte & 0xC0) >> 6

	switch flag {
	case 0: // 0x00......: 6 bit length, 8-bit integer follows
		return uint64(firstByte & 0x3F), false, nil
	case 1: // 0x01......: 14 bit length, combine next 6 bits with the next byte, 6+8=14, 16-bit integer follows
		secondByte, err := r.ReadByte()
		if err != nil {
			return 0, false, err
		}
		length := uint64(firstByte&0x3F)<<8 | uint64(secondByte) // big-endian byte arrangement
		return length, false, nil
	case 2: // 0x10......: 32 bit length, uint32 big-endian encoding, 32-bit integer follows
		var length uint32
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return 0, false, err
		}
		return uint64(length), false, nil
	case 3: // 0x11......: special encoding, LZF compression string follows
		// in this case, the remaining 6 bits are the type of encoding, not string length
		return uint64(firstByte & 0x3F), true, nil
	default:
		return 0, false, fmt.Errorf("Invalid RDB length flag: %d", flag)
	}
}

// RDB style string encoding, it's an internal helper function
func readEncodedRdbString(r *bufio.Reader, encodeType uint64) (string, error) {
	switch encodeType { // this deals with the encoding types, in the 0x11 case (case 3 in readRdbLength), the remaining 6 bits are the type of encoding, not string length
	case 0: // 8-bit integer
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(int64(int8(b)), 10), nil

	case 1: // 16-bit integer
		var length uint16
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return "", err
		}
		return strconv.FormatInt(int64(length), 10), nil

	case 2: // 32-bit integer
		var length uint32
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return "", err
		}
		return strconv.FormatInt(int64(length), 10), nil

	case 3: // LZF compressed string - too complex just gonna skip it for now
		fmt.Println("LZF compressed string - too complex just gonna skip it for now")
		return "", nil
	default:
		return "", fmt.Errorf("Invalid RDB encoding type: %d", encodeType)
	}
}
