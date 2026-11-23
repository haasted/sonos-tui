package localmodel_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/localmodel"
)

// yesNoSchema asks for one word, yes or no.
const yesNoSchema = `{"title":"Answer","type":"object","additionalProperties":false,"x-order":["word"],` +
	`"required":["word"],"properties":{"word":{"type":"string","enum":["yes","no"]}}}`

// Case: fm where there is none. It is not installed, said without running.
func TestAvailable_NoFm(t *testing.T) {
	err := localmodel.AvailableAt(t.Context(), filepath.Join(t.TempDir(), "fm"))

	assert.ErrorIs(t, err, localmodel.ErrNotInstalled)
	var unavailable *localmodel.UnavailableError
	assert.True(t, errors.As(err, &unavailable))
}

// Case: a question with a schema. The answer is decoded into the type.
func TestRespond_DecodesIntoTheType(t *testing.T) {
	requireFm(t)

	answer, err := localmodel.Respond[struct{ Word string }](t.Context(),
		"Answer the question.", yesNoSchema, "Is water wet?")

	require.NoError(t, err)
	assert.Contains(t, []string{"yes", "no"}, answer.Word)
}

// Case: an answer with a field the type does not have. It does not fit, so
// it is an error, not a half-filled value, and the log keeps what fm said.
func TestRespond_AnAnswerThatDoesNotFitIsAnError(t *testing.T) {
	requireFm(t)
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	_, err := localmodel.Respond[struct{ Other string }](t.Context(),
		"Answer the question.", yesNoSchema, "Is water wet?")

	assert.ErrorContains(t, err, "does not fit")
	assert.Contains(t, log.String(), `word`, "the log does not hold the reply")
}

// Case: a schema without fm's title and x-order. fm refuses it, and the
// error says what fm said.
func TestRespond_ARefusedSchemaIsARunError(t *testing.T) {
	requireFm(t)

	_, err := localmodel.Respond[struct{ Word string }](t.Context(),
		"Answer the question.", `{"type":"object"}`, "Is water wet?")

	var run *localmodel.RunError
	require.True(t, errors.As(err, &run), "got %v", err)
	assert.Equal(t, "respond", run.Command)
	assert.Contains(t, run.Stderr, "Invalid schema")
}

// Case: a deadline that passes before fm answers. The run stops and says
// why.
func TestRespond_ADeadlineStopsIt(t *testing.T) {
	requireFm(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := localmodel.Respond[struct{ Word string }](ctx,
		"Answer the question.", yesNoSchema, "Is water wet?")

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

// requireFm skips the test where fm cannot answer.
func requireFm(t *testing.T) {
	t.Helper()
	if err := localmodel.Available(t.Context()); err != nil {
		t.Skipf("fm cannot be used here: %v", err)
	}
}
