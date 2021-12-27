package main

import (
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/mfr"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/tower"
	"github.com/lbryio/transcoder/tower/queue"
	"github.com/lbryio/transcoder/video"

	"github.com/alecthomas/kong"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const configName = "tower"

var CLI struct {
	queue.MigratorCLI
	Serve struct {
		RMQAddr   string `optional:"" help:"RabbitMQ server address" default:"amqp://guest:guest@localhost/"`
		HttpBind  string `optional:"" help:"Address for HTTP server to listen on" default:"0.0.0.0:8080"`
		HttpURL   string `help:"URL at which callback server will be accessible from the outside"`
		StateFile string `optional:"" help:"State file to synchronize to and load on start up"`
		DevMode   bool   `help:"Development mode (purges queues before start)"`
	} `cmd:"" help:"Start tower server"`
	Debug bool `optional:"" help:"Enable debug logging" default:false`
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
		logger = logging.Create("tower", logging.Dev).Desugar()
	} else {
		logger = logging.Create("tower", logging.Prod).Desugar()
	}

	if !CLI.Debug {
		db.SetLogger(logging.Create("db", logging.Prod))
		encoder.SetLogger(logging.Create("encoder", logging.Prod))
		video.SetLogger(logging.Create("video", logging.Prod))
		manager.SetLogger(logging.Create("claim", logging.Prod))
		storage.SetLogger(logging.Create("storage", logging.Prod))
		formats.SetLogger(logging.Create("formats", logging.Prod))
		mfr.SetLogger(logging.Create("mfr", logging.Prod))
		dispatcher.SetLogger(logging.Create("dispatcher", logging.Prod))
	}

	log := logger.Sugar()

	cfg, err := readConfig()
	if err != nil {
		log.Fatal("unable to read config", err)
	}

	s3cfg := cfg.GetStringMapString("s3")
	towerCfg := cfg.GetStringMapString("tower")
	libCfg := cfg.GetStringMapString("library")

	err = os.MkdirAll(libCfg["sqlite"], os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(libCfg["videos"], os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(towerCfg["workdir"], os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	vdb := db.OpenDB(path.Join(libCfg["sqlite"], "videos.sqlite"))
	err = vdb.MigrateUp(video.InitialMigration)
	if err != nil {
		log.Fatal(err)
	}
	libConfig := video.Configure().
		LocalStorage(storage.Local(libCfg["videos"])).
		MaxLocalSize(libCfg["maxsize"]).
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
		libConfig.RemoteStorage(s3d)
		log.Infow("s3 storage configured", "bucket", s3cfg["bucket"])
	}
	lib := video.NewLibrary(libConfig)

	var s3StopChan chan<- interface{}
	// if s3cfg["bucket"] != "" {
	// 	s3StopChan = video.SpawnS3uploader(lib)
	// }

	manager.LoadConfiguredChannels(
		cfg.GetStringSlice("prioritychannels"),
		cfg.GetStringSlice("enabledchannels"),
		cfg.GetStringSlice("disabledchannels"),
	)

	cleanStopChan := video.SpawnLibraryCleaning(lib)

	adQueue := cfg.GetStringMapString("adaptivequeue")
	minHits, _ := strconv.Atoi(adQueue["minhits"])
	mgr := manager.NewManager(lib, minHits)

	qCfg := cfg.GetStringMapString("queue")
	qDB, err := queue.ConnectDB(queue.DefaultDBConfig().DSN(qCfg["dsn"]))
	if err != nil {
		log.Fatal("queue db initialization failed", err)
	}

	serverConfig := tower.DefaultServerConfig().
		Logger(zapadapter.NewKV(logger)).
		HttpServer(CLI.Serve.HttpBind, CLI.Serve.HttpURL).
		VideoManager(mgr).
		WorkDir(towerCfg["workdir"]).
		RMQAddr(CLI.Serve.RMQAddr).
		DB(qDB)

	if CLI.Serve.DevMode {
		serverConfig = serverConfig.DevMode()
	}

	server, err := tower.NewServer(serverConfig)
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
