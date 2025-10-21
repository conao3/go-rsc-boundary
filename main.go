package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config holds the configuration for the boundary scanner
type Config struct {
	Directives       []string
	SearchExtensions []string
	MaxReadBytes     int64
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Directives:       []string{"'use client'", `"use client"`},
		SearchExtensions: []string{".tsx", ".ts", ".jsx", ".js"},
		MaxReadBytes:     4096,
	}
}

// ImportInfo represents a parsed import statement
type ImportInfo struct {
	Source     string
	Specifiers []string
}

// PathAlias represents a path alias mapping
type PathAlias struct {
	Alias  string
	Target string
}

// TSConfig represents a subset of tsconfig.json/jsconfig.json
type TSConfig struct {
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
	Extends string `json:"extends"`
}

var (
	// Regular expressions for parsing
	importRegex = regexp.MustCompile(`^\s*import\s+(.+?)(?:\s+from\s+)?['"]([^'"]+)['"]`)
	jsxTagRegex = regexp.MustCompile(`<\s*(\w+)`)
)

func main() {
	var (
		path    = flag.String("path", ".", "path to scan")
		verbose = flag.Bool("v", false, "verbose output")
	)
	flag.Parse()

	config := DefaultConfig()

	if err := scanPath(*path, config, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func scanPath(root string, config *Config, verbose bool) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip node_modules, .git, etc.
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process supported file extensions
		if !isSupportedFile(path, config.SearchExtensions) {
			return nil
		}

		if err := scanFile(path, config, verbose); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to scan %s: %v\n", path, err)
			}
		}

		return nil
	})
}

func isSupportedFile(path string, extensions []string) bool {
	ext := filepath.Ext(path)
	for _, e := range extensions {
		if ext == e {
			return true
		}
	}
	return false
}

func scanFile(filePath string, config *Config, verbose bool) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	// Parse imports
	imports := parseImports(lines)
	if len(imports) == 0 {
		return nil
	}

	// Resolve aliases
	baseDir := filepath.Dir(filePath)
	aliases, err := loadPathAliases(baseDir)
	if err != nil && verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to load aliases for %s: %v\n", filePath, err)
	}

	// Collect client components
	clientComponents := make(map[string]bool)

	for _, imp := range imports {
		// Resolve import path
		resolvedPaths := resolveImportPath(baseDir, imp.Source, aliases, config)

		for _, resolvedPath := range resolvedPaths {
			if fileHasDirective(resolvedPath, config) {
				for _, spec := range imp.Specifiers {
					clientComponents[spec] = true
				}
				break
			}
		}
	}

	if len(clientComponents) == 0 {
		return nil
	}

	// Find JSX usages and output in grep format
	for lineNum, line := range lines {
		for component := range clientComponents {
			if containsJSXTag(line, component) {
				// Output in grep format: filename:line:content
				fmt.Printf("%s:%d:%s\n", filePath, lineNum+1, line)
				break
			}
		}
	}

	return nil
}

func parseImports(lines []string) []ImportInfo {
	var imports []ImportInfo
	var currentImport string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if currentImport != "" {
			currentImport += " " + trimmed
		} else if strings.HasPrefix(trimmed, "import ") {
			currentImport = trimmed
		}

		if currentImport != "" {
			// Check if import statement is complete
			if strings.Contains(currentImport, `"`) || strings.Contains(currentImport, `'`) {
				if imp := parseImportStatement(currentImport); imp != nil {
					imports = append(imports, *imp)
				}
				currentImport = ""
			}
		}
	}

	return imports
}

