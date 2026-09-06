package dl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"utsusemi/ctl/internal/core"
)

func testClient(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewClient(core.DownloadSettings{GithubAPI: srv.URL})
}

func ghJSON(tag string, assets ...string) string {
	body := `{"tag_name":"` + tag + `","assets":[`
	for i, a := range assets {
		if i > 0 {
			body += ","
		}
		body += `{"name":"` + a + `","browser_download_url":"https://x/` + a + `","size":1}`
	}
	return body + "]}"
}

func TestListVersionsSkipsAssetlessReleases(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Ylarod/Florida/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("[" +
			ghJSON("17.17.0") + "," + // 无资产，跳过
			ghJSON("17.9.1", "florida-server-17.9.1-android-arm64.gz", "florida-gadget-17.9.1-android-arm64.so.gz") + "," +
			ghJSON("17.16.4") + "]"))
	})
	c := testClient(t, mux)
	vs, err := c.ListVersions(context.Background(), "florida")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0] != "17.9.1" {
		t.Fatalf("got %v", vs)
	}
}

func TestFindAssetUndetectedGadgetNamingIrregular(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/ultrafunkamsterdam/undetected-frida/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("[" + ghJSON("17.7.2",
			"undetected-frida-gadget-17.7.2-android-arm64.xz",   // 无 .so
			"undetected-frida-gadget-17.7.2-android-arm.so.xz", // 有 .so
			"undetected-frida-server-17.7.2-android-arm64.xz") + "]"))
	})
	c := testClient(t, mux)
	a, err := c.FindAsset(context.Background(), "undetected", "17.7.2", "gadget", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "undetected-frida-gadget-17.7.2-android-arm64.xz" || a.BinType != "gadget" || a.Arch != "arm64" {
		t.Fatalf("bad asset %+v", a)
	}
	if _, err := c.FindAsset(context.Background(), "undetected", "17.7.2", "server", "x86"); err == nil {
		t.Fatal("x86 server should not match")
	}
}

func TestDownloadURLMirrorPrefix(t *testing.T) {
	c := NewClient(core.DownloadSettings{Mirror: "https://ghproxy.net/"})
	got := c.DownloadURL(Asset{URL: "https://github.com/a/b"})
	if got != "https://ghproxy.net/https://github.com/a/b" {
		t.Fatalf("got %s", got)
	}
}
