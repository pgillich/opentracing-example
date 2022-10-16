/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package main

import (
	"context"
	"os"

	"github.com/pgillich/opentracing-example/cmd"
	"github.com/pgillich/opentracing-example/internal"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cmd.Execute(ctx, os.Args[1:], internal.RunServer)
	cancel()
}
