package database

import (
	"context"
	"strings"
	"testing"
)

func TestConnectRejectsInvalidDatabaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "wrong scheme", url: "mysql://user:password@localhost:3306/diuk"},
		{name: "malformed", url: "postgresql://user:password@localhost:not-a-port/diuk"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := Connect(context.Background(), tt.url)
			if err == nil {
				db.Close()
				t.Fatal("Connect() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), "database url") {
				t.Errorf("error = %q, want it to mention the database url", err)
			}
		})
	}
}

func TestConnectDoesNotLeakThePassword(t *testing.T) {
	_, err := Connect(context.Background(), "postgresql://user:sup3rsecret@localhost:not-a-port/diuk")
	if err == nil {
		t.Fatal("Connect() error = nil, want an error")
	}
	if strings.Contains(err.Error(), "sup3rsecret") {
		t.Errorf("error leaks the password: %v", err)
	}
}
