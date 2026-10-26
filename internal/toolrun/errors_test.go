package toolrun

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
)

func TestIOAndSerializationErrorsRemainInspectable(t *testing.T) {
	root := t.TempDir()
	if _, err := readTool(filepath.Join(root, "missing"), digest.Digest(strings.Repeat("a", 64))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("lost executable open error", err)
	}
	var unsupported *json.UnsupportedTypeError
	if err := WriteJSON(filepath.Join(root, "invalid.json"), make(chan int)); !errors.As(err, &unsupported) {
		t.Fatal("lost JSON error", err)
	}
	path := filepath.Join(root, "record.json")
	if err := WriteJSON(path, struct{}{}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(path, struct{}{}); !errors.Is(err, os.ErrExist) {
		t.Fatal("lost create error", err)
	}
	if _, err := Inventory(filepath.Join(root, "missing"), "q"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("lost inventory error", err)
	}
	if _, err := Execute(context.Background(), filepath.Join(root, "missing"), "tool", "unused", 32); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("lost output creation error", err)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestOutputWriteFailureIsRetainedAndCancels(t *testing.T) {
	diskErr := errors.New("disk failure")
	for _, test := range []struct {
		name        string
		cause, want error
	}{
		{name: "write error", cause: diskErr, want: diskErr},
		{name: "short write", want: io.ErrShortWrite},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := limitedWriter{w: failingWriter{err: test.cause}, remaining: 32, cancel: cancel}
			if _, err := writer.Write([]byte("s SATISFIABLE\n")); !errors.Is(err, test.want) {
				t.Fatalf("write error = %v, want %v", err, test.want)
			}
			if !errors.Is(writer.err, test.want) || !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("error not retained or process not canceled: %v, %v", writer.err, ctx.Err())
			}
		})
	}
}
