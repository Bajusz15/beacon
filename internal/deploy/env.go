package deploy

import (
	"fmt"
	"os"
	"strings"

	"beacon/internal/config"
	"beacon/internal/secrets"
	"beacon/internal/util"
)

func CommandEnv(cfg *config.Config, extra ...string) ([]string, error) {
	envMap := util.EnvSliceToMap(os.Environ())

	secureEnvPath := os.ExpandEnv(cfg.SecureEnvPath)
	// Older project env files commonly keep the secure env path inline in the
	// deploy command. Use that path when the configured/default path is absent.
	if commandPath := secureEnvPathFromDeployCommand(cfg.DeployCommand); commandPath != "" {
		if secureEnvPath == "" || !pathExists(secureEnvPath) {
			secureEnvPath = commandPath
		}
	}

	if secureEnvPath != "" {
		path := os.ExpandEnv(secureEnvPath)
		values, err := util.ReadEnvFileMap(path, envMap)
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

func secureEnvPathFromDeployCommand(command string) string {
	for _, token := range strings.Fields(command) {
		token = strings.Trim(token, "\"'")
		if strings.HasPrefix(token, "BEACON_SECURE_ENV_PATH=") {
			return strings.Trim(strings.TrimPrefix(token, "BEACON_SECURE_ENV_PATH="), "\"'")
		}
	}
	return ""
}

func pathExists(path string) bool {
	_, err := os.Stat(os.ExpandEnv(path))
	return err == nil
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
	if cfg.SecretsEnabled != nil {
		if !*cfg.SecretsEnabled {
			return nil
		}
	} else {
		exists, err := store.Exists(cfg.ProjectName, projectEnv)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
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
