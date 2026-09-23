// Internal, to reach picks. What an interface report says about itself is
// public; which one the program chose is not.
package sonos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Case: the shape a search can actually leave by. Everything else on a Mac is
// a tunnel, a bridge or an interface holding no address at all — there were
// twenty-one of those against one usable when this was written.
func TestInterfaceReport_UsableIsTheLANShapedOne(t *testing.T) {
	lan := InterfaceReport{Name: "en0", Up: true, Broadcast: true, Multicast: true,
		IPv4: []string{"192.168.68.67/22"}}
	assert.True(t, lan.Usable())

	for name, iface := range map[string]InterfaceReport{
		"down":         {Name: "en0", Multicast: true, IPv4: []string{"192.168.68.67/22"}},
		"no multicast": {Name: "en0", Up: true, IPv4: []string{"192.168.68.67/22"}},
		"loopback":     {Name: "lo0", Up: true, Multicast: true, Loopback: true, IPv4: []string{"127.0.0.1/8"}},
		"no address":   {Name: "utun0", Up: true, Multicast: true, PointToPoint: true},
	} {
		assert.False(t, iface.Usable(), "%s must not be treated as usable", name)
	}
}

// Case: a VPN tunnel that DOES carry an address, which is the machine this
// investigation is about. It passes every test above, so the shape is what
// tells it apart — and that is why the flags are logged.
func TestInterfaceReport_ATunnelWithAnAddressLooksUsable(t *testing.T) {
	tunnel := InterfaceReport{Name: "utun4", Up: true, Multicast: true, PointToPoint: true,
		IPv4: []string{"10.8.0.2/24"}}

	assert.True(t, tunnel.Usable(), "nothing in Usable rules a tunnel out")
	assert.Contains(t, tunnel.flags(), "point-to-point", "so the log must say what it is")
	assert.NotContains(t, tunnel.flags(), "broadcast", "and that it carries no broadcast")
}

// Case: the flags, which are the whole reason this is logged. A line that only
// said an interface was passed over would not say why.
func TestInterfaceReport_FlagsSpellTheShapeOut(t *testing.T) {
	assert.Equal(t, "up|broadcast|multicast",
		InterfaceReport{Up: true, Broadcast: true, Multicast: true}.flags())
	assert.Equal(t, "none", InterfaceReport{}.flags())
}

// Case: one address being a prefix of another. 192.168.1.1 must not claim
// 192.168.1.10's interface, which is what a plain prefix test would do.
func TestPicks_AnAddressDoesNotClaimALongerOne(t *testing.T) {
	iface := InterfaceReport{Name: "en0", IPv4: []string{"192.168.1.10/24"}}

	assert.False(t, picks(iface, "192.168.1.1"))
	assert.True(t, picks(iface, "192.168.1.10"))
}

// Case: the survey against this machine, whatever it holds. It must answer,
// and it must find the loopback, which every machine has.
func TestInterfaces_ReportsThisMachine(t *testing.T) {
	reports, err := Interfaces()
	assert.NoError(t, err)
	assert.NotEmpty(t, reports)

	var loopback bool
	for _, report := range reports {
		if report.Loopback {
			loopback = true
		}
	}
	assert.True(t, loopback, "every machine has a loopback, so finding none means the survey is broken")
}
