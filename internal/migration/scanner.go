package migration

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/gobwas/glob"
)

// FileInfo represents metadata about a file to be migrated
type FileInfo struct {
	RelPath string
	AbsPath string
	Size    int64
	Mode    os.FileMode
	ModTime int64
}

// ScanResult contains the results of scanning a directory
type ScanResult struct {
	Files     []FileInfo
	Count     int
	TotalSize int64
}

// LargestFiles returns the n largest files from the scan result
func (r *ScanResult) LargestFiles(n int) []FileInfo {
	sorted := make([]FileInfo, len(r.Files))
	copy(sorted, r.Files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Size > sorted[j].Size
	})
	if len(sorted) > n {
		return sorted[:n]
	}
	return sorted
}

// Scanner scans a directory tree and identifies files for migration
type Scanner struct {
	rootPath string
	excludes []glob.Glob
}

// NewScanner creates a new scanner with the given root path and exclusion patterns
func NewScanner(rootPath string, patterns []string) *Scanner {
	excludes := make([]glob.Glob, 0, len(patterns))
	for _, p := range patterns {
		if g, err := glob.Compile(p); err == nil {
			excludes = append(excludes, g)
		}
	}
	return &Scanner{rootPath: rootPath, excludes: excludes}
}

// Scan walks the directory tree and collects file metadata
func (s *Scanner) Scan() (*ScanResult, error) {
	result := &ScanResult{}

	err := filepath.Walk(s.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err // Propagate walk errors
		}

		relPath, _ := filepath.Rel(s.rootPath, path)

		// For root directory, skip exclusion check
		if relPath == "." {
			return nil
		}

		// Check exclusions
		for _, g := range s.excludes {
			// Try matching against relative path with and without trailing slash for directories
			if info.IsDir() {
				if g.Match(relPath+"/") || g.Match(relPath) || g.Match(info.Name()+"/") || g.Match(info.Name()) {
					return filepath.SkipDir
				}
			} else {
				if g.Match(relPath) || g.Match(info.Name()) {
					return nil
				}
			}
		}

		// Skip directories and symlinks
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		result.Files = append(result.Files, FileInfo{
			RelPath: relPath,
			AbsPath: path,
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime().Unix(),
		})
		result.Count++
		result.TotalSize += info.Size()

		return nil
	})

	return result, err
}
