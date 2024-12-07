package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/denizydmr07/zapwrapper/pkg/zapwrapper"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

var logger *zap.Logger = zapwrapper.NewLogger(
	zapwrapper.DefaultFilepath,   // Log file path
	zapwrapper.DefaultMaxBackups, // Max number of log files to retain
	zapwrapper.DefaultLogLevel,   // Log level
)

type ServerInfo struct {
	HeartbeatAddress string     // address which server sends heartbeats
	ServingAddress   string     // address which server serves
	LastHeartbeat    time.Time  // last  time the server sent a heartbeat
	IsHealthy        bool       // is the server healthy
	heartBeatConn    net.Conn   // connection which server sends heartbeats from HeartbeatAddress
	Mutex            sync.Mutex // mutex to lock the server
}

type LoadBalancer struct {
	Servers         map[string]*ServerInfo // key is the HeartbeatAddress
	ServerKeys      []string               // keys of the Servers map to get the server in round-robin fashion
	RoundRobinIndex int                    // last index of the ServerKeys to get the server in round-robin fashion
	Timeout         time.Duration          // timeout to consider a server unhealthy
	Mutex           sync.Mutex             // mutex to lock the LoadBalancer
}

// NewLoadBalancer creates a new LoadBalancer with the given timeout
func NewLoadBalancer(timeout time.Duration) *LoadBalancer {
	return &LoadBalancer{
		Servers:    make(map[string]*ServerInfo),
		ServerKeys: []string{},
		Timeout:    timeout,
	}
}

// MonitorHeartbeats checks the heartbeats of the servers
// works in a separate goroutine
func (lb *LoadBalancer) MonitorHeartbeats() {
	for { // infinite loop
		time.Sleep(lb.Timeout) // sleep for the timeout duration
		lb.Mutex.Lock()

		// for each server
		for _, server := range lb.Servers {

			// if the server's last heartbeat is older than the timeout
			if time.Since(server.LastHeartbeat) > lb.Timeout {
				logger.Debug("Server is unhealthy", zap.String("address", server.HeartbeatAddress))
				server.IsHealthy = false // mark the server as unhealthy

				// close the connection
				server.heartBeatConn.Close()

				// remove the server from the list
				delete(lb.Servers, server.HeartbeatAddress)

				// remove the server from the keys
				for i, key := range lb.ServerKeys {
					if key == server.HeartbeatAddress {
						lb.ServerKeys = append(lb.ServerKeys[:i], lb.ServerKeys[i+1:]...)
						break
					}
				}

				logger.Debug("Server removed", zap.String("address", server.HeartbeatAddress))

				//TODO: We may need to approach differently, I wrote isHealthy for any case
			}
		}
		lb.Mutex.Unlock()
	}
}

