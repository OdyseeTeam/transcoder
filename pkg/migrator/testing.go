package migrator

import (
	"database/sql"
	"embed"

	"github.com/Pallinder/go-randomdata"
)

type TestDBCleanup func() error

func CreateTestDB(mfs embed.FS) (*sql.DB, TestDBCleanup, error) {
	db, err := ConnectDB(DefaultDBConfig().NoMigration(), mfs)
	tdbn := "test-db-" + randomdata.Alphanumeric(12)
	if err != nil {
		return nil, nil, err
	}
	m := NewMigrator(db, mfs)
	m.CreateDB(tdbn)

	tdb, err := ConnectDB(DefaultDBConfig().Name(tdbn), mfs)
	if err != nil {
		return nil, nil, err
	}
	tm := NewMigrator(tdb, mfs)
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
