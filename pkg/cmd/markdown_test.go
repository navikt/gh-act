package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsMarkdownFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "README.md", want: true},
		{name: "readme.MD", want: true},
		{name: "CONTRIBUTING.markdown", want: true},
		{name: "docs.MARKDOWN", want: true},
		{name: "workflow.yml", want: false},
		{name: "action.yaml", want: false},
		{name: "noextension", want: false},
		{name: ".md", want: true}, // dotfile with .md extension
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isMarkdownFile(tt.name))
		})
	}
}

func TestExtractYAMLBlocks(t *testing.T) {
	tests := []struct {
		name           string
		markdown       string
		wantOffsets    []int
		wantContents   []string
		wantBlockCount int
	}{
		{
			name:           "single yaml block",
			markdown:       "# Title\n\nSome text.\n\n```yaml\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{6},
			wantContents:   []string{"jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4"},
		},
		{
			name:           "multiple blocks",
			markdown:       "```yaml\nfirst: block\n```\n\nSome prose.\n\n```yaml\nsecond: block\n```\n",
			wantBlockCount: 2,
			wantOffsets:    []int{2, 8},
			wantContents:   []string{"first: block", "second: block"},
		},
		{
			name:           "non-yaml fences are ignored",
			markdown:       "```bash\necho hello\n```\n\n```go\nfmt.Println()\n```\n\n```yml\nruns: {}\n```\n",
			wantBlockCount: 0,
		},
		{
			name:           "unclosed fence is discarded",
			markdown:       "```yaml\njobs: {}\n",
			wantBlockCount: 0,
		},
		{
			name:           "no code blocks",
			markdown:       "# Just prose\n\nNo fences here.\n",
			wantBlockCount: 0,
		},
		{
			name:           "case insensitive fence",
			markdown:       "```YAML\njobs: {}\n```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{2},
			wantContents:   []string{"jobs: {}"},
		},
		{
			name:           "fence with leading whitespace",
			markdown:       "  ```yaml\n  jobs: {}\n  ```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{2},
			wantContents:   []string{"  jobs: {}"},
		},
		{
			name:           "empty yaml block",
			markdown:       "```yaml\n```\n",
			wantBlockCount: 1,
			wantOffsets:    []int{2},
			wantContents:   []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := extractYAMLBlocks([]byte(tt.markdown))
			require.Len(t, blocks, tt.wantBlockCount)

			for i, block := range blocks {
				require.Equal(t, tt.wantOffsets[i], block.lineOffset, "block %d lineOffset", i)
				require.Equal(t, tt.wantContents[i], string(block.content), "block %d content", i)
			}
		})
	}
}

func TestExtractYAMLBlocksCRLF(t *testing.T) {
	// CRLF line endings should be handled transparently.
	markdown := "```yaml\r\njobs:\r\n  build:\r\n    steps:\r\n      - uses: actions/checkout@v4\r\n```\r\n"
	blocks := extractYAMLBlocks([]byte(markdown))
	require.Len(t, blocks, 1)
	require.Equal(t, 2, blocks[0].lineOffset)
	// \r should be stripped from content lines.
	require.Equal(t, "jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4", string(blocks[0].content))
}

func TestFindActionRefsInMarkdownFile(t *testing.T) {
	dir := t.TempDir()

	// The uses: line is on markdown file line 7 (fence opens line 5, content
	// starts line 6, uses: is the second content line → line 7).
	markdown := "# README\n\nSome docs.\n\n```yaml\njobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n```\n"
	// Line numbers:        1          2              3       4           5       6       7       8       9                                10

	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(markdown), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	require.Equal(t, "actions/checkout@v4", actions[0].Node.Value)
	// Fence opens on line 5 → content starts line 6. uses: is YAML line 4 → file line 6+4-1 = 9.
	require.Equal(t, 9, actions[0].Node.Line)
}

func TestFindActionRefsInMarkdownFileMultipleBlocks(t *testing.T) {
	dir := t.TempDir()

	markdown := "# Title\n\n```yaml\njobs:\n  a:\n    steps:\n      - uses: actions/checkout@v4\n```\n\nMore prose.\n\n```yaml\njobs:\n  b:\n    steps:\n      - uses: actions/setup-go@v5\n```\n"

	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(markdown), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Len(t, actions, 2)

	values := []string{actions[0].Node.Value, actions[1].Node.Value}
	require.Contains(t, values, "actions/checkout@v4")
	require.Contains(t, values, "actions/setup-go@v5")
}

func TestFindActionRefsInMarkdownFileNoBlocks(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte("# Just prose\n\nNo YAML here.\n"), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Empty(t, actions)
}

func TestFindActionRefsInMarkdownFileMalformedYAML(t *testing.T) {
	dir := t.TempDir()

	// Malformed YAML inside a fence should not cause an error — it's skipped.
	markdown := "```yaml\n: : : invalid\n```\n\n```yaml\njobs:\n  b:\n    steps:\n      - uses: actions/checkout@v4\n```\n"
	path := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(path, []byte(markdown), 0o600))

	actions, err := findActionRefsInMarkdownFile(path)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	require.Equal(t, "actions/checkout@v4", actions[0].Node.Value)
}

func TestFindMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeFile(t, "README.md", "# root\n")
	writeFile(t, "CONTRIBUTING.markdown", "# contrib\n")
	writeFile(t, filepath.Join("docs", "guide.md"), "# guide\n")
	writeFile(t, filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"), "# pr\n")
	// Non-markdown files must be excluded.
	writeFile(t, "workflow.yml", "jobs: {}\n")
	writeFile(t, filepath.Join("docs", "notes.txt"), "ignored\n")
	// Skipped directories must not appear.
	writeFile(t, filepath.Join(".git", "COMMIT_EDITMSG"), "ignored\n")
	writeFile(t, filepath.Join("node_modules", "pkg", "README.md"), "ignored\n")
	writeFile(t, filepath.Join("vendor", "dep", "README.md"), "ignored\n")

	files, err := findMarkdownFiles()
	require.NoError(t, err)

	require.ElementsMatch(t, []string{
		"README.md",
		"CONTRIBUTING.markdown",
		filepath.Join("docs", "guide.md"),
		filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"),
	}, files)
}
