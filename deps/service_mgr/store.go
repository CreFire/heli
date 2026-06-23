package servicemgr

import (
	"game/deps/basal"

	"github.com/sasha-s/go-deadlock"
)

type Service struct {
	name      string
	mu        deadlock.RWMutex
	instances []*ServiceInstance
	index     map[int32]int
	chash     *basal.ConsistentHash[int32]
}

func newService(name string) *Service {
	return &Service{
		name:      name,
		instances: make([]*ServiceInstance, 0, 64),
		index:     make(map[int32]int),
		chash:     basal.NewConsistentHash[int32](0, nil),
	}
}

func (s *Service) setInstance(inst *ServiceInstance) (old *ServiceInstance, replaced bool) {
	if inst == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if idx, ok := s.index[inst.InstanceId]; ok {
		newSlice := make([]*ServiceInstance, len(s.instances))
		copy(newSlice, s.instances)
		old = newSlice[idx]
		inst.SetNetStatus(old.NetState())
		inst.Load_ = old.Load()
		newSlice[idx] = inst
		s.instances = newSlice
		return old, true
	}

	newSlice := make([]*ServiceInstance, len(s.instances)+1)
	copy(newSlice, s.instances)
	newSlice[len(s.instances)] = inst
	s.instances = newSlice
	s.index[inst.InstanceId] = len(newSlice) - 1
	s.chash.AddNode(inst.InstanceId)
	return nil, false
}

func (s *Service) removeInstance(instanceID int32) *ServiceInstance {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.index[instanceID]
	if !ok {
		return nil
	}

	old := s.instances
	if len(old) == 0 {
		return nil
	}
	newSlice := make([]*ServiceInstance, len(old))
	copy(newSlice, old)

	removed := newSlice[idx]
	lastIdx := len(newSlice) - 1
	if idx != lastIdx {
		lastInst := newSlice[lastIdx]
		newSlice[idx] = lastInst
		s.index[lastInst.InstanceId] = idx
	}
	newSlice = newSlice[:lastIdx]
	s.instances = newSlice
	delete(s.index, instanceID)
	s.chash.RemoveNode(instanceID)

	return removed
}

func (s *Service) getInstance(instanceID int32) (*ServiceInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx, ok := s.index[instanceID]
	if !ok {
		return nil, false
	}
	return s.instances[idx], true
}

func (s *Service) snapshot(filter func(*ServiceInstance) bool) []*ServiceInstance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if filter == nil {
		return s.instances
	}

	result := make([]*ServiceInstance, 0, len(s.instances))
	for _, inst := range s.instances {
		if filter(inst) {
			result = append(result, inst)
		}
	}
	return result
}

func (s *Service) len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.instances)
}

func (s *Service) GetByHashKey(key any) (*ServiceInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodeID, ok := s.chash.GetNode(key)
	if !ok {
		return nil, false
	}
	idx, ok := s.index[nodeID]
	if !ok || idx < 0 || idx >= len(s.instances) {
		return nil, false
	}
	return s.instances[idx], true
}
