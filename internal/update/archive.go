package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// AssetName returns the GoReleaser archive name for a release target.
func AssetName(tag, goos, goarch string) string {
	return fmt.Sprintf("corum_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), goos, goarch)
}

// Verify checks data against the exact archive entry in checksums.txt.
func Verify(data []byte, name, checksums string) error {
	wantName := strings.TrimSpace(name)
	got := fmt.Sprintf("%x", sha256.Sum256(data))
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != wantName {
			continue
		}
		if fields[0] != got {
			return fmt.Errorf("checksum mismatch for %s", wantName)
		}
		return nil
	}
	return fmt.Errorf("checksums.txt has no entry for %s", wantName)
}

// Extract reads the regular corum binary from a tar.gz archive without writing
// any archive-controlled path to disk.
func Extract(data []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("archive has no corum binary")
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if header.Typeflag == tar.TypeReg && path.Base(header.Name) == "corum" {
			binary, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("extract corum: %w", err)
			}
			return binary, nil
		}
	}
}
