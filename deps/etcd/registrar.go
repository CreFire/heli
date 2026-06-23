package etcd

import (
	"context"
	"errors"
	"fmt"
	"game/deps/kit"
	"game/deps/xlog"
	"maps"
	"sync/atomic"
	"time"

	"github.com/sasha-s/go-deadlock"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	serviceLeaseTTL     int64 = 15
	lockTTL             int   = 5
	lockTimeout               = 5 * time.Second
	unlockTimeout             = 3 * time.Second
	leaseGrantTimeout         = 5 * time.Second
	registerPutTimeout        = 5 * time.Second
	updateGetTimeout          = 3 * time.Second
	updatePutTimeout          = 3 * time.Second
	revokeTimeout             = 3 * time.Second
	keepAliveRetryDelay       = 5 * time.Second
)

var (
	errNoValidLease = errors.New("no valid lease")
	errLeaseChanged = errors.New("lease changed")
)

// ServiceRegistrar 管理单个服务的注册和心跳。
type ServiceRegistrar struct {
	client  *clientv3.Client
	service *ServiceInstance
	key     string
	ttl     int64
	leaseID atomic.Int64
	lease   clientv3.Lease

	mu     deadlock.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewServiceRegistrar 创建并启动一个单服务的注册器。
// 它会立即注册服务并启动心跳和状态更新协程。
// ttl 是租约的生命周期（秒），建议设置为15秒或以上。
func NewServiceRegistrar(client *EtcdClient, cluster string, service *ServiceInstance) *ServiceRegistrar {
	ctx, cancel := context.WithCancel(context.Background())
	ssr := &ServiceRegistrar{
		client:  client.Client,
		service: service,
		key:     RegistrarKey(cluster, service.ServiceName, service.InstanceId), // key 示例: /cluster1/logic/1
		ttl:     serviceLeaseTTL,
		lease:   clientv3.NewLease(client.Client),
		ctx:     ctx,
		cancel:  cancel,
	}

	return ssr
}

func (ssr *ServiceRegistrar) Register() error {
	if err := ssr.withRegisterLock(ssr.registerWithNewLease); err != nil {
		ssr.Close()
		xlog.Errorf("ServiceRegistrar: initial registration failed for key '%s': %v", ssr.key, err)
		return fmt.Errorf("initial registration failed: %w", err)
	}

	go ssr.keepAliveLoop()

	xlog.Infof("ServiceRegistrar: service '%s' registered successfully at key '%s'", ssr.service.ServiceName, ssr.key)
	return nil
}

func (ssr *ServiceRegistrar) withRegisterLock(fn func() error) error {
	lockCtx, lockCancel := context.WithTimeout(ssr.ctx, lockTimeout)
	defer lockCancel()
	lockSession, err := concurrency.NewSession(ssr.client, concurrency.WithTTL(lockTTL), concurrency.WithContext(lockCtx))
	if err != nil {
		return fmt.Errorf("failed to create lock session for key '%s': %w", ssr.key, err)
	}

	lockKey := fmt.Sprintf("lock-%s", ssr.key)
	mutex := concurrency.NewMutex(lockSession, lockKey)
	if err := mutex.Lock(lockCtx); err != nil {
		lockSession.Close()
		return fmt.Errorf("failed to acquire lock for key '%s': %w", ssr.key, err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
		if err := mutex.Unlock(unlockCtx); err != nil {
			xlog.Warnf("ServiceRegistrar: failed to release lock for key '%s': %v", ssr.key, err)
		}
		cancel()
		lockSession.Close()
	}()

	return fn()
}

// keepAliveLoop 维持与 etcd 服务器的租约。
func (ssr *ServiceRegistrar) keepAliveLoop() {
	defer kit.Exception(func(err error) {
		xlog.Errorf("ServiceRegistrar: keepAliveLoop panic: %v", err)
	})

	for {
		select {
		case <-ssr.ctx.Done():
			xlog.Infof("ServiceRegistrar: keepAliveLoop stopped for key '%s'.", ssr.key)
			return
		default:
		}

		// 确保有有效的租约ID
		leaseID := clientv3.LeaseID(ssr.leaseID.Load())
		if leaseID == 0 {
			xlog.Errorf("ServiceRegistrar: invalid leaseID, attempting to re-register for key '%s'", ssr.key)
			if err := ssr.reRegister(); err != nil {
				xlog.Errorf("ServiceRegistrar: re-register failed for key '%s': %v", ssr.key, err)
				// 等待一段时间后重试
				select {
				case <-time.After(keepAliveRetryDelay):
					continue
				case <-ssr.ctx.Done():
					return
				}
			}
			leaseID = clientv3.LeaseID(ssr.leaseID.Load())
		}

		ch, err := ssr.lease.KeepAlive(ssr.ctx, leaseID)
		if err != nil {
			xlog.Errorf("ServiceRegistrar: KeepAlive setup failed for key '%s': %v. Will retry.", ssr.key, err)
			ssr.leaseID.CompareAndSwap(int64(leaseID), 0)
			// 等待一段时间后重试
			select {
			case <-time.After(keepAliveRetryDelay):
				continue
			case <-ssr.ctx.Done():
				return
			}
		}

		// 处理 keep-alive 响应
		keepAliveActive := true
		for keepAliveActive {
			select {
			case ka, ok := <-ch:
				if !ok {
					xlog.Errorf("ServiceRegistrar: KeepAlive channel closed for key '%s'. Will re-register.", ssr.key)
					ssr.leaseID.CompareAndSwap(int64(leaseID), 0)
					keepAliveActive = false // 退出内层循环，重新注册
					break
				}
				xlog.Debugf("ServiceRegistrar: key %s (lease %x) keep-alive, TTL: %d", ssr.key, ka.ID, ka.TTL)
			case <-ssr.ctx.Done():
				xlog.Infof("ServiceRegistrar: keepAliveLoop stopped for key '%s'.", ssr.key)
				return
			}
		}
	}
}

// reRegister 尝试重新注册服务，线程安全
func (ssr *ServiceRegistrar) reRegister() error {
	return ssr.withRegisterLock(func() error {
		if err := ssr.revokeCurrentLease(); err != nil {
			xlog.Warnf("ServiceRegistrar: failed to revoke old lease for key '%s': %v", ssr.key, err)
		}
		return ssr.registerWithNewLease()
	})
}

func (ssr *ServiceRegistrar) registerWithNewLease() error {
	ssr.mu.RLock()
	val, err := marshal(ssr.service)
	ssr.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to marshal service instance: %w", err)
	}

	ctx, cancel := context.WithTimeout(ssr.ctx, leaseGrantTimeout)
	grantResp, err := ssr.lease.Grant(ctx, ssr.ttl)
	cancel()
	if err != nil {
		return fmt.Errorf("failed to grant lease: %w", err)
	}
	leaseID := grantResp.ID

	if err := ssr.putServiceIfAbsent(val, leaseID); err != nil {
		if revokeErr := ssr.revokeLease(leaseID); revokeErr != nil {
			xlog.Warnf("ServiceRegistrar: failed to revoke unused lease %x: %v", leaseID, revokeErr)
		}
		return err
	}

	ssr.leaseID.Store(int64(leaseID))
	return nil
}

func (ssr *ServiceRegistrar) putServiceIfAbsent(val string, leaseID clientv3.LeaseID) error {
	putCtx, putCancel := context.WithTimeout(ssr.ctx, registerPutTimeout)
	defer putCancel()

	resp, err := ssr.client.Txn(putCtx).
		If(clientv3.Compare(clientv3.CreateRevision(ssr.key), "=", 0)).
		Then(clientv3.OpPut(ssr.key, val, clientv3.WithLease(leaseID))).
		Else(clientv3.OpGet(ssr.key)).
		Commit()
	if err != nil {
		return fmt.Errorf("failed to put service key: %w", err)
	}
	if resp.Succeeded {
		return nil
	}
	value := ""
	if len(resp.Responses) > 0 {
		if rangeResp := resp.Responses[0].GetResponseRange(); rangeResp != nil && len(rangeResp.Kvs) > 0 {
			value = string(rangeResp.Kvs[0].Value)
		}
	}
	xlog.Errorf("Etcd registration key: %s already exists; refusing to start duplicate instance.", ssr.key)
	return fmt.Errorf("ServiceRegistrar: service '%s' instId: '%d' value: '%s', refusing to start duplicate instance",
		ssr.service.ServiceName, ssr.service.InstanceId, value)
}

func (ssr *ServiceRegistrar) revokeCurrentLease() error {
	leaseID := clientv3.LeaseID(ssr.leaseID.Load())
	ssr.leaseID.Store(0)
	if leaseID == 0 {
		return nil
	}
	return ssr.revokeLease(leaseID)
}

func (ssr *ServiceRegistrar) revokeLease(leaseID clientv3.LeaseID) error {
	ctx, cancel := context.WithTimeout(context.Background(), revokeTimeout)
	_, err := ssr.lease.Revoke(ctx, leaseID)
	cancel()
	return err
}

// updateValueToEtcd 将当前服务信息写入 etcd。
func (ssr *ServiceRegistrar) updateValueToEtcd() error {
	leaseID := clientv3.LeaseID(ssr.leaseID.Load())
	if leaseID == 0 {
		xlog.Errorf("ServiceRegistrar: updateValueToEtcd skipped, no valid lease for key '%s'", ssr.key)
		return errNoValidLease
	}

	err := ssr.updateValueWithLease(leaseID)
	if !errors.Is(err, errLeaseChanged) {
		return err
	}

	currentLeaseID := clientv3.LeaseID(ssr.leaseID.Load())
	if currentLeaseID == 0 || currentLeaseID == leaseID {
		return err
	}
	return ssr.updateValueWithLease(currentLeaseID)
}

func (ssr *ServiceRegistrar) updateValueWithLease(leaseID clientv3.LeaseID) error {
	getCtx, getCancel := context.WithTimeout(ssr.ctx, updateGetTimeout)
	resp, err := ssr.client.Get(getCtx, ssr.key)
	getCancel()
	if err != nil {
		xlog.Errorf("ServiceRegistrar: failed to get current service value for key '%s': %v", ssr.key, err)
		return fmt.Errorf("failed to get current service value: %w", err)
	}

	var oldService *ServiceInstance
	keyExists := len(resp.Kvs) > 0
	if keyExists {
		keyLease := clientv3.LeaseID(resp.Kvs[0].Lease)
		if keyLease != leaseID {
			return fmt.Errorf("%w for key '%s': want %x, got %x", errLeaseChanged, ssr.key, leaseID, keyLease)
		}

		var unmarshalErr error
		oldService, unmarshalErr = unmarshal(resp.Kvs[0].Value)
		if unmarshalErr != nil {
			xlog.Warnf("ServiceRegistrar: failed to unmarshal existing service value for key '%s', will attempt to PUT anyway: %v", ssr.key, unmarshalErr)
			oldService = nil
		}
	}

	ssr.mu.RLock()
	if oldService != nil && oldService.Equal(ssr.service) {
		ssr.mu.RUnlock()
		xlog.Debugf("ServiceRegistrar: service value for key '%s' is already up-to-date. Skipping update.", ssr.key)
		return nil
	}

	newVal, err := marshal(ssr.service)
	ssr.mu.RUnlock()

	if err != nil {
		xlog.Errorf("ServiceRegistrar: failed to marshal service for update: %v", err)
		return err
	}

	if keyExists {
		err = ssr.putServiceWithSameLease(newVal, leaseID)
	} else {
		err = ssr.putMissingService(newVal, leaseID)
	}
	if err != nil {
		xlog.Errorf("ServiceRegistrar: failed to update service value for key '%s': %v", ssr.key, err)
	} else {
		oldVal, _ := marshal(oldService)
		xlog.Debugf("ServiceRegistrar: successfully updated service value for key: '%s' old: '%s'  new : '%s'", ssr.key, oldVal, newVal)
	}
	return err
}

func (ssr *ServiceRegistrar) putServiceWithSameLease(val string, leaseID clientv3.LeaseID) error {
	putCtx, putCancel := context.WithTimeout(ssr.ctx, updatePutTimeout)
	defer putCancel()

	resp, err := ssr.client.Txn(putCtx).
		If(clientv3.Compare(clientv3.LeaseValue(ssr.key), "=", int64(leaseID))).
		Then(clientv3.OpPut(ssr.key, val, clientv3.WithLease(leaseID))).
		Else(clientv3.OpGet(ssr.key)).
		Commit()
	if err != nil {
		return fmt.Errorf("failed to update service key: %w", err)
	}
	if resp.Succeeded {
		return nil
	}
	gotLeaseID := clientv3.LeaseID(0)
	if len(resp.Responses) > 0 {
		rangeResp := resp.Responses[0].GetResponseRange()
		if rangeResp != nil && len(rangeResp.Kvs) > 0 {
			gotLeaseID = clientv3.LeaseID(rangeResp.Kvs[0].Lease)
		}
	}
	return fmt.Errorf("%w for key '%s': want %x, got %x", errLeaseChanged, ssr.key, leaseID, gotLeaseID)
}

func (ssr *ServiceRegistrar) putMissingService(val string, leaseID clientv3.LeaseID) error {
	putCtx, putCancel := context.WithTimeout(ssr.ctx, updatePutTimeout)
	defer putCancel()

	resp, err := ssr.client.Txn(putCtx).
		If(clientv3.Compare(clientv3.CreateRevision(ssr.key), "=", 0)).
		Then(clientv3.OpPut(ssr.key, val, clientv3.WithLease(leaseID))).
		Else(clientv3.OpGet(ssr.key)).
		Commit()
	if err != nil {
		return fmt.Errorf("failed to create missing service key: %w", err)
	}
	if resp.Succeeded {
		return nil
	}
	gotLeaseID := clientv3.LeaseID(0)
	if len(resp.Responses) > 0 {
		rangeResp := resp.Responses[0].GetResponseRange()
		if rangeResp != nil && len(rangeResp.Kvs) > 0 {
			gotLeaseID = clientv3.LeaseID(rangeResp.Kvs[0].Lease)
		}
	}
	return fmt.Errorf("%w for key '%s': want %x, got %x", errLeaseChanged, ssr.key, leaseID, gotLeaseID)
}

// UpdateInstanceData 线程安全地修改服务的元数据并立即更新到 etcd。
// 这是一个阻塞操作，会返回更新是否成功。
func (ssr *ServiceRegistrar) UpdateInstanceData(inst *ServiceInstance) error {
	// 先加锁更新内存中的数据，然后立即释放锁
	ssr.mu.Lock()
	ssr.service.OnlineCount = inst.OnlineCount
	ssr.service.Enable = inst.Enable
	ssr.service.Healthy = inst.Healthy
	ssr.service.Weight = inst.Weight
	ssr.service.ProVersion = inst.ProVersion
	ssr.service.ConfVersion = inst.ConfVersion
	ssr.service.UpdateTime = inst.UpdateTime
	ssr.service.MetaData = maps.Clone(inst.MetaData)
	ssr.mu.Unlock()

	// 在没有锁的情况下执行网络IO操作，避免死锁
	return ssr.updateValueToEtcd()
}

// Close 停止注册器并从 etcd 注销服务。
func (ssr *ServiceRegistrar) Close() {
	ssr.cancel() // 停止所有后台协程

	// 撤销租约，etcd 会自动删除关联的 key
	if err := ssr.revokeCurrentLease(); err != nil {
		xlog.Warnf("ServiceRegistrar: failed to revoke lease for key '%s': %v", ssr.key, err)
	}

	ssr.lease.Close()
	xlog.Infof("ServiceRegistrar: service '%s' deregistered from key '%s'", ssr.service.ServiceName, ssr.key)
}
