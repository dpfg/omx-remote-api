package main

import (
	"testing"
)

func TestPList_NewEmpty(t *testing.T) {
	plist := NewPlayList(make([]MediaEntry, 0))
	if plist.AutoPlay != true {
		t.Error("New play list without autoplay")
	}

	if plist.CurrentIndex != positionNone {
		t.Error("Not undefined position")
	}

	if len(plist.Entries) != 0 {
		t.Error("Not empty list")
	}
}

func TestPList_NewNonEmpty(t *testing.T) {
	plist := NewPlayList([]MediaEntry{
		MediaEntry{RawURL: "http://example/1"},
		MediaEntry{RawURL: "http://example/2"},
	})

	if plist.AutoPlay != true {
		t.Error("New play list without autoplay")
	}

	if plist.CurrentIndex != positionNone {
		t.Error("Not undefined position")
	}

	if len(plist.Entries) != 2 {
		t.Error("Size doesn't match")
	}
}

func TestPList_Next(t *testing.T) {
	plist := NewPlayList([]MediaEntry{
		MediaEntry{RawURL: "http://example/1"},
	})
	entry := plist.Next()

	if entry.RawURL != "http://example/1" {
		t.Error("Wrong next entry")
	}

	if plist.CurrentIndex != 0 {
		t.Error("Incorrect current index")
	}
}

func TestPList_OverNext(t *testing.T) {
	plist := NewPlayList([]MediaEntry{
		MediaEntry{RawURL: "http://example/1"},
	})
	plist.Next()
	entry := plist.Next()

	if plist.CurrentIndex != positionNone {
		t.Error("Position was not reset")
	}

	if entry != nil {
		t.Error("Entry is not nil")
	}
}

func TestPList_Select(t *testing.T) {
	plist := NewPlayList([]MediaEntry{
		MediaEntry{RawURL: "http://example/1"},
		MediaEntry{RawURL: "http://example/2"},
		MediaEntry{RawURL: "http://example/3"},
	})
	selectedIndex := 2
	entry := plist.Select(selectedIndex)

	if entry.RawURL != "http://example/3" {
		t.Error("Wrong enrty was selected")
	}

	if plist.CurrentIndex != selectedIndex {
		t.Errorf("Unexpected current index: %d", plist.CurrentIndex)
	}
}

func TestPList_cleanup(t *testing.T) {
	plist := NewPlayList([]MediaEntry{
		MediaEntry{RawURL: "http://example/1"},
		MediaEntry{RawURL: "http://example/2"},
		MediaEntry{RawURL: "http://example/3"},
		MediaEntry{RawURL: "http://example/4"},
		MediaEntry{RawURL: "http://example/5"},
		MediaEntry{RawURL: "http://example/6"},
	})

	plist.Select(5)

	if len(plist.Entries) != 4 {
		t.Errorf("Unexpected size of the play list: %d", len(plist.Entries))
	}

	if plist.CurrentIndex != 3 {
		t.Errorf("Unexpected current index %d", plist.CurrentIndex)
	}
}

func TestPList_AddEntry(t *testing.T) {
	plist := NewPlayList(make([]MediaEntry, 0))
	plist.AddEntry(MediaEntry{RawURL: "http://example/1"})

	if len(plist.Entries) != 1 {
		t.Errorf("Unexpected size of play list: %d", len(plist.Entries))
	}

	url := plist.Entries[0].RawURL
	if url != "http://example/1" {
		t.Error("Unexpected url of entry: %s", url)
	}
}
