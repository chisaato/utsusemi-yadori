// Package webui 内嵌前端构建产物（scripts/build.sh 从 web/dist 拷贝覆盖）。
// 占位 index.html 保证空仓库可编译；真产物伺服 SPA
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS 返回以 dist/ 内容为根的文件系统（Open("index.html") 直接可用）
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // 静态前缀，编译期注定成功
	}
	return sub
}
