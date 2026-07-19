package osc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type QueryClient struct {
	httpClient *http.Client
}

func NewQueryClient(timeout time.Duration) *QueryClient {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &QueryClient{
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *QueryClient) HostInfo(ctx context.Context, baseURL string) (HostInfo, error) {
	requestURL, err := appendRawQuery(baseURL, "HOST_INFO")
	if err != nil {
		return HostInfo{}, err
	}
	var raw json.RawMessage
	if err := c.getJSON(ctx, requestURL, &raw); err != nil {
		return HostInfo{}, err
	}

	// The proposal returns HOST_INFO directly. Some implementations wrap it.
	var direct HostInfo
	if err := json.Unmarshal(raw, &direct); err == nil {
		if direct.Name != "" || direct.OSCIP != "" || direct.OSCPort != 0 ||
			direct.OSCTransport != "" || direct.WSIP != "" || direct.WSPort != 0 ||
			direct.Extensions != nil {
			return direct, nil
		}
	}
	var wrapped struct {
		HostInfo HostInfo `json:"HOST_INFO"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return HostInfo{}, fmt.Errorf("decode OSCQuery HOST_INFO: %w", err)
	}
	return wrapped.HostInfo, nil
}

func (c *QueryClient) Node(ctx context.Context, baseURL, fullPath string) (*QueryNode, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse OSCQuery base URL: %w", err)
	}
	u.Path = cleanOSCPath(fullPath)
	u.RawQuery = ""
	var node QueryNode
	if err := c.getJSON(ctx, u.String(), &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (c *QueryClient) getJSON(ctx context.Context, requestURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("OSCQuery node not found: %w", ErrNodeNotFound)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OSCQuery HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode OSCQuery response: %w", err)
	}
	return nil
}

var ErrNodeNotFound = errors.New("OSCQuery node not found")

func (c *QueryClient) WatchChanges(
	ctx context.Context,
	baseURL string,
	hostInfo HostInfo,
	onCommand func(QueryCommand),
) error {
	wsURL, err := websocketURL(baseURL, hostInfo)
	if err != nil {
		return err
	}
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: c.httpClient,
	})
	if err != nil {
		return fmt.Errorf("connect OSCQuery websocket: %w", err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(256 * 1024)

	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if messageType != websocket.MessageText {
			continue
		}
		var command QueryCommand
		if err := json.Unmarshal(payload, &command); err != nil {
			continue
		}
		if onCommand != nil {
			onCommand(command)
		}
	}
}

func CandidateBaseURLs(service DiscoveredService) []string {
	if service.Port <= 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var result []string
	appendURL := func(host string) {
		if host == "" {
			return
		}
		value := "http://" + net.JoinHostPort(host, fmt.Sprintf("%d", service.Port))
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	if serviceIsLocal(service) {
		appendURL("127.0.0.1")
	}
	for _, ip := range service.IPv4 {
		appendURL(ip.String())
	}
	for _, ip := range service.IPv6 {
		appendURL(ip.String())
	}
	if service.HostName != "" {
		appendURL(strings.TrimSuffix(service.HostName, "."))
	}
	return result
}

func serviceIsLocal(service DiscoveredService) bool {
	local := localIPSet()
	for _, ip := range service.Addresses() {
		if ip.IsLoopback() {
			return true
		}
		if _, ok := local[ip.String()]; ok {
			return true
		}
	}
	return false
}

func localIPSet() map[string]struct{} {
	result := make(map[string]struct{})
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip != nil {
				result[ip.String()] = struct{}{}
			}
		}
	}
	return result
}

func appendRawQuery(baseURL, query string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u.Path = "/"
	u.RawQuery = query
	return u.String(), nil
}

func websocketURL(baseURL string, hostInfo HostInfo) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	port := u.Port()
	if hostInfo.WSIP != "" {
		host = hostInfo.WSIP
	}
	if hostInfo.WSPort != 0 {
		port = fmt.Sprintf("%d", hostInfo.WSPort)
	}
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	secure := strings.EqualFold(u.Scheme, "https")
	if secure {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Host = net.JoinHostPort(host, port)
	u.Path = "/"
	u.RawQuery = ""
	return u.String(), nil
}

func SortServices(services []DiscoveredService) {
	sort.SliceStable(services, func(i, j int) bool {
		leftVRChat := strings.Contains(strings.ToLower(services[i].Instance), "vrchat")
		rightVRChat := strings.Contains(strings.ToLower(services[j].Instance), "vrchat")
		if leftVRChat != rightVRChat {
			return leftVRChat
		}
		leftLocal := serviceIsLocal(services[i])
		rightLocal := serviceIsLocal(services[j])
		if leftLocal != rightLocal {
			return leftLocal
		}
		return services[i].Instance < services[j].Instance
	})
}
