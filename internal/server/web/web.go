package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed dist/*
var embeddedFiles embed.FS

// GetFileSystem 返回前端静态资源的 http.FileSystem
func GetFileSystem() http.FileSystem {
	sub, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// DownloadHandler 提供 /download/ 目录下载各架构 Agent 二进制
func DownloadHandler(binDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.URL.Path)
		if filename == "" || filename == "." || strings.Contains(filename, "..") {
			http.NotFound(w, r)
			return
		}

		filePath := filepath.Join(binDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// 若当前目录不存在，尝试在上级目录或当前可执行文件同级目录寻找
			altPath := filepath.Join(".", filename)
			if _, err2 := os.Stat(altPath); err2 == nil {
				filePath = altPath
			} else {
				http.NotFound(w, r)
				return
			}
		}

		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, filePath)
	}
}
