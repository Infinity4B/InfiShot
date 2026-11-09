//go:build e2e

package main

// Lets mygo-automation drive the app in tests: go build -tags e2e, then
// start it with MYGO_AUTOMATION set to a socket path.
import _ "github.com/egoist/mygo/plugins/automation"
