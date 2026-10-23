package web

import (
	"embed"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed dist
var embeddedFiles embed.FS

// RegisterStaticRoutes 精准且稳健地挂载所有嵌入式前端静态资源
// 彻底解决 http.FileServer 在特定系统/代理下对根路径 "/" 返回 404 的问题
func RegisterStaticRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" || cleanPath == "index.html" {
			cleanPath = "index.html"
		}

		fullPath := "dist/" + cleanPath
		data, err := embeddedFiles.ReadFile(fullPath)
		if err != nil {
			// 若未匹配且不含扩展名，兜底到 index.html
			if !strings.Contains(cleanPath, ".") {
				data, err = embeddedFiles.ReadFile("dist/index.html")
				cleanPath = "index.html"
			}
		}

		if err != nil {
			http.NotFound(w, r)
			return
		}

		// 根据文件后缀精准识别 Content-Type
		ext := filepath.Ext(cleanPath)
		ctype := mime.TypeByExtension(ext)
		if ctype == "" {
			switch ext {
			case ".html":
				ctype = "text/html; charset=utf-8"
			case ".css":
				ctype = "text/css; charset=utf-8"
			case ".js":
				ctype = "application/javascript; charset=utf-8"
			case ".json":
				ctype = "application/json; charset=utf-8"
			case ".svg":
				ctype = "image/svg+xml"
			case ".png":
				ctype = "image/png"
			default:
				ctype = "text/plain; charset=utf-8"
			}
		}

		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}
