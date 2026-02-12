package main

import (
	"encoding/json"
	"fmt"
)

// runRemove sends a remove request to the daemon.
func runRemove(stateDir, name string) error {
	daemonSock := stateDir + "/daemon.sock"

	params, _ := json.Marshal(map[string]string{
		"name": name,
	})

	conn, resp, err := sendDaemonRequestWithResponse(daemonSock, "rm", params)
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("rm failed: %s", resp.Error)
	}

	fmt.Printf("Workstream '%s' destroyed.\n", name)
	return nil
}
