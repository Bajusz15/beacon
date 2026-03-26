package main

import (
	"fmt"
	"log"
	"os"

	"beacon/internal/cloud"
	"beacon/internal/config"
	"beacon/internal/identity"

	"github.com/spf13/cobra"
)

func createConfigCommand() *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Print Beacon home, config file path, and resolved identity",
		Run: func(cmd *cobra.Command, args []string) {
			base, err := config.BeaconHomeDir()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("beacon_home: %s\n", base)
			if v := os.Getenv("BEACON_HOME"); v != "" {
				fmt.Printf("BEACON_HOME: %s\n", v)
			}
			p, err := identity.UserConfigPath()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("config_file: %s\n", p)
			uc, err := identity.LoadUserConfig()
			if err != nil {
				log.Printf("error loading config: %v", err)
				return
			}
			if uc == nil {
				fmt.Println("config_loaded: (none)")
				fmt.Printf("cloud_api_base_default: %s\n", cloud.BeaconInfraAPIBase())
				return
			}
			deviceName := uc.DeviceName
			if deviceName == "" {
				deviceName, _ = os.Hostname()
			}
			fmt.Printf("device_name: %s\n", deviceName)
			fmt.Printf("cloud_api_base_default: %s\n", cloud.BeaconInfraAPIBase())
			fmt.Printf("cloud_api_base_effective: %s\n", uc.EffectiveCloudAPIBase())
			fmt.Printf("cloud_reporting_enabled: %v\n", uc.CloudReportingEnabled)
			if uc.APIKey != "" {
				fmt.Println("api_key: (set)")
			} else {
				fmt.Println("api_key: (not set)")
			}
			fmt.Printf("projects: %d\n", len(uc.Projects))
		},
	}
	root := &cobra.Command{
		Use:   "config",
		Short: "Inspect Beacon paths and identity on disk",
	}
	root.AddCommand(showCmd)
	return root
}
