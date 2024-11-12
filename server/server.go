package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/denizydmr07/zapwrapper/pkg/zapwrapper"
	"go.uber.org/zap"

	"github.com/denizydmr07/rpc-project/server/stub"
)

func main() {
	portPtr := flag.String("p", "8081", "Port to listen")

	flag.Parse()

	logger := zapwrapper.NewLogger(
		zapwrapper.DefaultFilepath,   // Log file path
		zapwrapper.DefaultMaxBackups, // Max number of log files to retain
		zapwrapper.DefaultLogLevel,   // Log level
	)

	defer logger.Sync() // Flush any buffered log entries

	// channel to detect if the load balancer is down
	lbDown := make(chan struct{})

	// Channel to listen SIGINT and SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Context to cancel the server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen on port 8080
	ln, err := net.Listen("tcp", ":"+*portPtr)
	if err != nil {
		logger.Error("Error in Listen", zap.Error(err))
		return
	}
	defer ln.Close()
	logger.Info("Server started")

	// Start the server
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					logger.Error("Error in Accept", zap.Error(err))
					continue
				}
			}

			logger.Info("Client connected", zap.String("address", conn.RemoteAddr().String()))
			go stub.HandleConnection(conn)
		}
	}()

	//? Would it violate the RPC principles if the server sends heartbeats to the load balancer explicitly?
	go stub.SendHeartbeats(lbDown, *portPtr)

	// waiting for the load balancer to go down or the server to receive a signal
	select {
	case <-lbDown:
		logger.Error("Load balancer is down")
	case <-stop:
		logger.Info("Received signal to stop")
	}

	// Stop the server
	cancel()

	// waiting 1 second
	<-time.After(1 * time.Second)

	// Close the listener
	ln.Close()
	logger.Info("Server stopped")
}
