package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "usage: <command> <image> <url>")
		os.Exit(-1)
	}

	n := time.Now()
	fmt.Println("> Loading DB into memory")
	db, err := newDb()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading overlay2 database", err.Error())
		os.Exit(1)
	}

	fmt.Println("> Copying layers from overlay2 to", layerFolder)
	manifest, err := generateManifestFromDocker(context.Background(), os.Args[1], db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	fmt.Println("> Startup was done in", time.Since(n))
	p := newPusher(&manifest, 3)
	urlRaw := os.Args[2]
	u, err := url.Parse("http://" + urlRaw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Parsing url", err)
		os.Exit(1)
	}

	path := u.Path
	tagIndex := strings.LastIndex(path, ":")
	domainWithProto := "http://" + u.Host
	if err := p.push(context.Background(), domainWithProto, path[:tagIndex], path[tagIndex+1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pushing image", err.Error())
		os.Exit(1)
	}

	fmt.Println("> Finished in", time.Since(n))
}
