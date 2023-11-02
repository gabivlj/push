# Push, a fast and simple docker push
`push` is a simple CLI that pushes a local docker image to a repository.


## How does it work
```
# my-image:1.0 should exist in local Docker
push my-image:1.0 my-repo/hello-world:1.0
```

## Support
Right now, it only works in Linux and overlay2, and only tested with docker 24.0.

Authorization is only done with username password with --password-stdin and --username.

## Why?
This is just a thing that I wanted to work on to try to have a 
faster `docker push` that is simpler to maintain, and doesn't 
compress layers.

## Warning
- If specifying compression level: it uses zstd by default, so it might not work for some use-cases, use --compression-algo to change it.
- Only works in Linux with Docker's overlay2, this is very experimental.
