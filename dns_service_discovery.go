package leaderless_key_value_store

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/46bit/leaderless-key-value-store/api"
	"google.golang.org/grpc"
)

// FIXME: Add TTL-style system to remove expired records
func PerformDnsServiceDiscovery(ctx context.Context, config *DnsServiceDiscoveryConfig, clusterDesc *ClusterDescription) {
	ticker := time.NewTicker(config.UpdateInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		ips, err := net.LookupIP(config.StorageNodeDomain)
		if err != nil {
			// FIXME: Improve errors
			log.Println(err)
		}

		for _, ip := range ips {
			address := fmt.Sprintf("%s:%d", ip.String(), config.StorageNodePort)
			go func() {
				healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				health, err := getHealthFromNode(healthCtx, address)
				if err != nil {
					log.Println(err)
				}

				clusterDesc.Lock()
				defer clusterDesc.Unlock()
				storageNode, ok := clusterDesc.StorageNodes[health.NodeId]
				if !ok {
					log.Println(fmt.Errorf("got DNS for unknown Storage Node with ID: '%s'", health.NodeId))
					return
				}
				storageNode.Address = &address
				err = clusterDesc.ConnManager.Add(ctx, address, true)
				if err != nil {
					fmt.Println(err)
				}
			}()
		}
	}
}

func getHealthFromNode(ctx context.Context, address string) (*api.HealthResponse, error) {
	conn, err := grpc.DialContext(
		ctx,
		address,
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20), grpc.MaxCallSendMsgSize(64<<20)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("did not connect: %w", err)
	}
	defer conn.Close()

	return api.NewNodeClient(conn).Health(ctx, &api.HealthRequest{})
}
