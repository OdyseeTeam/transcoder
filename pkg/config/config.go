package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const configName = "transcoder"

func Read() (*viper.Viper, error) {
	cfg := viper.New()
	cfg.SetConfigName(configName)
	cfg.AddConfigPath(ProjectRoot())
	cfg.AddConfigPath(".")
	return cfg, cfg.ReadInConfig()
}

func ProjectRoot() string {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return filepath.Dir(ex)
}
