package rest

import (
	"context"
	"encoding/json"
	"exorsus/application"
	"exorsus/configuration"
	"exorsus/process"
	"exorsus/status"
	"exorsus/version"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"sync"
	"time"
)

type Service struct {
	port int
	store *application.Storage
	proc *process.Manager
	server *http.Server
	mainWaitGroup *sync.WaitGroup
	config *configuration.Configuration
	logger *logrus.Logger
}

func (service *Service) Start() {
	router := mux.NewRouter()
	router.HandleFunc("/applications/", service.listApplications).Methods("GET")
	router.HandleFunc("/applications/{name}", service.getApplication).Methods("GET")
	router.HandleFunc("/applications/", service.createApplication).Methods("POST")
	router.HandleFunc("/applications/{name}", service.updateApplication).Methods("PUT")
	router.HandleFunc("/applications/{name}", service.deleteApplication).Methods("DELETE")
	router.HandleFunc("/actions/start/", service.startAll).Methods("GET")
	router.HandleFunc("/actions/stop/", service.stopAll).Methods("GET")
	router.HandleFunc("/actions/restart/", service.restartAll).Methods("GET")
	router.HandleFunc("/actions/start/{name}", service.startApplication).Methods("GET")
	router.HandleFunc("/actions/stop/{name}", service.stopApplication).Methods("GET")
	router.HandleFunc("/actions/restart/{name}", service.restartApplication).Methods("GET")
	router.HandleFunc("/status/", service.statusAll).Methods("GET")
	router.HandleFunc("/status/{name}", service.status).Methods("GET")
	router.HandleFunc("/version/", service.getVersion).Methods("GET")

	service.server = &http.Server{Addr: fmt.Sprintf(":%d", service.port), Handler: router}
	service.logger.
		WithField("source", "rest").
		Trace("Starting REST")
	service.mainWaitGroup.Add(1)
	go func() {
		err := service.server.ListenAndServe()
		if err != nil && err.Error() != "http: Server closed" {
			service.logger.
				WithField("source", "rest").
				WithField("error", err.Error()).
				Error("Can not start REST")
			os.Exit(1)
		}
		service.logger.
			WithField("source", "rest").
			Info("REST stopped")
		service.mainWaitGroup.Done()
	}()
	service.logger.
		WithField("source", "rest").
		Info("REST started")
}

func (service *Service) Stop() {
	service.logger.
		WithField("source", "rest").
		Trace("Stopping REST")
	ctx, _ := context.WithTimeout(context.Background(), 5 * time.Second)
	err := service.server.Shutdown(ctx)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			Error("REST shutdown error")
	}
}

func (service *Service) httpError(responseWriter http.ResponseWriter, request *http.Request, httpStatus int, errorText string) {
	responseWriter.Header().Set("Content-Type", "application/json")
	jsonError := []byte(fmt.Sprintf("{\"error\":\"%s\"}", errorText))
	responseWriter.WriteHeader(httpStatus)
	_, err := responseWriter.Write(jsonError)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Request error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Request success")
	}
}

func (service *Service) httpSuccess(responseWriter http.ResponseWriter, request *http.Request, successText string) {
	responseWriter.Header().Set("Content-Type", "application/json")
	jsonSuccess := []byte(fmt.Sprintf("{\"success\":\"%s\"}", successText))
	responseWriter.WriteHeader(http.StatusOK)
	_, err := responseWriter.Write(jsonSuccess)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Response error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Response success")
	}
}

func (service *Service) getApplication(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "Application name required")
		return
	}
	app, ok := service.store.Get(applicationName)
	if !ok {
		service.httpError(responseWriter, request, http.StatusNotFound, "application not found")
		return
	}
	jsonApp, err := json.Marshal(app)
	if err != nil {
		service.httpError(responseWriter, request, http.StatusBadRequest, err.Error())
		return
	}
	_, err = responseWriter.Write(jsonApp)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Response error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Response success")
	}
}

func (service *Service) listApplications(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	applications := service.store.List()
	jsonApplications, err := json.Marshal(applications)
	if err != nil {
		service.httpError(responseWriter, request, http.StatusBadRequest, err.Error())
		return
	}
	if len(applications) == 0 {
		service.httpError(responseWriter, request, http.StatusNotFound, "no applications found")
		return
	}
	_, err = responseWriter.Write(jsonApplications)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Response error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Response success")
	}
}

func (service *Service) createApplication(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	var app application.Application
	err := json.NewDecoder(request.Body).Decode(&app)
	if err != nil {
		service.httpError(responseWriter, request, http.StatusBadRequest, err.Error())
		return
	}
	err = service.store.Add(app)
	if err != nil {
		service.httpError(responseWriter, request, 400, err.Error())
	} else {
		service.proc.Append(process.New(&app, status.New(100), service.mainWaitGroup, service.config, service.logger))
		service.httpSuccess(responseWriter, request, app.Name)
	}
}

