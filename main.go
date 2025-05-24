package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

var fileExtensions = []string{".go", ".js", ".ts", ".html", ".css", ".json", ".md", ".txt", ".yaml", ".yml", ".toml", ".ini", ".env", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".psd1", ".pssc", ".psscx", ".psscy", ".psscz", ".pssc0", ".pssc1", ".pssc2", ".pssc3", ".pssc4", ".pssc5", ".pssc6", ".pssc7", ".pssc8", ".pssc9", ".pssc10"}

type Counter struct {
	mu    sync.Mutex
	total int64
}

func (c *Counter) Inc() {
	c.mu.Lock()
	c.total++
	c.mu.Unlock()
}

func (c *Counter) Value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

func main() {
	dir := "/Users/lucas/Development" // Change to your target directory

	var wg sync.WaitGroup
	counter := Counter{}
	scanDir(dir, &wg, &counter)
	wg.Wait()
	fmt.Println("All files processed.")
	fmt.Println("Total files:", counter.Value())
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
			go func(p string) {
				defer wg.Done()
				counter.Inc()
				fmt.Println("Processing:", p)
			}(fullPath)
		}
	}
}
