package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ServerSettings struct {
	Autostart      bool     `json:"autostart"`
	Active         string   `json:"active"` // manifest 中 server 的 file key
	Args           []string `json:"args"`
	RestartOnCrash bool     `json:"restart_on_crash"`
}

type GadgetSettings struct {
	Active string `json:"active"` // manifest 中 gadget 的 file key
}

type WebSettings struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
}

type DownloadSettings struct {
	Mirror    string `json:"mirror"`     // 资产下载 URL 前缀，空=直连
	GithubAPI string `json:"github_api"` // API base，默认 https://api.github.com
}

type Settings struct {
	Server   ServerSettings   `json:"server"`
	Gadget   GadgetSettings   `json:"gadget"`
	Web      WebSettings      `json:"web"`
	Download DownloadSettings `json:"download"`
}

func DefaultSettings() Settings {
	return Settings{
		Server:   ServerSettings{Autostart: true, Args: []string{"-l", "127.0.0.1:27042"}},
		Web:      WebSettings{Port: 23333},
		Download: DownloadSettings{GithubAPI: "https://api.github.com"},
	}
}

// LoadSettings 文件缺失时返回默认值；解析失败也返回默认值与错误由调用方决定
func LoadSettings(p Paths) (Settings, error) {
	s := DefaultSettings()
	raw, err := os.ReadFile(p.Settings())
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return DefaultSettings(), err
	}
	return s, nil
}

// SaveSettings 原子写：tmp + rename
func SaveSettings(p Paths, s Settings) error {
	if err := p.Ensure(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.Settings() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(p.Root, "settings.json"))
}
