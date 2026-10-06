package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	amigrate "ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/sqltool"
)

func TestWriteChecksum(t *testing.T) {
	path := t.TempDir()
	file := filepath.Join(path, "20260101000000_example.sql")
	write := func(sql string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(sql), 0o644); err != nil {
			t.Fatalf("write migration: %v", err)
		}
	}
	validate := func() error {
		t.Helper()
		dir, err := sqltool.NewGooseDir(path)
		if err != nil {
			t.Fatalf("open dir: %v", err)
		}
		return amigrate.Validate(dir)
	}

	write("-- +goose Up\nCREATE TABLE example (id bigint);\n")
	if err := writeChecksum(path); err != nil {
		t.Fatalf("checksum: %v", err)
	}
	if err := validate(); err != nil {
		t.Fatalf("sum file should match right after checksum: %v", err)
	}

	// Uma edição manual deixa o atlas.sum para trás até o próximo checksum.
	write("-- +goose Up\nCREATE TABLE example (id bigint, name text);\n")
	if err := validate(); !errors.Is(err, amigrate.ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch after a manual edit", err)
	}
	if err := writeChecksum(path); err != nil {
		t.Fatalf("checksum after edit: %v", err)
	}
	if err := validate(); err != nil {
		t.Fatalf("sum file should match again: %v", err)
	}
}
