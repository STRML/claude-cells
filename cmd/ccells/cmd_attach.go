package main

// The "attach" command is handled by runUp() in cmd_up.go.
// If the tmux session already exists, runUp starts the daemon and attaches.
// If not, it creates everything from scratch.
// This file is kept for documentation — the command dispatch is in main.go.
