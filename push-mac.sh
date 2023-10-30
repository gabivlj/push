VERSION=1.0.0
rm -rf bin
mkdir -p bin
CGO=0 GOOS=linux go build .
id=push-"$VERSION"
mv push bin/$id
IMAGE=push-local-build-"$id":latest
if [[ "$(docker images -q "$IMAGE" 2> /dev/null)" == "" ]]; then
    docker build --build-arg WHERE=bin/"$id" . --tag $IMAGE
fi

output=$(docker run -it --privileged --pid=host justincormack/nsenter1 /usr/bin/find /var/lib/docker/overlay2 -name "$id" -print -quit)
print_command=$(echo $output | tr -d "[:space:]")
docker run -it --rm --privileged --pid=host justincormack/nsenter1 $print_command $1 $2
docker rmi $IMAGE