package sonos

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// ContainerKind is what a music-service container holds. Sonos names it with
// eight hex digits in front of the service's own id.
type ContainerKind string

const (
	ContainerPlaylist ContainerKind = "00060000"
	ContainerAlbum    ContainerKind = "00040000"
)

// containerClasses is the upnp:class a speaker gives each kind of container.
var containerClasses = map[ContainerKind]string{
	ContainerPlaylist: "object.container.playlistContainer",
	ContainerAlbum:    "object.container.album.musicAlbum",
}

// The namespaces a DIDL-Lite document carries.
const (
	didlNamespace   = "urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"
	dcNamespace     = "http://purl.org/dc/elements/1.1/"
	upnpNamespace   = "urn:schemas-upnp-org:metadata-1-0/upnp/"
	rinconNamespace = "urn:schemas-rinconnetworks-com:metadata-1-0/"
)

// The fixed values a speaker writes in a container's DIDL.
const (
	didlNoParent      = "-1"
	didlServiceDescID = "cdudn"
)

// ServiceContainer is a playlist or album on a music service, kept as the few
// values that differ from one to the next. The URI and the DIDL a speaker
// needs to queue it are built from these.
type ServiceContainer struct {
	Kind    ContainerKind
	ID      string // the service's own id, e.g. "libraryplaylist:p.V7VYpk7FYWLeW"
	Title   string // what the speaker reports back as the queue's name
	Service int    // sid: 204 is Apple Music
	Account int    // sn: which account on the service, as this household numbers them
}

type (
	didlLite struct {
		XMLName   xml.Name `xml:"DIDL-Lite"`
		DC        string   `xml:"xmlns:dc,attr"`
		UPnP      string   `xml:"xmlns:upnp,attr"`
		R         string   `xml:"xmlns:r,attr"`
		Namespace string   `xml:"xmlns,attr"`
		Item      didlItem `xml:"item"`
	}

	didlItem struct {
		ID         string   `xml:"id,attr"`
		ParentID   string   `xml:"parentID,attr"`
		Restricted bool     `xml:"restricted,attr"`
		Title      string   `xml:"dc:title"`
		Class      string   `xml:"upnp:class"`
		Desc       didlDesc `xml:"desc"`
	}

	// didlDesc tells a speaker which service an item belongs to.
	didlDesc struct {
		ID        string `xml:"id,attr"`
		NameSpace string `xml:"nameSpace,attr"`
		Value     string `xml:",chardata"`
	}
)

// URI is what AddURIToQueue takes to queue the container.
func (c ServiceContainer) URI() string {
	return fmt.Sprintf("x-rincon-cpcontainer:%s?sid=%d&flags=0&sn=%d", c.itemID(), c.Service, c.Account)
}

// Metadata is the DIDL that goes with URI. It leaves out the artwork, which
// nothing here shows and which some services sign to expire within a day.
func (c ServiceContainer) Metadata() (string, error) {
	class, ok := containerClasses[c.Kind]
	if !ok {
		return "", fmt.Errorf("describing %q: unknown container kind %q", c.Title, c.Kind)
	}

	out, err := xml.Marshal(didlLite{
		DC:        dcNamespace,
		UPnP:      upnpNamespace,
		R:         rinconNamespace,
		Namespace: didlNamespace,
		Item: didlItem{
			ID:         c.itemID(),
			ParentID:   didlNoParent,
			Restricted: true,
			Title:      c.Title,
			Class:      class,
			Desc: didlDesc{
				ID:        didlServiceDescID,
				NameSpace: rinconNamespace,
				Value:     c.serviceToken(),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describing %q: %w", c.Title, err)
	}
	return string(out), nil
}

// itemID is the kind's prefix and the escaped id, written with lowercase hex
// as a speaker writes it: "00060000libraryplaylist%3ap.V7VYpk7FYWLeW".
func (c ServiceContainer) itemID() string {
	escaped := url.QueryEscape(c.ID)
	var b strings.Builder
	for i := 0; i < len(escaped); i++ {
		b.WriteByte(escaped[i])
		if escaped[i] == '%' && i+2 < len(escaped) {
			b.WriteString(strings.ToLower(escaped[i+1 : i+3]))
			i += 2
		}
	}
	return string(c.Kind) + b.String()
}

// serviceToken names the service. The service type is sid*256+7, as observed
// for Apple Music; other services are untested. The account is in the URI.
func (c ServiceContainer) serviceToken() string {
	serviceType := c.Service<<8 + 7
	return fmt.Sprintf("SA_RINCON%d_X_#Svc%d-0-Token", serviceType, serviceType)
}
