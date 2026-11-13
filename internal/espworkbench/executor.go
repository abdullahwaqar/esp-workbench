package espworkbench

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Longest single line of tool output we keep. Anything longer is reported and
// the rest of the stream is drained so the child process never blocks.
const maxLogLineBytes = 1024 * 1024

var (
	ansiSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

	// ESP-IDF log lines look like "E (1234) tag: message", optionally wrapped
	// in color codes. The level letter is far more reliable than keywords.
	espLogPrefix = regexp.MustCompile(`^\s*([EWIDV]) \(\d+\)`)

	errorWords   = regexp.MustCompile(`\b(error|errors|failed|failure|fatal)\b`)
	warnWords    = regexp.MustCompile(`\b(warning|warnings|warn)\b`)
	successWords = regexp.MustCompile(`\b(done|success|successfully|complete|completed|written|verified|leaving)\b`)
)

func classifyLog(line string) LogLevel {
	plain := ansiSequence.ReplaceAllString(line, "")

	if m := espLogPrefix.FindStringSubmatch(plain); m != nil {
		switch m[1] {
		case "E":
			return LogLevelError
		case "W":
			return LogLevelWarn
		default:
			return LogLevelInfo
		}
	}

	lowerLine := strings.ToLower(plain)
	switch {
	case errorWords.MatchString(lowerLine):
		return LogLevelError
	case warnWords.MatchString(lowerLine):
		return LogLevelWarn
	case successWords.MatchString(lowerLine):
		return LogLevelSuccess
	default:
		return LogLevelInfo
	}
}

// Splits on "\n", "\r\n" or a bare "\r". A bare carriage return is how
// esptool redraws its "Writing at 0x... (42 %)" progress line, so treating it
// as a line break lets progress show up while it happens instead of only
// once the whole flash has finished.
func scanLinesOrCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 >= len(data) && !atEOF {
				// need one more byte to know whether "\n" follows
				return 0, nil, nil
			}
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			if i == 0 {
				// a leading "\r" just starts a redraw; it isn't an empty line
				return 1, nil, nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Reads r line by line into logChannel until EOF. If a single line is longer
// than maxLogLineBytes the remainder is drained and discarded, because
// stopping early would leave the child blocked on a full pipe forever.
func streamOutput(r io.Reader, logChannel chan<- LogLine) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes)
	scanner.Split(scanLinesOrCR)
	for scanner.Scan() {
		line := scanner.Text()
		logChannel <- LogLine{Text: line, Level: classifyLog(line)}
	}
	if errors.Is(scanner.Err(), bufio.ErrTooLong) {
		logChannel <- LogLine{
			Text:  fmt.Sprintf("a line of output was longer than %d KB, so the rest of this output is hidden", maxLogLineBytes/1024),
			Level: LogLevelWarn,
		}
		_, _ = io.Copy(io.Discard, r)
	}
}

// Checks the serial port and, if it isn't accessible, tries the automatic
// permission fix. Every step is reported to logChannel. Returns nil when the
// port is ready to use.
func ensureSerialAccess(port string, logChannel chan<- LogLine) error {
	err := CheckSerialAccess(port)
	if err == nil {
		return nil
	}
	logChannel <- LogLine{Text: err.Error(), Level: LogLevelError}
	logChannel <- LogLine{Text: "", Level: LogLevelSystem}
	logChannel <- LogLine{Text: "attempting automatic permission fix...", Level: LogLevelSystem}

	if fixErr := AttemptTemporaryFix(port); fixErr != nil {
		logChannel <- LogLine{Text: "automatic fix failed. run the command shown above, then try again.", Level: LogLevelWarn}
		return err
	}
	logChannel <- LogLine{Text: "permissions fixed. retrying...", Level: LogLevelSuccess}
	if recheckErr := CheckSerialAccess(port); recheckErr != nil {
		logChannel <- LogLine{Text: recheckErr.Error(), Level: LogLevelError}
		return recheckErr
	}
	return nil
}

// Starts cmd, streams stdout and stderr into logChannel and reports the
// result. Shared by every build/flash/erase operation.
func runStreaming(cmd *exec.Cmd, toolName string, successText string, logChannel chan<- LogLine) OperationDoneMsg {
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
		logChannel <- LogLine{Text: "failed to start " + toolName + ": " + err.Error(), Level: LogLevelError}
		return OperationDoneMsg{Err: err}
	}

	done := make(chan struct{}, 2)
	for _, pipe := range []io.Reader{stdout, stderr} {
		go func(reader io.Reader) {
			defer func() { done <- struct{}{} }()
			streamOutput(reader, logChannel)
		}(pipe)
	}
	<-done
	<-done

	err = cmd.Wait()
	if err != nil {
		logChannel <- LogLine{
			Text:  fmt.Sprintf("%s exited with an error (%v). check the output above for the cause.", toolName, err),
			Level: LogLevelError,
		}
	} else {
		logChannel <- LogLine{Text: successText, Level: LogLevelSuccess}
	}
	return OperationDoneMsg{Err: err}
}

func WaitForLog(channel chan LogLine) tea.Cmd {
	return func() tea.Msg {
		logLine := <-channel
		return LogMsg(logLine)
	}
}

// Returns the value following -p/--port in args, or "".
func portFromArgs(args []string) string {
	for index, argument := range args {
		if (argument == "-p" || argument == "--port") && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func RunIDFCommand(projectPath string, args []string, logChannel chan LogLine) tea.Cmd {
	return func() tea.Msg {
		logChannel <- LogLine{
			Text:  fmt.Sprintf("$ idf.py %s", strings.Join(args, " ")),
			Level: LogLevelSystem,
		}

		if port := portFromArgs(args); port != "" {
			if err := ensureSerialAccess(port, logChannel); err != nil {
				return OperationDoneMsg{Err: err}
			}
		}

		cmd := Toolchain().Command("idf.py", args...)
		cmd.Dir = projectPath
		return runStreaming(cmd, "idf.py", "operation completed", logChannel)
	}
}
