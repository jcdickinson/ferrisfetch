package db

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("creating test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestValidateEmbedding(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		if err := validateEmbedding([]float32{0.1, 0.2, 0.3}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if err := validateEmbedding([]float32{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("NaN", func(t *testing.T) {
		err := validateEmbedding([]float32{1.0, float32(math.NaN()), 3.0})
		if err == nil {
			t.Fatal("expected error for NaN")
		}
	})

	t.Run("Inf", func(t *testing.T) {
		err := validateEmbedding([]float32{float32(math.Inf(1))})
		if err == nil {
			t.Fatal("expected error for Inf")
		}
	})
}

func TestInsertEmbedding(t *testing.T) {
	db := testDB(t)

	emb := make([]float32, 1024)
	for i := range emb {
		emb[i] = float32(i) / 1024.0
	}

	t.Run("valid", func(t *testing.T) {
		if err := db.InsertEmbedding("hash1", "chunk text", 0, emb); err != nil {
			t.Fatal(err)
		}
		if !db.HasEmbeddings("hash1") {
			t.Error("expected HasEmbeddings=true after insert")
		}
	})

	t.Run("wrong_dimension", func(t *testing.T) {
		err := db.InsertEmbedding("hash2", "text", 0, []float32{1, 2, 3})
		if err == nil {
			t.Fatal("expected error for wrong dimension")
		}
	})

	t.Run("NaN_rejected", func(t *testing.T) {
		bad := make([]float32, 1024)
		bad[0] = float32(math.NaN())
		err := db.InsertEmbedding("hash3", "text", 0, bad)
		if err == nil {
			t.Fatal("expected error for NaN embedding")
		}
	})
}

func TestVectorSearch(t *testing.T) {
	db := testDB(t)

	// Insert two embeddings with different content hashes
	emb1 := make([]float32, 1024)
	emb2 := make([]float32, 1024)
	for i := range emb1 {
		emb1[i] = 1.0
		emb2[i] = -1.0
	}

	if err := db.InsertEmbedding("hash_a", "text a", 0, emb1); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertEmbedding("hash_b", "text b", 0, emb2); err != nil {
		t.Fatal(err)
	}

	// Search with emb1 — should find hash_a as most similar
	results, err := db.VectorSearch(emb1, 0.0, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].ContentHash != "hash_a" {
		t.Errorf("expected hash_a first, got %s", results[0].ContentHash)
	}

	// Search with high threshold — should filter out dissimilar
	results, err = db.VectorSearch(emb1, 0.99, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.ContentHash == "hash_b" {
			t.Error("hash_b should be filtered by high threshold")
		}
	}

	// Search with crate filter — need items for crate filtering to work
	crate, err := db.UpsertCrate("testcrate", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertItem(&Item{CrateID: crate.ID, RustdocID: "1", Name: "A", Path: "A", Kind: "struct", ContentHash: "hash_a"}); err != nil {
		t.Fatal(err)
	}
	results, err = db.VectorSearch(emb1, 0.0, 10, []int{crate.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ContentHash != "hash_a" {
		t.Errorf("crate filter: expected only hash_a, got %v", results)
	}

	// Limit
	results, err = db.VectorSearch(emb1, 0.0, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("limit=1 but got %d results", len(results))
	}
}

func TestGetCratesForItems(t *testing.T) {
	db := testDB(t)

	t.Run("empty", func(t *testing.T) {
		result, err := db.GetCratesForItems(nil)
		if err != nil {
			t.Fatal(err)
		}
		if result != nil {
			t.Error("expected nil for empty input")
		}
	})

	crate, err := db.UpsertCrate("mycrate", "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	item := &Item{CrateID: crate.ID, RustdocID: "100", Name: "Foo", Path: "mycrate::Foo", Kind: "struct"}
	if err := db.InsertItem(item); err != nil {
		t.Fatal(err)
	}

	t.Run("single", func(t *testing.T) {
		result, err := db.GetCratesForItems([]int{item.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if result[item.ID].Name != "mycrate" {
			t.Errorf("expected mycrate, got %s", result[item.ID].Name)
		}
	})

	item2 := &Item{CrateID: crate.ID, RustdocID: "101", Name: "Bar", Path: "mycrate::Bar", Kind: "fn"}
	if err := db.InsertItem(item2); err != nil {
		t.Fatal(err)
	}

	t.Run("multiple", func(t *testing.T) {
		result, err := db.GetCratesForItems([]int{item.ID, item2.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}
	})
}

func TestResolveReexport(t *testing.T) {
	db := testDB(t)
	crate, err := db.UpsertCrate("mylib", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Insert exact re-export
	if err := db.InsertReexport(crate.ID, "mylib::re::Thing", "dep", "dep::original::Thing"); err != nil {
		t.Fatal(err)
	}
	// Insert glob re-export
	if err := db.InsertReexport(crate.ID, "mylib::prelude", "dep", "dep::types"); err != nil {
		t.Fatal(err)
	}

	t.Run("exact_match", func(t *testing.T) {
		src, path, found := db.ResolveReexport(crate.ID, "mylib::re::Thing")
		if !found {
			t.Fatal("expected match")
		}
		if src != "dep" || path != "dep::original::Thing" {
			t.Errorf("got src=%s path=%s", src, path)
		}
	})

	t.Run("glob_prefix", func(t *testing.T) {
		src, path, found := db.ResolveReexport(crate.ID, "mylib::prelude::Widget")
		if !found {
			t.Fatal("expected glob match")
		}
		if src != "dep" {
			t.Errorf("expected dep, got %s", src)
		}
		if path != "dep::types::Widget" {
			t.Errorf("expected dep::types::Widget, got %s", path)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		_, _, found := db.ResolveReexport(crate.ID, "mylib::unrelated::Stuff")
		if found {
			t.Error("expected no match")
		}
	})
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
