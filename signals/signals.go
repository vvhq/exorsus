package signals

import (
	"exorsus/process"
	"exorsus/rest"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"sync"
	"time"
)

func HandleSignals(wg *sync.WaitGroup, signalChan chan os.Signal, procManager *process.Manager, restService *rest.Service, timeout int, logger *logrus.Logger) {
	receivedSignal := <-signalChan
	logger.
		WithField("source", "signals").
		WithField("signal", receivedSignal.String()).
		Info("Signal received")
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
	wg.Done()
}