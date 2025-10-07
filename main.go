package main

import (
	"fmt"
	"os"
)

func main() {
	// Create and run the GUI application
	gui := NewGUI()

	// Set up error handling
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Application panic: %v\n", r)
			os.Exit(1)
		}
	}()

	// Run the GUI
	gui.Run()
}
