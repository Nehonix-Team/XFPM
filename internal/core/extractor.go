package core

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
)

type StreamingExtractor struct {
	cas *Cas
}

func NewStreamingExtractor(cas *Cas) *StreamingExtractor {
	return &StreamingExtractor{cas: cas}
}

// Extract extracts a .tgz stream into the CAS and returns a map of filenames to their hashes
func (e *StreamingExtractor) Extract(reader io.Reader) (map[string]string, error) {
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	fileMap := make(map[string]string, 512)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		cleanPath := filepath.ToSlash(filepath.Clean(header.Name))
		if _, exists := fileMap[cleanPath]; exists {
			continue
		}

		isExecutable := (header.Mode & 0111) != 0

		// Store in CAS
		hash, err := e.cas.StoreStream(tr, isExecutable)
		if err != nil {
			return nil, fmt.Errorf("failed to store %s in CAS: %w", cleanPath, err)
		}
		fileMap[cleanPath] = hash
	}

	return fileMap, nil
}
