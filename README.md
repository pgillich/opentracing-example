# opentracing-example
OpenTracing example

# Introduction

It's an example implementation for my article on medium.com.

# Running

## Getting the code

```sh
git clone https://github.com/pgillich/opentracing-example.git
```

## Running in CLI

Compiling the binary (Go 1.18):

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

## Running as unit test

Test cases are in `test/e2e_test.go`.
