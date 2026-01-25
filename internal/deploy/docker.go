package deploy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"beacon/internal/config"
	"beacon/internal/state"
)

// DockerRegistryClient handles Docker registry operations
type DockerRegistryClient struct {
	image    string
	registry string
	username string
	password string
	token    string
}

// NewDockerRegistryClient creates a new Docker registry client from image config
func NewDockerRegistryClient(imgCfg *config.DockerImageConfig) *DockerRegistryClient {
	// Parse image to extract registry if not explicitly set
	image := imgCfg.Image
	registry := imgCfg.Registry

	// Auto-detect registry from image name if not specified
	if registry == "" {
		parts := strings.Split(image, "/")
		if len(parts) > 2 {
			// Has registry prefix (e.g., ghcr.io/username/app)
			registry = parts[0]
			image = strings.Join(parts[1:], "/")
		} else {
			// Default to Docker Hub
			registry = "docker.io"
		}
	}

	// Remove registry prefix from image if present
	if strings.HasPrefix(image, registry+"/") {
		image = strings.TrimPrefix(image, registry+"/")
	}

	return &DockerRegistryClient{
		image:    image,
		registry: registry,
		username: imgCfg.Username,
		password: imgCfg.Password,
		token:    imgCfg.Token,
	}
}

// CheckForNewImageTag polls all Docker images for new tags
func CheckForNewImageTag(cfg *config.Config, status *state.Status) {
	if len(cfg.DockerImages) == 0 {
		log.Println("[Beacon] No Docker images configured")
		return
	}

	// Get the base storage directory from the existing status
	// We'll use the same directory structure but with image-specific subdirectories
	statusStorageDir := filepath.Join(os.Getenv("HOME"), ".beacon", cfg.ProjectDir)

	// Check each configured image
	for _, imgCfg := range cfg.DockerImages {
		// Create a unique status storage for each image
		// Sanitize image name for filesystem use
		sanitizedImageName := strings.ReplaceAll(strings.ReplaceAll(imgCfg.Image, "/", "_"), ":", "_")
		imageStatusDir := filepath.Join(statusStorageDir, "docker_images", sanitizedImageName)
		imageStatus := state.NewStatus(imageStatusDir)

		client := NewDockerRegistryClient(&imgCfg)

		lastTag, _ := imageStatus.Get()

		// Check if we need to do initial deployment
		shouldDeploy := false
		if lastTag == "" {
			log.Printf("[Beacon] No previous tag found for image %s. Performing initial deployment...\n", imgCfg.Image)
			shouldDeploy = true
		}

		// Get latest tag from registry
		latestTag, err := client.getLatestTag()
		if err != nil {
			log.Printf("[Beacon] Error getting latest tag from registry for %s: %v\n", imgCfg.Image, err)
			continue
		}

		if latestTag == "" {
			log.Printf("[Beacon] No tags found for image %s\n", imgCfg.Image)
			continue
		}

		// Check if we have a new tag
		if !shouldDeploy && latestTag == lastTag {
			continue
		}

		if shouldDeploy {
			log.Printf("[Beacon] Initial deployment for image %s with tag: %s\n", imgCfg.Image, latestTag)
		} else {
			log.Printf("[Beacon] New tag found for image %s: %s (prev: %s)\n", imgCfg.Image, latestTag, lastTag)
		}

		// Deploy the new image
		if err := DeployDockerImage(&imgCfg, cfg, latestTag, imageStatus); err != nil {
			log.Printf("[Beacon] Error deploying Docker image %s: %v\n", imgCfg.Image, err)
		}
	}
}

// getLatestTag fetches the latest tag from the Docker registry
func (c *DockerRegistryClient) getLatestTag() (string, error) {
	// Use Docker Registry API v2 to list tags
	tags, err := c.listTags()
	if err != nil {
		return "", fmt.Errorf("failed to list tags: %w", err)
	}

	if len(tags) == 0 {
		return "", nil
	}

	// Sort tags by creation date (newest first)
	// For simplicity, we'll use semantic versioning if available, otherwise lexicographic
	sort.Slice(tags, func(i, j int) bool {
		return compareTags(tags[i], tags[j]) > 0
	})

	return tags[0], nil
}

// listTags lists all tags for the image from the registry
func (c *DockerRegistryClient) listTags() ([]string, error) {
	// Docker Hub has a special API endpoint
	if c.registry == "docker.io" || c.registry == "registry-1.docker.io" {
		return c.listTagsViaDockerHubAPI()
	}

	// Use standard Docker Registry API v2 for other registries
	return c.listTagsViaAPI()
}

