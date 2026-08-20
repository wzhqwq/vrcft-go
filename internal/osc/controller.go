package osc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ControllerConfig struct {
	ServiceName string
	HTTPBind    string
	OSCBind     string
	Interfaces  []net.Interface

	PreferredVRChatService string
	QueryTimeout           time.Duration
	QueryPollInterval      time.Duration
	DiscoveryRetryInterval time.Duration
	SendInterval           time.Duration

	Sender SenderConfig
}

type EventKind string

const (
	EventVRChatConnected    EventKind = "vrchat_connected"
	EventVRChatDisconnected EventKind = "vrchat_disconnected"
	EventCatalogUpdated     EventKind = "catalog_updated"
	EventAvatarChanged      EventKind = "avatar_changed"
	EventError              EventKind = "error"
)

type ControllerEvent struct {
	Kind      EventKind
	Time      time.Time
	Message   string
	Service   string
	AvatarID  string
	Catalog   *Catalog
	OSCTarget *net.UDPAddr
	Err       error
}

type activeVRChat struct {
	service  DiscoveredService
	baseURL  string
	hostInfo HostInfo
	cancel   context.CancelFunc
}

type refreshRequest struct {
	reason string
	force  bool
}

type Controller struct {
	config ControllerConfig
	specs  *ParameterCatalog
	source ValueSource

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	queryListener net.Listener
	queryServer   *QueryServer
	udp           *UDPTransport
	advertiser    *Advertiser
	browser       *Browser
	sender        *ParameterSender
	queryClient   *QueryClient

	servicesMu sync.Mutex
	services   map[string]DiscoveredService

	activeMu sync.Mutex
	active   *activeVRChat

	catalog    atomic.Pointer[Catalog]
	generation atomic.Uint64
	avatarID   atomic.Value // string

	refreshCh chan refreshRequest
	failedCh  chan error
	events    chan ControllerEvent

	onIncoming func(Message, *net.UDPAddr)
}

func NewController(
	config ControllerConfig,
	source ValueSource,
	onIncoming func(Message, *net.UDPAddr),
) (*Controller, error) {
	specs, err := NewVRCFTParameterCatalog()
	if err != nil {
		return nil, fmt.Errorf("compile VRCFT OSC parameter definitions: %w", err)
	}
	return NewControllerWithCatalog(config, source, specs, onIncoming)
}

func NewControllerWithCatalog(
	config ControllerConfig,
	source ValueSource,
	specs *ParameterCatalog,
	onIncoming func(Message, *net.UDPAddr),
) (*Controller, error) {
	if specs == nil {
		return nil, errors.New("OSC parameter catalog is nil")
	}
	if config.ServiceName == "" {
		config.ServiceName = "VRCFaceTracking-Go"
	}
	if config.HTTPBind == "" {
		config.HTTPBind = "0.0.0.0:0"
	}
	if config.OSCBind == "" {
		config.OSCBind = "0.0.0.0:0"
	}
	if config.QueryTimeout <= 0 {
		config.QueryTimeout = 3 * time.Second
	}
	if config.QueryPollInterval <= 0 {
		config.QueryPollInterval = 2 * time.Second
	}
	if config.DiscoveryRetryInterval <= 0 {
		config.DiscoveryRetryInterval = 5 * time.Second
	}
	if config.SendInterval <= 0 {
		config.SendInterval = 10 * time.Millisecond
	}
	controller := &Controller{
		config:     config,
		specs:      specs,
		source:     source,
		services:   make(map[string]DiscoveredService),
		refreshCh:  make(chan refreshRequest, 8),
		failedCh:   make(chan error, 4),
		events:     make(chan ControllerEvent, 64),
		onIncoming: onIncoming,
	}
	controller.avatarID.Store("")
	return controller, nil
}

