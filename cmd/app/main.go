package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tuiapp "espworkbench/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// Set at build time by the Makefile and the release workflow:
// -ldflags "-X main.Version=v1.2.3 -X 'main.BuildTime=...'"
var (
	Version   = "dev"
	BuildTime = ""
)

func main() {
	var projectPath string
	var version bool

	flag.StringVar(&projectPath, "project", ".", "path to esp-idf project folder")
	flag.BoolVar(&version, "version", false, "show version")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: esp-workbench [--project PATH | PATH]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if version {
		if BuildTime != "" {
			fmt.Printf("esp-workbench %s (built %s)\n", Version, BuildTime)
		} else {
			fmt.Printf("esp-workbench %s\n", Version)
		}
		os.Exit(0)
	}

	projectFlagSet := false
	flag.Visit(func(f *flag.Flag) { projectFlagSet = projectFlagSet || f.Name == "project" })
	switch {
	case flag.NArg() > 1:
		fmt.Fprintln(os.Stderr, "error: expected at most one project path")
		flag.Usage()
		os.Exit(2)
	case flag.NArg() == 1 && projectFlagSet:
		fmt.Fprintln(os.Stderr, "error: give the project path either as an argument or with --project, not both")
		os.Exit(2)
	case flag.NArg() == 1:
		projectPath = flag.Arg(0)
	}

	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid project path: %v\n", err)
		os.Exit(1)
	}

	if info, err := os.Stat(absProjectPath); err != nil || !info.IsDir() {
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
