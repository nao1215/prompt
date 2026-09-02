package prompt

// KeyAction represents the action to perform when a key is pressed
type KeyAction int

// Key action constants define the actions that can be performed when keys are pressed
const (
	ActionNone KeyAction = iota
	ActionSubmit
	ActionCancel
	ActionMoveLeft
	ActionMoveRight
	ActionMoveUp
	ActionMoveDown
	ActionMoveHome
	ActionMoveEnd
	ActionMoveWordLeft
	ActionMoveWordRight
	ActionDeleteChar
	// ActionDeleteLine deletes the line the cursor is on, which on an entry of
	// one line is the whole of it. What discards an entry of several is
	// ActionCancel, which ends the line and reports ErrInterrupted.
	ActionDeleteLine
	ActionDeleteToEnd
	ActionDeleteWordBack
	ActionComplete
	// ActionHistoryUp moves back through the history and ActionHistoryDown
	// forward, whatever the entry looks like. The arrow keys do the same on an
	// entry of one line; on a multiline one they move the cursor between its
	// lines instead, which is why these exist. The default key map binds no key
	// to either, so they are reached through Bind -- Ctrl+P and Ctrl+N are what
	// a shell puts them on.
	ActionHistoryUp
	ActionHistoryDown
	ActionHistorySearch
	ActionNewLine
	actionPasteStart
	actionPasteEnd
	// ActionClearScreen clears the terminal screen and redraws the prompt at the
	// top of it with the current input preserved, like Ctrl+L in a typical
	// shell. The scrollback is left alone: it holds whatever the application
	// printed before the prompt, which is what the user has in front of them
	// when they reach for the key.
	ActionClearScreen
)

const (
	bracketedPasteEnableSequence  = "\x1b[?2004h"
	bracketedPasteDisableSequence = "\x1b[?2004l"
	// maxCSILength bounds how much of a CSI sequence is remembered while looking
	// for its final byte. It is far longer than any key sequence a terminal
	// sends; a sequence that outgrows it is read to its end and reported as no
	// sequence at all, since nothing could be bound to it anyway.
	maxCSILength = 32
	// csiParamFirst and csiParamLast bracket the parameter and intermediate
	// bytes of a CSI sequence, and csiFinalFirst and csiFinalLast its final
	// byte. A byte outside both ranges aborts the sequence.
	csiParamFirst = ' '
	csiParamLast  = '?'
	csiFinalFirst = '@'
	csiFinalLast  = '~'
)

// KeyMap holds the key binding configuration
type KeyMap struct {
	bindings  map[rune]KeyAction
	sequences map[string]KeyAction
}

// NewDefaultKeyMap creates the default key bindings for the prompt.
//
// The default key map includes common terminal shortcuts and navigation keys.
// You can create a custom key map by modifying the returned KeyMap or by
// creating a new one and using the Bind and BindSequence methods.
//
// Default key bindings:
//   - Enter/Return: Submit input
//   - Ctrl+C: Cancel (interrupt)
//   - Ctrl+A: Move to beginning of line
//   - Ctrl+E: Move to end of line
//   - Ctrl+K: Delete from cursor to end of line
//   - Ctrl+U: Delete the line the cursor is on
//   - Ctrl+W: Delete word backwards
//   - Ctrl+R: Reverse history search
//   - Ctrl+L: Clear the screen, keeping the scrollback
//   - Tab: Auto-completion
//   - Backspace: Delete character backwards
//   - Arrow keys: Navigate history and move cursor
//   - Home/End: Move to line beginning/end
//   - Delete: Delete character forwards
//   - Ctrl+Left/Right: Move by word
//
// Example:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Reach the history from a multiline entry, where the arrow keys are
//	// moving the cursor between its lines.
//	keyMap.Bind('\x10', prompt.ActionHistoryUp)   // Ctrl+P
//	keyMap.Bind('\x0E', prompt.ActionHistoryDown) // Ctrl+N
//
//	p, err := prompt.New("$ ", prompt.WithKeyMap(keyMap))
func NewDefaultKeyMap() *KeyMap {
	km := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Default key bindings
	km.bindings['\r'] = ActionSubmit
	km.bindings['\n'] = ActionSubmit
	km.bindings['\x03'] = ActionCancel         // Ctrl+C
	km.bindings['\x01'] = ActionMoveHome       // Ctrl+A
	km.bindings['\x05'] = ActionMoveEnd        // Ctrl+E
	km.bindings['\x0B'] = ActionDeleteToEnd    // Ctrl+K
	km.bindings['\x15'] = ActionDeleteLine     // Ctrl+U
	km.bindings['\x17'] = ActionDeleteWordBack // Ctrl+W
	km.bindings['\x12'] = ActionHistorySearch  // Ctrl+R
	km.bindings['\x0C'] = ActionClearScreen    // Ctrl+L
	km.bindings['\t'] = ActionComplete
	km.bindings['\x7f'] = ActionDeleteChar // Backspace
	km.bindings['\b'] = ActionDeleteChar   // Backspace

	// Escape sequences
	km.sequences["[A"] = ActionMoveUp
	km.sequences["[B"] = ActionMoveDown
	km.sequences["[C"] = ActionMoveRight
	km.sequences["[D"] = ActionMoveLeft
	km.sequences["[H"] = ActionMoveHome
	km.sequences["[F"] = ActionMoveEnd
	km.sequences["[1;5C"] = ActionMoveWordRight // Ctrl+Right
	km.sequences["[1;5D"] = ActionMoveWordLeft  // Ctrl+Left
	km.sequences["[3~"] = ActionDeleteChar      // Delete
	km.sequences["[200~"] = actionPasteStart
	km.sequences["[201~"] = actionPasteEnd

	return km
}

