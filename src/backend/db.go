package main

import (
	"encoding/binary"
	"strconv"
)

// DB schema
// key `jobs`
//   key `job_name`
// 	- count

// JobsBucket contains all registered jobs
// Schema (key is the name of the file):
// | defaultParams | null    |
// | desc          | New job |
// | interval      |         |
// | active        | true    |
var JobsBucket = []byte("jobs")

// GlobalBucket contains information about global configuration
// - count: id of the build, increments
var GlobalBucket = []byte("global")

// HistoryBucket contains information about all executed builds
var HistoryBucket = []byte("history")

// ByteToInt convert byte to int via string
func ByteToInt(b []byte) (int, error) {
	bs := string(b)
	bi, err := strconv.Atoi(bs)
	if err != nil {
		return 0, err
	}
	return bi, nil
}

// IntToByte converts integer to byte via string
func IntToByte(i int) []byte {
	s := strconv.Itoa(i)
	return []byte(s)
}

// Itob converts int to sorted byte array
func Itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}
