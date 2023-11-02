VERSION=1.0.0
id=push-"$VERSION"
IMAGE=push-local-build-"$id":latest
if [[ "$(docker images -q "$IMAGE" 2> /dev/null)" == "" ]]; then
    docker build --build-arg VERSION="$id" . --tag $IMAGE
fi

output=$(docker run -it --privileged --pid=host justincormack/nsenter1 /usr/bin/find /var/lib/docker/overlay2 -name "$id" -print -quit)
print_command=$(echo $output | tr -d "[:space:]")
echo $print_command
docker run -it --rm --privileged --pid=host --net=host justincormack/nsenter1 $print_command $@
docker rmi $IMAGE