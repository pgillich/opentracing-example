# nats-example

NATS example

## Introduction

It's an example implementation for my article at ?
It's based on <https://github.com/pgillich/opentracing-example>.

## Running

## Starting Jaeger a server

```sh
docker run -d --name jaeger -e COLLECTOR_ZIPKIN_HOST_PORT=:9411 -e COLLECTOR_OTLP_ENABLED=true -p 6831:6831/udp -p 6832:6832/udp -p 5778:5778 -p 16686:16686 -p 4317:4317 -p 4318:4318 -p 14250:14250 -p 14268:14268 -p 14269:14269 -p 9411:9411 jaegertracing/all-in-one:1.38
```

## Getting the source code

```sh
git clone https://github.com/pgillich/opentracing-example.git
git checkout nats
```

### Running in CLI

Compiling the binary (Go 1.20):

```sh
go build
```

Example commands to the servers:

```sh
LISTENADDR=127.0.0.1:55501 INSTANCE=backend-1 ./opentracing-example backend --response PONG_1 &
LISTENADDR=127.0.0.1:55502 INSTANCE=backend-2 ./opentracing-example backend --response PONG_2 &
LISTENADDR=127.0.0.1:55500 INSTANCE=frontend ./opentracing-example frontend &
```

Example command to run client:

```sh
SERVER=127.0.0.1:55500 INSTANCE=client-1 ./opentracing-example client http://127.0.0.1:55501/ping http://127.0.0.1:55502/ping http://127.0.0.1:55502/ping
```

Example command to send request to frontend without client:

```sh
curl -X GET http://127.0.0.1:55500/proxy --data-binary 'http://127.0.0.1:55501/ping http://127.0.0.1:55502/ping http://127.0.0.1:55502/ping'
```

### Running as unit test

Test cases are in `test/e2e_test.go`.

### Running in Kubernetes

Below examples are working with <https://github.com/pgillich/kind-on-dev/tree/1.24>.

Add below line to `/etc/hosts` similar to:

```text
172.18.255.128  opentracing-example.kind-01.company.com
```

```sh
make build image
kind load docker-image pgillich/opentracing-example:v0.0.1 --name demo
sync && echo 3 | sudo tee /proc/sys/vm/drop_caches
```

Running servers:

```sh
kubectl apply -k deployments/kustomize/
```

Running client:

```sh
SERVER=opentracing-example.kind-01.company.com INSTANCE=client-1 JAEGERURL=http://jaeger-collector.kind-01.company.com/api/traces ./build/bin/opentracing-example client http://backend:55501/ping http://backend:55501/ping http://backend:55501/ping
```
