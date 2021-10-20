package main

import (
	"io/ioutil"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

// ExpandTasks :
// - replaces include keyword with extracted tasks
// - moves tasks outside of blocks statement
func ExpandTasks(tasks *[]*Task) error {
	finished := false
	for !finished {
		for idx, t := range *tasks {
			// Handle `include`
			if t.IncludePath != "" {
				Logger.Printf("Expanding include %s...\n", t.IncludePath)
				toInclude, err := ReadTasks(t.IncludePath)
				if err != nil {
					return err
				}
				injectExpandedTasks(t, idx, toInclude, tasks)
				break
			}

			// Handle `block`
			if t.Block != nil {
				Logger.Printf("Expanding block %v...\n", t.Name)
				injectExpandedTasks(t, idx, t.Block, tasks)
				break
			}
		}

		allExpanded := true
		for _, vt := range *tasks {
			if vt.IncludePath != "" || vt.Block != nil {
				allExpanded = false
				break
			}
		}

		if allExpanded {
			finished = true
		}
	}
	return nil
}

func injectExpandedTasks(t *Task, pos int, toInject []*Task, tasks *[]*Task) {
	// Delete "included" item
	*tasks = append((*tasks)[:pos], (*tasks)[pos+1:]...)
	// Insert new items
	for i, ti := range toInject {
		*tasks = append((*tasks)[:pos+i], append([]*Task{ti}, (*tasks)[pos+i:]...)...)
		// Copy environment and conditions
		if t.Env != nil {
			(*tasks)[pos+i].Env = t.Env
		}
		if t.When != "" {
			if (*tasks)[pos+i].When != "" {
				(*tasks)[pos+i].When += " && "
			}
			(*tasks)[pos+i].When += t.When
		}
		(*tasks)[pos+i].Kind = t.Kind
	}
}

// ReadTasks returns parsed tasks from the file
func ReadTasks(path string) ([]*Task, error) {
	includePath := filepath.Join(Config.JobDir, path)
	if filepath.IsAbs(path) {
		includePath = path
	}
	data, err := ioutil.ReadFile(includePath)
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
