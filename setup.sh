#!/bin/bash

docker network create students-net

cd otel && docker build -t otel-collector . && cd -
docker run -d -p 14268:14268 -p 4317:4317 -p 4318:4318  --privileged --name otel-collector --net=students-net localhost/otel-collector:latest

docker run -d \
  -e POSTGRES_USER=go \
  -e POSTGRES_PASSWORD=your-strong-pass \
  -e POSTGRES_DB=go \
  --name students-db \
  --net=students-net \
  postgres:11.5

docker build -t students-app .

docker run -d -p 8080:8080 \
      -e DB_PASS='your-strong-pass' \
      --net=students-net students-app



