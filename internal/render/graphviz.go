package render

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"

	"github.com/hightemp/php-call-graph-viz/internal/callgraph"
	"github.com/hightemp/php-call-graph-viz/internal/config"
)

// Render builds a Graphviz graph from the call graph and writes it to the output file.
func Render(cg *callgraph.CallGraph, cfg *config.Config) error {
	ctx := context.Background()

	g, err := graphviz.New(ctx)
	if err != nil {
		return fmt.Errorf("creating graphviz instance: %w", err)
	}
	defer g.Close()

	// Set layout engine.
	g.SetLayout(toLayout(cfg.Layout))

	graph, err := g.Graph(
		graphviz.WithDirectedType(graphviz.Directed),
		graphviz.WithName("CallGraph"),
	)
	if err != nil {
		return fmt.Errorf("creating graph: %w", err)
	}
	defer graph.Close()

	// Graph-level settings.
	graph.SetRankDir(toRankDir(cfg.RankDir))
	graph.SetFontName("Helvetica")
	graph.SetFontSize(11)
	graph.SetNodeSeparator(0.4)
	graph.SetRankSeparator(0.8)
	graph.SetCompound(true)

	// Render based on grouping mode.
	switch cfg.GroupBy {
	case "class":
		renderGroupedByClass(graph, cg, cfg)
	case "namespace":
		renderGroupedByNamespace(graph, cg, cfg)
	default:
		renderFlat(graph, cg, cfg)
	}

	// Determine output format.
	format := toFormat(cfg.OutputFormat)

	// go-graphviz WASM requires absolute paths.
	outPath := cfg.OutputFile
	if !filepath.IsAbs(outPath) {
		abs, err := filepath.Abs(outPath)
		if err != nil {
			return fmt.Errorf("resolving output path: %w", err)
		}
		outPath = abs
	}

	log.Printf("[render] Writing %s to %s...", cfg.OutputFormat, outPath)

	// Render to buffer first, then write to file.
	var buf bytes.Buffer
	if err := g.Render(ctx, graph, format, &buf); err != nil {
		return fmt.Errorf("rendering graph: %w", err)
	}

	if buf.Len() == 0 {
		return fmt.Errorf("rendered graph is empty")
	}

	if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing to %s: %w", outPath, err)
	}

	log.Printf("[render] Done: %s (%d bytes)", cfg.OutputFile, buf.Len())
	return nil
}

// renderGroupedByClass groups methods into cluster subgraphs per class.
func renderGroupedByClass(graph *cgraph.Graph, cg *callgraph.CallGraph, cfg *config.Config) {
	// Collect nodes by class.
	classBuckets := make(map[string][]callgraph.NodeID)
	var noClassNodes []callgraph.NodeID

	for nid, node := range cg.Nodes {
		if node.ClassFQN != "" {
			classBuckets[node.ClassFQN] = append(classBuckets[node.ClassFQN], nid)
		} else {
			noClassNodes = append(noClassNodes, nid)
		}
	}

	gvNodes := make(map[callgraph.NodeID]*cgraph.Node)

	// Create cluster subgraphs per class.
	for classFQN, nodeIDs := range classBuckets {
		clusterName := "cluster_" + sanitizeID(classFQN)
		sub, err := graph.CreateSubGraphByName(clusterName)
		if err != nil {
			log.Printf("[render] WARN: cannot create subgraph for %s: %v", classFQN, err)
			continue
		}

		sub.SetLabel(escapeLabel(classFQN))
		sub.SetStyle(cgraph.RoundedGraphStyle)
		setSubgraphStyle(sub, cg.Nodes[nodeIDs[0]])

		for _, nid := range nodeIDs {
			node := cg.Nodes[nid]
			gvNode, err := graph.CreateNodeByName(sanitizeID(string(nid)))
			if err != nil {
				continue
			}
			styleNode(gvNode, node, cfg)
			sub.CreateSubNode(gvNode)
			gvNodes[nid] = gvNode
		}
	}

	// Create nodes without a class (standalone functions, unresolved).
	for _, nid := range noClassNodes {
		node := cg.Nodes[nid]
		gvNode, err := graph.CreateNodeByName(sanitizeID(string(nid)))
		if err != nil {
			continue
		}
		styleNode(gvNode, node, cfg)
		gvNodes[nid] = gvNode
	}

	// Create edges.
	createEdges(graph, cg, gvNodes, cfg)
}

