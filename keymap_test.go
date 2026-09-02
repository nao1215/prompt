package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestReadEscapeSequence(t *testing.T) {
	t.Parallel()

	// Test with mock terminal that provides escape sequences
	mock := &mockTerminal{
		input: []rune("[A"), // Up arrow (without initial ESC)
	}

	p := &Prompt{
		config:   options{Prefix: "test> "},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
	}

	seq, err := p.readEscapeSequence()
	if err != nil {
		t.Errorf("Expected no error reading escape sequence, got: %v", err)
	}

	if seq != "[A" {
		t.Errorf("Expected sequence '[A', got %q", seq)
	}
}

// TestReadEscapeSequenceBareEscape covers ESC that does not introduce a
// sequence: a bare Escape key, or Alt+key. The rune that follows belongs to the
// input, so it must be pushed back rather than consumed as sequence bytes —
// consuming it ate the next characters the user typed.
func TestReadEscapeSequenceBareEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantSeq string
		wantNex rune
	}{
		{name: "ESC followed by a letter reports no sequence and keeps the letter", input: "abc", wantSeq: "", wantNex: 'a'},
		{name: "ESC followed by a digit reports no sequence and keeps the digit", input: "1;", wantSeq: "", wantNex: '1'},
		{name: "CSI arrow is still recognized as a sequence", input: "[Ax", wantSeq: "[A", wantNex: 'x'},
		{name: "SS3 function key is still recognized as a sequence", input: "OPx", wantSeq: "OP", wantNex: 'x'},
		{name: "long CSI is read to its final byte", input: "[1;2;3;4;5;6;7;8;9;10Cx", wantSeq: "[1;2;3;4;5;6;7;8;9;10C", wantNex: 'x'},
		// A sequence too long to name is still read to its end, so its tail
		// cannot be picked up as typed text; only what follows it is.
		{name: "CSI past the bound reports no sequence and is consumed whole", input: "[" + strings.Repeat("1;", 40) + "Rx", wantSeq: "", wantNex: 'x'},
		// A byte outside the CSI grammar aborts the sequence and belongs to the
		// input, so it is pushed back the way a bare Escape's rune is.
		{name: "a control byte aborts a CSI and is kept", input: "[1;\rx", wantSeq: "", wantNex: '\r'},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &Prompt{
				config:   options{Prefix: "test> "},
				terminal: newMockTerminal(tt.input),
				keyMap:   NewDefaultKeyMap(),
			}

			seq, err := p.readEscapeSequence()
			if err != nil {
				t.Fatalf("readEscapeSequence returned error: %v", err)
			}
			if seq != tt.wantSeq {
				t.Errorf("sequence = %q, want %q", seq, tt.wantSeq)
			}

			next, err := p.readRune()
			if err != nil {
				t.Fatalf("reading the rune after the sequence: %v", err)
			}
			if next != tt.wantNex {
				t.Errorf("next rune = %q, want %q", next, tt.wantNex)
			}
		})
	}
}

func TestKeyMapMethods(t *testing.T) {
	t.Parallel()

	km := NewDefaultKeyMap()

	// Test Bind method
	km.Bind('x', ActionSubmit)
	action := km.Action('x')
	if action != ActionSubmit {
		t.Errorf("Expected ActionSubmit after binding 'x', got %v", action)
	}

	// Test BindSequence method
	km.BindSequence("TEST", ActionCancel)
	seqAction := km.SequenceAction("TEST")
	if seqAction != ActionCancel {
		t.Errorf("Expected ActionCancel after binding sequence 'TEST', got %v", seqAction)
	}

	// Test GetAction with unbound key
	action = km.Action('z')
	if action != ActionNone {
		t.Errorf("Expected ActionNone for unbound key 'z', got %v", action)
	}

	// Test GetSequenceAction with unbound sequence
	seqAction = km.SequenceAction("UNBOUND")
	if seqAction != ActionNone {
		t.Errorf("Expected ActionNone for unbound sequence 'UNBOUND', got %v", seqAction)
	}

	// Test nil KeyMap
	var nilKm *KeyMap
	action = nilKm.Action('a')
	if action != ActionNone {
		t.Errorf("Expected ActionNone for nil KeyMap, got %v", action)
	}

	seqAction = nilKm.SequenceAction("test")
	if seqAction != ActionNone {
		t.Errorf("Expected ActionNone for nil KeyMap sequence, got %v", seqAction)
	}
}

func TestNewDefaultKeyMapAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := NewDefaultKeyMap()
	if keyMap == nil {
		t.Error("Expected non-nil KeyMap")
		return
	}

	// Test some default key bindings
	if keyMap.bindings == nil {
		t.Error("Expected initialized bindings map")
	}

	if keyMap.sequences == nil {
		t.Error("Expected initialized sequences map")
	}

	// Test that default keys are bound
	enterAction := keyMap.Action('\r')
	if enterAction == ActionNone {
		t.Error("Expected Enter key to be bound")
	}

	backspaceAction := keyMap.Action('\b')
	if backspaceAction == ActionNone {
		t.Error("Expected Backspace key to be bound")
	}
}

func TestDefaultKeyMapBindsClearScreen(t *testing.T) {
	t.Parallel()

	km := NewDefaultKeyMap()
	if got := km.Action('\x0C'); got != ActionClearScreen {
		t.Errorf("Ctrl+L action = %v, want ActionClearScreen", got)
	}
}

func TestKeyMapBindAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Test binding a key
	keyMap.Bind('x', ActionComplete)

	action := keyMap.Action('x')
	if action != ActionComplete {
		t.Error("Expected bound action to be retrievable")
	}

	// Test overwriting a binding
	keyMap.Bind('x', ActionSubmit)

	retrievedAction := keyMap.Action('x')
	if retrievedAction != ActionSubmit {
		t.Error("Expected overwritten action to be retrievable")
	}
}

func TestKeyMapBindSequenceAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Test binding a sequence
	keyMap.BindSequence("[A", ActionHistoryUp)

	action := keyMap.SequenceAction("[A")
	if action != ActionHistoryUp {
		t.Error("Expected bound sequence action to be retrievable")
	}

	// Test nonexistent sequence
	action = keyMap.SequenceAction("[Z")
	if action != ActionNone {
		t.Error("Expected ActionNone for nonexistent sequence")
	}
}

func TestKeyMapGetActionAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Test getting action for unbound key
	action := keyMap.Action('z')
	if action != ActionNone {
		t.Error("Expected ActionNone for unbound key")
	}

	// Test getting action for bound key
	keyMap.Bind('z', ActionSubmit)

	action = keyMap.Action('z')
	if action != ActionSubmit {
		t.Error("Expected ActionSubmit for bound key")
	}
}

// TestReadEscapeSequenceConsumesASequenceItCannotName covers what happens past
// the bound on a CSI: the prompt stops naming the sequence, but it must still
// take it out of the input. Stopping the read left the rest of the sequence to
// be picked up as keystrokes, and the user watched a terminal reply appear in
// their line.
func TestReadEscapeSequenceConsumesASequenceItCannotName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "a sequence past the bound leaves nothing behind",
			input: "\x1b[" + strings.Repeat("1;", 40) + "R" + "ok\r",
			want:  "ok",
		},
		{
			name:  "a sequence within the bound is a key, not text",
			input: "\x1b[1;5Cok\r",
			want:  "ok",
		},
		{
			name:  "a control byte ends a sequence and is a key of its own",
			input: "ab\x1b[\r",
			want:  "ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.input))
			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}
