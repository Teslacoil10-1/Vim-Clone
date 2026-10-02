package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"unicode"

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
	WordBackSpace
	Enter
	Escape
	Unknown
)

var LINES [][]rune

type event struct {
	Type KeyType
	Rune rune
}

type editor struct {
	cx int
	cy int
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

	fmt.Print("\033[?1049h\033[2J")

	defer fmt.Print("\033[?1049l")

	LINES = append(LINES, []rune{})
	dataChan := make(chan event)
	go read(fd, ctx, dataChan)

	ed := editor{cx: 0, cy: 0}
	renderAll(ed)

	for ev := range dataChan {
		switch ev.Type {
		case UpArrow:
			if ed.cy > 0 {
				ed.cy--
				if ed.cx > len(LINES[ed.cy]) {
					ed.cx = len(LINES[ed.cy])
				}
				renderAll(ed)
			}
		case DownArrow:
			if ed.cy < len(LINES)-1 {
				ed.cy++
				if ed.cx > len(LINES[ed.cy]) {
					ed.cx = len(LINES[ed.cy])
				}
				renderAll(ed)
			}
		case RightArrow:
			if ed.cx < len(LINES[ed.cy]) {
				ed.cx++
				renderAll(ed)
			} else if ed.cy < len(LINES)-1 {
				ed.cy++
				ed.cx = 0
				renderAll(ed)
			}
		case LeftArrow:
			if ed.cx > 0 {
				ed.cx--
				renderAll(ed)
			} else if ed.cy > 0 {
				ed.cy--
				ed.cx = len(LINES[ed.cy])
				renderAll(ed)
			}
		case Char:
			currentLine := LINES[ed.cy]
			newLine := make([]rune, 0, len(currentLine)+1)
			newLine = append(newLine, currentLine[:ed.cx]...)
			newLine = append(newLine, ev.Rune)
			newLine = append(newLine, currentLine[ed.cx:]...)
			LINES[ed.cy] = newLine
			ed.cx++
			renderAll(ed)
		case BackSpace:
			if ed.cx > 0 {
				currentLine := LINES[ed.cy]
				newLine := make([]rune, 0, len(currentLine)-1)
				newLine = append(newLine, currentLine[:ed.cx-1]...)
				newLine = append(newLine, currentLine[ed.cx:]...)
				LINES[ed.cy] = newLine
				ed.cx--
				renderAll(ed)
			} else if ed.cy > 0 {
				prevLineIdx := ed.cy - 1
				ed.cx = len(LINES[prevLineIdx])
				mergedLine := make([]rune, 0, len(LINES[prevLineIdx])+len(LINES[ed.cy]))
				mergedLine = append(mergedLine, LINES[prevLineIdx]...)
				mergedLine = append(mergedLine, LINES[ed.cy]...)
				LINES[prevLineIdx] = mergedLine
				LINES = append(LINES[:ed.cy], LINES[ed.cy+1:]...)
				ed.cy--
				renderAll(ed)
			}
		case WordBackSpace:
			if ed.cx > 0 {
				currentLine := LINES[ed.cy]

				newCx := ed.cx

				for newCx > 0 && unicode.IsSpace(currentLine[newCx-1]) {
					newCx--
				}

				for newCx > 0 && !unicode.IsSpace(currentLine[newCx-1]) {
					newCx--
				}

				newLine := make([]rune, 0, len(currentLine)-(ed.cx-newCx))
				newLine = append(newLine, currentLine[:newCx]...)
				newLine = append(newLine, currentLine[ed.cx:]...)
				LINES[ed.cy] = newLine

				ed.cx = newCx
				renderAll(ed)
			} else if ed.cy > 0 {

				prevLineIdx := ed.cy - 1
				ed.cx = len(LINES[prevLineIdx])
				mergedLine := make([]rune, 0, len(LINES[prevLineIdx])+len(LINES[ed.cy]))
				mergedLine = append(mergedLine, LINES[prevLineIdx]...)
				mergedLine = append(mergedLine, LINES[ed.cy]...)
				LINES[prevLineIdx] = mergedLine
				LINES = append(LINES[:ed.cy], LINES[ed.cy+1:]...)
				ed.cy--
				renderAll(ed)
			}
		case Enter:
			currentLine := LINES[ed.cy]
			remainingText := []rune{}
			if ed.cx < len(currentLine) {
				remainingText = make([]rune, len(currentLine[ed.cx:]))
				copy(remainingText, currentLine[ed.cx:])
				LINES[ed.cy] = currentLine[:ed.cx]
			} else {
				LINES[ed.cy] = currentLine
			}
			newLines := make([][]rune, 0, len(LINES)+1)
			newLines = append(newLines, LINES[:ed.cy+1]...)
			newLines = append(newLines, remainingText)
			newLines = append(newLines, LINES[ed.cy+1:]...)
			LINES = newLines
			ed.cy++
			ed.cx = 0
			renderAll(ed)
		}
	}
}

func renderAll(ed editor) {
	fmt.Print("\033[H\033[J")

	tabStop := 4
	visualCx := 0

	for i, line := range LINES {
		fmt.Printf("\033[%d;1H", i+1)

		var visualLine string
		currentCol := 0

		for idx, r := range line {
			if i == ed.cy && idx == ed.cx {
				visualCx = currentCol
			}

			if r == '\t' {
				spaces := tabStop - (currentCol % tabStop)
				for s := 0; s < spaces; s++ {
					visualLine += " "
					currentCol++
				}
			} else {
				visualLine += string(r)
				currentCol++
			}
		}

		if i == ed.cy && ed.cx == len(line) {
			visualCx = currentCol
		}

		fmt.Print(visualLine)
	}

	fmt.Printf("\033[%d;%dH", ed.cy+1, visualCx+1)
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
		if r == 0x03 {
			break
		}

		if r == 0x17 {
			dataChan <- event{Type: WordBackSpace}
			continue
		}
		if r == 0x7f || r == 0x08 {

			dataChan <- event{Type: BackSpace}
			continue
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

			if err == nil && (peek[0] == 0x7f || peek[0] == 0x08) {
				reader.Discard(1)
				dataChan <- event{Type: WordBackSpace}
				continue
			}

			dataChan <- event{Type: Escape}
			continue
		}

		switch r {
		case '\r', '\n':
			dataChan <- event{Type: Enter}
		case '\t':
			dataChan <- event{Type: Char, Rune: '\t'}
		default:
			dataChan <- event{Type: Char, Rune: r}
		}
	}
}
