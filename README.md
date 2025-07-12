# beacon
Lightweight deployment and reporting agent for self-hosted IoT devices such as Raspberry Pi.

---

## ✨ Features

- Polls a Git repo for new tags
- Automatically deploys latest tagged version
- Executes custom deploy commands (Docker, scripts, etc.)
- Runs an HTTP status server
- Systemd compatible
- Minimal setup

---

## **Project Structure for Multi-Repo Support**

```
/etc/beacon/
  projects/
    myapp/
      env
    anotherapp/
      env
/opt/beacon/
  myapp/
  anotherapp/
systemd/
  beacon@.service
```

---

## **Example Project Environment File**

`beacon.env.example` (copy to `/etc/beacon/projects/myapp/env`):
```
# Example Beacon project environment file

# Repository URL (supports both HTTPS and SSH)
BEACON_REPO_URL=git@github.com:yourusername/yourrepo.git
# For HTTPS: BEACON_REPO_URL=https://github.com/yourusername/yourrepo.git

# Path to SSH private key (optional, for SSH URLs)
BEACON_SSH_KEY_PATH=/etc/beacon/id_ed25519

# Personal access token (optional, for HTTPS URLs)
BEACON_GIT_TOKEN=

# Local deployment path
BEACON_LOCAL_PATH=/opt/beacon/project

# Deploy command to run after update (optional)
# Example: "docker compose up --build -d" or "./install.sh"
BEACON_DEPLOY_COMMAND=docker compose up --build -d

# Polling interval (e.g., 30s, 1m, 5m)
BEACON_POLL_INTERVAL=60s

# HTTP server port
BEACON_PORT=8080
```

> **Note:**
> - Use `BEACON_DEPLOY_COMMAND` to specify a shell command to run after each deployment. This can be a Docker Compose command, a shell script, or any command your project needs.
> - For more complex setups, place an `install.sh` or similar script in your repository and set `BEACON_DEPLOY_COMMAND=./install.sh`.
> - Beacon will log whether the deploy command succeeded or failed.

---

## **Systemd Service Template**

`systemd/beacon@.service`:
```ini
[Unit]
Description=Beacon Agent for %i - Lightweight deployment and reporting for IoT
After=network.target

[Service]
EnvironmentFile=/etc/beacon/projects/%i/env
Type=simple
ExecStart=/usr/local/bin/beacon
WorkingDirectory=/opt/beacon/%i
Restart=always
RestartSec=5
User=pi

# Logging
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

> **Note:** With systemd, logs are stored in the systemd journal. View them with:
> ```bash
> journalctl -u beacon@myapp -f
> ```
```

---

## 🚀 Installation

1. Build the binary:
   ```bash
   GOOS=linux GOARCH=arm GOARM=7 go build -o beacon ./cmd/beacon
   ```
   For arm64:
   ```bash
   GOOS=linux GOARCH=arm64 go build -o beacon ./cmd/beacon
   ```

2. Copy to your system:
   ```bash
   chmod +x beacon
   sudo cp beacon /usr/local/bin/beacon
   ```

3. (Optional) Set up systemd:
   ```bash
   sudo cp systemd/beacon@.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable beacon@myproject
   sudo systemctl start beacon@myproject
   ```

---

## **How to Use**

1. **Create a project env file:**  
   Copy the example to `/etc/beacon/projects/<projectname>/env` and edit as needed.

2. **Create the deployment directory:**  
   `sudo mkdir -p /opt/beacon/<projectname>`

3. **Enable and start the service:**  
   ```bash
   sudo systemctl enable --now beacon@projectname
   ```

---

<pre lang="markdown">

### 🧪 Example Run on Raspberry Pi

```bash
pi@raspberrypi:/media/pi/HIKSEMI/applications/beacon-tests/beacon $ beacon
2025/07/11 17:40:54 [Beacon] Agent starting...
Enter the Git repo URL [https://github.com/yourusername/yourrepo.git]: https://github.com/Bajusz15/beacon.git
Enter the local path for the project [/opt/beacon/project]: /media/pi/HIKSEMI/applications/beacon-tests/test
Enter the port to run on [8080]: 8080
Enter the SSH key path (optional): 
Enter the Git token (optional): ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
2025/07/11 17:41:22 [Beacon] Status server listening on :8080
2025/07/11 17:42:24 [Beacon] New tag found: v0.0.1 (prev: )
2025/07/11 17:42:24 [Beacon] Deploying tag v0.0.1...
2025/07/11 17:42:25 [Beacon] Deployment of tag v0.0.1 complete.
```
</pre>

---

[☕ Buy me a coffee](coff.ee/matebajusz)  
If you find Beacon helpful, consider supporting my work!


# Beacon

Beacon is a lightweight deployment and update agent for self-hosted and IoT devices. It polls a Git repository for new tags and deploys code when a new tag appears.

## ✨ Features

- Polls a Git repo for new tags
- Automatically deploys latest tagged version
- Executes custom deploy commands (Docker, scripts, etc.)
- Runs an HTTP status server
- Systemd compatible
- Minimal setup

## 🚀 Installation

1. Build the binary:
   ```bash
   GOOS=linux GOARCH=arm GOARM=7 go build -o beacon ./cmd/beacon
   ```
   For arm64:
   ```bash
   GOOS=linux GOARCH=arm64 go build -o beacon ./cmd/beacon
   ```

2. Copy to your system:
   ```bash
   chmod +x beacon
   sudo cp beacon /usr/local/bin/beacon
   ```

3. (Optional) Set up systemd:
   ```bash
   sudo cp systemd/beacon@.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable beacon@myproject
   sudo systemctl start beacon@myproject
   ```

## 🧪 Example Run on Raspberry Pi

```bash
pi@raspberrypi:/media/pi/HIKSEMI/applications/beacon-tests/beacon $ beacon
2025/07/11 17:40:54 [Beacon] Agent starting...
Enter the Git repo URL [https://github.com/yourusername/yourrepo.git]: https://github.com/Bajusz15/beacon.git
Enter the local path for the project [/opt/beacon/project]: /media/pi/HIKSEMI/applications/beacon-tests/test
Enter the port to run on [8080]: 8080
Enter the SSH key path (optional): 
Enter the Git token (optional): ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
2025/07/11 17:41:22 [Beacon] Status server listening on :8080
2025/07/11 17:42:24 [Beacon] New tag found: v0.0.1 (prev: )
2025/07/11 17:42:24 [Beacon] Deploying tag v0.0.1...
2025/07/11 17:42:25 [Beacon] Deployment of tag v0.0.1 complete.
```

## ⚙️ Alternative: Run in Background (without systemd)

If you prefer not to use systemd, you can run `beacon` in the background and log output to a file:

```bash
nohup beacon > beacon.log 2>&1 &
```

To stop it later:
```bash
kill $(pgrep beacon)
```

## 📄 License

Apache 2.0