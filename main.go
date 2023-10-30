package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

var passwordStdin = flag.Bool("password-stdin", false, "if you want to use a password from stdin")
var username = flag.String("username", "", "if you're authenticating, you should set the username")

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "usage: <command> <image> <url>")
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
	manifest, err := generateManifestFromDocker(context.Background(), os.Args[1], db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	fmt.Println("> Startup was done in", time.Since(n))
	p := newPusher(&manifest, 3, &c)
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
