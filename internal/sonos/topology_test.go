package sonos_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a household where every speaker is on its own, which is the resting
// state and the one a reader sees most.
func TestTopology_ReadsAHouseholdOfStandaloneSpeakers(t *testing.T) {
	topology := topologyFrom(t, "zonegroupstate_standalone.xml")

	require.Len(t, topology.Groups, 4)
	for _, group := range topology.Groups {
		require.Len(t, group.Members, 1, "a standalone speaker is a group of one")
		assert.Equal(t, group.Coordinator, group.Members[0].ID,
			"a speaker alone coordinates itself")
		assert.False(t, group.Grouped())
	}
}

// Case: two rooms playing together, which is the whole point of the service.
// The queue belongs to the coordinator, so which member that is matters more
// than anything else the document says.
func TestTopology_ReadsAGroupAndItsCoordinator(t *testing.T) {
	topology := topologyFrom(t, "zonegroupstate_grouped.xml")

	group, found := topology.GroupOf(kitchen)
	require.True(t, found)

	assert.True(t, group.Grouped())
	assert.Equal(t, upstairs, group.Coordinator,
		"the kitchen joined upstairs, so upstairs owns the queue")
	assert.Len(t, group.Members, 2)
}

// Case: the same speaker read both ways. Alone it coordinates itself; in a
// group it answers with whoever does, which is what a command has to be sent
// to rather than the speaker the reader is looking at.
func TestTopology_CoordinatorFollowsTheGrouping(t *testing.T) {
	alone, _ := topologyFrom(t, "zonegroupstate_standalone.xml").CoordinatorOf(kitchen)
	assert.Equal(t, kitchen, alone)

	joined, _ := topologyFrom(t, "zonegroupstate_grouped.xml").CoordinatorOf(kitchen)
	assert.Equal(t, upstairs, joined)
}

// Case: a speaker this household has never heard of. Discovery crosses Sonos
// systems and a topology does not, so an unknown id is an ordinary answer
// rather than a fault.
func TestTopology_AnswersNothingForASpeakerItDoesNotHold(t *testing.T) {
	topology := topologyFrom(t, "zonegroupstate_standalone.xml")

	_, found := topology.GroupOf("RINCON_NOTINTHISHOUSEHOLD")
	assert.False(t, found)

	_, found = topology.CoordinatorOf("RINCON_NOTINTHISHOUSEHOLD")
	assert.False(t, found)
}

// Case: the members themselves, which have to carry enough to be matched to
// a discovered speaker later -- the id to join on, and the room to show.
func TestTopology_MembersCarryTheirIdentity(t *testing.T) {
	group, found := topologyFrom(t, "zonegroupstate_grouped.xml").GroupOf(kitchen)
	require.True(t, found)

	var member sonos.ZoneMember
	for _, m := range group.Members {
		if m.ID == kitchen {
			member = m
		}
	}

	assert.Equal(t, "Køkken", member.RoomName)
	assert.Contains(t, member.Location, "192.168.68.51", "the location carries its address")
}

// Case: the group's own id, which nothing keys on but which cannot be worked
// out again once dropped. It names its coordinator, which is why the
// coordinator is the identity and this is not.
func TestTopology_KeepsTheGroupsOwnID(t *testing.T) {
	group, _ := topologyFrom(t, "zonegroupstate_grouped.xml").GroupOf(kitchen)

	assert.Equal(t, upstairs+":3844401901", group.ID)
}

// Case: the whole path, from the request to the parsed answer.
func TestTopology_ReadsItFromASpeaker(t *testing.T) {
	var path, body string
	state := readFile(t, "zonegroupstate_grouped.xml")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		path, body = r.URL.Path, string(raw)

		io.WriteString(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><u:GetZoneGroupStateResponse xmlns:u="`+
			`urn:schemas-upnp-org:service:ZoneGroupTopology:1"><ZoneGroupState>`)
		xmlEscape(w, state)
		io.WriteString(w, `</ZoneGroupState></u:GetZoneGroupStateResponse></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	topology, err := clientFor(server).GetZoneGroupState(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "/ZoneGroupTopology/Control", path,
		"the topology does not live under MediaRenderer, unlike the rest")
	assert.Contains(t, body, "u:GetZoneGroupState")
	assert.Len(t, topology.Groups, 3)
}

// Case: a speaker that answers with nothing. Better an error than a household
// that looks as though every speaker has gone.
func TestTopology_RefusesAnEmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><u:GetZoneGroupStateResponse/></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	_, err := clientFor(server).GetZoneGroupState(context.Background())
	assert.Error(t, err)
}

// The two speakers the captures were taken from.
const (
	kitchen  = "RINCON_804AF293EC4C01400"
	upstairs = "RINCON_74CA6032755801400"
)

func topologyFrom(t *testing.T, name string) sonos.Topology {
	t.Helper()

	topology, err := sonos.ParseZoneGroupState(readFile(t, name))
	require.NoError(t, err)
	return topology
}

func readFile(t *testing.T, name string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(body)
}

// xmlEscape writes the document as the service does: escaped, inside the
// response element.
func xmlEscape(w io.Writer, s string) {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(s))
	_, _ = w.Write(escaped.Bytes())
}

// Case: the bare id, which is what the topology calls a speaker and what
// discovery wraps in a USN.
func TestSpeakerID_UnwrapsTheUSN(t *testing.T) {
	for usn, want := range map[string]string{
		"uuid:RINCON_804AF293EC4C01400::urn:schemas-upnp-org:device:ZonePlayer:1": "RINCON_804AF293EC4C01400",
		"uuid:RINCON_804AF293EC4C01400":                                           "RINCON_804AF293EC4C01400",
		"RINCON_804AF293EC4C01400":                                                "RINCON_804AF293EC4C01400",
		"":                                                                        "",
	} {
		assert.Equal(t, want, sonos.Speaker{USN: usn}.ID(), "from %q", usn)
	}
}

// Case: the id joining a discovered speaker to the topology, which is the
// whole reason it is unwrapped.
func TestSpeakerID_MatchesTheTopology(t *testing.T) {
	speaker := sonos.Speaker{USN: "uuid:" + kitchen + "::urn:schemas-upnp-org:device:ZonePlayer:1"}

	_, found := topologyFrom(t, "zonegroupstate_grouped.xml").GroupOf(speaker.ID())
	assert.True(t, found, "a discovered speaker must find itself in the topology")
}
