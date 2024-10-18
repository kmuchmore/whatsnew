package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/dgraph-io/badger/v4"
)

type CLI struct {
	Input  string `short:"i" required:"" help:"Directory to scan"`
	Output string `short:"o" required:"" help:"Directory to copy new files to"`
	Write  bool   `short:"w" help:"Update the Badger database"`
	DryRun bool   `short:"d" help:"Perform a dry run without making any changes"`
	DBPath string `default:"." help:"Directory to store the Badger database"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	err := processDirectories(cli.Input, cli.Output, cli.DBPath, cli.Write, cli.DryRun)
	if err != nil {
		log.Fatal(err)
	}
}

// processDirectories handles the main logic of validating paths, scanning directories, and updating the database
func processDirectories(inputDir, outputDir, dbPath string, writeFlag, dryRun bool) error {
	var err error
	// Validate and sanitize input paths (Abs calls Clean internally)
	inputDir, err = filepath.Abs(inputDir)
	if err != nil {
		return fmt.Errorf("invalid input directory '%s': %w", inputDir, err)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("invalid output directory '%s': %w", outputDir, err)
	}
	dbPath, err = filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("invalid database directory '%s': %w", dbPath, err)
	}

	if !dryRun {
		// Create output directory if it doesn't exist
		err = os.MkdirAll(outputDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
		}
	}

	tmpDir := filepath.Join(os.TempDir(), "whatsnew")
	defer os.RemoveAll(tmpDir)
	// Open Badger database
	opts := badger.DefaultOptions(tmpDir).WithLogger(nil)
	// opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		return err
	}
	defer db.Close()

	dbName := filepath.Join(dbPath, filepath.Base(inputDir)+".db")

	if _, err := os.Stat(dbName); err == nil {
		err := func() error {
			f, err := os.Open(dbName)
			if err != nil {
				return fmt.Errorf("failed to open database file '%s': %w", dbPath, err)
			}
			defer f.Close()
			if err := db.Load(f, 10); err != nil {
				return fmt.Errorf("failed to load database file '%s': %w", dbPath, err)
			}
			fmt.Printf("Loaded database from file: %s\n", dbName)
			return nil
		}()
		if err != nil {
			return err
		}
	}

	// Scan the input directory and compare/store files
	err = scanAndCompare(db, inputDir, outputDir, writeFlag, dryRun)
	if err != nil {
		return err
	}

	if writeFlag {
		if err = db.Sync(); err != nil {
			return fmt.Errorf("failed to sync database: %w", err)
		}
		// Create database directory if it doesn't exist
		err = os.MkdirAll(dbPath, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create database directory '%s': %w", dbPath, err)
		}
		f, err := os.Create(dbName)
		if err != nil {
			return fmt.Errorf("failed to open database file '%s': %w", dbName, err)
		}
		defer f.Close()
		if _, err := db.Backup(f, 0); err != nil {
			return fmt.Errorf("failed to backup database file '%s': %w", dbName, err)
		}
		fmt.Printf("Saved database to file: %s\n", dbName)
	}

	return nil
}

// scanAndCompare scans the input directory, compares with the database, and stores new entries
func scanAndCompare(db *badger.DB, inputDir, outputDir string, writeFlag, dryRun bool) error {
	return filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		return db.Update(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(path))
			if err == badger.ErrKeyNotFound {
				return handleNewFile(txn, path, info, inputDir, outputDir, writeFlag, dryRun)
			} else if err == nil {
				return handleExistingFile(txn, item, path, info, inputDir, outputDir, writeFlag, dryRun)
			}
			return err
		})
	})
}

// handleNewFile processes a new file found during the scan
func handleNewFile(txn *badger.Txn, path string, info os.FileInfo, inputDir, outputDir string, writeFlag, dryRun bool) error {
	fmt.Println("New:", path)
	if writeFlag {
		if err := txn.Set([]byte(path), int64ToBytes(info.Size())); err != nil {
			return fmt.Errorf("failed to store file '%s' in database: %w", path, err)
		}
	}
	if dryRun {
		return nil
	}
	return copyFileToOutput(path, inputDir, outputDir)
}

// handleExistingFile processes an existing file found during the scan
func handleExistingFile(txn *badger.Txn, item *badger.Item, path string, info os.FileInfo, inputDir, outputDir string, writeFlag, dryRun bool) error {
	var storedSize int64
	if err := item.Value(func(val []byte) error {
		storedSize = bytesToInt64(val)
		return nil
	}); err != nil {
		return err
	}

	if storedSize != info.Size() {
		fmt.Println("Updated:", path)
		if writeFlag {
			if err := txn.Set([]byte(path), int64ToBytes(info.Size())); err != nil {
				return fmt.Errorf("failed to update file '%s' in database: %w", path, err)
			}
		}
		if dryRun {
			return nil
		}
		return copyFileToOutput(path, inputDir, outputDir)
	}
	return nil
}

// int64ToBytes converts an int64 to a byte slice
func int64ToBytes(num int64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(num))
	return buf
}

func bytesToInt64(buf []byte) int64 {
	return int64(binary.LittleEndian.Uint64(buf))
}

// copyFileToOutput copies a file from the input directory to the output directory
func copyFileToOutput(path, inputDir, outputDir string) error {
	relPath, err := filepath.Rel(inputDir, path)
	if err != nil {
		return fmt.Errorf("failed to get relative path for file '%s': %w", path, err)
	}
	destPath := filepath.Join(outputDir, relPath)
	if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory for file '%s': %w", path, err)
	}
	return copyFile(path, destPath)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file '%s': %w", src, err)
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file '%s': %w", dst, err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file from '%s' to '%s': %w", src, dst, err)
	}
	return err
}
