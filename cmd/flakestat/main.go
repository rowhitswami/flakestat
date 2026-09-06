// Command flakestat finds flaky tests in any language, locally.
package main

import (
	"os"

	"github.com/rowhitswami/flakestat/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args, os.Stdout, os.Stderr))
}