func (service *Service) updateApplication(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "Application name required")
		return
	}
	var app application.Application
	err := json.NewDecoder(request.Body).Decode(&app)
	if err != nil {
		service.httpError(responseWriter, request, http.StatusBadRequest, err.Error())
		return
	}
	err = service.store.Update(applicationName, app)
	if err != nil {
		service.httpError(responseWriter, request, 404, err.Error())
	} else {
		procStatus, _ := service.proc.Status(applicationName)
		updatedProc := process.New(&app, status.New(100), service.mainWaitGroup, service.config, service.logger)
		service.proc.Delete(applicationName)
		service.proc.Append(updatedProc)
		if procStatus.State == "Started" {
			updatedProc.Start()
		}
		service.httpSuccess(responseWriter, request, app.Name)
	}
}

func (service *Service) deleteApplication(responseWriter http.ResponseWriter, request *http.Request) {
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "application name required")
		return
	}
	err := service.store.Delete(applicationName)
	if err != nil {
		service.httpError(responseWriter, request, 404, err.Error())
	} else {
		service.proc.Delete(applicationName)
		service.httpSuccess(responseWriter, request, applicationName)
	}
}

func (service *Service) startApplication(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "application name required")
		return
	}
	app, ok := service.store.Get(applicationName)
	if !ok {
		service.httpError(responseWriter, request, http.StatusNotFound, "application not found")
		return
	}
	service.proc.Start(app.Name)
	service.httpSuccess(responseWriter, request, app.Name)
}

func (service *Service) stopApplication(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "Application name required")
		return
	}
	app, ok := service.store.Get(applicationName)
	if !ok {
		service.httpError(responseWriter, request, http.StatusNotFound, "application not found")
		return
	}
	service.proc.Stop(app.Name)
	service.httpSuccess(responseWriter, request, app.Name)
}

func (service *Service) restartApplication(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "Application name required")
		return
	}
	app, ok := service.store.Get(applicationName)
	if !ok {
		service.httpError(responseWriter, request, http.StatusNotFound, "application not found")
		return
	}
	service.proc.Restart(app.Name)
	service.httpSuccess(responseWriter, request, app.Name)
}

func (service *Service) startAll(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	applications := service.store.List()
	if len(applications) == 0 {
		service.httpError(responseWriter, request, http.StatusNotFound, "no applications found")
		return
	}
	service.proc.StartAll()
	service.httpSuccess(responseWriter, request, "all")
}

func (service *Service) stopAll(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	applications := service.store.List()
	if len(applications) == 0 {
		service.httpError(responseWriter, request, http.StatusNotFound, "no applications found")
		return
	}
	service.proc.StopAll()
	service.httpSuccess(responseWriter, request, "all")
}

func (service *Service) restartAll(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	applications := service.store.List()
	if len(applications) == 0 {
		service.httpError(responseWriter, request, http.StatusNotFound, "no applications found")
		return
	}
	service.proc.RestartAll()
	service.httpSuccess(responseWriter, request, "all")
}

func (service *Service) statusAll(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	allStatus := service.proc.StatusAll()
	jsonAllStatus, err := json.Marshal(allStatus)
	if err != nil {
		service.httpError(responseWriter, request, http.StatusBadRequest, err.Error())
		return
	}
	if len(allStatus) == 0 {
		service.httpError(responseWriter, request, http.StatusNotFound, "status: no applications found")
		return
	}
	_, err = responseWriter.Write(jsonAllStatus)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Response error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Response success")
	}
}

func (service *Service) status(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	urlParameters := mux.Vars(request)
	applicationName, ok := urlParameters["name"]
	if !ok {
		service.httpError(responseWriter, request, http.StatusBadRequest, "application name required")
		return
	}
	app, ok := service.store.Get(applicationName)
	if !ok {
		service.httpError(responseWriter, request, http.StatusNotFound, "application not found")
		return
	}
	appStatus, ok := service.proc.Status(app.Name)
	if !ok {
		service.httpError(responseWriter, request, http.StatusNotFound, "application not found")
		return
	}
	jsonAppStatus, err := json.Marshal(appStatus)
	if err != nil {
		service.httpError(responseWriter, request, http.StatusBadRequest, err.Error())
		return
	}
	_, err = responseWriter.Write(jsonAppStatus)
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Response error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Response success")
	}
}

func (service *Service) getVersion(responseWriter http.ResponseWriter, request *http.Request) {
	jsonVersion := fmt.Sprintf("{\"version\": \"%s\"}", version.Version)
	responseWriter.Header().Set("Content-Type", "application/json")
	_, err := responseWriter.Write([]byte(jsonVersion))
	if err != nil {
		service.logger.
			WithField("source", "rest").
			WithField("error", err.Error()).
			WithField("request", request.RequestURI).
			Error("Response error")
	} else {
		service.logger.
			WithField("source", "rest").
			WithField("request", request.RequestURI).
			Trace("Response success")
	}
}

func New(port int, store *application.Storage, proc *process.Manager, wg *sync.WaitGroup, config *configuration.Configuration, logger *logrus.Logger) *Service {
	return &Service{port: port, store: store, proc: proc, mainWaitGroup: wg, config: config, logger: logger}
}
