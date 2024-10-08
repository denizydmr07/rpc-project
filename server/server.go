package main

import (
	"net"

	"github.com/denizydmr07/zapwrapper/pkg/zapwrapper"
	"go.uber.org/zap"

	"github.com/denizydmr07/rpc-project/server/stub"
)

func main() {
	logger := zapwrapper.NewLogger(
		zapwrapper.DefaultFilepath,   // Log file path
		zapwrapper.DefaultMaxBackups, // Max number of log files to retain
		zapwrapper.DefaultLogLevel,   // Log level
	)

	defer logger.Sync() // Flush any buffered log entries

	// Start the server
	func() {
		ln, err := net.Listen("tcp", ":8080")
		logger.Info("Server started")
		if err != nil {
			logger.Error("Error in Listen", zap.Error(err))
			return
		}
		defer ln.Close()

		for {
			conn, err := ln.Accept()
			if err != nil {
				logger.Error("Error in Accept", zap.Error(err))
				return
			}

			logger.Info("Client connected", zap.String("address", conn.RemoteAddr().String()))
			go stub.HandleConnection(conn)
		}
	}()
}
