// Package web embeds the built frontend (web/dist, produced by `npm run
// build`) so the Go binary can serve it without external files.
//
// dist/.gitkeep is the only file committed here; real build output is
// gitignored and produced at Docker build time (or by running `npm run
// build` locally). Without it, the server falls back to a "frontend not
// built" response instead of failing to start.
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
