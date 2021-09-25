package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	. "github.com/46bit/leaderless-key-value-store"
	"github.com/46bit/leaderless-key-value-store/api"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	coordinatorConfig, err := LoadCoordinatorConfig(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}

	clusterDesc := NewClusterDescription(coordinatorConfig)
	clusterServer := NewClusterServer(clusterDesc)

	if coordinatorConfig.DnsServiceDiscovery != nil {
		go PerformDnsServiceDiscovery(ctx, coordinatorConfig.DnsServiceDiscovery, clusterDesc)
	}

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(64<<20),
		grpc.MaxSendMsgSize(64<<20),
	)
	api.RegisterClusterServer(grpcServer, clusterServer)

	exitSignals := make(chan os.Signal, 1)
	signal.Notify(exitSignals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-exitSignals
		cancel()
		os.Exit(0)
	}()

	c, err := net.Listen("tcp", coordinatorConfig.BindAddress)
	if err != nil {
		log.Fatal(err)
	}
	if err := grpcServer.Serve(c); err != nil {
		log.Fatal(err)
	}
}
