package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	repoOwner = "wxfyes"
	repoName  = "AegisProbe"
	tag       = "v1.0.0"
)

type Release struct {
	ID        int64  `json:"id"`
	TagName   string `json:"tag_name"`
	UploadURL string `json:"upload_url"`
	Assets    []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"assets"`
}

func main() {
	log.Println("=================================================================")
	log.Printf("🚀 天阙·神盾 (AegisProbe) GitHub Release 极速发布工具 [%s]", tag)
	log.Println("=================================================================")

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		tokenBytes, err := os.ReadFile(".token")
		if err == nil {
			token = strings.TrimSpace(string(tokenBytes))
		}
	}
	if token == "" {
		log.Fatal("❌ 未找到 GitHub Token！请在 .token 文件中填写或设置 GITHUB_TOKEN 环境变量")
	}

	client := &http.Client{Timeout: 120 * time.Second}

	// 1. 读取发布说明
	notesBytes, err := os.ReadFile("scripts/release_notes.md")
	notes := "AegisProbe Release " + tag
	if err == nil {
		notes = string(notesBytes)
	}

	// 2. 检查或创建 Release
	release, err := getOrCreateRelease(client, token, notes)
	if err != nil {
		log.Fatalf("❌ 处理 Release 失败: %v", err)
	}
	log.Printf("✓ 目标 Release ID: %d (Tag: %s)", release.ID, release.TagName)

	// 3. 待上传的文件 (统一从 bin 目录读取最新交叉编译产物)
	filesToUpload := []string{
		// 主控服务端
		"bin/aegis_server_linux_amd64",
		"bin/aegis_server_windows_amd64.exe",
		// 受控 Agent
		"bin/aegis_agent_linux_amd64",
		"bin/aegis_agent_linux_arm64",
		"bin/aegis_agent_linux_386",
		"bin/aegis_agent_windows_amd64.exe",
		// 家宽测活微探针 Aegis-Ping (全架构)
		"bin/aegis_ping_linux_amd64",
		"bin/aegis_ping_linux_arm64",
		"bin/aegis_ping_linux_armv7",
		"bin/aegis_ping_linux_386",
		"bin/aegis_ping_windows_amd64.exe",
		// 一键部署脚本
		"install.sh",
		"scripts/install_aegis_ping.sh",
	}

	// 4. 清理旧资产
	for _, a := range release.Assets {
		log.Printf("🗑️  清理旧资产: %s (ID: %d)...", a.Name, a.ID)
		delURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/assets/%d", repoOwner, repoName, a.ID)
		req, _ := http.NewRequest(http.MethodDelete, delURL, nil)
		req.Header.Set("Authorization", "token "+token)
		resp, delErr := client.Do(req)
		if delErr == nil {
			resp.Body.Close()
		}
	}

	// 5. 依次上传资产
	for _, fp := range filesToUpload {
		info, statErr := os.Stat(fp)
		if statErr != nil {
			log.Printf("⚠️ 文件不存在，跳过: %s", fp)
			continue
		}

		fileName := filepath.Base(fp)
		log.Printf("⬆️  正在上传: %s (%.2f MB)...", fileName, float64(info.Size())/(1024*1024))

		fileData, readErr := os.ReadFile(fp)
		if readErr != nil {
			log.Printf("❌ 读取文件失败: %v", readErr)
			continue
		}

		uploadURL := fmt.Sprintf("https://uploads.github.com/repos/%s/%s/releases/%d/assets?name=%s", repoOwner, repoName, release.ID, fileName)
		req, reqErr := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(fileData))
		if reqErr != nil {
			log.Printf("❌ 创建上传请求失败: %v", reqErr)
			continue
		}
		req.Header.Set("Authorization", "token "+token)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("User-Agent", "TianQue-AegisProbe-Publisher")

		resp, doErr := client.Do(req)
		if doErr != nil {
			log.Printf("❌ 上传网络失败 %s: %v", fileName, doErr)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("✅ 上传成功: %s", fileName)
		} else {
			log.Printf("❌ 上传响应异常 %s [%d]: %s", fileName, resp.StatusCode, string(respBody))
		}
	}

	log.Println("")
	log.Println("=================================================================")
	log.Println("🎉 全架构二进制与一键脚本已成功发布至 GitHub Release！")
	log.Printf("👉 Release 页面: https://github.com/%s/%s/releases/tag/%s", repoOwner, repoName, tag)
	log.Printf("👉 最新下载页:   https://github.com/%s/%s/releases/latest", repoOwner, repoName)
	log.Println("=================================================================")
}

func getOrCreateRelease(client *http.Client, token, notes string) (*Release, error) {
	// 尝试直接获取
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", repoOwner, repoName, tag)
	req, _ := http.NewRequest(http.MethodGet, getURL, nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var r Release
		if json.NewDecoder(resp.Body).Decode(&r) == nil {
			return &r, nil
		}
	}
	if resp != nil {
		resp.Body.Close()
	}

	// 不存在则创建
	createURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", repoOwner, repoName)
	bodyMap := map[string]interface{}{
		"tag_name":         tag,
		"target_commitish": "main",
		"name":             fmt.Sprintf("🛡️ 天阙·神盾安全探针 %s 正式首发版", tag),
		"body":             notes,
		"draft":            false,
		"prerelease":       false,
	}
	bodyBytes, _ := json.Marshal(bodyMap)

	postReq, _ := http.NewRequest(http.MethodPost, createURL, bytes.NewReader(bodyBytes))
	postReq.Header.Set("Authorization", "token "+token)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Accept", "application/vnd.github.v3+json")

	postResp, err := client.Do(postReq)
	if err != nil {
		return nil, err
	}
	defer postResp.Body.Close()

	if postResp.StatusCode < 200 || postResp.StatusCode >= 300 {
		b, _ := io.ReadAll(postResp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", postResp.StatusCode, string(b))
	}

	var newRel Release
	if err := json.NewDecoder(postResp.Body).Decode(&newRel); err != nil {
		return nil, err
	}
	return &newRel, nil
}
