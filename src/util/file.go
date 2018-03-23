package util

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strings"
)

// ReadLines reads io.Reader line by line, not for huge lines
func ReadLines(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// LoadFileToReader provide whole file in io.Reader interface
func LoadFileToReader(path string) (io.Reader, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(file), err
}
