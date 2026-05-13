package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDirName  = "aila"
	configFileName = "config.toml"
)

var errConfigMissingRequiredKey = errors.New("config missing required key")

// Config is the complete user configuration supported in M9.
type Config struct {
	LLM      LLMConfig
	Autonomy AutonomyConfig
}

type LLMConfig struct {
	Model   string
	Utility UtilityLLMConfig
}

type UtilityLLMConfig struct {
	Model string
}

type AutonomyConfig struct {
	Level string
}

// DefaultConfig returns the README default user configuration.
func DefaultConfig() Config {
	return Config{
		LLM: LLMConfig{
			Model: defaultPrimaryModel,
			Utility: UtilityLLMConfig{
				Model: defaultUtilityModel,
			},
		},
		Autonomy: AutonomyConfig{
			Level: defaultAutonomy,
		},
	}
}

func ResolveConfigPath() (string, error) {
	return resolveConfigPath(os.LookupEnv)
}

func resolveConfigPath(lookup func(string) (string, bool)) (string, error) {
	if xdgConfigHome, ok := lookup("XDG_CONFIG_HOME"); ok && xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, configDirName, configFileName), nil
	}
	home, ok := lookup("HOME")
	if !ok || home == "" {
		return "", fmt.Errorf("resolve config path: HOME is unset and XDG_CONFIG_HOME is unset")
	}
	return filepath.Join(home, ".config", configDirName, configFileName), nil
}

func LoadConfig() (Config, string, error) {
	path, err := ResolveConfigPath()
	if err != nil {
		return Config{}, "", err
	}
	config, err := LoadConfigFile(path)
	return config, path, err
}

func LoadConfigFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("read config %s: %w", path, err)
		}
		config := DefaultConfig()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Config{}, fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(defaultConfigFile()), 0o644); err != nil {
			return Config{}, fmt.Errorf("create config %s: %w", path, err)
		}
		return config, nil
	}
	config, err := parseConfig(content)
	if err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	return config, nil
}

func ConfigCommandOutput(all bool) (string, error) {
	config, path, err := LoadConfig()
	if err != nil {
		return "", err
	}
	if all {
		return fmt.Sprintf("path: %s\nllm.model: %s\nllm.utility.model: %s\nautonomy.level: %s\n", path, config.LLM.Model, config.LLM.Utility.Model, config.Autonomy.Level), nil
	}
	return fmt.Sprintf("path: %s\ndeferred: interactive config UI\n", path), nil
}

func defaultConfigFile() string {
	return `[llm]
model = "opencode-go/deepseek-v4-pro:high" # <provider>/<model>[:reasoning]

[llm.utility]
model = "opencode-go/deepseek-v4-flash:max"

[autonomy]
level = "yolo"
`
}

func parseConfig(content []byte) (Config, error) {
	config := Config{}
	found := map[string]bool{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") || len(line) < 3 {
				return Config{}, fmt.Errorf("line %d: malformed section", lineNumber)
			}
			section = strings.TrimSpace(line[1 : len(line)-1])
			switch section {
			case "llm", "llm.utility", "autonomy":
				continue
			default:
				return Config{}, fmt.Errorf("line %d: unsupported section %q", lineNumber, section)
			}
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("line %d: malformed key/value", lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return Config{}, fmt.Errorf("line %d: malformed key/value", lineNumber)
		}
		parsedValue, err := parseQuotedString(value)
		if err != nil {
			return Config{}, fmt.Errorf("line %d: %w", lineNumber, err)
		}

		fullKey := key
		if section != "" {
			fullKey = section + "." + key
		}
		switch fullKey {
		case "llm.model":
			config.LLM.Model = parsedValue
		case "llm.utility.model":
			config.LLM.Utility.Model = parsedValue
		case "autonomy.level":
			config.Autonomy.Level = parsedValue
		default:
			return Config{}, fmt.Errorf("line %d: unsupported key %q", lineNumber, fullKey)
		}
		found[fullKey] = true
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("scan config: %w", err)
	}

	for _, required := range []string{"llm.model", "llm.utility.model", "autonomy.level"} {
		if !found[required] {
			return Config{}, fmt.Errorf("%w %q", errConfigMissingRequiredKey, required)
		}
	}
	return config, nil
}

func stripComment(line string) string {
	inString := false
	escaped := false
	for index, char := range line {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && inString {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if char == '#' && !inString {
			return line[:index]
		}
	}
	return line
}

func parseQuotedString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", fmt.Errorf("value must be a quoted string")
	}
	value = value[1 : len(value)-1]
	if strings.Contains(value, "\"") {
		return "", fmt.Errorf("quoted strings may not contain raw quotes")
	}
	return strings.ReplaceAll(value, `\"`, `"`), nil
}
