package webdav

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// Config holds WebDAV cloud drive credentials and E2E backup passphrase.
type Config struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	AutoSync   bool   `json:"autoSync"`
}

// RemoteFile represents a backup file stored on WebDAV.
type RemoteFile struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func normalizeDirURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("WebDAV URL is required")
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid WebDAV URL: %q", raw)
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String(), nil
}

func doReq(method, targetURL, user, pass string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, err
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return httpClient.Do(req)
}

// EnsureDir attempts MKCOL on the target directory URL (ignoring 405 Already Exists).
func EnsureDir(cfg Config) error {
	dirURL, err := normalizeDirURL(cfg.URL)
	if err != nil {
		return err
	}
	resp, err := doReq("MKCOL", dirURL, cfg.Username, cfg.Password, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 201 Created, 405 Method Not Allowed (already exists), 301/200 OK
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == 405 || resp.StatusCode == 301 {
		return nil
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("WebDAV 认证失败 (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// Test verifies connectivity and credentials against the WebDAV endpoint.
func Test(cfg Config) error {
	if err := EnsureDir(cfg); err != nil {
		return err
	}
	_, err := List(cfg)
	return err
}

// Upload writes data to filename inside the configured WebDAV directory.
func Upload(cfg Config, filename string, data []byte) error {
	dirURL, err := normalizeDirURL(cfg.URL)
	if err != nil {
		return err
	}
	_ = EnsureDir(cfg)

	target := dirURL + url.PathEscape(path.Base(filename))
	resp, err := doReq("PUT", target, cfg.Username, cfg.Password, bytes.NewReader(data), map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("WebDAV PUT failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Download fetches filename from the configured WebDAV directory.
func Download(cfg Config, filename string) ([]byte, error) {
	dirURL, err := normalizeDirURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	target := dirURL + url.PathEscape(path.Base(filename))
	resp, err := doReq("GET", target, cfg.Username, cfg.Password, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("WebDAV GET failed (HTTP %d)", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

type multistatusXML struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				ContentLength int64  `xml:"getcontentlength"`
				LastModified  string `xml:"getlastmodified"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

// List lists .vpscoop backup files in the configured WebDAV directory.
func List(cfg Config) ([]RemoteFile, error) {
	dirURL, err := normalizeDirURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	const propBody = `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:getcontentlength/>
    <D:getlastmodified/>
  </D:prop>
</D:propfind>`

	resp, err := doReq("PROPFIND", dirURL, cfg.Username, cfg.Password, strings.NewReader(propBody), map[string]string{
		"Depth":        "1",
		"Content-Type": "application/xml; charset=utf-8",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("WebDAV 认证失败 (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("WebDAV PROPFIND failed (HTTP %d)", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var ms multistatusXML
	if err := xml.Unmarshal(raw, &ms); err != nil {
		return nil, fmt.Errorf("parse WebDAV XML: %w", err)
	}

	files := []RemoteFile{}
	for _, r := range ms.Responses {
		decodedHref, _ := url.PathUnescape(r.Href)
		name := path.Base(strings.TrimRight(decodedHref, "/"))
		if !strings.HasSuffix(strings.ToLower(name), ".vpscoop") {
			continue
		}
		var size int64
		var mod string
		if len(r.Propstat) > 0 {
			size = r.Propstat[0].Prop.ContentLength
			mod = r.Propstat[0].Prop.LastModified
		}
		files = append(files, RemoteFile{
			Name:         name,
			Size:         size,
			LastModified: mod,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})
	return files, nil
}
