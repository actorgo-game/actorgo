package cconst

import (
	"fmt"
)

const (
	version = "v1.0.6"
)

var logo = `game sever framework @v%s
`

func GetLOGO() string {
	return fmt.Sprintf(logo, Version())
}

func Version() string {
	return version
}

const (
	DOT = "." //ActorPath的分隔符
)
