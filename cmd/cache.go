package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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

	if len(entries) == 0 {
		fmt.Println("Cache is empty.")
		return nil
	}

	terminal := term.FromEnv()
	w, _, _ := terminal.Size()
	if w == 0 {
		w = 80
	}
	tp := tableprinter.New(os.Stdout, terminal.IsTerminalOutput(), w)

	tp.AddField("TOOL")
	tp.AddField("SIZE")
	tp.AddField("FILES")
	tp.EndRow()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		toolCacheDir := filepath.Join(dirs.Cache, entry.Name())
		size, count := dirStats(toolCacheDir)

		tp.AddField(entry.Name())
		tp.AddField(formatSize(size))
		tp.AddField(fmt.Sprintf("%d", count))
		tp.EndRow()
	}

	return tp.Render()
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

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
