package web

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// envelope 与 cmd.Envelope JSON 形状一致（避免 web→cmd 反向依赖）
type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// Server 是按需启动的 Gin 远程服务：REST API + 内嵌前端静态资源
type Server struct {
	Ops    Ops
	TM     *TaskManager
	Token  string // 空 = 不鉴权（仅本地测试场景）
	Static fs.FS  // 可空；非空时 NoRoute 伺服前端（SPA 回落）
	OnStop func() // /api/web/stop 触发（serve 循环用它退出）
}

func NewServer(ops Ops, tm *TaskManager, token string, static fs.FS) *Server {
	return &Server{Ops: ops, TM: tm, Token: token, Static: static}
}

func (s *Server) Handler() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	api := r.Group("/api", s.auth())
	s.routes(api)
	if s.Static != nil {
		// 静态资源不鉴权（token 由入口 URL query 携带，前端捕获后存 localStorage）
		r.NoRoute(func(c *gin.Context) { s.serveStatic(c) })
	} else {
		r.NoRoute(func(c *gin.Context) {
			c.JSON(http.StatusOK, envelope{OK: false, Error: "not found"})
		})
	}
	return r.Handler()
}

// auth 校验 ?token= 或 Authorization: Bearer；Token 为空时放行
func (s *Server) auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.Token == "" {
			return
		}
		if c.Query("token") == s.Token {
			return
		}
		if h := c.GetHeader("Authorization"); h != "" && strings.TrimPrefix(h, "Bearer ") == s.Token {
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

// ok 统一信封输出：业务错误映射 ok:false（HTTP 仍 200）
func ok(c *gin.Context, data any, err error) {
	if err != nil {
		c.JSON(http.StatusOK, envelope{OK: false, Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, envelope{OK: true, Data: data})
}

func (s *Server) routes(api *gin.RouterGroup) {
	// call 统一「执行→信封」样板，避免多返回值与 gin.Context 混传
	call := func(c *gin.Context, fn func() (any, error)) {
		data, err := fn()
		ok(c, data, err)
	}

	api.GET("/status", func(c *gin.Context) { call(c, s.Ops.Status) })
	api.POST("/server/start", func(c *gin.Context) {
		call(c, func() (any, error) { return s.Ops.ServerAction("start") })
	})
	api.POST("/server/stop", func(c *gin.Context) {
		call(c, func() (any, error) { return s.Ops.ServerAction("stop") })
	})
	api.POST("/server/restart", func(c *gin.Context) {
		call(c, func() (any, error) { return s.Ops.ServerAction("restart") })
	})
	api.PUT("/server", func(c *gin.Context) {
		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			ok(c, nil, err)
			return
		}
		call(c, func() (any, error) { return s.Ops.ServerSet(payload) })
	})
	api.GET("/apps", func(c *gin.Context) { call(c, s.Ops.Apps) })
	api.GET("/gadget", func(c *gin.Context) { call(c, s.Ops.GadgetRules) })
	api.PUT("/gadget", func(c *gin.Context) {
		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			ok(c, nil, err)
			return
		}
		call(c, func() (any, error) { return s.Ops.GadgetSet(payload) })
	})
	api.GET("/bin", func(c *gin.Context) { call(c, s.Ops.BinList) })
	api.GET("/bin/sources", func(c *gin.Context) { call(c, s.Ops.BinSources) })
	api.POST("/bin/download", func(c *gin.Context) {
		var req struct {
			Variant string `json:"variant"`
			Type    string `json:"type"`
			Version string `json:"version"`
		}
		if err := c.BindJSON(&req); err != nil {
			ok(c, nil, err)
			return
		}
		call(c, func() (any, error) { return s.Ops.BinDownload(req.Variant, req.Type, req.Version, true, s.TM) })
	})
	api.POST("/bin/import", func(c *gin.Context) {
		path, err := saveUpload(c)
		if err != nil {
			ok(c, nil, err)
			return
		}
		call(c, func() (any, error) { return s.Ops.BinImport(c.PostForm("type"), path, c.PostForm("version"), true, s.TM) })
	})
	api.DELETE("/bin/:file", func(c *gin.Context) {
		call(c, func() (any, error) { return s.Ops.BinRemove(c.Query("type"), c.Param("file")) })
	})
	api.POST("/bin/use", func(c *gin.Context) {
		var req struct {
			Type string `json:"type"`
			File string `json:"file"`
		}
		if err := c.BindJSON(&req); err != nil {
			ok(c, nil, err)
			return
		}
		call(c, func() (any, error) { return s.Ops.BinUse(req.Type, req.File) })
	})
	api.POST("/bin/cleanup", func(c *gin.Context) { call(c, s.Ops.BinCleanup) })
	api.GET("/tasks/:id", func(c *gin.Context) {
		task, found := s.TM.Get(c.Param("id"))
		if !found {
			ok(c, nil, os.ErrNotExist)
			return
		}
		ok(c, task, nil)
	})
	api.GET("/logs", func(c *gin.Context) {
		name, tail := c.Query("name"), atoiDefault(c.Query("tail"), 200)
		call(c, func() (any, error) { return s.Ops.Logs(name, tail) })
	})
	api.GET("/web/info", func(c *gin.Context) { call(c, s.Ops.WebInfo) })
	api.PUT("/web/token", func(c *gin.Context) {
		payload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			ok(c, nil, err)
			return
		}
		call(c, func() (any, error) { return s.Ops.WebToken(payload) })
	})
	api.POST("/web/stop", func(c *gin.Context) {
		c.Header("X-Utsusemi-Stop", "1")
		ok(c, map[string]bool{"stopping": true}, nil)
		if s.OnStop != nil {
			go s.OnStop() // 异步：先让响应落地，serve 循环随后退出
		}
	})
}

// saveUpload 把 multipart 文件落到临时目录并保留原文件名（Import 以文件名为 key）
func saveUpload(c *gin.Context) (string, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "utsusemi-upload-*")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(fh.Filename))
	in, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return dst, nil
}

// serveStatic 伺服内嵌前端；未命中时回落 index.html（SPA，仅非 /api 路径）
func (s *Server) serveStatic(c *gin.Context) {
	// /api 前缀的未知路径保持信封语义，不回落静态页
	if strings.HasPrefix(c.Request.URL.Path, "/api") {
		c.JSON(http.StatusOK, envelope{OK: false, Error: "not found"})
		return
	}
	p := strings.TrimPrefix(c.Request.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.Static.Open(p); err == nil {
		if st, err := f.Stat(); err == nil && !st.IsDir() {
			defer f.Close()
			c.DataFromReader(http.StatusOK, st.Size(), contentType(p), f, nil)
			return
		}
		f.Close()
	}
	index, err := s.Static.Open("index.html")
	if err != nil {
		c.String(http.StatusNotFound, "no frontend embedded")
		return
	}
	defer index.Close()
	st, err := index.Stat()
	if err != nil {
		c.String(http.StatusNotFound, "no frontend embedded")
		return
	}
	c.DataFromReader(http.StatusOK, st.Size(), "text/html; charset=utf-8", index, nil)
}

func contentType(p string) string {
	switch {
	case strings.HasSuffix(p, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(p, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(p, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(p, ".png"):
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
