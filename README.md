# beacon
Lightweight deployment and reporting agent for self-hosted IoT devices such as Raspberry Pi.

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

`/etc/beacon/projects/myapp/env`:
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

# Polling interval (e.g., 30s, 1m, 5m)
BEACON_POLL_INTERVAL=60s

# HTTP server port
BEACON_PORT=8080
```

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

---

## **How to Use**

1. **Create a project env file:**  
   Copy the example to `/etc/beacon/projects/<projectname>/env` and edit as needed.

2. **Create the deployment directory:**  
   `sudo mkdir -p /opt/beacon/<projectname>`

3. **Enable and start the service:**  
   ```bash
   sudo systemctl enable --now beacon@myapp
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