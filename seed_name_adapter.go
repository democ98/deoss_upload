package main

import (
	"log"
	"strconv"
	"strings"
)

func SicHubAdapter(originalFilename string) (newFilename string) {
	newFilenameInt := 0
	originalFilenameWithOutZero := ""
	originalFilenameWithOutZero, ok := strings.CutPrefix(originalFilename, "00")
	if !ok {
		originalFilenameWithOutZero, ok = strings.CutPrefix(originalFilename, "0")
		if !ok {
			var err error
			newFilenameInt, err = strconv.Atoi(originalFilenameWithOutZero)
			if err != nil {
				log.Printf("this file is not belong to sci-hub seed~ return input filename\n")
				return originalFilename
			}
		}
	}
	newFilenameInt, err := strconv.Atoi(originalFilenameWithOutZero)
	if err != nil {
		log.Fatalf("recover uploadFileNameWithOutZero to number fail:%v\n", err)
	}
	right_number := newFilenameInt + 99999
	right_number_string := strconv.Itoa(right_number)

	for i := 0; i < 8-len(right_number_string); i++ {
		right_number_string = "0" + right_number_string
	}

	newFilename = "sm_" + originalFilename + "-" + right_number_string + ".txt"

	return
}
