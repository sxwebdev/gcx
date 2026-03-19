package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Zip creates zip archives.
type Zip struct{}

func (z *Zip) Extension() string { return "zip" }

func (z *Zip) Archive(srcPath, destPath string) (retErr error) {
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close archive file: %w", err)
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if err := zw.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close zip writer: %w", err)
		}
	}()

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if srcInfo.IsDir() {
		return addDirToZip(zw, srcPath, filepath.Base(srcPath))
	}
	return addFileToZip(zw, srcPath, filepath.Base(srcPath))
}

func addFileToZip(zw *zip.Writer, filePath, nameInZip string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() {
		_ = file.Close() // read-only, safe to ignore
	}()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	header, err := zip.FileInfoHeader(stat)
	if err != nil {
		return fmt.Errorf("create zip header: %w", err)
	}
	header.Name = nameInZip
	header.Method = zip.Deflate

	w, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create zip entry: %w", err)
	}
	if _, err := io.Copy(w, file); err != nil {
		return fmt.Errorf("write file to zip: %w", err)
	}
	return nil
}

func addDirToZip(zw *zip.Writer, dirPath, baseInZip string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}

		var nameInZip string
		if relPath == "." {
			nameInZip = baseInZip
		} else {
			nameInZip = filepath.Join(baseInZip, relPath)
		}

		if info.IsDir() {
			_, err := zw.Create(nameInZip + "/")
			return err
		}

		return addFileToZip(zw, path, nameInZip)
	})
}
