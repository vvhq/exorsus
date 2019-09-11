package process

import (
	"exorsus/application"
	"exorsus/configuration"
	"exorsus/logging"
	"exorsus/status"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Process struct {
	Name          string
	app           *application.Application
	status        *status.Status
	command       *exec.Cmd
	mainWaitGroup *sync.WaitGroup
	config        *configuration.Configuration
	stdLogger     *logrus.Logger
	logger        *logrus.Logger
}

func (process *Process) Start() {
	process.mainWaitGroup.Add(1)
	go process.start()
}

func (process *Process) Stop() {
	process.mainWaitGroup.Add(1)
	go process.stop()
}

func (process *Process) Restart() {
	process.mainWaitGroup.Add(2)
	go func() {
		process.stop()
		process.start()
	}()
}

func (process *Process) GetPid() int {
	return process.status.GetPid()
}

func (process *Process) GetExitCode() int {
	return process.status.GetExitCode()
}

func (process *Process) GetError() error {
	return process.status.GetError()
}

func (process *Process) GetState() int {
	return process.status.GetState()
}

func (process *Process) GetStdOut() []string {
	return process.status.ListStdOutItems()
}

func (process *Process) GetStdErr() [] string {
	return process.status.ListStdErrItems()
}

func (process *Process) Zombie() bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", process.getCurrentPid()))
	return !os.IsNotExist(err) && process.status.GetState() == status.Failed
}

func (process *Process) getCurrentPid() int {
	currentPid := 0
	if process.command != nil && process.command.Process != nil {
		currentPid = process.command.Process.Pid
	}
	return currentPid
}

func (process *Process) pipe2Channel(pipe io.Reader, channel chan<- string) {
	process.mainWaitGroup.Add(1)
	go func() {
		defer process.mainWaitGroup.Done()
		buffer := make([]byte, 4096, 4096)
		for {
			numberBytesRead, err := pipe.Read(buffer)
			if numberBytesRead > 0 {
				bytesRead := buffer[:numberBytesRead]
				channel <- strings.TrimRight(string(bytesRead), "\n")
			}
			if err != nil {
				if err != io.EOF {
					process.logger.
						WithField("source", "process").
						WithField("process", process.Name).
						WithField("state", process.GetState()).
						WithField("error", err.Error()).
						Error("Can not create pipe")
				}
				break
			}
		}
	}()
}

func (process *Process) stdOutChannelHandler(channel <-chan string) {
	process.mainWaitGroup.Add(1)
	go func() {
		defer process.mainWaitGroup.Done()
		for {
			item, out := <-channel
			if out {
				process.status.AddStdOutItem(item)
				process.stdLogger.
					WithField("source", "process").
					WithField("process", process.Name).
					WithField("state", process.GetState()).
					WithField("item", item).
					Info("Can not item from stdout channel")
			} else {
				break
			}
		}
	}()
}

func (process *Process) stdErrChannelHandler(channel <-chan string) {
	process.mainWaitGroup.Add(1)
	go func() {
		defer process.mainWaitGroup.Done()
		for {
			item, out := <-channel
			if out {
				process.status.AddStdErrItem(item)
				process.stdLogger.WithField("SOURCE", "Process").WithField("NAME", process.Name).Error(item)
			} else {
				break
			}
		}
	}()
}

func (process *Process) start() {
	defer process.mainWaitGroup.Done()
	if process.status.GetState() == status.Started {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("operation", "start").
			Warn("Process already started")
		return
	}
	if process.status.GetState() != status.Stopped {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("operation", "start").
			Warn("process busy")
		return
	}

	process.status.SetState(status.Starting)
	process.status.SetPid(process.getCurrentPid())
	process.status.SetExitCode(0)
	process.status.SetError(nil)

	arguments := strings.Fields(strings.TrimSpace(process.app.Arguments))

	process.command = exec.Command(process.app.Command, arguments...)

	if process.findCredential() != nil {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("user", process.app.User).
			WithField("group", process.app.Group).
			Trace("Start process as specific user/group")
		process.command.SysProcAttr = &syscall.SysProcAttr{}
		process.command.SysProcAttr.Credential = process.findCredential()
	}

	process.command.Env = os.Environ()
	for _, env := range process.app.Environment {
		process.command.Env = append(process.command.Env, fmt.Sprintf("%s=%s", env.Name, env.Value))
	}

	stdOutChan := make(chan string, 4096)
	stdErrChan := make(chan string, 4096)
	stdOutPipe, err := process.command.StdoutPipe()
	if err != nil {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("error", err.Error()).
			Error("Can not create stdout pipe")
	} else {
		process.pipe2Channel(stdOutPipe, stdOutChan)
		process.stdOutChannelHandler(stdOutChan)
	}
	stdErrPipe, err := process.command.StderrPipe()
	if err != nil {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("error", err.Error()).
			Error("Can not create stderr pipe")
	} else {
		process.pipe2Channel(stdErrPipe, stdErrChan)
		process.stdErrChannelHandler(stdErrChan)
	}

	err = process.command.Start()
	if err != nil {
		process.status.SetState(status.Stopped)
		process.status.SetPid(process.getCurrentPid())
		process.status.SetError(err)
		process.status.SetExitCode(-1)
		close(stdOutChan)
		close(stdErrChan)
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("error", err.Error()).
			Error("Can not start process")
		return
	}
	process.status.SetState(status.Started)
	process.status.SetPid(process.getCurrentPid())

	err = process.command.Wait()
	if  err != nil {
		process.status.SetExitCode(-1)
		exitError, ok := err.(*exec.ExitError)
		if ok {
			exitStatus, ok := exitError.Sys().(syscall.WaitStatus)
			if ok {
				process.status.SetExitCode(exitStatus.ExitStatus())
			}
		}
		process.status.SetError(err)
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("error", err.Error()).
			Error("Error waiting for process")
	} else {
		process.status.SetError(nil)
		process.status.SetExitCode(0)
	}
	close(stdOutChan)
	close(stdErrChan)
	process.status.SetState(status.Stopped)
}

