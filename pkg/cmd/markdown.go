package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// isMarkdownFile reports whether name has a markdown file extension.
func isMarkdownFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))

	return ext == ".md" || ext == ".markdown"
}

// yamlBlock is a YAML code fence extracted from a markdown file. lineOffset is
// the 1-based line number in the original markdown file of the first line of
// content inside the fence (i.e. the line immediately after the opening ```).
type yamlBlock struct {
	content    []byte
	lineOffset int
}

// extractYAMLBlocks scans markdown source line-by-line and returns every fenced
// YAML code block. Opening fences are ```yaml (case-insensitive), with optional
// leading whitespace. Unclosed fences are silently discarded.
func extractYAMLBlocks(data []byte) []yamlBlock {
	lines := bytes.Split(data, []byte("\n"))

	var blocks []yamlBlock

	inBlock := false

	var blockLines [][]byte

	blockStart := 0

	for i, raw := range lines {
		line := bytes.TrimRight(raw, "\r") // strip \r from CRLF files
		trimmed := bytes.TrimLeft(line, " \t")
		lower := strings.ToLower(string(trimmed))

		if !inBlock {
			if lower == "```yaml" {
				inBlock = true
				blockLines = nil
				blockStart = i + 2 // +1 for 0→1-indexed, +1 to skip the fence line itself
			}

			continue
		}

		if bytes.HasPrefix(trimmed, []byte("```")) {
			// Closing fence — collect the block.
			strs := make([]string, len(blockLines))
			for j, l := range blockLines {
				strs[j] = string(l)
			}

			blocks = append(blocks, yamlBlock{
				content:    []byte(strings.Join(strs, "\n")),
				lineOffset: blockStart,
			})

			inBlock = false
			blockLines = nil

			continue
		}

		blockLines = append(blockLines, line)
	}

	// Unclosed fences are discarded.
	return blocks
}

// findMarkdownFiles returns every markdown file in the repository. The .git,
// node_modules, and vendor directories are skipped.
func findMarkdownFiles() ([]string, error) {
	var files []string

	if err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // tolerate unreadable entries
		}

		if entry.IsDir() {
			if skippedWalkDirs[entry.Name()] {
				return filepath.SkipDir
			}

			return nil
		}

		if isMarkdownFile(entry.Name()) {
			files = append(files, filepath.Clean(path))
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("scan for markdown files: %w", err)
	}

	return files, nil
}

// findActionRefsInMarkdownFile parses every fenced YAML block in a markdown
// file and returns the action references found within them. Each Action's
// Node.Line is adjusted to be the correct 1-based line number within the
// markdown file rather than within the YAML fragment.
func findActionRefsInMarkdownFile(filePath string) ([]Action, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", filePath, err)
	}

	blocks := extractYAMLBlocks(data)

	var actions []Action

	for _, block := range blocks {
		refs, err := parseActionRefs(block.content, filePath)
		if err != nil {
			// A malformed YAML block in a README shouldn't abort the whole run.
			continue
		}

		for i := range refs {
			refs[i].Node.Line += block.lineOffset - 1
		}

		actions = append(actions, refs...)
	}

	return actions, nil
}
