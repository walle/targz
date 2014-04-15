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
	"log"
	"os"
	"path/filepath"
)

// Compress creates a archive from the folder inputFilePath points to in the file outputFilePath points to.
// Only adds the last directory in inputFilePath to the archive, not the whole path.
// It tries to create the directory structure outputFilePath contains if it doesn't exist.
// It returns potential errors to be checked or nil if everything works.
func Compress(inputFilePath, outputFilePath string) error {
	err := os.MkdirAll(filepath.Dir(outputFilePath), 0755)
	if err != nil {
		return err
	}

	err = compress(outputFilePath, inputFilePath, filepath.Dir(inputFilePath))

	return err
}

// Extract extracts a archive from the file inputFilePath points to in the directory outputFilePath points to.
// It tries to create the directory structure outputFilePath contains if it doesn't exist.
// It returns potential errors to be checked or nil if everything works.
func Extract(inputFilePath, outputFilePath string) error {
	err := os.MkdirAll(outputFilePath, 0755)
	if err != nil {
		return err
	}

	err = extract(inputFilePath, outputFilePath)

	return err
}

// The main interaction with tar and gzip. Creates a archive and recursivly adds all files in the directory.
// The finished archive contains just the directory added, not any parents.
// This is possible by giving the whole path exept the final directory in subPath.
func compress(outFilePath string, inPath string, subPath string) error {
	file, err := os.Create(outFilePath)
	if handle(err) != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	writeDirectory(inPath, tarWriter, subPath)

	return nil
}

// Extract the file in filePath to directory.
func extract(filePath string, directory string) error {
	file, err := os.Open(filePath)
	if handle(err) != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(bufio.NewReader(file))
	if handle(err) != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if handle(err) != nil {
			return err
		}

		fileInfo := header.FileInfo()
		dir := filepath.Join(directory, filepath.Dir(header.Name))
		filename := filepath.Join(dir, fileInfo.Name())

		err = os.MkdirAll(dir, 0755)
		if handle(err) != nil {
			return err
		}

		file, err := os.Create(filename)
		if handle(err) != nil {
			return err
		}
		defer func() {
			if err := file.Close(); err != nil {
				panic(err)
			}
		}()

		writer := bufio.NewWriter(file)

		buffer := make([]byte, 1024)
		for {
			n, err := tarReader.Read(buffer)
			if err != nil && err != io.EOF {
				panic(err)
			}
			if n == 0 {
				break
			}

			_, err = writer.Write(buffer[:n])
			if handle(err) != nil {
				return err
			}
		}

		err = writer.Flush()
		if handle(err) != nil {
			return err
		}
	}

	return nil
}

// Write path without the prefix in subPath to tar writer.
func writeTarGz(path string, tarWriter *tar.Writer, fileInfo os.FileInfo, subPath string) error {
	file, err := os.Open(path)
	if handle(err) != nil {
		return err
	}
	defer file.Close()

	header := new(tar.Header)
	header.Name = path[len(subPath):]
	header.Size = fileInfo.Size()
	header.Mode = int64(fileInfo.Mode())
	header.ModTime = fileInfo.ModTime()

	err = tarWriter.WriteHeader(header)
	if handle(err) != nil {
		return err
	}

	_, err = io.Copy(tarWriter, file)
	if handle(err) != nil {
		return err
	}

	return err
}

// Read a directy and write it to the tar writer. Recursive function that writes all sub folders.
func writeDirectory(directory string, tarWriter *tar.Writer, subPath string) error {
	files, err := ioutil.ReadDir(directory)
	if handle(err) != nil {
		return err
	}

	for _, file := range files {
		currentPath := filepath.Join(directory, file.Name())
		if file.IsDir() {
			err := writeDirectory(currentPath, tarWriter, subPath)
			if handle(err) != nil {
				return err
			}
		} else {
			err = writeTarGz(currentPath, tarWriter, file, subPath)
			if handle(err) != nil {
				return err
			}
		}
	}

	return nil
}

// Log the error and return it.
func handle(err error) error {
	if err != nil {
		log.Fatal(err)
	}

	return err
}
