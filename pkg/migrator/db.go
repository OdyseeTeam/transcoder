package migrator

import (
	"database/sql"
	"embed"
	"fmt"
)

type DBConfig struct {
	dsn, dbName, connOpts string
	migrate               bool
}

func DefaultDBConfig() *DBConfig {
	return &DBConfig{
		dsn:      "postgres://postgres:odyseeteam@localhost",
		dbName:   "postgres",
		connOpts: "sslmode=disable",
		migrate:  true,
	}
}

func (c *DBConfig) DSN(dsn string) *DBConfig {
	c.dsn = dsn
	return c
}

func (c *DBConfig) Name(dbName string) *DBConfig {
	c.dbName = dbName
	return c
}

func (c *DBConfig) ConnOpts(connOpts string) *DBConfig {
	c.connOpts = connOpts
	return c
}

func (c *DBConfig) NoMigration() *DBConfig {
	c.migrate = false
	return c
}

func (c *DBConfig) GetFullDSN() string {
	return fmt.Sprintf("%s/%s?%s", c.dsn, c.dbName, c.connOpts)
}

func ConnectDB(config *DBConfig, migrationsFS embed.FS) (*sql.DB, error) {
	var err error
	db, err := sql.Open("postgres", config.GetFullDSN())
	if err != nil {
		return nil, err
	}
	if config.migrate {
		_, err = NewMigrator(db, migrationsFS).MigrateUp()
		if err != nil {
			return nil, err
		}
	}

	return db, nil
}
