# Moltis port scope and validation

## Deployment contract

`moltisctl` retains the reference tool's per-person instance model, host-side Tailscale Services, and native runtime adapters.
It can import saved Shelley custom model connections, but does not migrate conversations or adopt Shelley containers, startup files, networks, or Tailscale services.
Moltis instances use separate ownership labels and persistent state.
The tools still share host resources, so each allocation checks that a backend port is available.

The image derives from [Moltis 20260913.02](https://github.com/moltis-org/moltis/releases/tag/20260913.02), pinned to the multi-platform OCI index digest in `image/Dockerfile`.
The upstream image supplies the Moltis binary, web assets, Chromium, and WASM artifacts.
The port adds operator utilities and an entrypoint for persistent homes and native runtime user identities.
Distribution packages can change when the image is rebuilt.

Moltis's [pinned Docker documentation](https://github.com/moltis-org/moltis/blob/20260913.02/docs/src/docker.md) supports `--no-tls` behind a reverse proxy with `MOLTIS_BEHIND_PROXY=true`.
The container exposes only HTTP port 9000 through the runtime's loopback publication.
Tailscale Serve provides the external HTTPS endpoint.
The CA-download and OAuth callback ports from the upstream deployment example are not published.
Provider flows that require a separate callback port therefore need additional deployment work.

Moltis's [pinned authentication documentation](https://github.com/moltis-org/moltis/blob/20260913.02/docs/src/authentication.md) identifies `/health` as a public endpoint.
Readiness checks require a successful 2xx response from that endpoint, not an onboarding redirect.
A healthy backend does not prove that onboarding, provider credentials, or Tailscale authorization are complete.

Moltis's [pinned sandbox documentation](https://github.com/moltis-org/moltis/blob/20260913.02/docs/src/sandbox.md) supports the `restricted-host` backend.
This port selects it for fresh homes rather than granting instances access to the host container runtime.
The execution host is the instance container.
This does not provide per-session container isolation, and tools can access that instance's own data.
The image removes the upstream passwordless sudo configuration and package.
Existing user configuration remains user-controlled, not an enforced security policy.

Moltis's [pinned provider documentation](https://github.com/moltis-org/moltis/blob/20260913.02/docs/src/providers.md) supports custom OpenAI-compatible providers in `moltis.toml`.
CLI bootstrapping uses that format while the instance is inactive.
It does not bypass the authenticated UI or call the provider.
The native image test runs `moltis config check` against the generated configuration.

## Shelley model import contract

The source schema comes from Shelley's [custom model table](https://github.com/boldsoftware/shelley/blob/v0.1363.920203221/db/schema/012-custom-models.sql) and [model queries](https://github.com/boldsoftware/shelley/blob/v0.1363.920203221/db/query/models.sql).
Source discovery uses the selected shelleyctl `state.json` record and `instances/<name>/home/.local/state/shelley/shelley.db`.
Import reads only `model_id`, `provider_type`, `endpoint`, `api_key`, and `model_name` from the `models` table.
Extra source columns are ignored, but missing required columns cause an error.

The embedded pure-Go SQLite driver opens with `mode=ro` and `query_only(1)`.
One SELECT reads a consistent snapshot, including committed WAL entries, without schema migrations or provider requests.
The importer does not use SQLite's immutable mode, because that can ignore WAL data.
Normal SQLite reading can require access to the source's WAL and shared-memory metadata.
SQL and parsing errors do not include saved credential values in user-visible diagnostics.

The importer validates the complete source before modifying the destination.
It merges the provider batch in memory and publishes one private TOML file through atomic replacement.
A failed conflict check leaves the prior TOML bytes and lifecycle record unchanged.
Configuration publication and lifecycle-record publication are separate filesystem operations, not one transaction.
An I/O failure after configuration publication can therefore leave imported TOML without a new lifecycle record, and retrying the command is safe.
Imports never start the destination or change host startup or Tailscale configuration.

## Automated checks

`mise run all` runs the race-enabled Go suite, vet, staticcheck, errcheck, testifylint, and whitespace checks.
The suite covers lifecycle recovery, ownership conflicts, runtime protocols, startup rendering, Tailscale protocols, offline provider setup, credential input, and Shelley imports.
Import tests exercise real SQLite databases with committed WAL entries, source retention, path escaping, source selection, unsupported models, grouping, retries, and late batch conflicts.
Tests use recorded native-command responses for host integration paths and do not require a live tailnet.

`mise run test-native` builds the embedded context, then uses the real native runtime without installing startup files or configuring Tailscale.
It checks:

- A running Moltis backend and its real `/health` endpoint.
- Setup-required authentication, with no local-access bypass.
- Non-root execution, libc user lookup, and absence of sudo and a host Docker socket.
- Manually configured and Shelley-imported provider TOML accepted by the pinned Moltis binary.
- Configuration and data-file retention across container removal and replacement.

The native test uses unique container and image names and temporary homes.
It removes its containers, networks, and test image after execution.
It does not contact a paid model provider.

## Results and remaining acceptance checks

During the port, the Go suite and static checks passed on macOS ARM64.
The native Apple-container image test also passed, including replacement with retained home files and manually configured and Shelley-imported provider settings.
The import also passed CGO-disabled model tests and a CGO-disabled Linux AMD64 cross-build.
The cross-build does not establish native Linux runtime acceptance.
The reference tool's live-host acceptance results do not establish acceptance for Moltis.

The following checks remain:

- A native Linux rootless Podman image run, including SELinux hosts where applicable.
- An AMD64 image build and run.
- End-to-end HTTPS access through a real `svc:moltis-NAME` Tailscale Service.
- Access from an allowed client and rejection of a denied client.
- WebSocket streaming and passkey registration through the Tailscale hostname.
- Authenticated onboarding, a paid-model conversation, tool execution, and browser automation.
- Moltis authentication and session-data retention through a real update.
- Startup recovery across a real logout or reboot.

App publishing, Shelley-specific skills, Muse transcription helpers, and Shelley conversation-data migration remain outside this port's scope.
Model connection imports do not prove that a particular Meta endpoint works with Moltis.
