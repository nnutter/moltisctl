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

## Preconfigure a custom provider

Configure an OpenAI-compatible provider before the first `create`:

```sh
moltisctl models configure alice \
  --api-url https://api.openai.com/v1 \
  --api-key-file ~/.config/moltisctl/provider-key \
  --model MODEL_A \
  --model MODEL_B
moltisctl create alice
```

Use `--api-key-file -` to read one nonempty key line from stdin, limited to 64 KiB.
There is no literal API-key flag.
Use `--api-type openai` for Chat Completions or `openai-responses` for Responses (the default).
Use the provider's base URL, not its `/chat/completions` or `/responses` endpoint.
Use HTTPS for remote endpoints to avoid transmitting credentials without encryption.
Other provider protocols and OAuth require Moltis's UI.

The default provider name is `custom-moltisctl`.
Use `--provider custom-NAME` to configure another custom provider.
Repeated configuration updates that provider's endpoint, key, wire format, and preferred model list while preserving unrelated settings.
An existing provider allowlist also gains the selected provider.
New provider entries disable dynamic model discovery and use the explicit model list.
The operation does not select a default model or send a provider test request.
Moltis can contact the provider after startup.

For an existing instance, run `delete`, configure its retained home, then run `create` again.
The command refuses active records and leftover containers, even if they are currently stopped.
This prevents concurrent writes from Moltis or scheduled recovery.
Alternatively, use Moltis's authenticated UI for live changes.
A new preconfigured record remains inactive until an explicit `create`.

The command rewrites `moltis.toml`, preserving unrelated values but not comments or formatting.
It stores provider keys in that file with mode 0600, not in lifecycle records or launchers.
Tools in the instance can read those keys.
Keep key source files private and back up configuration before changing it.

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
