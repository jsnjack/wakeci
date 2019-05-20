package main

import (
	"bytes"
	"encoding/binary"
)

// JobsBucket ...
var JobsBucket = []byte("jobs")

// itob returns an 8-byte big endian representation of v.
func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func btoi(data []byte) (ret int) {
	buf := bytes.NewBuffer(data)
	binary.Read(buf, binary.BigEndian, &ret)
	return ret
}
