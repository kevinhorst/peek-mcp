package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type prompter struct {
	scanner *bufio.Scanner
	out     io.Writer
	auto    bool
}

func newPrompter() *prompter {
	return &prompter{scanner: bufio.NewScanner(os.Stdin), out: os.Stdout}
}

func autoPrompter() *prompter {
	return &prompter{auto: true, out: os.Stdout}
}

func (p *prompter) Confirm(question string, defaultYes bool) bool {
	if p.auto {
		return true
	}
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	fmt.Fprintf(p.out, "%s [%s]: ", question, hint)
	if !p.scanner.Scan() {
		return defaultYes
	}
	switch strings.ToLower(strings.TrimSpace(p.scanner.Text())) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultYes
	}
}

func (p *prompter) Ask(question, defaultValue string) string {
	if defaultValue == "" {
		fmt.Fprintf(p.out, "%s: ", question)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", question, defaultValue)
	}
	if !p.scanner.Scan() {
		return defaultValue
	}
	text := strings.TrimSpace(p.scanner.Text())
	if text == "" {
		return defaultValue
	}
	return text
}

func (p *prompter) Choose(question string, options []string, defaultIdx int) int {
	if p.auto {
		return defaultIdx
	}
	fmt.Fprintln(p.out, question)
	for i, opt := range options {
		fmt.Fprintf(p.out, "  [%d] %s\n", i+1, opt)
	}
	fmt.Fprintf(p.out, "Choice [%d]: ", defaultIdx+1)
	if !p.scanner.Scan() {
		return defaultIdx
	}
	text := strings.TrimSpace(p.scanner.Text())
	if text == "" {
		return defaultIdx
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > len(options) {
		return defaultIdx
	}
	return n - 1
}
