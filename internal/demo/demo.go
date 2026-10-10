package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

var Project = model.Project{ID: 1, Name: "Atlas", Path: "platform/atlas", URL: "https://gitlab.com", DefaultBranch: "main"}

type Loader struct{ Count int }

func (l Loader) Load(ctx context.Context, p model.Project, publish func([]model.MR, []string)) error {
	titles := []string{"認証セッションの基盤を整える", "トークンの更新処理を追加", "アカウント画面を接続", "検索インデックスの移行", "検索 API のページング", "検索結果の新しい表示", "通知の配信基盤", "メール通知のテンプレート", "通知設定の画面", "開発環境のセットアップを更新", "セッションの監査イベントを記録", "監査イベントの結合テスト"}
	authors := []string{"sato", "tanaka", "mori", "yamada"}
	count := max(1, l.Count)
	mrs := make([]model.MR, 0, count)
	for i := 0; i < count; i++ {
		id := i + 201
		base := "main"
		if i%3 > 0 {
			base = fmt.Sprintf("change-%d", id-1)
		}
		ci := []string{"success", "running", "success", "failed", "none"}[i%5]
		conflict := "clear"
		if i%11 == 8 {
			conflict = "conflict"
		}
		required, left := 2, 0
		if i%4 == 1 {
			left = 1
		}
		state := "met"
		if left > 0 {
			state = "waiting"
		}
		mrs = append(mrs, model.MR{ID: id, IID: id, Title: titles[i%len(titles)], URL: "https://gitlab.com", Author: authors[i%len(authors)],
			SourceProjectID: 1, TargetProjectID: 1, SourceBranch: fmt.Sprintf("change-%d", id), TargetBranch: base,
			Draft: i%7 == 3, CI: ci, Conflict: conflict, Approval: model.Approval{State: state, Count: required - left, CountKnown: true, Required: &required, Left: &left},
			UpdatedAt: time.Now().Add(-time.Duration(i+1) * time.Hour), Warnings: []string{}})
	}
	// A sibling makes the demo show branching as well as linear stacks.
	if count >= 12 {
		mrs[10].TargetBranch = "change-201"
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	publish(mrs, []string{})
	return nil
}
