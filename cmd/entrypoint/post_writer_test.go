package main

import (
	"os"
	"testing"
)

func TestRealPostWriter_WriteFileContent(t *testing.T) {
	tests := []struct {
		name, file, content string
	}{{
		name:    "write a file content",
		file:    "sample.txt",
		content: "this is a sample file",
	}, {
		name: "write a file without specifying any path",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := realPostWriter{}
			rw.WriteFileContent(tt.file, tt.content)
			if tt.file != "" {
				defer os.Remove(tt.file)
				if _, err := os.Stat(tt.file); err != nil {
					t.Fatalf("Failed to create a file %q", tt.file)
				}
				b, err := os.ReadFile(tt.file)
				if err != nil {
					t.Fatalf("Failed to read the file %q", tt.file)
				}
				if tt.content != string(b) {
					t.Fatalf("Failed to write the desired content %q to the file %q", tt.content, tt.file)
				}
			}
		})
	}
}

func TestRealPostWriter_CreateStepPath(t *testing.T) {
	tests := []struct {
		name, source, link string
	}{{
		name:   "Create a path with a file",
		source: "sample.txt",
		link:   "0",
	}, {
		name: "Create a path without specifying any path",
	}, {
		name:   "Create a sym link without specifying any link path",
		source: "sample.txt",
	}, {
		name: "Create a sym link without specifying any source",
		link: "0.txt",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := realPostWriter{}
			rw.CreatePath(tt.source, tt.link)
			if tt.source != "" {
				defer os.Remove(tt.source)
				if _, err := os.Stat(tt.source); err != nil {
					t.Fatalf("Failed to create a file %q", tt.source)
				}
			}
			if tt.source != "" && tt.link != "" {
				defer os.Remove(tt.link)
				if _, err := os.Stat(tt.link); err != nil {
					t.Fatalf("Failed to create a sym link %q", tt.link)
				}
			}
		})
	}
}
