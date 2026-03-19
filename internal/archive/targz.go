package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// TarGz creates tar.gz archives.
type TarGz struct{}

func (t *TarGz) Extension() string { return "tar.gz" }

func (t *TarGz) Archive(srcPath, destPath string) (retErr error) {
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create archive file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close archive file: %w", err)
		}
	}()

	gw := gzip.NewWriter(f)
	defer func() {
		if err := gw.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close gzip writer: %w", err)
		}
	}()

	tw := tar.NewWriter(gw)
	defer func() {
		if err := tw.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("close tar writer: %w", err)
		}
	}()

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if srcInfo.IsDir() {
		return addDirToTar(tw, srcPath, filepath.Base(srcPath))
	}
	return addFileToTar(tw, srcPath, filepath.Base(srcPath))
}

func addFileToTar(tw *tar.Writer, filePath, nameInTar string) error {
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

	header := &tar.Header{
		Name:    nameInTar,
		Size:    stat.Size(),
		Mode:    int64(stat.Mode()),
		ModTime: stat.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("write file to tar: %w", err)
	}
	return nil
}

func addDirToTar(tw *tar.Writer, dirPath, baseInTar string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}

		var nameInTar string
		if relPath == "." {
			nameInTar = baseInTar
		} else {
			nameInTar = filepath.Join(baseInTar, relPath)
		}

		if info.IsDir() {
			header := &tar.Header{
				Name:     nameInTar + "/",
				Mode:     int64(info.Mode()),
				ModTime:  info.ModTime(),
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(header)
		}

		return addFileToTar(tw, path, nameInTar)
	})
}
