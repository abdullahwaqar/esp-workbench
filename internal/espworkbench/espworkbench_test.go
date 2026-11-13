package espworkbench

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClassifyLog(t *testing.T) {
	cases := []struct {
		line string
		want LogLevel
	}{
		{"E (1234) wifi: connect failed", LogLevelError},
		{"\x1b[0;31mE (1234) wifi: boom\x1b[0m", LogLevelError},
		{"W (55) phy: calibration data missing", LogLevelWarn},
		// level letter wins over keywords in the message
		{"I (99) app: no errors, done", LogLevelInfo},
		{"D (1) tag: failed? no", LogLevelInfo},
		{"main.c:12:5: error: expected ';'", LogLevelError},
		{"A fatal error occurred: Failed to connect", LogLevelError},
		{"ninja: build stopped: subcommand failed.", LogLevelError},
		// -Werror is a flag, not an error
		{"cc -Werror -Wall -o main.o", LogLevelInfo},
		{"warning: unused variable 'x'", LogLevelWarn},
		{"Hash of data verified.", LogLevelSuccess},
		{"Leaving...", LogLevelSuccess},
		{"Project build complete.", LogLevelSuccess},
		{"[12/900] Building C object esp-idf/main/main.c.obj", LogLevelInfo},
		// "abandoned" contains "done" but isn't the word
		{"abandoned buffer", LogLevelInfo},
	}
	for _, c := range cases {
		if got := classifyLog(c.line); got != c.want {
			t.Errorf("classifyLog(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func scanAll(t *testing.T, input string) []string {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(scanLinesOrCR)
	var out []string
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestScanLinesOrCR(t *testing.T) {
	cases := map[string][]string{
		"a\nb\n":                             {"a", "b"},
		"a\r\nb\r\n":                         {"a", "b"},
		"a\n\nb":                             {"a", "", "b"},
		"no newline":                         {"no newline"},
		"start\n\r10 %\r50 %\r100 %\ndone\n": {"start", "10 %", "50 %", "100 %", "done"},
		"x\ry":                               {"x", "y"},
		"trailing\r":                         {"trailing"},
		"":                                   nil,
	}
	for in, want := range cases {
		if got := scanAll(t, in); !reflect.DeepEqual(got, want) {
			t.Errorf("split %q = %q, want %q", in, got, want)
		}
	}
}

// A CR split across two reads must still be treated as one CRLF.
func TestScanLinesOrCRAcrossReads(t *testing.T) {
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("one\r"))
		time.Sleep(10 * time.Millisecond)
		pw.Write([]byte("\ntwo\n"))
		pw.Close()
	}()
	scanner := bufio.NewScanner(pr)
	scanner.Split(scanLinesOrCR)
	var got []string
	for scanner.Scan() {
		got = append(got, scanner.Text())
	}
	if !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("got %q", got)
	}
}

// Before: bufio.Scanner's default 64 KB limit stopped reading on one long
// line, nothing drained the pipe, and idf.py blocked forever writing to it,
// leaving the UI stuck in "building".
func TestStreamOutputSurvivesHugeLine(t *testing.T) {
	pr, pw := io.Pipe()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		pw.Write([]byte("before\n"))
		pw.Write([]byte(strings.Repeat("x", maxLogLineBytes+10) + "\n"))
		pw.Write([]byte("after\n"))
		pw.Close()
	}()

	logs := make(chan LogLine, 16)
	streamDone := make(chan struct{})
	go func() {
		streamOutput(pr, logs)
		close(streamDone)
	}()

	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("writer blocked: the stream stopped draining after a long line")
	}
	<-streamDone
	close(logs)

	var lines []LogLine
	for l := range logs {
		lines = append(lines, l)
	}
	if len(lines) < 2 || lines[0].Text != "before" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
	if last := lines[len(lines)-1]; last.Level != LogLevelWarn || !strings.Contains(last.Text, "hidden") {
		t.Errorf("expected a warning about the hidden output, got %+v", last)
	}
}

