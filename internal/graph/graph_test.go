package graph

import (
	"testing"

	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

func mr(id, sourceProject, targetProject int, source, target string) model.MR {
	return model.MR{ID: id, IID: id, SourceProjectID: sourceProject, TargetProjectID: targetProject, SourceBranch: source, TargetBranch: target}
}

func TestStackBranchingAndProjectIdentity(t *testing.T) {
	p := model.Project{ID: 1, DefaultBranch: "main"}
	g := Build(p, []model.MR{mr(1, 1, 1, "a", "main"), mr(2, 1, 1, "b", "a"), mr(3, 1, 1, "c", "a"), mr(4, 2, 1, "a", "main"), mr(5, 1, 1, "d", "release")})
	connections := map[string]bool{}
	for _, e := range g.Edges {
		connections[e.Source+"→"+e.Target] = true
	}
	for _, want := range []string{"branch:1:main→mr:1", "mr:1→mr:2", "mr:1→mr:3", "branch:1:main→mr:4", "branch:1:release→mr:5"} {
		if !connections[want] {
			t.Errorf("missing %s", want)
		}
	}
	if connections["mr:4→mr:2"] || connections["mr:4→mr:3"] {
		t.Fatal("同名の fork ブランチを誤って接続しました")
	}
	positions := map[string]model.Node{}
	for _, n := range g.Nodes {
		positions[n.ID] = n
	}
	if positions["mr:2"].X <= positions["mr:1"].X {
		t.Fatal("上流から下流へ配置されていません")
	}
	if positions["mr:2"].Y == positions["mr:3"].Y {
		t.Fatal("分岐カードが重なっています")
	}
}

func TestCycleAndMissingMetadataRetainAllMRs(t *testing.T) {
	g := Build(model.Project{ID: 1}, []model.MR{mr(1, 1, 1, "a", "b"), mr(2, 1, 1, "b", "a"), mr(3, 0, 0, "", "")})
	count, cycles := 0, 0
	for _, n := range g.Nodes {
		if n.MR != nil {
			count++
		}
	}
	for _, e := range g.Edges {
		if e.Cycle {
			cycles++
		}
	}
	if count != 3 || cycles == 0 || len(g.Warnings) < 2 {
		t.Fatalf("MRs=%d cycles=%d warnings=%v", count, cycles, g.Warnings)
	}
}

func TestEmptyGraphUsesJSONArrays(t *testing.T) {
	g := Build(model.Project{}, nil)
	if g.Nodes == nil || g.Edges == nil || g.Warnings == nil {
		t.Fatal("空グラフは null ではなく配列にします")
	}
}
