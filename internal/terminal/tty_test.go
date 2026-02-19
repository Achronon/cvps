package terminal

import (
	"testing"
)

func TestGetSize(t *testing.T) {
	// This test may fail in non-terminal environments (CI/CD)
	// but should at least not panic
	cols, rows, err := GetSize()

	// In a non-terminal environment, we expect an error
	// In a terminal, we expect positive dimensions
	if err == nil {
		if cols <= 0 {
			t.Errorf("GetSize() cols = %d, want > 0", cols)
		}
		if rows <= 0 {
			t.Errorf("GetSize() rows = %d, want > 0", rows)
		}
	}
	// If err != nil, it's expected in non-TTY environments
}

func TestSetRaw(t *testing.T) {
	// This test may fail in non-terminal environments
	// We're just testing it doesn't panic
	restore, err := SetRaw()

	if err == nil && restore != nil {
		// Restore immediately if we succeeded
		restore()
	}
	// In non-TTY environments, err is expected
}
