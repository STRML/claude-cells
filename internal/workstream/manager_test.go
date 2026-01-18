package workstream

import (
	"testing"
)

func TestManager_Add(t *testing.T) {
	m := NewManager()
	ws := New("test prompt")

	if err := m.Add(ws); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	got := m.Get(ws.ID)
	if got != ws {
		t.Error("Get() should return added workstream")
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager()
	ws := New("test prompt")
	_ = m.Add(ws)

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
	_ = m.Add(ws1)
	_ = m.Add(ws2)

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

	_ = m.Add(New("test"))

	if m.Count() != 1 {
		t.Error("Count() should be 1 after Add()")
	}
}

func TestManager_GetByBranch(t *testing.T) {
	m := NewManager()
	ws := New("add user auth")
	_ = m.Add(ws)

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
	_ = m.Add(ws1)
	_ = m.Add(ws2)

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
	_ = m.Add(ws)

	got := m.GetPairing()
	if got != ws {
		t.Error("GetPairing() should return pairing workstream")
	}
}

func TestManager_GetPairing_None(t *testing.T) {
	m := NewManager()
	ws := New("running")
	ws.SetState(StateRunning)
	_ = m.Add(ws)

	got := m.GetPairing()
	if got != nil {
		t.Error("GetPairing() should return nil when none pairing")
	}
}

func TestManager_MaxWorkstreams(t *testing.T) {
	m := NewManager()

	// Add up to the limit
	for i := 0; i < MaxWorkstreams; i++ {
		ws := New("test")
		if err := m.Add(ws); err != nil {
			t.Fatalf("Add() at %d should succeed, got error: %v", i, err)
		}
	}

	// Adding one more should fail
	ws := New("one too many")
	err := m.Add(ws)
	if err != ErrMaxWorkstreams {
		t.Errorf("Add() beyond limit should return ErrMaxWorkstreams, got %v", err)
	}

	if m.Count() != MaxWorkstreams {
		t.Errorf("Count() = %d, want %d", m.Count(), MaxWorkstreams)
	}
}

func TestManager_CanAdd(t *testing.T) {
	m := NewManager()

	if !m.CanAdd() {
		t.Error("CanAdd() should return true for empty manager")
	}

	// Fill up to limit
	for i := 0; i < MaxWorkstreams; i++ {
		_ = m.Add(New("test"))
	}

	if m.CanAdd() {
		t.Error("CanAdd() should return false when at limit")
	}

	// Remove one
	list := m.List()
	m.Remove(list[0].ID)

	if !m.CanAdd() {
		t.Error("CanAdd() should return true after removing one")
	}
}
