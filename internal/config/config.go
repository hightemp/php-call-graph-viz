package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure loaded from config.yaml.
type Config struct {
	// SourceDirs lists directories to scan for PHP files.
	SourceDirs []string `yaml:"source_dirs"`

	// PHPVersion specifies the PHP version for the parser (e.g. "8.1").
	PHPVersion string `yaml:"php_version"`

	// OutputFormat: "svg", "png", "dot".
	OutputFormat string `yaml:"output_format"`

	// OutputFile is the path to the rendered output.
	OutputFile string `yaml:"output_file"`

	// GroupBy controls graph clustering: "class", "namespace", or "none".
	GroupBy string `yaml:"group_by"`

	// Granularity controls the graph detail: "method" or "class".
	Granularity string `yaml:"granularity"`

	// Layout is the Graphviz layout engine: "dot", "circo", "fdp", "neato", "sfdp", "twopi", etc.
	Layout string `yaml:"layout"`

	// RankDir sets graph direction: "TB", "LR", "BT", "RL".
	RankDir string `yaml:"rank_dir"`

	// MaxDepth limits the call chain depth from entry points (0 = unlimited).
	MaxDepth int `yaml:"max_depth"`

	// EntryPoints restricts the graph to calls reachable from these FQN methods.
	// Example: ["App\\Controllers\\UserController::index"]
	EntryPoints []string `yaml:"entry_points"`

	// Workers is the number of parallel file-parsing goroutines.
	Workers int `yaml:"workers"`

	// ResolveTypes enables heuristic type resolution for $var->method() calls.
	ResolveTypes bool `yaml:"resolve_types"`

	// ShowUnresolved controls whether unresolved calls appear in the graph (dashed edges).
	ShowUnresolved bool `yaml:"show_unresolved"`

	// ShowFunctions controls whether standalone function calls appear in the graph.
	ShowFunctions bool `yaml:"show_functions"`

	// Filters control which classes/namespaces to include or exclude.
	Filters FilterConfig `yaml:"filters"`

	// TypeMappings allows manual variable → FQN type overrides.
	// Key is "$varName" or "ClassName::$propName", value is FQN.
	TypeMappings map[string]string `yaml:"type_mappings"`
}

// FilterConfig holds include/exclude filter lists.
type FilterConfig struct {
	IncludeNamespaces []string `yaml:"include_namespaces"`
	ExcludeNamespaces []string `yaml:"exclude_namespaces"`
	IncludeClasses    []string `yaml:"include_classes"`
	ExcludeClasses    []string `yaml:"exclude_classes"`
	ExcludeMethods    []string `yaml:"exclude_methods"`
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.PHPVersion == "" {
		c.PHPVersion = "8.1"
	}
	if c.OutputFormat == "" {
		c.OutputFormat = "svg"
	}
	if c.OutputFile == "" {
		c.OutputFile = "callgraph." + c.OutputFormat
	}
	if c.GroupBy == "" {
		c.GroupBy = "class"
	}
	if c.Granularity == "" {
		c.Granularity = "method"
	}
	if c.Layout == "" {
		c.Layout = "dot"
	}
	if c.RankDir == "" {
		c.RankDir = "TB"
	}
	if c.Workers <= 0 {
		c.Workers = 4
	}
}
