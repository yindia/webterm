package main

import (
	"fmt"
	"os"
	cmd "webterm/cmd"
)

func main() {
	root := cmd.NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
