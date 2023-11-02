VERSION=1.0.40
id=push-"$VERSION"
GOOS=linux go build -o $id .
IMAGE=push-local-build-"$id":latest
if [[ "$(docker images -q "$IMAGE" 2> /dev/null)" == "" ]]; then
    docker build --build-arg VERSION="$id" . --tag $IMAGE
fi

docker run -i --privileged --pid=host $IMAGE $@