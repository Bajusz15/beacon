#!/bin/bash

# Simple Git HTTP Server for E2E Testing
# This script runs a Git HTTP server that serves a bare Git repository over HTTP
# Uses a simple Go HTTP server

set -e

GIT_REPO_DIR="${1:-/tmp/beacon-e2e-git/test-repo.git}"
GIT_SERVER_PORT="${2:-8080}"

if [ ! -d "$GIT_REPO_DIR" ]; then
    echo "Error: Git repository not found at $GIT_REPO_DIR"
    exit 1
fi

# Create a simple Go HTTP server for Git
cat > /tmp/git-server.go << 'GOSERVER'
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

func main() {
    repoDir := os.Getenv("GIT_REPO_DIR")
    if repoDir == "" {
        repoDir = "/tmp/beacon-e2e-git/test-repo.git"
    }
    
    port := os.Getenv("GIT_SERVER_PORT")
    if port == "" {
        port = "8080"
    }
    
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        log.Printf("Request: %s %s", r.Method, r.URL.Path)
        
        // Extract repo name from path
        path := strings.TrimPrefix(r.URL.Path, "/")
        
        // Handle git-upload-pack (fetch/clone)
        if strings.Contains(path, "/git-upload-pack") || strings.Contains(path, "/info/refs") {
            // Extract repo name (everything before /info/refs or /git-upload-pack)
            repoName := path
            if idx := strings.Index(repoName, "/info/refs"); idx > 0 {
                repoName = repoName[:idx]
            } else if idx := strings.Index(repoName, "/git-upload-pack"); idx > 0 {
                repoName = repoName[:idx]
            }
            
            // Remove leading/trailing slashes
            repoName = strings.Trim(repoName, "/")
            
            // If repoName is empty, use default
            if repoName == "" {
                repoName = "test-repo.git"
            }
            
            // Construct full repo path
            fullRepoPath := filepath.Join("/tmp/beacon-e2e-git", repoName)
            
            if !strings.HasSuffix(fullRepoPath, ".git") {
                fullRepoPath = fullRepoPath + ".git"
            }
            
            // Check if repo exists for logging
            _, repoExists := os.Stat(fullRepoPath)
            log.Printf("Repository path: %s (exists: %v)", fullRepoPath, repoExists == nil)
            
            if r.Method == "GET" && strings.Contains(r.URL.Path, "/info/refs") {
                // Git info/refs request
                service := r.URL.Query().Get("service")
                if service == "git-upload-pack" {
                    // Check if repo exists
                    if _, err := os.Stat(fullRepoPath); os.IsNotExist(err) {
                        log.Printf("Repository not found: %s", fullRepoPath)
                        http.Error(w, fmt.Sprintf("Repository not found: %s", fullRepoPath), 404)
                        return
                    }
                    
                    // Use git-upload-pack with proper arguments for bare repo
                    // Change to repo directory first (git upload-pack works better this way)
                    originalDir, _ := os.Getwd()
                    defer os.Chdir(originalDir)
                    
                    if err := os.Chdir(fullRepoPath); err != nil {
                        log.Printf("Cannot chdir to repo: %v", err)
                        http.Error(w, fmt.Sprintf("Cannot access repository: %v", err), 500)
                        return
                    }
                    
                    // Try git upload-pack from within the repo directory
                    cmd := exec.Command("git", "upload-pack", "--advertise-refs", ".")
                    cmd.Env = append(os.Environ(), "GIT_PROTOCOL=version=2")
                    output, err := cmd.CombinedOutput()
                    if err != nil {
                        log.Printf("Git upload-pack advertise error (with protocol v2): %v, output: %s", err, string(output))
                        // Try without GIT_PROTOCOL version 2
                        cmd2 := exec.Command("git", "upload-pack", "--advertise-refs", ".")
                        output2, err2 := cmd2.CombinedOutput()
                        if err2 != nil {
                            log.Printf("Git upload-pack advertise error (without protocol v2): %v, output: %s", err2, string(output2))
                            http.Error(w, fmt.Sprintf("Git error: cannot advertise refs: %v", err2), 500)
                            return
                        }
                        output = output2
                    }
                    w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
                    w.Header().Set("Cache-Control", "no-cache")
                    w.WriteHeader(200)
                    w.Write([]byte("001e# service=git-upload-pack\n"))
                    w.Write([]byte("0000"))
                    w.Write(output)
                    return
                }
            }
            
            if r.Method == "POST" && strings.Contains(r.URL.Path, "/git-upload-pack") {
                // Git upload-pack request
                // Check if repo exists
                if _, err := os.Stat(fullRepoPath); os.IsNotExist(err) {
                    log.Printf("Repository not found: %s", fullRepoPath)
                    http.Error(w, fmt.Sprintf("Repository not found: %s", fullRepoPath), 404)
                    return
                }
                
                // Change to repo directory for upload-pack
                originalDir, _ := os.Getwd()
                defer os.Chdir(originalDir)
                
                if err := os.Chdir(fullRepoPath); err != nil {
                    log.Printf("Cannot chdir to repo: %v", err)
                    http.Error(w, fmt.Sprintf("Cannot access repository: %v", err), 500)
                    return
                }
                
                cmd := exec.Command("git", "upload-pack", "--stateless-rpc", ".")
                cmd.Env = append(os.Environ(), "GIT_PROTOCOL=version=2")
                cmd.Stdin = r.Body
                cmd.Stdout = w
                cmd.Stderr = os.Stderr
                w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
                w.Header().Set("Cache-Control", "no-cache")
                w.WriteHeader(200)
                if err := cmd.Run(); err != nil {
                    log.Printf("Git upload-pack error: %v", err)
                    // Don't return error here, git client will handle it
                }
                return
            }
        }
        
        http.NotFound(w, r)
    })
    
    log.Printf("Git HTTP server starting on :%s", port)
    log.Printf("Serving repository: %s", repoDir)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}
GOSERVER

# Build and run the Go server
cd /tmp
go build -o git-server git-server.go
export GIT_REPO_DIR="$GIT_REPO_DIR"
export GIT_SERVER_PORT="$GIT_SERVER_PORT"

echo "Starting Git HTTP server on port $GIT_SERVER_PORT"
echo "Repository: $GIT_REPO_DIR"
exec ./git-server
