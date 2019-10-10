package configuration

import (
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

const DefaultConfigPath string = "./config/"
const DefaultLogPath string = "./log/"
const DefaultLogLevel string = "info"
const DefaultLogMaxSize int = 10
const DefaultLogMaxBackups int = 30
const DefaultLogMaxAge int = 28
const DefaultLogLocalTime bool = true
const DefaultStdLinesCount int = 500
const DefaultStdDateLayout = "2006-01-02 15:04:05"
const DefaultStdDatePrefix = "["
const DefaultStdDateSuffix = "]"
const DefaultShutdownTimeout int = 4
const DefaultListenPort int = 5202
const DefaultConfigurationFileName string = "config.json"
const DefaultLogFileName string = "log.json"
const DefaultApplicationsFileName string = "applications.json"
const DefaultPidPath string = "/tmp/"
const DefaultPidFileName string = "exorsus.pid"

type Configuration struct {
	LogPath         string
	LogLevel        string
	LogMaxSize      int
	LogMaxBackups   int
	LogMaxAge       int
	LogLocalTime    bool
	StdLinesCount   int
	ShutdownTimeout int
	ListenPort      int
	DateLayout      string
	DatePrefix      string
	DateSuffix      string
	PidPath         string
	PidFileName     string
}

func (config *Configuration) GetLogPath() string {
	return config.LogPath
}

func (config *Configuration) GetLogLevel() logrus.Level {
	switch config.LogLevel {
	case "trace":
		return logrus.TraceLevel
	case "debug":
		return logrus.DebugLevel
	case "info":
		return logrus.InfoLevel
	case "warn":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	default:
		return logrus.TraceLevel
	}
}

func (config *Configuration) GetMaxStdLines() int {
	return config.StdLinesCount
}

func (config *Configuration) GetShutdownTimeout() int {
	return config.ShutdownTimeout
}

func (config *Configuration) GetListenPort() int {
	return config.ListenPort
}

func (config *Configuration) applyDefaults() {
	config.LogPath = DefaultLogPath
	config.LogLevel = DefaultLogLevel
	config.StdLinesCount = DefaultStdLinesCount
	config.ShutdownTimeout = DefaultShutdownTimeout
	config.ListenPort = DefaultListenPort
	config.DateLayout = DefaultStdDateLayout
	config.DatePrefix = DefaultStdDatePrefix
	config.DateSuffix = DefaultStdDateSuffix
	config.LogMaxSize = DefaultLogMaxSize
	config.LogMaxBackups = DefaultLogMaxBackups
	config.LogMaxAge = DefaultLogMaxAge
	config.LogLocalTime = DefaultLogLocalTime
	config.PidPath = DefaultPidPath
	config.PidFileName = DefaultPidFileName
	if _, err := os.Stat(DefaultConfigPath); os.IsNotExist(err) {
		err := os.Mkdir(DefaultConfigPath, 0755)
		if err != nil {
			fmt.Printf("Can not create default configuration directory: %s\n", err.Error())
			return
		}
	}
	if _, err := os.Stat(DefaultLogPath); os.IsNotExist(err) {
		err := os.Mkdir(DefaultLogPath, 0755)
		if err != nil {
			fmt.Printf("Can not create default log directory: %s\n", err.Error())
			return
		}
	}
	config.saveDefaults()
}

func (config *Configuration) saveDefaults() {
	prettyJson := "[]"
	jsonConfig, err := json.MarshalIndent(config, "", "    ")
	prettyJson = string(jsonConfig)
	if err != nil {
		fmt.Printf("Can not marshal JSON: %s\n", err.Error())
		return
	}
	err = ioutil.WriteFile(path.Join(path.Dir(DefaultConfigPath), DefaultConfigurationFileName), []byte(prettyJson), 0664)
	if err != nil {
		fmt.Printf("Can not save defaults: %s\n", err.Error())
	}
}

func New(configPath string) *Configuration {
	config := Configuration{}
	buf, err := ioutil.ReadFile(path.Join(path.Dir(configPath), DefaultConfigurationFileName))
	if err != nil {
		fmt.Printf("Configuration load error: %s\n", err.Error())
		config.applyDefaults()
		return &config
	}
	configJson := string(buf)
	err = json.NewDecoder(strings.NewReader(configJson)).Decode(&config)
	if err != nil {
		fmt.Printf("Configuration decode error: %s\n", err.Error())
		config.applyDefaults()
		return &config
	}
	return &config
}
