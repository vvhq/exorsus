package main

import (
	"exorsus/application"
	"exorsus/configuration"
	"exorsus/logging"
	"exorsus/process"
	"exorsus/rest"
	"exorsus/signals"
	"exorsus/status"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
)

var version string //set during build

func main() {
	configDir := flag.String("config", "./config/", "application directory path")
	printVersion := flag.Bool("version", false, "print version number")
	flag.Parse()
	if *printVersion {
		fmt.Println(version)
		os.Exit(0)
	}
	configDirPath := path.Dir(*configDir)
	configPath := path.Join(configDirPath, configuration.DefaultConfigurationFileName)
	config := configuration.New(configPath)

	var logger = logging.NewLogger(os.Stdout, config.GetLogLevel())
	loggerHook, err := logging.NewFileHook(logger, path.Join(path.Dir(config.GetLogPath()), configuration.DefaultLogFileName))
	if err != nil {
		logger.
			WithField("source", "main").
			WithField("error", err.Error()).
			Error("Can not set file hook")
	} else {
		logger.AddHook(loggerHook)
	}
	logger.WithField("Source", "Main").Trace("Exorsus starting")
	maxTimeout := 0
	var wg sync.WaitGroup
	storage := application.NewStorage(path.Join(configDirPath, configuration.DefaultApplicationsFileName), logger)
	procManager := process.NewManager(&wg, logger)
	for _, app := range storage.List() {
		appClone, err := app.Copy()
		if err != nil {
			logger.
				WithField("source", "main").
				WithField("error", err.Error()).
				Error("Skip application due error")
		} else {
			proc := process.New(appClone, status.New(config.GetMaxStdLines()), &wg, config, logger)
			procManager.Append(proc)
			if appClone.Timeout > maxTimeout {
				maxTimeout = appClone.Timeout
			}
		}
	}
	restService := rest.New(config.GetListenPort(), storage, procManager, &wg, config, logger)
	procManager.StartAll()
	restService.Start()
	maxTimeout = maxTimeout + config.GetShutdownTimeout()
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	wg.Add(1)
	go signals.HandleSignals(&wg, signalChan, procManager, restService, maxTimeout, logger)
	logger.
		WithField("source", "main").
		Info("Exorsus stared")
	wg.Wait()
	logger.
		WithField("source", "main").
		Info("Exorsus stopped")
}


