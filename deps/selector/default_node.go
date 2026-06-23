package selector

import (
	"game/deps/etcd"
	"strconv"
)

var _ Node = (*DefaultNode)(nil)

// DefaultNode is selector node
type DefaultNode struct {
	scheme   string
	addr     string
	weight   *int64
	version  string
	name     string
	metadata map[string]string
}

// Scheme is node scheme
func (n *DefaultNode) Scheme() string {
	return n.scheme
}

// Address is node address
func (n *DefaultNode) Address() string {
	return n.addr
}

// ServiceName is node serviceName
func (n *DefaultNode) ServiceName() string {
	return n.name
}

// InitialWeight is node initialWeight
func (n *DefaultNode) InitialWeight() *int64 {
	return n.weight
}

// Version is node version
func (n *DefaultNode) Version() string {
	return n.version
}

// Metadata is node metadata
func (n *DefaultNode) Metadata() map[string]string {
	return n.metadata
}

// NewNode new node
func NewNode(scheme, addr string, ins *etcd.ServiceInstance) Node {
	n := &DefaultNode{
		scheme: scheme,
		addr:   addr,
	}
	if ins != nil {
		n.name = ins.ServiceName
		if ins.ProVersion != 0 {
			n.version = strconv.FormatInt(int64(ins.ProVersion), 10)
		}
		n.metadata = ins.MetaData
		if ins.Weight != 0 {
			weight := int64(ins.Weight)
			n.weight = &weight
		}
	}
	return n
}
