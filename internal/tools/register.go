// Package tools is the list of tools built into the shell.
//
// To add a tool:
//  1. Implement toolhost.Tool under internal/tools/<id>.
//  2. Append it in All.
//  3. Either leave Meta.Panel empty and handle action "run" (the shell draws a form from Schema),
//     or set Panel to a key and register a matching component in frontend/src/tools/registry.ts.
package tools

import (
	"moce/internal/toolhost"
	"moce/internal/tools/txt2epub"
)

// All returns the tools shipped with this build, in sidebar order.
func All() []toolhost.Tool {
	return []toolhost.Tool{
		txt2epub.New(),
	}
}
