package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"beacon/internal/config"
	"beacon/internal/util"
)

const deployLogFile = "deploy.log"

func deployCommandOutput(cfg *config.Config, command string) (io.Writer, io.Writer, func()) {
	project := cfg.ProjectName
	if project == "" {
		project = filepath.Base(cfg.LocalPath)
	}
	if project == "." || project == string(filepath.Separator) || project == "" {
		return os.Stdout, os.Stderr, func() {}
	}
	base, err := config.BeaconHomeDir()
	if err != nil {
		logger.Infof("Failed to resolve Beacon home for deploy log: %v", err)
		return os.Stdout, os.Stderr, func() {}
	}
	logDir := filepath.Join(base, "logs", project)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		logger.Infof("Failed to create deploy log directory %s: %v", logDir, err)
		return os.Stdout, os.Stderr, func() {}
	}
	path := filepath.Join(logDir, deployLogFile)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		logger.Infof("Failed to open deploy log %s: %v", path, err)
		return os.Stdout, os.Stderr, func() {}
	}
	_, _ = fmt.Fprintf(file, "\n[%s] deploy command: %s\n", time.Now().UTC().Format(time.RFC3339), command)
	closeFn := func() {
		util.Close(file, "deploy log")
	}
	return io.MultiWriter(os.Stdout, file), io.MultiWriter(os.Stderr, file), closeFn
}
