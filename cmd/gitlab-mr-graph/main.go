package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tapioca24/gitlab-mr-graph/internal/demo"
	"github.com/tapioca24/gitlab-mr-graph/internal/gitlab"
	"github.com/tapioca24/gitlab-mr-graph/internal/model"
	"github.com/tapioca24/gitlab-mr-graph/internal/server"
	"github.com/tapioca24/gitlab-mr-graph/internal/update"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "エラー:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if len(os.Args) > 2 {
			return errors.New("使い方: gitlab-mr-graph update")
		}
		return installUpdate(ctx)
	}
	port := flag.Int("port", 8787, "ローカルサーバーのポート (0: 空きポート)")
	noOpen := flag.Bool("no-open", false, "ブラウザを開かず URL を表示")
	noUpdate := flag.Bool("no-update", false, "ツールの自動更新を無効化")
	host := flag.String("hostname", "", "GitLab ホストを明示指定")
	repo := flag.String("repo", "", "プロジェクトのパスまたは URL (既定: 現在のリポジトリ)")
	useDemo := flag.Bool("demo", false, "認証不要のデモデータを表示")
	demoCount := flag.Int("demo-count", 12, "デモの MR 件数 (1〜1000)")
	showVersion := flag.Bool("version", false, "バージョンを表示")
	flag.Parse()
	if *showVersion {
		fmt.Println("gitlab-mr-graph", version)
		return nil
	}
	if flag.NArg() != 0 {
		return errors.New("不明な引数です。--help を確認してください")
	}
	if *port < 0 || *port > 65535 {
		return errors.New("ポートは 0〜65535 で指定してください")
	}
	if *demoCount < 1 || *demoCount > 1000 {
		return errors.New("デモ件数は 1〜1000 で指定してください")
	}
	var p model.Project
	var loader server.Loader
	if *useDemo {
		p = demo.Project
		loader = demo.Loader{Count: *demoCount}
	} else {
		if _, err := exec.LookPath("glab"); err != nil {
			return errors.New("GitLab CLI (glab) をインストールし、glab auth login で認証してください")
		}
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		projectPath := strings.TrimSuffix(*repo, ".git")
		if strings.Contains(projectPath, "://") {
			u, err := url.Parse(projectPath)
			if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
				return errors.New("--repo の URL が不正です")
			}
			if *host != "" && *host != u.Host {
				return errors.New("--repo と --hostname のホストが一致しません")
			}
			*host = u.Host
			projectPath = strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		}
		if projectPath == "" {
			if err := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir").Run(); err != nil {
				return errors.New("GitLab リポジトリ内で実行するか、--repo group/project --hostname HOST を指定してください")
			}
		}
		c := &gitlab.Client{Run: gitlab.ExecRunner(dir), Hostname: *host, Repo: projectPath}
		fmt.Println("GitLab プロジェクトを確認しています…")
		resolveCtx, resolveCancel := context.WithTimeout(ctx, 25*time.Second)
		p, err = c.Resolve(resolveCtx)
		resolveCancel()
		if err != nil {
			return err
		}
		loader = c
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		explicit := false
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "port" {
				explicit = true
			}
		})
		if explicit {
			return err
		}
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
	}
	defer listener.Close()
	s := server.NewService(ctx, p, loader, version, *useDemo)
	handler, err := server.NewHandler(s, listener.Addr().String())
	if err != nil {
		return err
	}
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	failures := make(chan error, 1)
	go func() { failures <- httpServer.Serve(listener) }()
	s.Refresh()
	if !*noUpdate && !*useDemo && version != "dev" {
		go func() {
			updateCtx, updateCancel := context.WithTimeout(ctx, 90*time.Second)
			defer updateCancel()
			path, err := os.Executable()
			if err != nil {
				s.SetUpdate("更新先を確認できませんでした")
				return
			}
			result, err := (update.Client{}).CheckAndInstall(updateCtx, version, path)
			if err != nil {
				s.SetUpdate("自動更新できませんでした: " + err.Error())
				return
			}
			s.SetUpdate(result.Message)
		}()
	}
	address := "http://" + listener.Addr().String()
	fmt.Printf("%s\n%s\nCtrl-C で終了します。\n", p.Path, address)
	if *useDemo {
		fmt.Println("デモ表示です。実際の GitLab データではありません。")
	}
	if !*noOpen {
		if err := openBrowser(ctx, address); err != nil {
			fmt.Fprintln(os.Stderr, "ブラウザを開けませんでした。上の URL を開いてください。")
		}
	}
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-failures:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func openBrowser(ctx context.Context, address string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return exec.CommandContext(openCtx, command, address).Run()
}

func installUpdate(ctx context.Context) error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	result, err := (update.Client{}).CheckAndInstall(ctx, version, path)
	if err != nil {
		return err
	}
	fmt.Println(result.Message)
	return nil
}
