package scaffold

import "io/fs"

type Action string

const (
	Create    Action = "create"
	Overwrite Action = "overwrite"
	Skip      Action = "skip"
	Conflict  Action = "conflict"
)

type Operation struct {
	Path         string      `json:"path"`
	Action       Action      `json:"action"`
	Mode         fs.FileMode `json:"mode"`
	Content      []byte      `json:"-"`
	ExpectedHash string      `json:"-"`
}
