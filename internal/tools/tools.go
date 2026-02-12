//go:build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/matryer/moq"
	_ "mvdan.cc/gofumpt"
)
