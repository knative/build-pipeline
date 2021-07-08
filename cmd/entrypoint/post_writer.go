package main

import (
	"log"
	"os"

	"github.com/tektoncd/pipeline/pkg/entrypoint"
)

// realPostWriter actually writes files.
type realPostWriter struct{}

var _ entrypoint.PostWriter = (*realPostWriter)(nil)

func (*realPostWriter) Write(file string) {
	if file == "" {
		return
	}
	if _, err := os.Create(file); err != nil {
		log.Fatalf("Creating %q: %v", file, err)
	}
}

// WriteFileContent creates the file with the specified content provided the directory structure already exists
func (*realPostWriter) WriteFileContent(file, content string) {
	if file == "" {
		return
	}
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Creating %q: %v", file, err)
	}

	if _, err := f.WriteString(content); err != nil {
		log.Fatalf("Writing %q: %v", file, err)
	}
}

// CreatePath creates the specified path and a symbolic link to the path
func (*realPostWriter) CreatePath(source, link string) {
	if source == "" {
		return
	}
	if err := os.MkdirAll(source, 0770); err != nil {
		log.Fatalf("Creating file path %q: %v", source, err)
	}

	if link == "" {
		return
	}
	// create a symlink if it does not exist
	if _, err := os.Stat(link); os.IsNotExist(err) {
		// check if a source exist before creating a symbolic link
		if _, err := os.Stat(source); err == nil {
			if err := os.Symlink(source, link); err != nil {
				log.Fatalf("Creating a symlink %q: %v", link, err)
			}
		}
	}
}
