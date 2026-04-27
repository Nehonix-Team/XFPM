package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Println("[DIAG] Starting signal test. Press Ctrl+C...")
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-ch:
		fmt.Printf("[DIAG] Received signal: %v\n", sig)
	case <-time.After(10 * time.Second):
		fmt.Println("[DIAG] Timeout waiting for signal")
	}
}
