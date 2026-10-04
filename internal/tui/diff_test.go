package tui

import (
	"strings"
	"testing"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// changesSnapshot is a session with two runs and three changed paths: the
// smallest history that exercises run headers, the cursor and the diff.
func changesSnapshot() Snapshot {
	snap := baseSnapshot()
	snap.Runs = []workspace.Run{
		{
			Command: []string{"sh", "-c", "echo hi > notes.txt"},
			Entries: []workspace.Entry{{Path: "notes.txt", Kind: workspace.Added, Size: 3}},
		},
		{
			Command: []string{"sh", "-c", "edit notes.txt"},
			Entries: []workspace.Entry{
				{Path: "notes.txt", Kind: workspace.Modified, Size: 4},
				{Path: "gone.txt", Kind: workspace.Deleted},
			},
		},
	}
	snap.RunIDs = []string{"run-0001", "run-0002"}
	return snap
}

// A changed file must be reachable from the keyboard: the review view is the
// point of the session, and a list nobody can open is a list nobody uses.
func TestChangesViewOpensADiff(t *testing.T) {
	snap := changesSnapshot()
	st := newState(120, 40)
	st.View = ViewChanges

	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev.kind != evDiff {
		t.Fatalf("enter on a change must ask for its diff, got %v", ev.kind)
	}
	if ev.runID != "run-0001" || ev.entry != "notes.txt" {
		t.Fatalf("the request must address the selected path, got %q %q", ev.runID, ev.entry)
	}
	if !st.DiffOpen {
		t.Fatal("enter must open the diff pane")
	}
}

// The cursor must skip run headers. A header is a label; stopping on it would
// make Enter do nothing and read as a broken key.
func TestChangesCursorSkipsHeaders(t *testing.T) {
	snap := changesSnapshot()
	rows := BuildChangeRows(&snap)
	pick := Selectable(rows)
	if len(pick) != 3 {
		t.Fatalf("expected 3 selectable changes, got %d", len(pick))
	}
	for _, idx := range pick {
		if rows[idx].Header {
			t.Fatal("a header must never be selectable")
		}
	}
	st := newState(120, 40)
	st.View = ViewChanges
	st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev := st.handleKey(Key{Type: KeyDown}, &snap); ev.entry != "notes.txt" {
		t.Fatalf("down must reach the next change, got %q", ev.entry)
	}
	if ev := st.handleKey(Key{Type: KeyDown}, &snap); ev.entry != "gone.txt" {
		t.Fatalf("down must reach the last change, got %q", ev.entry)
	}
}

// With no changes at all there is nothing to review, and the view must say so
// instead of opening an empty pane.
func TestChangesViewWithNoEntriesStaysClosed(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.View = ViewChanges
	if ev := st.handleKey(Key{Type: KeyEnter}, &snap); ev.kind == evDiff {
		t.Fatal("there is nothing to review, so no diff may be requested")
	}
	if st.DiffOpen {
		t.Fatal("the diff pane must stay closed when there is nothing to show")
	}
}

// The loaded diff must belong to the highlighted path. Showing one file's
// content next to another file's name is the worst kind of wrong.
func TestDiffPaneRefusesAMismatchedDiff(t *testing.T) {
	snap := changesSnapshot()
	snap.Diff = DiffState{
		RunID: "run-0001", Path: "notes.txt", Loaded: true,
		Lines: []string{"+ secret content"},
	}
	st := newState(120, 40)
	st.View = ViewChanges
	st.DiffOpen = true
	text := renderText(120, 40, &snap, st)
	if !strings.Contains(text, "notes.txt") {
		t.Fatalf("the pane must name the selected path, got:\n%s", text)
	}
	if !strings.Contains(text, "secret content") {
		t.Fatalf("a matching diff must be shown, got:\n%s", text)
	}

	// Point the snapshot at another path: the stale diff must vanish.
	snap.Diff.Path = "gone.txt"
	text = renderText(120, 40, &snap, st)
	if strings.Contains(text, "secret content") {
		t.Fatalf("a diff for another path must never be shown, got:\n%s", text)
	}
}

// Binary content is reported as binary rather than rendered as noise.
func TestDiffPaneSaysBinaryInsteadOfGuessing(t *testing.T) {
	snap := changesSnapshot()
	snap.Diff = DiffState{
		RunID: "run-0001", Path: "notes.txt", Loaded: true, Binary: true,
	}
	st := newState(120, 40)
	st.View = ViewChanges
	st.DiffOpen = true
	if text := renderText(120, 40, &snap, st); !strings.Contains(text, "binary") {
		t.Fatalf("binary content must be named, got:\n%s", text)
	}
}

// A load that failed must say so. A pane that silently shows nothing reads as
// "this file did not change", which is a different and wrong claim.
func TestDiffPaneReportsAFailedLoad(t *testing.T) {
	snap := changesSnapshot()
	snap.Diff = DiffState{
		RunID: "run-0001", Path: "notes.txt", Loaded: true,
		Note: "the content was not captured",
	}
	st := newState(120, 40)
	st.View = ViewChanges
	st.DiffOpen = true
	text := renderText(120, 40, &snap, st)
	if !strings.Contains(text, "not captured") {
		t.Fatalf("the failure reason must be on screen, got:\n%s", text)
	}
}

// The diff pane must not cover the change list on a wide terminal: the review
// act is comparison, and the user has to see both sides.
func TestDiffPaneKeepsTheListVisible(t *testing.T) {
	snap := changesSnapshot()
	snap.Diff = DiffState{
		RunID: "run-0001", Path: "notes.txt", Loaded: true,
		Lines: []string{"+ hi"},
	}
	st := newState(160, 40)
	st.View = ViewChanges
	st.DiffOpen = true
	text := renderText(160, 40, &snap, st)
	if !strings.Contains(text, "gone.txt") {
		t.Fatalf("the change list must stay visible beside the diff, got:\n%s", text)
	}
	if !strings.Contains(text, "+ hi") {
		t.Fatalf("the diff must be shown, got:\n%s", text)
	}
}
