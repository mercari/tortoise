// Read about tools here: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
//go:build tools

package tools

import (
	_ "github.com/daixiang0/gci"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