func (c *Controller) Start(parent context.Context) error {
	if c.ctx != nil {
		return errors.New("OSC controller is already started")
	}
	if parent == nil {
		parent = context.Background()
	}
	c.ctx, c.cancel = context.WithCancel(parent)

	queryListener, err := ListenQueryTCP(c.config.HTTPBind)
	if err != nil {
		c.cancel()
		return err
	}
	c.queryListener = queryListener

	udp, err := ListenUDP(c.config.OSCBind)
	if err != nil {
		_ = queryListener.Close()
		c.cancel()
		return err
	}
	c.udp = udp
	c.sender = NewParameterSender(udp, c.config.Sender)
	c.queryClient = NewQueryClient(c.config.QueryTimeout)

	queryPort := queryListener.Addr().(*net.TCPAddr).Port
	oscPort := udp.LocalAddr().Port
	hostInfo := HostInfo{
		Name:         c.config.ServiceName,
		OSCPort:      oscPort,
		OSCTransport: "UDP",
		WSPort:       queryPort,
	}
	c.queryServer = NewQueryServer(queryListener, hostInfo, defaultReceiverTree())
	if err := c.queryServer.Start(c.ctx); err != nil {
		_ = udp.Close()
		_ = queryListener.Close()
		c.cancel()
		return err
	}

	advertiser, err := Advertise(c.config.ServiceName, queryPort, oscPort, c.config.Interfaces)
	if err != nil {
		_ = c.queryServer.Close(context.Background())
		_ = udp.Close()
		c.cancel()
		return err
	}
	c.advertiser = advertiser

	browser, err := NewBrowser(c.ctx, c.config.Interfaces)
	if err != nil {
		c.advertiser.Close()
		_ = c.queryServer.Close(context.Background())
		_ = udp.Close()
		c.cancel()
		return err
	}
	c.browser = browser
	if err := browser.Start(); err != nil {
		browser.Close()
		c.advertiser.Close()
		_ = c.queryServer.Close(context.Background())
		_ = udp.Close()
		c.cancel()
		return err
	}

	c.wg.Add(3)
	go c.runUDPReceiver()
	go c.runDiscovery()
	go c.runSendLoop()
	return nil
}

func (c *Controller) Close(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.clearActive(nil)
	if c.browser != nil {
		c.browser.Close()
	}
	if c.advertiser != nil {
		c.advertiser.Close()
	}
	if c.udp != nil {
		_ = c.udp.Close()
	}
	if c.queryServer != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		_ = c.queryServer.Close(ctx)
	}
	c.wg.Wait()
	close(c.events)
	return nil
}

func (c *Controller) Events() <-chan ControllerEvent { return c.events }

func (c *Controller) Catalog() *Catalog {
	return c.catalog.Load().Clone()
}

func (c *Controller) AvatarID() string {
	value, _ := c.avatarID.Load().(string)
	return value
}

func (c *Controller) SetAdvertisedTree(root *QueryNode, changedPath string) {
	if c.queryServer != nil {
		c.queryServer.ReplaceRoot(root, changedPath)
	}
}

func (c *Controller) TriggerParameterRefresh(force bool, reason string) {
	c.enqueueRefresh(refreshRequest{reason: reason, force: force})
}

func (c *Controller) runUDPReceiver() {
	defer c.wg.Done()
	err := c.udp.Serve(c.ctx, func(message Message, remote *net.UDPAddr) {
		if message.Address == "/avatar/change" && len(message.Args) > 0 && message.Args[0].Kind == ValueString {
			avatarID := message.Args[0].Str
			c.avatarID.Store(avatarID)
			c.emit(ControllerEvent{Kind: EventAvatarChanged, AvatarID: avatarID})
			c.enqueueAvatarRefreshes()
		}
		if c.onIncoming != nil {
			c.onIncoming(message, remote)
		}
	})
	if err != nil && c.ctx.Err() == nil {
		c.emit(ControllerEvent{Kind: EventError, Message: "OSC receiver stopped", Err: err})
	}
}

func (c *Controller) runDiscovery() {
	defer c.wg.Done()
	retryTicker := time.NewTicker(c.config.DiscoveryRetryInterval)
	defer retryTicker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case service, ok := <-c.browser.Updates():
			if !ok {
				return
			}
			if service.Instance == c.config.ServiceName {
				continue
			}
			c.servicesMu.Lock()
			c.services[serviceKey(service)] = service
			c.servicesMu.Unlock()
			if normalizedServiceType(service.Service) == ServiceOSCQuery && c.currentActive() == nil {
				c.connectBest()
			}
		case err := <-c.failedCh:
			c.clearActive(err)
		case <-retryTicker.C:
			if c.currentActive() == nil {
				c.connectBest()
			}
		}
	}
}

func (c *Controller) connectBest() {
	c.servicesMu.Lock()
	var candidates []DiscoveredService
	for _, service := range c.services {
		if normalizedServiceType(service.Service) != ServiceOSCQuery {
			continue
		}
		if time.Since(service.LastSeen) > 5*time.Minute {
			continue
		}
		if c.config.PreferredVRChatService != "" && service.Instance != c.config.PreferredVRChatService {
			continue
		}
		candidates = append(candidates, service)
	}
	c.servicesMu.Unlock()
	SortServices(candidates)

	for _, service := range candidates {
		active, err := c.probeService(service)
		if err != nil {
			continue
		}
		c.setActive(active)
		return
	}
}

