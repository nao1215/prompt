package prompt

import (
	"fmt"

	"github.com/mattn/go-runewidth"
)

// The prompt draws its block from wherever the cursor is, so the block is only
// ever as tall as the terminal by luck. A taller one is drawn anyway: the
// terminal scrolls by however far it overflows, and what leaves the top of the
// screen is not the prompt's to spend -- it is the scrollback the application
// printed before the prompt, and it goes again on the next keystroke, because
// the erase that precedes a redraw is a move up that the terminal clamps at its
// top row.
//
// A block taller than the terminal is ordinary rather than exotic: the one user
// of this library is a SQL shell whose entries are multiline by design, and a
// statement longer than a split pane is a statement a person writes.
//
// So the block gets a viewport: the rows around the caret that the terminal has
// room for are drawn in place, and the rows above and below are left out rather
// than emitted and scrolled away. Clamping the escape sequences instead would
// change nothing, because the terminal already clamps them; the rows have to
// stop being drawn.

// prefixSegment marks a run of a row that came from the prompt prefix or the
// continuation prefix rather than from the input. The highlighter's spans index
// the input, so a run it cannot address needs a value no rune offset can be.
const prefixSegment = -1

// rowSegment is a run of text on one terminal row, tagged with where it came
// from: start is the rune offset of its first rune within the input, or
// prefixSegment.
type rowSegment struct {
	text  string
	start int
}

// blockRow is one terminal row of the drawn block, in the order the runs are
// written.
type blockRow []rowSegment

// rowSplitter cuts the text of a block into the terminal rows it occupies, by
// the same rules layout counts them with. The two have to agree: layout says how
// tall the block is and which row the caret sits on, and this says which rows are
// drawn, so a disagreement puts the caret on a row holding something else.
type rowSplitter struct {
	width int
	col   int
	rows  []blockRow
	row   blockRow
	buf   []rune
	start int // where buf[0] came from
	next  int // where the next rune must come from to continue the run
}

func newRowSplitter(width int) *rowSplitter {
	return &rowSplitter{width: width, start: prefixSegment, next: prefixSegment}
}

// add places one rune, breaking the row first when the terminal would have.
func (s *rowSplitter) add(r rune, origin int) {
	switch r {
	case '\t':
		// A tab stops one column short of the margin rather than taking the
		// wrap, so it never fills a row; a tab that starts on a filled one
		// belongs to the row after it.
		if s.col >= s.width {
			s.breakRow()
		}
		s.col = min(s.col+tabWidth-s.col%tabWidth, s.width-1)
	default:
		// A rune of no width joins the cell already written -- a combining mark
		// is the case -- so it starts no row and takes no column.
		if w := runewidth.RuneWidth(r); w > 0 {
			if s.col+w > s.width && s.col > 0 {
				s.breakRow()
			}
			s.col += w
		}
	}
	s.put(r, origin)
}

func (s *rowSplitter) put(r rune, origin int) {
	if len(s.buf) > 0 && origin != s.next {
		s.flush()
	}
	if len(s.buf) == 0 {
		s.start = origin
	}
	s.buf = append(s.buf, r)
	s.next = origin + 1
	if origin == prefixSegment {
		s.next = prefixSegment
	}
}

func (s *rowSplitter) flush() {
	if len(s.buf) == 0 {
		return
	}
	s.row = append(s.row, rowSegment{text: string(s.buf), start: s.start})
	s.buf = s.buf[:0]
}

// breakRow ends the row being built, whether the terminal wrapped it or the
// input broke it.
func (s *rowSplitter) breakRow() {
	s.flush()
	s.rows = append(s.rows, s.row)
	s.row, s.col = nil, 0
}

func (s *rowSplitter) finish() []blockRow {
	s.flush()
	return append(s.rows, s.row)
}

// blockRowsOf returns the terminal rows the block occupies, each one carrying
// the runs that are drawn on it. A row holding nothing is still a row: an empty
// line in the entry occupies one.
func (r *renderer) blockRowsOf(prefix, input string) []blockRow {
	splitter := newRowSplitter(r.terminalWidth())
	lineStart := 0
	for i, line := range r.splitIntoLines(input) {
		if i > 0 {
			splitter.breakRow()
		}
		for _, ru := range r.linePrefix(i, prefix) {
			splitter.add(ru, prefixSegment)
		}
		runes := []rune(line)
		for offset, ru := range runes {
			splitter.add(ru, lineStart+offset)
		}
		// The next line starts after this one and the newline between them,
		// which is what the highlighter's spans are counted in.
		lineStart += len(runes) + 1
	}
	return splitter.finish()
}

// viewportTop returns the first row of the block to draw: the window of height
// rows that holds the caret, moved as little as the caret allows.
//
// Moving as little as possible is the point. Centering the caret would put the
// block somewhere different on every cursor key, so a person reading what they
// are typing would lose their place to the scrolling rather than to the height.
func viewportTop(previous, caret, total, height int) int {
	if height <= 0 || total <= height {
		return 0
	}
	top := min(max(previous, 0), total-height)
	if caret < top {
		top = caret
	}
	if caret >= top+height {
		top = caret - height + 1
	}
	return min(max(top, 0), total-height)
}

// renderClipped draws the rows of the block the terminal has room for and
// returns how many it drew and which of them the caret belongs on.
//
// Every row is written out rather than left to the terminal to wrap, because a
// window that starts in the middle of a wrapped line has no whole line to hand
// over. That is why this is not the path a block that fits takes: there, the
// terminal does its own wrapping and the renderer stays out of the way.
func (r *renderer) renderClipped(prefix, input string, caretRow int, spans []StyleSpan) (drawn, caret int, err error) {
	rows := r.blockRowsOf(prefix, input)
	top := viewportTop(r.viewTop, caretRow, len(rows), r.terminalHeight())
	r.viewTop = top

	end := min(top+r.terminalHeight(), len(rows))
	for i := top; i < end; i++ {
		if i > top {
			if _, err := fmt.Fprint(r.output, "\n"); err != nil {
				return 0, 0, err
			}
		}
		if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
			return 0, 0, err
		}
		if err := r.writeRow(rows[i], spans); err != nil {
			return 0, 0, err
		}
	}
	return end - top, caretRow - top, nil
}

// writeRow writes one row's runs, coloring each the way the unclipped path
// colors the line it belongs to.
func (r *renderer) writeRow(row blockRow, spans []StyleSpan) error {
	for _, segment := range row {
		if segment.start == prefixSegment {
			if _, err := fmt.Fprint(r.output, r.colorScheme.Prefix.ToANSI(), segment.text, ansiReset()); err != nil {
				return err
			}
			continue
		}
		if err := r.renderLineContent(segment.text, segment.start, spans); err != nil {
			return err
		}
	}
	return nil
}
