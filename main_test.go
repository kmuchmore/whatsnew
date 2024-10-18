package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
)

func TestProcessDirectories(t *testing.T) {
	// Create temporary directories for input, output, and database
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	dbDir := t.TempDir()

	// Create a temporary file in the input directory
	tempFile, err := os.Create(filepath.Join(inputDir, "testfile.txt"))
	assert.NoError(t, err, "Failed to create temporary file")
	tempFile.Close()

	// Test cases
	tests := []struct {
		name      string
		inputDir  string
		outputDir string
		dbPath    string
		writeFlag bool
		wantErr   bool
	}{
		{
			name:      "Valid directories with write flag",
			inputDir:  inputDir,
			outputDir: filepath.Join(outputDir, "t1"),
			dbPath:    dbDir,
			writeFlag: true,
			wantErr:   false,
		},
		{
			name:      "Invalid input directory",
			inputDir:  "/invalid/input/dir",
			outputDir: filepath.Join(outputDir, "t2"),
			dbPath:    dbDir,
			writeFlag: true,
			wantErr:   true,
		},
		{
			name:      "Valid directories without write flag",
			inputDir:  inputDir,
			outputDir: outputDir,
			dbPath:    filepath.Join(dbDir, "t3"),
			writeFlag: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processDirectories(tt.inputDir, tt.outputDir, tt.dbPath, tt.writeFlag, false)
			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Did not expect an error but got one")
			}

			// Verify that the file was copied to the output directory if no error was expected
			if !tt.wantErr && tt.writeFlag {
				_, err := os.Stat(filepath.Join(tt.outputDir, "testfile.txt"))
				assert.NoError(t, err, "Expected file to be copied to output directory, but it was not")
			}
		})
	}
}
func TestScanAndCompare(t *testing.T) {
	// Create temporary directories for input, output, and database
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	dbDir := t.TempDir()

	// Create a temporary file in the input directory
	tempFile, err := os.Create(filepath.Join(inputDir, "testfile.txt"))
	assert.NoError(t, err, "Failed to create temporary file")
	tempFile.Close()

	// Open Badger database
	dbName := filepath.Join(dbDir, "test.db")
	opts := badger.DefaultOptions(dbName)
	opts.Logger = nil
	db, err := badger.Open(opts)
	assert.NoError(t, err, "Failed to open Badger database")
	defer db.Close()

	// Test cases
	tests := []struct {
		name      string
		inputDir  string
		outputDir string
		writeFlag bool
		dryRun    bool
		wantErr   bool
	}{
		{
			name:      "Valid directories with write flag",
			inputDir:  inputDir,
			outputDir: filepath.Join(outputDir, "t1"),
			writeFlag: true,
			dryRun:    false,
			wantErr:   false,
		},
		{
			name:      "Valid directories with dry run",
			inputDir:  inputDir,
			outputDir: filepath.Join(outputDir, "t2"),
			writeFlag: false,
			dryRun:    true,
			wantErr:   false,
		},
		{
			name:      "Invalid input directory",
			inputDir:  "/invalid/input/dir",
			outputDir: filepath.Join(outputDir, "t3"),
			writeFlag: true,
			dryRun:    false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scanAndCompare(db, tt.inputDir, tt.outputDir, tt.writeFlag, tt.dryRun)
			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				assert.NoError(t, err, "Did not expect an error but got one")
			}

			// Verify that the file was copied to the output directory if no error was expected and not a dry run
			if !tt.wantErr && tt.writeFlag && !tt.dryRun {
				_, err := os.Stat(filepath.Join(tt.outputDir, "testfile.txt"))
				assert.NoError(t, err, "Expected file to be copied to output directory, but it was not")
			}
		})
	}
}

func Benchmark(b *testing.B) {
	numbers := make([]int64, 1000)
	for i := int64(0); i < 1000; i++ {
		numbers[i] = i
	}

	b.ResetTimer()

	b.Run("Sprintf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, n := range numbers {
				_ = fmt.Sprintf("%d", n)
			}
		}
	})
	b.Run("FormatInt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, n := range numbers {
				_ = strconv.FormatInt(n, 10)
			}
		}
	})
	b.Run("binary", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, n := range numbers {
				buf := make([]byte, 8)
				binary.LittleEndian.PutUint64(buf, uint64(n))
			}
		}
	})
}
