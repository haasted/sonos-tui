package sonoscloud

import (
	"strconv"
	"sync"
	"time"
)

// pageLife is how long an opened artist or album is given again without a
// request. What an artist or an album holds rarely changes within a sitting.
const pageLife = 30 * time.Minute

// cached is an answer kept, and when it came.
type cached[T any] struct {
	value T
	at    time.Time
}

// pageCache keeps opened pages in memory for pageLife, by household, service
// and id. Only answers are kept: a failure is asked again. The zero value is
// ready to use.
type pageCache[T any] struct {
	pages map[string]cached[T]

	mu sync.Mutex // guards pages
}

// get is the page kept for key, if it is younger than pageLife at now. An
// older one is dropped.
func (p *pageCache[T]) get(key string, now time.Time) (T, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	page, ok := p.pages[key]
	if !ok || now.Sub(page.at) >= pageLife {
		delete(p.pages, key)
		var zero T
		return zero, false
	}
	return page.value, true
}

// put keeps value for key, as it was at now.
func (p *pageCache[T]) put(key string, value T, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pages == nil {
		p.pages = map[string]cached[T]{}
	}
	p.pages[key] = cached[T]{value: value, at: now}
}

// clear drops every page kept.
func (p *pageCache[T]) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.pages)
}

// pageKey names a page of a household's service.
func pageKey(household string, service int, objectID string) string {
	return household + "/" + strconv.Itoa(service) + "/" + objectID
}
