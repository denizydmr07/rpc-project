package main

import (
	"github.com/denizydmr07/zapwrapper/pkg/zapwrapper"
	"go.uber.org/zap"

	"github.com/denizydmr07/rpc-project/client/stub"
)

func main() {
	logger := zapwrapper.NewLogger(
		zapwrapper.DefaultFilepath,   // Log file path
		zapwrapper.DefaultMaxBackups, // Max number of log files to retain
		zapwrapper.DefaultLogLevel,   // Log level
	)

	defer logger.Sync() // Flush any buffered log entries
	logger.Info("Client started")

	result, err := stub.Add(1, 2)
	if err != nil {
		logger.Error("Error in Add", zap.Error(err))
	}
	logger.Info("Add result", zap.Float64("result", result))

	result, err = stub.Sub(1, 2)
	if err != nil {
		logger.Error("Error in Sub", zap.Error(err))
	}
	logger.Info("Sub result", zap.Float64("result", result))
}
