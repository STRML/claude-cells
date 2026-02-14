package daemon

// PaneState represents a tmux pane from list-panes.
type PaneState struct {
	PaneID     string
	Workstream string // from @ccells-workstream option
	Container  string // from @ccells-container option
}

// ContainerState represents a Docker container.
type ContainerState struct {
	ID      string
	Name    string
	Running bool
	Labels  map[string]string
}

// ReconcileResult describes the current state after cross-referencing
// tmux panes with Docker containers.
type ReconcileResult struct {
	Healthy            []HealthyWorkstream
	OrphanedContainers []ContainerState // container running, no matching pane
	OrphanedPanes      []PaneState      // pane exists, no matching container
}

// HealthyWorkstream is a pane+container pair that match.
type HealthyWorkstream struct {
	PaneID      string
	ContainerID string
	Workstream  string
	Running     bool
}

// Reconciler cross-references tmux and Docker state.
type Reconciler struct{}

// Reconcile compares tmux panes with Docker containers and categorizes them.
func (r *Reconciler) Reconcile(panes []PaneState, containers []ContainerState) ReconcileResult {
	result := ReconcileResult{}

	// Index containers by name
	containerByName := make(map[string]ContainerState, len(containers))
	matched := make(map[string]bool)
	for _, c := range containers {
		containerByName[c.Name] = c
	}

	// Match panes to containers
	for _, p := range panes {
		if p.Container == "" {
			continue // non-ccells pane, ignore
		}
		if c, ok := containerByName[p.Container]; ok {
			result.Healthy = append(result.Healthy, HealthyWorkstream{
				PaneID:      p.PaneID,
				ContainerID: c.ID,
				Workstream:  p.Workstream,
				Running:     c.Running,
			})
			matched[p.Container] = true
		} else {
			result.OrphanedPanes = append(result.OrphanedPanes, p)
		}
	}

	// Find unmatched containers
	for _, c := range containers {
		if !matched[c.Name] {
			result.OrphanedContainers = append(result.OrphanedContainers, c)
		}
	}

	return result
}
