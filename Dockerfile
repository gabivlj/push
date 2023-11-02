FROM golang:1.20 as build

ARG VERSION
COPY . /root/repo
RUN cd /root/repo && go build -o ${VERSION} .
