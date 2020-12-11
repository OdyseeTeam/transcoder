package db

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3" // sqlite
	"go.uber.org/zap"
)

var logger = zap.NewExample().Sugar().Named("db")

const defaultDBFile = "db.sqlite"

type DB struct {
	*sql.DB
	file    string
	cleanup func() error
}

// OpenDB opens sqlite database file.
func OpenDB(file string) *DB {
	if file == "" {
		file = defaultDBFile
	}
	logger.Infow("opening sqlite database", "file", file)

	stdDB, err := sql.Open("sqlite3", file)
	db := &DB{stdDB, file, func() error { return nil }}
	if err != nil {
		logger.Panic(err)
	}
	return db
}

// OpenTestDB generates a random database file name and opens it, returning cleanup function for use in tests.
func OpenTestDB() *DB {
	file := fmt.Sprintf("%v.sqlite", RandomString(16))
	db := OpenDB(file)
	db.cleanup = func() error {
		return os.Remove(file)
	}
	return db
}

func (db *DB) MigrateUp(file string) error {
	s, err := ioutil.ReadFile(file)
	schemaBits := strings.Split(string(s), "-- +migrate Down")
	stmt, err := db.Prepare(schemaBits[0])
	if err != nil {
		return err
	}
	_, err = stmt.Exec()
	return err
}

func (db *DB) MigrateDown(file string) error {
	s, err := ioutil.ReadFile(file)
	schemaBits := strings.Split(string(s), "-- +migrate Down")
	stmt, err := db.Prepare(schemaBits[1])
	if err != nil {
		return err
	}
	_, err = stmt.Exec()
	return err
}

func (db *DB) Cleanup() error {
	return db.cleanup()
}

// RandomString generates a random string of length `n`.
func RandomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
