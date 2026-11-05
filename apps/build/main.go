// CI entry point; shares all packaging with the portable setup executable.
package main

import (
	"fmt"
	"jbl-bar-toolkit/internal/buildtool"
	"os"
)

func main() {
	if e := buildtool.CLI(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "Build:", e)
		os.Exit(1)
	}
}
