package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
)

var passwordStdin = flag.Bool("password-stdin", false, "if you want to use a password from stdin")
var username = flag.String("username", "", "if you're authenticating, you should set the username")
var compressionLevel = flag.Int("compression-level", 0, "compresssion level\n gzip: from 0 (no compression at all) to 9 (max compression)\nzstd (default): from 0 (no compression at all) to 3 (max compression)")
var compressionAlgorithm = flag.String("compression-algo", Zstd, "compresssion algorithm to use, can be either gzip or zstd")
var nJobs = flag.Int("push-workers", 3, "number of workers that should be asynchronously running")
var dryRun = flag.Bool("dry-run", false, "If you want to inspect the layers of the image")

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	signalHandler := make(chan os.Signal, 5)
	signal.Notify(signalHandler, os.Interrupt)
	go func() {
		<-signalHandler
		cancel()
	}()

	defer func() {
		cancel()
		close(signalHandler)
	}()

	if len(os.Args) <= 2 {
		fmt.Fprintln(os.Stderr, "usage: <command> <image> <url>")
		os.Exit(-1)
	}

	switch *compressionAlgorithm {
	case Zstd, Gzip:
	default:
		fmt.Fprintln(os.Stderr, "unknown algorithm", *compressionAlgorithm)
		fmt.Fprintln(os.Stderr, "only gzip or zstd are allowed")
		os.Exit(-1)
	}

	flag.CommandLine.Parse(os.Args[3:])
	c := configuration{}
	c.username = *username
	if *passwordStdin {
		r := bufio.NewReader(os.Stdin)
		password, err := r.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Reading password:", err)
			os.Exit(1)
		}

		c.password = strings.TrimSpace(string(password))
	} else if c.username != "" {
		fmt.Fprintln(os.Stderr, "we need --password-stdin if you want authentication")
		os.Exit(1)
	}

	n := time.Now()
	fmt.Println("> Loading DB into memory")
	db, err := newDb()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading overlay2 database", err.Error())
		os.Exit(1)
	}

	fmt.Println("> Copying layers from overlay2 to", layerFolder)
	manifest, err := generateManifestFromDocker(ctx, os.Args[1], db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if *dryRun {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "\t")
		encoder.Encode(manifest)
		return
	}

	fmt.Println("> Startup was done in", time.Since(n))
	p := newPusher(&manifest, *nJobs, &c)
	urlRaw := os.Args[2]
	u, err := url.Parse("http://" + urlRaw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Parsing url", err)
		os.Exit(1)
	}

	path := u.Path
	tagIndex := strings.LastIndex(path, ":")
	domainWithProto := "http://" + u.Host
	if err := p.push(ctx, domainWithProto, path[:tagIndex], path[tagIndex+1:], pushConfiguration{compressionLevel: *compressionLevel, algo: *compressionAlgorithm}); err != nil {
		fmt.Fprintln(os.Stderr, "pushing image", err.Error())
		os.Exit(1)
	}

	fmt.Println("> Finished in", time.Since(n))
}
