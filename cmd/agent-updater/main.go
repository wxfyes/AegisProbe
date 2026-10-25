// aegis-agent-updater is a privileged, narrowly scoped helper. It only replaces
// the fixed Agent binary after verifying a GitHub Actions Sigstore bundle.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"tianque_probe/internal/common"
)

const repository = "wxfyes/AegisProbe"

type config struct {
	ServerURL   string `json:"server_url"`
	NodeID      string `json:"node_id"`
	Token       string `json:"token"`
	AgentPath   string `json:"agent_path"`
	UpdaterPath string `json:"updater_path"`
	Service     string `json:"service_manager"` // systemd or openrc
}

type manifest struct {
	Version string            `json:"version"`
	Assets  map[string]string `json:"assets"`
}

type updateResponse struct {
	Pending bool   `json:"pending"`
	Version string `json:"version"`
}

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
var versionOutputPattern = regexp.MustCompile(`v[0-9]+\.[0-9]+\.[0-9]+`)

func main() {
	configPath := flag.String("config", "/opt/aegis-probe/agent/update.json", "updater config file")
	showVersion := flag.Bool("v", false, "show updater version")
	flag.Parse()
	if *showVersion {
		fmt.Printf("AegisProbe Agent updater %s\n", common.Version)
		return
	}
	cfg, err := readConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if os.Geteuid() != 0 {
		log.Fatal("updater must run as root")
	}
	if cfg.Service != "systemd" && cfg.Service != "openrc" {
		log.Fatalf("unsupported service manager %q", cfg.Service)
	}
	if runtime.GOOS != "linux" {
		log.Fatalf("unsupported operating system %q", runtime.GOOS)
	}
	log.Printf("Aegis Agent updater started for node %s; polling every 60 seconds", cfg.NodeID)
	client := &http.Client{Timeout: 45 * time.Second}
	for {
		if err := checkAndUpdate(context.Background(), client, cfg); err != nil {
			log.Printf("update check: %v", err)
		}
		time.Sleep(time.Minute)
	}
}

func readConfig(path string) (config, error) {
	var cfg config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if cfg.ServerURL == "" || cfg.NodeID == "" || cfg.Token == "" || cfg.AgentPath != "/opt/aegis-probe/agent/aegis_agent" || cfg.UpdaterPath != "/opt/aegis-probe/agent/aegis_agent_updater" {
		return cfg, errors.New("invalid updater configuration")
	}
	if !strings.HasPrefix(cfg.ServerURL, "https://") && !strings.HasPrefix(cfg.ServerURL, "http://") {
		return cfg, errors.New("server URL must use HTTP or HTTPS")
	}
	return cfg, nil
}

func checkAndUpdate(ctx context.Context, client *http.Client, cfg config) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ServerURL+"/api/agent/update/check", nil)
	setNodeAuth(req, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("master update check returned HTTP %d", resp.StatusCode)
	}
	var update updateResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&update); err != nil {
		return err
	}
	if !update.Pending {
		return nil
	}
	if !versionPattern.MatchString(update.Version) {
		return errors.New("master requested an invalid release version")
	}
	agentCurrent, agentErr := installedVersion(cfg.AgentPath)
	updaterCurrent, updaterErr := installedVersion(cfg.UpdaterPath)
	if agentErr == nil && updaterErr == nil && compareVersions(agentCurrent, update.Version) >= 0 && compareVersions(updaterCurrent, update.Version) >= 0 {
		return reportStatus(ctx, client, cfg, "success", "当前 Agent 已是该版本或更新版本")
	}
	_ = reportStatus(ctx, client, cfg, "downloading", "正在从 GitHub 下载签名清单")
	if err := installRelease(ctx, client, cfg, update.Version); err != nil {
		_ = reportStatus(ctx, client, cfg, "error", err.Error())
		return err
	}
	return reportStatus(ctx, client, cfg, "success", "Agent 已完成签名校验、更新并重启")
}