func TestStreamOutputClassifies(t *testing.T) {
	logs := make(chan LogLine, 8)
	streamOutput(strings.NewReader("E (1) x: bad\nfine\n"), logs)
	close(logs)
	var got []LogLevel
	for l := range logs {
		got = append(got, l.Level)
	}
	if !reflect.DeepEqual(got, []LogLevel{LogLevelError, LogLevelInfo}) {
		t.Errorf("levels = %v", got)
	}
}

func TestPortFromArgs(t *testing.T) {
	cases := map[string][]string{
		"/dev/ttyUSB0": {"-p", "/dev/ttyUSB0", "flash"},
		"COM3":         {"build", "--port", "COM3"},
		"":             {"build"},
	}
	for want, args := range cases {
		if got := portFromArgs(args); got != want {
			t.Errorf("portFromArgs(%v) = %q, want %q", args, got, want)
		}
	}
	if got := portFromArgs([]string{"flash", "-p"}); got != "" {
		t.Errorf("dangling -p should give no port, got %q", got)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGuessFlashAddr(t *testing.T) {
	dir := t.TempDir()

	// a large ordinary app image: the old size heuristic sent this to 0x0,
	// overwriting the bootloader and partition table
	bigApp := filepath.Join(dir, "my_app.bin")
	writeFile(t, bigApp, make([]byte, 900*1024))

	merged := make([]byte, 0x9000)
	merged[0x8000], merged[0x8001] = partitionMagicLow, partitionMagicHigh
	mergedPath := filepath.Join(dir, "anything.bin")
	writeFile(t, mergedPath, merged)

	small := filepath.Join(dir, "tiny.bin")
	writeFile(t, small, []byte{1, 2, 3})

	cases := []struct {
		path, name, target, want string
	}{
		{bigApp, "my_app.bin", "esp32", "0x10000"},
		{mergedPath, "anything.bin", "esp32-s3", "0x0"},
		{small, "tiny.bin", "esp32", "0x10000"},
		{"", "bootloader.bin", "esp32", "0x1000"},
		{"", "bootloader.bin", "esp32-s2", "0x1000"},
		{"", "bootloader.bin", "esp32s2", "0x1000"},
		{"", "bootloader.bin", "esp32-s3", "0x0"},
		{"", "bootloader.bin", "esp32-c3", "0x0"},
		{"", "bootloader.bin", "esp32-c6", "0x0"},
		{"", "bootloader.bin", "esp32-h2", "0x0"},
		{"", "bootloader.bin", "esp32-p4", "0x2000"},
		{"", "bootloader.bin", "esp32-c5", "0x2000"},
		{"", "bootloader.bin", "", "0x1000"},
		{"", "partition-table.bin", "esp32-s3", "0x8000"},
		{"", "ota_data_initial.bin", "esp32", "0xd000"},
		{"/does/not/exist.bin", "app.bin", "esp32", "0x10000"},
	}
	for _, c := range cases {
		if got := GuessFlashAddr(c.path, c.name, c.target); got != c.want {
			t.Errorf("GuessFlashAddr(%q, %q) = %s, want %s", c.name, c.target, got, c.want)
		}
	}
}

func partitionEntry(name string, ptype, subtype byte, offset, size uint32) []byte {
	b := make([]byte, partitionEntrySize)
	b[0], b[1], b[2], b[3] = partitionMagicLow, partitionMagicHigh, ptype, subtype
	binary.LittleEndian.PutUint32(b[4:8], offset)
	binary.LittleEndian.PutUint32(b[8:12], size)
	copy(b[12:28], name)
	return b
}

func TestParsePartitionTable(t *testing.T) {
	var data []byte
	// deliberately out of order; the parser sorts by offset
	data = append(data, partitionEntry("factory", 0x00, 0x00, 0x10000, 0x100000)...)
	data = append(data, partitionEntry("nvs", 0x01, 0x02, 0x9000, 0x6000)...)
	data = append(data, partitionEntry("ota_1", 0x00, 0x11, 0x210000, 0x100000)...)
	data = append(data, partitionEntry("storage", 0x01, 0x82, 0x310000, 0xf0000)...)
	// MD5 record and erased flash end the table
	md5 := make([]byte, partitionEntrySize)
	md5[0], md5[1] = 0xEB, 0xEB
	data = append(data, md5...)
	data = append(data, []byte(strings.Repeat("\xff", 64))...)

	entries, err := parsePartitionTable(data)
	if err != nil {
		t.Fatal(err)
	}
	var names, subs []string
	for _, e := range entries {
		names = append(names, e.Name)
		subs = append(subs, e.TypeLabel+"/"+e.SubLabel)
	}
	if !reflect.DeepEqual(names, []string{"nvs", "factory", "ota_1", "storage"}) {
		t.Errorf("names = %v", names)
	}
	if !reflect.DeepEqual(subs, []string{"data/nvs", "app/factory", "app/ota_1", "data/spiffs"}) {
		t.Errorf("types = %v", subs)
	}

	if _, err := parsePartitionTable(make([]byte, partitionTableSize)); err == nil {
		t.Error("an all-zero table should be an error")
	}
	if _, err := parsePartitionTable([]byte(strings.Repeat("\xff", partitionTableSize))); err == nil || !strings.Contains(err.Error(), "erased") {
		t.Errorf("erased flash should say so, got %v", err)
	}
	if _, err := parsePartitionTable(nil); err == nil {
		t.Error("empty input should be an error")
	}
}

func TestSubTypeLabels(t *testing.T) {
	cases := []struct {
		ptype, sub byte
		want       string
	}{
		{0x00, 0x00, "factory"}, {0x00, 0x10, "ota_0"}, {0x00, 0x1f, "ota_15"}, {0x00, 0x20, "test"},
		{0x01, 0x00, "ota_data"}, {0x01, 0x01, "phy"}, {0x01, 0x03, "coredump"}, {0x01, 0x81, "fat"},
		{0x01, 0x99, "0x99"}, {0x40, 0x01, "0x01"},
	}
	for _, c := range cases {
		if got := subTypeLabel(c.ptype, c.sub); got != c.want {
			t.Errorf("subTypeLabel(%#x, %#x) = %q, want %q", c.ptype, c.sub, got, c.want)
		}
	}
	if typeLabel(0x40) != "0x40" || typeLabel(0) != "app" || typeLabel(1) != "data" {
		t.Error("typeLabel mismatch")
	}
}

func TestFlashSizeRegex(t *testing.T) {
	m := flashSizeRegex.FindStringSubmatch("Manufacturer: c8\nDetected flash size: 16MB\n")
	if len(m) != 3 || m[1] != "16" || m[2] != "MB" {
		t.Errorf("match = %v", m)
	}
	if flashSizeRegex.MatchString("Detected flash size: Unknown") {
		t.Error("unknown size should not match")
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\nb\nc\nd\n", 2); got != "c\nd" {
		t.Errorf("lastLines = %q", got)
	}
	if got := lastLines("a\nb", 5); got != "a\nb" {
		t.Errorf("short input changed: %q", got)
	}
}

func TestParseFlasherArgs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "flasher_args.json"), []byte(`{
		"write_flash_args": ["--flash_mode", "dio", "--flash_size", "4MB"],
		"flash_files": {"0x10000": "app.bin", "0x1000": "bootloader/bootloader.bin", "0x8000": "partition_table/partition-table.bin"}
	}`))
	opt, err := BuildFullFlashOption(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"write_flash", "--flash_mode", "dio", "--flash_size", "4MB",
		"0x1000", filepath.Join(dir, "bootloader/bootloader.bin"),
		"0x8000", filepath.Join(dir, "partition_table/partition-table.bin"),
		"0x10000", filepath.Join(dir, "app.bin"),
	}
	if !reflect.DeepEqual(opt.EspArgs, want) {
		t.Errorf("args = %v\nwant   %v", opt.EspArgs, want)
	}
	if opt.FileCount != 3 || !opt.IsFullFlash {
		t.Errorf("option = %+v", opt)
	}

	writeFile(t, filepath.Join(dir, "flasher_args.json"), []byte("{not json"))
	if _, err := BuildFullFlashOption(dir); err == nil {
		t.Error("bad json should be an error")
	}
	if _, err := BuildFullFlashOption(t.TempDir()); err == nil {
		t.Error("missing file should be an error")
	}
}

func TestBrowseDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "zeta.bin"), make([]byte, 2048))
	writeFile(t, filepath.Join(dir, "ALPHA.BIN"), []byte{1})
	writeFile(t, filepath.Join(dir, "notes.txt"), []byte("x"))
	writeFile(t, filepath.Join(dir, ".hidden.bin"), []byte("x"))
	writeFile(t, filepath.Join(dir, "flasher_args.json"), []byte("{}"))
	os.Mkdir(filepath.Join(dir, "bootloader"), 0o755)
	os.Mkdir(filepath.Join(dir, ".git"), 0o755)

	items, err := browseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range items {
		names = append(names, it.Name)
	}
	want := []string{"..", "[full flash]", "bootloader", "ALPHA.BIN", "zeta.bin"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("entries = %v, want %v", names, want)
	}
	if items[4].SizeLabel != "2 KB" {
		t.Errorf("size label = %q", items[4].SizeLabel)
	}

	if _, err := browseDir(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing dir should be an error")
	}
	if root, _ := browseDir("/"); len(root) > 0 && root[0].Name == ".." {
		t.Error("filesystem root should have no .. entry")
	}
}

func TestStartBrowserPicksBuildDir(t *testing.T) {
	dir := t.TempDir()
	msg := StartBrowserCmd(dir)().(DirLoadedMsg)
	if msg.Path != dir {
		t.Errorf("no build dir: path = %s", msg.Path)
	}
	os.Mkdir(filepath.Join(dir, "build"), 0o755)
	msg = StartBrowserCmd(dir)().(DirLoadedMsg)
	if msg.Path != filepath.Join(dir, "build") {
		t.Errorf("with build dir: path = %s", msg.Path)
	}
}

