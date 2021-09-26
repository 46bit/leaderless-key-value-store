package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	. "github.com/46bit/leaderless-key-value-store"
	"github.com/46bit/leaderless-key-value-store/api"
)

func main() {
	storageNodeConfig, err := LoadStorageNodeConfig(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	// FIXME: Support persistent disk locations in config
	storage, err := NewStorage(storageNodeConfig.BadgerDbFolder)
	if err != nil {
		log.Fatal(fmt.Errorf("error initialising db: %w", err))
	}
	if storageNodeConfig.BindMetricsAddress == "" {
		log.Println("Not starting metrics server because BindMetricsAddress not configured")
	} else {
		SetupBadgerStorageMetrics()
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			// FIXME: Stop using fatal, everywhere. Shutdown badger gracefully!
			log.Fatal(http.ListenAndServe(storageNodeConfig.BindMetricsAddress, nil))
		}()
	}

	clockServer, err := NewClockServer(storageNodeConfig.ClockEpochFilePath)
	if err != nil {
		log.Fatal(err)
	}

	nodeServer := NewNodeServer(storageNodeConfig.Id, storage)

	grpcServer := NewGrpcServer(
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
	)
	api.RegisterClockServer(grpcServer, clockServer)
	api.RegisterNodeServer(grpcServer, nodeServer)
	grpc_prometheus.Register(grpcServer)

	exitSignals := make(chan os.Signal, 1)
	signal.Notify(exitSignals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-exitSignals
		storage.Close()
		os.Exit(0)
	}()

	c, err := net.Listen("tcp", storageNodeConfig.BindAddress)
	if err != nil {
		log.Fatal(err)
	}
	if err := grpcServer.Serve(c); err != nil {
		log.Fatal(err)
	}
}
