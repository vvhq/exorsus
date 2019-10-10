package signals

import (
	"exorsus/configuration"
	"exorsus/logging"
	"exorsus/process"
	"exorsus/rest"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"sync"
	"syscall"
	"time"
)

func HandleSignals(wg *sync.WaitGroup,
	signalChan chan os.Signal,
	procManager *process.Manager,
	restService *rest.Service,
	timeout int,
	config *configuration.Configuration,
	logger *logrus.Logger,
	loggerHook *logging.FileHook) {
		for {
			receivedSignal := <-signalChan
			logger.
				WithField("source", "signals").
				WithField("signal", receivedSignal.String()).
				Info("Signal received")
			if receivedSignal == syscall.SIGUSR1 {
				handleUSR1(logger, loggerHook)
			} else if receivedSignal == syscall.SIGHUP {
				handleHUP(logger)
			} else if receivedSignal == syscall.SIGINT || receivedSignal == syscall.SIGTERM {
				handleSTOP(procManager, restService, timeout, logger)
				wg.Done()
				return
			} else {
				logger.
					WithField("source", "signals").
					WithField("signal", "UNHANDLED").
					Info("Signal received")
			}
		}
}

func handleUSR1(logger *logrus.Logger, loggerHook *logging.FileHook) {
	loggerHook.Rotate()
	logger.
		WithField("source", "signals").
		Info("Log rotated")
}

func handleHUP(logger *logrus.Logger) {
	logger.
		WithField("source", "signals").
		WithField("signal", "HUP").
		Info("Signal received")
}

func handleSTOP(procManager *process.Manager, restService *rest.Service, timeout int, logger *logrus.Logger) {
	procManager.StopAll()
	restService.Stop()
	logger.
		WithField("source", "signal").
		Infof("Waiting for the processes to complete %d seconds", timeout)
	time.Sleep(time.Second * time.Duration(timeout))
	for _, proc := range procManager.List() {
		if proc.Zombie() {
			logger.
				WithField("source", "main").
				WithField("process", proc.Name).
				WithField("pid", fmt.Sprintf("%d", proc.GetPid())).
				Error("Found zombie process, force exit")
			os.Exit(2)
		}
	}
}