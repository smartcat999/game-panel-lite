package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

func (a *Adapter) LogsWorkload(ctx context.Context, runtimeID string, follow bool) (io.ReadCloser, error) {
	stream, err := a.client.ContainerLogs(ctx, runtimeID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: follow, Tail: "120"})
	if err != nil {
		return nil, err
	}
	return demuxLogStream(stream), nil
}

func (a *Adapter) LogSnapshotWorkload(ctx context.Context, runtimeID string) (io.ReadCloser, error) {
	stream, err := a.client.ContainerLogs(ctx, runtimeID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: false, Tail: "300"})
	if err != nil {
		return nil, err
	}
	return demuxLogStream(stream), nil
}

func demuxLogStream(stream io.ReadCloser) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		defer stream.Close()
		_, copyErr := stdcopy.StdCopy(writer, writer, stream)
		_ = writer.CloseWithError(copyErr)
	}()
	return reader
}

func (a *Adapter) SendCommandWorkload(ctx context.Context, runtimeID string, command string) error {
	conn, err := a.client.ContainerAttach(ctx, runtimeID, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Conn.Write([]byte(command + "\n"))
	return err
}
