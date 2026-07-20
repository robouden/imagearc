// Package web embeds the FotoArch dark-theme web UI assets.
package web

import "embed"

//go:embed index.html style.css app.js
var Assets embed.FS
