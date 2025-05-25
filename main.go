package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

var fileExtensions = []string{".go", ".js", ".ts", ".html", ".css", ".json", ".md", ".txt", ".yaml", ".yml", ".toml", ".ini", ".env", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".psd1", ".pssc", ".psscx", ".psscy", ".psscz", ".pssc0", ".pssc1", ".pssc2", ".pssc3", ".pssc4", ".pssc5", ".pssc6", ".pssc7", ".pssc8", ".pssc9", ".pssc10"}

type Counter struct {
	mu         sync.Mutex
	total      int64
	linesByExt map[string]int64
	byExt      map[string]int64
}

func NewCounter() *Counter {
	return &Counter{
		byExt:      make(map[string]int64),
		linesByExt: make(map[string]int64),
	}
}

func (c *Counter) Inc(ext string, lines int) {
	c.mu.Lock()
	c.total++
	c.byExt[ext]++
	c.linesByExt[ext] += int64(lines)
	c.mu.Unlock()
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

func main() {
	dir := "/Users/lucas/Development" // Change to your target directory

	var wg sync.WaitGroup
	counter := NewCounter()
	scanDir(dir, &wg, counter)
	wg.Wait()
	fmt.Println("All files processed.")
	fmt.Println("Total files:", counter.Value())
	fmt.Println("Counts by extension:")
	for ext, count := range counter.ExtCounts() {
		fmt.Printf("%s: %d\n", ext, count)
	}
	fmt.Println("Lines by extension:")
	for ext, count := range counter.LinesByExt() {
		fmt.Printf("%s: %d\n", ext, count)
	}
	fmt.Println("Total lines:", counter.Lines())
}

func countLines(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading file:", path, err)
		return 0
	}
	return len(strings.Split(string(content), "\n"))
}

func scanDir(dir string, wg *sync.WaitGroup, counter *Counter) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("Error reading directory:", dir, err)
		return
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		ext := filepath.Ext(fullPath)
		if entry.IsDir() {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				scanDir(p, wg, counter)
			}(fullPath)
		} else if slices.Contains(fileExtensions, ext) {
			wg.Add(1)
			go func(p, ext string) {
				defer wg.Done()
				lines := countLines(p)
				counter.Inc(ext, lines)
				fmt.Println("Processing:", p, "Lines:", lines)
			}(fullPath, ext)
		}
	}
}
