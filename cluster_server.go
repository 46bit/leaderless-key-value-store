package leaderless_key_value_store

import (
	"context"
	"fmt"
	"time"

	"github.com/46bit/leaderless-key-value-store/api"
	"google.golang.org/protobuf/types/known/durationpb"
)

type ClusterServer struct {
	api.UnimplementedClusterServer

	startTime   *time.Time
	clusterDesc *ClusterDescription
}

var _ api.ClusterServer = (*ClusterServer)(nil)

func NewClusterServer(clusterDesc *ClusterDescription) *ClusterServer {
	now := time.Now()
	return &ClusterServer{
		startTime:   &now,
		clusterDesc: clusterDesc,
	}
}

func (s *ClusterServer) Info(ctx context.Context, req *api.ClusterInfoRequest) (*api.ClusterInfoResponse, error) {
	s.clusterDesc.Lock()
	defer s.clusterDesc.Unlock()

	response := &api.ClusterInfoResponse{
		Uptime:           durationpb.New(s.uptime()),
		ReplicationLevel: int64(s.clusterDesc.ReplicationLevel),
		StorageNodes:     []*api.StorageNode{},
	}

	for _, storageNodeDesc := range s.clusterDesc.StorageNodes {
		storageNode := &api.StorageNode{
			Id: storageNodeDesc.Id,
		}
		if storageNodeDesc.Address != nil {
			storageNode.Address = *storageNodeDesc.Address
		}
		response.StorageNodes = append(response.StorageNodes, storageNode)
	}

	return response, nil
}

func (s *ClusterServer) Get(ctx context.Context, req *api.GetRequest) (*api.GetResponse, error) {
	entry, err := Read(req.Key, s.clusterDesc)
	if err != nil {
		fmt.Println(fmt.Errorf("error getting value from cluster: %w", err))
	}
	var pbEntry *api.Entry
	if entry != nil {
		pbEntry = &api.Entry{
			Key:   entry.Key,
			Value: entry.Value,
		}
	}
	return &api.GetResponse{
		Entry: pbEntry,
	}, err
}

func (s *ClusterServer) Set(ctx context.Context, req *api.SetRequest) (*api.SetResponse, error) {
	entry := Entry{
		Key:   req.Entry.Key,
		Value: req.Entry.Value,
	}
	err := Write(entry, s.clusterDesc)
	if err != nil {
		fmt.Println(fmt.Errorf("error setting value in cluster: %w", err))
	}
	return &api.SetResponse{}, err
}

func (s *ClusterServer) uptime() time.Duration {
	uptime := time.Duration(0)
	if s.startTime != nil {
		uptime = time.Now().Sub(*s.startTime)
	}
	return uptime
}