func releaseURL(version, asset string) string {
	return "https://github.com/" + repository + "/releases/download/" + version + "/" + asset
}

func download(ctx context.Context, client *http.Client, url string, max int64) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("download exceeded allowed size")
	}
	return b, nil
}

func installRelease(ctx context.Context, client *http.Client, cfg config, requested string) error {
	manifestBytes, err := download(ctx, client, releaseURL(requested, "update-manifest.json"), 1<<20)
	if err != nil {
		return fmt.Errorf("download signed manifest: %w", err)
	}
	bundleBytes, err := download(ctx, client, releaseURL(requested, "update-manifest.sigstore.json"), 4<<20)
	if err != nil {
		return fmt.Errorf("download signature bundle: %w", err)
	}
	if err := verifyManifest(manifestBytes, bundleBytes); err != nil {
		return fmt.Errorf("manifest signature verification failed: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return err
	}
	if m.Version != requested {
		return errors.New("signed manifest version does not match requested version")
	}
	if current, err := installedVersion(cfg.UpdaterPath); err != nil || compareVersions(current, m.Version) < 0 {
		updaterAsset := "aegis_agent_updater_linux_" + runtime.GOARCH
		updaterHash, ok := m.Assets[updaterAsset]
		if !ok || len(updaterHash) != 64 {
			return fmt.Errorf("signed manifest has no valid hash for %s", updaterAsset)
		}
		updaterBinary, err := download(ctx, client, releaseURL(requested, updaterAsset), 100<<20)
		if err != nil {
			return fmt.Errorf("download update helper: %w", err)
		}
		hash := sha256.Sum256(updaterBinary)
		if hex.EncodeToString(hash[:]) != strings.ToLower(updaterHash) {
			return errors.New("update helper SHA-256 does not match the signed manifest")
		}
		if err := replaceBinary(cfg.UpdaterPath, updaterBinary, m.Version); err != nil {
			return fmt.Errorf("replace update helper: %w", err)
		}
		if err := restartUpdaterService(cfg.Service); err != nil {
			return fmt.Errorf("update helper installed but restart failed: %w", err)
		}
		return nil
	}
	asset := "aegis_agent_linux_" + runtime.GOARCH
	want, ok := m.Assets[asset]
	if !ok || len(want) != 64 {
		return fmt.Errorf("signed manifest has no valid hash for %s", asset)
	}
	binary, err := download(ctx, client, releaseURL(requested, asset), 100<<20)
	if err != nil {
		return fmt.Errorf("download Agent: %w", err)
	}
	hash := sha256.Sum256(binary)
	if hex.EncodeToString(hash[:]) != strings.ToLower(want) {
		return errors.New("Agent SHA-256 does not match the signed manifest")
	}
	_ = reportStatus(ctx, client, cfg, "verifying", "签名及 SHA-256 校验通过，准备安装")
	_ = reportStatus(ctx, client, cfg, "installing", "正在替换 Agent 并重启固定服务")
	return replaceAndRestart(cfg, binary, m.Version)
}

func verifyManifest(manifestBytes, bundleBytes []byte) error {
	tmp, err := os.MkdirTemp("", "aegis-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	manifestPath := filepath.Join(tmp, "update-manifest.json")
	bundlePath := filepath.Join(tmp, "update-manifest.sigstore.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(bundlePath, bundleBytes, 0600); err != nil {
		return err
	}
	b, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return err
	}
	trustOptions := tuf.DefaultOptions()
	trustOptions.DisableLocalCache = true
	tufClient, err := tuf.New(trustOptions)
	if err != nil {
		return fmt.Errorf("load Sigstore trust root: %w", err)
	}
	trusted, err := root.GetTrustedRoot(tufClient)
	if err != nil {
		return fmt.Errorf("load Sigstore trusted root: %w", err)
	}
	identity, err := verify.NewShortCertificateIdentity(
		"https://token.actions.githubusercontent.com", "", "",
		`^https://github\.com/wxfyes/AegisProbe/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`,
	)
	if err != nil {
		return err
	}
	verifier, err := verify.NewVerifier(trusted, verify.WithSignedCertificateTimestamps(1), verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	if err != nil {
		return err
	}
	artifact, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer artifact.Close()
	_, err = verifier.Verify(b, verify.NewPolicy(verify.WithArtifact(artifact), verify.WithCertificateIdentity(identity)))
	return err
}

func replaceAndRestart(cfg config, binary []byte, expectedVersion string) error {
	backupPath := cfg.AgentPath + ".previous"
	if err := replaceBinary(cfg.AgentPath, binary, expectedVersion); err != nil {
		return err
	}
	if err := restartService(cfg.Service); err != nil {
		_ = os.Remove(cfg.AgentPath)
		_ = os.Rename(backupPath, cfg.AgentPath)
		_ = restartService(cfg.Service)
		return fmt.Errorf("restart failed; restored previous Agent: %w", err)
	}
	time.Sleep(2 * time.Second)
	if !serviceIsActive(cfg.Service) {
		_ = os.Remove(cfg.AgentPath)
		_ = os.Rename(backupPath, cfg.AgentPath)
		_ = restartService(cfg.Service)
		return errors.New("updated Agent did not become active; restored previous Agent")
	}
	_ = os.Remove(backupPath)
	return nil
}

func replaceBinary(path string, binary []byte, expectedVersion string) error {
	dir := filepath.Dir(path)
	tmpPath := filepath.Join(dir, ".aegis_update.new")
	backupPath := path + ".previous"
	if err := os.WriteFile(tmpPath, binary, 0755); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	versionOut, err := exec.Command(tmpPath, "-v").CombinedOutput()
	if err != nil || !bytes.Contains(versionOut, []byte(expectedVersion)) {
		_ = os.Remove(tmpPath)
		return errors.New("downloaded binary version does not match the signed manifest")
	}
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return err
	}
	return nil
}

func serviceIsActive(service string) bool {
	var cmd *exec.Cmd
	if service == "systemd" {
		cmd = exec.Command("systemctl", "is-active", "--quiet", "aegis-agent.service")
	} else {
		cmd = exec.Command("rc-service", "aegis-agent", "status")
	}
	return cmd.Run() == nil
}

func installedVersion(path string) (string, error) {
	out, err := exec.Command(path, "-v").CombinedOutput()
	if err != nil {
		return "", err
	}
	version := versionOutputPattern.Find(out)
	if len(version) == 0 {
		return "", errors.New("installed Agent did not report a semantic version")
	}
	return string(version), nil
}

func compareVersions(a, b string) int {
	var av, bv [3]int
	_, _ = fmt.Sscanf(strings.TrimPrefix(a, "v"), "%d.%d.%d", &av[0], &av[1], &av[2])
	_, _ = fmt.Sscanf(strings.TrimPrefix(b, "v"), "%d.%d.%d", &bv[0], &bv[1], &bv[2])
	for i := range av {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}

func restartService(service string) error {
	var cmd *exec.Cmd
	if service == "systemd" {
		cmd = exec.Command("systemctl", "restart", "aegis-agent.service")
	} else {
		cmd = exec.Command("rc-service", "aegis-agent", "restart")
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func restartUpdaterService(service string) error {
	var cmd *exec.Cmd
	if service == "systemd" {
		cmd = exec.Command("systemctl", "restart", "aegis-agent-updater.service")
	} else {
		cmd = exec.Command("rc-service", "aegis-agent-updater", "restart")
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func setNodeAuth(req *http.Request, cfg config) {
	req.Header.Set("X-Agent-Node-ID", cfg.NodeID)
	req.Header.Set("X-Agent-Token", cfg.Token)
}

func reportStatus(ctx context.Context, client *http.Client, cfg config, status, message string) error {
	body, _ := json.Marshal(map[string]string{"status": status, "message": message})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ServerURL+"/api/agent/update/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setNodeAuth(req, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update status returned HTTP %d", resp.StatusCode)
	}
	return nil
}
