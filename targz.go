// Package targz contains methods to create and extract tar gz archives.
//
// Usage (discarding potential errors):
//   	targz.Compress("path/to/the/directory/to/compress", "my_archive.tar.gz")
//   	targz.Extract("my_archive.tar.gz", "directory/to/extract/to")
// This creates an archive in ./my_archive.tar.gz with the folder "compress" (last in the path).
// And extracts the folder "compress" to "directory/to/extract/to/". The folder structure is created if it doesn't exist.
package targz

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// Compress creates a archive from the folder inputFilePath points to in the file outputFilePath points to.
// Only adds the last directory in inputFilePath to the archive, not the whole path.
// It tries to create the directory structure outputFilePath contains if it doesn't exist.
// It returns potential errors to be checked or nil if everything works.
func Compress(inputFilePath, outputFilePath string) error {
	firstDirCreated, err := mkdirAll(filepath.Dir(outputFilePath), 0755)
	if err != nil {
		if firstDirCreated != "" {
			os.RemoveAll(firstDirCreated)
		}
		return err
	}

	err = compress(inputFilePath, outputFilePath, filepath.Dir(inputFilePath))
	if err != nil {
		if firstDirCreated != "" {
			os.RemoveAll(firstDirCreated)
		} else {
			if exist, _ := exists(outputFilePath); exist {
				os.Remove(outputFilePath)
			}
		}
		return err
	}

	return nil
}

// Extract extracts a archive from the file inputFilePath points to in the directory outputFilePath points to.
// It tries to create the directory structure outputFilePath contains if it doesn't exist.
// It returns potential errors to be checked or nil if everything works.
func Extract(inputFilePath, outputFilePath string) error {
	firstDirCreated, err := mkdirAll(outputFilePath, 0755)
	if err != nil {
		if firstDirCreated != "" {
			os.RemoveAll(firstDirCreated)
		}
		return err
	}

	err = extract(inputFilePath, outputFilePath)
	if err != nil {
		if firstDirCreated != "" {
			os.RemoveAll(firstDirCreated)
		}
		return err
	}

	return err
}

// Creates all directories like os.MakedirAll but returns the path to the first created directory so cleanup is possible.
// The directories under the returned path is created by the mkdirAll call so they should be safe to delete.
func mkdirAll(directory string, perm os.FileMode) (string, error) {
	firstDirCreated := ""
	hierarchy := directoryHierarchy(directory)

	for _, directory := range hierarchy {
		exists, err := exists(directory)
		if err != nil {
			return firstDirCreated, err
		}
		if !exists {
			err := os.Mkdir(directory, perm)
			if err != nil {
				return firstDirCreated, err
			} else if firstDirCreated == "" {
				firstDirCreated = directory
			}
		}
	}

	return firstDirCreated, nil
}

// Returns a list with all paths required for the next level.
// eg. directoryHierarchy("a/b/c") => ["a", "a/b", "a/b/c"]
func directoryHierarchy(directory string) []string {
	hierarchy := make([]string, 0, 5)
	directories := strings.Split(directory, string(os.PathSeparator))

	for index, directory := range directories {
		path := ""
		for _, dir := range directories[:index] {
			path += dir + string(os.PathSeparator)
		}
		path += directory

		if path != "" {
			hierarchy = append(hierarchy, path)
		}
	}

	return hierarchy
}

// Check if path exists or not
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// The main interaction with tar and gzip. Creates a archive and recursivly adds all files in the directory.
// The finished archive contains just the directory added, not any parents.
// This is possible by giving the whole path exept the final directory in subPath.
func compress(inPath, outFilePath, subPath string) error {
	file, err := os.Create(outFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	err = writeDirectory(inPath, tarWriter, subPath)
	if err != nil {
		return err
	}

	return nil
}

// Read a directy and write it to the tar writer. Recursive function that writes all sub folders.
func writeDirectory(directory string, tarWriter *tar.Writer, subPath string) error {
	files, err := ioutil.ReadDir(directory)
	if err != nil {
		return err
	}

	for _, file := range files {
		currentPath := filepath.Join(directory, file.Name())
		if file.IsDir() {
			err := writeDirectory(currentPath, tarWriter, subPath)
			if err != nil {
				return err
			}
		} else {
			err = writeTarGz(currentPath, tarWriter, file, subPath)
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
	defer file.Close()

	header := new(tar.Header)
	header.Name = path[len(subPath):]
	header.Size = fileInfo.Size()
	header.Mode = int64(fileInfo.Mode())
	header.ModTime = fileInfo.ModTime()

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
	defer file.Close()

	gzipReader, err := gzip.NewReader(bufio.NewReader(file))
	if err != nil {
		return err
	}
	defer gzipReader.Close()

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
