package espworkbench

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"slices"
	"strings"
)

type PermissionError struct {
	Port         string
	IsReadable   bool
	InDialout    bool
	CurrentUser  string
	DevicePerms  string
	ErrorMessage string
}

func CheckSerialAccess(port string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	if _, err := os.Stat(port); err != nil {
		return fmt.Errorf("device %s not found", port)
	}

	fileHandle, err := os.OpenFile(port, os.O_RDWR, 0)
	if err == nil {
		fileHandle.Close()
		return nil
	}

	currentUser, _ := user.Current()

	groupOutput, _ := exec.Command("id", "-nG").Output()
	groups := strings.Fields(string(groupOutput))

	inDialoutGroup := slices.Contains(groups, "dialout")

	permOutput, _ := exec.Command("ls", "-l", port).CombinedOutput()

	permErr := &PermissionError{
		Port:        port,
		IsReadable:  false,
		InDialout:   inDialoutGroup,
		CurrentUser: currentUser.Username,
		DevicePerms: string(permOutput),
	}

	lines := []string{
		"",
		"──────────────────────────────────────────────────────────────",
		"permission denied on serial device",
		"──────────────────────────────────────────────────────────────",
		"",
		fmt.Sprintf("device  : %s", port),
		fmt.Sprintf("user    : %s", currentUser.Username),
		fmt.Sprintf("status  : cannot read %s", port),
		"",
	}

	if !inDialoutGroup {
		lines = append(lines,
			"fix: add user to dialout group",
			"",
			"run once:",
			fmt.Sprintf("  $ sudo usermod -aG dialout %s", currentUser.Username),
			"",
			"then log out and log back in (or reboot).",
			"",
		)
	} else {
		lines = append(lines,
			"fix: reset device permissions (temporary)",
			"",
			"run:",
			fmt.Sprintf("  $ sudo chmod a+rw %s", port),
			"",
		)
	}

	lines = append(lines,
		"permissions:",
		strings.TrimSpace(string(permOutput)),
		"",
		"──────────────────────────────────────────────────────────────",
	)

	permErr.ErrorMessage = strings.Join(lines, "\n")
	return permErr
}

func AttemptTemporaryFix(port string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	cmd := exec.Command("pkexec", "chmod", "a+rw", port)
	err := cmd.Run()
	if err == nil {
		return nil
	}

	cmd = exec.Command("sudo", "chmod", "a+rw", port)
	err = cmd.Run()
	if err == nil {
		return nil
	}

	return fmt.Errorf("automatic fix failed; run: sudo chmod a+rw %s", port)
}

func (pe *PermissionError) Error() string {
	return pe.ErrorMessage
}
