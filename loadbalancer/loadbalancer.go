package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/denizydmr07/zapwrapper/pkg/zapwrapper"
	"go.uber.org/zap"
)

var logger *zap.Logger = zapwrapper.NewLogger(
	zapwrapper.DefaultFilepath,   // Log file path
	zapwrapper.DefaultMaxBackups, // Max number of log files to retain
	zapwrapper.DefaultLogLevel,   // Log level
)

type ServerInfo struct {
	Address       string
	LastHeartbeat time.Time
	IsHealthy     bool
	heartBeatConn net.Conn
	Mutex         sync.Mutex
}

type LoadBalancer struct {
	Servers map[string]*ServerInfo
	Timeout time.Duration
	Mutex   sync.Mutex
}

func NewLoadBalancer(timeout time.Duration) *LoadBalancer {
	return &LoadBalancer{
		Servers: make(map[string]*ServerInfo),
		Timeout: timeout,
	}
}

// MonitorHeartbeats checks the heartbeats of the servers
func (lb *LoadBalancer) MonitorHeartbeats() {
	for {
		time.Sleep(lb.Timeout)
		lb.Mutex.Lock()
		for _, server := range lb.Servers {
			if time.Since(server.LastHeartbeat) > lb.Timeout {
				logger.Debug("Server is unhealthy", zap.String("address", server.Address))
				server.IsHealthy = false

				// close the connection
				server.heartBeatConn.Close()

				// remove the server from the list
				delete(lb.Servers, server.Address)

				logger.Debug("Server removed", zap.String("address", server.Address))

				//TODO: We may need to approach differently, I wrote isHealthy for any case
			}
		}
		lb.Mutex.Unlock()
	}
}

// listen for heartbeats on tcp port 7070
func (lb *LoadBalancer) ListenForHeartbeats() error {
	ln, err := net.Listen("tcp", ":7070")
	if err != nil {
		logger.Error("Error in Listen", zap.Error(err))
		return err
	}
	defer ln.Close()
	logger.Info("Load balancer started")
	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Error("Error in Accept", zap.Error(err))
			continue
		}
		go lb.handleHeartbeat(conn)
	}
}

// server will connect once and send heartbeats periodically
func (lb *LoadBalancer) handleHeartbeat(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	var request map[string]interface{}

	for {
		err := decoder.Decode(&request)
		if err != nil {
			return
		}
		if _, ok := request["heartbeat"]; ok {
			logger.Debug("Received heartbeat from server", zap.String("address", conn.RemoteAddr().String()))
			address := conn.RemoteAddr().String()
			lb.Mutex.Lock()
			if server, ok := lb.Servers[address]; ok {
				server.LastHeartbeat = time.Now()
				server.IsHealthy = true
			} else {
				logger.Debug("New server connected", zap.String("address", address))
				server := &ServerInfo{
					Address:       address,
					LastHeartbeat: time.Now(),
					IsHealthy:     true,
					heartBeatConn: conn,
				}
				lb.Servers[address] = server
			}
		} else {
			logger.Error("Invalid heartbeat request from server", zap.Any("request", request))
		}
		lb.Mutex.Unlock()
	}
}

// TODO: Implement the load balancing algorithm

// ! This main function is just for testing the heartbeats
// ! The load balancing algorithm is not implemented yet
// TODO: Clients will connect to the load balancer and the load balancer will forward the requests to the servers
func main() {
	lb := NewLoadBalancer(5 * time.Second)

	// Channel to listen SIGINT and SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Context to cancel the server
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// listen sigint and sigterm signals
	go func() {
		<-stop
		cancel()
	}()

	// Listen for heartbeats
	go lb.ListenForHeartbeats()

	// Monitor heartbeats
	go lb.MonitorHeartbeats()

	// wait for the signal to stop
	<-ctx.Done()

	// sleep for 0.5 second
	<-time.After(500 * time.Millisecond)

	logger.Info("Load balancer stopped")
}
