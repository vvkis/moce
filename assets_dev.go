//go:build !production

package main

import "embed"

// assets is empty unless the production build embeds frontend/dist.
var assets embed.FS
