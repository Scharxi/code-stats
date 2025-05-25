package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

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

type ExportStats struct {
	Extension       string  `json:"extension"`
	FileCount       int64   `json:"file_count"`
	TotalLines      int64   `json:"total_lines"`
	CommentLines    int64   `json:"comment_lines"`
	EmptyLines      int64   `json:"empty_lines"`
	AvgLinesPerFile float64 `json:"avg_lines_per_file"`
}

type ExportSummary struct {
	TotalFiles        int64   `json:"total_files"`
	TotalLines        int64   `json:"total_lines"`
	TotalCommentLines int64   `json:"total_comment_lines"`
	TotalEmptyLines   int64   `json:"total_empty_lines"`
	AvgLinesPerFile   float64 `json:"avg_lines_per_file"`
}

type ExportData struct {
	Stats   []ExportStats `json:"stats"`
	Summary ExportSummary `json:"summary"`
}

func runStats(targetDir string, extensions []string, ignoreList []string, useColor bool, showProgress bool, exportJSON, exportCSV, exportHTML bool, outputFile string, verbose bool) {
	var wg sync.WaitGroup
	counter := NewCounter()
	scanDirWithProgress(targetDir, &wg, counter, extensions, ignoreList, showProgress && !verbose)
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

	// Prepare export data
	var exportStats []ExportStats
	for ext := range extCounts {
		avg := float64(0)
		if extCounts[ext] > 0 {
			avg = float64(linesByExt[ext]) / float64(extCounts[ext])
		}
		exportStats = append(exportStats, ExportStats{
			Extension:       ext,
			FileCount:       extCounts[ext],
			TotalLines:      linesByExt[ext],
			CommentLines:    commentByExt[ext],
			EmptyLines:      0, // not tracked per ext
			AvgLinesPerFile: avg,
		})
	}
	exportSummary := ExportSummary{
		TotalFiles:        totalFiles,
		TotalLines:        totalLines,
		TotalCommentLines: totalComment,
		TotalEmptyLines:   totalEmpty,
		AvgLinesPerFile:   counter.GetAverageLinesPerFile(),
	}
	exportData := ExportData{
		Stats:   exportStats,
		Summary: exportSummary,
	}

	if exportJSON && outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating JSON file: %v\n", err)
			return
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(exportData); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
		}
		if !verbose {
			fmt.Printf("Exported stats as JSON to %s\n", outputFile)
		}
	}
	if exportCSV && outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating CSV file: %v\n", err)
			return
		}
		defer f.Close()
		writeStatsCSV(f, exportStats, exportSummary)
		if !verbose {
			fmt.Printf("Exported stats as CSV to %s\n", outputFile)
		}
	}

	if exportHTML && outputFile != "" {
		if err := writeStatsHTML(outputFile, exportStats, exportSummary); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating HTML file: %v\n", err)
		} else if !verbose {
			fmt.Printf("Exported stats as HTML to %s\n", outputFile)
		}
	}

	if verbose {
		return
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

func writeStatsCSV(w io.Writer, stats []ExportStats, summary ExportSummary) {
	csvw := csv.NewWriter(w)
	csvw.Write([]string{"Extension", "File Count", "Total Lines", "Comment Lines", "Empty Lines", "Avg Lines/File"})
	for _, s := range stats {
		csvw.Write([]string{
			s.Extension,
			fmt.Sprintf("%d", s.FileCount),
			fmt.Sprintf("%d", s.TotalLines),
			fmt.Sprintf("%d", s.CommentLines),
			fmt.Sprintf("%d", s.EmptyLines),
			fmt.Sprintf("%.2f", s.AvgLinesPerFile),
		})
	}
	csvw.Write([]string{})
	csvw.Write([]string{"Total Files", "Total Lines", "Total Comment Lines", "Total Empty Lines", "Avg Lines/File"})
	csvw.Write([]string{
		fmt.Sprintf("%d", summary.TotalFiles),
		fmt.Sprintf("%d", summary.TotalLines),
		fmt.Sprintf("%d", summary.TotalCommentLines),
		fmt.Sprintf("%d", summary.TotalEmptyLines),
		fmt.Sprintf("%.2f", summary.AvgLinesPerFile),
	})
	csvw.Flush()
}

func writeStatsHTML(filename string, stats []ExportStats, summary ExportSummary) error {
	const tpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Code Stats Report</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --primary: #2563eb;
            --secondary: #38a169;
            --bg: #f8fafc;
            --card-bg: #fff;
            --border: #e2e8f0;
            --header: #1e293b;
            --shadow: 0 4px 24px #0002;
        }
        html { box-sizing: border-box; }
        *, *:before, *:after { box-sizing: inherit; }
        body {
            font-family: 'Inter', system-ui, sans-serif;
            background: var(--bg);
            color: #222;
            margin: 0;
            padding: 0;
        }
        header {
            background: var(--primary);
            color: #fff;
            padding: 2rem 1rem 1.5rem 1rem;
            text-align: center;
            box-shadow: var(--shadow);
        }
        header h1 {
            margin: 0;
            font-size: 2.5rem;
            font-weight: 700;
            letter-spacing: -1px;
        }
        main {
            max-width: 900px;
            margin: -2rem auto 0 auto;
            padding: 2rem 1rem 3rem 1rem;
        }
        .card {
            background: var(--card-bg);
            border-radius: 1.2rem;
            box-shadow: var(--shadow);
            padding: 2rem 1.5rem;
            margin-bottom: 2.5rem;
        }
        h2 {
            color: var(--header);
            font-size: 1.4rem;
            font-weight: 600;
            margin-top: 0;
        }
        table {
            border-collapse: collapse;
            width: 100%;
            background: var(--card-bg);
            border-radius: 0.7rem;
            overflow: hidden;
            box-shadow: 0 2px 8px #0001;
        }
        th, td {
            border: 1px solid var(--border);
            padding: 0.7rem 1rem;
            text-align: left;
        }
        th {
            background: var(--primary);
            color: #fff;
            font-weight: 600;
            font-size: 1rem;
            letter-spacing: 0.5px;
        }
        .summary-table th { background: var(--secondary); }
        tr:nth-child(even) td { background: #f1f5f9; }
        tr:hover td { background: #e0e7ef; transition: background 0.2s; }
        @media (max-width: 700px) {
            main { padding: 1rem 0.2rem; }
            .card { padding: 1rem 0.5rem; }
            th, td { padding: 0.5rem 0.4rem; font-size: 0.95rem; }
            header h1 { font-size: 1.5rem; }
        }
        footer {
            text-align: center;
            color: #64748b;
            font-size: 0.95rem;
            padding: 1.5rem 0 0.7rem 0;
        }
        a { color: var(--primary); text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <header>
        <h1>Code Stats Report</h1>
        <div>Automatically generated &bull; {{ now }}</div>
    </header>
    <main>
        <div class="card">
            <h2>Per Extension</h2>
            <table>
                <thead>
                    <tr>
                        <th>Extension</th>
                        <th>File Count</th>
                        <th>Total Lines</th>
                        <th>Comment Lines</th>
                        <th>Empty Lines</th>
                        <th>Avg Lines/File</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Stats}}
                    <tr>
                        <td>{{.Extension}}</td>
                        <td>{{.FileCount}}</td>
                        <td>{{.TotalLines}}</td>
                        <td>{{.CommentLines}}</td>
                        <td>{{.EmptyLines}}</td>
                        <td>{{printf "%.2f" .AvgLinesPerFile}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        <div class="card">
            <h2>Summary</h2>
            <table class="summary-table">
                <thead>
                    <tr>
                        <th>Total Files</th>
                        <th>Total Lines</th>
                        <th>Total Comment Lines</th>
                        <th>Total Empty Lines</th>
                        <th>Avg Lines/File</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>{{.Summary.TotalFiles}}</td>
                        <td>{{.Summary.TotalLines}}</td>
                        <td>{{.Summary.TotalCommentLines}}</td>
                        <td>{{.Summary.TotalEmptyLines}}</td>
                        <td>{{printf "%.2f" .Summary.AvgLinesPerFile}}</td>
                    </tr>
                </tbody>
            </table>
        </div>
    </main>
    <footer>
        &copy; {{ year }} Code Stats &mdash; Generated with <a href="https://github.com/jedib0t/go-pretty">go-pretty</a> and <a href="https://github.com/spf13/cobra">Cobra</a>
    </footer>
</body>
</html>`
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	t := template.Must(template.New("stats").Funcs(template.FuncMap{
		"now":  func() string { return (time.Now()).Format("02.01.2006 15:04") },
		"year": func() int { return time.Now().Year() },
	}).Parse(tpl))
	return t.Execute(f, struct {
		Stats   []ExportStats
		Summary ExportSummary
	}{stats, summary})
}

func main() {
	var extFlag string
	var ignoreFlag string
	var colorFlag bool
	var progressFlag bool
	var jsonFlag bool
	var csvFlag bool
	var htmlFlag bool
	var outputFile string
	var verboseFlag bool
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
			runStats(dir, extensions, ignoreList, colorFlag, progressFlag, jsonFlag, csvFlag, htmlFlag, outputFile, verboseFlag)
		},
	}
	rootCmd.Flags().StringVarP(&extFlag, "ext", "e", "", "Comma-separated list of file extensions to include (e.g. 'go,js,ts')")
	rootCmd.Flags().StringVarP(&ignoreFlag, "ignore", "i", "", "Comma-separated list of directories to ignore (e.g. 'node_modules,dist,.git')")
	rootCmd.Flags().BoolVarP(&colorFlag, "color", "c", false, "Enable colored output")
	rootCmd.Flags().BoolVarP(&progressFlag, "progress", "p", false, "Show progress output for each processed file")
	rootCmd.Flags().BoolVar(&jsonFlag, "json", false, "Export stats as JSON (use with -o)")
	rootCmd.Flags().BoolVar(&csvFlag, "csv", false, "Export stats as CSV (use with -o)")
	rootCmd.Flags().BoolVar(&htmlFlag, "html", false, "Export stats as HTML (use with -o)")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file for JSON, CSV or HTML export")
	rootCmd.Flags().BoolVar(&verboseFlag, "verbose", false, "Disable all console output except errors and export confirmation")
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