func (process *Process) stop() {
	defer process.mainWaitGroup.Done()
	if process.status.GetState() == status.Stopped {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("operation", "stop").
			Trace("Process already stopped")
		return
	}
	if process.status.GetState() != status.Started {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("operation", "stop").
			Warn("Process busy")
		return
	}
	process.status.SetState(status.Stopping)
	process.status.SetError(nil)
	process.status.SetExitCode(-1)
	err := process.command.Process.Signal(syscall.SIGINT)
	if err != nil {
		process.status.SetExitCode(0)
		exitError, okErrCast := err.(*exec.ExitError)
		if okErrCast {
			exitStatus, okStatusCast := exitError.Sys().(syscall.WaitStatus)
			if okStatusCast {
				process.status.SetExitCode(exitStatus.ExitStatus())
			}
		}
		process.status.SetPid(process.getCurrentPid())
		process.status.SetError(err)
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("pid", fmt.Sprintf("%d", process.status.GetPid())).
			WithField("code", fmt.Sprintf("%d", process.status.GetExitCode())).
			WithField("error", err.Error()).
			Error("Can not gracefully stop process")
	}  else {
		process.status.SetPid(process.getCurrentPid())
		process.status.SetState(status.Stopped)
		process.status.SetError(nil)
		process.status.SetExitCode(0)
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("pid", fmt.Sprintf("%d", process.status.GetPid())).
			WithField("code", fmt.Sprintf("%d", process.status.GetExitCode())).
			Trace("Process stopped successfully")
	}
	time.Sleep(time.Second * time.Duration(process.app.Timeout +10))
	if process.status.GetState() != status.Stopped {
		err := process.command.Process.Kill()
		if err != nil {
			process.status.SetExitCode(0)
			exitError, okErrCast := err.(*exec.ExitError)
			if okErrCast {
				exitStatus, okStatusCast := exitError.Sys().(syscall.WaitStatus)
				if okStatusCast {
					process.status.SetExitCode(exitStatus.ExitStatus())
				}
			}
			process.status.SetPid(process.getCurrentPid())
			process.status.SetError(err)
			process.logger.
				WithField("source", "process").
				WithField("process", process.Name).
				WithField("state", process.GetState()).
				WithField("pid", fmt.Sprintf("%d", process.status.GetPid())).
				WithField("code", fmt.Sprintf("%d", process.status.GetExitCode())).
				WithField("error", err.Error()).
				Error("Can not KILL process")
			process.status.SetError(err)
		} else {
			process.status.SetPid(process.getCurrentPid())
			process.status.SetState(status.Stopped)
			process.status.SetError(nil)
			process.status.SetExitCode(0)
			process.logger.
				WithField("source", "process").
				WithField("process", process.Name).
				WithField("state", process.GetState()).
				WithField("pid", fmt.Sprintf("%d", process.status.GetPid())).
				WithField("code", fmt.Sprintf("%d", process.status.GetExitCode())).
				Trace("process stopped")
		}
	}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", process.getCurrentPid())); !os.IsNotExist(err) {
		process.status.SetState(status.Failed)
		process.status.SetExitCode(-1)
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("pid", fmt.Sprintf("%d", process.status.GetPid())).
			WithField("code", fmt.Sprintf("%d", process.status.GetExitCode())).
			WithField("error", err.Error()).
			Error("Process running detached")
	} else {
		process.logger.
			WithField("source", "process").
			WithField("process", process.Name).
			WithField("state", process.GetState()).
			WithField("pid", fmt.Sprintf("%d", process.status.GetPid())).
			WithField("code", fmt.Sprintf("%d", process.status.GetExitCode())).
			Trace("Process does not exists")
	}
}

