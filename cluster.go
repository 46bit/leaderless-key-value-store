package leaderless_key_value_store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/46bit/leaderless-key-value-store/api"
	"github.com/spaolacci/murmur3"
)

type ClusterDescription struct {
	sync.Mutex

	ReplicationLevel      int
	RendezvousHashingSeed uint32
	StorageNodes          map[string]*StorageNodeDescription
	ConnManager           *ConnManager
}

func NewClusterDescription(cfg *CoordinatorConfig, connManager *ConnManager) *ClusterDescription {
	d := &ClusterDescription{
		ReplicationLevel:      cfg.ReplicationLevel,
		RendezvousHashingSeed: cfg.RendezvousHashingSeed,
		StorageNodes:          map[string]*StorageNodeDescription{},
		ConnManager:           connManager,
	}
	for _, storageNodeId := range cfg.StorageNodeIds {
		d.StorageNodes[storageNodeId] = NewStorageNodeDescription(storageNodeId, cfg.RendezvousHashingSeed)
	}
	if cfg.StaticServiceDiscovery != nil {
		for storageNodeId, address := range cfg.StaticServiceDiscovery {
			if _, ok := d.StorageNodes[storageNodeId]; !ok {
				continue
			}
			addressCopy := address
			d.StorageNodes[storageNodeId].Address = &addressCopy
			d.ConnManager.Add(address)
		}
	}
	return d
}

type StorageNodeDescription struct {
	Id      string
	Hash    uint32
	Address *string
}

func NewStorageNodeDescription(id string, rendezvousHashingSeed uint32) *StorageNodeDescription {
	return &StorageNodeDescription{
		Id:      id,
		Hash:    murmur3.Sum32WithSeed([]byte(id), rendezvousHashingSeed),
		Address: nil,
	}
}

func (s *StorageNodeDescription) Found() bool {
	return s.Address != nil && *s.Address != ""
}

type FoundNode struct {
	CombinedHash uint64
	Node         *StorageNodeDescription
}

func (c *ClusterDescription) FindNodesForKey(key string) []FoundNode {
	c.Lock()
	defer c.Unlock()

	keyBytes := []byte(key)

	// FIXME: Optimise? Avoid allocations, etc
	numberOfNodes := len(c.StorageNodes)
	combinedHashToNode := make(map[uint64]*StorageNodeDescription, numberOfNodes)
	combinedHashes := []uint64{}
	for _, node := range c.StorageNodes {
		combinedHash := murmur3.Sum64WithSeed(keyBytes, node.Hash)
		combinedHashToNode[combinedHash] = node
		combinedHashes = append(combinedHashes, combinedHash)
	}
	// Sort combined hashes into descending order
	sort.Slice(combinedHashes, func(i, j int) bool { return combinedHashes[i] > combinedHashes[j] })

	bestNodes := make([]FoundNode, c.ReplicationLevel)
	for i := 0; i < c.ReplicationLevel; i++ {
		combinedHash := combinedHashes[i]
		bestNodes[i] = FoundNode{
			CombinedHash: combinedHash,
			Node:         combinedHashToNode[combinedHash],
		}
	}
	return bestNodes
}

func Read(key string, cluster *ClusterDescription) (*Entry, error) {
	chosenReplicas := cluster.FindNodesForKey(key)
	if len(chosenReplicas) == 0 {
		return nil, fmt.Errorf("no nodes found for key")
	}
	clockedEntry, err := getQuorateValue(key, chosenReplicas, cluster.ConnManager)
	if err != nil {
		return nil, err
	}
	return &Entry{
		Key:   clockedEntry.Entry.Key,
		Value: clockedEntry.Entry.Value,
	}, nil
}

func readEntryFromNode(ctx context.Context, key string, node *StorageNodeDescription, connManager *ConnManager) (*api.ClockedEntry, error) {
	if node.Address == nil {
		return nil, fmt.Errorf("no address known for node")
	}
	conn, ok := connManager.Get(*node.Address)
	if !ok {
		return nil, fmt.Errorf("no connection available to node")
	}

	r, err := api.NewNodeClient(conn).Get(ctx, &api.GetRequest{Key: key})
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return r.ClockedEntry, nil
}

func Write(entry Entry, cluster *ClusterDescription) error {
	chosenReplicas := cluster.FindNodesForKey(entry.Key)
	if len(chosenReplicas) == 0 {
		return fmt.Errorf("no nodes found to accept key")
	}

	quorateClock, err := getQuorateClock(chosenReplicas, cluster.ConnManager)
	if err != nil {
		return err
	}
	if err = setQuorateClock(quorateClock, chosenReplicas, cluster.ConnManager); err != nil {
		return err
	}

	clockedEntry := &api.ClockedEntry{
		Entry: &api.Entry{
			Key:   entry.Key,
			Value: entry.Value,
		},
		Clock: quorateClock,
	}
	return setQuorateValue(clockedEntry, chosenReplicas, cluster.ConnManager)
}

func writeEntryToNode(ctx context.Context, clockedEntry *api.ClockedEntry, node *StorageNodeDescription, connManager *ConnManager) error {
	if node.Address == nil {
		return fmt.Errorf("no address known for node")
	}
	conn, ok := connManager.Get(*node.Address)
	if !ok {
		return fmt.Errorf("no connection available to node")
	}

	_, err := api.NewNodeClient(conn).Set(ctx, &api.NodeSetRequest{
		ClockedEntry: clockedEntry,
	})
	return err
}

