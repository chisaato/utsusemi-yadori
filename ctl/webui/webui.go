// Package webui 内嵌前端构建产物（scripts/build.sh 从 web/dist 拷贝覆盖）。
// .placeholder 入库保证空仓库可编译；真产物由构建流水线生成，不入 git。
// 产物缺失时 FS() 自动降级为内置提示页，REST API 不受影响。
package webui

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"time"
)

//go:embed all:dist
var distFS embed.FS

//go:embed fallback.html
var fallbackHTML []byte

// FS 返回以 dist/ 为根的文件系统；产物缺失时返回兜底页
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // 静态前缀，编译期注定成功
	}
	return WithFallback(sub)
}

// WithFallback 在 f 缺少 index.html 时切换到内置兜底 fs（空产物 → 编译仍可用）
func WithFallback(f fs.FS) fs.FS {
	if _, err := fs.Stat(f, "index.html"); err != nil {
		return fallbackFS{}
	}
	return f
}

// fallbackFS 仅伺服内置提示页
type fallbackFS struct{}

func (fallbackFS) Open(name string) (fs.File, error) {
	if name != "index.html" {
		return nil, fs.ErrNotExist
	}
	return &memFile{Reader: bytes.NewReader(fallbackHTML), size: int64(len(fallbackHTML))}, nil
}

// memFile 把内存字节包装成 fs.File（serveStatic 只用 Read/Stat）
type memFile struct {
	*bytes.Reader
	size int64
}

func (m *memFile) Close() error               { return nil }
func (m *memFile) Stat() (fs.FileInfo, error) { return memInfo{size: m.size}, nil }

type memInfo struct{ size int64 }

func (i memInfo) Name() string       { return "index.html" }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o444 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }

// 编译期断言接口实现
var _ fs.File = (*memFile)(nil)
var _ fs.FileInfo = memInfo{}
var _ io.Reader = (*memFile)(nil)