func (process *Process) findCredential() *syscall.Credential {
	uid := -1
	gid := -1
	if process.app.User != "" {
		lookupUser, err := user.Lookup(process.app.User)
		if err == nil {
			lookupUid, err := strconv.Atoi(lookupUser.Uid)
			if err == nil {
				uid = lookupUid
			}
		}
	}
	if  process.app.Group != "" {
		lookupGroup, err := user.LookupGroup(process.app.Group)
		if err == nil {
			lookupGid, err := strconv.Atoi(lookupGroup.Gid)
			if err == nil {
				gid = lookupGid
			}
		}
	}
	if uid > 0 && gid > 0 {
		return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	} else {
		return nil
	}

}

func New(app *application.Application, status *status.Status, wg *sync.WaitGroup, config *configuration.Configuration, logger *logrus.Logger) *Process {
	logPath := path.Join(path.Dir(config.LogPath), fmt.Sprintf("app_%s.json", app.Name))
	hostName, err := os.Hostname()
	if err == nil {
		logDirName, logFileName := filepath.Split(logPath)
		logPath = path.Join(logDirName, fmt.Sprintf("%s_%s", hostName, logFileName))
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		logFile = os.Stdout
		logger.
			WithField("source", "process").
			WithField("process", app.Name).
			WithField("error", err.Error()).
			Error("Can not open log file")
	}
	stdLogger := logging.NewLogger(logFile, logrus.TraceLevel)
	stdLogger.SetFormatter(&logrus.JSONFormatter{})
	return &Process{Name: app.Name, app: app, status: status, mainWaitGroup: wg, config: config, stdLogger: stdLogger, logger: logger}
}

type Status struct {
	Name string  `json:"name"`
	Pid          int `json:"pid"`
	Code         int `json:"code"`
	StartupError string `json:"error"`
	State        string `json:"state"`
	StdOut  []string  `json:"stdout"`
	StdErr  []string  `json:"stderr"`
}

func NewStatus(process *Process) Status {
	states := []string{"Stopped", "Started", "Stopping", "Starting", "Failed"}
	errorMessage := ""
	if process.GetError() != nil {
		errorMessage = process.GetError().Error()
	}
	procStatus := Status{
		Name: process.Name,
		Pid: process.GetPid(),
		Code: process.GetExitCode(),
		StartupError: errorMessage,
		State: states[process.GetState()],
		StdOut: process.GetStdOut(),
		StdErr: process.GetStdErr()}
	return procStatus
}

type Manager struct {
	processes sync.Map
	mainWaitGroup *sync.WaitGroup
	logger *logrus.Logger
}

func (manager *Manager) Append(process *Process) {
	manager.processes.Store(process.Name, process)
}

func (manager *Manager) Delete(name string) {
	value, ok := manager.processes.Load(name)
	if ok {
		proc := value.(*Process)
		proc.Stop()
		manager.processes.Delete(name)
	}
}

func (manager *Manager) StartAll() {
	manager.processes.Range(func(key, value interface{}) bool {
		proc := value.(*Process)
		proc.Start()
		return true
	})
}

func (manager *Manager) StopAll() {
	manager.processes.Range(func(key, value interface{}) bool {
		proc := value.(*Process)
		proc.Stop()
		return true
	})
}

func (manager *Manager) RestartAll() {
	manager.processes.Range(func(key, value interface{}) bool {
		proc := value.(*Process)
		proc.Restart()
		return true
	})
}

func (manager *Manager) Start(name string) {
	value, ok := manager.processes.Load(name)
	if ok {
		proc := value.(*Process)
		proc.Start()
	}
}

func (manager *Manager) Stop(name string) {
	value, ok := manager.processes.Load(name)
	if ok {
		proc := value.(*Process)
		proc.Stop()
	}
}

func (manager *Manager) Restart(name string) {
	value, ok := manager.processes.Load(name)
	if ok {
		proc := value.(*Process)
		proc.Restart()
	}
}

func (manager *Manager) List() []*Process {
	var processes []*Process
	manager.processes.Range(func(key, value interface{}) bool {
		processes = append(processes, value.(*Process))
		return true
	})
	return processes
}

func (manager *Manager) StatusAll() []Status {
	var allStatus []Status
	manager.processes.Range(func(key, value interface{}) bool {
		proc := value.(*Process)
		procStatus := NewStatus(proc)
		allStatus = append(allStatus, procStatus)
		return true
	})
	return allStatus
}

func (manager *Manager) Status(name string) (Status, bool) {
	value, ok := manager.processes.Load(name)
	if ok {
		proc := value.(*Process)
		procStatus := NewStatus(proc)
		return procStatus, true
	}
	return Status{}, false
}

func NewManager(wg *sync.WaitGroup, logger *logrus.Logger) *Manager {
	return &Manager{mainWaitGroup: wg, logger: logger}
}
