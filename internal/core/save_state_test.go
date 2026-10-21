package core

import "testing"

func TestSaveStateTracksDirtyTextAndPath(t *testing.T) {
	state := NewSaveState("alpha")
	if state.Dirty() {
		t.Fatal("new text state must be clean")
	}
	if state.Text() != "alpha" {
		t.Fatalf("text = %q", state.Text())
	}

	state.SetText("beta")
	if !state.Dirty() {
		t.Fatal("edit must mark dirty")
	}

	state.MarkSaved(`C:\Temp\beta.txt`)
	if state.Dirty() {
		t.Fatal("saved state must be clean")
	}
	if state.Path() != `C:\Temp\beta.txt` {
		t.Fatalf("path = %q", state.Path())
	}
	if state.ObservedPath() != state.Path() {
		t.Fatalf("observed path = %q, path = %q", state.ObservedPath(), state.Path())
	}
}

func TestSaveStateBecomesCleanWhenTextMatchesLastSave(t *testing.T) {
	state := NewSaveState("alpha")
	state.SetText("beta")
	state.MarkSaved(`C:\Temp\beta.txt`)
	state.SetText("changed")
	state.SetText("beta")

	if state.Dirty() {
		t.Fatal("matching the last saved text must clear the dirty state")
	}
}

func TestSaveStateKeepsNewerEditsDirtyAfterSavingEarlierText(t *testing.T) {
	state := NewSaveState("alpha")
	state.SetText("written")
	state.SetText("newer")

	state.MarkSavedText(`C:\Temp\written.txt`, "written")
	if !state.Dirty() {
		t.Fatal("edits made after the saved text must remain dirty")
	}
	if state.Path() != `C:\Temp\written.txt` {
		t.Fatalf("path = %q", state.Path())
	}
}

func TestReadOnlySaveStateIgnoresTextEdits(t *testing.T) {
	state := NewReadOnlySaveState()
	state.SetText("ignored")

	if state.Text() != "" {
		t.Fatalf("read-only text = %q", state.Text())
	}
	if state.Dirty() {
		t.Fatal("read-only state must never be dirty")
	}
}
