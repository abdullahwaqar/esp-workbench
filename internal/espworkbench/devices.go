package espworkbench

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// scanPorts returns available serial port paths for the current OS.
func scanPorts() []string {
	var ports []string
	switch runtime.GOOS {
	case "linux":
		for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
			matches, _ := filepath.Glob(pattern)
			ports = append(ports, matches...)
		}
	case "darwin":
		for _, pattern := range []string{
			"/dev/cu.usbserial*",
			"/dev/cu.SLAB_USBtoUART*",
			"/dev/cu.usbmodem*",
			"/dev/cu.wchusbserial*",
		} {
			matches, _ := filepath.Glob(pattern)
			ports = append(ports, matches...)
		}
	case "windows":
		output, err := exec.Command("cmd", "/c", "mode").Output()
		if err == nil {
			for _, line := range strings.Split(string(output), "\r\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "COM") && strings.HasSuffix(line, ":") {
					ports = append(ports, strings.TrimSuffix(line, ":"))
				}
			}
		}
	}
	return ports
}

// probeChip runs esptool.py to read chip info from a port (3s timeout).
func probeChip(port string) (chipType string, macAddr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "esptool.py", "--port", port, "--no-stub", "chip_id")
	output, err := cmd.Output()
	if err != nil {
		outputString := string(output)

		switch {
		case strings.Contains(outputString, "Permission denied"):
			return "[NO ACCESS]", ""
		case strings.Contains(outputString, "could not open port"):
			return "[BUSY]", ""
		case strings.Contains(outputString, "timeout"):
			return "[TIMEOUT]", ""
		default:
			return "[UNKNOWN]", ""
		}
	}

	chipType = "ESP32"
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "Chip is "); idx >= 0 {
			raw := strings.TrimSpace(line[idx+8:])
			// strip "(revision x.y)" suffix — it wraps in narrow panels and adds little value
			if revIdx := strings.Index(raw, " (revision"); revIdx >= 0 {
				raw = raw[:revIdx]
			}
			chipType = raw
		}
		if idx := strings.Index(line, "MAC: "); idx >= 0 {
			macAddr = strings.TrimSpace(line[idx+5:])
		}
	}
	return
}

// ScanDevicesCmd is a tea.Cmd that scans ports and returns DevicesScannedMsg.
func ScanDevicesCmd() tea.Cmd {
	return func() tea.Msg {
		ports := scanPorts()
		devices := make([]Device, 0, len(ports))
		for _, port := range ports {
			chipType, macAddr := probeChip(port)
			devices = append(devices, Device{
				Port:     port,
				ChipType: chipType,
				MAC:      macAddr,
			})
		}
		return DevicesScannedMsg(devices)
	}
}

// TickCmd schedules the next auto-scan.
func TickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(timestamp time.Time) tea.Msg {
		return TickMsg{
			Timestamp: timestamp,
		}
	})
}
