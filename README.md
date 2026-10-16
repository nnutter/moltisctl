# moltisctl

Manage one persistent [Moltis](https://moltis.org/) container per person.
Use rootless Podman on Linux or Apple's `container` CLI on macOS.
The host Tailscale daemon exposes each instance through its own Tailscale Service.

## Build and run

Build with Go 1.27.2 or later.
Install the CLI at a stable path before creating instances, because startup recovery uses that executable.

```sh
go install ./cmd/moltisctl
moltisctl build
moltisctl create alice
moltisctl status
moltisctl update alice
moltisctl delete alice
```

Before creation, define `svc:moltis-alice` in the Tailscale admin console with a TCP endpoint on port 443.
Grant the intended user access to that service.
Approve the deployment host for that service after creation.
The CLI manages only the local Serve endpoint, not service definitions, approval, or access policy.

Open the URL from `status` to complete Moltis onboarding.
Retrieve the setup code from `podman logs moltisctl-alice` on Linux or `container logs moltisctl-alice` on macOS.
Set a password or register a passkey, then configure a provider in Moltis.
Authentication remains enabled, including for requests through Tailscale Serve.

## Persistence and isolation

The default state directory is `~/.local/state/moltisctl` on Linux, or `$XDG_STATE_HOME/moltisctl` when set.
On macOS, it is `~/Library/Application Support/moltisctl`.
Use `--state-dir /absolute/path` or `MOLTISCTL_STATE_DIR` to select another directory.
Each instance retains its `instances/<name>/home` directory, mounted at `/home/moltis`.
Moltis configuration lives in `.config/moltis`, and data lives in `.moltis` within that home.
Treat the entire home as sensitive data.

Creation and updates can be retried after failure.
Updates require an image built or pulled locally and replace the container while retaining its home and allocated port.
Deletion disables startup and removes the container and its owned network, but retains the home and instance record.
Use the same name with `create` to reuse retained data.
There is no automatic permanent-data deletion or backup.

The image pins Moltis 20260913.02 by its multi-platform registry digest.
Rebuilding does not select a newer upstream Moltis release or replace existing containers.
Moltis listens on HTTP port 9000 inside the container, published only to a stable host loopback port.
Host Tailscale Serve terminates HTTPS.
Each instance has its own container network and separate Moltis resource names and ownership labels.

The image uses a non-root user and removes upstream's passwordless sudo.
No host runtime socket or Tailscale state is mounted.
Fresh instances select Moltis's restricted-host tool backend, where the execution host is the instance container.
This backend is not a per-command container sandbox and does not isolate tools from that instance's own credentials or data.
Container isolation does not protect against hostile users or kernel exploits.
App publishing and the Shelley-specific voice helpers are not included.

## Startup prerequisites

On Linux, configure rootless Podman and a working systemd user manager.
Enable lingering for the owning account if recovery must run without a login session.
On macOS, start Apple's container services, install its recommended kernel, and approve Local Network access.
Startup recovery uses a launchd LaunchAgent and requires a GUI login session.
Both platforms reconcile active instances approximately every 30 seconds.
A current host-side Tailscale client, MagicDNS, HTTPS, and permission to configure Serve are required.

## Validation

```sh
go test -race -shuffle=on ./...
go vet ./...
# Builds the embedded image and tests native creation and replacement.
# Does not configure Tailscale or host startup files.
MOLTISCTL_TEST_NATIVE=1 go test -v ./image -run TestNativeEmbeddedContext
```

The local `shelleyctl/` directory is an ignored reference copy, not part of this tool.
