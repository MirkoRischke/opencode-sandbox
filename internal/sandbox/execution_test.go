package sandbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	m "github.com/superradcompany/microsandbox/sdk/go"
)

func commandEvents(events ...m.ExecEvent) func(context.Context) (*m.ExecEvent, error) {
	return func(context.Context) (*m.ExecEvent, error) {
		if len(events) == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		event := events[0]
		events = events[1:]
		return &event, nil
	}
}

func TestGuestExecutionKeepsOutputStreamsSeparate(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	events := commandEvents(
		m.ExecEvent{Kind: m.ExecEventStdout, Data: []byte("output")},
		m.ExecEvent{Kind: m.ExecEventStderr, Data: []byte("warning")},
		m.ExecEvent{Kind: m.ExecEventExited, ExitCode: 0},
		m.ExecEvent{Kind: m.ExecEventDone},
	)

	err := receiveCommand(t.Context(), events, &stdout, &stderr, nil)

	require.NoError(t, err)
	require.Equal(t, "output", stdout.String())
	require.Equal(t, "warning", stderr.String())
}

func TestGuestExecutionReportsExitAfterDrainingOutput(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	events := commandEvents(
		m.ExecEvent{Kind: m.ExecEventExited, ExitCode: 7},
		m.ExecEvent{Kind: m.ExecEventStdout, Data: []byte("last output")},
		m.ExecEvent{Kind: m.ExecEventDone},
	)

	err := receiveCommand(t.Context(), events, &stdout, nil, nil)

	require.ErrorContains(t, err, "exited with code 7")
	require.Equal(t, "last output", stdout.String())
}

func TestGuestExecutionPropagatesOutputFailures(t *testing.T) {
	t.Parallel()
	for _, stream := range []struct {
		name string
		kind m.ExecEventKind
	}{{"stdout", m.ExecEventStdout}, {"stderr", m.ExecEventStderr}} {
		t.Run(stream.name, func(t *testing.T) {
			t.Parallel()
			failure := errors.New("output unavailable")
			events := commandEvents(m.ExecEvent{Kind: stream.kind, Data: []byte("output")})
			writer := failingCommandWriter{err: failure}

			err := receiveCommand(t.Context(), events, writer, writer, nil)

			require.ErrorIs(t, err, failure)
		})
	}
}

func TestGuestExecutionSuppressesInputFailureDetails(t *testing.T) {
	t.Parallel()
	input := make(chan error, 1)
	input <- errors.New("FAKE_TOKEN")
	events := commandEvents(m.ExecEvent{Kind: m.ExecEventExited}, m.ExecEvent{Kind: m.ExecEventDone})

	err := receiveCommand(t.Context(), events, nil, nil, input)

	require.EqualError(t, err, "write guest stdin failed")
}

func TestGuestExecutionCancelsWhileWaitingForInput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	events := commandEvents(m.ExecEvent{Kind: m.ExecEventExited}, m.ExecEvent{Kind: m.ExecEventDone})

	err := receiveCommand(ctx, events, nil, nil, make(chan error))

	require.ErrorIs(t, err, context.Canceled)
}

type failingCommandWriter struct{ err error }

func (w failingCommandWriter) Write([]byte) (int, error) { return 0, w.err }
