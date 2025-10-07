package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompression(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "compression_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test XML file
	testXML := `<?xml version="1.0" encoding="UTF-8"?>
<root>
    <job>
        <id>test123</id>
        <symbol>AAPL</symbol>
        <timeframe>1D</timeframe>
        <parameters>
            <param1>value1</param1>
            <param2>value2</param2>
        </parameters>
    </job>
</root>`

	xmlPath := filepath.Join(tempDir, "test.xml")
	err = os.WriteFile(xmlPath, []byte(testXML), 0644)
	if err != nil {
		t.Fatalf("Failed to create test XML file: %v", err)
	}

	// Test compression
	jobPath, err := CompressXMLFile(xmlPath, true)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	// Check that the compressed file exists
	if _, err := os.Stat(jobPath); os.IsNotExist(err) {
		t.Fatalf("Compressed file was not created: %s", jobPath)
	}

	// Check that the original XML file was deleted
	if _, err := os.Stat(xmlPath); !os.IsNotExist(err) {
		t.Fatalf("Original XML file was not deleted: %s", xmlPath)
	}

	// Test decompression
	decompressedPath, err := DecompressJobFile(jobPath)
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	// Check that the decompressed file exists
	if _, err := os.Stat(decompressedPath); os.IsNotExist(err) {
		t.Fatalf("Decompressed file was not created: %s", decompressedPath)
	}

	// Read and verify the decompressed content
	decompressedContent, err := os.ReadFile(decompressedPath)
	if err != nil {
		t.Fatalf("Failed to read decompressed file: %v", err)
	}

	if string(decompressedContent) != testXML {
		t.Fatalf("Decompressed content does not match original")
	}

	t.Logf("Compression test passed successfully")
	t.Logf("Original size: %d bytes", len(testXML))

	compressedInfo, _ := os.Stat(jobPath)
	t.Logf("Compressed size: %d bytes", compressedInfo.Size())

	compressionRatio := float64(compressedInfo.Size()) / float64(len(testXML)) * 100
	t.Logf("Compression ratio: %.1f%%", compressionRatio)
}
