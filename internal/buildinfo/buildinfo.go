// Package buildinfo holds process-wide build metadata injected at compile
// time via `-ldflags "-X ...=..."`. It exists so build state lives in its
// own package rather than inside an HTTP adapter (internal/adapter/httpapi
// held Version directly until issue #5): a Dockerfile or `go build`
// invocation is the only thing that should set these.
package buildinfo

// Version is the build version (e.g. a git tag or short SHA). Defaults to
// "dev" for local `go run`/`go build` invocations that don't pass -ldflags.
var Version = "dev"

// Commit is the git SHA the binary was built from. Defaults to "unknown"
// when not injected.
var Commit = "unknown"

// BuildTime is the RFC 3339 timestamp the binary was built at. Defaults to
// "unknown" when not injected.
var BuildTime = "unknown"
