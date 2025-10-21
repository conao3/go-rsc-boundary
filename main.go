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

type Config struct {
	Directives       []string
	SearchExtensions []string
	MaxReadBytes     int64
}

func DefaultConfig() *Config {
	return &Config{
		Directives:       []string{"'use client'", `"use client"`},
		SearchExtensions: []string{".tsx", ".ts", ".jsx", ".js"},
		MaxReadBytes:     4096,
	}
}

type ImportInfo struct {
	Source     string
	Specifiers []string
}

type PathAlias struct {
	Alias  string
	Target string
}

type TSConfig struct {
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
	Extends string `json:"extends"`
}

var (
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
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

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

	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	imports := parseImports(lines)
	if len(imports) == 0 {
		return nil
	}

	baseDir := filepath.Dir(filePath)
	aliases, err := loadPathAliases(baseDir)
	if err != nil && verbose {
		fmt.Fprintf(os.Stderr, "Warning: failed to load aliases for %s: %v\n", filePath, err)
	}

	clientComponents := make(map[string]bool)

	for _, imp := range imports {
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

	for lineNum, line := range lines {
		for component := range clientComponents {
			if containsJSXTag(line, component) {
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
	if regexp.MustCompile(`^\s*import\s+type\s`).MatchString(stmt) {
		return nil
	}

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

	var specifiers []string

	clause := regexp.MustCompile(`^\s*import\s+(.*?)\s+from\s+`).FindStringSubmatch(stmt)
	if clause == nil || len(clause) < 2 {
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	clauseText := strings.TrimSpace(clause[1])
	clauseText = regexp.MustCompile(`^type\s+`).ReplaceAllString(clauseText, "")
	clauseText = strings.TrimSpace(clauseText)

	if clauseText == "" {
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	if match := regexp.MustCompile(`^([\w$]+)\s*,\s*\{(.*)\}$`).FindStringSubmatch(clauseText); match != nil {
		specifiers = append(specifiers, strings.TrimSpace(match[1]))
		specifiers = append(specifiers, parseNamedSpecifiers(match[2])...)
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	if match := regexp.MustCompile(`^\{(.*)\}$`).FindStringSubmatch(clauseText); match != nil {
		specifiers = append(specifiers, parseNamedSpecifiers(match[1])...)
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

	if regexp.MustCompile(`^\*\s+as\s+[\w$]+$`).MatchString(clauseText) {
		return &ImportInfo{Source: source, Specifiers: specifiers}
	}

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

	if strings.HasPrefix(importPath, ".") {
		basePath := filepath.Join(baseDir, importPath)
		candidates = append(candidates, expandPath(basePath, config)...)
		return candidates
	}

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

	if fileExists(basePath) {
		paths = append(paths, basePath)
		return paths
	}

	for _, ext := range config.SearchExtensions {
		pathWithExt := basePath + ext
		if fileExists(pathWithExt) {
			paths = append(paths, pathWithExt)
		}
	}

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

		for _, directive := range config.Directives {
			trimmedLine := strings.TrimSuffix(line, ";")
			if trimmedLine == directive {
				return true
			}
		}

		if strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "/*") {
			if !strings.Contains(line, "*/") {
				inBlockComment = true
			}
			continue
		}

		if !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") {
			break
		}
	}

	return false
}

func containsJSXTag(line, componentName string) bool {
	pattern := `<\s*` + regexp.QuoteMeta(componentName) + `\b`
	matched, _ := regexp.MatchString(pattern, line)
	return matched
}

func loadPathAliases(baseDir string) ([]PathAlias, error) {
	configPaths := []string{
		"tsconfig.json",
		"jsconfig.json",
		"tsconfig.base.json",
	}

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

		alias := strings.TrimSuffix(aliasPattern, "/*")
		alias = strings.TrimSuffix(alias, "*")

		target := targets[0]
		target = strings.TrimSuffix(target, "/*")
		target = strings.TrimSuffix(target, "*")
		target = strings.TrimPrefix(target, "./")

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
