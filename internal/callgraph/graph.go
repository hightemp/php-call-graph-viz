package callgraph

import (
	"fmt"
	"strings"
)

// CallType describes the kind of a method/function call.
type CallType int

const (
	CallInstance   CallType = iota // $obj->method()
	CallNullsafe                   // $obj?->method()
	CallStatic                     // Class::method()
	CallFunction                   // functionName()
	CallNew                        // new ClassName()
	CallUnresolved                 // could not resolve receiver type
	CallDynamic                    // $obj->$method() — dynamic name
)

func (ct CallType) String() string {
	switch ct {
	case CallInstance:
		return "instance"
	case CallNullsafe:
		return "nullsafe"
	case CallStatic:
		return "static"
	case CallFunction:
		return "function"
	case CallNew:
		return "new"
	case CallUnresolved:
		return "unresolved"
	case CallDynamic:
		return "dynamic"
	default:
		return "unknown"
	}
}

// NodeID uniquely identifies a graph node (a method or function).
type NodeID string

// MakeMethodID creates a NodeID for a class method.
func MakeMethodID(classFQN, method string) NodeID {
	return NodeID(classFQN + "::" + method)
}

// MakeFunctionID creates a NodeID for a standalone function.
func MakeFunctionID(functionFQN string) NodeID {
	return NodeID(functionFQN + "()")
}

// CallEdge represents a directed edge in the call graph.
type CallEdge struct {
	From     NodeID
	To       NodeID
	Type     CallType
	Label    string // human-readable label for the edge
	FileLine string // "file.php:42" where the call occurs
}

// CallNode represents a node in the call graph (a method or function).
type CallNode struct {
	ID        NodeID
	Label     string // short display label, e.g. "getUser"
	ClassFQN  string // "" for standalone functions
	ClassName string // short class name for display
	Method    string // method/function name
	IsStatic  bool
	Kind      string // "class", "interface", "trait", "enum", "function"
	File      string
}

// CallGraph is the internal call graph data structure.
type CallGraph struct {
	Nodes map[NodeID]*CallNode
	Edges []*CallEdge

	// Index: from node → list of edges.
	OutEdges map[NodeID][]*CallEdge
	InEdges  map[NodeID][]*CallEdge
}

// NewCallGraph creates an empty call graph.
func NewCallGraph() *CallGraph {
	return &CallGraph{
		Nodes:    make(map[NodeID]*CallNode),
		OutEdges: make(map[NodeID][]*CallEdge),
		InEdges:  make(map[NodeID][]*CallEdge),
	}
}

// AddNode adds a node if it doesn't already exist.
func (cg *CallGraph) AddNode(node *CallNode) {
	if _, exists := cg.Nodes[node.ID]; !exists {
		cg.Nodes[node.ID] = node
	}
}

// AddEdge adds a directed edge and ensures both nodes exist.
func (cg *CallGraph) AddEdge(edge *CallEdge) {
	cg.Edges = append(cg.Edges, edge)
	cg.OutEdges[edge.From] = append(cg.OutEdges[edge.From], edge)
	cg.InEdges[edge.To] = append(cg.InEdges[edge.To], edge)

	// Ensure placeholder nodes exist for endpoints.
	if _, exists := cg.Nodes[edge.From]; !exists {
		cg.Nodes[edge.From] = &CallNode{
			ID:    edge.From,
			Label: string(edge.From),
		}
	}
	if _, exists := cg.Nodes[edge.To]; !exists {
		cg.Nodes[edge.To] = &CallNode{
			ID:    edge.To,
			Label: string(edge.To),
		}
	}
}

// FilterByEntryPoints returns a new graph containing only nodes reachable
// from the given entry points within maxDepth steps (0 = unlimited).
func (cg *CallGraph) FilterByEntryPoints(entryPoints []NodeID, maxDepth int) *CallGraph {
	if len(entryPoints) == 0 {
		return cg
	}

	reachable := make(map[NodeID]bool)
	var bfs func(nodes []NodeID, depth int)
	bfs = func(nodes []NodeID, depth int) {
		if maxDepth > 0 && depth > maxDepth {
			return
		}
		var next []NodeID
		for _, nid := range nodes {
			if reachable[nid] {
				continue
			}
			reachable[nid] = true
			for _, e := range cg.OutEdges[nid] {
				if !reachable[e.To] {
					next = append(next, e.To)
				}
			}
		}
		if len(next) > 0 {
			bfs(next, depth+1)
		}
	}
	bfs(entryPoints, 1)

	filtered := NewCallGraph()
	for nid, node := range cg.Nodes {
		if reachable[nid] {
			filtered.AddNode(node)
		}
	}
	for _, edge := range cg.Edges {
		if reachable[edge.From] && reachable[edge.To] {
			filtered.AddEdge(edge)
		}
	}
	return filtered
}

