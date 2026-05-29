package deploy

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"beacon/internal/config"
	"beacon/internal/secrets"
	"beacon/internal/util"
)

func CommandEnv(cfg *config.Config, extra ...string) ([]string, error) {
	envMap := envSliceToMap(os.Environ())

	if cfg.SecureEnvPath != "" {
		path := os.ExpandEnv(cfg.SecureEnvPath)
		values, err := readEnvFile(path, envMap)
		if err != nil {
			if os.IsNotExist(err) {
				logger.Infof("Warning: Secure environment file not found: %s\n", path)
			} else {
				return nil, fmt.Errorf("load secure env file %s: %w", path, err)
			}
		} else {
			logger.Infof("Loading secure environment file: %s\n", path)
			for key, value := range values {
				envMap[key] = value
			}
		}
	}

	if err := mergeBeaconSecrets(cfg, envMap); err != nil {
		return nil, err
	}

	for _, item := range extra {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, key+"="+value)
	}
	return env, nil
}

func mergeBeaconSecrets(cfg *config.Config, envMap map[string]string) error {
	if cfg.ProjectName == "" {
		return nil
	}
	projectEnv := cfg.ProjectEnv
	if projectEnv == "" {
		projectEnv = "default"
	}
	store, err := secrets.NewStore()
	if err != nil {
		return err
	}
	exists, err := store.Exists(cfg.ProjectName, projectEnv)
	if err != nil {
		return err
	}
	if cfg.SecretsEnabled != nil {
		if !*cfg.SecretsEnabled {
			return nil
		}
	} else if !exists {
		return nil
	}
	values, err := store.Load(cfg.ProjectName, projectEnv)
	if err != nil {
		return fmt.Errorf("load Beacon secrets for %s/%s: %w", cfg.ProjectName, projectEnv, err)
	}
	for key, value := range values {
		envMap[key] = value
	}
	return nil
}

func readEnvFile(path string, base map[string]string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer util.DeferClose(file, "env file")()

	values := make(map[string]string, len(base))
	for key, value := range base {
		values[key] = value
	}
	out := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		value = os.Expand(value, func(key string) string {
			return values[key]
		})
		values[key] = value
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return out, nil
}

func envSliceToMap(env []string) map[string]string {
	values := map[string]string{}
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	return values
}