func (c *Controller) probeService(service DiscoveredService) (*activeVRChat, error) {
	for _, baseURL := range CandidateBaseURLs(service) {
		ctx, cancel := context.WithTimeout(c.ctx, c.config.QueryTimeout)
		hostInfo, err := c.queryClient.HostInfo(ctx, baseURL)
		if err != nil {
			cancel()
			continue
		}
		root, err := c.queryClient.Node(ctx, baseURL, "/")
		cancel()
		if err != nil {
			continue
		}
		isVRChat := strings.Contains(strings.ToLower(service.Instance), "vrchat") ||
			strings.Contains(strings.ToLower(hostInfo.Name), "vrchat") ||
			root.Find("/avatar") != nil
		if !isVRChat {
			continue
		}
		target, err := c.resolveOSCTarget(service, baseURL, hostInfo)
		if err != nil {
			continue
		}
		c.udp.SetTarget(target)
		return &activeVRChat{service: service, baseURL: baseURL, hostInfo: hostInfo}, nil
	}
	return nil, fmt.Errorf("service %s is not a usable VRChat OSCQuery service", service.Instance)
}

func (c *Controller) resolveOSCTarget(service DiscoveredService, baseURL string, hostInfo HostInfo) (*net.UDPAddr, error) {
	if hostInfo.OSCTransport != "" && !strings.EqualFold(hostInfo.OSCTransport, "UDP") {
		return nil, fmt.Errorf("unsupported OSC transport %q", hostInfo.OSCTransport)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	host := parsed.Hostname()
	if hostInfo.OSCIP != "" {
		host = hostInfo.OSCIP
	}
	port := hostInfo.OSCPort
	if port == 0 {
		if paired, ok := c.findPairedOSCService(service.Instance); ok {
			port = paired.Port
			if hostInfo.OSCIP == "" && len(paired.IPv4) > 0 {
				host = paired.IPv4[0].String()
			}
		} else {
			port = service.Port // Proposal fallback: same host/port as OSCQuery.
		}
	}
	return net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}

func (c *Controller) findPairedOSCService(instance string) (DiscoveredService, bool) {
	c.servicesMu.Lock()
	defer c.servicesMu.Unlock()
	for _, service := range c.services {
		if service.Instance == instance && normalizedServiceType(service.Service) == ServiceOSC {
			return service, true
		}
	}
	return DiscoveredService{}, false
}

func (c *Controller) setActive(active *activeVRChat) {
	c.activeMu.Lock()
	if c.active != nil && c.active.cancel != nil {
		c.active.cancel()
	}
	workerCtx, cancel := context.WithCancel(c.ctx)
	active.cancel = cancel
	c.active = active
	c.activeMu.Unlock()

	c.emit(ControllerEvent{
		Kind:      EventVRChatConnected,
		Service:   active.service.Instance,
		OSCTarget: c.udp.Target(),
	})

	c.wg.Add(1)
	go c.runActiveVRChat(workerCtx, active)
}

func (c *Controller) clearActive(reason error) {
	c.activeMu.Lock()
	active := c.active
	if active != nil && active.cancel != nil {
		active.cancel()
	}
	c.active = nil
	c.activeMu.Unlock()
	if c.udp != nil {
		c.udp.SetTarget(nil)
	}
	c.catalog.Store(nil)
	if c.sender != nil {
		c.sender.SetCatalog(nil)
	}
	if active != nil {
		c.emit(ControllerEvent{
			Kind:    EventVRChatDisconnected,
			Service: active.service.Instance,
			Err:     reason,
		})
	}
}

func (c *Controller) currentActive() *activeVRChat {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	return c.active
}

func (c *Controller) runActiveVRChat(ctx context.Context, active *activeVRChat) {
	defer c.wg.Done()
	pollTicker := time.NewTicker(c.config.QueryPollInterval)
	defer pollTicker.Stop()

	if supportsPathNotifications(active.hostInfo) {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			err := c.queryClient.WatchChanges(ctx, active.baseURL, active.hostInfo, func(command QueryCommand) {
				switch strings.ToUpper(command.Command) {
				case "PATH_CHANGED", "PATH_ADDED", "PATH_REMOVED", "PATH_RENAMED":
					if commandAffectsAvatar(command) {
						c.enqueueRefresh(refreshRequest{reason: command.Command})
					}
				}
			})
			if err != nil && ctx.Err() == nil {
				// WebSocket support is optional; polling continues after failure.
				c.emit(ControllerEvent{Kind: EventError, Message: "OSCQuery websocket watcher stopped", Err: err})
			}
		}()
	}

	c.enqueueRefresh(refreshRequest{reason: "connected", force: true})
	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-c.refreshCh:
			err := c.refreshCatalog(ctx, active, request.force)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					select {
					case c.failedCh <- err:
					default:
					}
					return
				}
				continue
			}
			consecutiveFailures = 0
		case <-pollTicker.C:
			err := c.refreshCatalog(ctx, active, false)
			if err != nil {
				consecutiveFailures++
				if consecutiveFailures >= 3 {
					select {
					case c.failedCh <- err:
					default:
					}
					return
				}
			} else {
				consecutiveFailures = 0
			}
		}
	}
}

