package model

import "context"

const QueueHeaderHost = "X-Queue-Host"
const QueueHeaderMethod = "X-Queue-Method"
const QueueHeaderServer = "X-Queue-Server"
const QueueHeaderClient = "X-Queue-Client"

type MsgRequester interface {
	Request(ctx context.Context, req Request) (*Response, error)
}

type Request struct {
	Queue   string              `json:"queue"`
	Header  map[string][]string `json:"header"`
	Payload []byte              `json:"payload"`
}

type Response struct {
	Header  map[string][]string `json:"header"`
	Payload []byte              `json:"payload"`
	Status  int                 `json:"status"`
	Error   string              `json:"error"`
}

type MsgReceiver interface {
	Receive(ctx context.Context, req Request) (*Response, error)
}
