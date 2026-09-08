package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type cancellingWriter struct{ cancel context.CancelFunc }

func (w cancellingWriter) Write(p []byte) (int, error) { w.cancel(); return len(p), nil }

func TestScopedArchive(t *testing.T) {
	source := fstest.MapFS{"worlds/save": {Data: []byte("world")}, "runtime": {Data: []byte("excluded")}}
	metadata := Metadata{FormatVersion: 1, GameKey: "game", ProviderKey: "provider", ConfigVersion: 2}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, source, "worlds", &metadata); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 {
		t.Fatal("unexpected archive files")
	}
	for _, file := range reader.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch file.Name {
		case MetadataPath:
			var got Metadata
			if json.Unmarshal(data, &got) != nil || got != metadata {
				t.Fatal("metadata changed")
			}
		case "worlds/save":
			if string(data) != "world" {
				t.Fatal("content changed")
			}
		default:
			t.Fatal("scope escaped", file.Name)
		}
	}
}

func TestArchiveRejectsInvalidSources(t *testing.T) {
	for _, tc := range []struct {
		source fstest.MapFS
		path   string
	}{
		{fstest.MapFS{"world": {Data: []byte("world")}}, "../world"},
		{fstest.MapFS{MetadataPath: {Data: []byte("forged")}}, "."},
		{fstest.MapFS{"link": {Mode: fs.ModeSymlink, Data: []byte("outside")}}, "."},
		{fstest.MapFS{"pipe": {Mode: fs.ModeNamedPipe}}, "."},
	} {
		if err := Write(context.Background(), io.Discard, tc.source, tc.path, nil); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	if err := Write(context.Background(), io.Discard, fstest.MapFS{}, ".", &Metadata{}); err == nil {
		t.Fatal("invalid metadata accepted")
	}
}

func TestArchiveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := fstest.MapFS{"world": {Data: []byte(strings.Repeat("world", 10000))}}
	if err := Write(ctx, cancellingWriter{cancel}, source, ".", nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	var out bytes.Buffer
	if err := Write(ctx, &out, source, ".", nil); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("cancelled call wrote output")
	}
}

func TestArchiveCopyStopsBetweenChunks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data := bytes.Repeat([]byte("a"), 256<<10)
	copied, err := io.Copy(cancellingWriter{cancel}, contextReader{ctx: ctx, source: bytes.NewReader(data)})
	if !errors.Is(err, context.Canceled) || copied <= 0 || copied >= int64(len(data)) {
		t.Fatalf("copy continued after cancellation: %d %v", copied, err)
	}
}
