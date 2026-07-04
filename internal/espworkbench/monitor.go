package espworkbench

import (
	"bufio"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/creack/pty"
)

type MonitorStartedMsg struct {
	Pty *os.File
	Err error
}

type MonitorDoneMsg struct{}

func StartMonitorCmd(projectPath, port string, logChannel chan LogLine, done chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if err := CheckSerialAccess(port); err != nil {
			if fixErr := AttemptTemporaryFix(port); fixErr != nil {
				return MonitorStartedMsg{Err: err}
			}
			if recheckErr := CheckSerialAccess(port); recheckErr != nil {
				return MonitorStartedMsg{Err: recheckErr}
			}
		}

		logChannel <- LogLine{
			Text:  fmt.Sprintf("$ idf.py -p %s monitor", port),
			Level: LogLevelSystem,
		}

		cmd := Toolchain().Command("idf.py", "-p", port, "monitor")
		cmd.Dir = projectPath

		ptmx, err := pty.Start(cmd)
		if err != nil {
			return MonitorStartedMsg{Err: fmt.Errorf("failed to start monitor: %w", err)}
		}

		go func() {
			defer close(done)
			scanner := bufio.NewScanner(ptmx)
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				logChannel <- LogLine{Text: line, Level: classifyLog(line)}
			}
			cmd.Wait()
			ptmx.Close()
			logChannel <- LogLine{Text: "monitor stopped", Level: LogLevelSystem}
		}()

		return MonitorStartedMsg{Pty: ptmx}
	}
}

func StopMonitorCmd(ptmx *os.File) tea.Cmd {
	return func() tea.Msg {
		if ptmx != nil {
			ptmx.Write([]byte{0x1d}) // Ctrl+]
		}
		return nil
	}
}

func WaitForMonitorDone(done chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-done
		return MonitorDoneMsg{}
	}
}
