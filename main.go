package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "usage: <command> <image>")
		os.Exit(-1)
	}

	db, err := newDb()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading overlay2 database", err.Error())
		os.Exit(1)
	}

	manifest, err := generateManifestFromDocker(context.Background(), os.Args[1], db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	fmt.Println("The manifest:", manifest)
}