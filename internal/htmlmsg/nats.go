package htmlmsg

import (
	"fmt"
	"time"

	nats_server "github.com/nats-io/nats-server/v2/server"
	nats_test "github.com/nats-io/nats-server/v2/test"
)

const NatsHeaderStatus = "X-Nats-Status"
const NatsHeaderError = "X-Nats-Error"

// NatsRunServerCallback is an adapted github.com/nats-io/nats-server/v2/test/test.go:RunServerCallback
// Only for testing
func NatsRunServerCallback(opts *nats_server.Options, callback func(*nats_server.Server)) *nats_server.Server {
	if opts == nil {
		opts = &nats_test.DefaultTestOptions
	}
	s, err := nats_server.NewServer(opts)
	if err != nil || s == nil {
		panic(fmt.Sprintf("No NATS Server object returned: %v", err))
	}

	if !opts.NoLog {
		s.ConfigureLogger()
	}

	if callback != nil {
		callback(s)
	}

	// Run server in Go routine.
	go s.Start()

	// Wait for accept loop(s) to be started
	if !s.ReadyForConnections(1 * time.Second) {
		panic("Unable to start NATS Server in Go Routine")
	}

	return s
}
