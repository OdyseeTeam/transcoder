package main

import (
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower"
	"github.com/lbryio/transcoder/video"

	"github.com/alecthomas/kong"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const configName = "tower"

var CLI struct {
	Serve struct {
		RMQAddr  string `optional help:"RabbitMQ server address" default:"amqp://guest:guest@localhost/"`
		HttpBind string `optional help:"Address for HTTP server to listen on" default:"0.0.0.0:8080"`
		HttpURL  string `help:"URL at which callback server will be accessible from the outside"`
	} `cmd help:"Start tower server"`
	Debug bool `optional help:"Enable debug logging" default:false`
}

func main() {
	ctx := kong.Parse(&CLI)

	switch ctx.Command() {
	case "serve":
		serve()
	default:
		panic(ctx.Command())
	}
}

func serve() {
	var logger *zap.Logger
	if CLI.Debug {
		logger, _ = zap.NewProductionConfig().Build()
	} else {
		logger, _ = zap.NewDevelopmentConfig().Build()
	}
	log := logger.Sugar()

	cfg, err := readConfig()
	if err != nil {
		log.Fatal("unable to read config", err)
	}

	s3cfg := cfg.GetStringMapString("s3")
	local := cfg.GetStringMapString("local")
	towerCfg := cfg.GetStringMapString("tower")

	vdb := db.OpenDB(path.Join(cfg.GetString("datapath"), "videos.sqlite"))
	err = vdb.MigrateUp(video.InitialMigration)
	if err != nil {
		log.Fatal(err)
	}
	libCfg := video.Configure().
		LocalStorage(storage.Local(local["path"])).
		MaxLocalSize(local["maxsize"]).
		MaxRemoteSize(s3cfg["maxsize"]).
		DB(vdb)

	if s3cfg["bucket"] != "" {
		s3d, err := storage.InitS3Driver(
			storage.S3Configure().
				Endpoint(s3cfg["endpoint"]).
				Credentials(s3cfg["key"], s3cfg["secret"]).
				Bucket(s3cfg["bucket"]))
		if err != nil {
			log.Fatal("s3 driver initialization failed", err)
		}
		libCfg.RemoteStorage(s3d)
		log.Infow("s3 storage configured", "bucket", s3cfg["bucket"])
	}
	lib := video.NewLibrary(libCfg)

	var s3StopChan chan<- interface{}
	if s3cfg["bucket"] != "" {
		s3StopChan = video.SpawnS3uploader(lib)
	}

	manager.LoadConfiguredChannels(
		cfg.GetStringSlice("prioritychannels"),
		cfg.GetStringSlice("enabledchannels"),
		cfg.GetStringSlice("disabledchannels"),
	)

	cleanStopChan := video.SpawnLibraryCleaning(lib)

	adQueue := cfg.GetStringMapString("adaptivequeue")
	minHits, _ := strconv.Atoi(adQueue["minhits"])
	mgr := manager.NewManager(lib, minHits)

	server, err := tower.NewServer(tower.DefaultServerConfig().
		Logger(zapadapter.NewKV(logger)).
		HttpServer(CLI.Serve.HttpBind, CLI.Serve.HttpURL).
		VideoManager(mgr).
		WorkDir(towerCfg["workdir"]).
		RestoreState(path.Join(towerCfg["workdir"], "tower-state.json")).
		RMQAddr(CLI.Serve.RMQAddr),
	)
	if err != nil {
		log.Fatal("unable to initialize tower server", err)
	}

	err = server.StartAll()
	if err != nil {
		log.Fatal("unable to start tower server", err)
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-stopChan
	log.Infof("caught an %v signal, shutting down...", sig)

	close(cleanStopChan)
	log.Infof("cleanup shut down")

	mgr.Pool().Stop()
	log.Infof("manager shut down")

	if s3StopChan != nil {
		close(s3StopChan)
		log.Infof("S3 uploader shut down")
	}

	server.StopAll()
	log.Infof("tower server stopped")
}

func readConfig() (*viper.Viper, error) {
	cfg := viper.New()
	cfg.SetConfigName(configName)

	exec, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cfg.AddConfigPath(filepath.Dir(exec))
	cfg.AddConfigPath(".")

	return cfg, cfg.ReadInConfig()
}