// renderGroupedByNamespace groups classes into cluster subgraphs per namespace.
func renderGroupedByNamespace(graph *cgraph.Graph, cg *callgraph.CallGraph, cfg *config.Config) {
	// Collect nodes by namespace.
	nsBuckets := make(map[string][]callgraph.NodeID)

	for nid, node := range cg.Nodes {
		ns := extractNamespace(node.ClassFQN)
		nsBuckets[ns] = append(nsBuckets[ns], nid)
	}

	gvNodes := make(map[callgraph.NodeID]*cgraph.Node)

	for ns, nodeIDs := range nsBuckets {
		if ns == "" {
			// No namespace — add to root.
			for _, nid := range nodeIDs {
				node := cg.Nodes[nid]
				gvNode, err := graph.CreateNodeByName(sanitizeID(string(nid)))
				if err != nil {
					continue
				}
				styleNode(gvNode, node, cfg)
				gvNodes[nid] = gvNode
			}
			continue
		}

		clusterName := "cluster_" + sanitizeID(ns)
		sub, err := graph.CreateSubGraphByName(clusterName)
		if err != nil {
			log.Printf("[render] WARN: cannot create subgraph for ns %s: %v", ns, err)
			continue
		}

		sub.SetLabel(escapeLabel(ns))
		sub.SetStyle(cgraph.RoundedGraphStyle)
		sub.SetBackgroundColor("#f8f9fa")

		for _, nid := range nodeIDs {
			node := cg.Nodes[nid]
			gvNode, err := graph.CreateNodeByName(sanitizeID(string(nid)))
			if err != nil {
				continue
			}
			styleNode(gvNode, node, cfg)
			sub.CreateSubNode(gvNode)
			gvNodes[nid] = gvNode
		}
	}

	createEdges(graph, cg, gvNodes, cfg)
}

// renderFlat renders all nodes without grouping.
func renderFlat(graph *cgraph.Graph, cg *callgraph.CallGraph, cfg *config.Config) {
	gvNodes := make(map[callgraph.NodeID]*cgraph.Node)

	for nid, node := range cg.Nodes {
		gvNode, err := graph.CreateNodeByName(sanitizeID(string(nid)))
		if err != nil {
			continue
		}
		styleNode(gvNode, node, cfg)
		gvNodes[nid] = gvNode
	}

	createEdges(graph, cg, gvNodes, cfg)
}

// createEdges creates all edges in the graphviz graph.
func createEdges(graph *cgraph.Graph, cg *callgraph.CallGraph, gvNodes map[callgraph.NodeID]*cgraph.Node, cfg *config.Config) {
	for i, edge := range cg.Edges {
		if !cfg.ShowUnresolved && (edge.Type == callgraph.CallUnresolved || edge.Type == callgraph.CallDynamic) {
			continue
		}

		fromGV, ok1 := gvNodes[edge.From]
		toGV, ok2 := gvNodes[edge.To]
		if !ok1 || !ok2 {
			continue
		}

		edgeName := fmt.Sprintf("e%d", i)
		gvEdge, err := graph.CreateEdgeByName(edgeName, fromGV, toGV)
		if err != nil {
			continue
		}

		styleEdge(gvEdge, edge)
	}
}

// --- Styling ---

func styleNode(gvNode *cgraph.Node, node *callgraph.CallNode, cfg *config.Config) {
	// Label: for method granularity show method name, for class — show class name.
	label := node.Label
	if cfg.Granularity == "method" && node.ClassName != "" {
		label = node.ClassName + "::" + node.Method
	}
	gvNode.SetLabel(escapeLabel(label))
	gvNode.SetFontName("Helvetica")
	gvNode.SetFontSize(10)

	switch node.Kind {
	case "class":
		gvNode.SetShape(cgraph.BoxShape)
		gvNode.SetStyle(cgraph.FilledNodeStyle)
		gvNode.SetFillColor("#e3f2fd") // light blue
		gvNode.SetColor("#1565c0")
	case "interface":
		gvNode.SetShape(cgraph.BoxShape)
		gvNode.SetStyle(cgraph.FilledNodeStyle)
		gvNode.SetFillColor("#e8f5e9") // light green
		gvNode.SetColor("#2e7d32")
	case "trait":
		gvNode.SetShape(cgraph.BoxShape)
		gvNode.SetStyle(cgraph.FilledNodeStyle)
		gvNode.SetFillColor("#fff3e0") // light orange
		gvNode.SetColor("#e65100")
	case "enum":
		gvNode.SetShape(cgraph.BoxShape)
		gvNode.SetStyle(cgraph.FilledNodeStyle)
		gvNode.SetFillColor("#f3e5f5") // light purple
		gvNode.SetColor("#6a1b9a")
	case "function":
		gvNode.SetShape(cgraph.EllipseShape)
		gvNode.SetStyle(cgraph.FilledNodeStyle)
		gvNode.SetFillColor("#fce4ec") // light pink
		gvNode.SetColor("#c62828")
	case "unresolved":
		gvNode.SetShape(cgraph.EllipseShape)
		gvNode.SetStyle(cgraph.DottedNodeStyle)
		gvNode.SetColor("#9e9e9e")
		gvNode.SetFontColor("#9e9e9e")
	default:
		gvNode.SetShape(cgraph.BoxShape)
		gvNode.SetStyle(cgraph.FilledNodeStyle)
		gvNode.SetFillColor("#f5f5f5")
	}

	if node.IsStatic {
		gvNode.SetPenWidth(2.0)
	}
}

