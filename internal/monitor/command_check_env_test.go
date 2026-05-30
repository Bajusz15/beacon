package monitor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A command check for a project must run with the project's deploy environment
// (secure env file) so things like `docker compose ps` can interpolate required
// variables. Without this the check fails even though the deploy succeeds.
func TestCommandCheckEnv_InjectsSecureEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BEACON_HOME", home)

	projectDir := filepath.Join(home, "config", "projects", "beaconinfra")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	secureEnv := filepath.Join(home, "secure.env")
	require.NoError(t, os.WriteFile(secureEnv,
		[]byte("DATABASE_URL=postgres://u:p@db/app\nJWT_SECRET=s3cret\n"), 0o600))

	localPath := filepath.Join(home, "proj")
	require.NoError(t, os.MkdirAll(localPath, 0o755))

	envFile := filepath.Join(projectDir, "env")
	require.NoError(t, os.WriteFile(envFile,
		[]byte("BEACON_SECURE_ENV_PATH="+secureEnv+"\nBEACON_LOCAL_PATH="+localPath+"\n"), 0o600))

	m := &Monitor{config: &Config{}, configPath: filepath.Join(projectDir, "monitor.yml")}

	env, dir := m.commandCheckEnv()
	require.NotNil(t, env)
	require.Equal(t, localPath, dir)
	require.Contains(t, env, "DATABASE_URL=postgres://u:p@db/app")
	require.Contains(t, env, "JWT_SECRET=s3cret")
}

// With no project env file there is nothing project-specific to inject, so the
// check falls back to the inherited environment (nil env, empty dir).
func TestCommandCheckEnv_NoProjectContextFallsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BEACON_HOME", home)

	projectDir := filepath.Join(home, "config", "projects", "beaconinfra")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	m := &Monitor{config: &Config{}, configPath: filepath.Join(projectDir, "monitor.yml")}

	env, dir := m.commandCheckEnv()
	require.Nil(t, env)
	require.Equal(t, "", dir)
}
