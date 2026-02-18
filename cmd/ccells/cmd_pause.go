package main

import (
	"encoding/json"
	"fmt"
)

// runPause sends a pause request to the daemon.
func runPause(stateDir, name string) error {
	daemonSock := stateDir + "/daemon.sock"

	params, _ := json.Marshal(map[string]string{
		"name": name,
	})

	resp, err := sendDaemonRequestWithResponse(daemonSock, "pause", params)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("pause failed: %s", resp.Error)
	}

	fmt.Printf("Workstream '%s' paused.\n", name)
	return nil
}

// runUnpause sends an unpause request to the daemon.
func runUnpause(stateDir, name string) error {
	daemonSock := stateDir + "/daemon.sock"

	params, _ := json.Marshal(map[string]string{
		"name": name,
	})

	resp, err := sendDaemonRequestWithResponse(daemonSock, "unpause", params)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("unpause failed: %s", resp.Error)
	}

	fmt.Printf("Workstream '%s' resumed.\n", name)
	return nil
}