func setSubgraphStyle(sub *cgraph.Graph, node *callgraph.CallNode) {
	if node == nil {
		sub.SetBackgroundColor("#f8f9fa")
		return
	}

	switch node.Kind {
	case "class":
		sub.SetBackgroundColor("#e3f2fd")
	case "interface":
		sub.SetBackgroundColor("#e8f5e9")
	case "trait":
		sub.SetBackgroundColor("#fff3e0")
	case "enum":
		sub.SetBackgroundColor("#f3e5f5")
	default:
		sub.SetBackgroundColor("#f8f9fa")
	}
}

func styleEdge(gvEdge *cgraph.Edge, edge *callgraph.CallEdge) {
	gvEdge.SetFontName("Helvetica")
	gvEdge.SetFontSize(8)

	switch edge.Type {
	case callgraph.CallInstance:
		gvEdge.SetColor("#1565c0")
		gvEdge.SetStyle(cgraph.SolidEdgeStyle)
	case callgraph.CallNullsafe:
		gvEdge.SetColor("#1565c0")
		gvEdge.SetStyle(cgraph.DashedEdgeStyle)
	case callgraph.CallStatic:
		gvEdge.SetColor("#c62828")
		gvEdge.SetStyle(cgraph.SolidEdgeStyle)
		gvEdge.SetPenWidth(1.5)
	case callgraph.CallNew:
		gvEdge.SetColor("#2e7d32")
		gvEdge.SetStyle(cgraph.DottedEdgeStyle)
		gvEdge.SetArrowHead(cgraph.DiamondArrow)
	case callgraph.CallFunction:
		gvEdge.SetColor("#6a1b9a")
		gvEdge.SetStyle(cgraph.SolidEdgeStyle)
	case callgraph.CallUnresolved:
		gvEdge.SetColor("#9e9e9e")
		gvEdge.SetStyle(cgraph.DottedEdgeStyle)
		gvEdge.SetLabel("?")
	case callgraph.CallDynamic:
		gvEdge.SetColor("#ff6f00")
		gvEdge.SetStyle(cgraph.DottedEdgeStyle)
		gvEdge.SetLabel("dynamic")
	}
}

// --- Helpers ---

// escapeLabel escapes backslashes in Graphviz labels so that
// PHP namespace separators (\) are rendered literally.
func escapeLabel(s string) string {
	return strings.ReplaceAll(s, "\\", "\\\\")
}

func sanitizeID(s string) string {
	r := strings.NewReplacer(
		"\\", "_",
		"::", "__",
		"(", "_",
		")", "_",
		"$", "_",
		"?", "q",
		" ", "_",
		"-", "_",
		"@", "_at_",
	)
	return r.Replace(s)
}

func extractNamespace(fqn string) string {
	idx := strings.LastIndex(fqn, "\\")
	if idx < 0 {
		return ""
	}
	return fqn[:idx]
}

func toFormat(s string) graphviz.Format {
	switch strings.ToLower(s) {
	case "png":
		return graphviz.PNG
	case "jpg", "jpeg":
		return graphviz.JPG
	case "dot":
		return graphviz.XDOT
	default:
		return graphviz.SVG
	}
}

func toLayout(s string) graphviz.Layout {
	switch strings.ToLower(s) {
	case "circo":
		return graphviz.CIRCO
	case "fdp":
		return graphviz.FDP
	case "neato":
		return graphviz.NEATO
	case "sfdp":
		return graphviz.SFDP
	case "twopi":
		return graphviz.TWOPI
	case "osage":
		return graphviz.OSAGE
	case "patchwork":
		return graphviz.PATCHWORK
	default:
		return graphviz.DOT
	}
}

func toRankDir(s string) cgraph.RankDir {
	switch strings.ToUpper(s) {
	case "LR":
		return cgraph.LRRank
	case "BT":
		return cgraph.BTRank
	case "RL":
		return cgraph.RLRank
	default:
		return cgraph.TBRank
	}
}
