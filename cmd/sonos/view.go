package main

const helpLine = "\n\ntab/shift+tab to switch rooms · q to quit\n"

func (m model) View() string {
	switch {
	case len(m.tabs.Tabs) == 0 && m.discoveryDone:
		return "No Sonos speakers found on the local network." + helpLine
	case len(m.tabs.Tabs) == 0:
		return "Discovering Sonos speakers..." + helpLine
	default:
		return m.tabs.View() + helpLine
	}
}
