package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestartCommandDefaultsToMaster(t *testing.T) {
	var restarted []string
	old := restartBeaconService
	restartBeaconService = func(service string) error {
		restarted = append(restarted, service)
		return nil
	}
	t.Cleanup(func() { restartBeaconService = old })

	cmd := restartCmd
	cmd.SetArgs(nil)
	require.NoError(t, cmd.Execute())
	require.Equal(t, []string{"master"}, restarted)
}

func TestRestartCommandExplicitServices(t *testing.T) {
	for _, service := range []string{"master", "start", "deploy", "monitor"} {
		t.Run(service, func(t *testing.T) {
			var restarted []string
			old := restartBeaconService
			restartBeaconService = func(service string) error {
				restarted = append(restarted, service)
				return nil
			}
			t.Cleanup(func() { restartBeaconService = old })

			cmd := restartCmd
			cmd.SetArgs([]string{service})
			require.NoError(t, cmd.Execute())

			want := service
			if want == "start" {
				want = "master"
			}
			require.Equal(t, []string{want}, restarted)
		})
	}
}

func TestRestartCommandRejectsUnknownService(t *testing.T) {
	cmd := restartCmd
	cmd.SetArgs([]string{"unknown"})
	require.Error(t, cmd.Execute())
}