// FilterByNamespaces returns a new graph keeping only nodes whose ClassFQN
// matches at least one include pattern and none of the exclude patterns.
func (cg *CallGraph) FilterByNamespaces(includes, excludes []string) *CallGraph {
	if len(includes) == 0 && len(excludes) == 0 {
		return cg
	}

	keep := func(nid NodeID) bool {
		node, ok := cg.Nodes[nid]
		if !ok {
			return false
		}
		fqn := node.ClassFQN
		if fqn == "" {
			fqn = string(nid)
		}

		if len(includes) > 0 {
			matched := false
			for _, inc := range includes {
				if strings.HasPrefix(fqn, inc) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}

		for _, exc := range excludes {
			if strings.HasPrefix(fqn, exc) {
				return false
			}
		}
		return true
	}

	filtered := NewCallGraph()
	for nid, node := range cg.Nodes {
		if keep(nid) {
			filtered.AddNode(node)
		}
	}
	for _, edge := range cg.Edges {
		if keep(edge.From) && keep(edge.To) {
			filtered.AddEdge(edge)
		}
	}
	return filtered
}

// FilterByClasses returns a new graph keeping only nodes whose ClassFQN or
// short ClassName matches the include/exclude lists.
func (cg *CallGraph) FilterByClasses(includes, excludes []string) *CallGraph {
	if len(includes) == 0 && len(excludes) == 0 {
		return cg
	}

	includeSet := toSet(includes)
	excludeSet := toSet(excludes)

	keep := func(nid NodeID) bool {
		node, ok := cg.Nodes[nid]
		if !ok {
			return false
		}
		fqn := node.ClassFQN

		if len(includeSet) > 0 {
			if !includeSet[fqn] && !includeSet[node.ClassName] {
				return false
			}
		}
		if excludeSet[fqn] || excludeSet[node.ClassName] {
			return false
		}
		return true
	}

	filtered := NewCallGraph()
	for nid, node := range cg.Nodes {
		if keep(nid) {
			filtered.AddNode(node)
		}
	}
	for _, edge := range cg.Edges {
		if keep(edge.From) && keep(edge.To) {
			filtered.AddEdge(edge)
		}
	}
	return filtered
}

// ExcludeMethods removes edges pointing to methods in the exclude list.
func (cg *CallGraph) ExcludeMethods(methods []string) *CallGraph {
	if len(methods) == 0 {
		return cg
	}
	excludeSet := toSet(methods)

	filtered := NewCallGraph()
	for _, node := range cg.Nodes {
		if !excludeSet[node.Method] {
			filtered.AddNode(node)
		}
	}
	for _, edge := range cg.Edges {
		fromNode := cg.Nodes[edge.From]
		toNode := cg.Nodes[edge.To]
		if fromNode != nil && toNode != nil && !excludeSet[fromNode.Method] && !excludeSet[toNode.Method] {
			filtered.AddEdge(edge)
		}
	}
	return filtered
}

// CollapseToClassLevel merges all methods of each class into a single node,
// producing a class→class dependency graph.
func (cg *CallGraph) CollapseToClassLevel() *CallGraph {
	collapsed := NewCallGraph()

	// Create one node per class.
	classNodes := make(map[string]*CallNode)
	for _, node := range cg.Nodes {
		key := node.ClassFQN
		if key == "" {
			key = node.Method // standalone function
		}
		if _, exists := classNodes[key]; !exists {
			cn := &CallNode{
				ID:        NodeID(key),
				Label:     shortName(key),
				ClassFQN:  key,
				ClassName: shortName(key),
				Kind:      node.Kind,
				File:      node.File,
			}
			classNodes[key] = cn
			collapsed.AddNode(cn)
		}
	}

	// Create one edge per unique class→class pair.
	edgeSet := make(map[string]bool)
	for _, edge := range cg.Edges {
		fromNode := cg.Nodes[edge.From]
		toNode := cg.Nodes[edge.To]
		if fromNode == nil || toNode == nil {
			continue
		}
		fromClass := fromNode.ClassFQN
		if fromClass == "" {
			fromClass = fromNode.Method
		}
		toClass := toNode.ClassFQN
		if toClass == "" {
			toClass = toNode.Method
		}
		if fromClass == toClass {
			continue // skip self-calls within same class
		}
		edgeKey := fmt.Sprintf("%s→%s", fromClass, toClass)
		if edgeSet[edgeKey] {
			continue
		}
		edgeSet[edgeKey] = true
		collapsed.AddEdge(&CallEdge{
			From:  NodeID(fromClass),
			To:    NodeID(toClass),
			Type:  edge.Type,
			Label: "",
		})
	}

	return collapsed
}

// Stats returns a human-readable summary of the graph.
func (cg *CallGraph) Stats() string {
	return fmt.Sprintf("%d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func shortName(fqn string) string {
	parts := strings.Split(fqn, "\\")
	return parts[len(parts)-1]
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
