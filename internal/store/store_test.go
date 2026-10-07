package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// An install from before the rename (barn.db) keeps its data.
func TestAdoptsLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	old, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.CreateFirstUser(ctx, "martin", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	old.Close()
	if err := os.Rename(filepath.Join(dir, "openbot.db"), filepath.Join(dir, "barn.db")); err != nil {
		t.Fatal(err)
	}

	st, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if u, err := st.PrimaryUser(ctx); err != nil || u.Username != "martin" {
		t.Fatalf("user = %+v, %v", u, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "barn.db")); !os.IsNotExist(err) {
		t.Fatal("barn.db should have been renamed")
	}
}
