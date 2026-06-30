package espworkbench

import (
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	partitionTableOffset = 0x8000

	// 3072 bytes — ESP-IDF's default reserved size for the table
	partitionTableSize = 0xc00

	// bytes per entry in the binary format
	partitionEntrySize = 32
	partitionMagicLow  = 0xAA
	partitionMagicHigh = 0x50
)

type PartitionEntry struct {
	Name      string
	Type      byte
	SubType   byte
	TypeLabel string
	SubLabel  string
	Offset    uint32
	Size      uint32
}

type PartitionTable struct {
	Entries []PartitionEntry

	// total flash size in bytes, 0 if detection failed
	FlashSize uint64
}

type PartitionsReadMsg struct {
	Table PartitionTable
	Err   error
}

// Reads the partition table directly off the connected
// device via esptool.py and decodes it. It also probes the flash chip size
// so usage can be shown proportionally. Reuses the same permission-check
// flow as every other device operation for consistency.
func ReadPartitionTableCmd(port string) tea.Cmd {
	return func() tea.Msg {
		if err := CheckSerialAccess(port); err != nil {
			if fixErr := AttemptTemporaryFix(port); fixErr != nil {
				return PartitionsReadMsg{Err: fmt.Errorf("%v\n\nrun manually: sudo chmod a+rw %s", err, port)}
			}
		}

		tmpFile, err := os.CreateTemp("", "esp-workbench-partitions-*.bin")
		if err != nil {
			return PartitionsReadMsg{Err: fmt.Errorf("could not create temp file: %w", err)}
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		readCmd := Toolchain().Command("esptool.py",
			"--port", port, "-b", "460800",
			"read_flash",
			fmt.Sprintf("0x%x", partitionTableOffset),
			fmt.Sprintf("0x%x", partitionTableSize),
			tmpPath,
		)
		if out, err := readCmd.CombinedOutput(); err != nil {
			return PartitionsReadMsg{Err: fmt.Errorf("failed to read partition table from device: %v\n%s", err, lastLines(string(out), 4))}
		}

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return PartitionsReadMsg{Err: fmt.Errorf("could not read temp file: %w", err)}
		}

		entries, err := parsePartitionTable(data)
		if err != nil {
			return PartitionsReadMsg{Err: err}
		}

		return PartitionsReadMsg{Table: PartitionTable{
			Entries:   entries,
			FlashSize: detectFlashSize(port),
		}}
	}
}

// Decodes the ESP-IDF binary partition table format:
// each 32-byte entry is magic(2) + type(1) + subtype(1) + offset(4) + size(4)
// + label(16, null-padded) + flags(4). The table ends at the first entry
// whose magic bytes don't match (typically 0xFFFF padding on erased flash).
func parsePartitionTable(data []byte) ([]PartitionEntry, error) {
	var entries []PartitionEntry
	for offset := 0; offset+partitionEntrySize <= len(data); offset += partitionEntrySize {
		chunk := data[offset : offset+partitionEntrySize]
		if chunk[0] != partitionMagicLow || chunk[1] != partitionMagicHigh {
			break
		}

		ptype := chunk[2]
		subtype := chunk[3]
		partOffset := binary.LittleEndian.Uint32(chunk[4:8])
		partSize := binary.LittleEndian.Uint32(chunk[8:12])
		name := strings.TrimRight(string(chunk[12:28]), "\x00")

		entries = append(entries, PartitionEntry{
			Name:      name,
			Type:      ptype,
			SubType:   subtype,
			TypeLabel: typeLabel(ptype),
			SubLabel:  subTypeLabel(ptype, subtype),
			Offset:    partOffset,
			Size:      partSize,
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no partition table found at 0x%x — flash may be erased or unflashed", partitionTableOffset)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Offset < entries[j].Offset })
	return entries, nil
}

func typeLabel(t byte) string {
	switch t {
	case 0x00:
		return "app"
	case 0x01:
		return "data"
	default:
		return fmt.Sprintf("0x%02x", t)
	}
}

func subTypeLabel(ptype byte, subtype byte) string {
	switch ptype {
	// app
	case 0x00:
		switch {
		case subtype == 0x00:
			return "factory"
		case subtype >= 0x10 && subtype <= 0x1f:
			return fmt.Sprintf("ota_%d", subtype-0x10)
		case subtype == 0x20:
			return "test"
		}
	// data
	case 0x01:
		switch subtype {
		case 0x00:
			return "ota_data"
		case 0x01:
			return "phy"
		case 0x02:
			return "nvs"
		case 0x03:
			return "coredump"
		case 0x04:
			return "nvs_keys"
		case 0x05:
			return "efuse"
		case 0x06:
			return "undefined"
		case 0x80:
			return "esphttpd"
		case 0x81:
			return "fat"
		case 0x82:
			return "spiffs"
		}
	}
	return fmt.Sprintf("0x%02x", subtype)
}

// Runs esptool flash_id and parses the reported chip size.
// Returns 0 on any failure — callers must treat 0 as "unknown" and fall
// back to sizing the bar by table extent instead of true chip capacity.
func detectFlashSize(port string) uint64 {
	cmd := Toolchain().Command("esptool.py", "--port", port, "flash_id")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0
	}
	matches := flashSizeRegex.FindStringSubmatch(string(output))
	if len(matches) != 3 {
		return 0
	}
	value, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(matches[2]) {
	case "MB":
		return value * 1024 * 1024
	case "KB":
		return value * 1024
	}
	return 0
}

var flashSizeRegex = regexp.MustCompile(`(?i)Detected flash size:\s*(\d+)\s*(MB|KB)`)

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
