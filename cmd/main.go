package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

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
	Unkown
)

var BUFFER strings.Builder // created temporarly will add rope later

type event struct {
	Type KeyType
	Rune rune
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
	dataChan := make(chan event)

	go read(fd, ctx, dataChan)

	for ev := range dataChan {
		switch ev.Type {
		case UpArrow:
			fmt.Fprintf(os.Stdout, "\033[A")
		case DownArrow:
			fmt.Fprintf(os.Stdout, "\033[B")
		case RightArrow:
			fmt.Fprintf(os.Stdout, "\033[C")
		case LeftArrow:
			fmt.Fprintf(os.Stdout, "\033[D")

		case Char:
			BUFFER.WriteRune(ev.Rune)
			fmt.Print(string(ev.Rune))

		case BackSpace:
			str := BUFFER.String()
			if len(str) > 0 {
				BUFFER.Reset()
				BUFFER.WriteString(str[:len(str)-1])
			}
			fmt.Print("\b \b")
		case Enter:
			fmt.Print("\r\n")
		}
	}

	fmt.Print("closing channel\n")
}

func read(fd uintptr, ctx context.Context, dataChan chan<- event) {
	defer close(dataChan)

	reader := bufio.NewReader(os.Stdin)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(os.Stderr, "read error")
			break
		}

		if b == 0x03 {
			fmt.Print("exiting...\r\n")
			break
		}

		if b == 0x1b {
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
					dataChan <- event{Type: Unkown}
				}
				continue
			}

			dataChan <- event{Type: Escape}
			continue
		}

		switch b {
		case '\r', '\n':
			dataChan <- event{Type: Enter}
		case 0x7f, 0x8f:
			dataChan <- event{Type: BackSpace}
		default:
			dataChan <- event{Type: Char, Rune: rune(b)}
		}
	}
}
