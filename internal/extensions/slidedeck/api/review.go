// SPDX-License-Identifier: Apache-2.0

package api

// A deck object's review: the comment threads on its slides and the marks of
// its past reviews. It is the object's `review` named file, beside it in
// decks/ — never in its body (OD-017). The deck view reads and writes it; an
// agent asked to address a review reads it with the named-file tools.

// DeckReview is the whole review file.
type DeckReview struct {
	Threads []ReviewThread `json:"threads"           jsonschema:"description=The comment threads in the order they were opened"`
	Reviews []ReviewMark   `json:"reviews,omitempty" jsonschema:"description=The reviews marked done - the last one is what changes are counted from"`
}

// SlideAnchor names the slide a thread is about so that it survives the slide
// moving: its title first, its position when the slide has no title or the
// title is gone.
type SlideAnchor struct {
	Index int    `json:"index"           jsonschema:"description=The slide's number from 0 in reading order when the thread was opened"`
	Title string `json:"title,omitempty" jsonschema:"description=The slide's title text when it has one"`
}

// ReviewThread is a conversation about one slide.
type ReviewThread struct {
	ID       string          `json:"id"                 jsonschema:"description=A handle unique in the file"`
	Slide    SlideAnchor     `json:"slide"              jsonschema:"description=The slide the thread is about"`
	Resolved bool            `json:"resolved,omitempty" jsonschema:"description=Closed by its author or the deck's author"`
	Comments []ReviewComment `json:"comments"           jsonschema:"description=The first comment then the replies"`
}

// ReviewComment is one message of a thread.
type ReviewComment struct {
	Author string `json:"author" jsonschema:"description=Who wrote it - the person's name"`
	Text   string `json:"text"   jsonschema:"description=What they wrote"`
	At     string `json:"at"     jsonschema:"description=When - an RFC 3339 time"`
}

// ReviewMark is a review marked done: the deck's slides as they were then, so
// a later look can say what was added and removed and changed since.
type ReviewMark struct {
	At     string      `json:"at"           jsonschema:"description=When the review was marked done - an RFC 3339 time"`
	By     string      `json:"by,omitempty" jsonschema:"description=Who marked it"`
	Slides []SlideMark `json:"slides"       jsonschema:"description=Every slide then in reading order"`
}

// SlideMark is one slide as a review saw it.
type SlideMark struct {
	Title string `json:"title,omitempty" jsonschema:"description=The slide's title text when it has one"`
	Hash  string `json:"hash"            jsonschema:"description=A fingerprint of the slide's content - its settings and slots and notes"`
}
