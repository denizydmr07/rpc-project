
# Build the application from source
FROM golang:1.23.1-alpine AS build-stage

WORKDIR /app

RUN apk add --no-cache python3 py3-pip py3-setuptools py3-wheel bash
RUN ln -sf /usr/bin/python3 /usr/bin/python

COPY client/ /client/
RUN cd /client && go mod download

COPY server/ /server/
RUN cd /server && go mod download

COPY loadbalancer/ /loadbalancer/
RUN cd /loadbalancer && go mod download

COPY ./idl /idl/
COPY ./generator_server_stub/ /generator_server_stub/
COPY ./generator_client_stub/ /generator_client_stub/
COPY scripts/ /scripts/
RUN chmod +x /scripts/*
RUN python3 /scripts/run_generators.py

RUN cd /client && CGO_ENABLED=0 GOOS=linux go build -o /cl
RUN cd /server && CGO_ENABLED=0 GOOS=linux go build -o /sv
RUN cd /loadbalancer && CGO_ENABLED=0 GOOS=linux go build -o /lb

FROM alpine:3.19.4 AS release-stage

WORKDIR /

COPY ./loadbalancer/.env /.env
COPY ./loadbalancer/lb.crt /lb.crt
COPY ./loadbalancer/lb.key /lb.key
COPY --from=build-stage /cl /client
COPY --from=build-stage /sv /server
COPY --from=build-stage /lb /loadbalancer

EXPOSE 7070
EXPOSE 8080