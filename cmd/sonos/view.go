package main

func (m model) View() string {
	switch {
	case len(m.tabs.Tabs) == 0 && m.discoveryDone:
		return "No Sonos speakers found on the local network."
	case len(m.tabs.Tabs) == 0:
		return "Discovering Sonos speakers..."
	default:
		return m.tabs.View()
	}
}
