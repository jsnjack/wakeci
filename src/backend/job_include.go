package main

import (
	"io/ioutil"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

// ExpandInclude replaces include keyword with extracted tasks
func ExpandInclude(tasks *[]*Task) error {
	finished := false
	for !finished {
		for idx, t := range *tasks {
			if t.IncludePath != "" {
				Logger.Printf("Expanding include %s...\n", t.IncludePath)
				toInclude, err := ReadTasks(t.IncludePath)
				if err != nil {
					return err
				}
				// Delete "included" item
				*tasks = append((*tasks)[:idx], (*tasks)[idx+1:]...)
				// Insert new items
				for tiidx, ti := range toInclude {
					*tasks = append((*tasks)[:idx+tiidx], append([]*Task{ti}, (*tasks)[idx+tiidx:]...)...)
					// Copy environment and conditions
					(*tasks)[idx+tiidx].Env = t.Env
					(*tasks)[idx+tiidx].When = t.When
					(*tasks)[idx+tiidx].Kind = t.Kind
				}
				break
			}
		}
		finished = true
	}
	return nil
}

// ReadTasks returns parsed tasks from the file
func ReadTasks(path string) ([]*Task, error) {
	data, err := ioutil.ReadFile(filepath.Join(Config.JobDir, path))
	if err != nil {
		return nil, err
	}
	tasks := make([]*Task, 0)
	err = yaml.Unmarshal(data, &tasks)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}
