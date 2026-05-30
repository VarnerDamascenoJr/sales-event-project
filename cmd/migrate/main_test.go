package main

import "testing"

func TestParseVersion(t *testing.T) {
	version, err := parseVersion("00012_add_check_ins.sql")
	if err != nil {
		t.Fatalf("parse version failed: %v", err)
	}
	if version != 12 {
		t.Fatalf("expected version 12, got %d", version)
	}
}

func TestParseUpSQL(t *testing.T) {
	sql, err := parseUpSQL(`
-- +goose Up
CREATE TABLE example (id UUID PRIMARY KEY);

-- +goose Down
DROP TABLE example;
`, "00001_example.sql")
	if err != nil {
		t.Fatalf("parse up SQL failed: %v", err)
	}
	if sql != "CREATE TABLE example (id UUID PRIMARY KEY);" {
		t.Fatalf("unexpected up SQL: %q", sql)
	}
}

func TestSplitSQLStatements(t *testing.T) {
	statements := splitSQLStatements(`
CREATE TABLE one (id UUID PRIMARY KEY);
CREATE INDEX idx_one_id ON one(id);
`)
	if len(statements) != 2 {
		t.Fatalf("expected two statements, got %d", len(statements))
	}
}
