package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tuiapp "espworkbench/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var projectPath string
	var version bool

	flag.StringVar(&projectPath, "project", ".", "path to esp-idf project folder")
	flag.BoolVar(&version, "version", false, "show version")
	flag.Parse()

	if version {
		fmt.Println("esp-workbench v1.0.0")
		os.Exit(0)
	}

	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid project path: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(absProjectPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: project folder not found: %s\n", absProjectPath)
		os.Exit(1)
	}

	program := tea.NewProgram(
		tuiapp.InitialModel(absProjectPath),
		tea.WithAltScreen(),
	)

	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
