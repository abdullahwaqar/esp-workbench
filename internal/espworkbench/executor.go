package espworkbench

import (
	"bufio"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func classifyLog(line string) LogLevel {
	lowerLine := strings.ToLower(line)
	switch {
	case strings.Contains(lowerLine, "error") ||
		strings.Contains(lowerLine, "failed") ||
		strings.Contains(lowerLine, "fatal"):
		return LogLevelError
	case strings.Contains(lowerLine, "warning") ||
		strings.Contains(lowerLine, "warn"):
		return LogLevelWarn
	case strings.Contains(lowerLine, "done") ||
		strings.Contains(lowerLine, "success") ||
		strings.Contains(lowerLine, "complete") ||
		strings.Contains(lowerLine, "written") ||
		strings.Contains(lowerLine, "verified") ||
		strings.Contains(lowerLine, "leaving"):
		return LogLevelSuccess
	default:
		return LogLevelInfo
	}
}

func WaitForLog(channel chan LogLine) tea.Cmd {
	return func() tea.Msg {
		logLine := <-channel
		return LogMsg(logLine)
	}
}

func RunIDFCommand(projectPath string, args []string, logChannel chan LogLine) tea.Cmd {
	return func() tea.Msg {
		logChannel <- LogLine{
			Text:  fmt.Sprintf("$ idf.py %s", strings.Join(args, " ")),
			Level: LogLevelSystem,
		}

		portToCheck := ""
		for index, argument := range args {
			if (argument == "-p" || argument == "--port") && index+1 < len(args) {
				portToCheck = args[index+1]
				break
			}
		}

		if portToCheck != "" {
			if err := CheckSerialAccess(portToCheck); err != nil {
				logChannel <- LogLine{
					Text:  err.Error(),
					Level: LogLevelError,
				}

				logChannel <- LogLine{
					Text:  "",
					Level: LogLevelSystem,
				}
				logChannel <- LogLine{
					Text:  "attempting automatic permission fix...",
					Level: LogLevelSystem,
				}

				if fixErr := AttemptTemporaryFix(portToCheck); fixErr == nil {
					logChannel <- LogLine{
						Text:  "permissions fixed. retrying...",
						Level: LogLevelSuccess,
					}

					if reCheckErr := CheckSerialAccess(portToCheck); reCheckErr != nil {
						logChannel <- LogLine{
							Text:  reCheckErr.Error(),
							Level: LogLevelError,
						}
						return OperationDoneMsg{Err: reCheckErr}
					}
				} else {
					logChannel <- LogLine{
						Text:  "automatic fix failed. run the command shown above manually.",
						Level: LogLevelWarn,
					}
					return OperationDoneMsg{Err: err}
				}
			}
		}

		cmd := Toolchain().Command("idf.py", args...)
		cmd.Dir = projectPath
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
			logChannel <- LogLine{Text: "failed to start idf.py: " + err.Error(), Level: LogLevelError}
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
			logChannel <- LogLine{
				Text:  fmt.Sprintf("exited with error: %v", err),
				Level: LogLevelError,
			}
		} else {
			logChannel <- LogLine{
				Text:  "operation completed",
				Level: LogLevelSuccess,
			}
		}
		return OperationDoneMsg{Err: err}
	}
}
