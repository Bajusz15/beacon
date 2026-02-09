package k8sobserver

import (
	"fmt"
	"os"
	"strings"
	"time"

	"beacon/internal/config"

	"github.com/spf13/cobra"
)

const defaultStatusTimeout = 30 * time.Second

// AddSourceCommand returns the cobra command for `beacon source add kubernetes`.
func AddSourceCommand() *cobra.Command {
	var kubeconfig, namespace, project, name string
	var inCluster bool

	cmd := &cobra.Command{
		Use:   "kubernetes [name]",
		Short: "Add a Kubernetes observation source",
		Long:  `Add a read-only Kubernetes source to watch workloads (Deployments, StatefulSets, DaemonSets, Pods). Uses kubeconfig or in-cluster config.`,
		Example: `  beacon source add kubernetes my-k8s --kubeconfig ~/.kube/config --namespace default --project myapp
  beacon source add kubernetes --in-cluster --project myapp`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName := name
			if len(args) > 0 {
				sourceName = args[0]
			}
			if sourceName == "" {
				sourceName = "kubernetes-default"
			}
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			paths, err := config.NewBeaconPaths()
			if err != nil {
				return err
			}
			if err := paths.EnsureDirectories(); err != nil {
				return err
			}
			if err := paths.ValidateProjectName(project); err != nil {
				return err
			}
			if err := paths.CreateProjectStructure(project); err != nil {
				return err
			}
			sourcesPath := paths.GetProjectSourcesFile(project)
			cfg, err := LoadSourcesConfig(sourcesPath)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if cfg == nil {
				cfg = &SourcesConfig{}
			}
			// Update or append
			found := false
			for i := range cfg.Sources {
				if cfg.Sources[i].Name == sourceName {
					cfg.Sources[i].Type = "kubernetes"
					cfg.Sources[i].Enabled = true
					cfg.Sources[i].Kubeconfig = kubeconfig
					cfg.Sources[i].Namespace = namespace
					cfg.Sources[i].InCluster = inCluster
					found = true
					break
				}
			}
			if !found {
				cfg.Sources = append(cfg.Sources, SourceConfig{
					Name:       sourceName,
					Type:       "kubernetes",
					Enabled:    true,
					Kubeconfig: kubeconfig,
					Namespace:  namespace,
					InCluster:  inCluster,
				})
			}
			if err := SaveSourcesConfig(sourcesPath, cfg); err != nil {
				return err
			}
			fmt.Printf("Added Kubernetes source %q to project %s (config: %s)\n", sourceName, project, sourcesPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: KUBECONFIG env or ~/.kube/config)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace to watch (empty = all)")
	cmd.Flags().StringVar(&project, "project", "", "Beacon project name")
	cmd.Flags().StringVar(&name, "name", "", "Source name (optional; can pass as argument)")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "Use in-cluster config (when running inside Kubernetes)")
	return cmd
}

// ListSourcesCommand returns the cobra command for `beacon source list`.
func ListSourcesCommand() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List observation sources",
		Long:  `List configured observation sources for a project or all projects.`,
		Example: `  beacon source list
  beacon source list --project myapp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := config.NewBeaconPaths()
			if err != nil {
				return err
			}
			projects, err := paths.ListProjects()
			if err != nil {
				return err
			}
			if project != "" {
				projects = []string{project}
			}
			for _, proj := range projects {
				sourcesPath := paths.GetProjectSourcesFile(proj)
				cfg, err := LoadSourcesConfig(sourcesPath)
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return err
				}
				for _, s := range cfg.Sources {
					enabled := "disabled"
					if s.Enabled {
						enabled = "enabled"
					}
					fmt.Printf("%s\t%s\t%s\t%s\n", proj, s.Name, s.Type, enabled)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project name (default: list all projects)")
	return cmd
}

// RemoveSourceCommand returns the cobra command for `beacon source remove`.
func RemoveSourceCommand() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an observation source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			paths, err := config.NewBeaconPaths()
			if err != nil {
				return err
			}
			sourcesPath := paths.GetProjectSourcesFile(project)
			cfg, err := LoadSourcesConfig(sourcesPath)
			if err != nil {
				return err
			}
			var kept []SourceConfig
			for _, s := range cfg.Sources {
				if s.Name != name {
					kept = append(kept, s)
				}
			}
			if len(kept) == len(cfg.Sources) {
				return fmt.Errorf("source %q not found in project %s", name, project)
			}
			cfg.Sources = kept
			return SaveSourcesConfig(sourcesPath, cfg)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project name")
	return cmd
}

// StatusSourceCommand returns the cobra command for `beacon source status`.
func StatusSourceCommand() *cobra.Command {
	var project, name string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "status [name]",
		Short: "Show observed Kubernetes workloads",
		Long:  `Connect to the cluster, sync once, and print workload status (desired/available, drift, health).`,
		Example: `  beacon source status my-k8s --project myapp`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if len(args) > 0 {
				name = args[0]
			}
			paths, err := config.NewBeaconPaths()
			if err != nil {
				return err
			}
			sourcesPath := paths.GetProjectSourcesFile(project)
			cfg, err := LoadSourcesConfig(sourcesPath)
			if err != nil {
				return err
			}
			var src *SourceConfig
			for i := range cfg.Sources {
				if cfg.Sources[i].Type == "kubernetes" && (name == "" || cfg.Sources[i].Name == name) {
					src = &cfg.Sources[i]
					break
				}
			}
			if src == nil {
				return fmt.Errorf("no Kubernetes source found (use --project and optional source name)")
			}
			stateDir := paths.GetProjectStateDir(project)
			clusterID := ClusterIDFromKubeconfig(src.Kubeconfig, src.InCluster)
			obsCfg := K8sObserverConfig{
				SourceConfig: *src,
				StateDir:     stateDir,
				ClusterID:    clusterID,
			}
			// Use no-op sink so status is read-only: connect, sync, print, exit without writing state
			observations, err := RunObserverOnce(obsCfg, NoopSink{}, timeout)
			if err != nil {
				return err
			}
			// Table
			fmt.Printf("%-30s %-12s %-20s %6s %6s %-8s %s\n", "WORKLOAD", "NAMESPACE", "KIND", "DESIRED", "AVAIL", "DRIFT", "HEALTH")
			fmt.Println(strings.Repeat("-", 100))
			for _, o := range observations {
				drift := "no"
				if o.InDrift {
					drift = strings.Join(o.DriftReasons, ",")
				}
				health := strings.Join(o.HealthSignals, ",")
				if health == "" {
					health = "-"
				}
				fmt.Printf("%-30s %-12s %-20s %6d %6d %-8s %s\n", o.Name, o.Namespace, o.Kind, o.DesiredReplicas, o.AvailableReplicas, drift, health)
			}
			fmt.Printf("\nTotal: %d workload(s)\n", len(observations))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project name")
	cmd.Flags().StringVar(&name, "name", "", "Source name (optional)")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultStatusTimeout, "Timeout for cluster sync")
	return cmd
}
