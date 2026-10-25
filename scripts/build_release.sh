#!/usr/bin/env bash
set -euo pipefail

OUT="release_bins"
rm -rf "$OUT"
mkdir -p "$OUT/downloads" "$OUT/aegis-ping"
export CGO_ENABLED=0

build() {
  local goos="$1" goarch="$2" pkg="$3" output="$4" goarm="${5:-}"
  GOOS="$goos" GOARCH="$goarch" GOARM="$goarm" go build -trimpath -ldflags='-s -w' -o "$output" "$pkg"
}

build linux amd64 ./cmd/server "$OUT/aegis_server_linux_amd64"
build windows amd64 ./cmd/server "$OUT/aegis_server_windows_amd64.exe"
build linux amd64 ./cmd/agent "$OUT/downloads/aegis_agent_linux_amd64"
build linux arm64 ./cmd/agent "$OUT/downloads/aegis_agent_linux_arm64"
build linux 386 ./cmd/agent "$OUT/downloads/aegis_agent_linux_386"
build windows amd64 ./cmd/agent "$OUT/downloads/aegis_agent_windows_amd64.exe"
build linux amd64 ./cmd/agent-updater "$OUT/downloads/aegis_agent_updater_linux_amd64"
build linux arm64 ./cmd/agent-updater "$OUT/downloads/aegis_agent_updater_linux_arm64"
build linux 386 ./cmd/agent-updater "$OUT/downloads/aegis_agent_updater_linux_386"
build linux amd64 ./cmd/aegis-ping "$OUT/aegis-ping/aegis_ping_linux_amd64"
build linux arm64 ./cmd/aegis-ping "$OUT/aegis-ping/aegis_ping_linux_arm64"
build linux 386 ./cmd/aegis-ping "$OUT/aegis-ping/aegis_ping_linux_386"
build linux arm ./cmd/aegis-ping "$OUT/aegis-ping/aegis_ping_linux_armv7" 7
build windows amd64 ./cmd/aegis-ping "$OUT/aegis-ping/aegis_ping_windows_amd64.exe"
cp scripts/install_aegis_ping.sh "$OUT/aegis-ping/install_aegis_ping.sh"
cp install.sh "$OUT/install.sh"

python3 - <<'PY'
import hashlib, json, pathlib, subprocess
root = pathlib.Path("release_bins")
assets = {}
for path in sorted((root / "downloads").iterdir()):
    if path.is_file():
        assets[path.name] = hashlib.sha256(path.read_bytes()).hexdigest()
tag = subprocess.check_output(["git", "describe", "--tags", "--exact-match", "HEAD"], text=True).strip()
(root / "update-manifest.json").write_text(json.dumps({"version": tag, "assets": assets}, indent=2) + "\n")
PY
