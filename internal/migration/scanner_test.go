package migration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanner_Scan(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"file1.txt":              "content1",
		"file2.txt":              "content2",
		"subdir/file3.txt":       "content3",
		"subdir/nested/file4.go": "package main",
		"node_modules/pkg.json":  "{}",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("scan all files", func(t *testing.T) {
		scanner := NewScanner(tmpDir, nil)
		result, err := scanner.Scan()
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		if result.Count != 5 {
			t.Errorf("expected 5 files, got %d", result.Count)
		}

		if len(result.Files) != 5 {
			t.Errorf("expected 5 files in Files slice, got %d", len(result.Files))
		}

		// Verify total size matches
		var expectedSize int64
		for _, content := range testFiles {
			expectedSize += int64(len(content))
		}
		if result.TotalSize != expectedSize {
			t.Errorf("expected total size %d, got %d", expectedSize, result.TotalSize)
		}
	})

	t.Run("scan with exclusions", func(t *testing.T) {
		scanner := NewScanner(tmpDir, []string{"node_modules/"})
		result, err := scanner.Scan()
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		// Should exclude node_modules directory
		if result.Count != 4 {
			t.Errorf("expected 4 files (excluding node_modules), got %d", result.Count)
		}

		// Verify no node_modules files in results
		for _, f := range result.Files {
			if filepath.HasPrefix(f.RelPath, "node_modules") {
				t.Errorf("found excluded file: %s", f.RelPath)
			}
		}
	})

	t.Run("scan with glob pattern exclusion", func(t *testing.T) {
		scanner := NewScanner(tmpDir, []string{"*.go"})
		result, err := scanner.Scan()
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		// Should exclude .go files
		if result.Count != 4 {
			t.Errorf("expected 4 files (excluding .go), got %d", result.Count)
		}

		// Verify no .go files in results
		for _, f := range result.Files {
			if filepath.Ext(f.RelPath) == ".go" {
				t.Errorf("found excluded .go file: %s", f.RelPath)
			}
		}
	})
}

func TestScanner_LargestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with different sizes
	testFiles := map[string]int{
		"small.txt":  10,
		"medium.txt": 100,
		"large.txt":  1000,
	}

	for name, size := range testFiles {
		path := filepath.Join(tmpDir, name)
		content := make([]byte, size)
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	scanner := NewScanner(tmpDir, nil)
	result, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	t.Run("get top 2 largest files", func(t *testing.T) {
		largest := result.LargestFiles(2)
		if len(largest) != 2 {
			t.Errorf("expected 2 files, got %d", len(largest))
		}

		// First should be largest
		if largest[0].Size != 1000 {
			t.Errorf("expected largest file size 1000, got %d", largest[0].Size)
		}

		// Second should be medium
		if largest[1].Size != 100 {
			t.Errorf("expected second file size 100, got %d", largest[1].Size)
		}
	})

	t.Run("get more files than available", func(t *testing.T) {
		largest := result.LargestFiles(10)
		if len(largest) != 3 {
			t.Errorf("expected 3 files (all available), got %d", len(largest))
		}
	})
}

func TestScanner_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	scanner := NewScanner(tmpDir, nil)
	result, err := scanner.Scan()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if result.Count != 0 {
		t.Errorf("expected 0 files, got %d", result.Count)
	}

	if result.TotalSize != 0 {
		t.Errorf("expected 0 total size, got %d", result.TotalSize)
	}
}

func TestScanner_NonExistentDirectory(t *testing.T) {
	scanner := NewScanner("/nonexistent/path", nil)
	_, err := scanner.Scan()
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}
