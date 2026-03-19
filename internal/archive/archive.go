package archive

import "fmt"

// Archiver creates an archive from a source path.
type Archiver interface {
	// Archive creates an archive from srcPath and writes it to destPath.
	Archive(srcPath, destPath string) error
	// Extension returns the file extension (e.g., "tar.gz", "zip").
	Extension() string
}

// New creates an Archiver for the given format.
func New(format string) (Archiver, error) {
	switch format {
	case "tar.gz":
		return &TarGz{}, nil
	case "zip":
		return &Zip{}, nil
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", format)
	}
}
