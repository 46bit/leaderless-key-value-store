package leaderless_key_value_store

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
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

	storageNodeConfig := nestedStorageNodeConfig.StorageNode

	nodeId := os.Getenv("NODE_ID")
	if nodeId != "" {
		storageNodeConfig.Id = nodeId
	}

	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress != "" {
		storageNodeConfig.BindAddress = bindAddress
	}

	clockEpochFilePath := os.Getenv("CLOCK_EPOCH_FILE_PATH")
	if clockEpochFilePath != "" {
		storageNodeConfig.ClockEpochFilePath = clockEpochFilePath
	}

	badgerDbFolder := os.Getenv("BADGERDB_FOLDER")
	if badgerDbFolder != "" {
		storageNodeConfig.BadgerDbFolder = badgerDbFolder
	}

	return &storageNodeConfig, nil
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
	StorageNodePort   int64         `yaml:"storage_node_port"`
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

	coordinatorConfig := nestedCoordinatorConfig.CoordinatorNode

	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress != "" {
		coordinatorConfig.BindAddress = bindAddress
	}

	// FIXME: This code needs a major tidyup
	dnsSDStorageNodeDomain := os.Getenv("DNS_SD_STORAGE_NODE_DOMAIN")
	if dnsSDStorageNodeDomain != "" {
		if coordinatorConfig.DnsServiceDiscovery == nil {
			return nil, fmt.Errorf("cannot specify DNS_SD_STORAGE_NODE_DOMAIN unless update_interval configured in yaml")
		}
		coordinatorConfig.DnsServiceDiscovery.StorageNodeDomain = dnsSDStorageNodeDomain
	}
	dnsSDStorageNodePort := os.Getenv("DNS_SD_STORAGE_NODE_PORT")
	if dnsSDStorageNodePort != "" {
		if coordinatorConfig.DnsServiceDiscovery == nil {
			return nil, fmt.Errorf("cannot specify DNS_SD_STORAGE_NODE_PORT unless update_interval configured in yaml")
		}
		port, err := strconv.ParseInt(dnsSDStorageNodePort, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("DNS_SD_STORAGE_NODE_PORT must be an integer")
		}
		coordinatorConfig.DnsServiceDiscovery.StorageNodePort = port
	}

	return &coordinatorConfig, nil
}
