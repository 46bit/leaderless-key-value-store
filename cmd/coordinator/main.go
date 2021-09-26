package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	clusterDesc := NewClusterDescription(coordinatorConfig, connManager)
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
		log.Println("Exiting...")
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
