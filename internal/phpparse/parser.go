package phpparse

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/VKCOM/php-parser/pkg/ast"
	"github.com/VKCOM/php-parser/pkg/conf"
	phperrors "github.com/VKCOM/php-parser/pkg/errors"
	"github.com/VKCOM/php-parser/pkg/parser"
	"github.com/VKCOM/php-parser/pkg/version"
	"github.com/VKCOM/php-parser/pkg/visitor/nsresolver"
	"github.com/VKCOM/php-parser/pkg/visitor/traverser"
)

// ParseResult holds the AST and resolved names for a single PHP file.
type ParseResult struct {
	FilePath      string
	Root          ast.Vertex
	ResolvedNames map[ast.Vertex]string
}

// ParseFiles discovers and parses all .php files in the given directories
// using numWorkers parallel goroutines. Returns a slice of ParseResult.
func ParseFiles(dirs []string, phpVer string, numWorkers int) ([]*ParseResult, error) {
	ver, err := version.New(phpVer)
	if err != nil {
		return nil, fmt.Errorf("invalid PHP version %q: %w", phpVer, err)
	}

	// Discover all .php files.
	var files []string
	for _, dir := range dirs {
		found, err := discoverPHPFiles(dir)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", dir, err)
		}
		files = append(files, found...)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .php files found in %v", dirs)
	}

	log.Printf("[parse] Found %d PHP files, parsing with %d workers...", len(files), numWorkers)

	// Parse in parallel via a worker pool.
	type job struct {
		path string
	}
	jobs := make(chan job, len(files))
	for _, f := range files {
		jobs <- job{path: f}
	}
	close(jobs)

	var (
		mu      sync.Mutex
		results []*ParseResult
		wg      sync.WaitGroup
	)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				res, err := parseFile(j.path, ver)
				if err != nil {
					log.Printf("[parse] WARN: skipping %s: %v", j.path, err)
					continue
				}
				mu.Lock()
				results = append(results, res)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	log.Printf("[parse] Successfully parsed %d files", len(results))
	return results, nil
}

// parseFile parses a single PHP file and runs the namespace resolver.
func parseFile(path string, ver *version.Version) (*ParseResult, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var parseErrors []string
	cfg := conf.Config{
		Version: ver,
		ErrorHandlerFunc: func(e *phperrors.Error) {
			parseErrors = append(parseErrors, e.String())
		},
	}

	rootNode, err := parser.Parse(src, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	if len(parseErrors) > 0 {
		log.Printf("[parse] WARN: %s has %d parse errors: %s", path, len(parseErrors), strings.Join(parseErrors, "; "))
	}

	// Run namespace resolver to get fully qualified names.
	nsRes := nsresolver.NewNamespaceResolver()
	tv := traverser.NewTraverser(nsRes)
	rootNode.Accept(tv)

	return &ParseResult{
		FilePath:      path,
		Root:          rootNode,
		ResolvedNames: nsRes.ResolvedNames,
	}, nil
}

// discoverPHPFiles walks a directory tree and returns all .php file paths.
func discoverPHPFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".php") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
