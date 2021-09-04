package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/walle/targz"
)

func main() {
	// Create a temporary file structiure we can use
	tmpDir, dirToCompress := createExampleData()

	// Comress a folder to my_archive.tar.gz
	err := targz.Compress(dirToCompress, filepath.Join(tmpDir, "my_archive.tar.gz"))
	if err != nil {
		fmt.Println("Comress error")
		panic(err)
		os.Exit(1)
	}

	// Extract my_archive.tar.gz to a new folder called extracted
	err = targz.Extract(filepath.Join(tmpDir, "my_archive.tar.gz"), filepath.Join(tmpDir, "extracted"))
	if err != nil {
		fmt.Println("Extract error")
		panic(err)
		os.Exit(1)
	}

	// Open so we can se the files and remove the directory if we'd like.
	cmd := exec.Command("open", tmpDir)
	cmd.Run()

	os.Exit(0)
}

func createExampleData() (string, string) {
	tmpDir, err := ioutil.TempDir("", "targz-example")
	if err != nil {
		fmt.Println("tmpdir error")
		panic(err)
		os.Exit(1)
	}

	directory := filepath.Join(tmpDir, "my_folder")
	subDirectory := filepath.Join(directory, "my_sub_folder")
	err = os.MkdirAll(subDirectory, 0755)
	if err != nil {
		fmt.Println("mkdir error")
		panic(err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(subDirectory, "my_file.txt"), []byte("example data\n"), 0666); err != nil {
		fmt.Println("create file error")
		panic(err)
		os.Exit(1)
	}

	if err := os.Symlink(filepath.Join(subDirectory, "my_file.txt"), filepath.Join(subDirectory, "my_link")); err != nil {
		fmt.Println("create symlink error")
		panic(err)
		os.Exit(1)
	}

	return tmpDir, directory
}
