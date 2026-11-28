package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"vpscoop/internal/version"
)

type ghAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt string    `json:"published_at"`
	Assets      []ghAsset `json:"assets"`
}

// UpgradeCheckResult describes the current version and latest GitHub Release status.
type UpgradeCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseName    string `json:"releaseName"`
	ReleaseNotes   string `json:"releaseNotes"`
	PublishedAt    string `json:"publishedAt"`
	HTMLURL        string `json:"htmlUrl"`
	AssetName      string `json:"assetName"`
	DownloadURL    string `json:"downloadUrl"`
	AssetSize      int64  `json:"assetSize"`
	CanSelfUpdate  bool   `json:"canSelfUpdate"`
	Reason         string `json:"reason,omitempty"`
	ExecPath       string `json:"execPath"`
	Repo           string `json:"repo"`
}

func getUpdateRepo() string {
	if repo := strings.TrimSpace(os.Getenv("VPSCOOP_REPO")); repo != "" {
		return repo
	}
	return "Jlan45/VPSCoop"
}

func expectedAssetName() string {
	name := fmt.Sprintf("vpscoop-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, false
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		nums[i] = n
	}
	return nums, true
}

func isNewerVersion(current, latest string) bool {
	if latest == "" {
		return false
	}
	cNums, cOk := parseSemver(current)
	lNums, lOk := parseSemver(latest)
	if !cOk && lOk {
		// e.g. current is "dev"
		return true
	}
	if !cOk || !lOk {
		return strings.TrimPrefix(current, "v") != strings.TrimPrefix(latest, "v")
	}
	for i := 0; i < 3; i++ {
		if lNums[i] > cNums[i] {
			return true
		}
		if lNums[i] < cNums[i] {
			return false
		}
	}
	return false
}

func resolveExecPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	return p, nil
}

func checkDirWritable(execPath string) error {
	dir := filepath.Dir(execPath)
	f, err := os.CreateTemp(dir, ".vpscoop-perm-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func fetchLatestRelease(ctx context.Context, repo string) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VPSCoop-Updater/"+version.Version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func findAsset(rel *ghRelease, name string) *ghAsset {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i]
		}
	}
	return nil
}

func validBinaryMagic(head []byte) bool {
	if len(head) < 4 {
		return false
	}
	switch runtime.GOOS {
	case "linux":
		return head[0] == 0x7f && head[1] == 'E' && head[2] == 'L' && head[3] == 'F'
	case "darwin":
		// Mach-O 64-bit / universal magic
		magic := string(head[:4])
		return magic == "\xcf\xfa\xed\xfe" || magic == "\xfe\xed\xfa\xcf" || magic == "\xca\xfe\xba\xbe"
	case "windows":
		return head[0] == 'M' && head[1] == 'Z'
	default:
		return true
	}
}

func (s *Server) handleUpgradeCheck(w http.ResponseWriter, r *http.Request) {
	repo := getUpdateRepo()
	rel, err := fetchLatestRelease(r.Context(), repo)
	if err != nil {
		writeError(w, http.StatusBadGateway, "检查更新失败: "+err.Error())
		return
	}

	assetName := expectedAssetName()
	asset := findAsset(rel, assetName)
	execPath, execErr := resolveExecPath()

	res := UpgradeCheckResult{
		CurrentVersion: version.Version,
		LatestVersion:  rel.TagName,
		HasUpdate:      isNewerVersion(version.Version, rel.TagName),
		ReleaseName:    rel.Name,
		ReleaseNotes:   rel.Body,
		PublishedAt:    rel.PublishedAt,
		HTMLURL:        rel.HTMLURL,
		AssetName:      assetName,
		ExecPath:       execPath,
		Repo:           repo,
	}

	if asset != nil {
		res.DownloadURL = asset.BrowserDownloadURL
		res.AssetSize = asset.Size
	}

	switch {
	case execErr != nil:
		res.Reason = "无法定位当前可执行文件路径: " + execErr.Error()
	case asset == nil:
		res.Reason = fmt.Sprintf("当前发布版本中未找到适配 %s/%s 的构建产物 (%s)", runtime.GOOS, runtime.GOARCH, assetName)
	case checkDirWritable(execPath) != nil:
		res.Reason = fmt.Sprintf("当前进程对目录 %s 无写入权限，无法直接自更新（请使用 install.sh 或赋予权限）", filepath.Dir(execPath))
	default:
		res.CanSelfUpdate = true
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleUpgradeApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Force bool `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	repo := getUpdateRepo()
	rel, err := fetchLatestRelease(r.Context(), repo)
	if err != nil {
		writeError(w, http.StatusBadGateway, "获取最新版本信息失败: "+err.Error())
		return
	}
	if !body.Force && !isNewerVersion(version.Version, rel.TagName) {
		writeError(w, http.StatusBadRequest, "当前已经是最新版本 ("+version.Version+")")
		return
	}

	assetName := expectedAssetName()
	asset := findAsset(rel, assetName)
	if asset == nil || asset.BrowserDownloadURL == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("未找到适配 %s/%s 的安装包 (%s)", runtime.GOOS, runtime.GOARCH, assetName))
		return
	}

	execPath, err := resolveExecPath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法定位当前程序路径: "+err.Error())
		return
	}
	if err := checkDirWritable(execPath); err != nil {
		writeError(w, http.StatusForbidden, fmt.Sprintf("目录 %s 无写入权限: %v", filepath.Dir(execPath), err))
		return
	}

	dlCtx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	dlReq, err := http.NewRequestWithContext(dlCtx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dlReq.Header.Set("User-Agent", "VPSCoop-Updater/"+version.Version)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(dlReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "下载更新包失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("下载更新包返回 HTTP %d", resp.StatusCode))
		return
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), ".vpscoop-upgrade-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建临时文件失败: "+err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		_ = tmpFile.Close()
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "写入更新包失败: "+err.Error())
		return
	}
	if written < 1024*1024 || (asset.Size > 0 && written != asset.Size) {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("更新包大小校验失败 (期望 %d, 实际 %d)", asset.Size, written))
		return
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(tmpFile, magic); err != nil || !validBinaryMagic(magic) {
		writeError(w, http.StatusBadGateway, "更新包格式校验失败，已中止替换")
		return
	}
	if err := tmpFile.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "设置可执行权限失败: "+err.Error())
		return
	}
	if err := os.Rename(tmpPath, execPath); err != nil {
		var linkErr *os.LinkError
		if errors.As(err, &linkErr) {
			writeError(w, http.StatusInternalServerError, "替换可执行文件失败: "+linkErr.Err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "替换可执行文件失败: "+err.Error())
		return
	}
	cleanupTmp = false

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"version":    rel.TagName,
		"restarting": true,
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	time.AfterFunc(400*time.Millisecond, func() {
		if s.sshMgr != nil {
			s.sshMgr.CloseAll()
		}
		s.mu.RLock()
		st := s.store
		s.mu.RUnlock()
		if st != nil {
			_ = st.Close()
		}
		if s.onRestart != nil {
			s.onRestart(execPath)
		}
	})
}
