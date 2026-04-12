package main

import (
	"embed"

	"multisnekkvm/internal/bootstrap"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var embeddedTrayIcon []byte

//go:embed build/appicon.png
var embeddedAppIcon []byte

func main() {
	bootstrap.Run(assets, embeddedTrayIcon, embeddedAppIcon)
}
