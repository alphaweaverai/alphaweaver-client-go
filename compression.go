package main

import (
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CompressFile compresses a file using zlib with best compression level
// This matches the compression used in the DLL: boost::iostreams::zlib_compressor(boost::iostreams::zlib::best_compression)
func CompressFile(inputFile, outputFile string) error {
	// Open input file
	input, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file %s: %w", inputFile, err)
	}
	defer input.Close()

	// Create output file
	output, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", outputFile, err)
	}
	defer output.Close()

	// Create zlib writer with best compression level (9)
	writer, err := zlib.NewWriterLevel(output, zlib.BestCompression)
	if err != nil {
		return fmt.Errorf("failed to create zlib writer: %w", err)
	}
	defer writer.Close()

	// Copy data from input to compressed output
	_, err = io.Copy(writer, input)
	if err != nil {
		return fmt.Errorf("failed to compress file: %w", err)
	}

	return nil
}

// DecompressFile decompresses a zlib compressed file
func DecompressFile(inputFile, outputFile string) error {
	// Open input file
	input, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file %s: %w", inputFile, err)
	}
	defer input.Close()

	// Create output file
	output, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", outputFile, err)
	}
	defer output.Close()

	// Create zlib reader
	reader, err := zlib.NewReader(input)
	if err != nil {
		return fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer reader.Close()

	// Copy data from compressed input to output
	_, err = io.Copy(output, reader)
	if err != nil {
		return fmt.Errorf("failed to decompress file: %w", err)
	}

	return nil
}

// CompressXMLFile compresses an XML file and optionally deletes the original
// Returns the path to the compressed file
func CompressXMLFile(xmlPath string, deleteOriginal bool) (string, error) {
	// Create compressed file path with .job extension
	dir := filepath.Dir(xmlPath)
	baseName := filepath.Base(xmlPath)
	jobName := filepath.Join(dir, baseName[:len(baseName)-len(filepath.Ext(baseName))]+".job")

	// Compress the file
	err := CompressFile(xmlPath, jobName)
	if err != nil {
		return "", fmt.Errorf("failed to compress XML file: %w", err)
	}

	// Delete original XML file if requested
	if deleteOriginal {
		err = os.Remove(xmlPath)
		if err != nil {
			return jobName, fmt.Errorf("compression successful but failed to delete original file: %w", err)
		}
	}

	return jobName, nil
}

// DecompressJobFile decompresses a .job file back to XML
func DecompressJobFile(jobPath string) (string, error) {
	// Create XML file path
	dir := filepath.Dir(jobPath)
	baseName := filepath.Base(jobPath)
	xmlName := filepath.Join(dir, baseName[:len(baseName)-len(filepath.Ext(baseName))]+".xml")

	// Decompress the file
	err := DecompressFile(jobPath, xmlName)
	if err != nil {
		return "", fmt.Errorf("failed to decompress job file: %w", err)
	}

	return xmlName, nil
}
