package web

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Task 是异步操作（下载/导入）的状态快照
type Task struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`           // running | done | error
	Phase     string    `json:"phase,omitempty"` // download | decompress | install
	Detail    string    `json:"detail,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewTaskManager() *TaskManager {
	return &TaskManager{tasks: map[string]*Task{}}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start 注册并异步执行 fn；upd 更新进度（phase/detail），fn 返回错误即终态 error
func (m *TaskManager) Start(id string, fn func(upd func(phase, detail string) error) error) {
	if id == "" {
		id = newID()
	}
	m.mu.Lock()
	m.tasks[id] = &Task{ID: id, State: "running", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	m.mu.Unlock()
	go func() {
		upd := func(phase, detail string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			t := m.tasks[id]
			if t == nil {
				return nil
			}
			t.Phase, t.Detail, t.UpdatedAt = phase, detail, time.Now()
			return nil
		}
		err := fn(upd)
		m.mu.Lock()
		defer m.mu.Unlock()
		t := m.tasks[id]
		if t == nil {
			return
		}
		t.UpdatedAt = time.Now()
		if err != nil {
			t.State, t.Error = "error", err.Error()
		} else {
			t.State = "done"
		}
	}()
}

func (m *TaskManager) Get(id string) (Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *t, true
}
