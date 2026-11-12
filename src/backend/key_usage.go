package main

import (
	"sort"
	"time"

	"github.com/sasha-s/go-deadlock"
)

// How API keys are used: each key's requests are counted in memory and
// saved with the key's last use, which touchAPIKey writes at most once a
// minute, and by flushAPIKeyUses when wakeci stops, so counting costs no
// database write per request. A crash loses at most the last minute's
// counts of the keys in use.

// usageDays is how many days of requests a key keeps, today included
const usageDays = 30

// DayUses is how many requests a key made on a day, in the server's time
// zone
type DayUses struct {
	// Day is the date, such as 2026-11-12
	Day  string `json:"day"`
	Uses int64  `json:"uses"`
}

// keyUse is what a key did since its use was last saved
type keyUse struct {
	count int64
	last  time.Time
	days  map[string]int64
}

type apiKeyUsage struct {
	mu      deadlock.Mutex
	pending map[string]keyUse
}

// apiKeyUses holds the uses of every key not saved yet
var apiKeyUses = newAPIKeyUsage()

func newAPIKeyUsage() *apiKeyUsage {
	return &apiKeyUsage{pending: map[string]keyUse{}}
}

// usageDay names the day of t in the server's time zone
func usageDay(t time.Time) string {
	return t.Local().Format(time.DateOnly)
}

// add counts a request of key id at now
func (u *apiKeyUsage) add(id string, now time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	use := u.pending[id]
	use.count++
	if now.After(use.last) {
		use.last = now
	}
	if use.days == nil {
		use.days = map[string]int64{}
	}
	use.days[usageDay(now)]++
	u.pending[id] = use
}

// take removes and returns the uses of key id not saved yet
func (u *apiKeyUsage) take(id string) keyUse {
	u.mu.Lock()
	defer u.mu.Unlock()
	use := u.pending[id]
	delete(u.pending, id)
	return use
}

// takeAll removes and returns the uses of every key not saved yet
func (u *apiKeyUsage) takeAll() map[string]keyUse {
	u.mu.Lock()
	defer u.mu.Unlock()
	pending := u.pending
	u.pending = map[string]keyUse{}
	return pending
}

// peek returns the uses of key id not saved yet, keeping them
func (u *apiKeyUsage) peek(id string) keyUse {
	u.mu.Lock()
	defer u.mu.Unlock()
	use := u.pending[id]
	days := make(map[string]int64, len(use.days))
	for day, n := range use.days {
		days[day] = n
	}
	use.days = days
	return use
}

// putBack returns uses that couldn't be saved, to be saved later
func (u *apiKeyUsage) putBack(id string, use keyUse) {
	if use.count == 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	pending := u.pending[id]
	pending.count += use.count
	if use.last.After(pending.last) {
		pending.last = use.last
	}
	if pending.days == nil {
		pending.days = map[string]int64{}
	}
	for day, n := range use.days {
		pending.days[day] += n
	}
	u.pending[id] = pending
}

// addTo adds the uses to key: its count, last use and days
func (use keyUse) addTo(key *APIKey) {
	if use.count == 0 {
		return
	}
	key.Uses += use.count
	if key.LastUsedAt == nil || use.last.After(*key.LastUsedAt) {
		last := use.last
		key.LastUsedAt = &last
	}
	byDay := map[string]int64{}
	for _, day := range key.UsesByDay {
		byDay[day.Day] += day.Uses
	}
	for day, n := range use.days {
		byDay[day] += n
	}
	key.UsesByDay = key.UsesByDay[:0]
	for day, n := range byDay {
		key.UsesByDay = append(key.UsesByDay, DayUses{Day: day, Uses: n})
	}
	sort.Slice(key.UsesByDay, func(i, j int) bool { return key.UsesByDay[i].Day < key.UsesByDay[j].Day })
	pruneUsage(key, use.last)
}

// pruneUsage keeps the days of the last usageDays days before now, and
// never leaves the list null
func pruneUsage(key *APIKey, now time.Time) {
	first := usageDay(now.AddDate(0, 0, -(usageDays - 1)))
	kept := []DayUses{}
	for _, day := range key.UsesByDay {
		if day.Day >= first {
			kept = append(kept, day)
		}
	}
	key.UsesByDay = kept
}
