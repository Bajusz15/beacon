package secrets

import "beacon/internal/util"

func readEnvFile(path string, baseEnv []string) (map[string]string, error) {
	return util.ReadEnvFileMap(path, util.EnvSliceToMap(baseEnv))
}
