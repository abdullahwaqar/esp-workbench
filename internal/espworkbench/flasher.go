package espworkbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type FileBrowserEntry struct {
	Name        string
	IsDir       bool
	Size        int64
	SizeLabel   string
	IsFullFlash bool
}

type DirLoadedMsg struct {
	Path  string
	Items []FileBrowserEntry
	Err   error
}

// Opens the browser at <projectPath>/build/ if it exists,
// otherwise at projectPath itself.
func StartBrowserCmd(projectPath string) tea.Cmd {
	startPath := filepath.Join(projectPath, "build")
	if _, err := os.Stat(startPath); err != nil {
		startPath = projectPath
	}
	return BrowseDirCmd(startPath)
}

// Loads a directory asynchronously and returns DirLoadedMsg.
func BrowseDirCmd(path string) tea.Cmd {
	return func() tea.Msg {
		items, err := browseDir(path)
		return DirLoadedMsg{Path: path, Items: items, Err: err}
	}
}

func browseDir(path string) ([]FileBrowserEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var result []FileBrowserEntry

	// ".." always first so user can navigate up; omit only at true filesystem root
	if filepath.Dir(path) != path {
		result = append(result, FileBrowserEntry{Name: "..", IsDir: true})
	}

	// Synthetic [full flash] entry when the ESP-IDF build manifest is present
	if _, err := os.Stat(filepath.Join(path, "flasher_args.json")); err == nil {
		result = append(result, FileBrowserEntry{Name: "[full flash]", IsFullFlash: true})
	}

	// Non-hidden directories first, then .bin files
	var dirs []FileBrowserEntry
	var bins []FileBrowserEntry
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, _ := entry.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		if entry.IsDir() {
			dirs = append(dirs, FileBrowserEntry{Name: entry.Name(), IsDir: true})
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".bin") {
			bins = append(bins, FileBrowserEntry{
				Name:      entry.Name(),
				Size:      size,
				SizeLabel: formatFileSize(size),
			})
		}
	}
	result = append(result, dirs...)
	result = append(result, bins...)
	return result, nil
}

type BinFlashOption struct {
	Label       string
	IsFullFlash bool
	BinPath     string
	FlashAddr   string   // e.g., "0x10000"
	EspArgs     []string // write_flash + all args (full flash only)
	FileCount   int
}

// Infers the flash offset for a single .bin file.
//
// A merged image (idf.py merge-bin, esptool merge_bin) always goes to 0x0 and
// is recognised by the partition table magic it carries at 0x8000, not by its
// size: a normal application image is often larger than 512 KB, and writing
// one at 0x0 would overwrite the bootloader and partition table.
//
// The bootloader offset depends on the chip family, so target is the project's
// IDF target ("esp32", "esp32-s3", "esp32s3", ...). Pass "" when unknown.
func GuessFlashAddr(path string, name string, target string) string {
	if isMergedImage(path) {
		return "0x0"
	}
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "bootloader"):
		return bootloaderOffset(target)
	case strings.Contains(lower, "partition"):
		return "0x8000"
	case strings.Contains(lower, "ota_data"):
		// otadata offset in ESP-IDF's two-OTA partition layouts
		return "0xd000"
	default:
		return "0x10000"
	}
}

// Reports whether the file at path contains a partition table at 0x8000,
// which only a merged full-flash image does.
func isMergedImage(path string) bool {
	if path == "" {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	magic := make([]byte, 2)
	if _, err := file.ReadAt(magic, partitionTableOffset); err != nil {
		return false
	}
	return magic[0] == partitionMagicLow && magic[1] == partitionMagicHigh
}

// Second-stage bootloader offset per chip family.
func bootloaderOffset(target string) string {
	switch strings.ReplaceAll(strings.ToLower(target), "-", "") {
	case "esp32", "esp32s2", "":
		return "0x1000"
	case "esp32p4", "esp32c5":
		return "0x2000"
	default:
		// esp32-s3, esp32-c2, esp32-c3, esp32-c6, esp32-c61, esp32-h2, ...
		return "0x0"
	}
}

// Parses flasher_args.json from buildDir and returns a
// ready-to-use BinFlashOption. All file paths are made absolute.
func BuildFullFlashOption(buildDir string) (BinFlashOption, error) {
	argsPath := filepath.Join(buildDir, "flasher_args.json")
	args, count, err := parseFlasherArgs(argsPath, buildDir)
	if err != nil {
		return BinFlashOption{}, err
	}
	return BinFlashOption{
		Label:       "full flash",
		IsFullFlash: true,
		EspArgs:     args,
		FileCount:   count,
	}, nil
}

type flasherArgsJSON struct {
	WriteFlashArgs []string          `json:"write_flash_args"`
	FlashFiles     map[string]string `json:"flash_files"`
}

// Reads flasher_args.json and assembles esptool.py arguments
// with absolute paths so the caller does not need to set cmd.Dir.
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
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].addrInt < sorted[j].addrInt })

	args = []string{"write_flash"}
	args = append(args, fa.WriteFlashArgs...)
	for _, af := range sorted {
		args = append(args, af.addr, filepath.Join(buildDir, af.file))
	}
	return args, len(sorted), nil
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

// Executes esptool.py to flash a binary (or full image) to the
// device at port. It reuses the same permission-check and log-streaming
// infrastructure as RunIDFCommand.
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

		if err := ensureSerialAccess(port, logChannel); err != nil {
			return OperationDoneMsg{Err: err}
		}

		cmd := Toolchain().Command("esptool.py", cmdArgs...)
		return runStreaming(cmd, "esptool.py", "flash complete", logChannel)
	}
}
