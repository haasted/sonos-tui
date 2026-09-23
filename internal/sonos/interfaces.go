package sonos

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// InterfaceReport is one network interface as the code choosing between them
// sees it. The flags matter more than the address: a VPN tunnel carries an
// address like any other and is told apart by its shape.
type InterfaceReport struct {
	Name         string
	Index        int
	MTU          int
	Up           bool
	Loopback     bool
	PointToPoint bool // a tunnel, typically: a VPN, and no speakers behind it
	Broadcast    bool
	Multicast    bool // SSDP and mDNS both need this

	// IPv4 are the addresses in CIDR form, in the order the interface gives
	// them. Empty for an interface that carries none, which several do.
	IPv4 []string
}

// Usable reports an interface a search could plausibly go out of: running,
// carrying multicast, not the loopback, and holding an address to send from.
func (r InterfaceReport) Usable() bool {
	return r.Up && r.Multicast && !r.Loopback && len(r.IPv4) > 0
}

var _ slog.LogValuer = InterfaceReport{}

func (r InterfaceReport) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", r.Name),
		slog.String("flags", r.flags()),
		slog.Int("mtu", r.MTU),
		slog.String("ipv4", strings.Join(r.IPv4, " ")),
		slog.Bool("usable", r.Usable()),
	)
}

// flags spells the shape out, so a log line says why an interface was passed
// over rather than only that it was.
func (r InterfaceReport) flags() string {
	var set []string
	for _, flag := range []struct {
		on   bool
		name string
	}{
		{r.Up, "up"}, {r.Loopback, "loopback"}, {r.PointToPoint, "point-to-point"},
		{r.Broadcast, "broadcast"}, {r.Multicast, "multicast"},
	} {
		if flag.on {
			set = append(set, flag.name)
		}
	}
	if len(set) == 0 {
		return "none"
	}
	return strings.Join(set, "|")
}

// Interfaces reports every network interface and its shape. It exists for the
// question of which one a search should leave by, which is not answered by an
// address alone.
func Interfaces() ([]InterfaceReport, error) {
	found, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing network interfaces: %w", err)
	}

	reports := make([]InterfaceReport, 0, len(found))
	for _, iface := range found {
		report := InterfaceReport{
			Name:         iface.Name,
			Index:        iface.Index,
			MTU:          iface.MTU,
			Up:           iface.Flags&net.FlagUp != 0,
			Loopback:     iface.Flags&net.FlagLoopback != 0,
			PointToPoint: iface.Flags&net.FlagPointToPoint != 0,
			Broadcast:    iface.Flags&net.FlagBroadcast != 0,
			Multicast:    iface.Flags&net.FlagMulticast != 0,
		}

		// An interface that will not say is reported without addresses rather
		// than dropped: which interfaces exist is half the answer.
		addrs, err := iface.Addrs()
		if err != nil {
			slog.Debug("interface would not list its addresses", "name", iface.Name, "err", err)
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				report.IPv4 = append(report.IPv4, ipnet.String())
			}
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// LogInterfaces writes the survey out, marking the address the program would
// pick. Run once at startup: the point is to see, on a machine where discovery
// found nothing, whether it was ever looking at the right interface.
func LogInterfaces() {
	chosen := ""
	if ipnet, err := localIPNet(); err == nil {
		chosen = ipnet.IP.String()
	}

	reports, err := Interfaces()
	if err != nil {
		slog.Warn("could not survey network interfaces", "err", err)
		return
	}

	usable := 0
	for _, report := range reports {
		if report.Usable() {
			usable++
		}
	}
	slog.Info("network interfaces", "count", len(reports), "usable", usable, "chosen", chosen)

	for _, report := range reports {
		slog.Info("interface", "iface", report, "picked", picks(report, chosen))
	}

	// The address is chosen from a FLAT list that says nothing about which
	// interface each address belongs to, so the order of that list is what
	// decides. It is logged as it really arrives.
	if addrs, err := net.InterfaceAddrs(); err == nil {
		var order []string
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
				order = append(order, ipnet.IP.String())
			}
		}
		slog.Info("the order addresses are offered in, first wins", "order", strings.Join(order, " "))
	}
}

// picks reports whether this interface holds the address the program chose.
func picks(report InterfaceReport, chosen string) bool {
	for _, cidr := range report.IPv4 {
		if strings.HasPrefix(cidr, chosen+"/") {
			return true
		}
	}
	return false
}
