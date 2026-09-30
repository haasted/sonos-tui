package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: the steps this build ships. They must apply to an empty file, which
// is the only guarantee that a first run and a later one agree.
func TestMigrate_TheShippedStepsApplyToAnEmptyFile(t *testing.T) {
	db := rawDB(t)

	require.NoError(t, migrate(t.Context(), db))

	assert.Equal(t, len(migrations), versionOf(t, db))
}

// Case: opening a file that is already current. Nothing may run a second
// time -- the steps are not written to survive it, and a re-run would be a
// CREATE TABLE against a table that exists.
func TestMigrate_AStepAlreadyAppliedIsNotRepeated(t *testing.T) {
	db := rawDB(t)
	require.NoError(t, migrate(t.Context(), db))

	require.NoError(t, migrate(t.Context(), db))

	assert.Equal(t, len(migrations), versionOf(t, db))
}

// Case: a file left part way, which is what an older build's database is.
// The version says where to start, so only the steps it has not had run.
func TestMigrate_APartMigratedFileIsFinished(t *testing.T) {
	restoreMigrationsAfterTest(t)

	db := rawDB(t)
	steps := []string{`CREATE TABLE first (a TEXT) STRICT;`, `CREATE TABLE second (b TEXT) STRICT;`}

	migrations = steps[:1]
	require.NoError(t, migrate(t.Context(), db))
	require.Equal(t, 1, versionOf(t, db))

	migrations = steps
	require.NoError(t, migrate(t.Context(), db))

	assert.Equal(t, 2, versionOf(t, db))
	assert.True(t, hasTable(t, db, "second"))
}

// Case: an older binary meeting a file a newer one wrote. It knows nothing
// about the columns that file has, so it must refuse rather than run queries
// against a schema it cannot describe.
func TestMigrate_AFileFromANewerBuildIsRefused(t *testing.T) {
	restoreMigrationsAfterTest(t)
	migrations = []string{`CREATE TABLE a (x TEXT) STRICT;`}

	db := rawDB(t)
	require.NoError(t, migrate(t.Context(), db))

	migrations = nil
	err := migrate(t.Context(), db)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "this build knows")
}

// Case: a step that fails half way. The schema and the version move together
// in one transaction, so the file must come back to the version it had and
// keep nothing the failed step created.
func TestMigrate_AFailedStepLeavesNoTrace(t *testing.T) {
	restoreMigrationsAfterTest(t)

	db := rawDB(t)
	migrations = []string{
		`CREATE TABLE good (a TEXT) STRICT;`,
		`CREATE TABLE half (b TEXT) STRICT; THIS IS NOT SQL;`,
	}

	err := migrate(t.Context(), db)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration 2")
	assert.Equal(t, 1, versionOf(t, db), "the version must not claim a step that rolled back")
	assert.False(t, hasTable(t, db, "half"), "a rolled-back step must leave no table behind")
	assert.True(t, hasTable(t, db, "good"), "the step that did succeed must stand")
}

// Enables a test to overwrite the package global migrations array.
func restoreMigrationsAfterTest(t *testing.T) {
	t.Helper()
	originalMigrations := migrations

	t.Cleanup(func() {
		migrations = originalMigrations
	})
}

// rawDB is an open database with no migration applied, for exercising the
// migration itself rather than what Open makes of it.
func rawDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open(driverName, dsn(filepath.Join(t.TempDir(), "sonos.db")))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, db.PingContext(t.Context()))
	return db
}

func versionOf(t *testing.T, db *sql.DB) int {
	t.Helper()

	version, err := readVersion(t.Context(), db)
	require.NoError(t, err)
	return version
}

func hasTable(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count))
	return count == 1
}
