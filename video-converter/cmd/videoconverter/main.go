package main

import (
	"path/filepath"
	"regexp"
	"strconv"
)

func main() {
	println("Hello, World")
}

func extractNumber(fileName string) int {
	re := regexp.MustCompile(`\d+`)
	numStr := re.FindString(filepath.Base(fileName))
	num, err := strconv.Atoi(numStr)

	if err != nil {
		return -1
	}

	return num
}
