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

func TestGuestExecutionPreservesInputFailure(t *testing.T) {
	t.Parallel()
	input := make(chan error, 1)
	failure := errors.New("input pipe closed")
	input <- failure
	events := commandEvents(m.ExecEvent{Kind: m.ExecEventExited}, m.ExecEvent{Kind: m.ExecEventDone})

	err := receiveCommand(t.Context(), events, nil, nil, input)

	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "write guest stdin: input pipe closed")
}

func TestGuestExecutionRetainsStructuredFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		kind m.ExecEventKind
	}{{"start", m.ExecEventFailed}, {"stdin", m.ExecEventStdinError}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errno := 2
			failure := m.ExecFailure{Kind: "io", Errno: &errno, ErrnoName: "ENOENT", Message: "executable unavailable", Path: "/bin/missing"}
			var output bytes.Buffer
			events := commandEvents(
				m.ExecEvent{Kind: tc.kind, Failure: &failure},
				m.ExecEvent{Kind: m.ExecEventStdout, Data: []byte("last output")},
				m.ExecEvent{Kind: m.ExecEventExited},
				m.ExecEvent{Kind: m.ExecEventDone},
			)

			err := receiveCommand(t.Context(), events, &output, nil, nil)

			var detail *guestExecFailure
			require.ErrorAs(t, err, &detail)
			require.Equal(t, failure, detail.ExecFailure)
			for _, message := range []string{failure.Message, failure.Kind, failure.Path, failure.ErrnoName, "errno=2"} {
				require.ErrorContains(t, err, message)
			}
			if tc.kind == m.ExecEventStdinError {
				require.ErrorContains(t, err, "write guest stdin")
				require.Equal(t, "last output", output.String(), "stdin failure does not terminate the stream")
			} else {
				require.ErrorContains(t, err, "start guest command")
			}
		})
	}
}

func TestGuestExecutionHandlesFailureWithoutDetails(t *testing.T) {
	t.Parallel()
	events := commandEvents(m.ExecEvent{Kind: m.ExecEventFailed})
	err := receiveCommand(t.Context(), events, nil, nil, nil)
	require.ErrorContains(t, err, "start guest command failed")
}

func TestGuestExecutionRetainsInputAndExitFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("input pipe failed")
	input := make(chan error, 1)
	input <- failure
	events := commandEvents(m.ExecEvent{Kind: m.ExecEventExited, ExitCode: 7}, m.ExecEvent{Kind: m.ExecEventDone})

	err := receiveCommand(t.Context(), events, nil, nil, input)

	require.ErrorIs(t, err, failure)
	var exit *guestExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 7, exit.code)
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
