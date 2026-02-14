package daemon

import "testing"

func TestReconcileHealthy(t *testing.T) {
	r := &Reconciler{}

	tmuxPanes := []PaneState{
		{PaneID: "%0", Workstream: "auth-system", Container: "ccells-repo-auth"},
	}
	dockerContainers := []ContainerState{
		{ID: "abc123", Name: "ccells-repo-auth", Running: true},
	}

	result := r.Reconcile(tmuxPanes, dockerContainers)

	if len(result.Healthy) != 1 {
		t.Errorf("expected 1 healthy workstream, got %d", len(result.Healthy))
	}
	if len(result.OrphanedContainers) != 0 {
		t.Errorf("expected 0 orphaned containers, got %d", len(result.OrphanedContainers))
	}
	if len(result.OrphanedPanes) != 0 {
		t.Errorf("expected 0 orphaned panes, got %d", len(result.OrphanedPanes))
	}
}

func TestReconcileOrphanedContainer(t *testing.T) {
	r := &Reconciler{}

	// Container exists but no matching pane
	tmuxPanes := []PaneState{}
	dockerContainers := []ContainerState{
		{ID: "abc123", Name: "ccells-repo-auth", Running: true},
	}

	result := r.Reconcile(tmuxPanes, dockerContainers)

	if len(result.OrphanedContainers) != 1 {
		t.Errorf("expected 1 orphaned container, got %d", len(result.OrphanedContainers))
	}
}

func TestReconcileOrphanedPane(t *testing.T) {
	r := &Reconciler{}

	// Pane exists but no matching container
	tmuxPanes := []PaneState{
		{PaneID: "%0", Workstream: "auth-system", Container: "ccells-repo-auth"},
	}
	dockerContainers := []ContainerState{}

	result := r.Reconcile(tmuxPanes, dockerContainers)

	if len(result.OrphanedPanes) != 1 {
		t.Errorf("expected 1 orphaned pane, got %d", len(result.OrphanedPanes))
	}
}

func TestReconcileMultipleMixed(t *testing.T) {
	r := &Reconciler{}

	tmuxPanes := []PaneState{
		{PaneID: "%0", Workstream: "auth", Container: "ccells-repo-auth"},
		{PaneID: "%1", Workstream: "fix-bug", Container: "ccells-repo-fix"},
		{PaneID: "%2", Workstream: "orphan-pane", Container: "ccells-repo-orphan"},
	}
	dockerContainers := []ContainerState{
		{ID: "abc123", Name: "ccells-repo-auth", Running: true},
		{ID: "def456", Name: "ccells-repo-fix", Running: false},
		{ID: "ghi789", Name: "ccells-repo-extra", Running: true},
	}

	result := r.Reconcile(tmuxPanes, dockerContainers)

	if len(result.Healthy) != 2 {
		t.Errorf("expected 2 healthy, got %d", len(result.Healthy))
	}
	if len(result.OrphanedContainers) != 1 {
		t.Errorf("expected 1 orphaned container, got %d", len(result.OrphanedContainers))
	}
	if len(result.OrphanedPanes) != 1 {
		t.Errorf("expected 1 orphaned pane, got %d", len(result.OrphanedPanes))
	}

	// Verify running status is correctly propagated
	for _, h := range result.Healthy {
		if h.Workstream == "fix-bug" && h.Running {
			t.Error("fix-bug should not be running")
		}
	}
}

func TestReconcileIgnoresNonCCellsPanes(t *testing.T) {
	r := &Reconciler{}

	// Pane without Container metadata = not managed by ccells
	tmuxPanes := []PaneState{
		{PaneID: "%0", Workstream: "", Container: ""},
	}
	dockerContainers := []ContainerState{}

	result := r.Reconcile(tmuxPanes, dockerContainers)

	if len(result.OrphanedPanes) != 0 {
		t.Errorf("expected 0 orphaned panes (non-ccells pane should be ignored), got %d", len(result.OrphanedPanes))
	}
}
