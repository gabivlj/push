FROM golang:1.20 as build

ARG VERSION
COPY . /root/repo
RUN rm -rf /root/repo/nsenter
RUN cd /root/repo && go build -o ${VERSION} .

FROM alpine:edge as build2
RUN apk update && apk add build-base
COPY nsenter/nsenter1.c ./
RUN cc -Wall -static nsenter1.c -o /usr/bin/nsenter1

FROM alpine:edge 
COPY --from=build2 /usr/bin/nsenter1 /usr/bin/nsenter1
ARG VERSION
COPY --from=build /root/repo /root/${VERSION}
ENV PUSH_VERSION=${VERSION}
ENTRYPOINT ["/usr/bin/nsenter1"]