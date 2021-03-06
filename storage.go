package leaderless_key_value_store

import (
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/46bit/leaderless-key-value-store/api"
)

type Entry struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type Storage struct {
	BadgerDb *badger.DB
}

func NewStorage(dbPath string) (*Storage, error) {
	options := badger.DefaultOptions(dbPath)
	// FIXME: Make this configurable
	options.ValueLogFileSize = 256 << 20

	badgerDb, err := badger.Open(options)
	if err != nil {
		return nil, err
	}
	return &Storage{BadgerDb: badgerDb}, nil
}

func (s *Storage) Get(key string) (*api.ClockedEntry, error) {
	var clockedEntryPointer *api.ClockedEntry
	err := s.BadgerDb.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(bytes []byte) error {
			var clockedEntry api.ClockedEntry
			if err := proto.Unmarshal(bytes, &clockedEntry); err != nil {
				return fmt.Errorf("error deserialising bytes into clocked entry: %w", err)
			}
			// FIXME: Optional debug check that the key matches?
			clockedEntryPointer = &clockedEntry
			return nil
		})
	})
	return clockedEntryPointer, err
}

func (s *Storage) Set(value *api.ClockedEntry) error {
	bytes, err := proto.Marshal(value)
	if err != nil {
		return fmt.Errorf("error serialising clocked entry to bytes: %w", err)
	}
	return s.BadgerDb.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(value.Entry.Key), bytes)
	})
}

func (s *Storage) Keys() ([]string, error) {
	keys := []string{}
	err := s.BadgerDb.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.PrefetchValues = false
		iter := txn.NewIterator(opt)
		defer iter.Close()
		for iter.Rewind(); iter.Valid(); iter.Next() {
			keys = append(keys, string(iter.Item().Key()))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *Storage) Close() {
	s.BadgerDb.Close()
}

func SetupBadgerStorageMetrics() {
	badgerExpvarCollector := collectors.NewExpvarCollector(map[string]*prometheus.Desc{
		"badger_v3_blocked_puts_total":   prometheus.NewDesc("badger_v3_blocked_puts_total", "Blocked Puts", nil, nil),
		"badger_v3_compactions_current":  prometheus.NewDesc("badger_v3_compactions_current", "Tables being compacted", nil, nil),
		"badger_v3_disk_reads_total":     prometheus.NewDesc("badger_v3_disk_reads_total", "Disk Reads", nil, nil),
		"badger_v3_disk_writes_total":    prometheus.NewDesc("badger_v3_disk_writes_total", "Disk Writes", nil, nil),
		"badger_v3_gets_total":           prometheus.NewDesc("badger_v3_gets_total", "Gets", nil, nil),
		"badger_v3_puts_total":           prometheus.NewDesc("badger_v3_puts_total", "Puts", nil, nil),
		"badger_v3_memtable_gets_total":  prometheus.NewDesc("badger_v3_memtable_gets_total", "Memtable gets", nil, nil),
		"badger_v3_lsm_size_bytes":       prometheus.NewDesc("badger_v3_lsm_size_bytes", "LSM Size in bytes", []string{"database"}, nil),
		"badger_v3_vlog_size_bytes":      prometheus.NewDesc("badger_v3_vlog_size_bytes", "Value Log Size in bytes", []string{"database"}, nil),
		"badger_v3_pending_writes_total": prometheus.NewDesc("badger_v3_pending_writes_total", "Pending Writes", []string{"database"}, nil),
		"badger_v3_read_bytes":           prometheus.NewDesc("badger_v3_read_bytes", "Read bytes", nil, nil),
		"badger_v3_written_bytes":        prometheus.NewDesc("badger_v3_written_bytes", "Written bytes", nil, nil),
		"badger_v3_lsm_bloom_hits_total": prometheus.NewDesc("badger_v3_lsm_bloom_hits_total", "LSM Bloom Hits", []string{"level"}, nil),
		"badger_v3_lsm_level_gets_total": prometheus.NewDesc("badger_v3_lsm_level_gets_total", "LSM Level Gets", []string{"level"}, nil),
	})
	prometheus.MustRegister(badgerExpvarCollector)
}
