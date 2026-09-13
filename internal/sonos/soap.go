package sonos

import (
	"encoding/xml"
	"fmt"
)

// The namespaces a SOAP envelope carries. Written as literal prefixes rather
// than through Go's namespace syntax, which emits a default namespace instead
// of the s: prefix — equivalent XML, but not the shape UPnP devices expect.
const (
	soapNamespace     = "http://schemas.xmlsoap.org/soap/envelope/"
	soapEncodingStyle = "http://schemas.xmlsoap.org/soap/encoding/"
)

// envelope is a SOAP request, with the action as its body. The body is an
// interface so the marshaller walks into whichever action it holds, and escapes
// that action's values on the way.
type envelope struct {
	XMLName       xml.Name `xml:"s:Envelope"`
	Namespace     string   `xml:"xmlns:s,attr"`
	EncodingStyle string   `xml:"s:encodingStyle,attr"`
	Body          soapBody `xml:"s:Body"`
}

type soapBody struct {
	Action any
}

// instanceAction is every action whose only argument is the instance. Each has
// its own element name, so the name travels as an attribute-free XMLName the
// caller fills in.
type instanceAction struct {
	XMLName    xml.Name
	Namespace  string `xml:"xmlns:u,attr"`
	InstanceID int    `xml:"InstanceID"`
}

// instancePayload builds the body for an action that takes nothing but the
// instance, such as Pause or GetPositionInfo.
func instancePayload(service, action string) instanceAction {
	return instanceAction{
		XMLName:   xml.Name{Local: "u:" + action},
		Namespace: service,
	}
}

// marshalRequest renders one action inside a SOAP envelope. Values are escaped
// by the marshaller, which is the whole reason these are structs: a URI holding
// an ampersand would otherwise produce a body no speaker can parse.
func marshalRequest(action any) ([]byte, error) {
	out, err := xml.Marshal(envelope{
		Namespace:     soapNamespace,
		EncodingStyle: soapEncodingStyle,
		Body:          soapBody{Action: action},
	})
	if err != nil {
		return nil, fmt.Errorf("building request body: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

// The actions that carry an argument. Each writes out its own XMLName rather
// than embedding a shared struct: encoding/xml reads XMLName only from the
// struct it is marshalling, so a promoted one is ignored and the element takes
// the Go type name instead.
type (
	playAction struct {
		XMLName    xml.Name
		Namespace  string `xml:"xmlns:u,attr"`
		InstanceID int    `xml:"InstanceID"`
		Speed      int    `xml:"Speed"`
	}

	channelAction struct {
		XMLName    xml.Name
		Namespace  string `xml:"xmlns:u,attr"`
		InstanceID int    `xml:"InstanceID"`
		Channel    string `xml:"Channel"`
	}

	setVolumeAction struct {
		XMLName    xml.Name
		Namespace  string `xml:"xmlns:u,attr"`
		InstanceID int    `xml:"InstanceID"`
		Channel    string `xml:"Channel"`
		Desired    int    `xml:"DesiredVolume"`
	}

	setRelativeVolumeAction struct {
		XMLName    xml.Name
		Namespace  string `xml:"xmlns:u,attr"`
		InstanceID int    `xml:"InstanceID"`
		Channel    string `xml:"Channel"`
		Adjustment int    `xml:"Adjustment"`
	}

	setMuteAction struct {
		XMLName    xml.Name
		Namespace  string `xml:"xmlns:u,attr"`
		InstanceID int    `xml:"InstanceID"`
		Channel    string `xml:"Channel"`
		Desired    int    `xml:"DesiredMute"`
	}

	// browseAction carries no InstanceID. ContentDirectory has no instance to
	// address, unlike the two MediaRenderer services.
	browseAction struct {
		XMLName        xml.Name
		Namespace      string `xml:"xmlns:u,attr"`
		ObjectID       string `xml:"ObjectID"`
		BrowseFlag     string `xml:"BrowseFlag"`
		Filter         string `xml:"Filter"`
		StartingIndex  int    `xml:"StartingIndex"`
		RequestedCount int    `xml:"RequestedCount"`
		SortCriteria   string `xml:"SortCriteria"`
	}
)