func TestFormatFileSize(t *testing.T) {
	cases := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1 KB", 1536 * 1024: "1.5 MB"}
	for in, want := range cases {
		if got := formatFileSize(in); got != want {
			t.Errorf("formatFileSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeTarget(t *testing.T) {
	cases := map[string]string{
		"esp32": "esp32", "esp32s2": "esp32-s2", "esp32s3": "esp32-s3", "esp32c2": "esp32-c2",
		"esp32c5": "esp32-c5", "esp32c61": "esp32-c61", "esp32h2": "esp32-h2", "esp32p4": "esp32-p4",
		"esp32-s3": "esp32-s3", " ESP32S3 ": "esp32-s3", "linux": "linux",
	}
	for in, want := range cases {
		if got := normalizeTarget(in); got != want {
			t.Errorf("normalizeTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func makeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if strings.HasSuffix(name, "/") {
			os.MkdirAll(filepath.Join(dir, name), 0o755)
			continue
		}
		writeFile(t, filepath.Join(dir, name), []byte(content))
	}
	return dir
}

func TestLoadProjectContext(t *testing.T) {
	dir := makeProject(t, map[string]string{
		"CMakeLists.txt":                   "# Blink example\ncmake_minimum_required(VERSION 3.16)\nset(IDF_TARGET esp32c6)\nproject(blink VERSION 2.1.0)\n",
		"main/":                            "",
		"sdkconfig.defaults":               "",
		"components/led/CMakeLists.txt":    "",
		"components/button/CMakeLists.txt": "",
		"components/empty/":                "",
		"Partitions_Custom.CSV":            "",
	})
	p := LoadProjectContext(dir)
	if !p.IsValid || p.ValidityLabel() != "valid esp-idf project" {
		t.Errorf("should be valid: %+v", p)
	}
	if p.Target != "esp32-c6" {
		t.Errorf("target = %q, want esp32-c6 from set(IDF_TARGET)", p.Target)
	}
	if p.ProjectVersion != "2.1.0" || p.Description != "Blink example" {
		t.Errorf("version/description = %q / %q", p.ProjectVersion, p.Description)
	}
	if p.HasSDKConfig || !p.HasSDKDefaults {
		t.Error("sdkconfig flags wrong")
	}
	if p.ComponentCount != 2 || !p.HasComponents {
		t.Errorf("components = %d", p.ComponentCount)
	}
	if p.PartitionTable != "Partitions_Custom.CSV" {
		t.Errorf("partition csv = %q", p.PartitionTable)
	}

	// sdkconfig overrides CMake for target and version
	writeFile(t, filepath.Join(dir, "sdkconfig"), []byte("CONFIG_IDF_TARGET=\"esp32s3\"\nCONFIG_APP_PROJECT_VER=\"3.0.0\"\n"))
	p = LoadProjectContext(dir)
	if p.Target != "esp32-s3" || p.ProjectVersion != "3.0.0" || !p.HasSDKConfig {
		t.Errorf("sdkconfig not applied: target=%q version=%q", p.Target, p.ProjectVersion)
	}
}

func TestProjectTargetFromCMakeVariants(t *testing.T) {
	cases := map[string]string{
		`set(IDF_TARGET "esp32s2")`:  "esp32-s2",
		`SET( IDF_TARGET esp32-h2 )`: "esp32-h2",
		`set(IDF_TARGET esp32)`:      "esp32",
		`# no target`:                "esp32",
	}
	for line, want := range cases {
		dir := makeProject(t, map[string]string{"CMakeLists.txt": line + "\nproject(x)\n"})
		if got := parseTargetFromCMake(filepath.Join(dir, "CMakeLists.txt")); got != want {
			t.Errorf("%s -> %q, want %q", line, got, want)
		}
	}
}

func TestValidityLabel(t *testing.T) {
	cases := map[string]ProjectContext{
		"missing CMakeLists.txt, main/": {},
		"missing main/":                 {HasCMake: true},
		"missing CMakeLists.txt":        {HasMain: true},
		"no esp-idf project":            {HasCMake: true, HasMain: true},
	}
	for want, p := range cases {
		if got := p.ValidityLabel(); got != want {
			t.Errorf("ValidityLabel(%+v) = %q, want %q", p, got, want)
		}
	}
}

func TestShellLineQuoting(t *testing.T) {
	tc := ToolchainEnv{ExportScript: "/home/o'brien/esp-idf/export.sh"}
	cmd := tc.Command("esptool.py", "--port", "/dev/tty USB0", "it's")
	if filepath.Base(cmd.Path) != "bash" || cmd.Args[1] != "-c" {
		t.Fatalf("expected bash -c, got %v", cmd.Args)
	}
	want := `source '/home/o'\''brien/esp-idf/export.sh' > /dev/null 2>&1 && exec 'esptool.py' '--port' '/dev/tty USB0' 'it'\''s'`
	if cmd.Args[2] != want {
		t.Errorf("shell line =\n%s\nwant\n%s", cmd.Args[2], want)
	}

	plain := ToolchainEnv{}.Command("idf.py", "build")
	if filepath.Base(plain.Path) == "bash" {
		t.Error("without an export script the tool should run directly")
	}
}

func TestCheckSerialAccessMissingPort(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission checks only run on linux")
	}
	err := CheckSerialAccess(filepath.Join(t.TempDir(), "ttyUSB9"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing port: %v", err)
	}
	// a readable, writable file passes
	f := filepath.Join(t.TempDir(), "ttyFake")
	writeFile(t, f, nil)
	if err := CheckSerialAccess(f); err != nil {
		t.Errorf("accessible port reported %v", err)
	}
}
