package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	bolt "go.etcd.io/bbolt"
)

// Builds started by other builds (F2 in docs/ui-roadmap.md). A job that
// starts others adds WAKE_PARENT=$WAKE_BUILD_ID to the run call, and the new
// build records its parent and the root, the first build of the chain. Not
// "parent", which a job may have as a parameter.
const (
	parentParam = "WAKE_PARENT"
	// againParam names the build that a run runs again, whose parent and
	// root the new build keeps, even after the parent ended
	againParam = "WAKE_AGAIN"
)

// GroupsBucket holds a bucket for each root, with its members' IDs as keys,
// so a group is read without scanning the history
var GroupsBucket = []byte("groups")

// buildGroup is where a build sits in a group: both 0 for a build that no
// build started
type buildGroup struct {
	Parent int
	Root   int
}

// errNoParent is the answer for a parent that doesn't exist and for one the
// caller may not read, so the second can't be told from the first
func errNoParent(id int) error {
	return fmt.Errorf("there is no build %d", id)
}

// groupOf returns the group of a run of job from the request's WAKE_PARENT
// or WAKE_AGAIN. The parent must still be in the queue, so no one can hang
// builds under an old build and a chain can't loop. Run again keeps the
// group of the build it runs again, of the same job, so it can't hang other
// jobs under a group either. Both must be builds the caller may read.
func groupOf(r *http.Request, job string) (buildGroup, error) {
	parentValue, againValue := r.Form.Get(parentParam), r.Form.Get(againParam)
	if parentValue == "" && againValue == "" {
		return buildGroup{}, nil
	}
	if parentValue != "" && againValue != "" {
		return buildGroup{}, fmt.Errorf("send %s or %s, not both", parentParam, againParam)
	}
	access := accessFromContext(r.Context())
	if againValue != "" {
		id, err := runBuildID(againParam, againValue)
		if err != nil {
			return buildGroup{}, err
		}
		build, err := readBuildStatus(id)
		if errors.Is(err, errBuildNotFound) || (err == nil && !access.Reads(build.Name)) {
			return buildGroup{}, errNoParent(id)
		}
		if err != nil {
			return buildGroup{}, err
		}
		if build.Name != job {
			return buildGroup{}, fmt.Errorf("build %d is a build of %s, not of %s", id, build.Name, job)
		}
		return buildGroup{Parent: build.Parent, Root: build.Root}, nil
	}
	id, err := runBuildID(parentParam, parentValue)
	if err != nil {
		return buildGroup{}, err
	}
	parent := GlobalQueue.Live(id)
	if parent == nil {
		ended, err := readBuildStatus(id)
		if err == nil && access.Reads(ended.Name) {
			return buildGroup{}, fmt.Errorf("build %d has ended", id)
		}
		return buildGroup{}, errNoParent(id)
	}
	if !access.Reads(parent.Job.Name) {
		return buildGroup{}, errNoParent(id)
	}
	group := buildGroup{Parent: id, Root: parent.Group().Root}
	if group.Root == 0 {
		group.Root = id
	}
	return group, nil
}

// runBuildID reads the build ID that a run sends as name
func runBuildID(name, value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 || strconv.Itoa(id) != value {
		return 0, fmt.Errorf("%s must be a build ID, such as $WAKE_BUILD_ID", name)
	}
	return id, nil
}

// Group returns where the build sits in a group
func (b *Build) Group() buildGroup {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.group
}

// putGroupMember records build id as a member of its root's group
func putGroupMember(tx *bolt.Tx, root, id int) error {
	if root == 0 {
		return nil
	}
	groups, err := tx.CreateBucketIfNotExists(GroupsBucket)
	if err != nil {
		return err
	}
	members, err := groups.CreateBucketIfNotExists(Itob(root))
	if err != nil {
		return err
	}
	return members.Put(Itob(id), nil)
}

// groupMembers returns the IDs of the builds in root's group, in the order
// they were created, without root itself
func groupMembers(tx *bolt.Tx, root int) []int {
	groups := tx.Bucket(GroupsBucket)
	if groups == nil {
		return nil
	}
	members := groups.Bucket(Itob(root))
	if members == nil {
		return nil
	}
	var ids []int
	_ = members.ForEach(func(k, _ []byte) error {
		ids = append(ids, int(binary.BigEndian.Uint64(k)))
		return nil
	})
	return ids
}

// deleteGroupMember forgets build id, a member of root's group, and the
// group once it has no members
func deleteGroupMember(tx *bolt.Tx, root, id int) error {
	groups := tx.Bucket(GroupsBucket)
	if groups == nil || root == 0 {
		return nil
	}
	members := groups.Bucket(Itob(root))
	if members == nil {
		return nil
	}
	if err := members.Delete(Itob(id)); err != nil {
		return err
	}
	if k, _ := members.Cursor().First(); k == nil {
		return groups.DeleteBucket(Itob(root))
	}
	return nil
}
