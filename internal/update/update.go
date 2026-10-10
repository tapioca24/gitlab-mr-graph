// Package update installs only checksum-verified stable releases, atomically.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const Repository = "tapioca24/gitlab-mr-graph"

type Client struct {
	HTTP    *http.Client
	APIBase string
	OS      string
	Arch    string
}

type Result struct {
	Updated bool
	Version string
	Message string
}
type release struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func version(s string) ([3]int, error) {
	var result [3]int
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return result, errors.New("安定版のバージョン番号ではありません")
	}
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return result, errors.New("不正なバージョン番号です")
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return result, errors.New("安定版のバージョン番号ではありません")
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return result, err
		}
		result[i] = n
	}
	return result, nil
}
func newer(candidate, current string) (bool, error) {
	a, err := version(candidate)
	if err != nil {
		return false, err
	}
	b, err := version(current)
	if err != nil {
		return false, err
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i], nil
		}
	}
	return false, nil
}

func (c Client) fetch(ctx context.Context, address string, limit int64) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("更新 URL は HTTPS が必要です")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	// Even an HTTPS asset must not redirect into an unencrypted download.
	copy := *client
	previous := copy.CheckRedirect
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("HTTP への更新リダイレクトは許可されません")
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("更新のリダイレクトが多すぎます")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gitlab-mr-graph")
	resp, err := copy.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新の取得に失敗しました (HTTP %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("更新ファイルがサイズ上限を超えました")
	}
	return data, nil
}

func (c Client) CheckAndInstall(ctx context.Context, current, path string) (Result, error) {
	if _, err := version(current); err != nil {
		return Result{Message: "開発ビルドでは自動更新しません"}, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Result{}, err
	}
	path = resolved
	original, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	lockPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".update.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		// A check is bounded by 90 seconds; an old lock is from an interrupted run.
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > 10*time.Minute {
			if removeErr := os.Remove(lockPath); removeErr == nil {
				lock, err = os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			}
		}
	}
	if err != nil {
		return Result{}, fmt.Errorf("更新用ファイルを作成できません (別の更新中、またはインストール先に書き込み権限がありません): %w", err)
	}
	lock.Close()
	defer os.Remove(lockPath)
	api := c.APIBase
	if api == "" {
		api = "https://api.github.com"
	}
	data, err := c.fetch(ctx, api+"/repos/"+Repository+"/releases/latest", 2<<20)
	if err != nil {
		return Result{}, err
	}
	var r release
	if err := json.Unmarshal(data, &r); err != nil {
		return Result{}, err
	}
	if r.Draft || r.Prerelease {
		return Result{Message: "安定版の更新はありません"}, nil
	}
	yes, err := newer(r.Tag, current)
	if err != nil {
		return Result{}, err
	}
	if !yes {
		return Result{Version: current, Message: "最新版です"}, nil
	}
	osName, arch := c.OS, c.Arch
	if osName == "" {
		osName = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	name := fmt.Sprintf("gitlab-mr-graph_%s_%s_%s", r.Tag, osName, arch)
	var assetURL, checksumURL string
	for _, a := range r.Assets {
		if a.Name == name {
			assetURL = a.URL
		}
		if a.Name == "checksums.txt" {
			checksumURL = a.URL
		}
	}
	if assetURL == "" || checksumURL == "" {
		return Result{}, errors.New("この OS・CPU 向けの更新ファイルまたはチェックサムがありません")
	}
	checksums, err := c.fetch(ctx, checksumURL, 1<<20)
	if err != nil {
		return Result{}, err
	}
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			expected = strings.ToLower(fields[0])
			break
		}
	}
	if len(expected) != 64 {
		return Result{}, errors.New("更新ファイルのチェックサムがありません")
	}
	binary, err := c.fetch(ctx, assetURL, 64<<20)
	if err != nil {
		return Result{}, err
	}
	actual := sha256.Sum256(binary)
	if hex.EncodeToString(actual[:]) != expected {
		return Result{}, errors.New("更新ファイルのチェックサムが一致しません")
	}
	now, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	if !os.SameFile(original, now) || !original.ModTime().Equal(now.ModTime()) || original.Size() != now.Size() {
		return Result{}, errors.New("確認中に実行ファイルが変わりました。次回起動で確認します")
	}
	if err := Install(path, binary); err != nil {
		return Result{}, err
	}
	return Result{Updated: true, Version: r.Tag, Message: r.Tag + " を取得しました。次回起動から使えます"}, nil
}

func Install(path string, data []byte) error {
	if len(data) == 0 {
		return errors.New("更新ファイルが空です")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("インストール先が通常ファイルではありません")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".mr-graph-update-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
