# Push, a fast and simple docker push
`push` is a simple CLI that pushes a local docker image to a repository.


## How does it work
```
# my-image:1.0 should exist in local Docker
push my-image:1.0 my-repo/hello-world:1.0
```

## Support
Right now, it only works in Linux and overlay2, and only tested with docker 24.0.

No authentication is implemented yet, that is in the roadmap soon.

## Why?
This is just a thing that I wanted to work on to try to have a 
faster `docker push` that is simpler to maintain, and doesn't 
compress layers.

## Warning
This is experimental and mostly to learn more about how Docker stores layers, use at your own risk.