package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"bxfferoverflow.me/code-stats/parser"
	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
)

var fileExtensions = []string{".go", ".rs", ".js", ".ts", ".html", ".css", ".json", ".md", ".txt", ".yaml", ".yml", ".toml", ".ini", ".env", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".psd1", ".pssc", ".psscx", ".psscy", ".psscz", ".pssc0", ".pssc1", ".pssc2", ".pssc3", ".pssc4", ".pssc5", ".pssc6", ".pssc7", ".pssc8", ".pssc9", ".pssc10"}
var ignoreDirs = []string{".git", ".idea", ".vscode", ".DS_Store", "build", "dist", "node_modules", "vendor", "tmp", "logs", "cache", ".next", ".venv"}

type Counter struct {
	mu                sync.Mutex
	total             int64
	totalFiles        int64
	emptyLines        int64
	commentLinesByExt map[string]int64
	linesByExt        map[string]int64
	byExt             map[string]int64
}

func (c *Counter) EmptyLines() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emptyLines
}

func NewCounter() *Counter {
	return &Counter{
		byExt:             make(map[string]int64),
		linesByExt:        make(map[string]int64),
		commentLinesByExt: make(map[string]int64),
		emptyLines:        0,
		totalFiles:        0,
	}
}

func (c *Counter) Inc(ext string, lines int, emptyLines int, commentLines int) {
	c.mu.Lock()
	c.total++
	c.totalFiles++
	c.byExt[ext]++
	c.linesByExt[ext] += int64(lines)
	c.emptyLines += int64(emptyLines)
	c.commentLinesByExt[ext] += int64(commentLines)
	c.mu.Unlock()
}

func (c *Counter) GetAverageLinesPerFile() float64 {
	return float64(c.Lines()) / float64(c.totalFiles)
}

func (c *Counter) GetAverageLinesPerFileByExt(ext string) float64 {
	return float64(c.LinesByExt()[ext]) / float64(c.byExt[ext])
}

func (c *Counter) Value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

func (c *Counter) Lines() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := int64(0)
	for _, count := range c.linesByExt {
		total += count
	}
	return total
}

func (c *Counter) LinesByExt() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.linesByExt
}

func (c *Counter) CommentLinesByExt() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.commentLinesByExt
}

func (c *Counter) ExtCounts() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Return a copy to avoid race conditions
	result := make(map[string]int64, len(c.byExt))
	for k, v := range c.byExt {
		result[k] = v
	}
	return result
}

func shouldIgnoreDir(name string, ignoreList []string) bool {
	for _, ignore := range ignoreList {
		if name == ignore {
			return true
		}
	}
	return false
}

func runStats(targetDir string, extensions []string, ignoreList []string, useColor bool, showProgress bool) {
	var wg sync.WaitGroup
	counter := NewCounter()
	scanDirWithProgress(targetDir, &wg, counter, extensions, ignoreList, showProgress)
	wg.Wait()

	extCounts := counter.ExtCounts()
	linesByExt := counter.LinesByExt()
	commentByExt := counter.CommentLinesByExt()
	totalFiles := counter.Value()
	totalLines := counter.Lines()
	totalEmpty := counter.EmptyLines()

	totalComment := int64(0)
	for _, v := range commentByExt {
		totalComment += v
	}

	// Extension-Tabelle
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	extHeader := table.Row{"Extension", "File Count", "Total Lines", "Comment Lines", "Empty Lines", "Avg Lines/File"}
	if useColor {
		cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
		for i, h := range extHeader {
			if s, ok := h.(string); ok {
				extHeader[i] = cyan(s)
			}
		}
	}
	t.AppendHeader(extHeader)
	for ext := range extCounts {
		avg := float64(0)
		if extCounts[ext] > 0 {
			avg = float64(linesByExt[ext]) / float64(extCounts[ext])
		}
		row := table.Row{
			ext,
			extCounts[ext],
			linesByExt[ext],
			commentByExt[ext],
			0, // Empty lines per ext not tracked
			fmt.Sprintf("%.2f", avg),
		}
		if useColor {
			row[0] = color.New(color.FgGreen, color.Bold).Sprint(row[0])
		}
		t.AppendRow(row)
	}
	t.Render()

	// Gesamtsummen-Tabelle
	sumT := table.NewWriter()
	sumT.SetOutputMirror(os.Stdout)
	sumHeader := table.Row{"Total Files", "Total Lines", "Total Comment Lines", "Total Empty Lines", "Avg Lines/File"}
	if useColor {
		magenta := color.New(color.FgMagenta, color.Bold).SprintFunc()
		for i, h := range sumHeader {
			if s, ok := h.(string); ok {
				sumHeader[i] = magenta(s)
			}
		}
	}
	sumT.AppendHeader(sumHeader)
	sumT.AppendRow(table.Row{
		totalFiles,
		totalLines,
		totalComment,
		totalEmpty,
		fmt.Sprintf("%.2f", counter.GetAverageLinesPerFile()),
	})
	sumT.Render()
}

