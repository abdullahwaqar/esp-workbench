package espworkbench

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type ToolchainEnv struct {
	Found        bool
	ExportScript string
	Err          error
}

var (
	toolchainOnce sync.Once
	toolchain     ToolchainEnv
)

func Toolchain() ToolchainEnv {
	toolchainOnce.Do(func() {
		toolchain = detectToolchain()
	})
	return toolchain
}

func detectToolchain() ToolchainEnv {
	if verifyIdfPyRuns(exec.Command("idf.py", "--version")) {
		return ToolchainEnv{Found: true}
	}

	if script := findExportScript(); script != "" {
		shellCmd := fmt.Sprintf("source %s > /dev/null 2>&1 && idf.py --version", shellQuote(script))
		if verifyIdfPyRuns(exec.Command("bash", "-c", shellCmd)) {
			return ToolchainEnv{Found: true, ExportScript: script}
		}
	}

	return ToolchainEnv{Err: fmt.Errorf("idf.py is not runnable, and no working esp-idf export.sh was found; activate your esp-idf environment before running esp-workbench")}
}

func verifyIdfPyRuns(cmd *exec.Cmd) bool {
	return cmd.Run() == nil
}

func findExportScript() string {
	var candidates []string

	if idfPath := os.Getenv("IDF_PATH"); idfPath != "" {
		candidates = append(candidates, filepath.Join(idfPath, "export.sh"))
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(homeDir, "esp", "esp-idf", "export.sh"),
			filepath.Join(homeDir, "esp-idf", "export.sh"),
			filepath.Join(homeDir, ".espressif", "esp-idf", "export.sh"),
		)
	}

	candidates = append(candidates,
		"/opt/esp-idf/export.sh",
		"/usr/local/esp-idf/export.sh",
	)

	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Builds a command for name+args. If idf.py already runs cleanly on PATH
// this is just exec.Command. Otherwise it sources the detected export.sh
// in a bash subshell first, so the right venv is present without the
// user having to activate anything manually.
func (toolchain ToolchainEnv) Command(name string, args ...string) *exec.Cmd {
	if toolchain.ExportScript == "" {
		return exec.Command(name, args...)
	}
	return exec.Command("bash", "-c", toolchain.shellLine(name, args))
}

func (toolchain ToolchainEnv) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	if toolchain.ExportScript == "" {
		return exec.CommandContext(ctx, name, args...)
	}
	return exec.CommandContext(ctx, "bash", "-c", toolchain.shellLine(name, args))
}

func (toolchain ToolchainEnv) shellLine(name string, args []string) string {
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = shellQuote(arg)
	}
	return fmt.Sprintf("source %s > /dev/null 2>&1 && exec %s %s",
		shellQuote(toolchain.ExportScript), shellQuote(name), strings.Join(quotedArgs, " "))
}
