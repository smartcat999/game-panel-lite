package tasklog

import (
	"context"
	"io"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

func (s *store) Stream(ctx context.Context, key string, opts StreamOptions) (io.ReadCloser, error) {
	if !opts.Follow || opts.Completed || s.bus == nil {
		st, err := s.loadState(ctx, key)
		if err != nil {
			return nil, err
		}
		return linesReader(tailSlice(st.Lines, opts.TailLines)), nil
	}
	return s.followStream(ctx, key, opts.TailLines)
}

func (s *store) followStream(ctx context.Context, key string, tailLines int) (io.ReadCloser, error) {
	events := make(chan mq.Event, 128)
	sub, err := s.bus.Subscribe(ctx, topicForKey(key), randomGroupID(), func(_ context.Context, ev mq.Event) error {
		select {
		case events <- ev:
		default:
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	st, err := s.loadState(ctx, key)
	if err != nil {
		_ = sub.Unsubscribe()
		return nil, err
	}
	pr, pw := io.Pipe()
	go s.pumpFollowPipe(ctx, pw, sub, events, st, tailLines)
	return pr, nil
}

func (s *store) pumpFollowPipe(
	ctx context.Context,
	pw *io.PipeWriter,
	sub mq.Subscription,
	events <-chan mq.Event,
	initial logState,
	tailLines int,
) {
	defer func() {
		_ = sub.Unsubscribe()
		_ = pw.Close()
	}()
	if !writeLines(pw, tailSlice(initial.Lines, tailLines)) || initial.Completed {
		return
	}
	lastSeq := initial.Seq
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			seq, _ := strconv.ParseInt(ev.Attributes["seq"], 10, 64)
			if seq > 0 && seq <= lastSeq {
				continue
			}
			if seq > lastSeq {
				lastSeq = seq
			}
			if ev.Type == EventEOF {
				return
			}
			if ev.Type == EventLine && ev.Attributes["chunk"] != "" {
				if !writeChunk(pw, ev.Attributes["chunk"]) {
					return
				}
			}
		}
	}
}

func linesReader(lines []string) io.ReadCloser {
	if len(lines) == 0 {
		return io.NopCloser(strings.NewReader(""))
	}
	return io.NopCloser(strings.NewReader(strings.Join(lines, "\n") + "\n"))
}

func writeLines(w io.Writer, lines []string) bool {
	if len(lines) == 0 {
		return true
	}
	return writeChunk(w, strings.Join(lines, "\n"))
}

func writeChunk(w io.Writer, chunk string) bool {
	if !strings.HasSuffix(chunk, "\n") {
		chunk += "\n"
	}
	_, err := io.WriteString(w, chunk)
	return err == nil
}