func main() {
	var extFlag string
	var ignoreFlag string
	var colorFlag bool
	var progressFlag bool
	var rootCmd = &cobra.Command{
		Use:   "code-stats [directory]",
		Short: "Count files, lines, comments, and more in a codebase.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			extensions := fileExtensions
			if extFlag != "" {
				// Split and normalize extensions
				parts := strings.Split(extFlag, ",")
				extensions = make([]string, 0, len(parts))
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						if !strings.HasPrefix(p, ".") {
							p = "." + p
						}
						extensions = append(extensions, p)
					}
				}
			}
			ignoreList := ignoreDirs
			if ignoreFlag != "" {
				parts := strings.Split(ignoreFlag, ",")
				ignoreList = make([]string, 0, len(parts))
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						ignoreList = append(ignoreList, p)
					}
				}
			}
			runStats(dir, extensions, ignoreList, colorFlag, progressFlag)
		},
	}
	rootCmd.Flags().StringVarP(&extFlag, "ext", "e", "", "Comma-separated list of file extensions to include (e.g. 'go,js,ts')")
	rootCmd.Flags().StringVarP(&ignoreFlag, "ignore", "i", "", "Comma-separated list of directories to ignore (e.g. 'node_modules,dist,.git')")
	rootCmd.Flags().BoolVarP(&colorFlag, "color", "c", false, "Enable colored output")
	rootCmd.Flags().BoolVarP(&progressFlag, "progress", "p", false, "Show progress output for each processed file")
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func countLines(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading file:", path, err)
		return 0
	}
	return len(strings.Split(string(content), "\n"))
}

func countEmptyLines(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading file:", path, err)
		return 0
	}

	lines := strings.Split(string(content), "\n")
	emptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyLines++
		}
	}
	return emptyLines
}

func countCommentLines(path, ext string) int {
	parser, ok := parser.CommentParsers[ext]
	if !ok {
		return 0
	}

	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading file:", path, err)
		return 0
	}

	lines := strings.Split(string(content), "\n")
	commentLines := 0
	for _, line := range lines {
		if parser.IsComment(line) {
			commentLines++
		}
	}
	return commentLines
}

func scanDirWithProgress(dir string, wg *sync.WaitGroup, counter *Counter, extensions []string, ignoreList []string, showProgress bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("Error reading directory:", dir, err)
		return
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		ext := filepath.Ext(fullPath)
		if entry.IsDir() {
			if shouldIgnoreDir(entry.Name(), ignoreList) {
				continue
			}
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				scanDirWithProgress(p, wg, counter, extensions, ignoreList, showProgress)
			}(fullPath)
		} else if slices.Contains(extensions, ext) {
			wg.Add(1)
			go func(p, ext string) {
				defer wg.Done()
				lines := countLines(p)
				emptyLines := countEmptyLines(p)
				commentLines := countCommentLines(p, ext)
				counter.Inc(ext, lines, emptyLines, commentLines)
				if showProgress {
					fmt.Printf("Processing: %s Lines: %d\n", p, lines)
				}
			}(fullPath, ext)
		}
	}
}
