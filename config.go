package leaderless_key_value_store

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type StorageNodeConfig struct {
	Id                 string `yaml:"id"`
	BindAddress        string `yaml:"bind_address"`
	BindMetricsAddress string `yaml:"bind_metrics_address"`
	ClockEpochFilePath string `yaml:"clock_epoch_file_path"`
	BadgerDbFolder     string `yaml:"badger_db_folder"`
}

func LoadStorageNodeConfig(path string) (*StorageNodeConfig, error) {
	var nestedStorageNodeConfig struct {
		StorageNode StorageNodeConfig `yaml:"storage_node"`
	}
	yamlBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading storage node config file: %w", err)
	}
	if err = yaml.Unmarshal(yamlBytes, &nestedStorageNodeConfig); err != nil {
		return nil, fmt.Errorf("error deserialising storage node config file: %w", err)
	}

	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress != "" {
		nestedStorageNodeConfig.StorageNode.BindAddress = bindAddress
	}

	return &nestedStorageNodeConfig.StorageNode, nil
}

type CoordinatorConfig struct {
	RendezvousHashingSeed uint32 `yaml:"rendezvous_hashing_seed"`
	ReplicationLevel      int    `yaml:"replication_level"`

	BindAddress string `yaml:"bind_address"`

	StorageNodeIds []string `yaml:"storage_node_ids"`
	// Map from storage node IDs to "hostname:port"
	StaticServiceDiscovery map[string]string          `yaml:"static_service_discovery"`
	DnsServiceDiscovery    *DnsServiceDiscoveryConfig `yaml:"dns_service_discovery"`
}

type DnsServiceDiscoveryConfig struct {
	StorageNodeDomain string        `yaml:"storage_node_domain"`
	StorageNodePort   int16         `yaml:"storage_node_port"`
	UpdateInterval    time.Duration `yaml:"update_interval"`
}

func LoadCoordinatorConfig(path string) (*CoordinatorConfig, error) {
	var nestedCoordinatorConfig struct {
		CoordinatorNode CoordinatorConfig `yaml:"coordinator_node"`
	}
	yamlBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading coordinator config file: %w", err)
	}
	if err = yaml.Unmarshal(yamlBytes, &nestedCoordinatorConfig); err != nil {
		return nil, fmt.Errorf("error deserialising coordinator config file: %w", err)
	}

	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress != "" {
		nestedCoordinatorConfig.CoordinatorNode.BindAddress = bindAddress
	}

	return &nestedCoordinatorConfig.CoordinatorNode, nil
}
