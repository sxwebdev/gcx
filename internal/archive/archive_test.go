package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("tar.gz", func(t *testing.T) {
		a, err := New("tar.gz")
		if err != nil {
			t.Fatal(err)
		}
		if a.Extension() != "tar.gz" {
			t.Errorf("Extension() = %q, want %q", a.Extension(), "tar.gz")
		}
	})

	t.Run("zip", func(t *testing.T) {
		a, err := New("zip")
		if err != nil {
			t.Fatal(err)
		}
		if a.Extension() != "zip" {
			t.Errorf("Extension() = %q, want %q", a.Extension(), "zip")
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		_, err := New("rar")
		if err == nil {
			t.Error("expected error for unsupported format")
		}
	})
}

func TestTarGzArchiveFile(t *testing.T) {
	dir := t.TempDir()

	// Create a source file
	srcFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(srcFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	destFile := filepath.Join(dir, "hello.tar.gz")
	a := &TarGz{}
	if err := a.Archive(srcFile, destFile); err != nil {
		t.Fatal(err)
	}

	// Verify archive contents
	f, err := os.Open(destFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != "hello.txt" {
		t.Errorf("Name = %q, want %q", hdr.Name, "hello.txt")
	}

	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Errorf("content = %q, want %q", string(content), "hello world")
	}
}

func TestTarGzArchiveDir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "myapp_v1.0.0_linux_amd64")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "myapp"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	destFile := filepath.Join(dir, "myapp.tar.gz")
	a := &TarGz{}
	if err := a.Archive(srcDir, destFile); err != nil {
		t.Fatal(err)
	}

	// Verify archive has entries
	f, err := os.Open(destFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
	}

	if len(names) < 2 {
		t.Errorf("expected at least 2 entries, got %d: %v", len(names), names)
	}
}

func TestZipArchiveFile(t *testing.T) {
	dir := t.TempDir()

	srcFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(srcFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	destFile := filepath.Join(dir, "hello.zip")
	a := &Zip{}
	if err := a.Archive(srcFile, destFile); err != nil {
		t.Fatal(err)
	}

	// Verify archive contents
	r, err := zip.OpenReader(destFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	if len(r.File) != 1 {
		t.Fatalf("expected 1 file in zip, got %d", len(r.File))
	}
	if r.File[0].Name != "hello.txt" {
		t.Errorf("Name = %q, want %q", r.File[0].Name, "hello.txt")
	}

	rc, err := r.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Errorf("content = %q, want %q", string(content), "hello world")
	}
}
