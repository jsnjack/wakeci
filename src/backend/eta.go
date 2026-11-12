package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	bolt "go.etcd.io/bbolt"
)

// DurationWindowLength shows how many duration samples are stored to calculate ETA
const DurationWindowLength = 5

// RecordBuildDuration saves build duration in JobsBucket
func RecordBuildDuration(jobName string, duration int) error {
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(JobsBucket)
		jb := b.Bucket([]byte(jobName))
		if jb == nil {
			return fmt.Errorf("job with name %s is not found in JobsBucket", jobName)
		}

		// Load duration list. The key doesn't exist yet for a job that
		// hasn't completed a build, which is a normal state, not an error.
		durationListByte := jb.Get([]byte("durationList"))
		var durationList []int
		if durationListByte != nil {
			if err := json.Unmarshal(durationListByte, &durationList); err != nil {
				slog.Log(context.Background(), LevelTrace, "unmarshal duration list", "job", jobName, "err", err)
			}
		}

		durationList = append(durationList, duration)
		// Shift duration list
		if len(durationList) > DurationWindowLength {
			durationList = durationList[1:]
		}

		// Save duration list
		newListByte, err := json.Marshal(durationList)
		if err != nil {
			return err
		}
		return jb.Put([]byte("durationList"), newListByte)
	})
	return err
}

// GetJobETA returns how long a build of the job is expected to take, in
// nanoseconds: the average of its last DurationWindowLength finished builds,
// 0 until it has that many
func GetJobETA(jobName string) int {
	var eta int
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(JobsBucket)
		jb := b.Bucket([]byte(jobName))
		if jb == nil {
			return fmt.Errorf("job with name %s is not found in JobsBucket", jobName)
		}

		// Load duration list. The key doesn't exist yet for a job that
		// hasn't completed a build, which is a normal state, not an error.
		durationListByte := jb.Get([]byte("durationList"))
		var durationList []int
		if durationListByte != nil {
			if err := json.Unmarshal(durationListByte, &durationList); err != nil {
				return err
			}
		}

		eta = calcAvg(&durationList)
		return nil
	})

	if err != nil {
		L.Error("get job eta", "job", jobName, "err", err)
	}
	return eta
}

func calcAvg(durationList *[]int) int {
	var eta int
	var sum int
	for _, item := range *durationList {
		sum += item
	}
	if len(*durationList) >= DurationWindowLength {
		eta = sum / len(*durationList)
	}
	return eta
}
