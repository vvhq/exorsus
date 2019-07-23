package application

import (
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"strings"
	"sync"
)

type Environment struct {
	Name string `json:"name"`
	Value string `json:"value"`
}

type Application struct {
	Name string               `json:"name"`
	Command string            `json:"command"`
	Arguments string          `json:"arguments"`
	Timeout int               `json:"timeout"`
	User string               `json:"user"`
	Group string              `json:"group"`
	Environment []Environment `json:"environment"`
}

func (app *Application) Copy() (*Application, error) {
	jsonApp, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}
	newApp := Application{}
	err = json.NewDecoder(strings.NewReader(string(jsonApp))).Decode(&newApp)
	if err != nil {
		return nil, err
	}
	return &newApp, nil
}

type Storage struct {
	applications sync.Map
	path string
	lock sync.Mutex
	logger *logrus.Logger
}

func (store *Storage) Add(applicationConfiguration Application) error {
	_, ok := store.applications.Load(applicationConfiguration.Name)
	if ok {
		return errors.New("already exist")
	}
	store.applications.Store(applicationConfiguration.Name, applicationConfiguration)
	store.replace()
	return nil
}

func (store *Storage) Update(name string, applicationConfiguration Application) error {
	_, ok := store.applications.Load(name)
	if !ok {
		return errors.New("not found")
	}
	if name != applicationConfiguration.Name {
		store.applications.Delete(name)
	}
	store.applications.Store(applicationConfiguration.Name, applicationConfiguration)
	store.replace()
	return nil
}

func (store *Storage) Get(name string) (Application, bool){
	rawApplication, ok := store.applications.Load(name)
	if ok {
		applicationConfiguration := rawApplication.(Application)
		return applicationConfiguration, true
	}
	return Application{}, false
}

func (store *Storage) Delete(name string) error {
	_, ok := store.applications.Load(name)
	if !ok {
		return errors.New("not found")
	}
	store.applications.Delete(name)
	store.replace()
	return nil
}

func (store *Storage) List() []Application {
	var applications []Application
	store.applications.Range(func(key, value interface{}) bool {
		applications = append(applications, value.(Application))
		return true
	})
	return applications
}

func (store *Storage) Load() {
	store.lock.Lock()
	defer store.lock.Unlock()
	buf, err := ioutil.ReadFile(store.path)
	if err != nil {
		store.logger.
			WithField("source", "storage").
			WithField("path", store.path).
			WithField("error", err.Error()).
			Error("Can not load config file")
		return
	}
	applicationsJson := string(buf)
	var applications []Application
	err = json.NewDecoder(strings.NewReader(applicationsJson)).Decode(&applications)
	if err != nil {
		store.logger.
			WithField("source", "storage").
			WithField("path", store.path).
			WithField("error", err.Error()).
			Error("Can not decode config file")
		return
	}
	for _, applicationConfiguration := range applications {
		store.applications.Store(applicationConfiguration.Name, applicationConfiguration)
	}
}

func (store *Storage) replace() {
	store.lock.Lock()
	defer store.lock.Unlock()
	var applications []Application
	store.applications.Range(func(key, value interface{}) bool {
		applications = append(applications, value.(Application))
		return true
	})
	prettyJson := "[]"
	if len(applications) != 0 {
		jsonApplications, err := json.MarshalIndent(applications, "", "    ")
		prettyJson = string(jsonApplications)
		if err != nil {
			store.logger.
				WithField("source", "storage").
				WithField("path", store.path).
				WithField("error", err.Error()).
				Error("Can not build JSON for applications")
			return
		}
	}
	err := ioutil.WriteFile(store.path, []byte(prettyJson), 0664)
	if err != nil {
		store.logger.
			WithField("source", "storage").
			WithField("path", store.path).
			WithField("error", err.Error()).
			Error("Can not write JSON to file")
	}
}

func NewStorage(storePath string, logger *logrus.Logger) *Storage {
	store := Storage{logger: logger, path: storePath}
	store.Load()
	return &store
}