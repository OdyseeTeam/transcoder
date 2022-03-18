package queue

import (
	"database/sql"

	"github.com/Pallinder/go-randomdata"
)

type TestDBCleanup func() error

func CreateTestDB() (*sql.DB, TestDBCleanup, error) {
	db, err := ConnectDB(DefaultDBConfig().NoMigration())
	tdbn := "test-db-" + randomdata.Alphanumeric(12)
	if err != nil {
		return nil, nil, err
	}
	m := NewMigrator(db, MigrationsFS)
	m.CreateDB(tdbn)

	tdb, err := ConnectDB(DefaultDBConfig().Name(tdbn))
	if err != nil {
		return nil, nil, err
	}
	tm := NewMigrator(tdb, MigrationsFS)
	_, err = tm.MigrateUp()
	if err != nil {
		return nil, nil, err
	}
	return tdb, func() error {
		tdb.Close()
		err := m.DropDB(tdbn)
		db.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}
