package workstream

import (
	"sync"
)

// Manager tracks all workstreams.
type Manager struct {
	mu          sync.RWMutex
	workstreams map[string]*Workstream
}

// NewManager creates a new workstream manager.
func NewManager() *Manager {
	return &Manager{
		workstreams: make(map[string]*Workstream),
	}
}

// Add registers a workstream.
func (m *Manager) Add(ws *Workstream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workstreams[ws.ID] = ws
}

// Remove unregisters a workstream.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workstreams, id)
}

// Get returns a workstream by ID.
func (m *Manager) Get(id string) *Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workstreams[id]
}

// GetByBranch returns a workstream by branch name.
func (m *Manager) GetByBranch(branchName string) *Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ws := range m.workstreams {
		if ws.BranchName == branchName {
			return ws
		}
	}
	return nil
}

// List returns all workstreams.
func (m *Manager) List() []*Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Workstream, 0, len(m.workstreams))
	for _, ws := range m.workstreams {
		list = append(list, ws)
	}
	return list
}

// Active returns workstreams with active containers.
func (m *Manager) Active() []*Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var active []*Workstream
	for _, ws := range m.workstreams {
		if ws.GetState().IsActive() {
			active = append(active, ws)
		}
	}
	return active
}

// Count returns the number of workstreams.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workstreams)
}

// GetPairing returns the workstream in pairing mode, if any.
func (m *Manager) GetPairing() *Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ws := range m.workstreams {
		if ws.GetState() == StatePairing {
			return ws
		}
	}
	return nil
}
