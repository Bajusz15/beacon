package util

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadEnvFileMap reads KEY=value lines from path, expanding values using baseEnv
// plus keys already parsed earlier in the same file. It returns only values
// defined by the file, not the full base environment.
func ReadEnvFileMap(path string, baseEnv map[string]string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer DeferClose(file, "env file")()

	values := make(map[string]string, len(baseEnv))
	for key, value := range baseEnv {
		values[key] = value
	}
	out := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Allow "export KEY=value" lines, common in env files meant to be
		// sourced by a shell. Without this the key would parse as "export KEY".
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		// Quoting controls expansion, matching common .env tooling:
		//   - single-quoted values are literal (no variable expansion), the
		//     escape hatch for secrets containing a literal '$';
		//   - double-quoted and unquoted values expand ${VAR}/$VAR references.
		expand := true
		if len(value) >= 2 {
			if value[0] == '\'' && value[len(value)-1] == '\'' {
				value = value[1 : len(value)-1]
				expand = false
			} else if value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}
		}
		if expand {
			value = os.Expand(value, func(key string) string {
				return values[key]
			})
		}
		values[key] = value
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return out, nil
}

func EnvSliceToMap(env []string) map[string]string {
	values := map[string]string{}
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	return values
}
