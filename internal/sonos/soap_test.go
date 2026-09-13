package sonos

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

// Case: the reason these are structs at all. A URI carrying an ampersand is
// ordinary for Sonos, and interpolating one into a format string yields a body
// no speaker can parse.
func TestSOAP_EscapesAValueThatWouldBreakTheBody(t *testing.T) {
	const uri = "x-sonos-http:librarytrack%3ai.8Wdz31psr2P52.mp4?sid=204&flags=8232&sn=2"

	body, err := marshalRequest(browseAction{
		XMLName:   xml.Name{Local: "u:Browse"},
		Namespace: contentDirectoryService,
		ObjectID:  uri,
	})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	if strings.Contains(string(body), "sid=204&flags") {
		t.Fatalf("the ampersand reached the body raw:\n%s", body)
	}
	if !strings.Contains(string(body), "&amp;") {
		t.Fatalf("the ampersand was not escaped:\n%s", body)
	}

	// And it survives the round trip, which a lost escape would not.
	var back struct {
		ObjectID string `xml:"Body>Browse>ObjectID"`
	}
	if err := xml.Unmarshal(body, &back); err != nil {
		t.Fatalf("the escaped body does not parse: %v", err)
	}
	if back.ObjectID != uri {
		t.Fatalf("round trip gave %q, want %q", back.ObjectID, uri)
	}
}

// Case: the shape UPnP devices expect. Go's own namespace syntax emits a
// default namespace rather than the u: prefix — equivalent XML that speakers
// are known to be fussy about, so the prefixes are written literally.
func TestSOAP_KeepsThePrefixedNamespaces(t *testing.T) {
	body, err := marshalRequest(instancePayload(avtransportService, "Pause"))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	for _, want := range []string{
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"`,
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"`,
		`<s:Body>`,
		`<u:Pause xmlns:u="` + avtransportService + `">`,
		`<InstanceID>0</InstanceID>`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the body is missing %q:\n%s", want, body)
		}
	}
}

// Case: every action, read back through a parser. This is what "semantically
// equivalent to the old strings" means now that the whitespace differs.
func TestSOAP_EveryActionParsesToItsOwnElement(t *testing.T) {
	for action, payload := range everyAction() {
		body, err := marshalRequest(payload)
		if err != nil {
			t.Errorf("%s: marshalling: %v", action, err)
			continue
		}

		got, err := actionElement(body)
		if err != nil {
			t.Errorf("%s: %v", action, err)
			continue
		}
		if got != action {
			t.Errorf("the body of %s holds a %q element", action, got)
		}
	}
}

// actionElement is the name of the element inside the SOAP body, which is the
// action a speaker will run. Read with a decoder because encoding/xml has no
// way to ask a struct tag for "whatever element is in there".
func actionElement(body []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(body)))

	inBody := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("no element inside the body: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if inBody {
			return start.Name.Local, nil
		}
		inBody = start.Name.Local == "Body"
	}
}

// Case: the values each action carries, which a wrong tag would silently drop.
func TestSOAP_CarriesItsArguments(t *testing.T) {
	cases := []struct {
		payload any
		want    string
	}{
		{playAction{XMLName: xml.Name{Local: "u:Play"}, Speed: 1}, "<Speed>1</Speed>"},
		{setVolumeAction{XMLName: xml.Name{Local: "u:SetVolume"}, Desired: 40}, "<DesiredVolume>40</DesiredVolume>"},
		{setRelativeVolumeAction{XMLName: xml.Name{Local: "u:SetRelativeVolume"}, Adjustment: -2}, "<Adjustment>-2</Adjustment>"},
		{setMuteAction{XMLName: xml.Name{Local: "u:SetMute"}, Desired: 1}, "<DesiredMute>1</DesiredMute>"},
		{browseAction{XMLName: xml.Name{Local: "u:Browse"}, StartingIndex: 100, RequestedCount: 50},
			"<StartingIndex>100</StartingIndex><RequestedCount>50</RequestedCount>"},
	}

	for _, c := range cases {
		body, err := marshalRequest(c.payload)
		if err != nil {
			t.Errorf("marshalling %T: %v", c.payload, err)
			continue
		}
		if !strings.Contains(string(body), c.want) {
			t.Errorf("%T does not carry %q:\n%s", c.payload, c.want, body)
		}
	}
}

// Case: every action, marshalled. encoding/xml reads XMLName only from the
// struct it is marshalling, so factoring the shared fields into an embedded
// struct either panics or silently names the element after the Go type. Both
// happened, and neither is visible without marshalling each one.
func TestSOAP_EveryActionNamesItsOwnElement(t *testing.T) {
	for want, payload := range everyAction() {
		body, err := marshalRequest(payload)
		if err != nil {
			t.Errorf("%s: %v", want, err)
			continue
		}

		got, err := actionElement(body)
		if err != nil {
			t.Errorf("%s: %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("%T marshals as %q, want %q — a promoted XMLName is ignored", payload, got, want)
		}
	}
}

// Case: the one action with no instance. ContentDirectory has nothing to
// address, unlike the two MediaRenderer services.
func TestSOAP_BrowseCarriesNoInstance(t *testing.T) {
	body, err := marshalRequest(browseAction{
		XMLName:   xml.Name{Local: "u:Browse"},
		Namespace: contentDirectoryService,
		ObjectID:  queueObjectID,
	})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	if strings.Contains(string(body), "InstanceID") {
		t.Fatalf("Browse carries an InstanceID it has no use for:\n%s", body)
	}
}

// everyAction is each SOAP action this package sends, by the element name it
// must produce. One list, so a new action is covered by every case at once.
func everyAction() map[string]any {
	return map[string]any{
		"Play":              playAction{XMLName: xml.Name{Local: "u:Play"}, Namespace: avtransportService, Speed: 1},
		"Pause":             instancePayload(avtransportService, "Pause"),
		"Stop":              instancePayload(avtransportService, "Stop"),
		"Next":              instancePayload(avtransportService, "Next"),
		"Previous":          instancePayload(avtransportService, "Previous"),
		"GetTransportInfo":  instancePayload(avtransportService, "GetTransportInfo"),
		"GetPositionInfo":   instancePayload(avtransportService, "GetPositionInfo"),
		"GetVolume":         channelAction{XMLName: xml.Name{Local: "u:GetVolume"}, Namespace: renderingService, Channel: masterChannel},
		"GetMute":           channelAction{XMLName: xml.Name{Local: "u:GetMute"}, Namespace: renderingService, Channel: masterChannel},
		"SetVolume":         setVolumeAction{XMLName: xml.Name{Local: "u:SetVolume"}, Namespace: renderingService, Channel: masterChannel, Desired: 40},
		"SetRelativeVolume": setRelativeVolumeAction{XMLName: xml.Name{Local: "u:SetRelativeVolume"}, Namespace: renderingService, Channel: masterChannel, Adjustment: -2},
		"SetMute":           setMuteAction{XMLName: xml.Name{Local: "u:SetMute"}, Namespace: renderingService, Channel: masterChannel, Desired: 1},
		"Browse":            browseAction{XMLName: xml.Name{Local: "u:Browse"}, Namespace: contentDirectoryService, ObjectID: "Q:0"},
	}
}
