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
	"unicode/utf8"
)

type ControllerConfig struct {
	ServiceName string
	HTTPBind    string
	OSCBind     string
	Interfaces  []net.Interface
	CatalogMode CatalogMode

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

type controllerQueryClient interface {
	HostInfo(context.Context, string) (HostInfo, error)
	Node(context.Context, string, string) (*QueryNode, error)
	WatchChanges(context.Context, string, HostInfo, func(QueryCommand)) error
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
	runtime       *sendRuntime
	queryClient   controllerQueryClient

	servicesMu sync.Mutex
	services   map[string]DiscoveredService

	activeMu sync.Mutex
	active   *activeVRChat

	generation    atomic.Uint64
	avatarID      atomic.Value // string
	avatarChanges avatarChangeMailbox

	refreshCh chan refreshRequest
	failedCh  chan error
	events    chan ControllerEvent
	closeOnce sync.Once

	statusMu  sync.Mutex
	running   bool
	lastError string

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
	if config.CatalogMode != CatalogOSCQuery && config.CatalogMode != CatalogExternal {
		return nil, fmt.Errorf("invalid OSC catalog mode %d", config.CatalogMode)
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
	sender := newParameterSender(nil, config.Sender)
	controller := &Controller{
		config:        config,
		specs:         specs,
		source:        source,
		sender:        sender,
		runtime:       newSendRuntime(sender, config.CatalogMode, source),
		services:      make(map[string]DiscoveredService),
		avatarChanges: newAvatarChangeMailbox(),
		refreshCh:     make(chan refreshRequest, 8),
		failedCh:      make(chan error, 4),
		events:        make(chan ControllerEvent, 64),
		onIncoming:    onIncoming,
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
	c.sender.transport = udp
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
	c.setRunning(true)
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
	c.setRunning(false)
	c.closeOnce.Do(func() {
		c.avatarChanges.close()
		close(c.events)
	})
	return nil
}

func (c *Controller) Events() <-chan ControllerEvent { return c.events }

func (c *Controller) AvatarChanges(ctx context.Context) <-chan AvatarChange {
	return c.avatarChanges.subscribe(ctx)
}

func (c *Controller) ClearRuntime() {
	c.runtime.clear()
}

func (c *Controller) InstallCatalog(catalog *Catalog) error {
	return c.runtime.installExternal(catalog)
}

func (c *Controller) Publish(generation uint64, source ValueSource) error {
	return c.runtime.publish(generation, source)
}

func (c *Controller) Catalog() *Catalog {
	return c.runtime.catalog()
}

func (c *Controller) Status() OSCStatus {
	c.statusMu.Lock()
	running := c.running
	lastError := c.lastError
	c.statusMu.Unlock()

	c.activeMu.Lock()
	connected := c.active != nil
	c.activeMu.Unlock()

	status := OSCStatus{Running: running, Connected: connected, LastError: lastError}
	if c.udp != nil {
		if target := c.udp.Target(); target != nil {
			status.HasTarget = true
			status.Target = OSCTarget{Host: target.IP.String(), Port: target.Port}
		}
	}
	return status
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
			c.publishAvatarChange(avatarID)
			c.emit(ControllerEvent{Kind: EventAvatarChanged, AvatarID: avatarID})
			if c.config.CatalogMode == CatalogOSCQuery {
				c.enqueueAvatarRefreshes()
			}
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
	if c.config.CatalogMode == CatalogExternal {
		c.runtime.resetChangeDetection()
	}

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
	if c.config.CatalogMode == CatalogOSCQuery {
		c.runtime.clear()
	}
	c.recordError(reason)
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
	if c.config.CatalogMode == CatalogExternal {
		_, err := c.queryClient.Node(queryCtx, active.baseURL, "/")
		return err
	}
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
	previous := c.runtime.catalog()
	changed := previous == nil || previous.Hash != catalog.Hash
	if changed {
		if err := c.runtime.installQuery(catalog); err != nil {
			return err
		}
	} else if force {
		// A different avatar can expose the same set of addresses. Re-send all values.
		c.runtime.resetChangeDetection()
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
	ticker := time.NewTicker(c.config.SendInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.udp.Target() == nil || c.runtime.catalog() == nil {
				continue
			}
			if err := c.runtime.send(); err != nil {
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
	c.recordError(event.Err)
	event.Time = time.Now()
	event.Catalog = event.Catalog.Clone()
	select {
	case c.events <- event:
	default:
	}
}

func (c *Controller) publishAvatarChange(avatarID string) {
	c.avatarChanges.publish(avatarID)
}

func (c *Controller) setRunning(running bool) {
	c.statusMu.Lock()
	c.running = running
	c.statusMu.Unlock()
}

func (c *Controller) recordError(err error) {
	if err == nil {
		return
	}
	message := strings.ToValidUTF8(err.Error(), "\uFFFD")
	message = strings.Join(strings.Fields(message), " ")
	const maxErrorBytes = 512
	if len(message) > maxErrorBytes {
		message = message[:maxErrorBytes]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
	}
	c.statusMu.Lock()
	c.lastError = message
	c.statusMu.Unlock()
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
