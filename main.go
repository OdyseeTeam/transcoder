package main

import (
	"math/rand"
	"os"
	"os/signal"
	"path"
	"runtime/pprof"
	"strconv"
	"syscall"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/config"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/mfr"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/video"
	"github.com/lbryio/transcoder/workers"
	"github.com/pkg/profile"

	"github.com/alecthomas/kong"
)

var logger = logging.Create("main", logging.Dev)

var CLI struct {
	Serve struct {
		Bind         string `optional name:"bind" help:"Address to listen on." default:":8080"`
		DataPath     string `optional name:"data-path" help:"Path to store database files and configs." type:"existingdir" default:"."`
		VideoPath    string `optional name:"video-path" help:"Path to store video." type:"existingdir" default:"."`
		Workers      int    `optional name:"workers" help:"Number of workers to start." type:"int" default:"10"`
		CDN          string `optional name:"cdn" help:"LBRY CDN endpoint address."`
		Debug        bool   `optional name:"debug" help:"Debug mode."`
		ProfileCPU   bool   `optional name:"profile-cpu" help:"Enable CPU profiling."`
		ProfileTrace bool   `optional name:"profile-trace" help:"Enable execution tracer."`
	} `cmd help:"Start transcoding server."`
}

const cpuPF = "cpu.pprof"

func main() {
	rand.Seed(time.Now().UnixNano())

	cfg, err := config.Read()
	cfg.SetDefault("CDNServer", "https://cdn.lbryplayer.xyz/api/v3/streams")
	if err != nil {
		logger.Fatal(err)
	}

	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "serve":
		if CLI.Serve.ProfileCPU {
			logger.Infof("outputting CPU profile to %v", cpuPF)
			f, err := os.Create(cpuPF)
			if err != nil {
				logger.Fatal("could not create CPU profile: ", err)
			}
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				logger.Fatal("could not start CPU profiling: ", err)
			}
			defer pprof.StopCPUProfile()
		}
		if CLI.Serve.ProfileTrace {
			logger.Info("tracing enabled")
			defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
		}

		if !CLI.Serve.Debug {
			db.SetLogger(logging.Create("db", logging.Prod))
			encoder.SetLogger(logging.Create("encoder", logging.Prod))
			video.SetLogger(logging.Create("video", logging.Prod))
			manager.SetLogger(logging.Create("claim", logging.Prod))
			storage.SetLogger(logging.Create("storage", logging.Prod))
			formats.SetLogger(logging.Create("formats", logging.Prod))
			mfr.SetLogger(logging.Create("mfr", logging.Prod))
			dispatcher.SetLogger(logging.Create("dispatcher", logging.Prod))
		}

		if CLI.Serve.CDN != "" {
			manager.SetCDNServer(CLI.Serve.CDN)
		} else {
			manager.SetCDNServer(cfg.GetString("CDNServer"))
		}

		vdb := db.OpenDB(path.Join(CLI.Serve.DataPath, "video.sqlite"))
		err := vdb.MigrateUp(video.InitialMigration)
		if err != nil {
			logger.Fatal(err)
		}

		s3cfg := cfg.GetStringMapString("s3")
		local := cfg.GetStringMapString("local")

		libCfg := video.Configure().
			LocalStorage(storage.Local(CLI.Serve.VideoPath)).
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
				logger.Fatalw("s3 driver initialization failed", "err", err)
			}
			libCfg.RemoteStorage(s3d)
			logger.Infow("s3 storage configured", "bucket", s3cfg["bucket"])
		}
		lib := video.NewLibrary(libCfg)

		var s3StopChan chan<- interface{}
		if s3cfg["bucket"] != "" {
			s3StopChan = video.SpawnS3uploader(lib)
		}

		manager.LoadEnabledChannels(cfg.GetStringSlice("enabledchannels"))

		cleanStopChan := video.SpawnLibraryCleaning(lib)

		adQueue := cfg.GetStringMapString("adaptivequeue")
		minHits, _ := strconv.Atoi(adQueue["minhits"])
		mgr := manager.NewManager(lib, minHits)

		encStopChan := workers.SpawnEncoderWorkers(CLI.Serve.Workers, mgr)

		httpAPI := manager.NewHttpAPI(
			manager.ConfigureHttpAPI().
				Debug(CLI.Serve.Debug).
				Addr(CLI.Serve.Bind).
				VideoPath(CLI.Serve.VideoPath).
				VideoManager(mgr),
		)
		logger.Infow("configured api server", "addr", CLI.Serve.Bind)

		go func() {
			err = httpAPI.Start()
			if err != nil {
				logger.Fatal(err)
			}
		}()

		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

		sig := <-stopChan
		logger.Infof("caught an %v signal, shutting down...", sig)

		encStopChan <- true
		logger.Infof("encoder shut down")

		cleanStopChan <- true
		logger.Infof("cleanup shut down")

		mgr.Pool().Stop()
		logger.Infof("manager shut down")

		if s3StopChan != nil {
			s3StopChan <- true
			logger.Infof("S3 uploader shut down")
		}

		httpAPI.Shutdown()
		logger.Infof("http API shut down")
	default:
		logger.Fatal(ctx.Command())
	}
}