func (c *Controller) refreshCatalog(ctx context.Context, active *activeVRChat, force bool) error {
	queryCtx, cancel := context.WithTimeout(ctx, c.config.QueryTimeout)
	defer cancel()
	root, err := c.queryClient.Node(queryCtx, active.baseURL, "/avatar/parameters")
	if err != nil {
		if !errors.Is(err, ErrNodeNotFound) {
			return err
		}
		root, err = c.queryClient.Node(queryCtx, active.baseURL, "/")
		if err != nil {
			return err
		}
	}

	generation := c.generation.Add(1)
	catalog, err := BuildCatalog(root, c.specs, generation)
	if err != nil {
		return err
	}
	previous := c.catalog.Load()
	changed := previous == nil || previous.Hash != catalog.Hash
	if changed {
		c.catalog.Store(catalog)
		c.sender.SetCatalog(catalog)
	} else if force {
		// A different avatar can expose the same set of addresses. Re-send all values.
		c.sender.ResetChangeDetection()
	}
	if changed || force {
		c.emit(ControllerEvent{
			Kind:    EventCatalogUpdated,
			Service: active.service.Instance,
			Catalog: catalog,
		})
	}
	return nil
}

func (c *Controller) runSendLoop() {
	defer c.wg.Done()
	if c.source == nil {
		<-c.ctx.Done()
		return
	}
	ticker := time.NewTicker(c.config.SendInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.udp.Target() == nil || c.catalog.Load() == nil {
				continue
			}
			if err := c.sender.Send(c.source); err != nil {
				c.emit(ControllerEvent{Kind: EventError, Message: "send OSC parameters", Err: err})
			}
		}
	}
}

func (c *Controller) enqueueAvatarRefreshes() {
	c.enqueueRefresh(refreshRequest{reason: "avatar_change", force: true})
	for _, delay := range []time.Duration{250 * time.Millisecond, 750 * time.Millisecond, 1500 * time.Millisecond} {
		delay := delay
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-c.ctx.Done():
				return
			case <-timer.C:
				c.enqueueRefresh(refreshRequest{reason: "avatar_change_retry", force: true})
			}
		}()
	}
}

func (c *Controller) enqueueRefresh(request refreshRequest) {
	select {
	case c.refreshCh <- request:
	default:
		// Coalesce bursts of PATH_CHANGED and avatar-change retries.
	}
}

func (c *Controller) emit(event ControllerEvent) {
	event.Time = time.Now()
	event.Catalog = event.Catalog.Clone()
	select {
	case c.events <- event:
	default:
	}
}

func defaultReceiverTree() *QueryNode {
	root := NewQueryRoot()
	_ = root.Add(NewMethod("/avatar/change", "s", AccessWriteOnly))
	_ = root.Add(NewContainer("/avatar/parameters"))
	return root
}

func supportsPathNotifications(info HostInfo) bool {
	for _, name := range []string{"PATH_CHANGED", "PATH_ADDED", "PATH_REMOVED", "PATH_RENAMED"} {
		if info.Extensions[name] {
			return true
		}
	}
	return false
}

func commandAffectsAvatar(command QueryCommand) bool {
	switch data := command.Data.(type) {
	case string:
		return data == "/" || strings.HasPrefix(cleanOSCPath(data), "/avatar")
	case map[string]any:
		for _, key := range []string{"OLD", "NEW"} {
			if value, ok := data[key].(string); ok && (value == "/" || strings.HasPrefix(cleanOSCPath(value), "/avatar")) {
				return true
			}
		}
	}
	return false
}

func serviceKey(service DiscoveredService) string {
	return normalizedServiceType(service.Service) + "|" + service.Instance
}

func normalizedServiceType(value string) string {
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ".local")
	return value
}
