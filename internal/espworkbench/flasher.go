package espworkbench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// BinFlashOption is a single flashable item discovered in the project build directory.
type BinFlashOption struct {
	Label       string   // display name shown in the picker
	Description string   // address + size, or file count for full flash
	IsFullFlash bool     // true = multi-file flash from flasher_args.json
	BinPath     string   // absolute path (single-file flash only)
	FlashAddr   string   // guessed or parsed address (single-file only)
	BuildDir    string   // working directory for esptool.py (full flash)
	EspArgs     []string // write_flash + all args built from flasher_args.json
	FileCount   int      // number of files (full flash only)
}

// BinFilesScannedMsg is returned by ScanBinFilesCmd.
type BinFilesScannedMsg []BinFlashOption

// ScanBinFilesCmd scans <projectPath>/build/ for flashable binaries.
func ScanBinFilesCmd(projectPath string) tea.Cmd {
	return func() tea.Msg {
		return BinFilesScannedMsg(scanBinFiles(projectPath))
	}
}

func scanBinFiles(projectPath string) []BinFlashOption {
	var options []BinFlashOption
	buildDir := filepath.Join(projectPath, "build")

	// Highest-quality option: full flash using the addresses idf.py computed.
	flasherArgsPath := filepath.Join(buildDir, "flasher_args.json")
	if espArgs, count, err := parseFlasherArgs(flasherArgsPath, buildDir); err == nil {
		options = append(options, BinFlashOption{
			Label:       "full flash",
			Description: fmt.Sprintf("%d files  ·  flasher_args.json", count),
			IsFullFlash: true,
			BuildDir:    buildDir,
			EspArgs:     espArgs,
			FileCount:   count,
		})
	}

	// Individual .bin files at the root of build/ (not in subdirectories).
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return options
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bin") {
			continue
		}
		info, _ := entry.Info()
		size := info.Size()
		addr := guessFlashAddr(entry.Name(), size)
		options = append(options, BinFlashOption{
			Label:       entry.Name(),
			Description: fmt.Sprintf("%s  ·  %s", addr, formatFileSize(size)),
			BinPath:     filepath.Join(buildDir, entry.Name()),
			FlashAddr:   addr,
		})
	}
	return options
}

// flasherArgsJSON mirrors the structure of idf.py's build/flasher_args.json.
type flasherArgsJSON struct {
	WriteFlashArgs []string          `json:"write_flash_args"`
	FlashFiles     map[string]string `json:"flash_files"`
}

// parseFlasherArgs reads flasher_args.json and returns a complete set of
// arguments for esptool.py write_flash, sorted by address ascending.
// All file paths are made absolute using buildDir so esptool.py can find
// them regardless of process working directory.
func parseFlasherArgs(path string, buildDir string) (args []string, fileCount int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	var fa flasherArgsJSON
	if err := json.Unmarshal(data, &fa); err != nil {
		return nil, 0, err
	}

	type addrFile struct {
		addr    string
		file    string
		addrInt uint64
	}
	var sorted []addrFile
	for addr, file := range fa.FlashFiles {
		n, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(addr), "0x"), 16, 64)
		sorted = append(sorted, addrFile{addr: addr, file: file, addrInt: n})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].addrInt < sorted[j].addrInt
	})

	args = []string{"write_flash"}
	args = append(args, fa.WriteFlashArgs...)
	for _, af := range sorted {
		// absolute path — esptool.py resolves files before Python CWD applies
		absPath := filepath.Join(buildDir, af.file)
		args = append(args, af.addr, absPath)
	}
	return args, len(sorted), nil
}

// guessFlashAddr infers the correct flash offset from filename and size.
func guessFlashAddr(name string, size int64) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "bootloader"):
		return "0x1000"
	case strings.Contains(lower, "partition"):
		return "0x8000"
	case size > 512*1024: // anything over 512 KB is almost certainly a merged image
		return "0x0"
	default:
		return "0x10000" // standard app partition offset
	}
}

func formatFileSize(size int64) string {
	switch {
	case size >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%d KB", size/1024)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// RunFlashBin flashes a single binary or a full multi-file image using esptool.py.
// It reuses the same permission-check and streaming logic as RunIDFCommand.
func RunFlashBin(port string, option BinFlashOption, logChannel chan LogLine) tea.Cmd {
	return func() tea.Msg {
		var cmdArgs []string

		if option.IsFullFlash {
			cmdArgs = append([]string{"--port", port, "-b", "460800"}, option.EspArgs...)
			logChannel <- LogLine{
				Text:  fmt.Sprintf("$ esptool.py --port %s write_flash  [%d files via flasher_args.json]", port, option.FileCount),
				Level: LogLevelSystem,
			}
		} else {
			cmdArgs = []string{
				"--port", port, "-b", "460800",
				"write_flash", "--flash_mode", "keep", "--flash_size", "detect",
				option.FlashAddr, option.BinPath,
			}
			logChannel <- LogLine{
				Text:  fmt.Sprintf("$ esptool.py --port %s write_flash %s %s", port, option.FlashAddr, option.Label),
				Level: LogLevelSystem,
			}
		}

		if err := CheckSerialAccess(port); err != nil {
			logChannel <- LogLine{Text: err.Error(), Level: LogLevelError}
			logChannel <- LogLine{Text: "", Level: LogLevelSystem}
			logChannel <- LogLine{Text: "attempting automatic permission fix...", Level: LogLevelSystem}
			if fixErr := AttemptTemporaryFix(port); fixErr == nil {
				logChannel <- LogLine{Text: "permissions fixed. retrying...", Level: LogLevelSuccess}
				if reCheckErr := CheckSerialAccess(port); reCheckErr != nil {
					logChannel <- LogLine{Text: reCheckErr.Error(), Level: LogLevelError}
					return OperationDoneMsg{Err: reCheckErr}
				}
			} else {
				logChannel <- LogLine{Text: "automatic fix failed. run the command shown above manually.", Level: LogLevelWarn}
				return OperationDoneMsg{Err: err}
			}
		}

		cmd := exec.Command("esptool.py", cmdArgs...)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			logChannel <- LogLine{Text: "stdout pipe error: " + err.Error(), Level: LogLevelError}
			return OperationDoneMsg{Err: err}
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			logChannel <- LogLine{Text: "stderr pipe error: " + err.Error(), Level: LogLevelError}
			return OperationDoneMsg{Err: err}
		}

		if err := cmd.Start(); err != nil {
			logChannel <- LogLine{Text: "failed to start esptool.py: " + err.Error(), Level: LogLevelError}
			return OperationDoneMsg{Err: err}
		}

		done := make(chan struct{}, 2)
		streamPipe := func(reader interface{ Read([]byte) (int, error) }) {
			defer func() { done <- struct{}{} }()
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				logChannel <- LogLine{Text: line, Level: classifyLog(line)}
			}
		}
		go streamPipe(stdout)
		go streamPipe(stderr)
		<-done
		<-done

		err = cmd.Wait()
		if err != nil {
			logChannel <- LogLine{Text: fmt.Sprintf("exited with error: %v", err), Level: LogLevelError}
		} else {
			logChannel <- LogLine{Text: "flash complete", Level: LogLevelSuccess}
		}
		return OperationDoneMsg{Err: err}
	}
}
