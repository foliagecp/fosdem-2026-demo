FROM golang:1.21.1 AS builder

WORKDIR /usr/src/app

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
      build-essential gcc

COPY ./go.mod ./go.sum ./
COPY ./m1/common ./m1/common/
COPY ./m1/connectors/virtual_machine ./connectors/virtual_machine

RUN cd ./connectors/virtual_machine && go build -o /usr/bin/runtime

FROM ubuntu:22.04

RUN apt update && \
    apt install ca-certificates curl -y && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/bin/runtime /usr/bin/
