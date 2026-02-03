package config

import "fmt"

type Config struct {
	port string

	// RDB
	rdbEnabled bool
	dir        string
	dbFilename string

	// AOF
	aofEnabled     bool
	appendFilename string
}

func DefaultConfig() *Config {
	return &Config{
		port:           "6379",
		rdbEnabled:     true,
		dir:            ".",
		dbFilename:     "dump.rdb",
		aofEnabled:     true,
		appendFilename: "appendonly.aof",
	}
}

// todo: load config from file & save

func (c *Config) Get(key string) string {
	switch key {
	case "port":
		return c.port
	case "rdbEnabled":
		return fmt.Sprintf("%t", c.rdbEnabled)
	case "dir":
		return c.dir
	case "dbfilename":
		return c.dbFilename
	case "aofEnabled":
		return fmt.Sprintf("%t", c.aofEnabled)
	case "appendfilename":
		return c.appendFilename
	default:
		return ""
	}
}

func (c *Config) Set(key, value string) error {
	switch key {
	case "port":
		c.port = value
	case "rdbEnabled":
		if value != "yes" && value != "no" {
			return fmt.Errorf("argument must be 'yes' or 'no'")
		}
		c.rdbEnabled = value == "yes"
	case "dir":
		c.dir = value
	case "dbfilename":
		c.dbFilename = value
	case "aofEnabled":
		if value != "yes" && value != "no" {
			return fmt.Errorf("argument must be 'yes' or 'no'")
		}
		c.aofEnabled = value == "yes"
	case "appendfilename":
		c.appendFilename = value
	default:
		return fmt.Errorf("parameter '%s' is not a valid configuration key", key)
	}
	return nil
}
