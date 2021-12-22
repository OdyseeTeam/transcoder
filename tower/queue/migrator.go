package queue

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
)

const dialect = "postgres"

type Migrator struct {
	db     *sql.DB
	source *migrate.EmbedFileSystemMigrationSource
}

func NewMigrator(db *sql.DB, fs embed.FS) Migrator {
	return Migrator{
		db,
		&migrate.EmbedFileSystemMigrationSource{
			FileSystem: fs,
			Root:       "migrations",
		},
	}
}

// MigrateUp executes forward migrations.
func (m Migrator) MigrateUp() (int, error) {
	return migrate.Exec(m.db, dialect, m.source, migrate.Up)
}

// MigrateDown undoes a specified number of migrations.
func (m Migrator) MigrateDown(max int) (int, error) {
	return migrate.ExecMax(m.db, dialect, m.source, migrate.Down, max)
}

// Truncate purges records from the requested tables.
func (m Migrator) Truncate(tables []string) error {
	_, err := m.db.Exec(fmt.Sprintf("TRUNCATE %s CASCADE;", strings.Join(tables, ", ")))
	return err
}

// CreateDB creates the requested database.
func (m Migrator) CreateDB(dbName string) error {
	// fmt.Sprintf is used instead of query placeholders because postgres does not
	// handle them in schema-modifying queries.
	_, err := m.db.Exec(fmt.Sprintf("create database %s;", pq.QuoteIdentifier(dbName)))
	// c.logger.WithFields(logrus.Fields{"db_name": dbName}).Info("created the database")
	return err
}

// DropDB drops the requested database.
func (m Migrator) DropDB(dbName string) error {
	_, err := m.db.Exec(fmt.Sprintf("drop database %s;", pq.QuoteIdentifier(dbName)))
	return err
}
