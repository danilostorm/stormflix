package webui

import "embed"

// Static contains the lightweight StormFlix web client.
//
//go:embed static/*
var Static embed.FS
