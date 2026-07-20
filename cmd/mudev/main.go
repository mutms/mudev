// Command mudev manages git checkouts for MuTMS / Moodle plugin development
// and assembles Moodle test-site code trees for CI. Linux only.
package main

import "github.com/mutms/mudev/internal/cli"

func main() {
	cli.Execute()
}
