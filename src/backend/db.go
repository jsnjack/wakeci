package main

import (
	"encoding/binary"
	"strconv"
)

// DB schema
// key `jobs`
//   key `job_name`
// 	- count

// JobsBucket ...
var JobsBucket = []byte("jobs")

// GlobalBucket contains information about global configuration
// - count: id of the build, increments
var GlobalBucket = []byte("global")

// HistoryBucket contains information about all executed builds
var HistoryBucket = []byte("history")

// ByteToInt convert bytes to int
func ByteToInt(b []byte) (int, error) {
	bs := string(b)
	bi, err := strconv.Atoi(bs)
	if err != nil {
		return 0, err
	}
	return bi, nil
}

// Itob converts int to
func Itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// Btoi converts bytes to int
func Btoi(b []byte) int {
	return int(binary.BigEndian.Uint64(b))
}

// IsBuildAborted verifies that a build is not aborted
func IsBuildAborted(id int) bool {
	for _, item := range BuildList {
		if item.ID == id {
			return false
		}
	}
	for _, item := range BuildQueue {
		if item.ID == id {
			return false
		}
	}
	return true
}
