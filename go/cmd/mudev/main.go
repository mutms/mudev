// Command mudev manages git checkouts for MuTMS / Moodle plugin development
// and assembles Moodle test-site code trees for CI. Linux only.
package main

import "github.com/mutms/mudev/go/internal/cli"

// version is stamped at build time via -ldflags.
var version = "dev"

func main() {
	cli.Execute(version)
}
