package main

import (
	"time"

	"github.com/pterm/pterm"
)

func main() {
	s, _ := pterm.DefaultSpinner.Start()
	for i := 0; i < 5; i++ {
		pterm.Printf("Log item %d\n", i)
		time.Sleep(100 * time.Millisecond)
	}
	s.Stop()
}
