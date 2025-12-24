FROM golang:1.21.1 AS builder

WORKDIR /usr/src/app

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
      build-essential gcc

COPY ./go.mod ./go.sum ./
COPY ./m3/common ./m3/common/
COPY ./m3/connectors/json_file ./connectors/json_file

RUN cd ./connectors/json_file && go build -o /usr/bin/runtime

FROM ubuntu:22.04

RUN apt update && \
    apt install ca-certificates curl -y && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/bin/runtime /usr/bin/

