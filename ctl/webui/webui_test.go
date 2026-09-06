package webui

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// WithFallback：有 index.html 的 fs 原样返回（读到的就是原内容）
func TestWithFallback_PassesThrough(t *testing.T) {
	inner := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("real")}}
	data, err := fs.ReadFile(WithFallback(inner), "index.html")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "real" {
		t.Fatalf("expected passthrough content, got %q", data)
	}
}

// WithFallback：无 index.html 时切换到兜底页
func TestWithFallback_SwitchesToFallback(t *testing.T) {
	inner := fstest.MapFS{"assets": &fstest.MapFile{Data: []byte("x")}}
	got := WithFallback(inner)
	data, err := fs.ReadFile(got, "index.html")
	if err != nil {
		t.Fatalf("open fallback index.html: %v", err)
	}
	if !strings.Contains(string(data), "utsusemi") {
		t.Fatalf("fallback html should mention utsusemi, got %q", data)
	}
}

// 兜底 fs：非 index.html 路径一律 ErrNotExist（serveStatic 会落到 404 分支前先拿到 fallback index）
func TestFallbackFS_NonIndexNotFound(t *testing.T) {
	if _, err := (fallbackFS{}).Open("assets/app.js"); err == nil {
		t.Fatal("expected ErrNotExist for non-index path")
	}
}

// FS() 必须可用（磁盘态无论空产物/真产物都不 panic 且 index.html 可读）
func TestFS_AlwaysUsable(t *testing.T) {
	if _, err := fs.Stat(FS(), "index.html"); err != nil {
		t.Fatalf("FS() must always serve index.html: %v", err)
	}
}
