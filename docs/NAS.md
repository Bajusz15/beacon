# NAS support architecture

Beacon should treat NAS access as a generic storage capability, not as an
integration with any single NAS product. The NAS stays private on the LAN.
Beacon is the boundary that reads explicitly shared paths and exposes them
through authenticated Beacon access paths.

This document describes the planned architecture. NAS file browsing and
downloads are not a shipped Beacon feature yet.

## Goals

- Browse selected NAS folders remotely without opening NAS ports to the internet.
- Support ordinary NAS setups first: SMB or NFS mounted by the operating system,
  then exposed to Beacon as local paths.
- Keep the first version read-only.
- Make small and medium browser downloads work through the existing remote-access
  flow.
- Leave a clear path for large direct transfers over Beacon VPN later.

## Recommended MVP

The first version should be a Beacon-local, read-only HTTP file gateway:

```text
NAS or mounted share -> Beacon file gateway -> Beacon remote access -> browser
```

Beacon reads files from configured local paths. Those paths may be normal
directories, USB disks, or OS-mounted SMB/NFS shares. Beacon does not need to
know NAS credentials for the first version; the user mounts the storage using
their OS or NAS tooling.

Example configuration shape:

```yaml
storage:
  enabled: true
  shares:
    - id: media
      name: Media
      root: /mnt/nas/media
      read_only: true
    - id: documents
      name: Documents
      root: /mnt/nas/documents
      read_only: true
```

The gateway should expose:

| API | Purpose |
|---|---|
| List directory | Names, type, size, modified time, and MIME hint for one path. |
| File metadata | Size, type, modified time, and download eligibility. |
| Download | Read-only file response with `Range` support. |
| Session token | Short-lived token scoped to device, share, path, and action. |
| Audit event | Access metadata with privacy-preserving filename handling by default. |

## Why this shape

Beacon's current remote-access tunnel is HTTP/WebSocket oriented, so an HTTP file
gateway matches the product that already exists. It is also browser-friendly:
directory listings and downloads can be represented as normal authenticated HTTP
requests.

Raw NAS protocols are a poor public boundary. SMB and NFS should remain LAN-only,
and Beacon should not forward them directly to the internet. If protocol-level
access is needed later, WebDAV or SFTP are better candidates than SMB/NFS because
they are file-transfer oriented and easier to scope behind one authenticated
service.

The tradeoff is bandwidth. In the MVP, downloaded bytes pass through the
BeaconInfra relay, so downloads must be explicit, metered, and size-aware. Before
large downloads are treated as supported, the tunnel needs a streaming transfer
path that preserves `Range` requests, supports cancellation, and accounts for
bytes without buffering whole responses.

## Evolution path

The file gateway should be designed as the data plane for a later direct path:

```text
remote Beacon client -> Beacon VPN -> Beacon file gateway -> NAS or mounted share
```

In this model BeaconInfra coordinates authentication, device selection, and
session grants, but file bytes flow directly over WireGuard between the remote
client and the Beacon device. That matches Beacon VPN's design: the cloud stores
public keys and endpoints but does not relay VPN traffic.

A browser-first direct transfer path could also be built later with WebRTC or a
QUIC-based peer-to-peer transport. That is a larger project because it needs
signaling, NAT traversal diagnostics, resumable chunking, relay fallback, and
relay quotas.

## Alternatives

| Option | Shape | BeaconInfra bandwidth | Fit |
|---|---|---:|---|
| HTTP file gateway | Beacon reads selected paths and serves an authenticated HTTP API. | High for downloads | Best MVP. Simple, browser-friendly, and keeps NAS ports private. |
| Direct Beacon VPN | Remote client connects to the Beacon gateway over `beacon0`. | Near zero | Best long-term path for large downloads. Requires a Beacon client and reachable VPN path. |
| P2P WebRTC/QUIC | Browser or client establishes a direct data channel to Beacon. | Zero when direct, high on relay fallback | Future browser-native option. More complex than VPN. |
| Raw protocol relay | Relay TCP for SMB/NFS/SFTP/WebDAV through BeaconInfra. | High | Avoid for SMB/NFS. Consider only explicit, metered WebDAV/SFTP modes. |

## Security requirements

- NAS SMB/NFS/WebDAV/SFTP ports stay private to the LAN.
- Beacon exposes only configured shares, never the entire NAS by default.
- Default mode is read-only.
- Every remote NAS session requires BeaconInfra authentication plus the
  device-side remote-access gate when configured.
- Authorization is enforced by Beacon before any file or directory is opened.
- Download tokens are short-lived, single-purpose, and scoped to device, share,
  path, and action.
- Path handling canonicalizes input and rejects traversal, ambiguous encodings,
  symlink escapes, hidden roots outside the configured share, disabled shares,
  and unauthorized devices.
- Access logs record metadata by default. Full filenames should be opt-in for
  detailed audit logging.
- Beacon should never forward raw SMB or NFS to the public internet.

## Implementation notes

Start with a local filesystem backend. That gives users NAS support immediately
when their NAS is already mounted at `/mnt`, `/media`, `/Volumes`, or another
local path.

Later backends can be added behind the same share interface:

- library-backed SMB/CIFS
- NFS through OS mounts
- SFTP
- WebDAV
- rclone-backed remotes

Credential storage should wait until Beacon needs to manage those connections
itself. If Beacon stores NAS credentials later, it should encrypt them using the
same local secret model used for other Beacon private material.

## Test plan

- Unit tests for path normalization, traversal rejection, symlink escape
  rejection, disabled shares, read-only enforcement, and token scope.
- Unit tests for `Range` requests, MIME metadata, missing files, directories
  requested as files, and permission errors.
- Integration tests with a temporary local directory as the first storage
  backend.
- Security tests for encoded traversal payloads, expired tokens, unauthorized
  devices, and share roots changed while the service is running.
- Tunnel tests proving large responses stream in bounded memory and byte
  accounting tracks downloads accurately.
- Optional backend tests with containerized Samba, WebDAV, or SFTP after the
  filesystem gateway is stable.
