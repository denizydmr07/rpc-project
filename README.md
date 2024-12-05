# rpc-project

### Version
go version go1.19 linux/amd64

### Run
1) "go mod tidy" (just at first) inside generator_client_stub and generator_server_stub
2) run "run_generators.py" which creates the stubs under scripts dir
3) "go mod tidy" (just at first) and "go run ." the load balancer under loadbalancer dir
4) "go mod tidy" (just at first) and "go run ." the server under server dir
5) "go mod tidy" (just at first) and "go run ." the client under client dir

### TODO

- [X] Return appropriate error to client when load balancer is down
- [X] Implement load balancing algorithm (round robin)
- [X] Handle edge cases: when server is down, load balancer realises that after 3 missing heartbets. At that interval, client may connect and load balancer may relays the request to the server. But the server is down. In this case, load balancer should return appropriate error to client or select another server.
- [ ] Adding TLS 
- [X] When server is unhealthy (missed 3 heartbets) we are removing it from the list of servers. But should we add it back when it is healthy again? 
- [X] Delete IsHealty variable from server struct 
