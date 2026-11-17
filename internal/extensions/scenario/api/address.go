// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// An element's address (the study gui.element-addresses §3) is
// `<universe>/<place>/<path>`: `dev/object//links`
// `archi/screen/marketplace/sidecars`. A scenario step `open:` names one, and
// `scenario gui <case> --open <address>` starts the case's GUI on it. The
// grammar is the GUI's (app/ktw-gui/frontend/src/address.ts, parseAddress):
// both read the same examples (app/ktw-gui/frontend/tests/fixtures/addresses.json).

// AddressLink is the prefix of a link from outside the GUI.
const AddressLink = "katyoti://open/"

// AddressScreens are the screens an address opens.
var AddressScreens = []string{"inbox", "marketplace", "debug", "guide", "settings", "ready"}

// addressRail are the rail entries an address can name, each opening a screen.
var addressRail = map[string]string{"inbox": "inbox", "marketplace": "marketplace", "debug": "debug", "settings": "settings"}

// addressPlaces are an address's places, its second segment.
var addressPlaces = []string{"object", "home", "screen", "rail"}

// Address is what an address opens: an object's tab (and a section of it),
// an entry of Home, or a screen (and a section of it).
type Address struct {
	Universe string
	// Kind is object, home or screen (a rail entry opens its screen).
	Kind string
	// ID is the object's id (Kind object), kept as written.
	ID string
	// Tab is the object's tab id: tab:<stem>, tab:children/<type>; empty
	// when the address names none (the pane opens on the type's first tab).
	Tab string
	// Section is a section of the tab (section:fields, section:cap/<cap>)
	// or of the screen.
	Section string
	// Entry is Home's strip entry (Kind home).
	Entry string
	// Screen is the screen opened (Kind screen).
	Screen string
}

// AddressOf is an address without its link prefix or the page's `#/open/`.
func AddressOf(raw string) string {
	s := strings.TrimSpace(raw)
	if v, ok := strings.CutPrefix(s, AddressLink); ok {
		s = v
	} else if v, ok := strings.CutPrefix(s, "#/open/"); ok {
		s = v
	}
	if d, err := url.PathUnescape(s); err == nil {
		s = d
	}
	return strings.Trim(s, "/")
}

// WithUniverse puts universe in front of an address that names none (its
// first segment is a place): `object//links` in the case's universe.
func WithUniverse(raw, universe string) string {
	a := AddressOf(raw)
	first, _, _ := strings.Cut(a, "/")
	if slices.Contains(addressPlaces, first) {
		return universe + "/" + a
	}
	return a
}

// ParseAddress is what an address opens, or why it opens nothing.
func ParseAddress(raw string) (Address, error) {
	var segs []string
	for _, s := range strings.Split(AddressOf(raw), "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) < 2 {
		return Address{}, fmt.Errorf("%q is no address: want <universe>/<place>/<path>", raw)
	}
	universe, place, rest := segs[0], segs[1], segs[2:]
	at := func(i int) string {
		if i < len(rest) {
			return rest[i]
		}
		return ""
	}
	switch place {
	case "object":
		id, tab := at(0), at(1)
		if id == "" {
			return Address{}, fmt.Errorf("%q: an object address names the object", raw)
		}
		a := Address{Universe: universe, Kind: "object", ID: id}
		switch {
		case tab == "":
			// No tab named: the pane opens on the type's first tab.
		case tab == "children":
			if at(2) == "" {
				return Address{}, fmt.Errorf("%q: children names their type", raw)
			}
			a.Tab = "tab:children/" + at(2)
		default:
			a.Tab = "tab:" + tab
			// A section lives in the Information tab: information/fields,
			// information/cap/<capability>.
			if at(2) == "cap" && at(3) != "" {
				a.Section = "section:cap/" + at(3)
			} else if at(2) != "" {
				a.Section = "section:" + at(2)
			}
		}
		return a, nil
	case "home":
		entry := at(0)
		if entry == "" {
			entry = "actions"
		}
		return Address{Universe: universe, Kind: "home", Entry: entry}, nil
	case "screen":
		if !slices.Contains(AddressScreens, at(0)) {
			return Address{}, fmt.Errorf("%q: no screen %q (%s)", raw, at(0), strings.Join(AddressScreens, ", "))
		}
		return Address{Universe: universe, Kind: "screen", Screen: at(0), Section: at(1)}, nil
	case "rail":
		screen, ok := addressRail[at(0)]
		if !ok {
			return Address{}, fmt.Errorf("%q: the rail entry %q opens no screen", raw, at(0))
		}
		return Address{Universe: universe, Kind: "screen", Screen: screen}, nil
	}
	return Address{}, fmt.Errorf("%q: no place %q (%s)", raw, place, strings.Join(addressPlaces, ", "))
}
