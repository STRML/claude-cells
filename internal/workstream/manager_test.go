package workstream

import (
	"testing"
)

func TestManager_Add(t *testing.T) {
	m := NewManager()
	ws := New("test prompt")

	m.Add(ws)

	got := m.Get(ws.ID)
	if got != ws {
		t.Error("Get() should return added workstream")
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager()
	ws := New("test prompt")
	m.Add(ws)

	m.Remove(ws.ID)

	got := m.Get(ws.ID)
	if got != nil {
		t.Error("Get() should return nil after Remove()")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()
	ws1 := New("first")
	ws2 := New("second")
	m.Add(ws1)
	m.Add(ws2)

	list := m.List()

	if len(list) != 2 {
		t.Errorf("List() len = %d, want 2", len(list))
	}
}

func TestManager_Count(t *testing.T) {
	m := NewManager()

	if m.Count() != 0 {
		t.Error("Count() should be 0 for new manager")
	}

	m.Add(New("test"))

	if m.Count() != 1 {
		t.Error("Count() should be 1 after Add()")
	}
}

func TestManager_GetByBranch(t *testing.T) {
	m := NewManager()
	ws := New("add user auth")
	m.Add(ws)

	got := m.GetByBranch("add-user-auth")
	if got != ws {
		t.Error("GetByBranch() should return workstream")
	}

	got = m.GetByBranch("nonexistent")
	if got != nil {
		t.Error("GetByBranch() should return nil for nonexistent")
	}
}

func TestManager_Active(t *testing.T) {
	m := NewManager()
	ws1 := New("running")
	ws1.SetState(StateRunning)
	ws2 := New("stopped")
	ws2.SetState(StateStopped)
	m.Add(ws1)
	m.Add(ws2)

	active := m.Active()

	if len(active) != 1 {
		t.Errorf("Active() len = %d, want 1", len(active))
	}
	if active[0] != ws1 {
		t.Error("Active() should return running workstream")
	}
}

func TestManager_GetPairing(t *testing.T) {
	m := NewManager()
	ws := New("pairing")
	ws.SetState(StatePairing)
	m.Add(ws)

	got := m.GetPairing()
	if got != ws {
		t.Error("GetPairing() should return pairing workstream")
	}
}

func TestManager_GetPairing_None(t *testing.T) {
	m := NewManager()
	ws := New("running")
	ws.SetState(StateRunning)
	m.Add(ws)

	got := m.GetPairing()
	if got != nil {
		t.Error("GetPairing() should return nil when none pairing")
	}
}
