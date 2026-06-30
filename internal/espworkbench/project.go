package espworkbench

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ProjectContext struct {
	Path string
	Name string

	// true when CMakeLists.txt + main/ both present
	IsValid   bool
	HasCMake  bool
	CMakePath string
	HasMain   bool

	// sdkconfig CONFIG_IDF_TARGET takes precedence over CMakeLists
	Target      string
	Description string

	// sdkconfig present → project has been configured
	HasSDKConfig bool

	// sdkconfig.defaults present
	HasSDKDefaults bool
	HasComponents  bool
	ComponentCount int

	// filename of custom partition CSV, empty if none
	PartitionTable string

	// from project() VERSION or sdkconfig CONFIG_APP_PROJECT_VER
	ProjectVersion string
}

func LoadProjectContext(projectPath string) ProjectContext {
	project := ProjectContext{
		Path:   projectPath,
		Name:   filepath.Base(projectPath),
		Target: "esp32",
	}

	cmakePath := filepath.Join(projectPath, "CMakeLists.txt")
	if _, err := os.Stat(cmakePath); err == nil {
		project.HasCMake = true
		project.CMakePath = cmakePath
		project.Description = parseDescriptionFromCMake(cmakePath)
		project.ProjectVersion = parseVersionFromCMake(cmakePath)
		project.Target = parseTargetFromCMake(cmakePath)
	}

	if info, err := os.Stat(filepath.Join(projectPath, "main")); err == nil && info.IsDir() {
		project.HasMain = true
	}

	project.IsValid = project.HasCMake && project.HasMain

	// sdkconfig gives us more reliable target + version once the project has been configured
	sdkconfigPath := filepath.Join(projectPath, "sdkconfig")
	if _, err := os.Stat(sdkconfigPath); err == nil {
		project.HasSDKConfig = true
		if target := parseTargetFromSDKConfig(sdkconfigPath); target != "" {
			project.Target = target
		}
		if ver := parseVersionFromSDKConfig(sdkconfigPath); ver != "" {
			project.ProjectVersion = ver
		}
	}

	if _, err := os.Stat(filepath.Join(projectPath, "sdkconfig.defaults")); err == nil {
		project.HasSDKDefaults = true
	}

	if info, err := os.Stat(filepath.Join(projectPath, "components")); err == nil && info.IsDir() {
		count := countComponents(filepath.Join(projectPath, "components"))
		if count > 0 {
			project.HasComponents = true
			project.ComponentCount = count
		}
	}

	project.PartitionTable = findPartitionTable(projectPath)

	return project
}

// Returns a short human-readable status string for the project.
func (p ProjectContext) ValidityLabel() string {
	if p.IsValid {
		return "valid esp-idf project"
	}
	var missing []string
	if !p.HasCMake {
		missing = append(missing, "CMakeLists.txt")
	}
	if !p.HasMain {
		missing = append(missing, "main/")
	}
	if len(missing) == 0 {
		return "no esp-idf project"
	}
	return fmt.Sprintf("missing %s", strings.Join(missing, ", "))
}


func parseTargetFromSDKConfig(sdkconfigPath string) string {
	file, err := os.Open(sdkconfigPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "CONFIG_IDF_TARGET=") {
			raw := strings.Trim(strings.TrimPrefix(line, "CONFIG_IDF_TARGET="), `"`)
			return normalizeTarget(raw)
		}
	}
	return ""
}

func parseVersionFromSDKConfig(sdkconfigPath string) string {
	file, err := os.Open(sdkconfigPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "CONFIG_APP_PROJECT_VER=") {
			return strings.Trim(strings.TrimPrefix(line, "CONFIG_APP_PROJECT_VER="), `"`)
		}
	}
	return ""
}

// Maps sdkconfig raw values ("esp32s3") to hyphenated form ("esp32-s3").
func normalizeTarget(raw string) string {
	replacements := map[string]string{
		"esp32s3": "esp32-s3",
		"esp32c3": "esp32-c3",
		"esp32c6": "esp32-c6",
		"esp32h2": "esp32-h2",
		"esp32p4": "esp32-p4",
	}
	if normalized, ok := replacements[raw]; ok {
		return normalized
	}
	return raw
}


var versionRegex = regexp.MustCompile(`(?i)project\s*\(\s*\w+\s+VERSION\s+([\d.]+)`)

func parseVersionFromCMake(cmakePath string) string {
	content, err := os.ReadFile(cmakePath)
	if err != nil {
		return ""
	}
	if matches := versionRegex.FindSubmatch(content); len(matches) >= 2 {
		return string(matches[1])
	}
	return ""
}

func parseTargetFromCMake(cmakePath string) string {
	content, err := os.ReadFile(cmakePath)
	if err != nil {
		return "esp32"
	}
	// look for explicit set(IDF_TARGET ...) calls, which are the authoritative way
	// to declare target in CMakeLists before project()
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(line, "set(idf_target") || strings.Contains(line, "set(idf_target,") {
			for _, chip := range []string{"esp32-s3", "esp32-c3", "esp32-c6", "esp32-h2", "esp32-p4", "esp32s3", "esp32c3", "esp32c6", "esp32h2", "esp32p4"} {
				if strings.Contains(line, chip) {
					return normalizeTarget(chip)
				}
			}
		}
	}
	return "esp32"
}

func parseDescriptionFromCMake(cmakePath string) string {
	content, err := os.ReadFile(cmakePath)
	if err != nil {
		return ""
	}
	for i, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			text := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if text != "" && i < 5 {
				return text
			}
		}
	}
	return ""
}

// Counts subdirectories inside components/ that contain a CMakeLists.txt,
// which is the standard indicator of a real ESP-IDF component.
func countComponents(componentsPath string) int {
	entries, err := os.ReadDir(componentsPath)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cmakePath := filepath.Join(componentsPath, entry.Name(), "CMakeLists.txt")
		if _, err := os.Stat(cmakePath); err == nil {
			count++
		}
	}
	return count
}

// Looks for a CSV file in the project root that names a custom partition table.
func findPartitionTable(projectPath string) string {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".csv") && strings.Contains(strings.ToLower(name), "partition") {
			return name
		}
	}
	return ""
}
