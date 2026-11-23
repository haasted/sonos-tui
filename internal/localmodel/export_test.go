package localmodel

import "context"

// Respond is respond on the default fm, for the engine's own tests.
func Respond[T any](ctx context.Context, instructions, schema, prompt string) (T, error) {
	return defaultFM.respond[T](ctx, instructions, schema, prompt)
}

// AvailableAt is Available for an fm at path, for one that is not there.
func AvailableAt(ctx context.Context, path string) error {
	return fm{path: path}.available(ctx)
}
