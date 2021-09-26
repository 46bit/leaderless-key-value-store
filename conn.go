package leaderless_key_value_store

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

const (
	MAX_MSG_SIZE = 64 << 20

	PING_RATE    = 5 * time.Second
	PING_TIMEOUT = 5 * time.Second
)

func NewGrpcServer(extraServerOptions ...grpc.ServerOption) *grpc.Server {
	keepaliveEnforcementPolicy := keepalive.EnforcementPolicy{
		PermitWithoutStream: true,
		MinTime:             PING_RATE / 10,
	}
	keepaliveServerParams := keepalive.ServerParameters{
		MaxConnectionIdle: 15 * time.Second,
		Time:              PING_RATE,
		Timeout:           PING_TIMEOUT,
	}
	serverOptions := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(MAX_MSG_SIZE),
		grpc.MaxSendMsgSize(MAX_MSG_SIZE),
		grpc.KeepaliveParams(keepaliveServerParams),
		grpc.KeepaliveEnforcementPolicy(keepaliveEnforcementPolicy),
	}
	serverOptions = append(serverOptions, extraServerOptions...)
	return grpc.NewServer(serverOptions...)
}

type ConnManager struct {
	sync.Mutex
	PoolSize          int
	RemoveUnusedAfter time.Duration
	Pools             map[string]ConnPool
	LastUsed          map[string]time.Time
}

func NewConnManager(poolSize int, removeUnusedAfter time.Duration) *ConnManager {
	return &ConnManager{
		PoolSize:          poolSize,
		RemoveUnusedAfter: removeUnusedAfter,
		Pools:             map[string]ConnPool{},
		LastUsed:          map[string]time.Time{},
	}
}

func (m *ConnManager) Add(address string) error {
	pool, err := NewRoundRobinConnPool(address, m.PoolSize)
	if err != nil {
		return err
	}

	m.Lock()
	defer m.Unlock()
	m.Pools[address] = pool
	m.LastUsed[address] = time.Now()
	return nil
}

func (m *ConnManager) Get(address string) (conn grpc.ClientConnInterface, ok bool) {
	m.Lock()
	defer m.Unlock()
	pool, ok := m.Pools[address]
	if ok {
		m.LastUsed[address] = time.Now()
	}
	return pool, ok
}

func (m *ConnManager) Run(ctx context.Context) {
	ticker := time.NewTicker(m.RemoveUnusedAfter)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.garbageCollect()
		}
	}
}

func (m *ConnManager) garbageCollect() {
	m.Lock()
	defer m.Unlock()
	for address, lastUsed := range m.LastUsed {
		if time.Since(lastUsed) < m.RemoveUnusedAfter {
			continue
		}
		pool := m.Pools[address]
		// FIXME: Find out if closing connections is expensive enough to be worth temporary unlocking
		m.Unlock()
		errs := pool.Close()
		log.Println(fmt.Errorf("ignored errors closing connection pool: %v", errs))
		m.Lock()
		delete(m.Pools, address)
		delete(m.LastUsed, address)
	}
}

type ConnPool interface {
	grpc.ClientConnInterface

	Conn() *grpc.ClientConn
	PoolSize() int
	Close() []error
}

type RoundRobinConnPool struct {
	Index       int32
	Size        int
	Connections []*grpc.ClientConn
}

var _ (ConnPool) = &RoundRobinConnPool{}

func NewRoundRobinConnPool(address string, poolSize int) (*RoundRobinConnPool, error) {
	conns := make([]*grpc.ClientConn, poolSize)
	for i := 0; i < poolSize; i += 1 {
		conn, err := connect(address)
		if err != nil {
			return nil, err
		}
		conns[i] = conn
	}
	// FIXME: Connect
	return &RoundRobinConnPool{
		Index:       0,
		Size:        poolSize,
		Connections: conns,
	}, nil
}

func connect(address string) (*grpc.ClientConn, error) {
	keepaliveClientParams := keepalive.ClientParameters{
		Time:                PING_RATE,
		Timeout:             PING_TIMEOUT,
		PermitWithoutStream: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return grpc.DialContext(
		ctx,
		address,
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(keepaliveClientParams),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MAX_MSG_SIZE), grpc.MaxCallSendMsgSize(MAX_MSG_SIZE)),
	)
}

func (r *RoundRobinConnPool) Conn() *grpc.ClientConn {
	newIdx := atomic.AddInt32(&r.Index, 1)
	return r.Connections[newIdx%int32(r.Size)]
}

func (r *RoundRobinConnPool) PoolSize() int {
	return r.Size
}

func (r *RoundRobinConnPool) Close() []error {
	errs := []error{}
	for _, conn := range r.Connections {
		err := conn.Close()
		errs = append(errs, err)
	}
	return errs
}

func (p *RoundRobinConnPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	return p.Conn().Invoke(ctx, method, args, reply, opts...)
}

func (p *RoundRobinConnPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return p.Conn().NewStream(ctx, desc, method, opts...)
}
