VERSION=1.0.0
id=push-"$VERSION"
IMAGE=push-local-build-"$id":latest
if [[ "$(docker images -q "$IMAGE" 2> /dev/null)" == "" ]]; then
    docker build --build-arg VERSION="$id" . --tag $IMAGE
fi

docker run -it --rm --privileged --pid=host --net=host $IMAGE $@
docker rmi $IMAGE