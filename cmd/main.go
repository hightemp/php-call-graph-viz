package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hightemp/php-call-graph-viz/internal/callgraph"
	"github.com/hightemp/php-call-graph-viz/internal/config"
	"github.com/hightemp/php-call-graph-viz/internal/phpparse"
	"github.com/hightemp/php-call-graph-viz/internal/render"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	flag.Parse()

	log.SetFlags(log.Ltime)
	log.SetPrefix("[php-call-graph] ")

	start := time.Now()

	// Phase 0: Load configuration.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	log.Printf("Config loaded from %s", *configPath)

	// Phase 1: Parse PHP files.
	results, err := phpparse.ParseFiles(cfg.SourceDirs, cfg.PHPVersion, cfg.Workers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing PHP files: %v\n", err)
		os.Exit(1)
	}

	// Phase 2: Build symbol table.
	symbols := phpparse.BuildSymbolTable(results)

	// Phase 3: Build call graph.
	cg := callgraph.BuildCallGraph(
		results,
		symbols,
		cfg.TypeMappings,
		cfg.ResolveTypes,
		cfg.ShowFunctions,
	)

	// Phase 4: Apply filters.
	cg = applyFilters(cg, cfg)

	// Phase 5: Collapse to class level if configured.
	if cfg.Granularity == "class" {
		cg = cg.CollapseToClassLevel()
		log.Printf("[filter] Collapsed to class level: %s", cg.Stats())
	}

	// Phase 6: Render.
	if err := render.Render(cg, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error rendering graph: %v\n", err)
		os.Exit(1)
	}

	log.Printf("Completed in %s", time.Since(start).Round(time.Millisecond))
}

func applyFilters(cg *callgraph.CallGraph, cfg *config.Config) *callgraph.CallGraph {
	// Filter by namespaces.
	cg = cg.FilterByNamespaces(cfg.Filters.IncludeNamespaces, cfg.Filters.ExcludeNamespaces)

	// Filter by classes.
	cg = cg.FilterByClasses(cfg.Filters.IncludeClasses, cfg.Filters.ExcludeClasses)

	// Exclude methods.
	cg = cg.ExcludeMethods(cfg.Filters.ExcludeMethods)

	// Filter by entry points + depth.
	if len(cfg.EntryPoints) > 0 {
		var entryIDs []callgraph.NodeID
		for _, ep := range cfg.EntryPoints {
			entryIDs = append(entryIDs, callgraph.NodeID(ep))
		}
		cg = cg.FilterByEntryPoints(entryIDs, cfg.MaxDepth)
		log.Printf("[filter] Filtered by %d entry points (max_depth=%d): %s",
			len(cfg.EntryPoints), cfg.MaxDepth, cg.Stats())
	}

	log.Printf("[filter] After all filters: %s", cg.Stats())
	return cg
}
