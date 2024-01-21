// Package targz contains methods to create and extract tar gz archives.
//
// Usage (discarding potential errors):
//
//	targz.Compress("path/to/the/directory/to/compress", "my_archive.tar.gz")
//	targz.Extract("my_archive.tar.gz", "directory/to/extract/to")
//
// This creates an archive in ./my_archive.tar.gz with the folder "compress" (last in the path).
// And extracts the folder "compress" to "directory/to/extract/to/". The folder structure is created if it doesn't exist.
package targz

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

// Compress creates an archive from the folder inputFilePath points to in the file outputFilePath points to.
// Only adds the last directory in inputFilePath to the archive, not the whole path.
// It tries to create the directory structure outputFilePath contains if it doesn't exist.
// It returns potential errors to be checked or nil if everything works.
func Compress(inputFilePath, outputFilePath string) (err error) {
	inputFilePath = stripTrailingSlashes(inputFilePath)
	inputFilePath, outputFilePath, err = makeAbsolute(inputFilePath, outputFilePath)
	if err != nil {
		return err
	}
	undoDir, err := mkdirAll(filepath.Dir(outputFilePath), 0755)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			handleErrorInDefer(undoDir())
		}
	}()

	// Return error if wildcard is used elsewhere than in the last path element
	if dir, _ := filepath.Split(inputFilePath); strings.Contains(dir, "*") {
		return errors.New("the wildcard \"*\" can be used only in the last path element")
	}

	var subPath string
	if strings.Contains(inputFilePath, "*") {
		inputFilePath = inputFilePath[:len(inputFilePath)-1]
		subPath = inputFilePath
	} else {
		subPath = filepath.Dir(inputFilePath)
	}

	err = compress(inputFilePath, outputFilePath, subPath)
	if err != nil {
		return err
	}

	return nil
}

// Extract extracts an archive from the file inputFilePath points to in the directory outputFilePath points to.
// It tries to create the directory structure outputFilePath contains if it doesn't exist.
// It returns potential errors to be checked or nil if everything works.
func Extract(inputFilePath, outputFilePath string) (err error) {
	outputFilePath = stripTrailingSlashes(outputFilePath)
	inputFilePath, outputFilePath, err = makeAbsolute(inputFilePath, outputFilePath)
	if err != nil {
		return err
	}
	undoDir, err := mkdirAll(outputFilePath, 0755)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			handleErrorInDefer(undoDir())
		}
	}()

	return extract(inputFilePath, outputFilePath)
}

// Creates all directories with os.MkdirAll and returns a function to remove the first created directory so cleanup is possible.
func mkdirAll(dirPath string, perm os.FileMode) (func() error, error) {
	var undoDir string
	defaultReturnFunc := func() error { return nil }

	for p := dirPath; ; p = path.Dir(p) {
		finfo, err := os.Stat(p)

		if err == nil {
			if finfo.IsDir() {
				break
			}

			finfo, err = os.Lstat(p)
			if err != nil {
				return defaultReturnFunc, err
			}

			if finfo.IsDir() {
				break
			}

			return defaultReturnFunc, &os.PathError{Op: "mkdirAll", Path: p, Err: syscall.ENOTDIR}
		}

		if os.IsNotExist(err) {
			undoDir = p
		} else {
			return defaultReturnFunc, err
		}
	}

	if undoDir == "" {
		return defaultReturnFunc, nil
	}

	if err := os.MkdirAll(dirPath, perm); err != nil {
		return defaultReturnFunc, err
	}

	return func() error {
		return os.RemoveAll(undoDir)
	}, nil
}

// Remove trailing slash if any.
func stripTrailingSlashes(path string) string {
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[0 : len(path)-1]
	}

	return path
}

// Make input and output paths absolute.
func makeAbsolute(inputFilePath, outputFilePath string) (string, string, error) {
	inputFilePath, err := filepath.Abs(inputFilePath)
	if err == nil {
		outputFilePath, err = filepath.Abs(outputFilePath)
	}

	return inputFilePath, outputFilePath, err
}

// The main interaction with tar and gzip. Creates an archive and recursively adds all files in the directory.
// The finished archive contains just the directory added, not any parents.
// This is possible by giving the whole path except the final directory in subPath.
func compress(inPath, outFilePath, subPath string) (err error) {
	files, err := os.ReadDir(inPath)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return errors.New("targz: input directory is empty")
	}

	file, err := os.Create(outFilePath)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			handleErrorInDefer(os.Remove(outFilePath))
		}
	}()

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	err = writeDirectory(inPath, tarWriter, subPath)
	if err != nil {
		return err
	}

	err = tarWriter.Close()
	if err != nil {
		return err
	}

	err = gzipWriter.Close()
	if err != nil {
		return err
	}

	err = file.Close()
	if err != nil {
		return err
	}

	return nil
}

// Read a directory and write it to the tar writer. Recursive function that writes all sub folders.
func writeDirectory(directory string, tarWriter *tar.Writer, subPath string) error {
	// Handle wildcards
	if strings.Contains(directory, "*") {
		matches, err := filepath.Glob(directory)
		if err != nil {
			return err
		}

		for _, match := range matches {
			if err := writeDirectory(match, tarWriter, subPath); err != nil {
				return err
			}
		}

		return nil
	}

	files, err := os.ReadDir(directory)
	if err != nil {
		return err
	}

	for _, dirEntry := range files {
		currentPath := filepath.Join(directory, dirEntry.Name())
		if dirEntry.IsDir() {
			err := writeDirectory(currentPath, tarWriter, subPath)
			if err != nil {
				return err
			}
		} else {
			fileInfo, err := dirEntry.Info()
			if err != nil {
				return err
			}

			err = writeTarGz(currentPath, tarWriter, fileInfo, subPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Write path without the prefix in subPath to tar writer.
func writeTarGz(path string, tarWriter *tar.Writer, fileInfo os.FileInfo, subPath string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		handleErrorInDefer(file.Close())
	}()

	evaluatedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}

	subPath, err = filepath.EvalSymlinks(subPath)
	if err != nil {
		return err
	}

	link := ""
	if evaluatedPath != path {
		link = evaluatedPath
	}

	header, err := tar.FileInfoHeader(fileInfo, link)
	if err != nil {
		return err
	}
	header.Name = evaluatedPath[len(subPath):]

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, file)
	if err != nil {
		return err
	}

	return err
}

// Extract the file in filePath to directory.
func extract(filePath string, directory string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() {
		handleErrorInDefer(file.Close())
	}()

	gzipReader, err := gzip.NewReader(bufio.NewReader(file))
	if err != nil {
		return err
	}
	defer func() {
		handleErrorInDefer(gzipReader.Close())
	}()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		fileInfo := header.FileInfo()
		dir := filepath.Join(directory, filepath.Dir(header.Name))
		filename := filepath.Join(dir, fileInfo.Name())

		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return err
		}

		file, err := os.Create(filename)
		if err != nil {
			return err
		}

		writer := bufio.NewWriter(file)

		buffer := make([]byte, 4096)
		for {
			n, err := tarReader.Read(buffer)
			if err != nil && err != io.EOF {
				panic(err)
			}
			if n == 0 {
				break
			}

			_, err = writer.Write(buffer[:n])
			if err != nil {
				return err
			}
		}

		err = writer.Flush()
		if err != nil {
			return err
		}

		err = file.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func handleErrorInDefer(err error) {
	if err != nil {
		fmt.Printf("Erorr occurred: %s\n", err.Error())
		os.Exit(1)
	}
}
