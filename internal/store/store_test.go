package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a first run. The file does not exist, nor does the directory holding
// it, and both must appear without the caller arranging anything.
func TestOpen_AnAbsentFileIsCreatedAndMigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh", "sonos.db")

	s, err := Open(t.Context(), path)
	require.NoError(t, err)
	defer s.Close()

	assert.FileExists(t, path)

	version, err := s.Version(t.Context())
	require.NoError(t, err)
	assert.Equal(t, len(migrations), version, "a new file must land on the schema this build knows")
}

// Case: the second run. The file is already at the current schema, and
// opening it must be the same act as creating one -- same path, same result.
func TestOpen_AnExistingFileKeepsWhatItHolds(t *testing.T) {
	require := require.New(t)
	path := filepath.Join(t.TempDir(), "sonos.db")

	first, err := Open(t.Context(), path)
	require.NoError(err)
	_, err = first.db.ExecContext(t.Context(),
		`INSERT INTO speaker_cache VALUES ('uuid:A','192.168.1.10','http://x','uuid:A','Kitchen','Play:1','S1','S1')`)
	require.NoError(err)
	require.NoError(first.Close())

	second, err := Open(t.Context(), path)
	require.NoError(err)
	defer second.Close()

	version, err := second.Version(t.Context())
	require.NoError(err)
	require.Equal(len(migrations), version)
	require.Equal(1, countIn(t, second, "speaker_cache"), "reopening must not empty the file")
}

// Case: the path the program actually uses. "Application Support" holds a
// space, which is not legal in a URI and would reach SQLite as a truncated
// name if the DSN were pasted together rather than escaped.
func TestOpen_APathWithASpaceOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Application Support", "sonos-tui", "sonos.db")

	s, err := Open(t.Context(), path)
	require.NoError(t, err)
	defer s.Close()

	assert.FileExists(t, path, "the database must be at the path asked for, not at a prefix of it")
}

// Case: a relative path, as -db takes from the command line. It opens in
// the working directory, rather than reading the first part as a host.
func TestOpen_ARelativePathOpens(t *testing.T) {
	t.Chdir(t.TempDir())

	s, err := Open(t.Context(), filepath.Join("demos", "bin", "sonos.db"))
	require.NoError(t, err)
	defer s.Close()

	assert.FileExists(t, filepath.Join("demos", "bin", "sonos.db"))
}

// Case: the pragmas, on a connection the pool opened later. They are set
// through the DSN for exactly this reason -- an Exec would have reached the
// first connection only.
func TestOpen_EveryConnectionCarriesThePragmas(t *testing.T) {
	require := require.New(t)
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "sonos.db"))
	require.NoError(err)
	defer s.Close()

	// Hold one connection open so the second query cannot be served by it.
	held, err := s.db.Conn(t.Context())
	require.NoError(err)
	defer held.Close()

	later, err := s.db.Conn(t.Context())
	require.NoError(err)
	defer later.Close()

	var journal, foreignKeys, busy string
	require.NoError(later.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journal))
	require.NoError(later.QueryRowContext(t.Context(), "PRAGMA foreign_keys").Scan(&foreignKeys))
	require.NoError(later.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&busy))

	require.Equal("wal", strings.ToLower(journal), "readers must not block the writer")
	require.Equal("1", foreignKeys, "SQLite leaves these off unless asked")
	require.Equal("5000", busy, "a held write lock must be waited for, not failed on")
}

// Case: where the file goes when nothing says otherwise. It must be under the
// config directory, which survives, rather than the cache directory, which
// the system may reclaim.
func TestDefaultPath_SitsUnderTheConfigDirectory(t *testing.T) {
	config, err := os.UserConfigDir()
	require.NoError(t, err)

	path, err := DefaultPath()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(config, dirName, fileName), path)
}

// countIn is how many rows a table holds.
func countIn(t *testing.T, s *Store, table string) int {
	t.Helper()

	var count int
	require.NoError(t, s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count))
	return count
}
