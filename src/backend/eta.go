package main

import (
	"encoding/json"
	"fmt"

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

		// Load duration list
		durationListByte := jb.Get([]byte("durationList"))
		var durationList []int

		json.Unmarshal(durationListByte, &durationList)

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

// GetJobETA returns ETA for job to complete, s
func GetJobETA(jobName string) int {
	var eta int
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(JobsBucket)
		jb := b.Bucket([]byte(jobName))
		if jb == nil {
			return fmt.Errorf("job with name %s is not found in JobsBucket", jobName)
		}

		// Load duration list
		durationListByte := jb.Get([]byte("durationList"))
		var durationList []int

		err := json.Unmarshal(durationListByte, &durationList)
		if err != nil {
			return err
		}

		eta = calcAvg(&durationList)
		return nil
	})

	if err != nil {
		Logger.Println(err)
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