// Bind adds or updates a key binding for a single character.
//
// Use this method to bind actions to control characters, printable characters,
// or special keys that can be represented as a single rune.
//
// Example:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Bind Ctrl+P to the previous history entry
//	keyMap.Bind('\x10', prompt.ActionHistoryUp)
//
// A key the default map already names keeps whatever it is bound to last, so
// binding one of those replaces the behavior this package documents for it. A
// key a terminal sends as an escape sequence -- a function key, an arrow -- is
// not a rune and is bound with BindSequence instead.
func (km *KeyMap) Bind(key rune, action KeyAction) {
	km.bindings[key] = action
}

// BindSequence adds or updates an escape sequence binding.
//
// Use this method to bind actions to escape sequences like function keys,
// arrow keys, or other multi-character key combinations that start with ESC.
// The sequence should not include the initial ESC character.
//
// Example:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Bind F1 key (ESC + OP)
//	keyMap.BindSequence("OP", prompt.ActionComplete)
//	// Bind Shift+Tab (ESC + [Z)
//	keyMap.BindSequence("[Z", prompt.ActionHistoryUp)
//	// Bind Page Up (ESC + [5~)
//	keyMap.BindSequence("[5~", prompt.ActionHistoryUp)
func (km *KeyMap) BindSequence(seq string, action KeyAction) {
	km.sequences[seq] = action
}

// Action returns what key is bound to, or ActionNone when it is bound to nothing.
func (km *KeyMap) Action(key rune) KeyAction {
	if km == nil || km.bindings == nil {
		return ActionNone
	}
	if action, exists := km.bindings[key]; exists {
		return action
	}
	return ActionNone
}

// SequenceAction returns what an escape sequence is bound to, or ActionNone when
// it is bound to nothing. The sequence is written without its leading ESC, as
// BindSequence takes it.
func (km *KeyMap) SequenceAction(seq string) KeyAction {
	if km == nil || km.sequences == nil {
		return ActionNone
	}
	if action, exists := km.sequences[seq]; exists {
		return action
	}
	return ActionNone
}

// readEscapeSequence reads what follows ESC and returns the key sequence it
// forms, without the ESC. It returns "" when ESC introduced no sequence at all:
// a bare Escape key, or Alt+key, which every terminal sends as ESC followed by
// the plain character.
//
// Only CSI ("[") and SS3 ("O") introduce a sequence, so any other rune is input
// in its own right and is pushed back for the read loop. Consuming it swallowed
// whatever the user typed right after pressing Escape.
func (p *Prompt) readEscapeSequence() (string, error) {
	r, err := p.readRune()
	if err != nil {
		return "", err
	}
	if r != '[' && r != 'O' {
		p.unreadRune(r)
		return "", nil
	}

	// SS3 is ESC O plus exactly one final character (F1-F4, keypad keys).
	if r == 'O' {
		final, err := p.readRune()
		if err != nil {
			return "", err
		}
		return string([]rune{r, final}), nil
	}

	// CSI is ESC [ parameters intermediates final, and ends at the first byte in
	// the final range. Reading to that byte keeps a long sequence (bracketed
	// paste markers, Ctrl+arrow, a mouse or paste report with many parameters)
	// whole instead of cutting it after three runes.
	//
	// The grammar decides where the sequence ends, not a rune count. A count can
	// only stop reading, and a sequence the terminal is still sending does not
	// stop with it: everything past the count stayed in the input and reached
	// the read loop as keystrokes, so a long terminal reply appeared in the
	// user's line one parameter at a time. maxCSILength bounds what is
	// remembered instead, so a sequence too long to name is still consumed
	// whole.
	seq := make([]rune, 0, 8)
	seq = append(seq, r)
	overlong := false
	for {
		r, err := p.readRune()
		if err != nil {
			return "", err
		}
		switch {
		case r >= csiFinalFirst && r <= csiFinalLast:
			if overlong {
				// The terminal is sending something this prompt cannot name. It
				// has been read to its end, so nothing is left to be mistaken
				// for typing.
				return "", nil
			}
			return string(append(seq, r)), nil
		case r >= csiParamFirst && r <= csiParamLast:
			if len(seq) >= maxCSILength {
				overlong = true
				continue
			}
			seq = append(seq, r)
		default:
			// A byte outside the grammar aborts the sequence and is input in its
			// own right. Counting it as a parameter swallowed it: ESC [ followed
			// by Enter left the line unsubmittable.
			p.unreadRune(r)
			return "", nil
		}
	}
}
