package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"strings"
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
	HeartbeatAddress string
	ServingAddress   string
	LastHeartbeat    time.Time
	IsHealthy        bool
	heartBeatConn    net.Conn
	Mutex            sync.Mutex
}

type LoadBalancer struct {
	Servers map[string]*ServerInfo // key is the HeartbeatAddress
	// an index for keeping the index of the servers for the round robin algorithm
	Index   int
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
				logger.Debug("Server is unhealthy", zap.String("address", server.HeartbeatAddress))
				server.IsHealthy = false

				// close the connection
				server.heartBeatConn.Close()

				// remove the server from the list
				delete(lb.Servers, server.HeartbeatAddress)

				logger.Debug("Server removed", zap.String("address", server.HeartbeatAddress))

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
				// remove the port from the address by finding the last colon
				servingAddress := strings.Split(address, ":")[0]
				// add the port from the request
				if port, ok := request["port"]; ok {
					servingAddress += ":" + port.(string)
				} else {
					logger.Error("Port not found in the heartbeat request", zap.Any("request", request))
					//! TODO: implement a mechanism to report the error to the server
					continue
				}
				server := &ServerInfo{
					HeartbeatAddress: address,
					ServingAddress:   servingAddress,
					LastHeartbeat:    time.Now(),
					IsHealthy:        true,
					heartBeatConn:    conn,
				}
				lb.Servers[address] = server
			}
		} else {
			logger.Error("Invalid heartbeat request from server", zap.Any("request", request))
		}
		lb.Mutex.Unlock()
	}
}

// listen for requests from clients
func (lb *LoadBalancer) ListenForRequests() error {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		logger.Error("Error in Listen", zap.Error(err))
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Error("Error in Accept", zap.Error(err))
			continue
		}
		logger.Debug("Client connected", zap.String("address", conn.RemoteAddr().String()))
		go lb.handleRequest(conn)
	}
}

// TODO: There is a time where server is closed yet not removed, thus can be selected. We need to handle this. Maybe fault tolarence?
func (lb *LoadBalancer) handleRequest(conn net.Conn) {
	defer conn.Close()
	request, response := make(map[string]interface{}), make(map[string]interface{})

	clientEncoder := json.NewEncoder(conn)
	clientDecoder := json.NewDecoder(conn)

	if err := clientDecoder.Decode(&request); err != nil {
		logger.Error("Error in decoding request", zap.Error(err))
		sendError(clientEncoder, "Error in decoding the request")
		return
	}

	logger.Debug("Request received from client", zap.Any("request", request))

	server := lb.getServer()
	if server == nil {
		sendError(clientEncoder, "No server available")
		return
	}

	serverConn, err := net.Dial("tcp", server.ServingAddress)
	if err != nil {
		logger.Error("Error connecting to server", zap.Error(err))
		sendError(clientEncoder, "Error in connecting to the server")
		return
	}
	defer serverConn.Close()

	if err := relayJSON(request, serverConn); err != nil {
		logger.Error("Error sending request to server", zap.Error(err))
		sendError(clientEncoder, "Error in relaying request to server")
		return
	}
	logger.Debug("Request sent to server")

	if err := receiveJSON(&response, serverConn); err != nil {
		logger.Error("Error receiving response from server", zap.Error(err))
		sendError(clientEncoder, "Error in receiving response from server")
		return
	}

	logger.Debug("Response received from server", zap.Any("response", response))

	if err := clientEncoder.Encode(response); err != nil {
		logger.Error("Error sending response to client", zap.Error(err))
	}
	logger.Debug("Response sent to client")
}

// Helper function to relay JSON data over a connection
func relayJSON(data interface{}, conn net.Conn) error {
	return json.NewEncoder(conn).Encode(data)
}

// Helper function to receive JSON data from a connection
func receiveJSON(data interface{}, conn net.Conn) error {
	return json.NewDecoder(conn).Decode(data)
}

// Helper function to send an error response to the client
func sendError(encoder *json.Encoder, message string) {
	response := map[string]interface{}{"error": message}
	encoder.Encode(response)
}

// TODO: Implement the load balancing algorithm
func (lb *LoadBalancer) getServer() *ServerInfo {
	// defer func() {
	// 	lb.Index = (lb.Index + 1) % len(lb.Servers)
	// 	lb.Mutex.Unlock()
	// }()

	// currentIndex := 0

	// lb.Mutex.Lock()
	// // check if lb.Index is greater than the length of the servers
	// if lb.Index >= len(lb.Servers) {
	// 	lb.Index = 0
	// }
	// for _, server := range lb.Servers {
	// 	// check if currentIndex is the same as the index
	// 	if currentIndex == lb.Index {
	// 		logger.Debug("Server selected with round robin index", zap.Int("index", lb.Index))
	// 		return server
	// 	} else {
	// 		currentIndex++
	// 	}
	// }

	// return nil

	// RETURNING FIRST SERVER FOR NOW
	for _, server := range lb.Servers {
		return server
	}

	return nil
}

// ! The load balancing algorithm is not implemented yet
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

	// Listen for requests
	go lb.ListenForRequests()

	// wait for the signal to stop
	<-ctx.Done()

	// sleep for 0.5 second
	<-time.After(500 * time.Millisecond)

	logger.Info("Load balancer stopped")
}
