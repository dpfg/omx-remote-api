package main

const (
	// defines number of item to preserve in the playlist
	maxHistorySize = 3

	// indicates that nothing is playing
	positionNone = -1
)

// PList holds the list of media items with pointer to the playing one
type PList struct {
	CurrentIndex int          `json:"current_index"`
	Entries      []MediaEntry `json:"entries,omitempty"`
	AutoPlay     bool         `json:"auto_play,omitempty"`
}

// Next moves pointer of a current element to the next element in the list and
// returns the media entry
func (pl *PList) Next() *MediaEntry {
	nextIndex := pl.CurrentIndex + 1

	length := len(pl.Entries)
	if length < nextIndex+1 {
		pl.CurrentIndex = positionNone
		return nil
	}

	return pl.Select(nextIndex)
}

// Select move pointer to a current element to the specific element refered by its index
// and return the media entry
func (pl *PList) Select(position int) *MediaEntry {
	plistSize := len(pl.Entries)
	if plistSize == 0 {
		return nil
	}

	if position < 0 || plistSize < position {
		return nil
	}

	pl.CurrentIndex = position

	entry := &pl.Entries[position]

	// pl.cleanUpHistory()

	return entry
}

// AddEntry adds a new media entry to the end of the playlist.
func (pl *PList) AddEntry(entry MediaEntry) int {
	pl.Entries = append(pl.Entries, entry)
	return len(pl.Entries) - 1
}

func (pl *PList) cleanUpHistory() {
	if pl.CurrentIndex == positionNone {
		return
	}

	historySize := pl.CurrentIndex + 1
	if historySize < maxHistorySize {
		return
	}

	dropIndex := historySize - maxHistorySize - 1
	if dropIndex < 0 {
		dropIndex = 0
	}

	pl.Entries = pl.Entries[dropIndex:]
	pl.CurrentIndex = pl.CurrentIndex - dropIndex

}

// NewPlayList creates new play list with default settings
func NewPlayList(entries []MediaEntry) *PList {
	return &PList{CurrentIndex: positionNone, AutoPlay: true, Entries: entries}
}
