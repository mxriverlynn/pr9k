package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"
)

// pos is a cursor position in the log panel. rawIdx and rawOffset are the
// authoritative coordinates — stable across rewrap and ring-buffer eviction
// decrements. visualRow and col are derived render-time coordinates in display
// cells; they are recomputed from visualLines on every rewrap. col is virtual
// (vim-style): movement preserves the intended column across shorter lines.
type pos struct {
	rawIdx    int
	rawOffset int
	visualRow int
	col       int
}

// selection holds the anchor and cursor positions for a text selection in the
// log panel. active is true while a mouse drag is in progress (mid-drag).
// committed is true once the drag has been released (or a keyboard selection
// has been finalised) and the range is ready to copy.
type selection struct {
	anchor    pos
	cursor    pos
	active    bool
	committed bool
}

// visible reports whether the selection should be displayed. It is true when
// the selection is active (mid-drag) or when it is committed with a non-empty
// range. Used to gate auto-scroll-to-bottom suppression.
func (s selection) visible() bool {
	if s.active {
		return true
	}
	if s.committed {
		start, end := s.normalized()
		return start.rawIdx != end.rawIdx || start.rawOffset != end.rawOffset
	}
	return false
}

// normalized returns the anchor and cursor ordered in reading order by
// (rawIdx, rawOffset), so start is always earlier in the document than end.
func (s selection) normalized() (start, end pos) {
	if s.anchor.rawIdx < s.cursor.rawIdx ||
		(s.anchor.rawIdx == s.cursor.rawIdx && s.anchor.rawOffset <= s.cursor.rawOffset) {
		return s.anchor, s.cursor
	}
	return s.cursor, s.anchor
}

// contains reports whether the given visual (row, col) falls within the
// selected range. Uses a half-open convention on col: the start column is
// included but the end column is excluded, consistent with extractText's
// rawOffset slicing.
func (s selection) contains(row, col int) bool {
	start, end := s.normalized()
	if start.visualRow == end.visualRow {
		return row == start.visualRow && col >= start.col && col < end.col
	}
	if row < start.visualRow || row > end.visualRow {
		return false
	}
	if row == start.visualRow {
		return col >= start.col
	}
	if row == end.visualRow {
		return col < end.col
	}
	return true
}

// extractText reconstructs the selected text from raw ring-buffer lines.
// It uses raw coordinates (not visual) so that wrap-induced visual segments
// never inject artificial newlines into the result. The output preserves the
// original newline structure of the ring buffer.
func extractText(lines []string, start, end pos) string {
	if start.rawIdx == end.rawIdx {
		return lines[start.rawIdx][start.rawOffset:end.rawOffset]
	}
	var sb strings.Builder
	sb.WriteString(lines[start.rawIdx][start.rawOffset:])
	for i := start.rawIdx + 1; i < end.rawIdx; i++ {
		sb.WriteByte('\n')
		sb.WriteString(lines[i])
	}
	sb.WriteByte('\n')
	sb.WriteString(lines[end.rawIdx][:end.rawOffset])
	return sb.String()
}

// MouseToViewport translates a tea.MouseMsg into viewport-content coordinates.
// topRow and leftCol are the screen-space top-left corner of the viewport
// content area (0-indexed). The returned pos has visualRow set to
// vp.YOffset + (msg.Y - topRow) and col set to msg.X - leftCol.
// ok is false when msg.Y is above topRow or below topRow + vp.Height - 1
// (i.e. the click landed on chrome rather than the viewport content area).
// Callers in an active drag may still consume the event for auto-scroll even
// when ok is false; this helper only reports geometry.
func MouseToViewport(msg tea.MouseMsg, topRow, leftCol int, vp viewport.Model) (pos, bool) {
	if msg.Y < topRow || msg.Y > topRow+vp.Height-1 {
		return pos{}, false
	}
	visualRow := vp.YOffset + (msg.Y - topRow)
	col := msg.X - leftCol
	return pos{visualRow: visualRow, col: col}, true
}

// visualColToRawOffset converts a display-column index within a visual segment
// to a byte offset within rawLine. segmentStart is the byte offset in rawLine
// where the visual segment begins. col is the 0-based display column to
// locate within the segment.
//
// The function walks forward from segmentStart counting display cells using
// Unicode grapheme-cluster boundaries (via github.com/rivo/uniseg). Because
// raw ring-buffer lines are plain text with tabs already normalised to spaces
// (see logModel.Update), the cell walk is straightforward.
//
// If col exceeds the total display cells available from segmentStart to the
// end of rawLine, the end offset of rawLine is returned.
func visualColToRawOffset(rawLine string, segmentStart int, col int) int {
	seg := rawLine[segmentStart:]
	cellCount := 0
	byteOffset := 0
	state := -1
	for byteOffset < len(seg) {
		cluster, _, _, newState := uniseg.FirstGraphemeClusterInString(seg[byteOffset:], state)
		if cluster == "" {
			break
		}
		w := uniseg.StringWidth(cluster)
		if cellCount+w > col {
			break
		}
		cellCount += w
		byteOffset += len(cluster)
		state = newState
	}
	return segmentStart + byteOffset
}
