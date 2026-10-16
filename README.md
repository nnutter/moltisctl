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
Tailscale permissions are additive, so remove or narrow broad grants that would also permit access to these services.
Test both an allowed client and a denied client.

For example, merge this grant into the existing tailnet policy rather than replacing the whole policy:

```json
{
  "grants": [
    {
      "src": ["alice@example.com"],
      "dst": ["svc:moltis-alice"],
      "ip": ["tcp:443"]
    }
  ]
}
```

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
For a consistent backup, delete the instance to stop it and disable recovery, copy its retained home and `state.json`, then create it again.
Do not copy an active SQLite database without its associated WAL files.

Names contain at most 40 lowercase letters, digits, or hyphens, starting with a letter and ending with a letter or digit.
Containers and networks use `moltisctl-<name>`, and services use `svc:moltis-<name>`.
Backend ports come from 19000–29999 and remain in the instance record after deletion.

The image pins Moltis 20260913.02 by its multi-platform registry digest.
Rebuilding does not select a newer upstream Moltis release or replace existing containers.
Use `--image REFERENCE build` to build a custom tag, then `update NAME --to-image REFERENCE` to select it explicitly.
Back up retained data before updates that can migrate Moltis's databases.
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

## Import models from shelleyctl

Import saved Shelley model connections instead of entering their endpoints and keys again:

```sh
# Source and destination use the same instance name.
moltisctl models import-shelleyctl alice
moltisctl create alice

# Select a different source instance or Shelley state directory.
moltisctl models import-shelleyctl alice \
  --from christy \
  --shelley-state-dir "/absolute/path/to/shelleyctl"
```

`NAME` is the destination Moltis instance.
`--from` defaults to `NAME`.
The source directory defaults to `SHELLEYCTL_STATE_DIR`, then the normal shelleyctl state location for the current platform.
The Moltis `--state-dir` flag selects only the destination state directory.
No host `sqlite3` executable is required.

The importer reads the source's recorded persistent `shelley.db` through a read-only SQLite connection, including committed WAL data.
The source can be running or retained after deletion.
Do not copy its active database without the associated WAL files.
The importer does not run Shelley, modify its saved models, or contact a model provider.

The destination must be new or deleted, with no leftover container.
For an existing Moltis instance, run `moltisctl delete NAME` first.
Do not delete the source Shelley instance merely to import its models.
Import leaves the Moltis instance inactive until an explicit `create`.

Models with the same saved endpoint, API format, and key become one Moltis provider with a deduplicated model list.
Different credentials or API formats remain separate providers.
A Meta connection imports when Shelley records it as `openai` (Chat Completions) or `openai-responses`.
The importer copies the saved URL and model names rather than choosing a Meta preset.
It rejects the whole import if any model uses another API format, lacks an explicit endpoint or key, or contains invalid connection settings.
It also rejects empty sources, unsupported schemas, and unrecorded source instances.

Imported names use the prefix `custom-shelleyctl-` and an identifier derived from the source instance name and a saved model ID.
They do not contain API keys or key-derived fingerprints.
Repeated imports of an unchanged source merge without duplicates.
Matching destination connections retain their tuning and additional model names.
Conflicting endpoints, credentials, API formats, or disabled providers cause the entire import to fail before changing destination configuration or lifecycle state.
For deliberate credential replacement, use `models configure --provider IMPORTED_NAME` while the Moltis instance is inactive.
Removing or regrouping source models can produce different provider names, and import never deletes old destination providers.

The import copies connection settings and model names only.
It does not copy conversations, default-model selection, display names, token limits, tags, or reasoning and image settings.
It extends an existing Moltis provider allowlist when needed.
Keys remain sensitive plaintext in the destination's mode-0600 `moltis.toml`, subject to the same access limits as manual provider configuration.
The atomic rewrite preserves unrelated TOML values, but not comments or formatting.
Back up destination configuration before importing.

## Startup prerequisites

On Linux, configure rootless Podman and a working systemd user manager.
Enable lingering for the owning account if recovery must run without a login session:

```sh
loginctl enable-linger "$USER"
```

This can require administrator approval.
Keep Podman's trusted image signature policy configured.
If Podman cannot locate its existing policy, select it with `MOLTISCTL_SIGNATURE_POLICY=/absolute/path/to/policy.json`.
The CLI does not generate a permissive policy.

On macOS, start Apple's container services and install its recommended kernel:

```sh
container system start
container system kernel set --recommended
```

Approve macOS Local Network access when prompted.
Startup recovery uses a launchd LaunchAgent and requires a GUI login session.
Both platforms reconcile active instances approximately every 30 seconds.
A current host-side Tailscale client, MagicDNS, HTTPS, and permission to configure Serve are required.

## Validation

```sh
mise run all
# Or run the core checks directly:
go test -race -shuffle=on ./...
go vet ./...
# Builds the embedded image and tests native creation and replacement.
# Does not configure Tailscale or host startup files.
MOLTISCTL_TEST_NATIVE=1 go test -v ./image -run TestNativeEmbeddedContext
```

Use `--plain` to disable interactive progress.
Generate shell completions with `moltisctl completion <shell>`.
Help and completion do not issue lifecycle commands.

See [port scope and validation](docs/port-validation.md) for upstream contracts, native test results, and remaining deployment checks.
The local `shelleyctl/` directory is an ignored reference copy, not part of this tool.
