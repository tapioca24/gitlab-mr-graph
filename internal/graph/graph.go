// Package graph builds a snapshot of current branch relationships without API access.
package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

const (
	Column = 344
	Row    = 180
)

func MRID(id int) string                         { return fmt.Sprintf("mr:%d", id) }
func branchID(project int, branch string) string { return fmt.Sprintf("branch:%d:%s", project, branch) }

func Build(project model.Project, input []model.MR) model.Graph {
	g := model.Graph{Project: project, Nodes: []model.Node{}, Edges: []model.Edge{}, Warnings: []string{}}
	mrs := append([]model.MR(nil), input...)
	sort.Slice(mrs, func(i, j int) bool { return mrs[i].IID < mrs[j].IID })
	heads := map[string][]string{}
	seen := map[int]bool{}
	for _, mr := range mrs {
		if seen[mr.ID] {
			continue
		}
		seen[mr.ID] = true
		copy := mr
		g.Nodes = append(g.Nodes, model.Node{ID: MRID(mr.ID), Kind: "mr", Label: mr.Title, MR: &copy, ProjectID: mr.TargetProjectID})
		if mr.SourceProjectID > 0 && mr.SourceBranch != "" {
			key := branchID(mr.SourceProjectID, mr.SourceBranch)
			heads[key] = append(heads[key], MRID(mr.ID))
		}
	}
	branches := map[string]bool{}
	for _, node := range append([]model.Node(nil), g.Nodes...) {
		mr := node.MR
		if mr.TargetProjectID <= 0 || mr.TargetBranch == "" {
			g.Warnings = append(g.Warnings, fmt.Sprintf("!%d のマージ先情報が不足しています", mr.IID))
			continue
		}
		key := branchID(mr.TargetProjectID, mr.TargetBranch)
		parents := heads[key]
		if len(parents) == 0 {
			if !branches[key] {
				kind := "branch"
				if mr.TargetProjectID == project.ID && mr.TargetBranch == project.DefaultBranch {
					kind = "root"
				}
				g.Nodes = append(g.Nodes, model.Node{ID: key, Kind: kind, Label: mr.TargetBranch, ProjectID: mr.TargetProjectID})
				branches[key] = true
			}
			parents = []string{key}
		}
		if len(parents) > 1 {
			g.Warnings = append(g.Warnings, fmt.Sprintf("!%d のマージ先に複数の MR が対応しています", mr.IID))
		}
		for _, parent := range parents {
			g.Edges = append(g.Edges, model.Edge{ID: parent + "→" + node.ID, Source: parent, Target: node.ID})
		}
	}
	layout(&g)
	return g
}

// Layout uses a deterministic spanning forest. Every relationship is still drawn;
// cyclic edges are identified individually rather than dropping whole MR nodes.
func layout(g *model.Graph) {
	index := map[string]int{}
	for i, n := range g.Nodes {
		index[n.ID] = i
	}
	children := map[string][]string{}
	for _, e := range g.Edges {
		children[e.Source] = append(children[e.Source], e.Target)
	}
	for k := range children {
		sort.Strings(children[k])
	}
	color := map[string]int{}
	parent := map[string]string{}
	forest := map[string][]string{}
	var visit func(string)
	visit = func(id string) {
		color[id] = 1
		for _, next := range children[id] {
			if color[next] == 1 {
				for i := range g.Edges {
					if g.Edges[i].Source == id && g.Edges[i].Target == next {
						g.Edges[i].Cycle = true
					}
				}
				continue
			}
			if color[next] == 0 {
				parent[next] = id
				forest[id] = append(forest[id], next)
				visit(next)
			}
		}
		color[id] = 2
	}
	var ids []string
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := g.Nodes[index[ids[i]]], g.Nodes[index[ids[j]]]
		if (a.MR == nil) != (b.MR == nil) {
			return a.MR == nil
		}
		if a.MR != nil {
			return a.MR.IID < b.MR.IID
		}
		return strings.Compare(a.Label, b.Label) < 0
	})
	for _, id := range ids {
		if color[id] == 0 {
			visit(id)
		}
	}
	for _, e := range g.Edges {
		if e.Cycle {
			g.Warnings = append(g.Warnings, "循環するブランチ関係があります。破線で表示しています")
			break
		}
	}
	row := 0
	var place func(string, int) int
	place = func(id string, depth int) int {
		n := &g.Nodes[index[id]]
		n.X = 40 + depth*Column
		if len(forest[id]) == 0 {
			n.Y = 48 + row*Row
			row++
			return n.Y
		}
		first, last := 0, 0
		for i, child := range forest[id] {
			y := place(child, depth+1)
			if i == 0 {
				first = y
			}
			last = y
		}
		n.Y = (first + last) / 2
		return n.Y
	}
	for _, id := range ids {
		if _, ok := parent[id]; !ok {
			place(id, 0)
			row++
		}
	}
	g.Width, g.Height = 640, 400
	for _, n := range g.Nodes {
		g.Width = max(g.Width, n.X+344)
		g.Height = max(g.Height, n.Y+210)
	}
}
