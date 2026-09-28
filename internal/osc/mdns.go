package osc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	ServiceOSCQuery = "_oscjson._tcp"
	ServiceOSC      = "_osc._udp"
	ServiceDomain   = "local."
)

type Advertiser struct {
	query *zeroconf.Server
	osc   *zeroconf.Server
}

func Advertise(instance string, queryPort, oscPort int, ifaces []net.Interface) (*Advertiser, error) {
	query, err := zeroconf.Register(instance, ServiceOSCQuery, ServiceDomain, queryPort, nil, ifaces)
	if err != nil {
		return nil, fmt.Errorf("advertise OSCQuery service: %w", err)
	}
	oscServer, err := zeroconf.Register(instance, ServiceOSC, ServiceDomain, oscPort, nil, ifaces)
	if err != nil {
		query.Shutdown()
		return nil, fmt.Errorf("advertise OSC UDP service: %w", err)
	}
	return &Advertiser{query: query, osc: oscServer}, nil
}

func (a *Advertiser) Close() {
	if a == nil {
		return
	}
	if a.query != nil {
		a.query.Shutdown()
	}
	if a.osc != nil {
		a.osc.Shutdown()
	}
}

type DiscoveredService struct {
	Instance string
	Service  string
	HostName string
	Port     int
	Text     []string
	IPv4     []net.IP
	IPv6     []net.IP
	LastSeen time.Time
}

func (s DiscoveredService) Addresses() []net.IP {
	result := make([]net.IP, 0, len(s.IPv4)+len(s.IPv6))
	for _, ip := range s.IPv4 {
		result = append(result, append(net.IP(nil), ip...))
	}
	for _, ip := range s.IPv6 {
		result = append(result, append(net.IP(nil), ip...))
	}
	return result
}

type Browser struct {
	ifaces []net.Interface

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	updates chan DiscoveredService
}

func NewBrowser(parent context.Context, ifaces []net.Interface) (*Browser, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Browser{
		ifaces:  append([]net.Interface(nil), ifaces...),
		ctx:     ctx,
		cancel:  cancel,
		updates: make(chan DiscoveredService, 64),
	}, nil
}

func (b *Browser) Updates() <-chan DiscoveredService { return b.updates }

func (b *Browser) Start() error {
	options := make([]zeroconf.ClientOption, 0, 1)
	if len(b.ifaces) > 0 {
		options = append(options, zeroconf.SelectIfaces(b.ifaces))
	}
	queryEntries := make(chan *zeroconf.ServiceEntry, 64)
	oscEntries := make(chan *zeroconf.ServiceEntry, 64)
	for _, target := range []struct {
		service string
		entries chan *zeroconf.ServiceEntry
	}{
		{ServiceOSCQuery, queryEntries},
		{ServiceOSC, oscEntries},
	} {
		resolver, err := zeroconf.NewResolver(options...)
		if err != nil {
			b.cancel()
			return fmt.Errorf("create mDNS resolver: %w", err)
		}
		if err := resolver.Browse(b.ctx, target.service, ServiceDomain, target.entries); err != nil {
			b.cancel()
			return fmt.Errorf("browse %s: %w", target.service, err)
		}
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer close(b.updates)
		for queryEntries != nil || oscEntries != nil {
			var entry *zeroconf.ServiceEntry
			select {
			case <-b.ctx.Done():
				return
			case item, ok := <-queryEntries:
				if !ok {
					queryEntries = nil
					continue
				}
				entry = item
			case item, ok := <-oscEntries:
				if !ok {
					oscEntries = nil
					continue
				}
				entry = item
			}
			if entry == nil {
				continue
			}
			service := DiscoveredService{
				Instance: entry.Instance,
				Service:  entry.Service,
				HostName: entry.HostName,
				Port:     entry.Port,
				Text:     append([]string(nil), entry.Text...),
				LastSeen: time.Now(),
			}
			for _, ip := range entry.AddrIPv4 {
				service.IPv4 = append(service.IPv4, append(net.IP(nil), ip...))
			}
			for _, ip := range entry.AddrIPv6 {
				service.IPv6 = append(service.IPv6, append(net.IP(nil), ip...))
			}
			select {
			case b.updates <- service:
			case <-b.ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (b *Browser) Close() {
	if b == nil {
		return
	}
	b.cancel()
	b.wg.Wait()
}
