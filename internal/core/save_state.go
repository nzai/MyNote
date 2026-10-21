package core

type SaveState struct {
	originalText  string
	currentText   string
	lastSavedText string
	savedPath     string
	readOnly      bool
}

func NewSaveState(text string) *SaveState {
	return &SaveState{
		originalText:  text,
		currentText:   text,
		lastSavedText: text,
	}
}

func NewReadOnlySaveState() *SaveState {
	return &SaveState{readOnly: true}
}

func (s *SaveState) Text() string {
	return s.currentText
}

func (s *SaveState) SetText(text string) {
	if s.readOnly {
		return
	}
	s.currentText = text
}

func (s *SaveState) MarkSaved(path string) {
	s.lastSavedText = s.currentText
	s.savedPath = path
}

func (s *SaveState) Path() string {
	return s.savedPath
}

func (s *SaveState) Dirty() bool {
	return !s.readOnly && s.currentText != s.lastSavedText
}

func (s *SaveState) ObservedPath() string {
	return s.savedPath
}
