# PHP Call Graph Visualizer

![Experimental](https://img.shields.io/badge/status-experimental-orange)
![Vibe Coded](https://img.shields.io/badge/vibe-coded-blueviolet)
![](https://goreportcard.com/badge/github.com/hightemp/php-call-graph-viz)

A static analysis tool that parses PHP source code and generates visual call graphs using Graphviz. Written in Go.

![](./screenshots/2026-02-12_17-59.png)

## Features

- **Static PHP parsing** — uses [VKCOM/php-parser](https://github.com/VKCOM/php-parser) to build a full AST (PHP 5.x–8.1)
- **Heuristic type resolution** — tracks `new`, parameter type hints, property types, and `$this` to resolve `$var->method()` calls
- **Multiple output formats** — SVG, PNG, JPG, DOT
- **Flexible grouping** — cluster nodes by class, by namespace, or render a flat graph
- **Granularity control** — method-level or class-level (collapsed) call graphs
- **Rich filtering** — include/exclude by namespace, class, or method name; entry-point + depth limiting
- **Manual type mappings** — override unresolvable types via `config.yaml`
- **Parallel parsing** — configurable worker pool for large codebases
- **Color-coded nodes & edges** — classes, interfaces, traits, enums, functions, static/instance/nullsafe calls are visually distinct

## Installation

### Quick Install (Recommended)

**Linux / macOS:**
```bash
curl -sSL https://raw.githubusercontent.com/hightemp/php-call-graph-viz/main/install.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/hightemp/php-call-graph-viz/main/install.ps1 | iex
```

### Alternative Methods

**Using Go:**
```bash
go install github.com/hightemp/php-call-graph-viz/cmd@latest
```

**Download pre-built binaries:**

Visit the [releases page](https://github.com/hightemp/php-call-graph-viz/releases/latest) to download binaries for:
- Linux (x86_64, arm64)
- macOS (x86_64, arm64)
- Windows (x86_64)

**Build from source:**
```bash
git clone https://github.com/hightemp/php-call-graph-viz.git
cd php-call-graph-viz
make build
```

## Makefile Commands

The project includes a Makefile for common tasks:

```bash
make build        # Build the binary
make install      # Install to $GOPATH/bin
make clean        # Remove binary and clean cache
make test         # Run tests
make run          # Build and run with config.yaml
make run-test     # Build and run with test/config.yaml
make fmt          # Format code
make vet          # Run go vet
make lint         # Run golangci-lint
make deps         # Download dependencies
make deps-update  # Update dependencies
make help         # Show all available targets
```

## Quick Start

1. Build the project:

```bash
make build
```

2. Copy the sample configuration and adjust it for your project:

```bash
cp config.yaml my-config.yaml
```

3. Edit `source_dirs` in `my-config.yaml` to point to your PHP source directories.

4. Run the tool:

```bash
./php-call-graph-viz -config my-config.yaml
```

Or use the Makefile:

```bash
make run          # Uses config.yaml
make run-test     # Uses test/config.yaml
```

The output file (default: `callgraph.svg`) will be generated in the current directory.

## Configuration

All options are set via a YAML file (default: `config.yaml`).

| Option | Type | Default | Description |
|---|---|---|---|
| `source_dirs` | `[]string` | — | Directories to scan for `.php` files |
| `php_version` | `string` | `"8.1"` | PHP version for the parser |
| `output_format` | `string` | `"svg"` | Output format: `svg`, `png`, `jpg`, `dot` |
| `output_file` | `string` | `"callgraph.svg"` | Output file path |
| `group_by` | `string` | `"class"` | Grouping: `class`, `namespace`, `none` |
| `granularity` | `string` | `"method"` | Detail level: `method` or `class` |
| `layout` | `string` | `"dot"` | Graphviz layout engine: `dot`, `circo`, `fdp`, `neato`, `sfdp`, `twopi` |
| `rank_dir` | `string` | `"TB"` | Graph direction: `TB`, `LR`, `BT`, `RL` |
| `max_depth` | `int` | `0` | Max call chain depth from entry points (0 = unlimited) |
| `entry_points` | `[]string` | `[]` | Restrict graph to calls reachable from these FQN methods |
| `workers` | `int` | `4` | Number of parallel parsing goroutines |
| `resolve_types` | `bool` | `false` | Enable heuristic type resolution |
| `show_unresolved` | `bool` | `false` | Show unresolved calls as dashed edges |
| `show_functions` | `bool` | `false` | Include standalone function calls |
| `type_mappings` | `map` | `{}` | Manual `$var` → FQN type overrides |

### Filters

```yaml
filters:
  include_namespaces:
    - "App\\Services"
    - "App\\Repositories"
  exclude_namespaces:
    - "App\\Tests"
  include_classes: []
  exclude_classes:
    - "App\\Services\\LogService"
  exclude_methods:
    - __construct
    - __destruct
```

### Entry Points

Restrict the graph to calls reachable from specific methods:

```yaml
entry_points:
  - "App\\Controllers\\UserController::index"
max_depth: 3
```

### Type Mappings

Manually specify types when automatic resolution fails:

```yaml
type_mappings:
  "$container": "App\\Container\\AppContainer"
  "App\\Services\\BaseService::$em": "Doctrine\\ORM\\EntityManager"
```

## Example Configuration

```yaml
source_dirs:
  - ./src

php_version: "8.1"
output_format: svg
output_file: callgraph.svg
group_by: class
granularity: method
layout: dot
rank_dir: LR
workers: 8
resolve_types: true
show_unresolved: false
show_functions: false

filters:
  include_namespaces: []
  exclude_namespaces: []
  include_classes: []
  exclude_classes: []
  exclude_methods:
    - __construct
    - __destruct

type_mappings: {}
```

## Visual Legend

| Element | Appearance |
|---|---|
| Class method | Blue box |
| Interface method | Green box |
| Trait method | Orange box |
| Enum method | Purple box |
| Standalone function | Pink ellipse |
| Unresolved call | Gray dotted ellipse |
| Instance call (`->`) | Blue solid arrow |
| Nullsafe call (`?->`) | Blue dashed arrow |
| Static call (`::`) | Red solid arrow (thicker) |
| `new` expression | Green dotted arrow (diamond head) |
| Function call | Purple solid arrow |
| Dynamic call (`$obj->$method`) | Orange dotted arrow |

## Project Structure

```
cmd/
  main.go              # CLI entry point
internal/
  config/
    config.go          # YAML config loading & defaults
  phpparse/
    parser.go          # Parallel PHP file discovery & parsing
    symbols.go         # Symbol table: classes, methods, properties, traits
  callgraph/
    graph.go           # Call graph data structure & filtering
    visitor.go         # AST visitor that extracts call edges
    resolver.go        # Heuristic type resolver
  render/
    graphviz.go        # Graphviz rendering with grouping & styling
```

## Requirements

- Go 1.21+

## License

MIT

![](https://asdertasd.site/counter/php-call-graph-viz)