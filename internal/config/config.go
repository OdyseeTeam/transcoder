package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Storages []S3Config

type ConductorConfig struct {
	Storages      Storages
	Redis         string
	AdaptiveQueue AdaptiveQueue
	Library       Library
}

type WorkerConfig struct {
	Storage   S3Config
	Redis     string
	EdgeToken string
}

type S3Config struct {
	Type         string
	Name         string
	Endpoint     string
	Region       string
	Bucket       string
	Key          string
	Secret       string
	MaxSize      string
	CreateBucket bool
}

type AdaptiveQueue struct {
	MinHits int
}

type Library struct {
	DSN          string
	ManagerToken string
}

func ProjectRoot() (string, error) {
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(ex), nil
}

func Read(name string, cfg any) error {
	v := viper.New()
	v.SetConfigName(name)

	pp, err := ProjectRoot()
	if err != nil {
		return err
	}
	v.AddConfigPath(pp)
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("fatal error reading config file: %w", err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unable to decode into struct: %w", err)
	}

	return nil
}

func ReadConductorConfig() (*ConductorConfig, error) {
	cfg := &ConductorConfig{}
	return cfg, Read("conductor", cfg)
}

func ReadWorkerConfig() (*WorkerConfig, error) {
	cfg := &WorkerConfig{}
	return cfg, Read("worker", cfg)
}

func (s Storages) Endpoints() []string {
	endpoints := []string{}
	for _, v := range s {
		endpoints = append(endpoints, v.Endpoint)
	}
	return endpoints
}
