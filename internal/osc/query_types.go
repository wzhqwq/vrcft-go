package osc

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

type Access int

const (
	AccessNone      Access = 0
	AccessReadOnly  Access = 1
	AccessWriteOnly Access = 2
	AccessReadWrite Access = 3
)

type Range struct {
	Min  *float64 `json:"MIN,omitempty"`
	Max  *float64 `json:"MAX,omitempty"`
	Vals []any    `json:"VALS,omitempty"`
}

type QueryNode struct {
	FullPath    string                `json:"FULL_PATH"`
	Type        string                `json:"TYPE,omitempty"`
	Access      *Access               `json:"ACCESS,omitempty"`
	Value       []any                 `json:"VALUE,omitempty"`
	Range       []Range               `json:"RANGE,omitempty"`
	Description string                `json:"DESCRIPTION,omitempty"`
	Contents    map[string]*QueryNode `json:"CONTENTS,omitempty"`
}

type HostInfo struct {
	Name         string          `json:"NAME,omitempty"`
	Extensions   map[string]bool `json:"EXTENSIONS,omitempty"`
	OSCIP        string          `json:"OSC_IP,omitempty"`
	OSCPort      int             `json:"OSC_PORT,omitempty"`
	OSCTransport string          `json:"OSC_TRANSPORT,omitempty"`
	WSIP         string          `json:"WS_IP,omitempty"`
	WSPort       int             `json:"WS_PORT,omitempty"`
}

func NewQueryRoot() *QueryNode {
	access := AccessNone
	return &QueryNode{
		FullPath:    "/",
		Access:      &access,
		Description: "root node",
		Contents:    make(map[string]*QueryNode),
	}
}

func NewContainer(fullPath string) *QueryNode {
	access := AccessNone
	return &QueryNode{
		FullPath: cleanOSCPath(fullPath),
		Access:   &access,
		Contents: make(map[string]*QueryNode),
	}
}

func NewMethod(fullPath, typ string, access Access, value ...any) *QueryNode {
	return &QueryNode{
		FullPath: cleanOSCPath(fullPath),
		Type:     typ,
		Access:   &access,
		Value:    value,
	}
}

func (n *QueryNode) Clone() *QueryNode {
	if n == nil {
		return nil
	}
	clone := *n
	if n.Access != nil {
		a := *n.Access
		clone.Access = &a
	}
	clone.Value = append([]any(nil), n.Value...)
	clone.Range = append([]Range(nil), n.Range...)
	if n.Contents != nil {
		clone.Contents = make(map[string]*QueryNode, len(n.Contents))
		for name, child := range n.Contents {
			clone.Contents[name] = child.Clone()
		}
	}
	return &clone
}

func (n *QueryNode) Add(node *QueryNode) error {
	if n == nil || node == nil {
		return fmt.Errorf("nil OSCQuery node")
	}
	fullPath := cleanOSCPath(node.FullPath)
	if fullPath == "/" {
		return fmt.Errorf("cannot add root as child")
	}
	segments := splitOSCPath(fullPath)
	current := n
	for i, segment := range segments {
		if current.Contents == nil {
			current.Contents = make(map[string]*QueryNode)
		}
		child, exists := current.Contents[segment]
		last := i == len(segments)-1
		if last {
			if exists {
				return fmt.Errorf("OSCQuery path already exists: %s", fullPath)
			}
			copyNode := node.Clone()
			copyNode.FullPath = fullPath
			current.Contents[segment] = copyNode
			return nil
		}
		if !exists {
			containerPath := "/" + strings.Join(segments[:i+1], "/")
			child = NewContainer(containerPath)
			current.Contents[segment] = child
		}
		current = child
	}
	return nil
}

func (n *QueryNode) Find(fullPath string) *QueryNode {
	if n == nil {
		return nil
	}
	fullPath = cleanOSCPath(fullPath)
	if fullPath == "/" {
		return n
	}
	current := n
	for _, segment := range splitOSCPath(fullPath) {
		if current.Contents == nil {
			return nil
		}
		current = current.Contents[segment]
		if current == nil {
			return nil
		}
	}
	return current
}

func (n *QueryNode) FlattenMethods() []*QueryNode {
	if n == nil {
		return nil
	}
	var methods []*QueryNode
	var walk func(*QueryNode)
	walk = func(current *QueryNode) {
		if current == nil {
			return
		}
		if current.Type != "" {
			methods = append(methods, current)
		}
		names := make([]string, 0, len(current.Contents))
		for name := range current.Contents {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			walk(current.Contents[name])
		}
	}
	walk(n)
	return methods
}

func (n *QueryNode) MarshalJSON() ([]byte, error) {
	type alias QueryNode
	if n == nil {
		return []byte("null"), nil
	}
	return json.Marshal((*alias)(n))
}

func cleanOSCPath(value string) string {
	if value == "" {
		return "/"
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(value, "/"))
	if cleaned == "." || cleaned == "" {
		return "/"
	}
	return cleaned
}

func splitOSCPath(value string) []string {
	value = strings.Trim(cleanOSCPath(value), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}
