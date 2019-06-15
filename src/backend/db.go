package main

import "strconv"

// DB schema
// key `jobs`
//   key `job_name`
// 	- count

// JobsBucket ...
var JobsBucket = []byte("jobs")

// GlobalBucket contains information about global configuration
// - count: id of the build, increments
var GlobalBucket = []byte("global")

// ByteToInt convert bytes to int
func ByteToInt(b []byte) (int, error) {
	bs := string(b)
	bi, err := strconv.Atoi(bs)
	if err != nil {
		return 0, err
	}
	return bi, nil
}
