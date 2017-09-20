package main

import (
	"bytes"
)

func GetReqCmd(data []byte) []string {
	reqs := make([]string, 0)
	lines := bytes.Split(data, []byte{0x0d, 0x0a})
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '*', '$', '+', '-', ':':
		default:
			reqs = append(reqs, string(line))
		}
	}
	return reqs
}
