package main

import (
	"os"

	"github.com/ascarter/gh-tool/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
