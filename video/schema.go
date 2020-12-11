package video

import (
	"database/sql"
	"io/ioutil"
	"os"
	"strings"
)

const dbFile = "video.db"

func OpenDB() *sql.DB {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		file, err := os.Create(dbFile)
		if err != nil {
			logger.Fatal(err)
		}
		file.Close()
	}

	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		logger.Fatal(err)
	}

	s, err := ioutil.ReadFile("schema.sql")
	schemaBits := strings.Split(string(s), "-- +migrate Down")
	stmt, err := db.Prepare(schemaBits[0])
	if err != nil {
		logger.Fatal(err)
	}
	_, err = stmt.Exec()
	if err != nil {
		logger.Fatal(err)
	}

	return db
}

func dbCleanup() {
	os.Remove(dbFile)
}
