package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

func clear() {
	fmt.Print("\r\033[K")
}

type KeyType int

const (
	Char KeyType = iota
	UpArrow
	DownArrow
	LeftArrow
	RightArrow
	BackSpace
	Enter
	Escape
	Unknown
)

// LINES is now a 2D slice: a slice of lines, where each line is a slice of runes
var LINES [][]rune

type event struct {
	Type KeyType
	Rune rune
}

type editor struct {
	cx int // Horizontal cursor position (column)
	cy int // Vertical cursor position (row)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		fmt.Fprintln(os.Stderr, "failed to enter raw mode")
		os.Exit(1)
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to change state")
		os.Exit(1)
	}
	defer term.Restore(fd, oldState)

	// Initialize with one empty line so LINES[0] exists
	LINES = append(LINES, []rune{})

	dataChan := make(chan event)
	go read(fd, ctx, dataChan)

	ed := editor{cx: 0, cy: 0}

	// Clear the screen and home the cursor at start
	fmt.Print("\033[2J\033[H")
	renderAll(ed)

	for ev := range dataChan {
		switch ev.Type {
		case UpArrow:
			if ed.cy > 0 {
				ed.cy--
				// Snap horizontal cursor if the upper line is shorter
				if ed.cx > len(LINES[ed.cy]) {
					ed.cx = len(LINES[ed.cy])
				}
				renderAll(ed)
			}

		case DownArrow:
			if ed.cy < len(LINES)-1 {
				ed.cy++
				// Snap horizontal cursor if the lower line is shorter
				if ed.cx > len(LINES[ed.cy]) {
					ed.cx = len(LINES[ed.cy])
				}
				renderAll(ed)
			}

		case RightArrow:
			if ed.cx < len(LINES[ed.cy]) {
				ed.cx++
				renderAll(ed)
			} else if ed.cy < len(LINES)-1 { // Wrap to next line
				ed.cy++
				ed.cx = 0
				renderAll(ed)
			}

		case LeftArrow:
			if ed.cx > 0 {
				ed.cx--
				renderAll(ed)
			} else if ed.cy > 0 { // Wrap to previous line
				ed.cy--
				ed.cx = len(LINES[ed.cy])
				renderAll(ed)
			}

		case Char:
			currentLine := LINES[ed.cy]
			// Insert rune at ed.cx
			LINES[ed.cy] = append(currentLine[:ed.cx], append([]rune{ev.Rune}, currentLine[ed.cx:]...)...)
			ed.cx++
			renderAll(ed)

		case BackSpace:
			if ed.cx > 0 {
				// Standard backspace on the same line
				currentLine := LINES[ed.cy]
				LINES[ed.cy] = append(currentLine[:ed.cx-1], currentLine[ed.cx:]...)
				ed.cx--
				renderAll(ed)
			} else if ed.cy > 0 {
				// Line joining: Backspace at the start of a line
				prevLineIdx := ed.cy - 1
				ed.cx = len(LINES[prevLineIdx]) // Cursor moves to end of previous line

				// Append current line content to the previous line
				LINES[prevLineIdx] = append(LINES[prevLineIdx], LINES[ed.cy]...)

				// Remove the current line from the 2D slice
				LINES = append(LINES[:ed.cy], LINES[ed.cy+1:]...)
				ed.cy--
				renderAll(ed)
			}

		case Enter:
			currentLine := LINES[ed.cy]
			remainingText := []rune{}

			if ed.cx < len(currentLine) {
				// Save text to the right of the cursor
				remainingText = currentLine[ed.cx:]
				// Truncate current line at the cursor
				LINES[ed.cy] = currentLine[:ed.cx]
			} else {
				LINES[ed.cy] = currentLine
			}

			// Insert a new line directly below the current row
			LINES = append(LINES[:ed.cy+1], append([][]rune{remainingText}, LINES[ed.cy+1:]...)...)
			ed.cy++
			ed.cx = 0
			renderAll(ed)
		}
	}
}

func renderAll(ed editor) {
	// 1. Move cursor to top-left home position (\033[H)
	fmt.Print("\033[H")

	// 2. Print all lines, clearing old content on each line
	for i, line := range LINES {
		fmt.Print(string(line))
		fmt.Print("\033[K") // Clear anything remaining to the right
		if i < len(LINES)-1 {
			fmt.Print("\r\n")
		}
	}

	// 3. Clear any leftover lines below our text if a line was deleted
	fmt.Print("\r\n\033[J")

	// 4. Reposition the hardware cursor to matches ed.cx and ed.cy
	// Terminal escape sequences for grid positioning are 1-indexed
	fmt.Printf("\033[%d;%dH", ed.cy+1, ed.cx+1)
}

func read(fd uintptr, ctx context.Context, dataChan chan<- event) {
	defer close(dataChan)
	reader := bufio.NewReader(os.Stdin)

	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(os.Stderr, "read error")
			break
		}

		if r == 0x03 { // Ctrl+C
			// Clean up screen exit
			fmt.Print("\033[2J\033[H")
			fmt.Print("exiting...\r\n")
			break
		}

		if r == 0x1b {
			peek, err := reader.Peek(2)
			if err == nil && peek[0] == '[' {
				reader.Discard(2)
				switch peek[1] {
				case 'A':
					dataChan <- event{Type: UpArrow}
				case 'B':
					dataChan <- event{Type: DownArrow}
				case 'C':
					dataChan <- event{Type: RightArrow}
				case 'D':
					dataChan <- event{Type: LeftArrow}
				default:
					dataChan <- event{Type: Unknown}
				}
				continue
			}
			dataChan <- event{Type: Escape}
			continue
		}

		switch r {
		case '\r', '\n':
			dataChan <- event{Type: Enter}
		case 0x7f, 0x08:
			dataChan <- event{Type: BackSpace}
		default:
			dataChan <- event{Type: Char, Rune: r}
		}
	}
}
