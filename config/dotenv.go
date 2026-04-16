package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadDotEnv reads a .env file and sets environment variables for any keys
// that are not already set in the current environment.
// This allows .env to provide defaults while real env vars take precedence.
func loadDotEnv() error {
	// Try multiple locations: executable dir, current dir, home dir
	locations := []string{
		dotEnvPathFromExe(),
		".env",
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		locations = append(locations, filepath.Join(homeDir, ".pinhaoclaw", ".env"))
	}

	for _, loc := range locations {
		if loc == "" {
			continue
		}
		data, err := os.ReadFile(loc)
		if err != nil {
			continue
		}
		if err := parseDotEnv(data); err != nil {
			return err
		}
		return nil // loaded first found
	}
	return nil
}

// dotEnvPathFromExe returns the .env path next to the running executable.
func dotEnvPathFromExe() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), ".env")
}

// parseDotEnv parses .env file content and sets env vars for keys not yet set.
//
// Format:
//
//	KEY=value
//	KEY="quoted value"
//	KEY='single quoted'
//	# comment
//	export KEY=value  # supported
//	blank lines ignored
func parseDotEnv(data []byte) error {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip "export " prefix
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		key, value, ok := parseDotEnvLine(line)
		if !ok {
			continue
		}

		// Only set if not already defined in the real environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

// parseDotEnvLine parses a single KEY=value line.
func parseDotEnvLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, "=")
	if idx < 1 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	if key == "" {
		return "", "", false
	}
	value = line[idx+1:]

	// Trim inline comments (only outside quotes)
	value = trimInlineComment(value)

	// Handle quoted values
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	value = strings.TrimSpace(value)
	return key, value, true
}

// trimInlineComment removes trailing # comments that are not inside quotes.
func trimInlineComment(s string) string {
	inDouble := false
	inSingle := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '#':
			if !inDouble && !inSingle {
				return strings.TrimRight(s[:i], " \t")
			}
		}
	}
	return s
}
