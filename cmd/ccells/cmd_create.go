package main

import (
	"encoding/json"
	"fmt"
)

// runCreate sends a create request to the daemon.
func runCreate(stateDir, branch, prompt, runtime string) error {
	daemonSock := stateDir + "/daemon.sock"

	params, _ := json.Marshal(map[string]string{
		"branch":  branch,
		"prompt":  prompt,
		"runtime": runtime,
	})

	conn, resp, err := sendDaemonRequestWithResponse(daemonSock, "create", params)
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("create failed: %s", resp.Error)
	}

	fmt.Printf("Workstream created on branch: %s\n", branch)
	return nil
}
