package status

import (
	"exorsus/configuration"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type IOStdStore struct {
	max       int
	warehouse []string
	lock      sync.RWMutex
}

func (store *IOStdStore) Append(item string) {
	item = fmt.Sprintf("%s%s%s %s",
		configuration.DefaultStdDatePrefix,
		time.Now().Format(configuration.DefaultStdDateLayout),
		configuration.DefaultStdDateSuffix,
		item)
	store.lock.Lock()
	defer store.lock.Unlock()
	store.warehouse = append(store.warehouse, item)
	if len(store.warehouse) > store.max {
		shifted := make([]string, store.max)
		idx := len(store.warehouse) - store.max
		copy(shifted, store.warehouse[idx:])
		store.warehouse = shifted
	}
}

func (store *IOStdStore) List() []string {
	store.lock.RLock()
	defer store.lock.RUnlock()
	if len(store.warehouse) == 0 {
		return []string{}
	}
	var warehouse []string
	if len(store.warehouse) > store.max {
		warehouse = make([]string, store.max)
		copy(warehouse, store.warehouse[(len(store.warehouse) - store.max):])
	} else {
		warehouse = make([]string, len(store.warehouse))
		copy(warehouse, store.warehouse)
	}
	return warehouse
}

func NewIOStdStore(max int) *IOStdStore {
	return &IOStdStore{max: max}
}

const Stopped int = 0
const Started int = 1
const Stopping int = 2
const Starting int = 3
const Failed int = 4

type Status struct {
	pid          int32
	code         int32
	state        int32
	startupError error
	stdOutStore  *IOStdStore
	stdErrStore  *IOStdStore
	lock         sync.RWMutex
}

func (status *Status) SetPid(pid int) {
	atomic.SwapInt32(&status.pid, int32(pid))
}

func (status *Status) GetPid() int {
	return int(atomic.LoadInt32(&status.pid))
}

func (status *Status) SetExitCode(code int) {
	atomic.SwapInt32(&status.code, int32(code))
}

func (status *Status) GetExitCode() int {
	return int(atomic.LoadInt32(&status.code))
}

func (status *Status) SetState(state int) {
	atomic.SwapInt32(&status.state, int32(state))
}

func (status *Status) GetState() int {
	return int(atomic.LoadInt32(&status.state))
}

func (status *Status) SetError(startupError error) {
	status.lock.Lock()
	defer status.lock.Unlock()
	status.startupError = startupError
}

func (status *Status) GetError() error {
	status.lock.RLock()
	defer status.lock.RUnlock()
	return status.startupError
}

func (status *Status) AddStdOutItem(item string) {
	status.stdOutStore.Append(item)
}

func (status *Status) ListStdOutItems() []string {
	return status.stdOutStore.List()
}

func (status *Status) AddStdErrItem(item string) {
	status.stdErrStore.Append(item)
}

func (status *Status) ListStdErrItems() []string {
	return status.stdErrStore.List()
}

func New(max int) *Status {
	return &Status{pid: 0, code: 0, startupError: nil, state: int32(Stopped), stdOutStore: NewIOStdStore(max), stdErrStore: NewIOStdStore(max)}
}
