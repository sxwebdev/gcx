package build

// Artifact holds structured metadata about a built binary.
// This eliminates the fragile filename-parsing approach.
type Artifact struct {
	BinaryName string
	Version    string
	OS         string
	Arch       string
	Arm        string
	DirPath    string // path to the directory containing the binary
}