func getClockFromNode(ctx context.Context, node *StorageNodeDescription, connManager *ConnManager) (*api.ClockGetResponse, error) {
	if node.Address == nil {
		return nil, fmt.Errorf("no address known for node")
	}
	conn, ok := connManager.Get(*node.Address)
	if !ok {
		return nil, fmt.Errorf("no connection available to node")
	}
	return api.NewClockClient(conn).Get(ctx, &api.ClockGetRequest{})
}

func setClockOnNode(ctx context.Context, clockValue *api.ClockValue, node *StorageNodeDescription, connManager *ConnManager) error {
	if node.Address == nil {
		return fmt.Errorf("no address known for node")
	}
	conn, ok := connManager.Get(*node.Address)
	if !ok {
		return fmt.Errorf("no connection available to node")
	}

	_, err := api.NewClockClient(conn).Set(ctx, &api.ClockSetRequest{
		Value: clockValue,
	})
	return err
}

func attemptQuoracy(
	foundNodes []FoundNode,
	action func(*StorageNodeDescription) bool,
) (bool, int) {
	success := make(chan bool, 1)
	for _, foundNode := range foundNodes {
		node := foundNode.Node
		go func() {
			success <- action(node)
		}()
	}

	remaining := len(foundNodes)
	succeeded := 0
	quorateAbove := len(foundNodes) / 2
	for s := range success {
		if s {
			succeeded += 1
			remaining -= 1
		} else {
			remaining -= 1
		}
		if succeeded > quorateAbove {
			return true, succeeded
		}
		if remaining <= 0 {
			return false, 0
		}
	}
	return false, 0
}

func getQuorateClock(nodes []FoundNode, connManager *ConnManager) (*api.ClockValue, error) {
	nodeClocks := make(chan *api.ClockValue, len(nodes))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quorate, succeeded := attemptQuoracy(nodes, func(node *StorageNodeDescription) bool {
		nodeClock, err := getClockFromNode(ctx, node, connManager)
		if err != nil {
			fmt.Println(fmt.Errorf("warning getting clock from node: %w", err))
			return false
		}
		nodeClocks <- nodeClock.Value
		return true
	})
	if !quorate {
		return nil, fmt.Errorf("could not get clock from a majority of replicas")
	}

	maxEpoch := uint64(0)
	maxClock := uint64(0)
	for i := 0; i < succeeded; i += 1 {
		nodeClock := <-nodeClocks
		if nodeClock.Epoch > maxEpoch {
			maxEpoch = nodeClock.Epoch
			maxClock = nodeClock.Clock
		} else if nodeClock.Epoch == maxEpoch && nodeClock.Clock > maxClock {
			maxClock = nodeClock.Clock
		}
	}

	// FIXME: Add extra defensive check that epoch and clock not zero
	return &api.ClockValue{
		Epoch: maxEpoch,
		Clock: maxClock + 1,
	}, nil
}

func setQuorateClock(clockValue *api.ClockValue, nodes []FoundNode, connManager *ConnManager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quorate, _ := attemptQuoracy(nodes, func(node *StorageNodeDescription) bool {
		err := setClockOnNode(ctx, clockValue, node, connManager)
		if err != nil {
			fmt.Println(fmt.Errorf("warning setting clock on node: %w", err))
			return false
		}
		return true
	})
	if !quorate {
		return fmt.Errorf("could not set clock on a majority of replicas")
	}
	return nil
}

func getQuorateValue(key string, nodes []FoundNode, connManager *ConnManager) (*api.ClockedEntry, error) {
	nodeEntries := make(chan *api.ClockedEntry, len(nodes))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quorate, succeeded := attemptQuoracy(nodes, func(node *StorageNodeDescription) bool {
		nodeEntry, err := readEntryFromNode(ctx, key, node, connManager)
		if err != nil {
			fmt.Println(fmt.Errorf("warning getting value from node: %w", err))
			return false
		}
		// FIXME: Record and count 404s? To allow "not found" as the quorate answer?
		if nodeEntry == nil {
			return false
		}
		nodeEntries <- nodeEntry
		return true
	})
	if !quorate {
		return nil, fmt.Errorf("could not get value from a majority of replicas")
	}

	maxEpoch := uint64(0)
	maxClock := uint64(0)
	var newestEntry *api.ClockedEntry
	for i := 0; i < succeeded; i += 1 {
		nodeEntry := <-nodeEntries
		// FIXME: Suggestion in http://rystsov.info/2018/10/01/tso.html to use
		// a hash of the current cluster node's ID as a consistent tiebreak for if
		// multiple clocked values have the same clock
		if nodeEntry.Clock.Epoch > maxEpoch {
			maxEpoch = nodeEntry.Clock.Epoch
			maxClock = nodeEntry.Clock.Clock
			newestEntry = nodeEntry
		} else if nodeEntry.Clock.Epoch == maxEpoch && nodeEntry.Clock.Clock > maxClock {
			maxClock = nodeEntry.Clock.Clock
			newestEntry = nodeEntry
		}
	}

	// FIXME: Return a "404" error type?
	return newestEntry, nil
}

func setQuorateValue(clockedEntry *api.ClockedEntry, nodes []FoundNode, connManager *ConnManager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	quorate, _ := attemptQuoracy(nodes, func(node *StorageNodeDescription) bool {
		err := writeEntryToNode(ctx, clockedEntry, node, connManager)
		if err != nil {
			fmt.Println(fmt.Errorf("warning setting value on node: %w", err))
			return false
		}
		return true
	})
	if !quorate {
		return fmt.Errorf("could not set value on a majority of replicas")
	}
	return nil
}
