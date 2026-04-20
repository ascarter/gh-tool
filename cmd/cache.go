package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the download cache",
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached downloads",
	RunE:  runCacheList,
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean [tool]",
	Short: "Remove cached downloads",
	Long: `Remove cached downloads.

With no arguments, removes the entire cache.
With an argument, removes the cache for a specific tool.`,
	RunE: runCacheClean,
}

func init() {
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
	rootCmd.AddCommand(cacheCmd)
}

func runCacheList(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()

	entries, err := os.ReadDir(dirs.Cache)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Cache is empty.")
			return nil
		}
		return err
	}

	type row struct {
		name  string
		size  int64
		count int
	}
	var rows []row
	var totalSize int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		size, count := dirStats(filepath.Join(dirs.Cache, entry.Name()))
		rows = append(rows, row{entry.Name(), size, count})
		totalSize += size
	}

	if len(rows) == 0 {
		fmt.Println("Cache is empty.")
		return nil
	}

	terminal := term.FromEnv()
	w, _, _ := terminal.Size()
	if w == 0 {
		w = 80
	}
	tp := tableprinter.New(os.Stdout, terminal.IsTerminalOutput(), w)

	// Compute column widths so the divider row matches each header.
	maxName := len("TOOL")
	maxSize := len("SIZE")
	maxFiles := len("FILES")
	formatted := make([]struct {
		name, size, files string
	}, len(rows))
	for i, r := range rows {
		formatted[i].name = r.name
		formatted[i].size = formatSize(r.size)
		formatted[i].files = fmt.Sprintf("%d", r.count)
		if l := len(r.name); l > maxName {
			maxName = l
		}
		if l := len(formatted[i].size); l > maxSize {
			maxSize = l
		}
		if l := len(formatted[i].files); l > maxFiles {
			maxFiles = l
		}
	}

	tp.AddField("TOOL")
	tp.AddField("SIZE")
	tp.AddField("FILES")
	tp.EndRow()
	tp.AddField(strings.Repeat("-", maxName))
	tp.AddField(strings.Repeat("-", maxSize))
	tp.AddField(strings.Repeat("-", maxFiles))
	tp.EndRow()

	for _, f := range formatted {
		tp.AddField(f.name)
		tp.AddField(f.size)
		tp.AddField(fmt.Sprintf("%*s", maxFiles, f.files))
		tp.EndRow()
	}

	if err := tp.Render(); err != nil {
		return err
	}

	noun := "tools"
	if len(rows) == 1 {
		noun = "tool"
	}
	fmt.Printf("\n%d cached %s, %s total\n",
		len(rows), noun, strings.TrimSpace(formatSize(totalSize)))
	return nil
}

func runCacheClean(cmd *cobra.Command, args []string) error {
	dirs := resolveDirs()

	if len(args) > 0 {
		cacheDir := dirs.CacheDir(args[0])
		if err := os.RemoveAll(cacheDir); err != nil {
			return err
		}
		fmt.Printf("✓ Cleaned cache for %s\n", args[0])
		return nil
	}

	if err := os.RemoveAll(dirs.Cache); err != nil {
		return err
	}
	fmt.Println("✓ Cleaned all cached downloads")
	return nil
}

func dirStats(dir string) (totalSize int64, fileCount int) {
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		totalSize += info.Size()
		fileCount++
		return nil
	})
	return
}

// formatSize renders a byte count with a fixed-width numeric portion so
// values line up vertically when emitted in a table column. The numeric
// part is right-padded to 5 characters ("999.9") and followed by a 2-char
// unit (B, KB, MB, GB).
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%5.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%5.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%5.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%5d B ", bytes)
	}
}
