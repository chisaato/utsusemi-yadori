package dl

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"utsusemi/ctl/internal/core"
)

type GHAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type GHRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []GHAsset `json:"assets"`
}

type SourceDef struct {
	Repo      string
	ServerPat *regexp.Regexp // capture: 1=version 2=arch
	GadgetPat *regexp.Regexp
}

var Variants = []string{"official", "florida", "undetected"}

var sources = map[string]SourceDef{
	"official": {
		Repo:      "frida/frida",
		ServerPat: regexp.MustCompile(`^frida-server-(\d+\.\d+\.\d+)-android-(arm64|arm|x86_64|x86)\.xz$`),
		GadgetPat: regexp.MustCompile(`^frida-gadget-(\d+\.\d+\.\d+)-android-(arm64|arm|x86_64|x86)\.so\.xz$`),
	},
	"florida": {
		Repo:      "Ylarod/Florida",
		ServerPat: regexp.MustCompile(`^florida-server-(\d+\.\d+\.\d+)-android-(arm64|arm|x86_64|x86)\.gz$`),
		GadgetPat: regexp.MustCompile(`^florida-gadget-(\d+\.\d+\.\d+)-android-(arm64|arm|x86_64|x86)\.so\.gz$`),
	},
	// undetected 上游命名不规则：gadget 的 .so 后缀时有时无
	"undetected": {
		Repo:      "ultrafunkamsterdam/undetected-frida",
		ServerPat: regexp.MustCompile(`^undetected-frida-server-(\d+\.\d+\.\d+)-android-(arm64|arm|x86_64|x86)\.xz$`),
		GadgetPat: regexp.MustCompile(`^undetected-frida-gadget-(\d+\.\d+\.\d+)-android-(arm64|arm|x86_64|x86)(?:\.so)?\.xz$`),
	},
}

func patFor(sd SourceDef, binType string) *regexp.Regexp {
	if binType == "gadget" {
		return sd.GadgetPat
	}
	return sd.ServerPat
}

type Asset struct {
	Name    string
	URL     string
	Version string
	Arch    string
	BinType string
	Size    int64
}

type Client struct {
	API    string
	Mirror string
	HTTP   *http.Client
}

func NewClient(d core.DownloadSettings) *Client {
	// 大文件读取不设整体 Timeout，避免慢速网络 30s 掐断
	// 在 Android CGO 环境下，net.Dialer 默认使用 cgo (getaddrinfo)，可直接解析系统 DNS
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &Client{
		API:    d.GithubAPI,
		Mirror: d.Mirror,
		HTTP: &http.Client{
			Transport: transport,
		},
	}
}

func (c *Client) Releases(ctx context.Context, repo string) ([]GHRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=40", strings.TrimRight(c.API, "/"), repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "utsusemi-ctl")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api %s: %s", url, resp.Status)
	}
	var rs []GHRelease
	return rs, json.NewDecoder(resp.Body).Decode(&rs)
}

// match 在单个 release 中按 binType+arch 找资产；version 参数为空则任意
func match(rel GHRelease, pat *regexp.Regexp, binType, arch, version string) (Asset, bool) {
	v := strings.TrimPrefix(rel.TagName, "v")
	for _, a := range rel.Assets {
		m := pat.FindStringSubmatch(a.Name)
		if m == nil {
			continue
		}
		if arch != "" && m[2] != arch {
			continue
		}
		if version != "" && v != version {
			continue
		}
		return Asset{Name: a.Name, URL: a.URL, Version: v, Arch: m[2], BinType: binType, Size: a.Size}, true
	}
	return Asset{}, false
}

// ListVersions 返回含匹配资产的 tag（新→旧，去重）
func (c *Client) ListVersions(ctx context.Context, variant string) ([]string, error) {
	sd, ok := sources[variant]
	if !ok {
		return nil, fmt.Errorf("unknown variant %q", variant)
	}
	rels, err := c.Releases(ctx, sd.Repo)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, r := range rels {
		tag := strings.TrimPrefix(r.TagName, "v")
		_, s := match(r, sd.ServerPat, "server", "", "")
		_, g := match(r, sd.GadgetPat, "gadget", "", "")
		if (s || g) && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out, nil
}

// FindAsset 精确 version+binType+arch 匹配
func (c *Client) FindAsset(ctx context.Context, variant, version, binType, arch string) (Asset, error) {
	sd, ok := sources[variant]
	if !ok {
		return Asset{}, fmt.Errorf("unknown variant %q", variant)
	}
	rels, err := c.Releases(ctx, sd.Repo)
	if err != nil {
		return Asset{}, err
	}
	for _, r := range rels {
		if a, hit := match(r, patFor(sd, binType), binType, arch, version); hit {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("no %s %s %s asset in %s", variant, binType, arch, version)
}

// DownloadURL 资产直连或镜像前缀
func (c *Client) DownloadURL(a Asset) string {
	if c.Mirror == "" {
		return a.URL
	}
	return strings.TrimRight(c.Mirror, "/") + "/" + a.URL
}
