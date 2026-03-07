package main

import (
	"os"
	"testing"
)

func TestMainVersion(t *testing.T) {
	prev := os.Args
	defer func() { os.Args = prev }()
	os.Args = []string{"webterm", "version"}
	main()
}

func TestMainDefaultHelp(t *testing.T) {
	prev := os.Args
	defer func() { os.Args = prev }()
	os.Args = []string{"webterm"}
	main()
}
