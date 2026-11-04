// How the program decides that a speaker is there. Three things can say so:
// the check of a cached speaker, an SSDP answer and an mDNS answer (the last
// two arrive as one stream from sonos.Discover). Whichever is first moves the
// speaker on; a cached speaker is given up only when all three have failed.
package main

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// applyFound takes a discovery answer. A new speaker is loaded. A cached one
// that has not answered yet counts as present: it loads on the Device its tab
// already has, or from the new address if it has moved.
func (m appModel) applyFound(speaker sonos.Speaker) (tea.Model, tea.Cmd) {
	id := speakerKey(speaker)
	m.found[id] = true

	if !m.hasSpeaker(id) {
		return m, loadSpeaker(m.ctx, m.eventManager, speaker)
	}
	if state, cached := m.cachedStates[id]; !cached || state == cachedLoading {
		return m, nil // already loading or loaded
	}
	m.cachedStates[id] = cachedLoading

	device, ok := m.deviceFor(id)
	if ok && device.IP == speaker.IP {
		return m, activateSpeaker(m.ctx, m.eventManager, device)
	}
	slog.Info("cached speaker answered from a new address", "id", id, "speaker", speaker)
	return m, loadSpeaker(m.ctx, m.eventManager, speaker)
}

// applyChecked takes the answer to a cached speaker's check. One that answered
// loads, unless a search found it first. One that did not stays on screen
// until the searches end, since they may still find it.
func (m appModel) applyChecked(msg cachedCheckedMsg) (tea.Model, tea.Cmd) {
	if m.cachedStates[msg.id] != cachedChecking {
		return m, nil // a search found it first, and it is loading already
	}
	if msg.err == nil {
		m.cachedStates[msg.id] = cachedLoading
		device, ok := m.deviceFor(msg.id)
		if !ok {
			return m, nil
		}
		return m, activateSpeaker(m.ctx, m.eventManager, device)
	}

	m.cachedStates[msg.id] = cachedFailed
	return m.dropUnanswered()
}

// dropUnanswered removes every cached speaker whose check failed and that no
// search found, once the searches have ended. Before then a search may still
// find it, so a failed check alone removes nothing.
func (m appModel) dropUnanswered() (tea.Model, tea.Cmd) {
	if !m.discoveryDone {
		return m, nil
	}
	var notices []tea.Cmd
	for id, state := range m.cachedStates {
		if state != cachedFailed || m.found[id] {
			continue
		}
		title := m.titleFor(id)
		slog.Info("cached speaker did not answer — dropping its tab", "id", id)
		m = m.removeSpeaker(id)
		delete(m.cachedStates, id)
		notices = append(notices, notify(panes.NoticeWarn, title+" did not answer — removed"))
	}
	return m, tea.Batch(notices...)
}

// placeSpeaker puts a loaded speaker on screen: a new tab, or in place of the
// tab it already has when a cached speaker answered from a new address. It
// reports whether it replaced one.
func (m appModel) placeSpeaker(msg speakerReadyMsg) (appModel, bool) {
	if idx := m.tabIndex(msg.id); idx >= 0 {
		m.tabs.Items[idx].Content = msg.speaker
		m.tabs.Items[idx].Title = msg.title
		return m, true
	}
	return m.insertSpeaker(msg.id, msg.title, msg.speaker), false
}
