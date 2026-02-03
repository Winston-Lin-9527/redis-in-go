package persistence

import (
	"github.com/Winston-Lin-9527/redis-in-go/app/config"
)

type RDB struct {
	dir        string
	dbfilename string
}

func NewRDB(config *config.Config) *RDB {
	dir := config.Get("dir")
	dbfilename := config.Get("dbfilename")
	return &RDB{dir: dir, dbfilename: dbfilename}
}

func (r *RDB) Save() {

}

func (r *RDB) GetDir() string {
	return r.dir
}

func (r *RDB) GetDBFilename() string {
	return r.dbfilename
}
