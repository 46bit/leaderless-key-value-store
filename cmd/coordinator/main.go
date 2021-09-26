package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc/keepalive"

	. "github.com/46bit/leaderless-key-value-store"
	"github.com/46bit/leaderless-key-value-store/api"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	coordinatorConfig, err := LoadCoordinatorConfig(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	connManager := NewConnManager(coordinatorConfig.SizeOfConnectionPools, coordinatorConfig.RemoveUnusedConnectionPoolsAfter)
	go connManager.Run(ctx)

	clusterDesc := NewClusterDescription(coordinatorConfig)
	clusterServer := NewClusterServer(clusterDesc)

	if coordinatorConfig.DnsServiceDiscovery != nil {
		go PerformDnsServiceDiscovery(ctx, coordinatorConfig.DnsServiceDiscovery, clusterDesc)
	}

	grpcServer := NewGrpcServer()
	api.RegisterClusterServer(grpcServer, clusterServer)

	exitSignals := make(chan os.Signal, 1)
	signal.Notify(exitSignals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-exitSignals
		cancel()
		// FIXME: Make a way to be sure that things have closed down cleanly, before
		// exiting. In particular so BadgerDb is closed cleanly.
		time.Sleep(5 * time.Second)
		grpcServer.GracefulStop()
	}()

	c, err := net.Listen("tcp", coordinatorConfig.BindAddress)
	if err != nil {
		log.Fatal(err)
	}
	if err := grpcServer.Serve(c); err != nil {
		log.Fatal(err)
	}
}

var kaep = keepalive.EnforcementPolicy{
	MinTime:             5 * time.Second, // If a client pings more than once every 5 seconds, terminate the connection
	PermitWithoutStream: true,            // Allow pings even when there are no active streams
}
var kasp = keepalive.ServerParameters{
	MaxConnectionIdle:     15 * time.Second, // If a client is idle for 15 seconds, send a GOAWAY
	MaxConnectionAge:      30 * time.Second, // If any connection is alive for more than 30 seconds, send a GOAWAY
	MaxConnectionAgeGrace: 5 * time.Second,  // Allow 5 seconds for pending RPCs to complete before forcibly closing connections
	Time:                  5 * time.Second,  // Ping the client if it is idle for 5 seconds to ensure the connection is still active
	Timeout:               1 * time.Second,  // Wait 1 second for the ping ack before assuming the connection is dead
}
