package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Tar takes a source and variable writers and walks 'source' writing each file
// found to the tar writer; the purpose for accepting multiple writers is to allow
// for multiple outputs (for example a file, or md5 hash)
func Tar(src string, gz bool, writer io.Writer, progress io.Writer) error {

	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("Unable to tar files - %v", err.Error())
	}

	var tw *tar.Writer

	if gz {
		gzw := gzip.NewWriter(writer)
		defer gzw.Close()
		tw = tar.NewWriter(gzw)
	} else {
		tw = tar.NewWriter(writer)

	}
	defer tw.Close()
	filesCount := 0
	filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		filesCount++
		return nil
	})
	var currentProgress float64
	// walk path
	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
		currentProgress++
		progress.Write([]byte(fmt.Sprintf("\r%d%%", int(currentProgress/float64(filesCount)*100))))
		// return on any error
		if err != nil {
			return err
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, src, "", -1), string(filepath.Separator))

		// write the header
		if errHeader := tw.WriteHeader(header); errHeader != nil {
			return errHeader
		}

		// return on directories since there will be no content to tar
		if fi.Mode().IsDir() {
			return nil
		}

		// open files for taring
		f, err := os.Open(file)
		defer f.Close()
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		return nil
	})
}