func parseImportStatement(stmt string) *ImportInfo {
	// Skip type-only imports
	if regexp.MustCompile(`^\s*import\s+type\s`).MatchString(stmt) {
		return nil
	}

	// Extract source path
	sourceMatch := regexp.MustCompile(`from\s+['"]([^'"]+)['"]|import\s+['"]([^'"]+)['"]`).FindStringSubmatch(stmt)
	if sourceMatch == nil {
		return nil
	}

	source := sourceMatch[1]
	if source == "" {
		source = sourceMatch[2]
	}

	if source == "" {
		return nil
	}

	// Extract specifiers
	var specifiers []string

	// Remove "from ..." part
	clause := regexp.MustCompile(`^\s*import\s+(.*?)\s+from\s+`).FindStringSubmatch(stmt)
	if clause == nil || len(clause) < 2 {
		// Side-effect import only
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	clauseText := strings.TrimSpace(clause[1])
	clauseText = regexp.MustCompile(`^type\s+`).ReplaceAllString(clauseText, "")
	clauseText = strings.TrimSpace(clauseText)

	if clauseText == "" {
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	// Handle default + named: import Default, { Named } from '...'
	if match := regexp.MustCompile(`^([\w$]+)\s*,\s*\{(.*)\}$`).FindStringSubmatch(clauseText); match != nil {
		specifiers = append(specifiers, strings.TrimSpace(match[1]))
		specifiers = append(specifiers, parseNamedSpecifiers(match[2])...)
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	// Handle named only: import { Named } from '...'
	if match := regexp.MustCompile(`^\{(.*)\}$`).FindStringSubmatch(clauseText); match != nil {
		specifiers = append(specifiers, parseNamedSpecifiers(match[1])...)
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	// Handle namespace: import * as Name from '...'
	if regexp.MustCompile(`^\*\s+as\s+[\w$]+$`).MatchString(clauseText) {
		// Namespace imports don't export individual components
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	// Default import
	specifiers = append(specifiers, clauseText)

	return &ImportInfo{Source: source, Specifiers: specifiers}
}

func parseNamedSpecifiers(body string) []string {
	var specifiers []string

	for _, chunk := range strings.Split(body, ",") {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" || strings.HasPrefix(trimmed, "type ") {
			continue
		}

		// Handle "Name as Alias"
		if match := regexp.MustCompile(`^.*\s+as\s+([\w$]+)$`).FindStringSubmatch(trimmed); match != nil {
			specifiers = append(specifiers, match[1])
		} else {
			specifiers = append(specifiers, trimmed)
		}
	}

	return specifiers
}

func resolveImportPath(baseDir, importPath string, aliases []PathAlias, config *Config) []string {
	var candidates []string

	// Handle relative imports
	if strings.HasPrefix(importPath, ".") {
		basePath := filepath.Join(baseDir, importPath)
		candidates = append(candidates, expandPath(basePath, config)...)
		return candidates
	}

	// Handle aliased imports
	for _, alias := range aliases {
		if strings.HasPrefix(importPath, alias.Alias) {
			remainder := strings.TrimPrefix(importPath, alias.Alias)
			remainder = strings.TrimPrefix(remainder, "/")

			targetPath := filepath.Join(alias.Target, remainder)
			candidates = append(candidates, expandPath(targetPath, config)...)
		}
	}

	return candidates
}

func expandPath(basePath string, config *Config) []string {
	var paths []string

	// Check if file exists as-is
	if fileExists(basePath) {
		paths = append(paths, basePath)
		return paths
	}

	// Try with extensions
	for _, ext := range config.SearchExtensions {
		pathWithExt := basePath + ext
		if fileExists(pathWithExt) {
			paths = append(paths, pathWithExt)
		}
	}

	// Try index files if path is a directory
	if info, err := os.Stat(basePath); err == nil && info.IsDir() {
		for _, ext := range config.SearchExtensions {
			indexPath := filepath.Join(basePath, "index"+ext)
			if fileExists(indexPath) {
				paths = append(paths, indexPath)
			}
		}
	}

	return paths
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fileHasDirective(filePath string, config *Config) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Read only the beginning of the file
	limitedReader := io.LimitReader(file, config.MaxReadBytes)
	scanner := bufio.NewScanner(limitedReader)

	inBlockComment := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
			}
			continue
		}

		// Check for directive
		for _, directive := range config.Directives {
			trimmedLine := strings.TrimSuffix(line, ";")
			if trimmedLine == directive {
				return true
			}
		}

		// Skip comments
		if strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "/*") {
			if !strings.Contains(line, "*/") {
				inBlockComment = true
			}
			continue
		}

		// If we hit non-comment, non-directive code, stop
		if !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") {
			break
		}
	}

	return false
}

func containsJSXTag(line, componentName string) bool {
	// Look for <ComponentName
	pattern := `<\s*` + regexp.QuoteMeta(componentName) + `\b`
	matched, _ := regexp.MatchString(pattern, line)
	return matched
}

func loadPathAliases(baseDir string) ([]PathAlias, error) {
	// Look for tsconfig.json or jsconfig.json
	configPaths := []string{
		"tsconfig.json",
		"jsconfig.json",
		"tsconfig.base.json",
	}

	// Search upwards for config file
	currentDir := baseDir
	for {
		for _, configFile := range configPaths {
			configPath := filepath.Join(currentDir, configFile)
			if fileExists(configPath) {
				return parseAliases(configPath)
			}
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	return nil, nil
}

func parseAliases(configPath string) ([]PathAlias, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config TSConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	var aliases []PathAlias
	baseDir := filepath.Dir(configPath)

	baseURL := config.CompilerOptions.BaseURL
	if baseURL == "" {
		baseURL = "."
	}

	if !filepath.IsAbs(baseURL) {
		baseURL = filepath.Join(baseDir, baseURL)
	}

	for aliasPattern, targets := range config.CompilerOptions.Paths {
		if len(targets) == 0 {
			continue
		}

		// Normalize alias pattern
		alias := strings.TrimSuffix(aliasPattern, "/*")
		alias = strings.TrimSuffix(alias, "*")

		// Normalize target
		target := targets[0]
		target = strings.TrimSuffix(target, "/*")
		target = strings.TrimSuffix(target, "*")
		target = strings.TrimPrefix(target, "./")

		// Resolve target path
		var targetPath string
		if filepath.IsAbs(target) {
			targetPath = target
		} else {
			targetPath = filepath.Join(baseURL, target)
		}

		aliases = append(aliases, PathAlias{
			Alias:  alias,
			Target: targetPath,
		})
	}

	return aliases, nil
}
