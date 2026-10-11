package model

import "time"

type Project struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	URL           string `json:"url"`
	DefaultBranch string `json:"defaultBranch"`
	CurrentUser   *User  `json:"currentUser,omitempty"`
}

type User struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

type Label struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	TextColor string `json:"textColor"`
}

type Approval struct {
	State      string `json:"state"`
	Count      int    `json:"count"`
	CountKnown bool   `json:"countKnown"`
	Required   *int   `json:"required,omitempty"`
	Left       *int   `json:"left,omitempty"`
	Users      []User `json:"users"`
}

type MR struct {
	ID              int       `json:"id"`
	IID             int       `json:"iid"`
	Title           string    `json:"title"`
	URL             string    `json:"url"`
	Author          string    `json:"author"`
	AuthorUser      User      `json:"authorUser"`
	Assignees       []User    `json:"assignees"`
	Labels          []Label   `json:"labels"`
	SourceProjectID int       `json:"sourceProjectId"`
	TargetProjectID int       `json:"targetProjectId"`
	SourceBranch    string    `json:"sourceBranch"`
	TargetBranch    string    `json:"targetBranch"`
	Draft           bool      `json:"draft"`
	CI              string    `json:"ci"`
	Conflict        string    `json:"conflict"`
	Approval        Approval  `json:"approval"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Warnings        []string  `json:"warnings"`
}

type Node struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	ProjectID int    `json:"projectId"`
	MR        *MR    `json:"mr,omitempty"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Cycle  bool   `json:"cycle,omitempty"`
}

type Graph struct {
	Project  Project  `json:"project"`
	Nodes    []Node   `json:"nodes"`
	Edges    []Edge   `json:"edges"`
	Warnings []string `json:"warnings"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
}