// ListenForHeartbeats listens for heartbeats from the servers on port 7070
func (lb *LoadBalancer) ListenForHeartbeats(LB_HB_ADDRESS string) error {
	ln, err := net.Listen("tcp", LB_HB_ADDRESS)
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

// handleHeartbeat handles the heartbeat from a server .
// a server connects to the load balancer on port 7070 and sends a heartbeat periodically.
// the first heartbeat contains the port on which the server is serving.
// persistent connection is used to send heartbeats.
func (lb *LoadBalancer) handleHeartbeat(conn net.Conn) {
	decoder := json.NewDecoder(conn)
	var request map[string]interface{}

	for { // infinite loop

		// decode the request
		err := decoder.Decode(&request)
		if err != nil {
			return
		}

		// if the request contains a heartbeat
		if _, ok := request["heartbeat"]; ok {
			logger.Debug("Received heartbeat from server", zap.String("address", conn.RemoteAddr().String()))
			address := conn.RemoteAddr().String()
			lb.Mutex.Lock()

			// if the server is already in the list
			if server, ok := lb.Servers[address]; ok {
				server.LastHeartbeat = time.Now()
				server.IsHealthy = true
			} else { // if the server is not in the list

				logger.Debug("New server connected", zap.String("address", address))

				// remove the port from the address by finding the last colon
				servingAddress := strings.Split(address, ":")[0]

				// add the port from the request
				if port, ok := request["port"]; ok { // if the port is found in the request
					servingAddress += ":" + port.(string)
				} else {
					logger.Error("Port not found in the heartbeat request", zap.Any("request", request))
					//! TODO: implement a mechanism to report the error to the server
					continue
				}

				// create a new server
				server := &ServerInfo{
					HeartbeatAddress: address,
					ServingAddress:   servingAddress,
					LastHeartbeat:    time.Now(),
					IsHealthy:        true,
					heartBeatConn:    conn,
				}

				// add the server to the map
				lb.Servers[address] = server

				// add the server to the keys slice
				lb.ServerKeys = append(lb.ServerKeys, address)
			}
		} else {
			logger.Error("Invalid heartbeat request from server", zap.Any("request", request))
		}
		lb.Mutex.Unlock()
	}
}

// ListenForRequests listens for requests from the clients on port 8080
func (lb *LoadBalancer) ListenForRequests(LB_CLIENT_ADDRESS string, tlsConfig *tls.Config) error {
	//ln, err := net.Listen("tcp", LB_CLIENT_ADDRESS)
	ln, err := tls.Listen("tcp", LB_CLIENT_ADDRESS, tlsConfig)
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

// handleRequest handles the request from a client.
// the request is relayed to a server and the response is sent back to the client.
// the server is selected using the load balancing algorithm.
func (lb *LoadBalancer) handleRequest(conn net.Conn) {
	defer conn.Close()

	// request and response maps
	request, response := make(map[string]interface{}), make(map[string]interface{})

	// encoder and decoder for the client connection
	clientEncoder := json.NewEncoder(conn)
	clientDecoder := json.NewDecoder(conn)

	// decode the request from the client
	if err := clientDecoder.Decode(&request); err != nil {
		logger.Error("Error in decoding request", zap.Error(err))
		sendError(clientEncoder, "Error in decoding the request")
		return
	}

	logger.Debug("Request received from client", zap.Any("request", request))

getServer:
	// get the server using the load balancing algorithm
	server := lb.getServer()
	if server == nil {
		sendError(clientEncoder, "No server available")
		return
	}

	// connect to the server server selected
	serverConn, err := net.Dial("tcp", server.ServingAddress)
	if err != nil {
		logger.Error("Error connecting to server", zap.Error(err))

		if _, ok := err.(*net.OpError); ok {
			// this mean tcp dial error, thus server is down yet not removed
			// we need to get a new server
			logger.Debug("Server is down, getting a new server")
			goto getServer
		} else {
			sendError(clientEncoder, "Error in connecting to server")
		}
		return
	}
	defer serverConn.Close()

	// relay the request to the server
	if err := relayJSON(request, serverConn); err != nil {
		logger.Error("Error sending request to server", zap.Error(err))
		sendError(clientEncoder, "Error in relaying request to server")
		return
	}
	logger.Debug("Request sent to server")

	// receive the response from the server
	if err := receiveJSON(&response, serverConn); err != nil {
		logger.Error("Error receiving response from server", zap.Error(err))
		sendError(clientEncoder, "Error in receiving response from server")
		return
	}

	logger.Debug("Response received from server", zap.Any("response", response))

	// send the response to the client
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
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	// if there are no servers
	if len(lb.ServerKeys) == 0 {
		return nil
	}

	// if the round robin index is greater than the number of servers
	if lb.RoundRobinIndex >= len(lb.ServerKeys) {
		lb.RoundRobinIndex = 0
	}
	logger.Debug("Round robin index", zap.Int("index", lb.RoundRobinIndex))

	// get the server using the round robin index
	server := lb.Servers[lb.ServerKeys[lb.RoundRobinIndex]]

	// increment the round robin index
	lb.RoundRobinIndex++

	logger.Debug("Selected server", zap.String("address", server.ServingAddress))
	return server
}

func main() {
	err := godotenv.Load()
	if err != nil {
		logger.Error("Error loading .env file", zap.Error(err))
	}

	LB_HB_ADDRESS := os.Getenv("LB_HB_ADDRESS")
	LB_CLIENT_ADDRESS := os.Getenv("LB_CLIENT_ADDRESS")

	if LB_HB_ADDRESS == "" || LB_CLIENT_ADDRESS == "" {
		logger.Error("LB_HB_ADDRESS or LB_CLIENT_ADDRESS is not set")
		return
	}

	cert, err := tls.LoadX509KeyPair("lb.crt", "lb.key")
	if err != nil {
		logger.Error("Error loading certificate", zap.Error(err))
		return
	}

	// creare config for tls
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Create a new load balancer with a timeout
	timeout := 1*time.Second + 200*time.Millisecond
	lb := NewLoadBalancer(timeout)

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
	go lb.ListenForHeartbeats(LB_HB_ADDRESS)

	// Monitor heartbeats
	go lb.MonitorHeartbeats()

	// Listen for requests
	go lb.ListenForRequests(LB_CLIENT_ADDRESS, tlsConfig)

	// wait for the signal to stop
	<-ctx.Done()

	// sleep for 0.5 second
	<-time.After(500 * time.Millisecond)

	logger.Info("Load balancer stopped")
}
