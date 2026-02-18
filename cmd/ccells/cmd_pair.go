package main

import (
	"encoding/json"
	"fmt"
)

// runPair sends a pair request to the daemon.
func runPair(stateDir, workstreamName string) error {
	daemonSock := stateDir + "/daemon.sock"

	// TODO(task-14): Resolve workstream name to branch/containerID/localPath
	// from the state file. For now, require explicit params.
	if workstreamName == "" {
		return fmt.Errorf("workstream name is required")
	}

	params, _ := json.Marshal(map[string]string{
		"branch":       workstreamName,
		"container_id": workstreamName, // placeholder
		"local_path":   ".",            // placeholder
	})

	resp, err := sendDaemonRequestWithResponse(daemonSock, "pair", params)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("pair failed: %s", resp.Error)
	}

	// Parse and display pairing state
	var state struct {
		Active        bool   `json:"active"`
		CurrentBranch string `json:"current_branch"`
		SyncHealthy   bool   `json:"sync_healthy"`
	}
	if resp.Data != nil {
		json.Unmarshal(resp.Data, &state)
	}

	fmt.Printf("Paired with workstream '%s' (branch: %s)\n", workstreamName, state.CurrentBranch)
	return nil
}

// runUnpair sends an unpair request to the daemon.
func runUnpair(stateDir string) error {
	daemonSock := stateDir + "/daemon.sock"

	resp, err := sendDaemonRequestWithResponse(daemonSock, "unpair", nil)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("unpair failed: %s", resp.Error)
	}

	fmt.Println("Unpairing complete. Local branch restored.")
	return nil
}

// runPairStatus sends a pair-status request and prints the result.
func runPairStatus(stateDir string) error {
	daemonSock := stateDir + "/daemon.sock"

	resp, err := sendDaemonRequestWithResponse(daemonSock, "pair-status", nil)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("pair-status failed: %s", resp.Error)
	}

	var state struct {
		Active         bool     `json:"active"`
		CurrentBranch  string   `json:"current_branch"`
		PreviousBranch string   `json:"previous_branch"`
		ContainerID    string   `json:"container_id"`
		SyncHealthy    bool     `json:"sync_healthy"`
		Conflicts      []string `json:"conflicts"`
		SyncStatusText string   `json:"sync_status_text"`
	}
	if resp.Data != nil {
		json.Unmarshal(resp.Data, &state)
	}

	if !state.Active {
		fmt.Println("Pairing: inactive")
		return nil
	}

	healthStr := "healthy"
	if !state.SyncHealthy {
		healthStr = "UNHEALTHY"
	}

	fmt.Printf("Pairing: active\n")
	fmt.Printf("  Branch:    %s\n", state.CurrentBranch)
	fmt.Printf("  Container: %s\n", state.ContainerID)
	fmt.Printf("  Sync:      %s\n", healthStr)
	if state.SyncStatusText != "" {
		fmt.Printf("  Status:    %s\n", state.SyncStatusText)
	}
	if len(state.Conflicts) > 0 {
		fmt.Printf("  Conflicts: %d\n", len(state.Conflicts))
		for _, c := range state.Conflicts {
			fmt.Printf("    - %s\n", c)
		}
	}

	return nil
}
