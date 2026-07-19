package osc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type QueryCommand struct {
	Command string `json:"COMMAND"`
	Data    any    `json:"DATA"`
}

type QueryServer struct {
	listener net.Listener
	server   *http.Server
	hostInfo HostInfo

	rootMu sync.RWMutex
	root   *QueryNode

	clientsMu sync.Mutex
	clients   map[*websocket.Conn]struct{}

	ctx    context.Context
	cancel context.CancelFunc
}

func NewQueryServer(listener net.Listener, hostInfo HostInfo, root *QueryNode) *QueryServer {
	if root == nil {
		root = NewQueryRoot()
	}
	if hostInfo.Extensions == nil {
		hostInfo.Extensions = map[string]bool{}
	}
	// We implement change notifications, but deliberately do not advertise LISTEN.
	hostInfo.Extensions["ACCESS"] = true
	hostInfo.Extensions["VALUE"] = true
	hostInfo.Extensions["RANGE"] = true
	hostInfo.Extensions["DESCRIPTION"] = true
	hostInfo.Extensions["PATH_CHANGED"] = true
	hostInfo.Extensions["PATH_ADDED"] = true
	hostInfo.Extensions["PATH_REMOVED"] = true
	hostInfo.Extensions["PATH_RENAMED"] = true

	return &QueryServer{
		listener: listener,
		hostInfo: hostInfo,
		root:     root.Clone(),
		clients:  make(map[*websocket.Conn]struct{}),
	}
}

func (s *QueryServer) Addr() net.Addr {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *QueryServer) Start(parent context.Context) error {
	if s.listener == nil {
		return errors.New("OSCQuery HTTP listener is nil")
	}
	if parent == nil {
		parent = context.Background()
	}
	s.ctx, s.cancel = context.WithCancel(parent)
	s.server = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		<-s.ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()

	go func() {
		_ = s.server.Serve(s.listener)
	}()
	return nil
}

func (s *QueryServer) Close(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	s.clientsMu.Lock()
	for client := range s.clients {
		_ = client.Close(websocket.StatusGoingAway, "server shutting down")
	}
	s.clients = make(map[*websocket.Conn]struct{})
	s.clientsMu.Unlock()

	if s.server == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.server.Shutdown(ctx)
}

func (s *QueryServer) Root() *QueryNode {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.root.Clone()
}

func (s *QueryServer) ReplaceRoot(root *QueryNode, changedPath string) {
	if root == nil {
		root = NewQueryRoot()
	}
	s.rootMu.Lock()
	s.root = root.Clone()
	s.rootMu.Unlock()
	if changedPath == "" {
		changedPath = "/"
	}
	s.Broadcast(QueryCommand{Command: "PATH_CHANGED", Data: cleanOSCPath(changedPath)})
}

func (s *QueryServer) Broadcast(command QueryCommand) {
	payload, err := json.Marshal(command)
	if err != nil {
		return
	}

	s.clientsMu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.Unlock()

	for _, client := range clients {
		writeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := client.Write(writeCtx, websocket.MessageText, payload)
		cancel()
		if err != nil {
			s.removeClient(client)
			_ = client.CloseNow()
		}
	}
}

func (s *QueryServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		s.serveWebSocket(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	attribute := firstQueryAttribute(r.URL.RawQuery)
	if attribute == "HOST_INFO" {
		writeJSON(w, http.StatusOK, s.hostInfo)
		return
	}

	s.rootMu.RLock()
	node := s.root.Find(r.URL.Path)
	if node != nil {
		node = node.Clone()
	}
	s.rootMu.RUnlock()
	if node == nil {
		http.NotFound(w, r)
		return
	}

	if attribute == "" {
		writeJSON(w, http.StatusOK, node)
		return
	}
	response, recognized, relevant := queryAttribute(node, attribute)
	if !recognized {
		http.Error(w, "unsupported OSCQuery attribute", http.StatusBadRequest)
		return
	}
	if !relevant {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{attribute: response})
}

func (s *QueryServer) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	client, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	client.SetReadLimit(64 * 1024)
	s.clientsMu.Lock()
	s.clients[client] = struct{}{}
	s.clientsMu.Unlock()

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		defer s.removeClient(client)
		defer client.CloseNow()
		for {
			messageType, payload, err := client.Read(ctx)
			if err != nil {
				return
			}
			if messageType != websocket.MessageText {
				continue
			}
			var command QueryCommand
			if json.Unmarshal(payload, &command) != nil {
				continue
			}
			// LISTEN/IGNORE are intentionally not implemented or advertised. We keep
			// the connection open so the client can receive path notifications.
		}
	}()
}

func (s *QueryServer) removeClient(client *websocket.Conn) {
	s.clientsMu.Lock()
	delete(s.clients, client)
	s.clientsMu.Unlock()
}

func queryAttribute(node *QueryNode, attribute string) (any, bool, bool) {
	switch attribute {
	case "FULL_PATH":
		return node.FullPath, true, true
	case "CONTENTS":
		return node.Contents, true, node.Contents != nil
	case "TYPE":
		return node.Type, true, node.Type != ""
	case "ACCESS":
		if node.Access == nil {
			return nil, true, false
		}
		return *node.Access, true, true
	case "VALUE":
		if node.Access != nil && (*node.Access&AccessReadOnly) == 0 {
			return nil, true, false
		}
		return node.Value, true, node.Value != nil
	case "RANGE":
		return node.Range, true, node.Range != nil
	case "DESCRIPTION":
		return node.Description, true, node.Description != ""
	default:
		return nil, false, false
	}
}

func firstQueryAttribute(raw string) string {
	if raw == "" {
		return ""
	}
	part := strings.Split(raw, "&")[0]
	part = strings.Split(part, "=")[0]
	return strings.ToUpper(part)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func ListenQueryTCP(address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen OSCQuery HTTP: %w", err)
	}
	return listener, nil
}
