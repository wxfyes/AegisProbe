---
name: tianque-probe
description: Project handoff and safe development/release guide for the TianQue-Probe (AegisProbe) repository.
---

# TianQue-Probe / AegisProbe project guide

Read this before making code, deployment, or release changes. The repository is `wxfyes/AegisProbe`; the Go module is `tianque_probe`.

## Current project model

- The Go master embeds the web UI from `internal/server/web/dist`. When editing files under `web/dist`, keep the embedded copy in sync.
- Agents report metrics to the master. The project deliberately has no remote shell, arbitrary command, or script execution channel.
- Agent automatic updates are separate from metric reporting. On supported Linux hosts, a privileged local updater polls a fixed master endpoint, checks a requested release, verifies the Sigstore bundle and signed manifest against the pinned GitHub Actions identity, checks the Agent SHA-256, replaces only the fixed Agent binary, and restarts only its fixed service.
- Admin-triggered Agent updates are per-node requests. Once the updater is installed, the operator can trigger a node from the master UI without SSH. This is not a general-purpose remote command system.
- The updater supports Linux with systemd or OpenRC. Do not claim it supports Windows or OpenWrt/procd unless the implementation changes.
- The updater is installed by the Agent setup command. Existing nodes without the updater need the setup/update path that installs it once; a green Agent version alone does not prove the updater service is installed. On Linux, check `systemctl is-active aegis-agent-updater` (systemd) or `rc-service aegis-agent-updater status` (OpenRC).

## Release signing: no local private key

The canonical release path is GitHub Actions, defined in `.github/workflows/release.yml`:

1. Update `internal/common/types.go` version and `scripts/release_notes.md` as appropriate. Check that all release assets and manifest generation in `scripts/build_release.sh` match the intended version and platforms.
2. Commit and push the release-ready code, then create/push a new `vX.Y.Z` tag pointing at that commit. The workflow triggers on `v*` tags.
3. GitHub Actions builds the binaries and `update-manifest.json`, installs the pinned Cosign version, and runs `cosign sign-blob --yes --bundle ...` with `id-token: write`. Sigstore uses GitHub's short-lived OIDC identity and transparency services; no developer creates, stores, or sends a signing private key.
4. The same workflow uploads binaries, install scripts, `update-manifest.json`, and `update-manifest.sigstore.json` to the GitHub Release.
5. Verify the Actions run is completed successfully and the Release contains both manifest files and all expected assets before telling the user the release is ready. A Release page appearing before the workflow finishes is not sufficient.

Never put access tokens, `.token` contents, signing keys, or credentials into this skill, source files, logs, chat, or commit output. Do not use local publishing scripts as a substitute for the canonical signed release workflow: they do not mint the GitHub Actions OIDC signature bundle required by the updater. Do not reuse or move a published release tag; use a new patch version for corrections.

The updater pins the accepted repository/workflow identity in `cmd/agent-updater/main.go`. If the repository owner, workflow path, tag format, permissions, or signing method changes, update and review both producer and verifier together. Do not weaken signature verification to make a release pass.

## Build and review notes

- `scripts/build_release.sh` requires the current `HEAD` to have an exact `vX.Y.Z` tag (`git describe --tags --exact-match`). Tag only the release commit.
- Master web assets are embedded by Go; edit both `web/dist/` and `internal/server/web/dist/` or synchronize them before building the master.
- Avoid making tests part of a task unless requested; for normal changes inspect the diff and run only the validation relevant to the request and repository instructions.
- Keep the operator UI understandable: a requested update is pending until that node's updater polls and reports its resulting version. Do not imply that clicking the button synchronously updated the remote machine.

## Where to look

- `.github/workflows/release.yml`: canonical CI build, keyless signature, and Release upload.
- `scripts/build_release.sh`: release binary matrix and signed manifest input.
- `cmd/agent-updater/`: fixed-scope updater, release verification, and service restart.
- `internal/server/handler/handler.go`: updater installation and per-node update request endpoints.
- `internal/server/config/`: persisted update-request configuration.
- `web/dist/` and `internal/server/web/dist/`: admin and public web UI plus embedded copies.
- `README.md` and `docs/WHITEPAPER_AND_HANDOVER.md`: user-facing operations and broader architecture; keep claims aligned with the implementation.
