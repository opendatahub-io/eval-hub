package sql

import (
	"testing"

	"github.com/eval-hub/eval-hub/pkg/api"
)

func TestStampCollectionOverrideSHA(t *testing.T) {
	t.Parallel()

	t.Run("nil collection is no-op", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{ID: "col-1"}
		stampCollectionOverrideSHA(ref, nil, 0, "abc123")
		if len(ref.Benchmarks) != 0 {
			t.Fatalf("expected no benchmarks appended, got %d", len(ref.Benchmarks))
		}
	})

	t.Run("out of range index is no-op", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{ID: "col-1"}
		col := &api.CollectionResource{CollectionConfig: api.CollectionConfig{
			Benchmarks: []api.CollectionBenchmarkConfig{
				{Ref: api.Ref{ID: "b1"}, ProviderID: "p1"},
			},
		}}
		stampCollectionOverrideSHA(ref, col, 5, "abc123")
		if len(ref.Benchmarks) != 0 {
			t.Fatalf("expected no benchmarks appended for out-of-range index, got %d", len(ref.Benchmarks))
		}
	})

	t.Run("stamps SHA on existing matching override", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{
			ID: "col-1",
			Benchmarks: []api.EvaluationBenchmarkConfig{
				{
					Ref:         api.Ref{ID: "b1"},
					ProviderID:  "p1",
					TestDataRef: &api.TestDataRef{Git: &api.GitTestDataRef{URL: "https://example.com"}},
				},
			},
		}
		col := &api.CollectionResource{CollectionConfig: api.CollectionConfig{
			Benchmarks: []api.CollectionBenchmarkConfig{
				{Ref: api.Ref{ID: "b1"}, ProviderID: "p1"},
			},
		}}
		stampCollectionOverrideSHA(ref, col, 0, "sha256")
		if ref.Benchmarks[0].TestDataRef.ResolvedSHA != "sha256" {
			t.Fatalf("ResolvedSHA = %q, want sha256", ref.Benchmarks[0].TestDataRef.ResolvedSHA)
		}
		if ref.Benchmarks[0].TestDataRef.Git == nil || ref.Benchmarks[0].TestDataRef.Git.URL != "https://example.com" {
			t.Fatal("existing Git source info was lost")
		}
	})

	t.Run("creates override with nil TestDataRef on existing entry", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{
			ID: "col-1",
			Benchmarks: []api.EvaluationBenchmarkConfig{
				{Ref: api.Ref{ID: "b1"}, ProviderID: "p1"},
			},
		}
		col := &api.CollectionResource{CollectionConfig: api.CollectionConfig{
			Benchmarks: []api.CollectionBenchmarkConfig{
				{Ref: api.Ref{ID: "b1"}, ProviderID: "p1"},
			},
		}}
		stampCollectionOverrideSHA(ref, col, 0, "sha256")
		if ref.Benchmarks[0].TestDataRef == nil || ref.Benchmarks[0].TestDataRef.ResolvedSHA != "sha256" {
			t.Fatalf("expected TestDataRef with ResolvedSHA=sha256, got %+v", ref.Benchmarks[0].TestDataRef)
		}
	})

	t.Run("appends override preserving collection TestDataRef source info", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{ID: "col-1"}
		col := &api.CollectionResource{CollectionConfig: api.CollectionConfig{
			Benchmarks: []api.CollectionBenchmarkConfig{
				{
					Ref:         api.Ref{ID: "b1"},
					ProviderID:  "p1",
					TestDataRef: &api.TestDataRef{Git: &api.GitTestDataRef{URL: "https://example.com", Ref: "main"}},
				},
			},
		}}
		stampCollectionOverrideSHA(ref, col, 0, "abc123")
		if len(ref.Benchmarks) != 1 {
			t.Fatalf("expected 1 override appended, got %d", len(ref.Benchmarks))
		}
		appended := ref.Benchmarks[0]
		if appended.ID != "b1" || appended.ProviderID != "p1" {
			t.Fatalf("appended override identity = (%q, %q), want (b1, p1)", appended.ID, appended.ProviderID)
		}
		if appended.TestDataRef == nil {
			t.Fatal("expected non-nil TestDataRef")
		}
		if appended.TestDataRef.ResolvedSHA != "abc123" {
			t.Fatalf("ResolvedSHA = %q, want abc123", appended.TestDataRef.ResolvedSHA)
		}
		if appended.TestDataRef.Git == nil || appended.TestDataRef.Git.URL != "https://example.com" {
			t.Fatal("Git source info was not preserved in appended override")
		}
	})

	t.Run("appends override with nil collection TestDataRef", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{ID: "col-1"}
		col := &api.CollectionResource{CollectionConfig: api.CollectionConfig{
			Benchmarks: []api.CollectionBenchmarkConfig{
				{Ref: api.Ref{ID: "b1"}, ProviderID: "p1"},
			},
		}}
		stampCollectionOverrideSHA(ref, col, 0, "abc123")
		if len(ref.Benchmarks) != 1 {
			t.Fatalf("expected 1 override appended, got %d", len(ref.Benchmarks))
		}
		if ref.Benchmarks[0].TestDataRef == nil || ref.Benchmarks[0].TestDataRef.ResolvedSHA != "abc123" {
			t.Fatalf("expected ResolvedSHA=abc123, got %+v", ref.Benchmarks[0].TestDataRef)
		}
	})

	t.Run("duplicate IDs use occurrence to select correct override", func(t *testing.T) {
		t.Parallel()
		ref := &api.CollectionRef{
			ID: "col-dup",
			Benchmarks: []api.EvaluationBenchmarkConfig{
				{
					Ref:         api.Ref{ID: "arc_easy"},
					ProviderID:  "leh",
					TestDataRef: &api.TestDataRef{Git: &api.GitTestDataRef{URL: "url-0"}},
				},
				{
					Ref:         api.Ref{ID: "arc_easy"},
					ProviderID:  "leh",
					TestDataRef: &api.TestDataRef{Git: &api.GitTestDataRef{URL: "url-1"}},
				},
			},
		}
		col := &api.CollectionResource{CollectionConfig: api.CollectionConfig{
			Benchmarks: []api.CollectionBenchmarkConfig{
				{Ref: api.Ref{ID: "arc_easy"}, ProviderID: "leh"},
				{Ref: api.Ref{ID: "arc_easy"}, ProviderID: "leh"},
			},
		}}

		stampCollectionOverrideSHA(ref, col, 0, "sha-first")
		if ref.Benchmarks[0].TestDataRef.ResolvedSHA != "sha-first" {
			t.Fatalf("index 0: ResolvedSHA = %q, want sha-first", ref.Benchmarks[0].TestDataRef.ResolvedSHA)
		}
		if ref.Benchmarks[1].TestDataRef.ResolvedSHA != "" {
			t.Fatalf("index 1 should not have been modified, got ResolvedSHA = %q", ref.Benchmarks[1].TestDataRef.ResolvedSHA)
		}

		stampCollectionOverrideSHA(ref, col, 1, "sha-second")
		if ref.Benchmarks[1].TestDataRef.ResolvedSHA != "sha-second" {
			t.Fatalf("index 1: ResolvedSHA = %q, want sha-second", ref.Benchmarks[1].TestDataRef.ResolvedSHA)
		}
	})
}
