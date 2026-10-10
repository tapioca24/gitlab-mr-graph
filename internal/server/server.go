package server

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tapioca24/gitlab-mr-graph/internal/graph"
	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

//go:embed web/*
var assets embed.FS

type Loader interface {
	Load(context.Context, model.Project, func([]model.MR, []string)) error
}

type Snapshot struct {
	Graph     model.Graph `json:"graph"`
	UpdatedAt *time.Time  `json:"updatedAt"`
	Loading   bool        `json:"loading"`
	Error     string      `json:"error"`
	Version   string      `json:"version"`
	Demo      bool        `json:"demo"`
	Update    string      `json:"update"`
}

type Service struct {
	mu       sync.RWMutex
	ctx      context.Context
	loader   Loader
	snapshot Snapshot
}

func NewService(ctx context.Context, p model.Project, loader Loader, version string, demo bool) *Service {
	return &Service{ctx: ctx, loader: loader, snapshot: Snapshot{Graph: graph.Build(p, nil), Version: version, Demo: demo}}
}
func (s *Service) Snapshot() Snapshot { s.mu.RLock(); defer s.mu.RUnlock(); return s.snapshot }
func (s *Service) SetUpdate(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Update = message
}
func (s *Service) Refresh() bool {
	s.mu.Lock()
	if s.snapshot.Loading || s.ctx.Err() != nil {
		s.mu.Unlock()
		return false
	}
	s.snapshot.Loading = true
	s.snapshot.Error = ""
	p := s.snapshot.Graph.Project
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
		defer cancel()
		err := s.loader.Load(ctx, p, func(mrs []model.MR, warnings []string) {
			g := graph.Build(p, mrs)
			g.Warnings = append(g.Warnings, warnings...)
			now := time.Now().UTC()
			s.mu.Lock()
			s.snapshot.Graph = g
			s.snapshot.UpdatedAt = &now
			s.mu.Unlock()
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		s.snapshot.Loading = false
		if err != nil {
			s.snapshot.Error = err.Error()
		}
	}()
	return true
}

func NewHandler(s *Service, host string) (http.Handler, error) {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(secret)
	web, _ := fs.Sub(assets, "web")
	fileServer := http.FileServer(http.FS(web))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(s.Snapshot())
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		s.Refresh()
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		data, _ := fs.ReadFile(web, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(strings.ReplaceAll(string(data), "SESSION_TOKEN", token)))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Host != host {
			http.Error(w, "ホストが一致しません", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
			http.Error(w, "別のサイトからは操作できません", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-MR-Graph-Session") != token {
			http.Error(w, "ページを再読み込みしてください", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}