// listTagsViaDockerHubAPI uses Docker Hub's special API endpoint
func (c *DockerRegistryClient) listTagsViaDockerHubAPI() ([]string, error) {
	// Docker Hub uses a different API: https://hub.docker.com/v2/repositories/{namespace}/{repository}/tags
	parts := strings.Split(c.image, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid Docker Hub image format: %s (expected username/repository)", c.image)
	}

	namespace := parts[0]
	repository := parts[1]
	apiURL := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/tags?page_size=100", namespace, repository)

	// Create HTTP request
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if available
	if authHeader := c.getAuthHeader(); authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	// Make the request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query Docker Hub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Docker Hub API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var response struct {
		Count   int `json:"count"`
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
		Next string `json:"next"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse Docker Hub response: %w", err)
	}

	// Extract tag names
	tags := make([]string, 0, len(response.Results))
	for _, result := range response.Results {
		tags = append(tags, result.Name)
	}

	// Handle pagination if needed (for now, we just get the first page)
	// TODO: Implement pagination for images with many tags

	return tags, nil
}

// listTagsViaAPI uses Docker Registry HTTP API v2 to list tags
func (c *DockerRegistryClient) listTagsViaAPI() ([]string, error) {
	schemes := []string{"https", "http"}

	var lastErr error
	for _, scheme := range schemes {
		apiURL := fmt.Sprintf("%s://%s/v2/%s/tags/list", scheme, c.registry, c.image)

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		if authHeader := c.getAuthHeader(); authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}

		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to query registry API (%s): %w", scheme, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("registry API returned status %d via %s: %s", resp.StatusCode, scheme, string(body))
			continue
		}

		var response struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body via %s: %w", scheme, err)
			continue
		}

		if err := json.Unmarshal(body, &response); err != nil {
			lastErr = fmt.Errorf("failed to parse registry response via %s: %w", scheme, err)
			continue
		}

		return response.Tags, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("failed to query registry API")
}

// getAuthHeader returns the Authorization header value for registry API requests
func (c *DockerRegistryClient) getAuthHeader() string {
	if c.token != "" {
		return fmt.Sprintf("Bearer %s", c.token)
	}

	if c.username != "" && c.password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
		return fmt.Sprintf("Basic %s", auth)
	}

	return ""
}

// getFullImageName returns the full image name with registry
func (c *DockerRegistryClient) getFullImageName() string {
	if c.registry == "docker.io" {
		return c.image
	}
	return fmt.Sprintf("%s/%s", c.registry, c.image)
}

// DeployDockerImage pulls and deploys a Docker image
func DeployDockerImage(imgCfg *config.DockerImageConfig, cfg *config.Config, tag string, status *state.Status) error {
	client := NewDockerRegistryClient(imgCfg)

	log.Printf("[Beacon] Deploying Docker image %s:%s...\n", client.getFullImageName(), tag)

	// Pull the Docker image
	fullImageName := fmt.Sprintf("%s:%s", client.getFullImageName(), tag)
	if err := pullDockerImage(fullImageName, client); err != nil {
		return fmt.Errorf("failed to pull Docker image: %w", err)
	}

	// Determine deploy command (use image-specific command if available, otherwise fallback to global)
	deployCommand := imgCfg.DeployCommand
	if deployCommand == "" {
		deployCommand = cfg.DeployCommand
	}

	// Execute deploy command if specified
	if deployCommand != "" {
		log.Printf("[Beacon] Executing deploy command: %s\n", deployCommand)

		// Build the command with secure environment file sourcing
		var command string
		if cfg.SecureEnvPath != "" {
			if _, err := os.Stat(cfg.SecureEnvPath); err == nil {
				log.Printf("[Beacon] Sourcing secure environment file: %s\n", cfg.SecureEnvPath)
				command = fmt.Sprintf("set -a && . %s && set +a && %s", cfg.SecureEnvPath, deployCommand)
			} else {
				log.Printf("[Beacon] Warning: Secure environment file not found: %s\n", cfg.SecureEnvPath)
				command = deployCommand
			}
		} else {
			command = deployCommand
		}

		// Set environment variables for use in deploy command
		env := os.Environ()
		env = append(env, fmt.Sprintf("BEACON_DOCKER_IMAGE=%s", fullImageName))
		env = append(env, fmt.Sprintf("BEACON_DOCKER_TAG=%s", tag))

		// Use DockerComposeFiles array (legacy DockerComposeFile is normalized during config load)
		composeFiles := imgCfg.DockerComposeFiles

		// If docker-compose files are specified, set them as environment variables
		if len(composeFiles) > 0 {
			// Resolve all compose file paths
			resolvedPaths := make([]string, 0, len(composeFiles))
			for _, composeFile := range composeFiles {
				composePath := composeFile
				// If path is relative, make it relative to LocalPath
				if !filepath.IsAbs(composePath) {
					composePath = filepath.Join(cfg.LocalPath, composePath)
				}
				resolvedPaths = append(resolvedPaths, composePath)

				// Verify docker-compose file exists
				if _, err := os.Stat(composePath); os.IsNotExist(err) {
					log.Printf("[Beacon] Warning: Docker Compose file not found: %s\n", composePath)
				}
			}

			// Set environment variables for compose files
			// BEACON_DOCKER_COMPOSE_FILES: Space-separated list of all files (for docker compose -f usage)
			env = append(env, fmt.Sprintf("BEACON_DOCKER_COMPOSE_FILES=%s", strings.Join(resolvedPaths, " ")))
		}

		// Set working directory to LocalPath
		workingDir := cfg.LocalPath
		if len(composeFiles) > 0 {
			// If compose files are specified, use the directory of the first file
			composePath := composeFiles[0]
			if !filepath.IsAbs(composePath) {
				composePath = filepath.Join(cfg.LocalPath, composePath)
			}
			workingDir = filepath.Dir(composePath)
		}

		// Execute the command
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = workingDir
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Printf("[Beacon] Deploy command failed: %v\n", err)
			return err
		}

		log.Printf("[Beacon] Deploy command completed successfully\n")
	}

	// Store the tag
	status.Set(tag, time.Now())

	log.Printf("[Beacon] Deployment of Docker image %s:%s complete.\n", client.getFullImageName(), tag)
	return nil
}

// pullDockerImage pulls a Docker image from the registry
func pullDockerImage(imageName string, client *DockerRegistryClient) error {
	log.Printf("[Beacon] Pulling Docker image: %s\n", imageName)

	// Build docker pull command
	cmd := exec.Command("docker", "pull", imageName)

	// Set up authentication if needed
	if client.username != "" && client.password != "" {
		// Use docker login if credentials are provided
		loginCmd := exec.Command("docker", "login", "-u", client.username, "-p", client.password, client.registry)
		loginCmd.Stdin = strings.NewReader(client.password)
		if err := loginCmd.Run(); err != nil {
			log.Printf("[Beacon] Warning: Docker login failed, trying pull without explicit login: %v\n", err)
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker pull failed: %w", err)
	}

	log.Printf("[Beacon] Successfully pulled Docker image: %s\n", imageName)
	return nil
}

// compareTags compares two tags, returning:
// - positive if tag1 > tag2 (tag1 is newer)
// - negative if tag1 < tag2 (tag2 is newer)
// - 0 if equal
func compareTags(tag1, tag2 string) int {
	// Try semantic versioning comparison first
	if isSemanticVersion(tag1) && isSemanticVersion(tag2) {
		return compareSemanticVersions(tag1, tag2)
	}

	// Fallback to lexicographic comparison
	if tag1 > tag2 {
		return 1
	} else if tag1 < tag2 {
		return -1
	}
	return 0
}

// isSemanticVersion checks if a tag looks like a semantic version
func isSemanticVersion(tag string) bool {
	// Simple check: starts with 'v' followed by numbers and dots
	if len(tag) > 0 && tag[0] == 'v' {
		tag = tag[1:]
	}

	parts := strings.Split(tag, ".")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 {
			return false
		}
		// Check if part is numeric (allowing for pre-release identifiers)
		for _, r := range part {
			if !((r >= '0' && r <= '9') || r == '-' || r == '+') {
				return false
			}
		}
	}
	return true
}

// compareSemanticVersions compares two semantic version tags
func compareSemanticVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	if len(v1) > 0 && v1[0] == 'v' {
		v1 = v1[1:]
	}
	if len(v2) > 0 && v2[0] == 'v' {
		v2 = v2[1:]
	}

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			// Extract numeric part (before any pre-release identifiers)
			numStr1 := parts1[i]
			if idx := strings.IndexAny(numStr1, "-+"); idx != -1 {
				numStr1 = numStr1[:idx]
			}
			fmt.Sscanf(numStr1, "%d", &num1)
		}
		if i < len(parts2) {
			numStr2 := parts2[i]
			if idx := strings.IndexAny(numStr2, "-+"); idx != -1 {
				numStr2 = numStr2[:idx]
			}
			fmt.Sscanf(numStr2, "%d", &num2)
		}

		if num1 > num2 {
			return 1
		} else if num1 < num2 {
			return -1
		}
	}

	return 0
}
